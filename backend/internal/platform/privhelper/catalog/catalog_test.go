package catalog

import (
	"errors"
	"slices"
	"testing"

	"llm-proxy/internal/platform/privhelper/wire"
)

// Fitness test 3 (design section 10): every row has a class, a switch, caps, a result kind and a jail, and takes its arguments only from
// its typed parameters.
func TestEveryActionIsFullyDescribed(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range All() {
		switch {
		case a.ID == "" || seen[a.ID]:
			t.Errorf("action %q: the ID is empty or repeated", a.ID)
		case a.Class != ClassNet && a.Class != ClassLog:
			t.Errorf("%s: unknown class %q", a.ID, a.Class)
		case a.Switch != SwitchKey:
			t.Errorf("%s: the switch must be %s, got %q", a.ID, SwitchKey, a.Switch)
		case a.Caps.Timeout <= 0 || a.Caps.MaxOutputBytes <= 0 || a.Caps.MaxPerHour <= 0:
			t.Errorf("%s: every cap is required: %+v", a.ID, a.Caps)
		case a.Caps.MaxOutputBytes > wire.MaxResponseBytes:
			t.Errorf("%s: output cap %d is over the wire's %d", a.ID, a.Caps.MaxOutputBytes, wire.MaxResponseBytes)
		case a.Result == "":
			t.Errorf("%s: no result kind", a.ID)
		case a.prepare == nil:
			t.Errorf("%s: no builder", a.ID)
		}
		seen[a.ID] = true
		if got, err := Lookup(a.ID); err != nil || got.ID != a.ID {
			t.Errorf("%s: Lookup = %v, %v", a.ID, got.ID, err)
		}
	}
	if len(seen) == 0 {
		t.Fatal("the catalogue is empty: the test would pass over nothing")
	}
}

func TestTheFirstCatalogueIsTheOneTheOwnerApproved(t *testing.T) {
	var ids []string
	for _, a := range All() {
		ids = append(ids, a.ID)
	}
	if want := []string{"scan_udp", "os_detect", "capture_window"}; !slices.Equal(ids, want) {
		t.Errorf("actions = %v, want %v (D1: read_auth_log is deferred; capture_window was added 2026-11-08 by the owner's go-ahead, plan network-capture-window-plan)", ids, want)
	}
}

func TestAnUnknownActionIsRefused(t *testing.T) {
	_, err := Lookup("rm_rf")
	if !errors.Is(err, ErrUnknown) || Code(err) != wire.CodeUnknown {
		t.Errorf("got %v (%s)", err, Code(err))
	}
}

func TestCodesFollowTheKindOfRefusal(t *testing.T) {
	cases := map[error]wire.Code{ErrScope: wire.CodeScope, ErrInvalid: wire.CodeInvalid, ErrUnknown: wire.CodeUnknown}
	for err, want := range cases {
		if got := Code(err); got != want {
			t.Errorf("%v: got %s, want %s", err, got, want)
		}
	}
}
