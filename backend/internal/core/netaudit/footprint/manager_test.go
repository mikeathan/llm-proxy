package footprint

import (
	"context"
	"testing"
	"time"

	"llm-proxy/models"
)

type memKey struct{ key string }

func (k *memKey) Saved() bool        { return k.key != "" }
func (k *memKey) Set(v string) error { k.key = v; return nil }
func (k *memKey) Clear() error       { k.key = ""; return nil }

func newManager() (*Manager, *stubs, *memKey) {
	cc := &memConfig{}
	st := &stubs{certs: []int64{1}, expiry: now.Add(100 * day)}
	at := now
	store := &memStore{}
	settings := NewSettings(cc)
	svc := NewService(Deps{Config: settings.Config, Store: store, Certs: st, Registry: st, Mail: st, Breaches: breachStub{st}, Now: func() time.Time { return at }})
	key := &memKey{}
	return &Manager{Settings: settings, Service: svc, Key: key, Now: func() time.Time { return at }}, st, key
}

func TestManager_ApplyCheckViewAndFindings(t *testing.T) {
	m, st, _ := newManager()
	for _, c := range []Command{
		{Action: ActionAddDomain, Name: "example.com", Frequency: models.FootprintDaily},
		{Action: ActionService, Service: ServiceExpiry, On: true},
	} {
		if err := m.Apply(c); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Check(context.Background(), "domain:example.com"); err != nil {
		t.Fatal(err)
	}
	v, err := m.View()
	if err != nil || len(v.Watches) != 1 || len(v.Watches[0].Results) != 1 || len(st.asked) != 1 {
		t.Fatalf("%+v %v %v", v, err, st.asked)
	}
	fs, checked := m.Findings(now)
	if len(fs) != 0 || !checked["domain:example.com"] {
		t.Errorf("%v %v", fs, checked)
	}
	if err := m.Apply(Command{Action: "nope"}); err == nil {
		t.Error("an unknown action must be refused")
	}
}

func TestManager_KeyIsValidatedAndNeverInTheView(t *testing.T) {
	m, _, key := newManager()
	for _, bad := range []string{"", "has space", string(make([]byte, MaxKeyLen+1))} {
		if err := m.SetKey(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if err := m.SetKey(" abc123 "); err != nil || key.key != "abc123" {
		t.Fatal("key not saved")
	}
	v, _ := m.View()
	if !v.KeySaved {
		t.Error("the page must say a key is saved")
	}
	if err := m.ClearKey(); err != nil || key.Saved() {
		t.Error("key not cleared")
	}
}
