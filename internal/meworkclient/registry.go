package meworkclient

import (
	"fmt"
	"time"
)

type Runtime struct {
	ID         string     `json:"id"`
	AccountID  string     `json:"account_id"`
	Code       string     `json:"code"`
	Label      string     `json:"label"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
}

type CreateRuntimeRequest struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type CreateRuntimeResponse struct {
	Runtime
	Token string `json:"token"`
}

func (c *Client) CreateRuntime(patToken, code, label string) (*CreateRuntimeResponse, error) {
	req := CreateRuntimeRequest{Code: code, Label: label}
	var res CreateRuntimeResponse
	_, err := c.do("POST", "/api/v1/runtimes", patToken, req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) ListRuntimes(patToken string) ([]Runtime, error) {
	var runtimes []Runtime
	_, err := c.do("GET", "/api/v1/runtimes", patToken, nil, &runtimes)
	if err != nil {
		return nil, err
	}
	return runtimes, nil
}

func (c *Client) DeleteRuntime(patToken, id string) error {
	_, err := c.do("DELETE", fmt.Sprintf("/api/v1/runtimes/%s", id), patToken, nil, nil)
	return err
}
