package stackport

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/radiodigest"
	"llm-proxy/models"
)

func radioAnswer(findings string) func(context.Context) (json.RawMessage, error) {
	return func(context.Context) (json.RawMessage, error) {
		return json.RawMessage(`{"findings":[` + findings + `]}`), nil
	}
}

func homeOf(devs []models.NetworkDevice, presence models.DevicePresence) Home {
	return Home{
		Devices: func() (models.DeviceInventory, error) {
			return models.DeviceInventory{Devices: devs, Changes: []models.AddressChange{{}}}, nil
		},
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
	if lines[0].ID == HomeNetworkOnline || !slices.ContainsFunc(lines, func(l stacksdk.HomeLine) bool { return l.ID == HomeNetworkUnknown }) {
		t.Errorf("want the unknown line, not an online count: %+v", lines)
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

// HomeLine keeps exactly these fields: a new text field would have to be argued for (ADR-0077). Level (ADR-0084) is one of three fixed words, never text.
func TestHomeLine_HasNoFieldBeyondIDCountWordsLevel(t *testing.T) {
	typ := reflect.TypeOf(stacksdk.HomeLine{})
	var got []string
	for i := range typ.NumField() {
		got = append(got, typ.Field(i).Name)
	}
	if !reflect.DeepEqual(got, []string{"ID", "Count", "Words", "Level"}) {
		t.Errorf("fields %v", got)
	}
}

// Every fact the core can word has a level, so a new id cannot reach the panel or the phone without being ranked.
func TestHomeLevel_EveryWordedIDHasOne(t *testing.T) {
	ids := append(slices.Collect(maps.Keys(homeWords)), radiodigest.FindingIDs()...)
	for _, id := range ids {
		if !slices.Contains(homeLevelValues, HomeLevel(id)) {
			t.Errorf("%s has level %q", id, HomeLevel(id))
		}
		if _, listed := homeLevels[id]; !listed {
			t.Errorf("%s is not in the level table", id)
		}
	}
	if got := HomeLevel("made_up"); got != stacksdk.HomeLevelInfo {
		t.Errorf("an unknown id is routine, not an alarm: %q", got)
	}
}

// Critical is kept to what is serious on its own: an open or outdated access point of the owner's. Nothing else interrupts.
func TestHomeLevel_OnlyAnOwnAccessPointThatIsOpenOrWeakIsCritical(t *testing.T) {
	var critical []string
	for id, lv := range homeLevels {
		if lv == stacksdk.HomeLevelCritical {
			critical = append(critical, id)
		}
	}
	if !reflect.DeepEqual(critical, []string{"radio_open_or_weak_ap"}) {
		t.Errorf("critical ids: %v", critical)
	}
}

// Lines carry their level, and the lines that need a look come before the routine ones, so the cut at the bound never drops them.
func TestHome_LinesAreRankedAndCarryTheirLevel(t *testing.T) {
	h := homeOf([]models.NetworkDevice{{MAC: "aa:bb:cc:dd:ee:01", Approved: true}}, models.PresenceOnline)
	h.Radio = radioAnswer(`{"id":"radio_hidden_ap_near","tier":"seen","count":1},{"id":"radio_open_or_weak_ap","tier":"seen","count":1}`)
	lines, _ := h.Lines(context.Background())
	var levels []string
	for _, l := range lines {
		levels = append(levels, l.Level)
	}
	if lines[0].ID != "radio_open_or_weak_ap" || levels[0] != stacksdk.HomeLevelCritical {
		t.Fatalf("critical first: %+v", lines)
	}
	if !sort.SliceIsSorted(lines, func(i, j int) bool { return homeRank(lines[i].Level) < homeRank(lines[j].Level) }) {
		t.Errorf("not ranked: %v", levels)
	}
	var devs []models.NetworkDevice
	for range 40 {
		devs = append(devs, models.NetworkDevice{})
	}
	h = homeOf(devs, models.PresenceOnline)
	var many []string
	// Radio keeps at most 16 findings; 15 routine ones and the critical one fill that, and with the network's lines the answer is over the bound.
	for range stacksdk.MaxHomeLines - 1 {
		many = append(many, `{"id":"radio_hidden_ap_near","tier":"seen","count":1}`)
	}
	h.Radio = radioAnswer(strings.Join(append(many, `{"id":"radio_open_or_weak_ap","tier":"seen","count":1}`), ","))
	lines, _ = h.Lines(context.Background())
	if len(lines) > stacksdk.MaxHomeLines || lines[0].ID != "radio_open_or_weak_ap" {
		t.Errorf("the cut dropped the critical line: %d lines, first %s", len(lines), lines[0].ID)
	}
}
