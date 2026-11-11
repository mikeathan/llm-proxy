package footprint

import (
	"context"
	"errors"
	"strings"
	"time"

	"llm-proxy/models"
)

// KeyStore keeps the breach service key. It is only ever saved, cleared or used for a look, never read back to the page.
type KeyStore interface {
	Saved() bool
	Set(key string) error
	Clear() error
}

// Actions of a settings command.
const (
	ActionAddDomain = "add_domain"
	ActionAddEmail  = "add_email"
	ActionRemove    = "remove"
	ActionFrequency = "frequency"
	ActionService   = "service"
)

// MaxKeyLen bounds a saved key.
const MaxKeyLen = 200

// Command is one change to the settings. Only the fields of its action are read.
type Command struct {
	Action    string `json:"action"`
	Name      string `json:"name,omitempty"`    // add_domain
	Address   string `json:"address,omitempty"` // add_email
	Label     string `json:"label,omitempty"`   // remove, frequency
	Frequency string `json:"frequency,omitempty"`
	Service   string `json:"service,omitempty"`
	On        bool   `json:"on,omitempty"`
}

// Manager is the footprint as the page and the audit use it: settings, looks and the key, in one place.
type Manager struct {
	Settings *Settings
	Service  *Service
	Key      KeyStore
	Now      func() time.Time
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// View is the page: settings, the latest results and whether a key is saved.
func (m *Manager) View() (View, error) {
	rs, err := m.Service.Results()
	if err != nil {
		return View{}, err
	}
	return ViewOf(m.Settings.Config(), rs, m.Key.Saved(), m.now()), nil
}

// Apply makes one settings change. A refusal is an error the operator can read as it is.
func (m *Manager) Apply(c Command) error {
	switch c.Action {
	case ActionAddDomain:
		_, err := m.Settings.AddDomain(c.Name, c.Frequency)
		return err
	case ActionAddEmail:
		_, err := m.Settings.AddEmail(c.Address, c.Frequency)
		return err
	case ActionRemove:
		return m.Settings.Remove(c.Label)
	case ActionFrequency:
		return m.Settings.SetFrequency(c.Label, c.Frequency)
	case ActionService:
		return m.Settings.SetService(c.Service, c.On)
	}
	return errors.New("unknown action")
}

// Check looks at one watch now (the operator's button).
func (m *Manager) Check(ctx context.Context, label string) error {
	_, err := m.Service.Check(ctx, label, true)
	return err
}

// SetKey saves the breach service key.
func (m *Manager) SetKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > MaxKeyLen || strings.ContainsAny(key, " \t\r\n") {
		return errors.New("enter the key exactly as the breach service gave it")
	}
	return m.Key.Set(key)
}

// ClearKey removes the saved key.
func (m *Manager) ClearKey() error { return m.Key.Clear() }

// Findings is what the audit reads: the findings of the stored results and which watches are fully and recently looked at. It
// asks no outside service.
func (m *Manager) Findings(now time.Time) ([]models.ScanFinding, map[string]bool) {
	rs, err := m.Service.Results()
	if err != nil {
		return nil, map[string]bool{} // unreadable results check nothing: every watch's earlier findings are carried
	}
	return Findings(m.Settings.Config(), rs, now)
}
