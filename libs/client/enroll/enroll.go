package enroll

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"mework/libs/shared/config"
)

type RunnerIdentity struct {
	RunnerID string `json:"runner_id"`
	Secret   string `json:"secret"`
}

func Enroll(ctx context.Context, hubURL, regToken string) (*RunnerIdentity, error) {
	body := map[string]string{"token": regToken}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hubURL+"/api/v1/runners/enroll", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+regToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enroll request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enroll rejected: status %d", resp.StatusCode)
	}

	var result RunnerIdentity
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if err := config.SaveIdentity(result.RunnerID, result.Secret); err != nil {
		return nil, fmt.Errorf("save identity: %w", err)
	}

	return &result, nil
}
