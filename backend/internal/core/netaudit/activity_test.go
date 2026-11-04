package netaudit

import (
	"testing"
	"time"

	"llm-proxy/models"
)

var activityNow = time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)

func activityDevice(mac, ip, name string, firstSeen time.Time) models.NetworkDevice {
	return models.NetworkDevice{MAC: mac, IP: ip, Name: name, FirstSeen: firstSeen, LastSeen: firstSeen}
}

func kindsOf(rows []ActivityRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Kind)
	}
	return out
}

func TestActivity_ANewDeviceAndAnAddressChangeAreRowsNewestFirst(t *testing.T) {
	inv := models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{
		activityDevice("aa:bb:cc:00:00:01", "192.168.1.5", "Printer", activityNow.Add(-3*time.Hour)),
	}, Changes: []models.AddressChange{{IP: "192.168.1.9", From: "aa:bb:cc:00:00:02", To: "aa:bb:cc:00:00:03", At: activityNow.Add(-time.Hour)}}}
	got := ActivityOf(ActivityInput{Now: activityNow, Inventory: inv}, 20)
	if want := []string{ActivityAddressChanged, ActivityNewDevice}; len(got.Rows) != 2 || got.Rows[0].Kind != want[0] || got.Rows[1].Kind != want[1] {
		t.Fatalf("kinds = %v, want %v", kindsOf(got.Rows), want)
	}
	if r := got.Rows[1]; r.Group != ActivityGroupDevices || r.Name != "Printer" || r.IP != "192.168.1.5" {
		t.Errorf("new device row = %+v", r)
	}
}

func TestActivity_AQuietNetworkIsNoRowsNotAnEmptyShapeOfNothing(t *testing.T) {
	got := ActivityOf(ActivityInput{Now: activityNow, Inventory: models.DeviceInventory{Baselined: true}}, 20)
	if got.Rows == nil || len(got.Rows) != 0 {
		t.Fatalf("a quiet network is an empty list that encodes as [], got %#v", got.Rows)
	}
}

func TestActivity_ADeviceFromTheFirstBaselineIsNotNewsUntilTheBaselineExists(t *testing.T) {
	inv := models.DeviceInventory{Baselined: false, Devices: []models.NetworkDevice{activityDevice("aa:bb:cc:00:00:01", "192.168.1.5", "", activityNow.Add(-time.Minute))}}
	if rows := ActivityOf(ActivityInput{Now: activityNow, Inventory: inv}, 20).Rows; len(rows) != 0 {
		t.Errorf("before the first baseline nothing is a change, got %+v", rows)
	}
}

func TestActivity_AlertsKeepTheirCountAndTextAndArriveAsAlertRows(t *testing.T) {
	alerts := []models.NetworkAlert{{ID: 4, Kind: models.NetworkAlertDecoy, Message: "A decoy service was touched by 192.168.1.77", At: activityNow.Add(-30 * time.Minute), Count: 3}}
	rows := ActivityOf(ActivityInput{Now: activityNow, Alerts: alerts}, 20).Rows
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	r := rows[0]
	if r.Kind != ActivityAlert || r.Group != ActivityGroupAlerts || r.AlertKind != string(models.NetworkAlertDecoy) || r.Count != 3 || r.Text == "" {
		t.Errorf("alert row = %+v", r)
	}
}

func TestActivity_PortChangesAreOneRowPerDayAndOnlyWhenSomethingChanged(t *testing.T) {
	series := []models.NetworkDay{
		{Date: "2026-11-02", PortsOpened: 0, PortsClosed: 0},
		{Date: "2026-11-03", PortsOpened: 2, PortsClosed: 1},
	}
	rows := ActivityOf(ActivityInput{Now: activityNow, Series: series}, 20).Rows
	if len(rows) != 1 || rows[0].Kind != ActivityPortsChanged || rows[0].Group != ActivityGroupPorts || rows[0].Opened != 2 || rows[0].Closed != 1 || !rows[0].Day {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].At.After(activityNow) {
		t.Errorf("a day row is never in the future: %v", rows[0].At)
	}
}

