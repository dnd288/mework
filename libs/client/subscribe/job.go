package subscribe

import (
	"fmt"
	"net/http"
)

type Job struct {
	ID                  string  `json:"id"`
	AccountID           string  `json:"account_id"`
	RuntimeID           string  `json:"runtime_id"`
	ExternalTaskID      string  `json:"external_task_id"`
	ExternalEventID     string  `json:"external_event_id"`
	ProviderCode        string  `json:"provider_code"`
	ExternalActorID     *string `json:"external_actor_id,omitempty"`
	TaskTitle           string  `json:"task_title"`
	TaskDescription     string  `json:"task_description"`
	ProfileBodySnapshot *string `json:"profile_body_snapshot,omitempty"`
	Workflow            string  `json:"workflow,omitempty"`
	Instructions        string  `json:"instructions"`
	Status              string  `json:"status"`
}

type AckRequest struct {
	Status        string  `json:"status"`
	ResultSummary *string `json:"result_summary,omitempty"`
	LastError     *string `json:"last_error,omitempty"`
}

// Claim calls POST /api/v1/jobs/claim to claim the oldest queued job for the runtime.
// It returns nil, nil if there is no job available.
func (c *Client) Claim(rtToken string) (*Job, error) {
	var job Job
	code, err := c.do("POST", "/api/v1/jobs/claim", rtToken, nil, &job)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNoContent {
		return nil, nil
	}
	return &job, nil
}

// Ack calls POST /api/v1/jobs/:id/ack to report status updates to the server.
func (c *Client) Ack(rtToken string, jobID string, status string, resultSummary string, lastError string) error {
	req := AckRequest{
		Status: status,
	}
	if resultSummary != "" {
		req.ResultSummary = &resultSummary
	}
	if lastError != "" {
		req.LastError = &lastError
	}

	_, err := c.do("POST", fmt.Sprintf("/api/v1/jobs/%s/ack", jobID), rtToken, req, nil)
	return err
}

// Heartbeat calls POST /api/v1/jobs/:id/heartbeat to extend the claim lease.
func (c *Client) Heartbeat(rtToken string, jobID string) error {
	_, err := c.do("POST", fmt.Sprintf("/api/v1/jobs/%s/heartbeat", jobID), rtToken, nil, nil)
	return err
}
