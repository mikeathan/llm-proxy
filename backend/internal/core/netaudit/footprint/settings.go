package footprint

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"llm-proxy/models"
)

// ConfigStore is the network-audit policy store: *storage's policy store satisfies it, the same one the other module switches use.
type ConfigStore interface {
	Get() models.NetauditConfig
	Update(func(*models.NetauditConfig) error) error
}

// Services name the four outside services, as the screen and the switch endpoint do.
const (
	ServiceCerts    = "certificates"
	ServiceExpiry   = "expiry"
	ServiceMail     = "mail_records"
	ServiceBreaches = "breaches"
)

// Settings validates and keeps what is watched and which services may be asked.
type Settings struct{ store ConfigStore }

func NewSettings(store ConfigStore) *Settings { return &Settings{store: store} }

// Config is a copy of the current settings.
func (s *Settings) Config() models.FootprintConfig {
	c := s.store.Get().Footprint
	c.Domains = slices.Clone(c.Domains)
	c.Emails = slices.Clone(c.Emails)
	return c
}

func validFrequency(f string) (string, error) {
	switch f {
	case "":
		return models.FootprintManual, nil
	case models.FootprintManual, models.FootprintDaily, models.FootprintWeekly:
		return f, nil
	}
	return "", errors.New("the frequency must be manual, daily or weekly")
}

func (s *Settings) update(fn func(*models.FootprintConfig) error) error {
	// A refusal from fn (a bad value, a cap) is the operator's to read as it is; only a failure to save is wrapped.
	var refused error
	err := s.store.Update(func(c *models.NetauditConfig) error {
		if refused = fn(&c.Footprint); refused != nil {
			return refused
		}
		return nil
	})
	if refused != nil {
		return refused
	}
	if err != nil {
		return fmt.Errorf("failed to save the footprint settings: %w", err)
	}
	return nil
}

// AddDomain starts watching a domain and returns its label. A domain already watched is an error, not a second watch.
func (s *Settings) AddDomain(raw, frequency string) (string, error) {
	name, err := ValidateDomain(raw)
	if err != nil {
		return "", err
	}
	freq, err := validFrequency(frequency)
	if err != nil {
		return "", err
	}
	return models.DomainFindingPrefix + name, s.update(func(c *models.FootprintConfig) error {
		if len(c.Domains) >= MaxDomains {
			return fmt.Errorf("at most %d domains can be watched", MaxDomains)
		}
		if slices.ContainsFunc(c.Domains, func(d models.FootprintDomain) bool { return d.Name == name }) {
			return errors.New("that domain is already watched")
		}
		c.Domains = append(c.Domains, models.FootprintDomain{Name: name, Frequency: freq})
		return nil
	})
}

// AddEmail starts watching an address and returns its label. The address gets an opaque id now; findings are keyed on the id, never on
// the address or anything derived from it.
func (s *Settings) AddEmail(raw, frequency string) (string, error) {
	addr, err := ValidateEmail(raw)
	if err != nil {
		return "", err
	}
	freq, err := validFrequency(frequency)
	if err != nil {
		return "", err
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	return models.EmailFindingPrefix + id, s.update(func(c *models.FootprintConfig) error {
		if len(c.Emails) >= MaxEmails {
			return fmt.Errorf("at most %d addresses can be watched", MaxEmails)
		}
		if slices.ContainsFunc(c.Emails, func(e models.FootprintEmail) bool { return e.Address == addr }) {
			return errors.New("that address is already watched")
		}
		c.Emails = append(c.Emails, models.FootprintEmail{ID: id, Address: addr, Frequency: freq})
		return nil
	})
}

func newID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to make an id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Remove stops watching the domain or address with this label.
func (s *Settings) Remove(label string) error {
	return s.update(func(c *models.FootprintConfig) error {
		before := len(c.Domains) + len(c.Emails)
		c.Domains = slices.DeleteFunc(c.Domains, func(d models.FootprintDomain) bool { return models.DomainFindingPrefix+d.Name == label })
		c.Emails = slices.DeleteFunc(c.Emails, func(e models.FootprintEmail) bool { return models.EmailFindingPrefix+e.ID == label })
		if len(c.Domains)+len(c.Emails) == before {
			return ErrNoSuchWatch
		}
		return nil
	})
}

// SetFrequency changes how often one watch is looked at on its own.
func (s *Settings) SetFrequency(label, frequency string) error {
	freq, err := validFrequency(frequency)
	if err != nil {
		return err
	}
	return s.update(func(c *models.FootprintConfig) error {
		for i := range c.Domains {
			if models.DomainFindingPrefix+c.Domains[i].Name == label {
				c.Domains[i].Frequency = freq
				return nil
			}
		}
		for i := range c.Emails {
			if models.EmailFindingPrefix+c.Emails[i].ID == label {
				c.Emails[i].Frequency = freq
				return nil
			}
		}
		return ErrNoSuchWatch
	})
}

// SetService switches one outside service on or off.
func (s *Settings) SetService(service string, on bool) error {
	return s.update(func(c *models.FootprintConfig) error {
		switch service {
		case ServiceCerts:
			c.CertificatesOn = on
		case ServiceExpiry:
			c.ExpiryOn = on
		case ServiceMail:
			c.MailRecordsOn = on
		case ServiceBreaches:
			c.BreachesOn = on
		default:
			return errors.New("unknown service")
		}
		return nil
	})
}
