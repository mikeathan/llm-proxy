package radio

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// retryData is a data frame from a device with the retry bit set.
func retryData(from MAC) []byte {
	return rtap(cat(hdr(0x88, 0x09, macAP, from, macAP), []byte{0, 0}, make([]byte, 10)))
}

func getQ(t *testing.T, r *listenRig, path, query string, v any) {
	t.Helper()
	resp, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: path, Query: query})
	if err != nil || resp.Status != http.StatusOK {
		t.Fatalf("%s?%s: %+v %v", path, query, resp, err)
	}
	if err := json.Unmarshal(resp.Body, v); err != nil {
		t.Fatal(err)
	}
}

func TestAirDevicesListsEachNeighbourWithWhatTheEvidenceIs(t *testing.T) {
	r := devRig(t)
	rec, _, err := r.s.recorder(r.s.h)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 14; i++ {
		frames := [][]byte{data(neighbour)}
		if i < 7 {
			frames = append(frames, retryData(neighbour))
		}
		cycleOf(t, rec, r.now, frames...)
	}
	var v AirDevices
	getQ(t, r, AirDevicesPath, "", &v)
	if len(v.Devices) != 1 || v.Listened != 14 || v.MinListened != 12 {
		t.Fatalf("%+v", v)
	}
	d := v.Devices[0]
	if d.Class != "infrastructure" || d.Why != "always_here_steady_signal" || d.CyclesHeard != 14 || d.Signal != "strong" {
		t.Errorf("%+v", d)
	}
	if len(d.ID) != 8 || len(d.Channels) != 1 || d.Channels[0] != 60 {
		t.Errorf("a short tag and the channel it was heard on: %+v", d)
	}
	if d.RetryPercent != 33 { // 7 retried of 21 frames
		t.Errorf("retry share %d, want 33", d.RetryPercent)
	}
	if d.FirstSeen == "" || d.LastSeen == "" {
		t.Errorf("%+v", d)
	}
}

func TestAirDevicesNeverCarriesAnAddressOrTheFullTag(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	cycleOf(t, rec, r.now, data(neighbour))
	resp, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: AirDevicesPath})
	tag, _ := fakeVault{}.Tag(context.Background(), tagDevice, neighbour[:])
	if strings.Contains(string(resp.Body), hex.EncodeToString(tag)) || strings.Contains(string(resp.Body), "b8:27") || strings.Contains(string(resp.Body), "b827") {
		t.Errorf("a short id only, never an address: %s", resp.Body)
	}
}

func TestAirDevicesFiltersByClassSortsAndIsBounded(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	cycleOf(t, rec, r.now, data(neighbour))
	var v AirDevices
	getQ(t, r, AirDevicesPath, url.Values{"class": {"personal"}}.Encode(), &v)
	if len(v.Devices) != 0 {
		t.Errorf("%+v", v)
	}
	getQ(t, r, AirDevicesPath, url.Values{"class": {"unknown"}, "sort": {"cycles"}}.Encode(), &v)
	if len(v.Devices) != 1 {
		t.Errorf("%+v", v)
	}
	for _, bad := range []string{"class=bogus", "sort=bogus"} {
		resp, _ := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: AirDevicesPath, Query: bad})
		if resp.Status != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, resp.Status)
		}
	}
}

// A device heard outside a finished cycle used to stay unsorted, so the card counted it and the log did not (31 against 22 on vertex).
func TestEveryKnownDeviceIsSortedSoTheCardAndTheListAgree(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	record(t, rec, data(neighbour), r.now) // heard with no cycle open: a device row and no daily row
	cycleOf(t, rec, r.now, data(MAC{0xb8, 0x27, 0xeb, 1, 2, 3}))
	var air AirView
	get(t, r, AirPath, &air)
	var v AirDevices
	getQ(t, r, AirDevicesPath, "", &v)
	if air.Total != 2 || len(v.Devices) != 2 || air.Counts["unknown"] != 2 {
		t.Fatalf("%+v %+v", air, v)
	}
	for _, d := range v.Devices {
		if d.Why != "needs_more_listening" || d.NeedHeard != 3 {
			t.Errorf("an unsorted device says what it waits for: %+v", d)
		}
	}
	if len(air.Changes) == 0 {
		t.Error("the sort is logged")
	}
}

func TestPresenceShowsTheOwnersDevicesHourByHour(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	ctx := context.Background()
	if err := rec.AddOwnerDevice(ctx, mineMAC, "my phone", r.now); err != nil {
		t.Fatal(err)
	}
	for h := 0; h < 3; h++ { // heard in hours 0 and 2, absent in hour 1
		at := r.now.Add(time.Duration(h) * time.Hour)
		if h == 1 {
			cycleOf(t, rec, at, data(neighbour))
			continue
		}
		cycleOf(t, rec, at, data(mineMAC))
	}
	r.mu.Lock()
	r.now = r.now.Add(2*time.Hour + 5*time.Minute)
	r.mu.Unlock()
	var v PresenceView
	getQ(t, r, PresencePath, "", &v)
	if len(v.Devices) != 1 {
		t.Fatalf("%+v", v)
	}
	d := v.Devices[0]
	if d.Label != "my phone" || d.State != "here" || d.Signal == "" || d.LastHeard == "" {
		t.Errorf("%+v", d)
	}
	states := map[string]int{}
	for _, c := range d.Hours {
		states[c.State]++
	}
	if states["here"] != 2 || states["away"] != 1 {
		t.Errorf("two hours here, one away: %v (%+v)", states, d.Hours)
	}
	if len(d.Hours) != presenceHours {
		t.Errorf("a fixed strip of %d hours, got %d", presenceHours, len(d.Hours))
	}
	if d.Hours[len(d.Hours)-1].State != "here" {
		t.Errorf("the strip ends with the current hour: %+v", d.Hours[len(d.Hours)-1])
	}
}

func TestAirwavesCountsByChannelAndClassWithNoDevice(t *testing.T) {
	r := devRig(t)
	rec, _, _ := r.s.recorder(r.s.h)
	for i := 0; i < 14; i++ {
		cycleOf(t, rec, r.now, data(neighbour), retryData(neighbour))
	}
	var v AirwavesView
	getQ(t, r, AirwavesPath, "", &v)
	if len(v.Channels) != 1 || v.Channels[0].Channel != 60 || v.Channels[0].Devices != 1 {
		t.Errorf("%+v", v.Channels)
	}
	if len(v.Classes) == 0 || v.Classes[0].Class != "infrastructure" || v.Classes[0].Devices != 1 || v.Classes[0].RetryPercent != 50 {
		t.Errorf("%+v", v.Classes)
	}
	if v.NearUnknown != 0 {
		t.Errorf("%d", v.NearUnknown)
	}
}

func TestTheFirstReleaseFilesGainTheNewTables(t *testing.T) {
	s, _ := openTest(t, 0)
	for _, q := range []string{`SELECT retries, channels FROM daily LIMIT 1`, `SELECT hour, cycles FROM owner_hours LIMIT 1`, `SELECT hour, cycles FROM air_hours LIMIT 1`} {
		rows, err := s.db.Query(q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		_ = rows.Close()
	}
}
