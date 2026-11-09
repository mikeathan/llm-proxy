package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

// fakeAsker answers as told and records what it was asked. Like the Network stack's sensor it reports every pair it asked (both questions
// of each device) and the ones that answered; err stands for a sensor that could not look at all.
type fakeAsker struct {
	answers []UDPPair
	asked   []netip.Addr
	err     error
}

func (f *fakeAsker) Look(_ context.Context, addrs []netip.Addr) (UDPLooked, error) {
	f.asked = addrs
	if f.err != nil {
		return UDPLooked{}, f.err
	}
	out := UDPLooked{Answered: f.answers}
	for _, a := range addrs {
		out.Asked = append(out.Asked, UDPPair{Addr: a, Port: 161}, UDPPair{Addr: a, Port: 1900})
	}
	return out, nil
}

func answer(addr string, port int) UDPPair {
	return UDPPair{Addr: netip.MustParseAddr(addr), Port: port}
}

func always() bool { return true }

func udpKeys(store *memBaselines) map[string]Finding {
	out := map[string]Finding{}
	for _, f := range store.data["ws/nightly"].Findings {
		if f.Check == CheckUDPService {
			out[f.Key()] = f
		}
	}
	return out
}

func TestUDPChecks_AnAnswerBecomesTheRuleFindingWithFixedWords(t *testing.T) {
	asker := &fakeAsker{answers: []UDPPair{answer("10.0.0.7", 161), answer("10.0.0.7", 1900)}}
	res := (&UDPChecks{Asker: asker}).Findings(context.Background(), []Host{hostOf("10.0.0.7", true, false, 80), hostOf("fe80::1", true, false)})
	if len(asker.asked) != 1 || asker.asked[0].String() != "10.0.0.7" {
		t.Fatalf("asked %v: only IPv4 hosts that are up are asked", asker.asked)
	}
	if len(res.Findings) != 2 {
		t.Fatalf("findings = %+v", res.Findings)
	}
	f := res.Findings[0]
	if f.Device != "10.0.0.7" || f.Port != 161 || f.Proto != "udp" || f.Check != CheckUDPService || f.Service != "SNMP" || f.Severity != SeverityMedium {
		t.Errorf("snmp finding = %+v", f)
	}
	if strings.ContainsAny(f.Evidence, "0123456789") {
		t.Errorf("the evidence carries a date or number that would change every night: %q", f.Evidence)
	}
	if res.Findings[1].Service != "UPnP" || res.Findings[1].Severity != SeverityLow {
		t.Errorf("upnp finding = %+v", res.Findings[1])
	}
}

// Acks, fixes and the baseline diff are keyed on a finding's fields. The words of a nightly UDP finding are therefore pinned exactly, so the
// day the question moves into the Network stack (ADR-0038) the same answer must produce the same finding, byte for byte, or every
// finding reads as resolved overnight and the operator's acknowledgements are lost.
func TestUDPChecks_TheFindingsOfAnAnswerAreByteIdentical(t *testing.T) {
	asker := &fakeAsker{answers: []UDPPair{answer("10.0.0.7", 161), answer("10.0.0.7", 1900)}}
	got := (&UDPChecks{Asker: asker}).Findings(context.Background(), []Host{hostOf("10.0.0.7", true, false, 80)}).Findings
	want := []Finding{
		{Device: "10.0.0.7", Port: 161, Proto: "udp", Service: "SNMP", Check: CheckUDPService, Severity: SeverityMedium,
			Evidence: "SNMP answered a check from this computer. SNMP often answers to a default password (\"public\") and then gives away how the device is set up"},
		{Device: "10.0.0.7", Port: 1900, Proto: "udp", Service: "UPnP", Check: CheckUDPService, Severity: SeverityLow,
			Evidence: "UPnP answered a check from this computer. UPnP lets other devices on the network ask this one to open ports to the internet"},
	}
	if len(got) != len(want) {
		t.Fatalf("findings = %+v", got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("finding %d changed:\n got  %+v\n want %+v", i, got[i], want[i])
		}
		if got[i].Key() != want[i].Key() {
			t.Errorf("finding %d key changed: %q", i, got[i].Key())
		}
	}
}

func TestUDPChecks_DownHostsAndUnscannedHostsAreNotAsked(t *testing.T) {
	asker := &fakeAsker{}
	(&UDPChecks{Asker: asker}).Findings(context.Background(), []Host{hostOf("10.0.0.7", false, false), hostOf("10.0.0.8", true, true), hostOf("10.0.0.9", true, false)})
	if len(asker.asked) != 1 || asker.asked[0].String() != "10.0.0.9" {
		t.Fatalf("asked %v", asker.asked)
	}
}

