package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mework/libs/server/session"
	"mework/libs/shared/config"
)

// defaultAttachIdle is how long `session attach` waits with no events before it
// exits cleanly, so a dropped terminal frame never hangs the terminal forever.
const defaultAttachIdle = 30 * time.Second

// sessionEndpoint resolves the mework-server base URL and the PAT used to
// authenticate the human caller's session commands. It errors with guidance
// when no PAT is configured, mirroring the other management commands.
func sessionEndpoint() (baseURL, token string, err error) {
	cfg, err := config.LoadConfig(profile())
	if err != nil {
		return "", "", err
	}
	token = ResolveToken(cfg)
	if token == "" {
		return "", "", fmt.Errorf("not authenticated — run `mework login --token <mello_pat_...>` or set MELLO_API_KEY")
	}
	baseURL = cfg.ServerURL
	if baseURL == "" {
		baseURL = os.Getenv("MEWORK_SERVER_URL")
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return strings.TrimRight(baseURL, "/"), token, nil
}

// sessionDo performs a PAT-authed JSON request against the session API and
// decodes the response into out (when non-nil). It returns the status code.
func sessionDo(method, path, token string, body, out any) (int, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, path, reqBody)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, err
		}
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return resp.StatusCode, fmt.Errorf("decode response: %w", err)
			}
		}
	}
	return resp.StatusCode, nil
}

// sessionRow is the decode target for a session in the list/get responses.
// SessionInfo (libs/shared/core) is serialized with capitalized JSON keys.
type sessionRow struct {
	ID     string `json:"ID"`
	Runner string `json:"Runner"`
	Agent  struct {
		Name string `json:"Name"`
	} `json:"Agent"`
	Status string `json:"Status"`
}

// sessionCmd is the parent grouping command for session operations.
var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Inspect and drive interactive sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return sessionListCmd.RunE(cmd, args)
	},
}

// sessionListCmd lists the caller's sessions via GET /api/v1/sessions.
// It is intentionally NOT parented under sessionCmd so unit tests can call
// RunE directly.
var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the caller's sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		base, token, err := sessionEndpoint()
		if err != nil {
			return err
		}

		var rows []sessionRow
		if _, err := sessionDo(http.MethodGet, base+"/api/v1/sessions", token, nil, &rows); err != nil {
			return err
		}

		showJSON, _ := cmd.Flags().GetBool("json")
		if showJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(rows)
		}

		tw := newTableTo(out)
		row(tw, "SESSION ID", "RUNNER", "AGENT", "STATUS")
		for _, s := range rows {
			row(tw, s.ID, s.Runner, s.Agent.Name, s.Status)
		}
		return tw.Flush()
	},
}

// sessionCreateCmd creates a session via POST /api/v1/sessions.
var sessionCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a session for a named agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		agent, _ := cmd.Flags().GetString("agent")
		if agent == "" {
			return fmt.Errorf("required flag(s) \"agent\" not set")
		}
		runner, _ := cmd.Flags().GetString("runner")
		ver, _ := cmd.Flags().GetString("version")

		base, token, err := sessionEndpoint()
		if err != nil {
			return err
		}

		body := map[string]string{"agent_name": agent, "runner": runner}
		if ver != "" {
			body["version"] = ver
		}
		var created sessionRow
		if _, err := sessionDo(http.MethodPost, base+"/api/v1/sessions", token, body, &created); err != nil {
			return err
		}

		showJSON, _ := cmd.Flags().GetBool("json")
		if showJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(created)
		}
		fmt.Fprintln(out, created.ID)
		return nil
	},
}

// sessionSendCmd submits a chat turn via POST /api/v1/sessions/{id}/messages.
var sessionSendCmd = &cobra.Command{
	Use:   "send <id> <message>",
	Short: "Send a chat turn to a session",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, message := args[0], args[1]
		base, token, err := sessionEndpoint()
		if err != nil {
			return err
		}
		body := map[string]string{"role": "user", "content": message}
		status, err := sessionDo(http.MethodPost, base+"/api/v1/sessions/"+id+"/messages", token, body, nil)
		if err != nil {
			return err
		}
		if status != http.StatusAccepted && status != http.StatusOK && status != http.StatusCreated {
			return fmt.Errorf("send: unexpected status %d", status)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "accepted")
		return nil
	},
}

// sessionCloseCmd closes a session via DELETE /api/v1/sessions/{id}.
var sessionCloseCmd = &cobra.Command{
	Use:   "close <id>",
	Short: "Close a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		base, token, err := sessionEndpoint()
		if err != nil {
			return err
		}
		if _, err := sessionDo(http.MethodDelete, base+"/api/v1/sessions/"+id, token, nil, nil); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "closed "+id)
		return nil
	},
}

// sessionAttachCmd streams a session's events via GET /api/v1/sessions/{id}/stream.
// It prints token/message content and exits on a terminal done/error event or
// after an idle interval with no events.
var sessionAttachCmd = &cobra.Command{
	Use:   "attach <id>",
	Short: "Stream a session's events until done or idle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		out := cmd.OutOrStdout()

		idle := defaultAttachIdle
		if v, _ := cmd.Flags().GetString("idle"); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				idle = d
			}
		}

		base, token, err := sessionEndpoint()
		if err != nil {
			return err
		}

		req, err := http.NewRequest(http.MethodGet, base+"/api/v1/sessions/"+id+"/stream", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := (&http.Client{}).Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("attach: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
		}

		// Decode SSE frames on a goroutine so the idle timer can preempt a
		// silent stream without blocking on the scanner.
		frames := make(chan session.ChatEvent, 16)
		go func() {
			defer close(frames)
			scanner := bufio.NewScanner(resp.Body)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			var data string
			for scanner.Scan() {
				line := scanner.Text()
				switch {
				case strings.HasPrefix(line, "data: "):
					data = strings.TrimPrefix(line, "data: ")
				case strings.HasPrefix(line, "data:"):
					data = strings.TrimPrefix(line, "data:")
				case line == "":
					if data != "" {
						var ev session.ChatEvent
						if json.Unmarshal([]byte(data), &ev) == nil {
							frames <- ev
						}
						data = ""
					}
				}
			}
		}()

		timer := time.NewTimer(idle)
		defer timer.Stop()
		for {
			select {
			case ev, ok := <-frames:
				if !ok {
					// Stream closed by the server without a terminal event.
					return nil
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(idle)
				switch ev.Kind {
				case session.EventToken, session.EventMessage:
					if ev.Content != "" {
						fmt.Fprintln(out, ev.Content)
					}
				case session.EventDone:
					return nil
				case session.EventError:
					if ev.Content != "" {
						fmt.Fprintln(out, ev.Content)
					}
					return fmt.Errorf("session error")
				}
			case <-timer.C:
				// Idle timeout: exit cleanly rather than blocking forever.
				return nil
			}
		}
	},
}

func init() {
	sessionListCmd.Flags().Bool("json", false, "Output as JSON")
	sessionCreateCmd.Flags().String("agent", "", "Agent name (required)")
	sessionCreateCmd.Flags().String("runner", "", "Target runner id")
	sessionCreateCmd.Flags().String("version", "", "Agent version (default latest)")
	sessionCreateCmd.Flags().Bool("json", false, "Output as JSON")
	sessionAttachCmd.Flags().String("idle", "", "Idle timeout before exiting (e.g. 30s)")

	sessionCmd.AddCommand(sessionListCmd, sessionCreateCmd, sessionSendCmd, sessionCloseCmd, sessionAttachCmd)
}
