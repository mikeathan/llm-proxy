package tools

import (
	"context"
	"fmt"
	"strings"

	"llm-proxy/models"
)

// Connector defines the interface for sending messages to external platforms.
// Each platform (Telegram, Slack, Discord, etc.) implements this interface.
// The Name() return value is used in error attribution.
type Connector interface {
	Send(ctx context.Context, message string) error
	Name() string
}

// WebhookAware is optionally implemented by connectors that support inbound
// webhook registration.  Connectors satisfying this interface are
// automatically re-registered on startup when a WebhookURL is stored.
type WebhookAware interface {
	RegisterWebhook(ctx context.Context, webhookURL, webhookSecret string) error
}

// ConnectorFactory builds a connector instance for a config entry. It returns
// ok=false when the connector type is unregistered or its required credentials
// are missing, in which case the caller skips the connector.
type ConnectorFactory func(
	name string,
	cfg models.ConnectorConfig,
	secrets models.SecretsStore,
	network *NetworkTools,
) (Connector, bool)

// connectorFactories maps a connector type string (e.g. "telegram") to its
// factory. Connector packages register themselves via RegisterConnectorFactory,
// so adding a new platform requires no changes to the wiring layer.
var connectorFactories = map[string]ConnectorFactory{}

// RegisterConnectorFactory registers a connector implementation under its type
// string. Intended to be called from a connector package's init().
func RegisterConnectorFactory(connectorType string, factory ConnectorFactory) {
	connectorFactories[connectorType] = factory
}

// GetConnectorFactory returns the factory registered for a connector type.
func GetConnectorFactory(connectorType string) (ConnectorFactory, bool) {
	f, ok := connectorFactories[connectorType]
	return f, ok
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

// GetByName returns the connector registered under the given name.
func (c *CommunicationTools) GetByName(name string) (Connector, bool) {
	nc, ok := c.connectors[name]
	return nc.connector, ok
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
