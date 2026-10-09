package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

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

// NamedConnector is one configured connector as a ConnectorSource builds it: the config map key, the config type and
// the instance.
type NamedConnector struct {
	Name string
	Type string // ConnectorConfig.Type, e.g. "telegram"
	Conn Connector
}

// ConnectorSource builds the current connector set from configuration. Connectors must be cheap to construct and own
// no goroutines or long-lived resources: Reload drops replaced instances without closing them.
type ConnectorSource func() []NamedConnector

// CommunicationTools manages a named map of connector instances, shared by the agent's notify_user tool, automation
// report delivery and the inbound webhook. The set is built from configuration at startup and rebuilt by Reload when
// the connector config or its secrets change, so Settings edits apply without a restart.
type CommunicationTools struct {
	mu         sync.RWMutex
	connectors map[string]namedConnector
	source     ConnectorSource
}

func NewCommunicationTools() *CommunicationTools {
	return &CommunicationTools{
		connectors: make(map[string]namedConnector),
	}
}

// SetSource sets where Reload reads the connector set from and loads it now.
func (c *CommunicationTools) SetSource(src ConnectorSource) {
	c.mu.Lock()
	c.source = src
	c.mu.Unlock()
	c.Reload()
}

// Reload rebuilds the connector set from the source (built outside the lock, then swapped in). A no-op without a
// source.
func (c *CommunicationTools) Reload() {
	c.mu.RLock()
	src := c.source
	c.mu.RUnlock()
	if src == nil {
		return
	}
	next := make(map[string]namedConnector)
	for _, nc := range src() {
		next[nc.Name] = namedConnector{connector: nc.Conn, connType: nc.Type}
	}
	c.mu.Lock()
	c.connectors = next
	c.mu.Unlock()
}

// AddConnector registers a connector under the given name.
// The name comes from the config map key (e.g. "my-telegram").
// connType is the connector type from ConnectorConfig.Type (e.g. "telegram").
func (c *CommunicationTools) AddConnector(name, connType string, conn Connector) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connectors[name] = namedConnector{connector: conn, connType: connType}
}

// GetByName returns the connector registered under the given name.
func (c *CommunicationTools) GetByName(name string) (Connector, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	nc, ok := c.connectors[name]
	return nc.connector, ok
}

// snapshot is the current set; sends run on it outside the lock so a slow connector never blocks a Reload.
func (c *CommunicationTools) snapshot() map[string]namedConnector {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connectors // replaced, never mutated in place, by Reload; AddConnector is startup/test only
}

// NotifyAll sends a message to registered connectors.
// If connectorType is non-empty, only connectors whose cfg.Type matches are
// called. If the filter matches no connectors, an error is returned so the
// agent knows the requested platform doesn't exist.
// Errors are collected and returned as a single combined error.
func (c *CommunicationTools) NotifyAll(ctx context.Context, message string, connectorType string) error {
	var errs []error
	var matched bool
	connectors := c.snapshot()
	for name, nc := range connectors {
		if connectorType != "" && !strings.EqualFold(nc.connType, connectorType) {
			continue
		}
		matched = true
		if err := nc.connector.Send(ctx, message); err != nil {
			errs = append(errs, fmt.Errorf("%s (%s): %w", name, nc.connector.Name(), err))
		}
	}
	if connectorType != "" && !matched {
		return fmt.Errorf("no connector found for type '%s' — available types: %s", connectorType, listTypes(connectors))
	}
	if len(errs) > 0 {
		// errors.Join preserves each connector's chain (%v flattened it), so a
		// typed terminal marker (models.ErrToolUnavailable) survives for the loop.
		return fmt.Errorf("some notifications failed: %w", errors.Join(errs...))
	}
	return nil
}

// listTypes returns a comma-separated list of unique connector types in the map.
func listTypes(connectors map[string]namedConnector) string {
	seen := make(map[string]bool)
	var types []string
	for _, nc := range connectors {
		if !seen[nc.connType] {
			seen[nc.connType] = true
			types = append(types, nc.connType)
		}
	}
	return strings.Join(types, ", ")
}
