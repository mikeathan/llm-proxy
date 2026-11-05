package netaudit

import (
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit/phonehome"
	"llm-proxy/models"
)

func phInv() models.DeviceInventory {
	return models.DeviceInventory{Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.20", Name: "Living room TV"},
		{MAC: "aa:bb:cc:00:00:02"}, // seen only over IPv6: no address to key a finding by
	}}
}

func TestPhoneHomeFindings_OneFindingPerDeviceAndKindKeyedByTheDevicesAddress(t *testing.T) {
	got := PhoneHomeFindings([]phonehome.Signal{
		{Kind: phonehome.NewPlace, MAC: "aa:bb:cc:00:00:01", Names: []string{"b.example.org", "a.example.org"}},
		{Kind: phonehome.Beaconing, MAC: "aa:bb:cc:00:00:01", Names: []string{"a.example.org"}},
		{Kind: phonehome.AroundDNS, MAC: "aa:bb:cc:00:00:01"},
		{Kind: phonehome.NewPlace, MAC: "aa:bb:cc:00:00:02", Names: []string{"x.example.org"}},
		{Kind: phonehome.NewPlace, MAC: "aa:bb:cc:99:99:99", Names: []string{"x.example.org"}},
	}, phInv())
	if len(got) != 3 {
		t.Fatalf("findings = %+v; want three (the device with no address and the unknown one have none)", got)
	}
	for _, f := range got {
		if f.Device != "192.168.1.20" || f.Port != 0 {
			t.Errorf("finding = %+v, want it keyed by the device's address", f)
		}
	}
	checks := map[string]Finding{}
	for _, f := range got {
		checks[f.Check] = f
	}
	if f := checks[CheckPhoneHomeNewPlace]; !strings.Contains(f.Evidence, "a.example.org, b.example.org") || f.Version == "" {
		t.Errorf("new place = %+v, want its names sorted in the evidence and a version that changes with the set", f)
	}
	if f := checks[CheckPhoneHomeAroundDNS]; f.Evidence == "" || f.Version != "" {
		t.Errorf("around dns = %+v", f)
	}
}

func TestPhoneHomeFindings_TheVersionChangesOnlyWhenTheSetOfNamesGrows(t *testing.T) {
	one := PhoneHomeFindings([]phonehome.Signal{{Kind: phonehome.NewPlace, MAC: "aa:bb:cc:00:00:01", Names: []string{"a.example.org"}}}, phInv())[0]
	same := PhoneHomeFindings([]phonehome.Signal{{Kind: phonehome.NewPlace, MAC: "aa:bb:cc:00:00:01", Names: []string{"a.example.org"}}}, phInv())[0]
	more := PhoneHomeFindings([]phonehome.Signal{{Kind: phonehome.NewPlace, MAC: "aa:bb:cc:00:00:01", Names: []string{"a.example.org", "b.example.org"}}}, phInv())[0]
	if one.Version != same.Version || one.Version == more.Version {
		t.Fatalf("versions %q %q %q: an acceptance must hold for the same names and lapse for a larger set", one.Version, same.Version, more.Version)
	}
}

func TestPhoneHomeFindings_NeverCarryTheDeviceNameAndCapTheNamesShown(t *testing.T) {
	var names []string
	for i := 0; i < 20; i++ {
		names = append(names, "host"+string(rune('a'+i))+".example.org")
	}
	f := PhoneHomeFindings([]phonehome.Signal{{Kind: phonehome.NewPlace, MAC: "aa:bb:cc:00:00:01", Names: names}}, phInv())[0]
	if strings.Contains(f.Evidence, "Living room") {
		t.Fatalf("evidence = %q: the operator's device name is not in a finding", f.Evidence)
	}
	if strings.Count(f.Evidence, "example.org") > maxPhoneHomeNames || !strings.Contains(f.Evidence, "more") {
		t.Fatalf("evidence = %q: at most %d names, and the rest counted", f.Evidence, maxPhoneHomeNames)
	}
}

func TestPhoneHomeFindings_HaveWordsForTheScreenAndAFix(t *testing.T) {
	for _, c := range []string{CheckPhoneHomeNewPlace, CheckPhoneHomeBeaconing, CheckPhoneHomeAroundDNS} {
		if checkTitles[c] == "" || remediations[c] == "" {
			t.Errorf("%s has no title or no fix text", c)
		}
	}
}
