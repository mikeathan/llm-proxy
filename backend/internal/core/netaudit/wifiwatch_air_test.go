package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit/wifilook"
	"llm-proxy/models"
)

// withAir gives the rig a Radio source that answers sightings, or is unavailable when off is true.
func (r *wifiRig) withAir(t *testing.T, sightings *[]models.WifiSighting, off *bool) {
	t.Helper()
	r.w.deps.Air = func(context.Context) ([]models.WifiSighting, bool) {
		if *off {
			return nil, false
		}
		return *sightings, true
	}
}

// One check, two sensors: an access point only Radio heard, under the owner's name and not confirmed, is a look-alike told in the same message,
// with the same throttle, as one this computer's own Wi-Fi heard.
func TestWifiWatch_AnAccessPointOnlyRadioHeardIsJudgedTheSameWay(t *testing.T) {
	r := newWifiRig(t)
	r.confirmed(t)
	sightings, off := []models.WifiSighting{{Name: "home", BSSID: twin, Security: "wpa2"}}, false
	r.withAir(t, &sightings, &off)
	r.w.Tick(context.Background())
	if len(r.sent) != 1 || !strings.Contains(r.sent[0], twin) {
		t.Fatalf("sent = %v, want one message naming the access point Radio heard", r.sent)
	}
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Errorf("the throttle covers Radio's sightings too: %v", r.sent)
	}
	v := r.w.View()
	var seen *WifiSeen
	for i := range v.Networks[0].Seen {
		if v.Networks[0].Seen[i].BSSID == twin {
			seen = &v.Networks[0].Seen[i]
		}
	}
	if seen == nil || !seen.Radio {
		t.Fatalf("the card lists what Radio heard, marked so, for the owner to confirm: %+v", v)
	}
	if err := r.w.Confirm("home", []string{twin}); err != nil {
		t.Errorf("an access point Radio heard can be confirmed on the card: %v", err)
	}
}

// An access point the owner confirmed on the Radio page before the lists were one is shown as such and not alerted, until the owner confirms
// it once on the card; it is never treated as confirmed.
func TestWifiWatch_AnAccessPointConfirmedInRadioBeforeIsShownNotAlerted(t *testing.T) {
	r := newWifiRig(t)
	r.confirmed(t)
	sightings, off := []models.WifiSighting{{Name: "home", BSSID: twin, Security: "wpa2", ConfirmedBefore: true}}, false
	r.withAir(t, &sightings, &off)
	r.w.Tick(context.Background())
	if len(r.sent) != 0 || len(WifiFindings(r.w.Snapshot(r.now))) != 0 {
		t.Fatalf("sent = %v; confirmed in Radio before, it waits for the owner and raises nothing", r.sent)
	}
	v := r.w.View()
	found := false
	for _, s := range v.Networks[0].Seen {
		if s.BSSID == twin && s.ConfirmedBefore && !s.Confirmed {
			found = true
		}
	}
	if !found {
		t.Errorf("the card shows it as confirmed in Radio before, not as confirmed: %+v", v)
	}
}

// Network does not depend on Radio: with Radio off the check works from this computer's own Wi-Fi as before; and when this computer's look
// fails but Radio answered, the look worked and is not a failed part.
func TestWifiWatch_EachSensorWorksWithoutTheOther(t *testing.T) {
	r := newWifiRig(t)
	r.confirmed(t)
	sightings, off := []models.WifiSighting(nil), true
	r.withAir(t, &sightings, &off)
	r.aps = append(r.aps, ap("home", twin, wifilook.WPA2))
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("with Radio off this computer's look still tells: %v", r.sent)
	}
	r.err, off, sightings = errors.New("nmcli is not installed"), false, []models.WifiSighting{{Name: "home", BSSID: twin2, Security: "open"}}
	r.w.Tick(context.Background())
	if r.w.Reading().Failed || r.w.View().Problem != "" {
		t.Errorf("Radio answered, so the look worked: %+v", r.w.View())
	}
	if len(r.sent) != 2 || !strings.Contains(r.sent[1], twin2) {
		t.Errorf("sent = %v, want Radio's look-alike told", r.sent)
	}
	off = true
	r.w.Tick(context.Background())
	if !r.w.Reading().Failed {
		t.Error("with neither sensor answering the look failed")
	}
}