func TestActivity_TodaysPortRowIsNotInTheFuture(t *testing.T) {
	rows := ActivityOf(ActivityInput{Now: activityNow, Series: []models.NetworkDay{{Date: "2026-11-04", PortsOpened: 1}}}, 20).Rows
	if len(rows) != 1 || rows[0].At.After(activityNow) {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestActivity_TheLastAuditIsARowOnlyWhenItFoundSomethingNewOrClosedSomething(t *testing.T) {
	base := models.ScanBaseline{ScannedAt: activityNow.Add(-2 * time.Hour), Findings: make([]models.ScanFinding, 5), NewKeys: []string{"a", "b"}, Resolved: make([]models.ScanFinding, 1)}
	rows := ActivityOf(ActivityInput{Now: activityNow, Baseline: base}, 20).Rows
	if len(rows) != 1 || rows[0].Kind != ActivityAudit || rows[0].Group != ActivityGroupFindings || rows[0].Opened != 2 || rows[0].Closed != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	quiet := models.ScanBaseline{ScannedAt: activityNow.Add(-2 * time.Hour), Findings: make([]models.ScanFinding, 5)}
	if rows := ActivityOf(ActivityInput{Now: activityNow, Baseline: quiet}, 20).Rows; len(rows) != 0 {
		t.Errorf("an audit that changed nothing is not a row: %+v", rows)
	}
}

func TestActivity_ANewDeviceShortlyBeforeADecoyTouchIsFlaggedAsPossiblyRelated(t *testing.T) {
	inv := models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{activityDevice("aa:bb:cc:00:00:09", "192.168.1.77", "", activityNow.Add(-50*time.Minute))}}
	alerts := []models.NetworkAlert{{ID: 1, Kind: models.NetworkAlertDecoy, Message: "touched", At: activityNow.Add(-20 * time.Minute), Count: 1}}
	rows := ActivityOf(ActivityInput{Now: activityNow, Inventory: inv, Alerts: alerts}, 20).Rows
	var found *ActivityRow
	for i := range rows {
		if rows[i].Kind == ActivityRelated {
			found = &rows[i]
		}
	}
	if found == nil {
		t.Fatalf("want a related row, got kinds %v", kindsOf(rows))
	}
	if found.IP != "192.168.1.77" || found.Group != ActivityGroupAlerts || !found.At.Equal(alerts[0].At) {
		t.Errorf("related row = %+v", *found)
	}
}

func TestActivity_NoRelatedRowWhenTheDeviceIsOlderOrArrivedAfterOrThereWasNoDecoy(t *testing.T) {
	cases := map[string]struct {
		first time.Time
		alert models.NetworkAlertKind
	}{
		"device older than an hour before": {activityNow.Add(-5 * time.Hour), models.NetworkAlertDecoy},
		"device arrived after the touch":   {activityNow.Add(-5 * time.Minute), models.NetworkAlertDecoy},
		"the alert is not a decoy touch":   {activityNow.Add(-50 * time.Minute), models.NetworkAlertPresence},
	}
	for name, c := range cases {
		inv := models.DeviceInventory{Baselined: true, Devices: []models.NetworkDevice{activityDevice("aa:bb:cc:00:00:09", "192.168.1.77", "", c.first)}}
		alerts := []models.NetworkAlert{{ID: 1, Kind: c.alert, Message: "x", At: activityNow.Add(-20 * time.Minute), Count: 1}}
		for _, r := range ActivityOf(ActivityInput{Now: activityNow, Inventory: inv, Alerts: alerts}, 20).Rows {
			if r.Kind == ActivityRelated {
				t.Errorf("%s: unexpected related row %+v", name, r)
			}
		}
	}
}

func TestActivity_TheLimitKeepsTheNewestAndAnUnreasonableOneIsBounded(t *testing.T) {
	var devices []models.NetworkDevice
	for i := 0; i < 30; i++ {
		devices = append(devices, activityDevice("aa:bb:cc:00:00:"+string(rune('a'+i%6))+"0", "192.168.1.1", "", activityNow.Add(-time.Duration(i+1)*time.Hour)))
	}
	in := ActivityInput{Now: activityNow, Inventory: models.DeviceInventory{Baselined: true, Devices: devices}}
	got := ActivityOf(in, 5).Rows
	if len(got) != 5 || !got[0].At.Equal(activityNow.Add(-time.Hour)) {
		t.Fatalf("got %d rows, newest %v", len(got), got[0].At)
	}
	if n := len(ActivityOf(in, 100000).Rows); n > MaxActivityRows {
		t.Errorf("rows = %d, above the bound %d", n, MaxActivityRows)
	}
	if n := len(ActivityOf(in, 0).Rows); n != DefaultActivityRows {
		t.Errorf("no limit means the default %d, got %d", DefaultActivityRows, n)
	}
}
