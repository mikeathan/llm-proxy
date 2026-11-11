package footprint

import (
	"encoding/json"
	"strings"
	"testing"

	"llm-proxy/models"
)

// The page never receives a full address, and says what each service learns before it is switched on.
func TestViewOf_MasksAddressesAndSaysWhatEachServiceLearns(t *testing.T) {
	v := ViewOf(cfg(), allFresh(), true, now)
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "jo@example.com") {
		t.Errorf("the full address reached the view: %s", raw)
	}
	var email WatchView
	for _, w := range v.Watches {
		if w.Kind == "email" {
			email = w
		}
	}
	if email.Name != "j***@example.com" || email.Label != "email:a1" {
		t.Errorf("%+v", email)
	}
	for _, s := range v.Services {
		if s.Learns == "" {
			t.Errorf("%s says nothing about what it learns", s.ID)
		}
	}
}

func TestViewOf_AFailedLookCarriesAFixedSentenceAndAStaleOneIsMarked(t *testing.T) {
	rs := allFresh()
	rs[2].Status, rs[2].Reason = models.FootprintCouldNotLook, ReasonRateLimit
	rs[0].LookedAt = now.Add(-5 * day)
	v := ViewOf(cfg(), rs, false, now)
	got := v.Watches[0].Results
	if got[2].ReasonText != ReasonText(ReasonRateLimit) || !got[0].Stale {
		t.Errorf("%+v", got)
	}
	if v.KeySaved {
		t.Error("no key saved")
	}
}
