package footprint

import (
	"context"
	"os"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func registryFor(t *testing.T) (*Registry, *fakeOpener) {
	op := &fakeOpener{answers: map[string]string{
		BootstrapURL: fixture(t, "iana-dns-bootstrap.json"),
		"https://rdap.verisign.com/com/v1/domain/example.com": fixture(t, "rdap-example-com.json"),
	}}
	return &Registry{Opener: op, Now: func() time.Time { return time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC) }}, op
}

// The fixtures are the real answers of IANA's bootstrap list and Verisign's RDAP server for example.com (2026-11-10).
func TestRegistry_ReadsTheExpiryOfARealAnswer(t *testing.T) {
	r, _ := registryFor(t)
	got, err := r.Expiry(context.Background(), "example.com")
	if err != nil || !got.Equal(time.Date(2027, 8, 13, 4, 0, 0, 0, time.UTC)) {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestRegistry_AskesIANAOncePerDay(t *testing.T) {
	r, op := registryFor(t)
	for i := 0; i < 3; i++ {
		if _, err := r.Expiry(context.Background(), "example.com"); err != nil {
			t.Fatal(err)
		}
	}
	boot := 0
	for _, u := range op.requests {
		if u == BootstrapURL {
			boot++
		}
	}
	if boot != 1 {
		t.Errorf("bootstrap asked %d times", boot)
	}
}

// A registry that is listed only over plain http is not used: the answer would travel unprotected.
func TestRegistry_AnEndingServedOnlyOverPlainHTTPHasNoRegistry(t *testing.T) {
	r, op := registryFor(t)
	_, err := r.Expiry(context.Background(), "example.kg")
	wantReason(t, err, ReasonNoRegistry)
	for _, u := range op.requests {
		if u != BootstrapURL {
			t.Errorf("asked %s", u)
		}
	}
}

func TestRegistry_AnUnlistedEndingHasNoRegistry(t *testing.T) {
	r, _ := registryFor(t)
	_, err := r.Expiry(context.Background(), "example.zz")
	wantReason(t, err, ReasonNoRegistry)
}

func TestRegistry_NoExpiryEventIsNotFine(t *testing.T) {
	r, op := registryFor(t)
	op.answers["https://rdap.verisign.com/com/v1/domain/example.com"] = `{"events":[{"eventAction":"registration","eventDate":"1995-08-14T04:00:00Z"}]}`
	_, err := r.Expiry(context.Background(), "example.com")
	wantReason(t, err, ReasonNoExpiry)
}

func TestRegistry_ARegistryThatIsDownIsDownNotExpiryUnknownEmpty(t *testing.T) {
	r, op := registryFor(t)
	delete(op.answers, "https://rdap.verisign.com/com/v1/domain/example.com")
	_, err := r.Expiry(context.Background(), "example.com")
	wantReason(t, err, ReasonDown)
}

func TestRegistry_GarbageIsUnreadable(t *testing.T) {
	r, op := registryFor(t)
	op.answers["https://rdap.verisign.com/com/v1/domain/example.com"] = "<html>"
	_, err := r.Expiry(context.Background(), "example.com")
	wantReason(t, err, ReasonUnreadable)
}
