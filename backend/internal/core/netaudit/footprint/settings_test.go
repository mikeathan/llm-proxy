package footprint

import (
	"strings"
	"testing"

	"llm-proxy/models"
)

type memConfig struct{ c models.NetauditConfig }

func (m *memConfig) Get() models.NetauditConfig { return m.c }
func (m *memConfig) Update(fn func(*models.NetauditConfig) error) error {
	next := m.c
	next.Footprint.Domains = append([]models.FootprintDomain(nil), m.c.Footprint.Domains...)
	next.Footprint.Emails = append([]models.FootprintEmail(nil), m.c.Footprint.Emails...)
	if err := fn(&next); err != nil {
		return err
	}
	m.c = next
	return nil
}

func TestSettings_AddsValidatedDomainsAndGivesAddressesAnOpaqueId(t *testing.T) {
	s := NewSettings(&memConfig{})
	label, err := s.AddDomain(" Example.COM ", models.FootprintDaily)
	if err != nil || label != "domain:example.com" {
		t.Fatalf("%q %v", label, err)
	}
	elabel, err := s.AddEmail("Jo@Example.com", "")
	if err != nil || !strings.HasPrefix(elabel, "email:") || strings.Contains(elabel, "jo") || strings.Contains(elabel, "example") {
		t.Fatalf("the label must be opaque: %q %v", elabel, err)
	}
	c := s.Config()
	if c.Emails[0].Address != "jo@example.com" || c.Emails[0].Frequency != models.FootprintManual {
		t.Errorf("%+v", c.Emails[0])
	}
}

func TestSettings_RefusesBadInputDuplicatesAndTooMany(t *testing.T) {
	s := NewSettings(&memConfig{})
	if _, err := s.AddDomain("10.0.0.1", ""); err == nil {
		t.Error("an address is not a domain")
	}
	if _, err := s.AddDomain("example.com", "hourly"); err == nil {
		t.Error("hourly is not a frequency")
	}
	_, _ = s.AddDomain("example.com", "")
	if _, err := s.AddDomain("EXAMPLE.com", ""); err == nil {
		t.Error("a duplicate")
	}
	for i := 0; i < MaxDomains; i++ {
		_, _ = s.AddDomain(strings.Repeat("a", i+1)+".com", "")
	}
	if len(s.Config().Domains) != MaxDomains {
		t.Errorf("cap: %d", len(s.Config().Domains))
	}
}

func TestSettings_RemoveFrequencyAndServices(t *testing.T) {
	s := NewSettings(&memConfig{})
	label, _ := s.AddDomain("example.com", "")
	if err := s.SetFrequency(label, models.FootprintWeekly); err != nil || s.Config().Domains[0].Frequency != models.FootprintWeekly {
		t.Errorf("frequency: %v", err)
	}
	if err := s.SetService(ServiceBreaches, true); err != nil || !s.Config().BreachesOn {
		t.Errorf("service: %v", err)
	}
	if err := s.SetService("nope", true); err == nil {
		t.Error("unknown service")
	}
	if err := s.Remove(label); err != nil || len(s.Config().Domains) != 0 {
		t.Errorf("remove: %v", err)
	}
	if err := s.Remove(label); err != ErrNoSuchWatch {
		t.Errorf("removing twice: %v", err)
	}
}
