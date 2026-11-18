package routerconn

import (
	"reflect"
	"strings"
	"testing"

	"llm-proxy/models"
)

// measuredShape is the shape read from the owner's ASUS AiMesh router on 2026-11-18, with made-up addresses: the satellite box first, the main
// box with the satellite's two backhaul radios as clients, a guest band and a wired list.
const measuredShape = `{"02:00:00:00:00:f0":{"2G":{"0a:00:00:00:00:1c":{"ip":"192.168.50.160","rssi":"-53"}},"5G":{"6e:00:00:00:00:90":{"ip":"192.168.50.127","rssi":"-69"}}},
"24:00:00:00:00:60":{"2G":{"02:00:00:00:00:f1":{"ip":"","rssi":"-52"},"0a:00:00:00:00:84":{"ip":"192.168.50.220","rssi":"-56"}},"2G_1":{"0e:00:00:00:00:08":{"ip":"192.168.101.212","rssi":"-61"}},
"5G":{"02:00:00:00:00:f4":{"ip":"","rssi":"-61"},"0a:00:00:00:00:23":{"ip":"192.168.50.10","rssi":"-57"}},"wired_mac":{"00:1c:2b:00:00:c7":{"ip":"192.168.50.107"}}}}`

func TestTheClientListGivesEachDeviceItsBoxBandAndSignalAndNothingElse(t *testing.T) {
	got, err := ParseClients([]byte(measuredShape))
	if err != nil {
		t.Fatal(err)
	}
	want := []models.MeshClient{
		{MAC: "6e:00:00:00:00:90", Box: "02:00:00:00:00:f0", Band: "5G", RSSI: -69},
		{MAC: "0a:00:00:00:00:1c", Box: "02:00:00:00:00:f0", Band: "2G", RSSI: -53},
		{MAC: "0a:00:00:00:00:23", Box: "24:00:00:00:00:60", Band: "5G", RSSI: -57},
		{MAC: "0a:00:00:00:00:84", Box: "24:00:00:00:00:60", Band: "2G", RSSI: -56},
		{MAC: "0e:00:00:00:00:08", Box: "24:00:00:00:00:60", Band: "2G", Guest: true, RSSI: -61},
	}
	byMAC := func(v []models.MeshClient) map[string]models.MeshClient {
		m := map[string]models.MeshClient{}
		for _, c := range v {
			m[c.MAC] = c
		}
		return m
	}
	if !reflect.DeepEqual(byMAC(got), byMAC(want)) {
		t.Errorf("the backhaul radios and the wired list are dropped, the guest band is marked:\ngot  %+v\nwant %+v", got, want)
	}
	if strings.Contains(strings.ToLower(strings.Join(func() []string {
		var s []string
		for _, c := range got {
			s = append(s, c.MAC+c.Box+c.Band)
		}
		return s
	}(), "")), "192.168") {
		t.Error("no address on this network is kept")
	}
}

func TestTheClientListIsReadTheSameWhateverTheOrderOfItsBoxes(t *testing.T) {
	a, _ := ParseClients([]byte(`{"24:00:00:00:00:60":{"5G":{"0a:00:00:00:00:01":{"rssi":"-50"}}},"02:00:00:00:00:f0":{"5G":{"0a:00:00:00:00:02":{"rssi":"-60"}}}}`))
	b, _ := ParseClients([]byte(`{"02:00:00:00:00:f0":{"5G":{"0a:00:00:00:00:02":{"rssi":"-60"}}},"24:00:00:00:00:60":{"5G":{"0a:00:00:00:00:01":{"rssi":"-50"}}}}`))
	if !reflect.DeepEqual(a, b) || len(a) != 2 {
		t.Errorf("%+v %+v", a, b)
	}
}

func TestABadClientListIsRefusedAndABadEntrySkipped(t *testing.T) {
	for _, raw := range []string{``, `[]`, `{"not a mac":{}}`, `{"24:00:00:00:00:60":{"5G":[1]}}`, `{"24:00:00:00:00:60":"x"}`} {
		if _, err := ParseClients([]byte(raw)); err == nil {
			t.Errorf("%q must be refused", raw)
		}
	}
	got, err := ParseClients([]byte(`{"24:00:00:00:00:60":{"5G":{"zz":{"rssi":"-50"},"0a:00:00:00:00:01":{"rssi":"-500"},"0a:00:00:00:00:02":{"rssi":"5"},"0a:00:00:00:00:03":{"rssi":"x"},"0a:00:00:00:00:04":{"rssi":"-40"}},"7G":{"0a:00:00:00:00:05":{"rssi":"-40"}}}}`))
	if err != nil || len(got) != 1 || got[0].MAC != "0a:00:00:00:00:04" {
		t.Errorf("a bad address, an impossible signal and an unknown band are skipped: %+v %v", got, err)
	}
	var many strings.Builder
	many.WriteString("{")
	for i := range maxMeshBoxes + 1 {
		if i > 0 {
			many.WriteString(",")
		}
		many.WriteString(`"24:00:00:00:00:` + string("0123456789"[i]) + `0":{}`)
	}
	many.WriteString("}")
	if _, err := ParseClients([]byte(many.String())); err == nil {
		t.Errorf("more than %d boxes is refused", maxMeshBoxes)
	}
}

func FuzzParseClients(f *testing.F) {
	f.Add([]byte(measuredShape))
	f.Add([]byte(`{"24:00:00:00:00:60":{"5G":{"0a:00:00:00:00:01":{"rssi":"-50"}}}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		got, err := ParseClients(raw)
		if err != nil {
			return
		}
		if len(got) > maxMeshClients {
			t.Fatalf("%d devices", len(got))
		}
		for _, c := range got {
			if c.RSSI < minRSSI || c.RSSI > 0 || len(c.MAC) != 17 || len(c.Box) != 17 || (c.Band != "2G" && c.Band != "5G" && c.Band != "6G") {
				t.Fatalf("%+v", c)
			}
		}
	})
}
