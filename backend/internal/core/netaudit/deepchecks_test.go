package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeDeepScanner struct {
	res    Result
	err    error
	device string
	ports  []int
	calls  int
}

func (f *fakeDeepScanner) DeepScan(_ context.Context, device string, ports []int) (Result, error) {
	f.calls++
	f.device, f.ports = device, ports
	return f.res, f.err
}

type memDeepStore struct {
	items []models.DeepCheck
	err   error
}

func (m *memDeepStore) ReadDeepChecks(string) ([]models.DeepCheck, error) {
	return slices.Clone(m.items), m.err
}
func (m *memDeepStore) WriteDeepChecks(_ string, items []models.DeepCheck) error {
	m.items = slices.Clone(items)
	return nil
}

type fixedBaseline struct {
	b   models.ScanBaseline
	err error
}

func (f fixedBaseline) ReadBaseline(string, string) (models.ScanBaseline, error) { return f.b, f.err }

func openPortFinding(device string, port int, proto string) models.ScanFinding {
	return models.ScanFinding{Device: device, Port: port, Proto: proto, Check: CheckOpenPort, Severity: SeverityInfo}
}

var deepNow = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func newDeepChecks(scanner DeepScanner, base models.ScanBaseline, store DeepStore) *DeepChecks {
	d := NewDeepChecks(scanner, fixedBaseline{b: base}, store)
	d.now = func() time.Time { return deepNow }
	return d
}

func baselineWith(findings ...models.ScanFinding) models.ScanBaseline {
	return models.ScanBaseline{ScannedAt: deepNow.Add(-time.Hour), Findings: findings}
}

func TestDeepChecks_RunLooksAtTheOpenTCPPortsTheLastScanFound(t *testing.T) {
	scanner := &fakeDeepScanner{res: Result{Engine: "nmap", Hosts: []Host{{Addr: netip.MustParseAddr("10.0.0.5"), Up: true, Ports: []Port{
		{Number: 22, Proto: "tcp", State: "open", Scripts: []Script{{ID: "ssh-auth-methods", Output: sshPassword}}},
		{Number: 443, Proto: "tcp", State: "open"},
	}}}, Findings: []Finding{{Device: "10.0.0.5", Port: 22, Proto: "tcp", Check: CheckSSHPasswordLogin, Severity: SeverityMedium, Evidence: "e"}}}}
	store := &memDeepStore{}
	base := baselineWith(openPortFinding("10.0.0.5", 443, "tcp"), openPortFinding("10.0.0.5", 22, "tcp"), openPortFinding("10.0.0.5", 53, "udp"), openPortFinding("10.0.0.6", 80, "tcp"),
		models.ScanFinding{Device: "10.0.0.5", Port: 22, Proto: "tcp", Check: CheckSSHWeakAlgorithms, Severity: SeverityLow})
	got, err := newDeepChecks(scanner, base, store).Run(context.Background(), "ws", "audit", "10.0.0.5")
	if err != nil {
		t.Fatal(err)
	}
	if scanner.device != "10.0.0.5" || !slices.Equal(scanner.ports, []int{22, 443}) {
		t.Errorf("only this device's open TCP ports, once each, from the stored scan: %s %v", scanner.device, scanner.ports)
	}
	want := []models.DeepPortResult{{Port: 22, SSH: true}, {Port: 443}}
	if !got.At.Equal(deepNow) || !slices.Equal(got.Ports, want) {
		t.Errorf("per port, which deep script produced output: %+v", got)
	}
	if len(got.Findings) != 1 || got.Findings[0].Check != CheckSSHPasswordLogin {
		t.Errorf("findings = %+v", got.Findings)
	}
	if len(store.items) != 1 || store.items[0].Device != "10.0.0.5" {
		t.Errorf("the result is stored: %+v", store.items)
	}
}

func TestDeepChecks_RefusesADeviceTheLastScanCannotVouchFor(t *testing.T) {
	scanner := &fakeDeepScanner{}
	run := func(base models.ScanBaseline, device string) error {
		_, err := newDeepChecks(scanner, base, &memDeepStore{}).Run(context.Background(), "ws", "audit", device)
		return err
	}
	if err := run(baselineWith(openPortFinding("10.0.0.6", 80, "tcp")), "10.0.0.5"); !errors.Is(err, ErrNoPorts) {
		t.Errorf("a device the scan did not see: %v", err)
	}
	unscanned := baselineWith(openPortFinding("10.0.0.5", 22, "tcp"))
	unscanned.Unscanned = []string{"10.0.0.5"}
	if err := run(unscanned, "10.0.0.5"); !errors.Is(err, ErrNoPorts) {
		t.Errorf("the scan ran out of time on it, so its ports are unknown: %v", err)
	}
	if err := run(models.ScanBaseline{}, "10.0.0.5"); !errors.Is(err, ErrNoPorts) {
		t.Errorf("no scan yet: %v", err)
	}
	for _, device := range []string{"printer.local", "10.0.0.0/24", ""} {
		if err := run(baselineWith(openPortFinding("10.0.0.5", 22, "tcp")), device); !errors.Is(err, ErrBadDevice) {
			t.Errorf("%q: %v", device, err)
		}
	}
	if scanner.calls != 0 {
		t.Errorf("nothing refused may reach the scanner (%d calls)", scanner.calls)
	}
}

