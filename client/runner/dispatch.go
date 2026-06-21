package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"mework/client/subscribe"
	"mework/sandbox/agent"
	"mework/sandbox/engine/local"
	"mework/sandbox/runtime"
	"mework/shared/core"
	"mework/shared/grant"
	"mework/shared/transport"
)

// processOpts holds external dependencies for the dispatch lifecycle.
type processOpts struct {
	hubURL     string
	catalogURL string
	secret     string
	client     *subscribe.Client
}

// processDispatch handles the full lifecycle of a single dispatch:
//  1. Parse and verify the permission grant carried by the dispatch.
//  2. Enforce required operations (pull, spawn) against the grant.
//  3. Pull the referenced agent version from the catalog.
//  4. Run the agent via the sandbox runtime.
//  5. Report the terminal result (done / failed / refused) to the hub.
//  6. Acknowledge the SSE message so it is not redelivered.
func processDispatch(ctx context.Context, d transport.Dispatch, eventID string, opts processOpts) error {
	// 1. Parse and verify grant integrity.
	g, err := parseAndVerifyGrant(d.Grant, []byte(opts.secret))
	if err != nil {
		return ackAndReturn(ctx, opts, eventID, fmt.Errorf("grant verification failed: %w", err))
	}

	// 2. Enforce required operations.
	if err := enforceGrant(g, grant.OpPullAgent); err != nil {
		reportRefused(ctx, opts, d.Session, fmt.Sprintf("pull not granted: %v", err))
		return ackAndReturn(ctx, opts, eventID, err)
	}
	if err := enforceGrant(g, grant.OpSpawn); err != nil {
		reportRefused(ctx, opts, d.Session, fmt.Sprintf("spawn not granted: %v", err))
		return ackAndReturn(ctx, opts, eventID, err)
	}

	// 3. Pull agent from catalog.
	artifact, err := pullAgent(ctx, opts.catalogURL, d.Agent, d.Grant)
	if err != nil {
		return ackAndReturn(ctx, opts, eventID, fmt.Errorf("pull agent: %w", err))
	}

	// 4. Run via sandbox.
	result := runAgent(ctx, artifact)

	// 5. Report terminal result.
	status := "done"
	var lastError string
	if result.Error != "" {
		status = "failed"
		lastError = result.Error
	}
	if err := reportResult(ctx, opts.hubURL, opts.secret, d.Session, status, result.Output, lastError); err != nil {
		log.Printf("report result failed: %v (dispatch session=%s)", err, d.Session)
		// Continue to ack even if reporting fails — the result is best-effort.
	}

	// 6. Ack the SSE message.
	return opts.client.AckMessage(opts.secret, eventID)
}

// pullAgent fetches an artifact for the given agent reference from the catalog.
func pullAgent(ctx context.Context, catalogURL string, ref transport.AgentRef, grantJSON json.RawMessage) (*transport.Artifact, error) {
	url := fmt.Sprintf("%s/api/v1/agents/%s/versions/%s/pull",
		strings.TrimRight(catalogURL, "/"), ref.Name, ref.Version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if len(grantJSON) > 0 {
		req.Header.Set("X-Dispatch-Grant", string(grantJSON))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned status %d", resp.StatusCode)
	}

	var artifact transport.Artifact
	if err := json.NewDecoder(resp.Body).Decode(&artifact); err != nil {
		return nil, fmt.Errorf("decode artifact: %w", err)
	}
	return &artifact, nil
}

// runAgent runs the agent artifact through the sandbox runtime. It is a
// package-level variable so tests can replace it with a mock. Production code
// uses defaultRunAgent — the prompt is fed to the AI CLI over stdin (never
// argv), matching the injection-safety invariant.
var runAgent = defaultRunAgent

// defaultRunAgent runs the agent artifact through the local sandbox driver. It
// detects the first available AI CLI backend and feeds the artifact content
// as the prompt over stdin (never argv).
func defaultRunAgent(ctx context.Context, artifact *transport.Artifact) core.Result {
	backend, ok := agent.Detect(nil)
	if !ok {
		return core.Result{Error: fmt.Sprintf("no AI backend detected; install one of %v", agent.DefaultBackends)}
	}

	// Use the local sandbox driver for the default path.
	drv := local.New()
	mgr := runtime.NewManager(drv)

	spec := core.RunSpec{
		AgentID:     artifact.Ref.Name,
		BackendPath: backend.Path,
		BackendName: backend.Name,
		Task:        string(artifact.Content),
		Timeout:     30 * time.Minute,
	}

	runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	s, err := mgr.Start(runCtx, spec)
	if err != nil {
		return core.Result{Error: fmt.Sprintf("start sandbox: %v", err)}
	}
	defer func() { _ = mgr.Destroy(context.Background(), s.ID()) }()

	var stdout, stderr bytes.Buffer
	exitCode, execErr := s.Exec(runCtx, []string{backend.Path}, bytes.NewReader(artifact.Content), &stdout, &stderr)

	res := core.Result{
		Output:   stdout.String() + stderr.String(),
		ExitCode: exitCode,
	}
	if execErr != nil {
		if exitCode <= 0 {
			res.ExitCode = -1
		}
		res.Error = execErr.Error()
	}
	return res
}

// reportResult posts a terminal result for a session to the hub.
func reportResult(ctx context.Context, hubURL, secret, session, status, summary, lastError string) error {
	body := map[string]string{
		"status":  status,
		"summary": summary,
	}
	if lastError != "" {
		body["error"] = lastError
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/runners/sessions/%s/result",
		strings.TrimRight(hubURL, "/"), session)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("hub returned status %d", resp.StatusCode)
	}
	return nil
}

// reportRefused posts a "refused" result to the hub when the grant check fails.
func reportRefused(ctx context.Context, opts processOpts, session, reason string) {
	_ = reportResult(ctx, opts.hubURL, opts.secret, session, "refused", reason, "")
}

// ackAndReturn acks the SSE message and returns the wrapped error.
func ackAndReturn(ctx context.Context, opts processOpts, eventID string, cause error) error {
	if ackErr := opts.client.AckMessage(opts.secret, eventID); ackErr != nil {
		log.Printf("ack failed for event %s: %v (original error: %v)", eventID, ackErr, cause)
	}
	return cause
}
