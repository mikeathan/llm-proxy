package sources

import (
	"slices"
	"strings"
	"testing"
)

// The built-in list must stay usable without the network: every entry passes the checks a topic applies on save, nothing repeats, and every category
// is one the form lists.
func TestCatalog_EveryEntryIsValidUniqueAndInAKnownCategory(t *testing.T) {
	labels, targets := map[string]bool{}, map[string]bool{}
	for _, e := range Catalog() {
		if err := e.Spec().Canonical().Validate(); err != nil {
			t.Errorf("%s: %v", e.Label, err)
		}
		if e.Kind != KindFeed && e.Kind != KindNews {
			t.Errorf("%s: a topic reads a feed or a news search here, not %q", e.Label, e.Kind)
		}
		if !slices.Contains(Categories, e.Category) {
			t.Errorf("%s: category %q is not listed in Categories", e.Label, e.Category)
		}
		if e.Kind == KindFeed && !strings.HasPrefix(e.Target, "https://") && !strings.HasPrefix(e.Target, "http://") {
			t.Errorf("%s: a feed target is an address", e.Label)
		}
		if k := strings.ToLower(e.Label); labels[k] {
			t.Errorf("label %q is used twice", e.Label)
		} else {
			labels[k] = true
		}
		if k := strings.ToLower(e.Target); targets[k] {
			t.Errorf("target %q is used twice", e.Target)
		} else {
			targets[k] = true
		}
	}
	if len(Catalog()) < 25 {
		t.Errorf("the starter list has %d entries, expected at least 25", len(Catalog()))
	}
}

func TestCatalog_ReturnsACopy(t *testing.T) {
	c := Catalog()
	c[0].Label = "changed"
	if Catalog()[0].Label == "changed" {
		t.Error("changing the returned list changed the built-in one")
	}
}
