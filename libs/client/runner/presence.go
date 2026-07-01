package runner

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// presencePOST sends an authenticated POST request to the given hub path.
func presencePOST(ctx context.Context, hubURL, runnerID, secret, path string) error {
	url := fmt.Sprintf("%s%s", strings.TrimRight(hubURL, "/"), path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("presence POST %s: status %d", path, resp.StatusCode)
	}
	return nil
}

// Heartbeat posts a heartbeat to the hub to extend the runner's presence TTL.
func Heartbeat(ctx context.Context, hubURL, runnerID, secret string) error {
	return presencePOST(ctx, hubURL, runnerID, secret, fmt.Sprintf("/api/v1/runners/presence/%s/heartbeat", runnerID))
}

// SetOnline marks the runner as online on the hub after a successful SSE connect.
func SetOnline(ctx context.Context, hubURL, runnerID, secret string) error {
	return presencePOST(ctx, hubURL, runnerID, secret, fmt.Sprintf("/api/v1/runners/presence/%s/online", runnerID))
}

// SetOffline marks the runner as offline on the hub on disconnect or graceful stop.
func SetOffline(ctx context.Context, hubURL, runnerID, secret string) error {
	return presencePOST(ctx, hubURL, runnerID, secret, fmt.Sprintf("/api/v1/runners/presence/%s/offline", runnerID))
}
