package routerconn

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"llm-proxy/internal/platform/routerssh"
	"llm-proxy/models"
)

type memSettings struct{ addr, user, host string }

func (m *memSettings) Get() (string, string, string) { return m.addr, m.user, m.host }
func (m *memSettings) Set(a, u, h string) error      { m.addr, m.user, m.host = a, u, h; return nil }

type memSecret struct{ v string }

func (m *memSecret) Value() string      { return m.v }
func (m *memSecret) Set(s string) error { m.v = s; return nil }
func (m *memSecret) Clear() error       { m.v = ""; return nil }

func privateKeyPEM(t *testing.T) string {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

const table = `ipv4 2 tcp 6 1 ESTABLISHED src=192.168.50.220 dst=52.48.210.116 sport=1 dport=8883 src=52.48.210.116 dst=192.168.0.137 sport=8883 dport=1
ipv4 2 tcp 6 1 ESTABLISHED src=192.168.50.220 dst=52.48.210.116 sport=2 dport=8883 src=52.48.210.116 dst=192.168.0.137 sport=8883 dport=2
ipv4 2 udp 17 1 src=192.168.50.31 dst=8.8.8.8 sport=1 dport=53 src=8.8.8.8 dst=192.168.0.137 sport=53 dport=1
`

type fixture struct {
	m      *Manager
	set    *memSettings
	secret *memSecret
	target routerssh.Target
	now    time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{set: &memSettings{addr: "192.168.50.1", user: "admin", host: "ssh-ed25519 AAAA"}, secret: &memSecret{v: "KEY"}, now: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
	f.m = New(Deps{
		Settings: f.set, Secret: f.secret,
		Read: func(_ context.Context, tg routerssh.Target) ([]byte, error) { f.target = tg; return []byte(table), nil },
		Probe: func(context.Context, string) (string, string, error) {
			return "ssh-ed25519 AAAAhost", "SHA256:abc", nil
		},
		Owners: func() Owners {
			return func(a netip.Addr) (models.TrafficOwner, bool) {
				if a.String() == "52.48.210.116" {
					return models.TrafficOwner{ASN: 16509, Name: "AMAZON-02", City: "Dublin"}, true
				}
				return models.TrafficOwner{}, false
			}
		},
		Now: func() time.Time { return f.now },
	})
	return f
}

func TestAReadGroupsEachDevicesConnectionsByPlaceAndNamesThem(t *testing.T) {
	f := newFixture(t)
	got, err := f.m.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Lines != 3 || got.Kept != 3 || len(got.Devices) != 2 || !got.At.Equal(f.now) {
		t.Fatalf("%+v", got)
	}
	d := got.Devices[0]
	if d.Address != "192.168.50.220" || d.Total != 2 || len(d.Places) != 1 || d.Places[0].Count != 2 || d.Places[0].Owner == nil || d.Places[0].Owner.City != "Dublin" {
		t.Errorf("device: %+v", d)
	}
	if f.target.User != "admin" || string(f.target.Key) != "KEY" || f.target.HostKey != "ssh-ed25519 AAAA" || f.m.View().Last != got {
		t.Errorf("target %+v", f.target)
	}
}

func TestNothingIsReadWithoutAddressUserAndKeyAndOnlyOneReadRunsAtATime(t *testing.T) {
	f := newFixture(t)
	f.secret.v = ""
	if _, err := f.m.Read(context.Background()); !errors.Is(err, ErrNotSet) {
		t.Errorf("no key: %v", err)
	}
	f.secret.v = "KEY"
	if _, err := f.m.Read(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Read(context.Background()); !errors.Is(err, ErrBusy) {
		t.Errorf("second: %v", err)
	}
	f.now = f.now.Add(gap + time.Second)
	if _, err := f.m.Read(context.Background()); err != nil {
		t.Errorf("after the gap: %v", err)
	}
}

func TestChangingTheAddressDropsThePinnedKeyAndTheLastRead(t *testing.T) {
	f := newFixture(t)
	_, _ = f.m.Read(context.Background())
	if err := f.m.SetTarget("192.168.50.1", "admin"); err != nil || f.set.host == "" || f.m.View().Last == nil {
		t.Fatalf("same address kept nothing: %v %+v", err, f.set)
	}
	if err := f.m.SetTarget("192.168.50.2", "admin"); err != nil || f.set.host != "" || f.m.View().Last != nil {
		t.Errorf("new address: %v %+v", err, f.set)
	}
	for _, c := range []struct {
		addr, user string
		want       error
	}{{"8.8.8.8", "admin", routerssh.ErrNotPrivate}, {"192.168.1.1", "a b; rm", ErrBadUser}, {"192.168.1.1", "", ErrBadUser}} {
		if err := f.m.SetTarget(c.addr, c.user); !errors.Is(err, c.want) {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

func TestOnlyAUsablePrivateKeyIsSavedAndIsNeverInTheView(t *testing.T) {
	f := newFixture(t)
	if err := f.m.SetKey("not a key"); !errors.Is(err, ErrBadKey) {
		t.Errorf("bad key: %v", err)
	}
	key := privateKeyPEM(t)
	if err := f.m.SetKey(key); err != nil || f.secret.v != key {
		t.Fatalf("%v", err)
	}
	v := f.m.View()
	if !v.KeySaved || strings.Contains(v.Address+v.User+v.Fingerprint, "PRIVATE") {
		t.Errorf("%+v", v)
	}
	_ = f.m.ClearKey()
	if f.m.View().KeySaved {
		t.Error("key still saved")
	}
}

func TestThePinNeedsTheFingerprintTheOperatorSawAndSavesTheKeyOnlyThen(t *testing.T) {
	f := newFixture(t)
	f.set.host = ""
	fp, err := f.m.Probe(context.Background())
	if err != nil || fp != "SHA256:abc" || f.set.host != "" {
		t.Fatalf("probe: %q %v host=%q", fp, err, f.set.host)
	}
	for _, bad := range []string{"", "SHA256:other"} {
		if err := f.m.Pin(context.Background(), bad); !errors.Is(err, ErrNotConfirmed) || f.set.host != "" {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if err := f.m.Pin(context.Background(), "SHA256:abc"); err != nil || f.set.host != "ssh-ed25519 AAAAhost" {
		t.Errorf("pin: %v host=%q", err, f.set.host)
	}
}
