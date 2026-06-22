package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"mework/libs/server/bus"
	"mework/libs/shared/grant"
	"mework/libs/shared/transport"
)

// runnerShortName strips the "runner-" prefix from a runner ID if present,
// so that FormatTopic produces "runner.<name>.dispatch" rather than
// "runner.runner-<name>.dispatch".
func runnerShortName(runnerID string) string {
	return strings.TrimPrefix(runnerID, "runner-")
}

// agentExists checks whether an agent with the given name is known to the
// system, consulting the in-memory store first and falling back to the
// DB-backed service when available.
func (h *AgentHandlers) agentExists(ctx context.Context, name string) (bool, error) {
	h.mu.RLock()
	_, inMem := h.agents[name]
	h.mu.RUnlock()
	if inMem {
		return true, nil
	}

	if h.service != nil && h.service.pool != nil {
		_, err := h.service.LookupAgentByName(ctx, name)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return false, err
		}
		return false, nil
	}

	return false, nil
}

// DispatchToRunner publishes a dispatch message to the target runner's topic.
func (h *AgentHandlers) DispatchToRunner(ctx context.Context, agentName, runnerID string, g *grant.Grant) error {
	return h.dispatch(ctx, agentName, "", runnerID, g, "")
}

// DispatchToRunnerWithChannel publishes a dispatch message with channel context
// so the worker can subscribe to the correct channel topic.
func (h *AgentHandlers) DispatchToRunnerWithChannel(ctx context.Context, agentName, runnerID string, g *grant.Grant, channelKey string) error {
	return h.dispatch(ctx, agentName, "", runnerID, g, channelKey)
}

// dispatch is the shared implementation for dispatching an agent to a runner.
func (h *AgentHandlers) dispatch(ctx context.Context, agentName, version, runnerID string, g *grant.Grant, channelKey string) error {
	if g == nil {
		return fmt.Errorf("dispatch requires a grant")
	}
	if agentName == "" {
		return fmt.Errorf("agent name is required")
	}

	exists, err := h.agentExists(ctx, agentName)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}

	agentRef := transport.AgentRef{Name: agentName}
	if version != "" {
		agentRef.Version = version
	}
	grantJSON, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("marshal grant: %w", err)
	}

	msg := transport.Dispatch{
		Agent:      agentRef,
		Grant:      grantJSON,
		Runner:     runnerID,
		ChannelKey: channelKey,
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal dispatch: %w", err)
	}

	topic := bus.FormatTopic(bus.TopicRunnerDispatch, runnerShortName(runnerID))
	return h.broker.Publish(ctx, topic, bus.Message{Payload: payload})
}

// DispatchSessionToRunner publishes an open-session dispatch to the named
// runner. A non-empty session id marks this as an open-session dispatch; the
// daemon (c0033) branches on it to open a long-lived sandbox rather than run a
// one-shot agent. Owner and tenant ride along so the runner can authorize the
// session's turns. It publishes to the same topic the daemon Engine subscribes
// to for runnerID, so the two are guaranteed to match.
func (h *AgentHandlers) DispatchSessionToRunner(ctx context.Context, agentName, runnerID, sessionID, owner, tenant string, g *grant.Grant) error {
	if g == nil {
		return fmt.Errorf("dispatch requires a grant")
	}
	if agentName == "" {
		return fmt.Errorf("agent name is required")
	}

	exists, err := h.agentExists(ctx, agentName)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}

	grantJSON, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("marshal grant: %w", err)
	}

	msg := transport.Dispatch{
		Agent:   transport.AgentRef{Name: agentName},
		Grant:   grantJSON,
		Session: sessionID,
		Owner:   owner,
		Tenant:  tenant,
		Runner:  runnerID,
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal dispatch: %w", err)
	}

	topic := bus.FormatTopic(bus.TopicRunnerDispatch, runnerID)
	return h.broker.Publish(ctx, topic, bus.Message{Payload: payload})
}

// DispatchVersionToRunner publishes a dispatch message for a specific agent version.
func (h *AgentHandlers) DispatchVersionToRunner(ctx context.Context, agentName, version, runnerID string, g *grant.Grant) error {
	return h.dispatch(ctx, agentName, version, runnerID, g, "")
}

// DispatchVersionToRunnerWithChannel publishes a dispatch for a specific version with channel context.
func (h *AgentHandlers) DispatchVersionToRunnerWithChannel(ctx context.Context, agentName, version, runnerID string, g *grant.Grant, channelKey string) error {
	return h.dispatch(ctx, agentName, version, runnerID, g, channelKey)
}
