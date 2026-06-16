package meworkclient

import (
	"fmt"
)

type ClientProfile struct {
	ID             string         `json:"id"`
	AccountID      string         `json:"account_id"`
	Name           string         `json:"name"`
	Body           string         `json:"body"`
	BackendHint    string         `json:"backend_hint,omitempty"`
	Harness        string         `json:"harness,omitempty"`
	WorkflowConfig map[string]any `json:"workflow_config"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type CreateProfileRequest struct {
	Name           string         `json:"name"`
	Body           string         `json:"body"`
	BackendHint    string         `json:"backend_hint"`
	Harness        string         `json:"harness"`
	WorkflowConfig map[string]any `json:"workflow_config"`
}

type UpdateProfileRequest struct {
	Body           string         `json:"body"`
	BackendHint    string         `json:"backend_hint"`
	Harness        string         `json:"harness"`
	WorkflowConfig map[string]any `json:"workflow_config"`
}

func (c *Client) CreateProfile(patToken string, req CreateProfileRequest) (*ClientProfile, error) {
	var res ClientProfile
	_, err := c.do("POST", "/api/v1/profiles", patToken, req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetProfile(patToken, name string) (*ClientProfile, error) {
	var res ClientProfile
	_, err := c.do("GET", fmt.Sprintf("/api/v1/profiles/%s", name), patToken, nil, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) ListProfiles(patToken string) ([]ClientProfile, error) {
	var res []ClientProfile
	_, err := c.do("GET", "/api/v1/profiles", patToken, nil, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) UpdateProfile(patToken, name string, req UpdateProfileRequest) (*ClientProfile, error) {
	var res ClientProfile
	_, err := c.do("PUT", fmt.Sprintf("/api/v1/profiles/%s", name), patToken, req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) DeleteProfile(patToken, name string) error {
	_, err := c.do("DELETE", fmt.Sprintf("/api/v1/profiles/%s", name), patToken, nil, nil)
	return err
}