func TestDeepChecks_ManyOpenPortsAreCappedNotRefused(t *testing.T) {
	var findings []models.ScanFinding
	for p := 1000; p < 1000+MaxDeepPorts+5; p++ {
		findings = append(findings, openPortFinding("10.0.0.5", p, "tcp"))
	}
	scanner := &fakeDeepScanner{}
	if _, err := newDeepChecks(scanner, baselineWith(findings...), &memDeepStore{}).Run(context.Background(), "ws", "a", "10.0.0.5"); err != nil {
		t.Fatal(err)
	}
	if len(scanner.ports) != MaxDeepPorts || scanner.ports[0] != 1000 {
		t.Errorf("the lowest %d ports are checked: %v", MaxDeepPorts, scanner.ports)
	}
}

// The cool-down comes from the stored time of the last check, so it survives a restart and has no second source of truth.
func TestDeepChecks_ADeviceIsCheckedOnceInACoolDown(t *testing.T) {
	store := &memDeepStore{items: []models.DeepCheck{{Device: "10.0.0.5", At: deepNow.Add(-3 * time.Minute)}}}
	scanner := &fakeDeepScanner{}
	d := newDeepChecks(scanner, baselineWith(openPortFinding("10.0.0.5", 22, "tcp")), store)
	_, err := d.Run(context.Background(), "ws", "a", "10.0.0.5")
	var cool *CooldownError
	if !errors.As(err, &cool) || cool.Remaining != DeepCooldown-3*time.Minute || scanner.calls != 0 {
		t.Fatalf("err = %v (calls %d), want a cool-down error with %v left", err, scanner.calls, DeepCooldown-3*time.Minute)
	}
	// Another device is not held up by it, and the same one is free once the time has passed.
	if _, err := newDeepChecks(scanner, baselineWith(openPortFinding("10.0.0.6", 22, "tcp")), store).Run(context.Background(), "ws", "a", "10.0.0.6"); err != nil {
		t.Errorf("another device: %v", err)
	}
	store.items = []models.DeepCheck{{Device: "10.0.0.5", At: deepNow.Add(-DeepCooldown - time.Second)}}
	if _, err := d.Run(context.Background(), "ws", "a", "10.0.0.5"); err != nil {
		t.Errorf("after the cool-down: %v", err)
	}
}

// A check that found no service to judge is still a result: it is stored with its time, so the report can say so.
func TestDeepChecks_ANoAnswerIsStoredAndFailureIsNot(t *testing.T) {
	store := &memDeepStore{}
	scanner := &fakeDeepScanner{res: Result{Hosts: []Host{{Addr: netip.MustParseAddr("10.0.0.5"), Up: true, Ports: []Port{{Number: 3389, Proto: "tcp", State: "open"}}}}}}
	got, err := newDeepChecks(scanner, baselineWith(openPortFinding("10.0.0.5", 3389, "tcp")), store).Run(context.Background(), "ws", "a", "10.0.0.5")
	if err != nil || len(got.Findings) != 0 || got.Ports[0].TLS || got.Ports[0].SSH || len(store.items) != 1 {
		t.Fatalf("nothing answered: %+v %v stored %d", got, err, len(store.items))
	}

	failing := &memDeepStore{}
	bad := &fakeDeepScanner{err: ErrNmapRequired}
	if _, err := newDeepChecks(bad, baselineWith(openPortFinding("10.0.0.5", 22, "tcp")), failing).Run(context.Background(), "ws", "a", "10.0.0.5"); !errors.Is(err, ErrNmapRequired) || len(failing.items) != 0 {
		t.Errorf("a failed run is not a result (and does not start a cool-down): %v, stored %d", err, len(failing.items))
	}
}

func TestDeepChecks_StoreKeepsOneResultPerDeviceAndIsBounded(t *testing.T) {
	store := &memDeepStore{}
	scanner := &fakeDeepScanner{}
	d := newDeepChecks(scanner, baselineWith(openPortFinding("10.0.0.5", 22, "tcp")), store)
	d.now = func() time.Time { return deepNow }
	for i := 0; i < 2; i++ {
		d.now = func() time.Time { return deepNow.Add(time.Duration(i) * (DeepCooldown + time.Minute)) }
		if _, err := d.Run(context.Background(), "ws", "a", "10.0.0.5"); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.items) != 1 || !store.items[0].At.After(deepNow) {
		t.Errorf("the newest result replaces the older one for the device: %+v", store.items)
	}
	many := make([]models.DeepCheck, maxDeepDevices)
	for i := range many {
		many[i] = models.DeepCheck{Device: netip.AddrFrom4([4]byte{10, 1, byte(i / 250), byte(i % 250)}).String(), At: deepNow.Add(-time.Duration(i+1) * time.Hour)}
	}
	store.items = many
	d.now = func() time.Time { return deepNow.Add(24 * time.Hour) }
	if _, err := d.Run(context.Background(), "ws", "a", "10.0.0.5"); err != nil {
		t.Fatal(err)
	}
	if len(store.items) != maxDeepDevices {
		t.Errorf("the store is capped at %d devices, got %d", maxDeepDevices, len(store.items))
	}
}
