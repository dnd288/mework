package subscribe

import (
	"fmt"
)

type Connection struct {
	ID            string         `json:"id"`
	AccountID     string         `json:"account_id"`
	ProviderCode  string         `json:"provider_code"`
	WebhookSecret string         `json:"webhook_secret,omitempty"`
	Config        map[string]any `json:"config"`
	CreatedAt     string         `json:"created_at"`
}

type CreateConnectionRequest struct {
	ProviderCode  string         `json:"provider_code"`
	ProviderToken string         `json:"provider_token"`
	WebhookSecret string         `json:"webhook_secret"`
	Config        map[string]any `json:"config"`
}

func (c *Client) CreateConnection(patToken, providerCode, providerToken, webhookSecret string, config map[string]any) (*Connection, error) {
	req := CreateConnectionRequest{
		ProviderCode:  providerCode,
		ProviderToken: providerToken,
		WebhookSecret: webhookSecret,
		Config:        config,
	}
	var res Connection
	_, err := c.do("POST", "/api/v1/connections", patToken, req, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) GetConnection(patToken, providerCode string) (*Connection, error) {
	var res Connection
	_, err := c.do("GET", fmt.Sprintf("/api/v1/connections/%s", providerCode), patToken, nil, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *Client) ListConnections(patToken string) ([]Connection, error) {
	var res []Connection
	_, err := c.do("GET", "/api/v1/connections", patToken, nil, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) DeleteConnection(patToken, providerCode string) error {
	_, err := c.do("DELETE", fmt.Sprintf("/api/v1/connections/%s", providerCode), patToken, nil, nil)
	return err
}
