package phonehome

import (
	"testing"
	"time"

	"llm-proxy/models"
)

func TestRegistrableKeepsTheDomainAPersonKnows(t *testing.T) {
	for in, want := range map[string]string{
		"a.b.googleapis.com": "googleapis.com", "apple.com": "apple.com", "www.bbc.co.uk": "bbc.co.uk", "bbc.co.uk": "bbc.co.uk",
		"localhost": "localhost", "Time.Apple.COM.": "apple.com", "x.example.com.au": "example.com.au",
	} {
		if got := Registrable(in); got != want {
			t.Errorf("%s = %s, want %s", in, got, want)
		}
	}
}

func TestLookupsListEachDeviceWithItsNewestNamesAndTheSeriousFlagPerName(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	state := models.PhoneHome{Devices: []models.PhoneHomeDevice{
		{MAC: "aa", First: now.Add(-48 * time.Hour), Last: now, Names: []models.PhoneHomeName{
			{Name: "old.example.com", Last: now.Add(-30 * time.Hour), Count: 2},
			{Name: "api.apple.com", Last: now.Add(-time.Hour), Count: 9},
			{Name: "bad.example.net", Last: now.Add(-time.Minute), Count: 1},
		}},
		{MAC: "bb", First: now.Add(-time.Hour), Last: now, Names: []models.PhoneHomeName{{Name: "x.y.z.com", Last: now, Count: 1}}},
	}}
	signals := []Signal{
		{Kind: NewPlace, MAC: "aa", Names: []string{"bad.example.net"}},
		{Kind: KnownBadName, MAC: "aa", Names: []string{"bad.example.net"}},
		{Kind: AroundDNS, MAC: "cc"},
	}
	got := Lookups(state, signals, now)
	if len(got) != 3 || got[0].MAC != "aa" || got[1].MAC != "cc" || got[2].MAC != "bb" {
		t.Fatalf("order: %+v", got)
	}
	aa := got[0]
	if aa.Today != 2 || aa.Total != 3 || aa.Names[0].Name != "bad.example.net" || aa.Names[0].Flag != string(KnownBadName) || aa.Names[1].Domain != "apple.com" || aa.Names[1].Flag != "" {
		t.Errorf("aa: %+v", aa)
	}
	if len(aa.Flags) != 2 || len(got[1].Names) != 0 || got[1].Flags[0] != string(AroundDNS) {
		t.Errorf("flags: %+v %+v", aa.Flags, got[1])
	}
}

func TestLookupsKeepOnlyTheNewestNamesOfADevice(t *testing.T) {
	now := time.Now()
	d := models.PhoneHomeDevice{MAC: "aa", Last: now}
	for i := range MaxLookupNames + 20 {
		d.Names = append(d.Names, models.PhoneHomeName{Name: "n.example.com", Last: now.Add(-time.Duration(i) * time.Second)})
	}
	got := Lookups(models.PhoneHome{Devices: []models.PhoneHomeDevice{d}}, nil, now)
	if len(got[0].Names) != MaxLookupNames || got[0].Total != MaxLookupNames+20 {
		t.Errorf("%d names, %d total", len(got[0].Names), got[0].Total)
	}
}
