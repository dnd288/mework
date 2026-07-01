package runner

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
)

// runRequest is the JSON-RPC 2.0 request body for the "run" method.
type runRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      interface{} `json:"id"`
}

// runResponse carries the JSON-RPC response fields of interest.
type runResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	ID interface{} `json:"id"`
}

// runResultFields is the decoded Result body of a successful "run" response.
type runResultFields struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
}

// SendInstruction connects to the offline agent's Unix socket, sends a
// JSON-RPC "run" request containing the instruction, and returns the exit
// code from the agent's response.  The instruction text is sent inside the
// JSON-RPC body — it is never placed on the command line (injection-safety
// invariant).  sender is an optional identity hint for policy enforcement.
func SendInstruction(socketPath, instruction, sender string) (int, error) {
	_, exitCode, err := SendInstructionResult(socketPath, instruction, sender)
	return exitCode, err
}

// SendInstructionResult connects to the offline agent's Unix socket, sends a
// JSON-RPC "run" request containing the instruction, and returns the output
// text, exit code, and any error from the agent.  The instruction text is sent
// inside the JSON-RPC body — it is never placed on the command line
// (injection-safety invariant).  sender is an optional identity hint for
// policy enforcement.
func SendInstructionResult(socketPath, instruction, sender string) (string, int, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return "", -1, fmt.Errorf("connect to offline agent at %s: %w", socketPath, err)
	}
	defer conn.Close()

	req := runRequest{
		JSONRPC: "2.0",
		Method:  "run",
		Params:  map[string]string{"instruction": instruction, "sender": sender},
		ID:      1,
	}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return "", -1, fmt.Errorf("send instruction: %w", err)
	}

	var resp runResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return "", -1, fmt.Errorf("read response: %w", err)
	}

	if resp.Error != nil {
		return "", -1, fmt.Errorf("agent error (%d): %s", resp.Error.Code, resp.Error.Message)
	}

	var result runResultFields
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", -1, fmt.Errorf("decode result: %w", err)
	}
	return result.Output, result.ExitCode, nil
}

// CheckAgentRunning returns true if a running offline agent is accepting
// connections at the given Unix socket path.  It checks whether the socket
// file exists (os.Stat) rather than dialing, so that the single-connection
// fake server used by CLI tests is not consumed by the check.
func CheckAgentRunning(socketPath string) bool {
	info, err := os.Stat(socketPath)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

