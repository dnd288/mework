package runner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"mework/libs/shared/policy"
	"mework/libs/sandbox"
)

// OfflineServer listens on a Unix socket, accepts JSON-RPC messages, and
// dispatches tasks to a workspace-bound session's sandbox over stdin (never
// argv), preserving the injection-safety invariant.
type OfflineServer struct {
	socketPath  string
	listener    net.Listener
	session     *Session
	done        chan struct{}
	mu          sync.Mutex
	closed      bool
	policy      *policy.Policy
	rateLimiter *policy.RateLimiter
}

// ---------------------------------------------------------------------------
// Socket path derivation
// ---------------------------------------------------------------------------

// SocketPath returns the deterministic Unix socket path for a workspace
// directory, derived from its SHA-256 hash.  Empty directories return an
// error.  Trailing slashes are normalised before hashing so that
// "/tmp/ws" and "/tmp/ws/" produce the same path.
func SocketPath(workspaceDir string) (string, error) {
	if workspaceDir == "" {
		return "", fmt.Errorf("workspace directory must not be empty")
	}
	normalised := strings.TrimRight(workspaceDir, "/")
	hash := sha256.Sum256([]byte(normalised))
	return fmt.Sprintf("/tmp/mework-offline-%x.sock", hash), nil
}

// ---------------------------------------------------------------------------
// Server lifecycle
// ---------------------------------------------------------------------------

// NewOfflineServer creates a new OfflineServer bound to the given workspace
// directory.  The session must already have been started (via
// StartWorkspaceSession or OpenSession).
// SetPolicy attaches a message policy to the server. When set, every
// incoming "run" request is checked against the policy before execution.
func (s *OfflineServer) SetPolicy(p *policy.Policy) {
	s.policy = p
	s.rateLimiter = policy.NewRateLimiter()
}

func NewOfflineServer(workspaceDir string, session *Session) (*OfflineServer, error) {
	sockPath, err := SocketPath(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("offline server: %w", err)
	}
	return &OfflineServer{
		socketPath: sockPath,
		session:    session,
		done:       make(chan struct{}),
	}, nil
}

// Start unlinks any stale socket at the path, begins listening, and accepts
// connections in a background goroutine.  It blocks until ctx is cancelled
// or a fatal error occurs.
func (s *OfflineServer) Start(ctx context.Context) error {
	// Unlink any stale socket from a previous run.
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket %s: %w", s.socketPath, err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socketPath, err)
	}
	// Restrict socket access to the owning user.
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("chmod %s: %w", s.socketPath, err)
	}
	s.listener = listener

	// Start the accept loop in the background.
	go s.acceptLoop(ctx)

	// Block until the context is cancelled (graceful shutdown).
	<-ctx.Done()
	_ = s.listener.Close()
	return ctx.Err()
}

// Close removes the socket file and marks the server as shut down.  It is
// safe to call multiple times.
func (s *OfflineServer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove socket file %s: %w", s.socketPath, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Connection handling — JSON-RPC over Unix socket
// ---------------------------------------------------------------------------

// acceptLoop accepts connections and dispatches each in its own goroutine.
func (s *OfflineServer) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Listener was closed (shutdown) or a transient error occurred.
			return
		}
		go s.handleConnection(ctx, conn)
	}
}

// jsonRPCRequest is a minimal JSON-RPC 2.0 request body.
type jsonRPCRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	ID     interface{}     `json:"id"`
}

// jsonRPCResponse is a minimal JSON-RPC 2.0 response body.
type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// runParams is the expected params for the "run" method.
type runParams struct {
	Instruction string `json:"instruction"`
	Sender      string `json:"sender,omitempty"`
}

// runResult is the result returned for a successful "run" invocation.
type runResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
}

// handleConnection reads one JSON-RPC request and dispatches it.  Only the
// "run" method is supported.
func (s *OfflineServer) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	var req jsonRPCRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	switch req.Method {
	case "run":
		var params runParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			sendJSONRPCError(conn, req.ID, -32602, "invalid params")
			return
		}

		// ---- POLICY ENFORCEMENT ----
		if s.policy != nil {
			sender := params.Sender
			if sender == "" {
				sender = "anonymous"
			}
			attrs := policy.Attributes{
				"sender":          sender,
				"authenticated":   "true",
				"content":         params.Instruction,
				"content_length":  fmt.Sprint(len(params.Instruction)),
				"time":            time.Now().UTC().Format(time.RFC3339),
				"channel":         "local",
			}
			result, err := s.policy.Enforce(attrs)
			if err != nil {
				sendJSONRPCError(conn, req.ID, -32603, "policy error: "+err.Error())
				return
			}
			if !result.Allowed {
				sendJSONRPCError(conn, req.ID, -32001, result.Reason)
				return
			}
			// Rate limit check
			if result.Reason != "" {
				if count, ok := policy.ParseLimit(result.Reason); ok {
					if !s.rateLimiter.Allow(sender, count) {
						sendJSONRPCError(conn, req.ID, -32002, "rate limit exceeded")
						return
					}
				}
			}
		}
		// ---- END POLICY ENFORCEMENT ----

		s.handleRun(ctx, conn, req.ID, params.Instruction)
	default:
		sendJSONRPCError(conn, req.ID, -32601, "method not found")
	}
}

// handleRun feeds the instruction to the session's sandbox over stdin and
// returns the output and exit code as a JSON-RPC response.
func (s *OfflineServer) handleRun(ctx context.Context, conn net.Conn, id interface{}, instruction string) {
	var out strings.Builder
	exitCode, execErr := s.session.sandbox.Exec(
		ctx,
		[]string{s.session.backend},
		strings.NewReader(instruction),
		&out, &out,
	)
	if execErr != nil {
		sendJSONRPCError(conn, id, -32000, execErr.Error())
		return
	}
	_ = json.NewEncoder(conn).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		Result:  runResult{Output: out.String(), ExitCode: exitCode},
		ID:      id,
	})
}

// sendJSONRPCError writes a JSON-RPC error response to conn.
func sendJSONRPCError(conn net.Conn, id interface{}, code int, message string) {
	_ = json.NewEncoder(conn).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		Error:   map[string]interface{}{"code": code, "message": message},
		ID:      id,
	})
}

// ---------------------------------------------------------------------------
// Engine validation
// ---------------------------------------------------------------------------

// ValidateOfflineEngine returns an error if the definition's engine is not
// "local".  Offline mode only supports the local engine; docker, cloudflare,
// and custom engines are rejected.
func ValidateOfflineEngine(def *sandbox.SandboxBundleMetadata) error {
	if def.Engine != "" && def.Engine != "local" {
		return fmt.Errorf("offline mode supports only 'local' engine, got %q", def.Engine)
	}
	return nil
}

// Compile-time interface check.
var _ io.Writer = (*strings.Builder)(nil)
