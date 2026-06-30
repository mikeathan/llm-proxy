package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Connector defines the interface for sending messages to external platforms.
// Each platform (Telegram, Slack, Discord, etc.) implements this interface.
// The Name() return value is used in error attribution.
type Connector interface {
	Send(ctx context.Context, message string) error
	Name() string
}

// namedConnector pairs a Connector with its config type string so NotifyAll
// can filter by type without requiring Connector.Name() to match cfg.Type.
type namedConnector struct {
	connector Connector
	connType  string // from ConnectorConfig.Type, e.g. "telegram"
}

// CommunicationTools manages a named map of connector instances.
// Connectors are registered by name at startup from the config map and
// dispatched via NotifyAll when the agent calls the notify_user tool.
type CommunicationTools struct {
	connectors map[string]namedConnector
}

func NewCommunicationTools() *CommunicationTools {
	return &CommunicationTools{
		connectors: make(map[string]namedConnector),
	}
}

// AddConnector registers a connector under the given name.
// The name comes from the config map key (e.g. "my-telegram").
// connType is the connector type from ConnectorConfig.Type (e.g. "telegram").
func (c *CommunicationTools) AddConnector(name, connType string, conn Connector) {
	c.connectors[name] = namedConnector{connector: conn, connType: connType}
}

// NotifyAll sends a message to registered connectors.
// If connectorType is non-empty, only connectors whose cfg.Type matches are
// called. If the filter matches no connectors, an error is returned so the
// agent knows the requested platform doesn't exist.
// Errors are collected and returned as a single combined error.
func (c *CommunicationTools) NotifyAll(ctx context.Context, message string, connectorType string) error {
	var errs []error
	var matched bool
	for name, nc := range c.connectors {
		if connectorType != "" && !strings.EqualFold(nc.connType, connectorType) {
			continue
		}
		matched = true
		if err := nc.connector.Send(ctx, message); err != nil {
			errs = append(errs, fmt.Errorf("%s (%s): %w", name, nc.connector.Name(), err))
		}
	}
	if connectorType != "" && !matched {
		return fmt.Errorf("no connector found for type '%s' — available types: %s", connectorType, c.listTypes())
	}
	if len(errs) > 0 {
		return fmt.Errorf("some notifications failed: %v", errs)
	}
	return nil
}

// listTypes returns a comma-separated list of unique connector types in the map.
func (c *CommunicationTools) listTypes() string {
	seen := make(map[string]bool)
	var types []string
	for _, nc := range c.connectors {
		if !seen[nc.connType] {
			seen[nc.connType] = true
			types = append(types, nc.connType)
		}
	}
	return strings.Join(types, ", ")
}

// TelegramNotifier implements the Connector interface for Telegram.
// Uses an injected *http.Client so that all network I/O routes through NetworkTools.
type TelegramNotifier struct {
	Token  string
	ChatID string
	client *http.Client
}

func NewTelegramNotifier(token, chatID string, client *http.Client) *TelegramNotifier {
	return &TelegramNotifier{Token: token, ChatID: chatID, client: client}
}

func (t *TelegramNotifier) Name() string { return "Telegram" }

func (t *TelegramNotifier) Send(ctx context.Context, message string) error {
	if t.Token == "" || t.ChatID == "" || t.client == nil {
		return fmt.Errorf("telegram connector not fully configured")
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)
	formData := url.Values{}
	formData.Set("chat_id", t.ChatID)
	formData.Set("text", message)
	formData.Set("parse_mode", "Markdown")

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telegram API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
