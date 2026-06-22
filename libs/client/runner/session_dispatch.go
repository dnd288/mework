package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"mework/libs/sandbox/runtime"
	"mework/libs/server/bus"
	"mework/libs/server/bus/memory"
	"mework/libs/server/session"
	"mework/libs/shared/core"
	"mework/libs/shared/grant"
	"mework/libs/shared/transport"
)

// Injectable constructors for the session path. They are package-level variables
// so the daemon can wire concrete implementations (an HTTP catalog resolver, an
// HTTP events broker) without the runner package importing the catalog package
// (which would form an import cycle), and so tests can substitute fakes.
var (
	// sessionResolverFor builds the DefinitionResolver used to resolve a session's
	// agent definition over the catalog. When nil the session path cannot resolve.
	sessionResolverFor func(catalogURL string) DefinitionResolver

	// sessionBrokerFor builds the bus.Broker the session's EventPublisher uses to
	// egress per-turn events to the server (an httpBroker POSTing to the events
	// endpoint). This is separate from the in-process broker the session.Manager
	// uses for its own lifecycle bookkeeping.
	sessionBrokerFor = func(hubURL, secret string) bus.Broker {
		return newHTTPBroker(hubURL, secret)
	}

	// sessionManagerFor builds the session.Manager that owns the session
	// lifecycle. The manager needs a fully-functional (Subscribe-capable)
	// in-process broker, so it gets its own memory broker — distinct from the
	// egress (events) broker.
	sessionManagerFor = func() *session.Manager {
		return session.NewManager(memory.New(), session.DefaultConfig())
	}

	// sessionRuntimeManagerFor maps an engine name to a sandbox runtime.Manager
	// for the opened session. When nil, OpenSession falls back to the default
	// local-by-default engine dispatch. Tests inject a fake-driver manager here.
	sessionRuntimeManagerFor func(engine string) *runtime.Manager
)

// SetSessionResolverFactory wires the factory used to build the catalog-backed
// definition resolver for interactive sessions. The daemon calls this at startup
// (the runner package cannot import the catalog package directly).
func SetSessionResolverFactory(f func(catalogURL string) DefinitionResolver) {
	sessionResolverFor = f
}

// processSessionDispatch drives an open-session (interactive) dispatch: it
// verifies the dispatch grant (requiring pull+spawn), builds the owning caller
// from the dispatch's owner/tenant, resolves the agent definition, opens a
// long-lived sandbox exactly once, registers the session by id, acks the
// dispatch, and starts a serial input loop that routes turns from the session's
// input topic to the live session.
func processSessionDispatch(ctx context.Context, e *Engine, d transport.Dispatch, eventID string, opts processOpts) error {
	// 1. Parse and verify the grant; enforce pull + spawn.
	g, err := parseAndVerifyGrant(d.Grant, []byte(opts.secret))
	if err != nil {
		return ackAndReturn(ctx, opts, eventID, fmt.Errorf("grant verification failed: %w", err))
	}
	if err := enforceGrant(g, grant.OpSpawn); err != nil {
		reportRefused(ctx, opts, d.Session, fmt.Sprintf("spawn not granted: %v", err))
		return ackAndReturn(ctx, opts, eventID, err)
	}

	// 2. Build the owning caller from the dispatch.
	caller := Caller{
		Account: core.AccountID(d.Owner),
		Tenant:  core.TenantID(d.Tenant),
		Grant:   g,
	}

	// 3. Resolve the definition and build session deps.
	if sessionResolverFor == nil {
		return ackAndReturn(ctx, opts, eventID, fmt.Errorf("no session resolver configured"))
	}
	resolver := sessionResolverFor(opts.catalogURL)
	eventsBroker := sessionBrokerFor(opts.hubURL, opts.secret)
	mgr := sessionManagerFor()

	deps := SessionDeps{
		Resolver:   resolver,
		ManagerFor: sessionRuntimeManagerFor,
		Broker:     eventsBroker,
		Sessions:   mgr,
		GrantKey:   []byte(opts.secret),
	}

	// 4. Open the session's sandbox exactly once.
	sess, err := OpenSession(ctx, d.Agent.Name+"@"+d.Agent.Version, caller, deps)
	if err != nil {
		return ackAndReturn(ctx, opts, eventID, fmt.Errorf("open session: %w", err))
	}

	// Register before acking so a duplicate dispatch sees the session as open.
	if !e.registerSession(d.Session, sess) {
		// A concurrent dispatch already opened it: tear down our extra sandbox.
		_ = sess.Close(ctx, caller)
		return opts.client.AckMessage(opts.secret, eventID)
	}

	// 5. Ack the open-session dispatch.
	if ackErr := opts.client.AckMessage(opts.secret, eventID); ackErr != nil {
		log.Printf("ack open-session dispatch failed: %v", ackErr)
	}

	// 6. Start the serial input loop for this session.
	e.runSessionInputLoop(ctx, d.Session, sess, caller)
	return nil
}

// inputMessage is the payload carried on a session's input topic. A normal turn
// carries a chat message; a control message (close/cancel) ends or interrupts
// the session.
type inputMessage struct {
	// Control, when set, is "close" or "cancel"; otherwise the message is a turn.
	Control string `json:"control,omitempty"`
	// Message is the chat turn content (used when Control is empty).
	Message session.ChatMessage `json:"message,omitempty"`
}

// runSessionInputLoop subscribes to the session's input topic and routes each
// message to the live session serially (one goroutine per session), preserving
// the one-agent-per-sandbox invariant. A close/cancel control message maps to
// Session.Close/Cancel and (on close) removes the session from the registry.
func (e *Engine) runSessionInputLoop(ctx context.Context, sessionID string, sess *Session, caller Caller) {
	topic := bus.FormatTopic(bus.TopicSessionInput, sessionID)
	stream, err := e.client.Subscribe(e.secret, []string{string(topic)}, "")
	if err != nil {
		log.Printf("subscribe to session input %q failed: %v", topic, err)
		return
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer stream.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case <-e.stopCh:
				return
			case ev, ok := <-stream.Events():
				if !ok {
					return
				}
				if done := e.handleSessionInput(ctx, sessionID, sess, caller, ev); done {
					return
				}
			}
		}
	}()
}

// handleSessionInput processes one input-topic event. It returns true when the
// session has ended (close) and the loop should stop.
func (e *Engine) handleSessionInput(ctx context.Context, sessionID string, sess *Session, caller Caller, ev bus.Event) bool {
	var msg inputMessage
	if err := json.Unmarshal(ev.Message.Payload, &msg); err != nil {
		log.Printf("decode session input for %q: %v", sessionID, err)
		return false
	}

	switch msg.Control {
	case "close":
		if err := sess.Close(ctx, caller); err != nil {
			log.Printf("close session %q: %v", sessionID, err)
		}
		e.removeSession(sessionID)
		return true
	case "cancel":
		if err := sess.Cancel(ctx, caller); err != nil {
			log.Printf("cancel session %q: %v", sessionID, err)
		}
		return false
	default:
		// A normal chat turn: route serially to the live session.
		if err := sess.Send(ctx, caller, msg.Message.Content); err != nil {
			log.Printf("send turn to session %q: %v", sessionID, err)
		}
		return false
	}
}
