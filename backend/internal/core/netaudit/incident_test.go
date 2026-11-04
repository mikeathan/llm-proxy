package netaudit

import (
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestIncidentFindings_ADecoyTouchJoinedByAnotherSignalIsOneIncident(t *testing.T) {
	touch := Finding{Device: "10.0.0.5", Port: 2222, Proto: "tcp", Check: CheckDecoyTouched, Severity: SeverityHigh}
	port := func(n int) Finding {
		return Finding{Device: "10.0.0.5", Port: n, Proto: "tcp", Check: CheckOpenPort, Severity: SeverityInfo}
	}
	tests := []struct {
		name     string
		current  []Finding
		previous []Finding
		want     string // a phrase of the evidence, or "" for no incident
	}{
		{"a touch alone stays what it is", []Finding{touch}, nil, ""},
		{"a new device", []Finding{touch, {Device: "10.0.0.5", Check: CheckNewDevice}}, nil, "new on the network"},
		{"a class mismatch", []Finding{touch, {Device: "10.0.0.5", Check: CheckClassMismatch}}, nil, "services of another kind"},
		{"an address taken over", []Finding{touch, {Device: "10.0.0.5", Check: CheckMACChanged}}, nil, "a different device"},
		{"a port opened since the last audit", []Finding{touch, port(22), port(8080)}, []Finding{port(22)}, "opened a port"},
		{"the same ports as before", []Finding{touch, port(22)}, []Finding{port(22)}, ""},
		{"a device never scanned before is not a port that opened", []Finding{touch, port(22)}, nil, ""},
		{"another device's signal does not join", []Finding{touch, {Device: "10.0.0.6", Check: CheckNewDevice}}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IncidentFindings(tt.current, tt.previous)
			if tt.want == "" {
				if len(got) != 0 {
					t.Fatalf("incidents = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 || got[0].Check != CheckDeviceIncident || got[0].Severity != SeverityHigh || got[0].Device != "10.0.0.5" {
				t.Fatalf("incidents = %+v, want one high device_incident on 10.0.0.5", got)
			}
			if !strings.Contains(got[0].Evidence, tt.want) {
				t.Errorf("evidence %q lacks %q", got[0].Evidence, tt.want)
			}
		})
	}
}

func TestIncidentFindings_EverySignalThatJoinedIsNamed(t *testing.T) {
	got := IncidentFindings([]Finding{
		{Device: "10.0.0.5", Port: 2222, Proto: "tcp", Check: CheckDecoyTouched},
		{Device: "10.0.0.5", Check: CheckNewDevice}, {Device: "10.0.0.5", Check: CheckClassMismatch},
	}, nil)
	if len(got) != 1 || !strings.Contains(got[0].Evidence, "new on the network") || !strings.Contains(got[0].Evidence, "another kind") {
		t.Fatalf("incidents = %+v", got)
	}
}

type touchesReader []models.DecoyTouch

func (t touchesReader) Read() ([]models.DecoyTouch, error) { return t, nil }

func TestRunReporter_ADecoyTouchFromADeviceThatOpenedAPortIsAnIncident(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	reporter(store, reportTime).Augment(scanned(finding("10.0.0.1", 22, "")), "ws", "nightly", "")
	touch := touchesReader{{Source: "10.0.0.1", Port: 2222, Count: 1, Last: reportTime}}
	r := reporter(store, reportTime.Add(24*time.Hour)).WithDecoys(touch)
	got := r.Augment(scanned(finding("10.0.0.1", 22, ""), finding("10.0.0.1", 8080, "")), "ws", "nightly", "")
	if !strings.Contains(got, "opened a port") {
		t.Fatalf("the incident must be in the report:\n%s", got)
	}
}

func TestDecoyAlertMessage_SaysWhenTheVisitorIsNewOnTheNetwork(t *testing.T) {
	now := reportTime
	inv := models.DeviceInventory{Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: "10.0.0.5", FirstSeen: now.Add(-time.Hour), LastSeen: now},
		{MAC: "aa:bb:cc:00:00:02", IP: "10.0.0.6", FirstSeen: now.Add(-72 * time.Hour), LastSeen: now},
		{MAC: "aa:bb:cc:00:00:03", IP: "10.0.0.7", FirstSeen: now.Add(-time.Hour), LastSeen: now, Approved: true},
	}}
	if got := DecoyAlertMessage([]string{"10.0.0.5"}, inv, now); !strings.Contains(got, DecoyNewDevicePhrase) {
		t.Errorf("a visitor first seen an hour ago: %q", got)
	}
	for _, src := range []string{"10.0.0.6", "10.0.0.7", "10.0.0.9"} {
		if got := DecoyAlertMessage([]string{src}, inv, now); got != DecoyMessage([]string{src}) {
			t.Errorf("%s is not new (older, approved or unknown): %q", src, got)
		}
	}
}
