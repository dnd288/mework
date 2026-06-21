package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"mework/client/subscribe"
	"mework/sandbox/agent"
	"mework/sandbox/engine/local"
	"mework/sandbox/runtime"
	"mework/server/bus"
	"mework/shared/config"
	"mework/shared/core"
)

// Run is the daemon's main loop: subscribe to the SSE message bus for the
// given profile's dispatch topic, run the AI agent on received jobs, and ack
// completion.
func Run(ctx context.Context, profile string, cfg *config.Config) error {
	if cfg.ServerURL == "" {
		return errors.New("server_url is not set in config; please configure it first")
	}
	if cfg.RuntimeToken == "" {
		return errors.New("rt_token is not set in config; please register this runtime first")
	}

	backend, ok := agent.Detect(cfg.Daemon.Backends)
	if !ok {
		log.Printf("warning: no AI CLI detected (looked for %v); triggers will be skipped until backend is installed", agent.DefaultBackends)
	} else {
		log.Printf("using AI backend %s (%s)", backend.Name, backend.Path)
	}

	// Select sandbox driver from config (default "local").
	mgr := runtime.NewManagerFor(cfg.Daemon.SandboxEngine)
	caps := mgr.Caps()
	log.Printf("using sandbox driver %q (isolated=%v)", caps.DriverName, caps.IsIsolated)

	client := subscribe.NewClient(cfg.ServerURL, 10*time.Second)

	topic := bus.FormatTopic(bus.TopicRunnerDispatch, profile)
	var lastEventID string

	for {
		select {
		case <-ctx.Done():
			log.Println("daemon stopping")
			return nil
		default:
		}

		if !ok {
			backend, ok = agent.Detect(cfg.Daemon.Backends)
			if !ok {
				// No backend detected; wait before retry.
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(5 * time.Second):
				}
				continue
			}
			log.Printf("detected AI backend %s (%s)", backend.Name, backend.Path)
		}

		log.Printf("subscribing to topic %s (last_event_id=%q)", topic, lastEventID)
		stream, err := client.Subscribe(cfg.RuntimeToken, []string{string(topic)}, lastEventID)
		if err != nil {
			log.Printf("error subscribing: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
			continue
		}

		log.Printf("connected to SSE stream for %s", profile)

	eventLoop:
		for {
			select {
			case <-ctx.Done():
				stream.Close()
				log.Println("daemon stopping")
				return nil
			case event, ok := <-stream.Events():
				if !ok {
					// Stream closed by server; reconnect.
					break eventLoop
				}
			log.Printf("received event %s on topic %s", event.ID, event.Topic)

			// Deserialize job from the event payload.
			var job subscribe.Job
			if err := json.Unmarshal(event.Message.Payload, &job); err != nil {
				log.Printf("error deserializing job from event %s: %v", event.ID, err)
				lastEventID = event.ID
				continue
			}

			log.Printf("processing job %s for task %s", job.ID, job.ExternalTaskID)

			// Transition to running
			if err := client.Ack(cfg.RuntimeToken, job.ID, "running", "", ""); err != nil {
				log.Printf("error acking running status for job %s: %v", job.ID, err)
				lastEventID = event.ID
				continue
			}

			// Start heartbeat (extends lease in background every 30s)
			stopHeartbeat := startHeartbeat(ctx, client, cfg.RuntimeToken, job.ID, 30*time.Second)

			// Prepare prompt
			prompt := buildPrompt(&job)
			workDir := local.WorkDir(config.ProfileDir(profile), job.ID)

			// Execute AI agent through the sandbox driver
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			spec := core.RunSpec{
				AgentID:     backend.Name,
				BackendPath: backend.Path,
				BackendName: backend.Name,
				SandboxID:   workDir,
				Timeout:     30 * time.Minute,
			}
			s, startErr := mgr.Start(runCtx, spec)

			var res core.Result
			if startErr != nil {
				cancel()
				res = core.Result{Error: fmt.Sprintf("start sandbox: %v", startErr)}
			} else {
				var stdout, stderr bytes.Buffer
				exitCode, execErr := s.Exec(runCtx, []string{backend.Path}, bytes.NewReader([]byte(prompt)), &stdout, &stderr)
				res = core.Result{
					Output:   stdout.String() + stderr.String(),
					ExitCode: exitCode,
				}
				if execErr != nil {
					res.Error = execErr.Error()
					if exitCode <= 0 {
						res.ExitCode = -1
					}
				}
				_ = mgr.Destroy(context.Background(), s.ID())
			}
			cancel()

			// Stop heartbeat
			stopHeartbeat()

			// Terminal transition
			status := "done"
			var lastError string
			if res.Error != "" {
				status = "failed"
				lastError = res.Error
			}
			summary := formatResult(backend.Name, res)

			if err := client.Ack(cfg.RuntimeToken, job.ID, status, summary, lastError); err != nil {
				log.Printf("error acking terminal status %s for job %s: %v", status, job.ID, err)
			} else {
				log.Printf("job %s completed (status=%s)", job.ID, status)
			}

			// Ack the SSE message (delivery acknowledgement).
			if err := client.AckMessage(cfg.RuntimeToken, event.ID); err != nil {
				log.Printf("error acking message %s: %v", event.ID, err)
			}

			lastEventID = event.ID
		}

		// Stream closed (disconnect or server shutdown). Reconnect with resume.
		log.Printf("SSE stream disconnected, reconnecting with last_event_id=%q", lastEventID)
		select {
		case <-ctx.Done():
			log.Println("daemon stopping")
			return nil
		case <-time.After(time.Second):
		}
	}
	}
}

func startHeartbeat(ctx context.Context, client *subscribe.Client, rtToken, jobID string, interval time.Duration) func() {
	hbCtx, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				if err := client.Heartbeat(rtToken, jobID); err != nil {
					log.Printf("Heartbeat failed for job %s: %v", jobID, err)
				}
			}
		}
	}()

	return cancel
}