func TestUDPChecks_SilenceKeepsAnEarlierFindingAndAnAnswerRefreshesIt(t *testing.T) {
	host := hostOf("10.0.0.7", true, false, 80)
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	reporter(store, reportTime.Add(-48*time.Hour)).WithUDPChecks(always, &UDPChecks{Asker: &fakeAsker{answers: []UDPPair{answer("10.0.0.7", 161)}}}).Augment(scannedHosts(host), "ws", "nightly", "")
	if len(udpKeys(store)) != 1 {
		t.Fatalf("first night: %v", udpKeys(store))
	}
	// A night with no answer says nothing about whether the service is gone: the finding stays.
	reporter(store, reportTime.Add(-24*time.Hour)).WithUDPChecks(always, &UDPChecks{Asker: &fakeAsker{}}).Augment(scannedHosts(host), "ws", "nightly", "")
	if len(udpKeys(store)) != 1 {
		t.Fatalf("a silent night resolved the finding: %v", udpKeys(store))
	}
	// Switched off: nothing is asked and the finding is not carried by this check.
	off := &fakeAsker{}
	reporter(store, reportTime).WithUDPChecks(func() bool { return false }, &UDPChecks{Asker: off}).Augment(scannedHosts(host), "ws", "nightly", "")
	if off.asked != nil {
		t.Fatal("a switched-off check asked a device")
	}
	if len(udpKeys(store)) != 0 {
		t.Fatalf("a switched-off check kept a finding it could not renew: %v", udpKeys(store))
	}
}

// A sensor that could not look at all (the stack is not running, or timed out) says nothing about any service, so the earlier findings of every
// device this scan found stay, exactly as on a night when each device was silent.
func TestUDPChecks_ASensorThatCannotLookKeepsEveryEarlierFinding(t *testing.T) {
	host := hostOf("10.0.0.7", true, false, 80)
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	reporter(store, reportTime.Add(-48*time.Hour)).WithUDPChecks(always, &UDPChecks{Asker: &fakeAsker{answers: []UDPPair{answer("10.0.0.7", 161), answer("10.0.0.7", 1900)}}}).Augment(scannedHosts(host), "ws", "nightly", "")
	if len(udpKeys(store)) != 2 {
		t.Fatalf("first night: %v", udpKeys(store))
	}
	broken := &fakeAsker{err: errors.New("the stack is not running")}
	reporter(store, reportTime.Add(-24*time.Hour)).WithUDPChecks(always, &UDPChecks{Asker: broken}).Augment(scannedHosts(host), "ws", "nightly", "")
	if len(udpKeys(store)) != 2 {
		t.Fatalf("a night the sensor could not look resolved the findings: %v", udpKeys(store))
	}
	res := (&UDPChecks{Asker: broken}).Findings(context.Background(), []Host{host})
	if len(res.Findings) != 0 {
		t.Errorf("a sensor that did not look found something: %+v", res.Findings)
	}
}

func TestUDPChecks_NotCarriedForADeviceTheScanDidNotLookAt(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	reporter(store, reportTime.Add(-24*time.Hour)).WithUDPChecks(always, &UDPChecks{Asker: &fakeAsker{answers: []UDPPair{answer("10.0.0.7", 161)}}}).Augment(scannedHosts(hostOf("10.0.0.7", true, false, 80)), "ws", "nightly", "")
	reporter(store, reportTime).WithUDPChecks(always, &UDPChecks{Asker: &fakeAsker{}}).Augment(scannedHosts(hostOf("10.0.0.8", true, false, 80)), "ws", "nightly", "")
	if _, ok := udpKeys(store)["10.0.0.7|udp|161|"+CheckUDPService]; !ok {
		t.Fatal("a device this scan did not look at must keep its earlier findings untouched")
	}
}

func TestUDPChecks_ANightlyAnswerAndAHelperResultForTheSamePortAreOneFinding(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	helper := models.NetworkDevice{IP: "10.0.0.7", UDPScannedAt: reportTime.Add(-72 * time.Hour), UDPPorts: []models.ProbeUDPPort{{Port: 161, State: "open"}}}
	r := reporter(store, reportTime).WithUDPChecks(always, &UDPChecks{Asker: &fakeAsker{answers: []UDPPair{answer("10.0.0.7", 161)}}})
	result := Result{Findings: ProbeFindings([]models.NetworkDevice{helper})}
	udp := r.addUDPChecks(context.Background(), &result, []Host{hostOf("10.0.0.7", true, false, 80)})
	n := 0
	for _, f := range result.Findings {
		if f.Check == CheckUDPService && f.Port == 161 {
			n++
			if strings.Contains(f.Evidence, "UDP scan on") {
				t.Errorf("the older helper wording won: %q", f.Evidence)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d findings for one port", n)
	}
	// A device the helper found and tonight did not answer: the helper finding is already in the result, so it is not carried twice.
	silent := (&UDPChecks{Asker: &fakeAsker{}}).Findings(context.Background(), []Host{hostOf("10.0.0.7", true, false, 80)})
	result2 := Result{Findings: ProbeFindings([]models.NetworkDevice{helper})}
	silent = silent.seeing(result2.Findings)
	if got := silent.Carry(result2.Findings); len(got) != 0 {
		t.Fatalf("carried a finding the result already has: %+v", got)
	}
	_ = udp
}
