package stackport

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

func radioAnswer(findings string) func(context.Context) (json.RawMessage, error) {
	return func(context.Context) (json.RawMessage, error) {
		return json.RawMessage(`{"findings":[` + findings + `]}`), nil
	}
}

func homeOf(devs []models.NetworkDevice, presence models.DevicePresence) Home {
	return Home{
		Devices:  func() (models.DeviceInventory, error) { return models.DeviceInventory{Devices: devs, Changes: []models.AddressChange{{}}}, nil },
		Presence: func(models.NetworkDevice) models.DevicePresence { return presence },
		RadioOn:  func(context.Context) bool { return true },
		Radio:    radioAnswer(`{"id":"radio_rogue_twin","tier":"seen","count":2,"claim":"EVIL SSID \"HomeWifi\" at 10.0.0.9"},{"id":"made_up","tier":"seen","count":9}`),
	}
}

// The lines carry counts and the core's own words, and no device name, address or text the stack sent.
func TestHome_LinesHoldOnlyCountsAndTheCoresWords(t *testing.T) {
	devs := []models.NetworkDevice{
		{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.20", Name: "Kitchen speaker", Approved: true},
		{MAC: "aa:bb:cc:dd:ee:02", IP: "192.168.1.21", Name: "Mystery phone"},
	}
	lines, err := homeOf(devs, models.PresenceOnline).Lines(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]stacksdk.HomeLine{}
	for _, l := range lines {
		byID[l.ID] = l
	}
	if byID[HomeNetworkOnline].Count != 2 || byID[HomeNetworkNotMine].Count != 1 || byID[HomeNetworkChanges].Count != 1 {
		t.Errorf("network counts: %+v", lines)
	}
	if byID["radio_rogue_twin"].Count != 2 {
		t.Errorf("radio finding missing: %+v", lines)
	}
	if _, ok := byID["made_up"]; ok {
		t.Error("a finding id the core has no words for must be dropped")
	}
	all, _ := json.Marshal(lines)
	for _, leak := range []string{"Kitchen", "Mystery", "192.168", "aa:bb", "EVIL", "HomeWifi", "10.0.0.9"} {
		if strings.Contains(string(all), leak) {
			t.Errorf("%q reached the stack: %s", leak, all)
		}
	}
}

// A presence check that is off says it cannot say, instead of reporting "0 online".
func TestHome_PresenceUnknownIsSaidNotCountedAsZero(t *testing.T) {
	lines, _ := homeOf([]models.NetworkDevice{{MAC: "aa:bb:cc:dd:ee:01", Approved: true}}, models.PresenceUnknown).Lines(context.Background())
	if lines[0].ID != HomeNetworkUnknown {
		t.Errorf("want the unknown line first: %+v", lines)
	}
}

// Radio off, unreadable or quiet each give one fixed line; a failing network read does not hide Radio.
func TestHome_EachPartFailsOnItsOwn(t *testing.T) {
	h := homeOf(nil, models.PresenceOnline)
	h.RadioOn = func(context.Context) bool { return false }
	h.Devices = func() (models.DeviceInventory, error) { return models.DeviceInventory{}, errors.New("disk") }
	lines, err := h.Lines(context.Background())
	if err != nil || len(lines) != 2 || lines[0].ID != HomeNetworkOff || lines[1].ID != HomeRadioOff {
		t.Fatalf("%+v, %v", lines, err)
	}
	h.RadioOn = func(context.Context) bool { return true }
	h.Radio = radioAnswer("")
	if lines, _ = h.Lines(context.Background()); lines[1].ID != HomeRadioQuiet {
		t.Errorf("quiet: %+v", lines)
	}
	h.Radio = func(context.Context) (json.RawMessage, error) { return nil, errors.New("down") }
	if lines, _ = h.Lines(context.Background()); lines[1].ID != HomeRadioUnreadable {
		t.Errorf("unreadable: %+v", lines)
	}
}

// The answer is bounded and every count is clamped.
func TestHome_IsBoundedAndClamped(t *testing.T) {
	var devs []models.NetworkDevice
	for range 200_000 {
		devs = append(devs, models.NetworkDevice{})
	}
	lines, _ := homeOf(devs, models.PresenceOnline).Lines(context.Background())
	if len(lines) > stacksdk.MaxHomeLines {
		t.Errorf("%d lines", len(lines))
	}
	if lines[0].Count != maxHomeCount {
		t.Errorf("count %d not clamped", lines[0].Count)
	}
}

// HomeLine keeps exactly these fields: a new text field would have to be argued for (ADR-0077).
func TestHomeLine_HasNoFieldBeyondIDCountWords(t *testing.T) {
	typ := reflect.TypeOf(stacksdk.HomeLine{})
	var got []string
	for i := range typ.NumField() {
		got = append(got, typ.Field(i).Name)
	}
	if !reflect.DeepEqual(got, []string{"ID", "Count", "Words"}) {
		t.Errorf("fields %v", got)
	}
}
