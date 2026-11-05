package netaudit

import (
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

const (
	apRouter = "192.168.1.1"
	apNAS    = "192.168.1.10"
	apCam    = "192.168.1.30"
)

func apInv(important ...string) models.DeviceInventory {
	inv := models.DeviceInventory{Gateway: apRouter, Devices: []models.NetworkDevice{
		{MAC: "aa:bb:cc:00:00:01", IP: apRouter}, {MAC: "aa:bb:cc:00:00:10", IP: apNAS, Name: "Nas"}, {MAC: "aa:bb:cc:00:00:30", IP: apCam},
	}}
	for i := range inv.Devices {
		for _, ip := range important {
			if inv.Devices[i].IP == ip {
				inv.Devices[i].Important = true
			}
		}
	}
	return inv
}

func f(device string, port int, check, sev string) Finding {
	return Finding{Device: device, Port: port, Proto: "tcp", Check: check, Severity: sev}
}

func TestAttackPaths_AnOpenPortAloneIsNeverAPath(t *testing.T) {
	got := AttackPaths(PathInput{Visible: []Finding{f(HostDevice, 22, CheckOpenPort, SeverityInfo), f(apRouter, 80, CheckOpenPort, SeverityInfo), f(apRouter, 1194, CheckInternetExposedPort, SeverityHigh)}, Inventory: apInv()})
	if len(got) != 0 {
		t.Fatalf("paths = %+v; a port with no weakness behind it is not a path", got)
	}
}

func TestAttackPaths_AWeaknessOnADeviceNobodyCaresAboutIsNotAPath(t *testing.T) {
	got := AttackPaths(PathInput{Visible: []Finding{f(apCam, 23, CheckTelnetOpen, SeverityHigh)}, Inventory: apInv()})
	if len(got) != 0 {
		t.Fatalf("paths = %+v; only this computer, the router and devices marked important are prizes", got)
	}
	got = AttackPaths(PathInput{Visible: []Finding{f(apCam, 23, CheckTelnetOpen, SeverityHigh)}, Inventory: apInv(apCam)})
	if len(got) != 1 || got[0].Prize.Kind != PrizeImportant || got[0].Prize.Where != apCam {
		t.Fatalf("paths = %+v, want one path into the important camera", got)
	}
}

func TestAttackPaths_ThisComputerWithPasswordLoginIsReachedFromAnyDeviceOnTheNetwork(t *testing.T) {
	got := AttackPaths(PathInput{Visible: []Finding{f(HostDevice, 0, CheckHostSSHPasswordLogin, SeverityLow)}, Inventory: apInv()})
	if len(got) != 1 {
		t.Fatalf("paths = %+v", got)
	}
	p := got[0]
	if p.Entry.Kind != EntryOnNetwork || p.Prize.Kind != PrizeComputer || len(p.Steps) != 1 || p.Steps[0].Check != CheckHostSSHPasswordLogin {
		t.Fatalf("path = %+v", p)
	}
	if p.Fix.Key != f(HostDevice, 0, CheckHostSSHPasswordLogin, SeverityLow).Key() || !strings.Contains(p.Fix.Text, "PasswordAuthentication") {
		t.Fatalf("fix = %+v, want the SSH password setting", p.Fix)
	}
}

func TestAttackPaths_TheInternetReachesTheRouterOnlyThroughAWeaknessOnTheRouter(t *testing.T) {
	in := PathInput{Visible: []Finding{f(apRouter, 1194, CheckInternetExposedPort, SeverityHigh), f(apRouter, 80, CheckAdminServiceOpen, SeverityMedium)}, Inventory: apInv()}
	got := AttackPaths(in)
	kinds := map[EntryKind]bool{}
	for _, p := range got {
		kinds[p.Entry.Kind] = true
		if p.Prize.Kind != PrizeRouter {
			t.Errorf("prize = %+v, want the router", p.Prize)
		}
	}
	if !kinds[EntryInternet] || !kinds[EntryOnNetwork] || len(got) != 2 {
		t.Fatalf("paths = %+v, want one from the internet and one from the network", got)
	}
	// With the weakness gone the open port is back to being a port.
	in.Visible = in.Visible[:1]
	if got := AttackPaths(in); len(got) != 0 {
		t.Fatalf("paths = %+v; an exposed port with nothing behind it is not a path", got)
	}
}

func TestAttackPaths_ADecoyTouchNamesTheNetworkEntry(t *testing.T) {
	in := PathInput{Visible: []Finding{f(HostDevice, 0, CheckHostSSHPasswordLogin, SeverityLow)}, Inventory: apInv(), Touches: []models.DecoyTouch{{Source: "192.168.1.77", Port: 2222, Count: 3}}}
	p := AttackPaths(in)[0]
	if !p.Entry.Probing || !strings.Contains(p.Entry.Text, "decoy") {
		t.Fatalf("entry = %+v, want it to say a device already probed a decoy", p.Entry)
	}
}

func TestAttackPaths_TheFixThatBreaksTheMostPathsComesFirstAndTheListHoldsThree(t *testing.T) {
	// One SSH setting on this computer is a step in the path from the network; the router has two weaknesses and two entries, so the
	// router's shared weakness breaks two paths, the computer's one, the camera's one.
	in := PathInput{Inventory: apInv(apCam, apNAS), Visible: []Finding{
		f(HostDevice, 0, CheckHostSSHPasswordLogin, SeverityLow),
		f(apRouter, 1194, CheckInternetExposedPort, SeverityHigh), f(apRouter, 80, CheckAdminServiceOpen, SeverityMedium),
		f(apCam, 23, CheckTelnetOpen, SeverityHigh), f(apNAS, 445, CheckSMBSigning, SeverityMedium),
	}}
	got := AttackPaths(in)
	if len(got) != 3 {
		t.Fatalf("paths = %d, want at most three", len(got))
	}
	if got[0].Prize.Kind != PrizeRouter || got[0].Breaks != 2 {
		t.Fatalf("first = %+v, want the router path, whose one fix breaks two paths", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i].Breaks > got[i-1].Breaks {
			t.Errorf("paths are not ranked by what one fix breaks: %d then %d", got[i-1].Breaks, got[i].Breaks)
		}
	}
}

func TestAttackPaths_AnAcceptedStepStaysInThePathMarkedAndRanksBelowALiveOne(t *testing.T) {
	in := PathInput{Inventory: apInv(apCam), Accepted: []Finding{f(apCam, 23, CheckTelnetOpen, SeverityHigh)}, Visible: []Finding{f(HostDevice, 0, CheckHostSSHPasswordLogin, SeverityLow)}}
	got := AttackPaths(in)
	if len(got) != 2 {
		t.Fatalf("paths = %+v", got)
	}
	if got[0].Prize.Kind != PrizeComputer || got[0].Steps[0].Accepted {
		t.Fatalf("first = %+v, want the live path", got[0])
	}
	if got[1].Prize.Kind != PrizeImportant || !got[1].Steps[0].Accepted || !got[1].AllAccepted {
		t.Fatalf("second = %+v, want the accepted path kept and marked", got[1])
	}
}

func TestAttackPaths_KnownFlawsCountOnlyWhenOneIsKnownToBeExploited(t *testing.T) {
	flaw := Finding{Device: apRouter, Port: 80, Proto: "tcp", Check: CheckKnownVulns, Severity: SeverityMedium, CVEs: []string{"CVE-2024-1"}}
	if got := AttackPaths(PathInput{Visible: []Finding{flaw}, Inventory: apInv()}); len(got) != 0 {
		t.Fatalf("paths = %+v; a possible flaw alone is not a way in", got)
	}
	flaw.Exploited = []string{"CVE-2024-1"}
	if got := AttackPaths(PathInput{Visible: []Finding{flaw}, Inventory: apInv()}); len(got) != 1 {
		t.Fatalf("paths = %+v; an exploited flaw is", got)
	}
}

func TestAttackPaths_TextIsFixedWordsAndNeverTheDeviceName(t *testing.T) {
	in := PathInput{Inventory: apInv(apNAS), Visible: []Finding{f(apNAS, 445, CheckSMBSigning, SeverityMedium)}}
	p := AttackPaths(in)[0]
	if p.Prize.Where != apNAS || p.Prize.Name != "Nas" || strings.Contains(PathsText([]AttackPath{p}), "Nas") {
		t.Fatalf("a device name must not reach the report text: %+v", p)
	}
}

func TestAttackPaths_DoNotChangeTheirInput(t *testing.T) {
	vis := []Finding{f(HostDevice, 0, CheckHostSSHPasswordLogin, SeverityLow), f(apRouter, 80, CheckAdminServiceOpen, SeverityMedium)}
	before := append([]Finding(nil), vis...)
	AttackPaths(PathInput{Visible: vis, Inventory: apInv()})
	for i := range vis {
		if vis[i].Key() != before[i].Key() {
			t.Fatal("the input was reordered or changed")
		}
	}
}

func TestAttackPathsConfig_IsOnByDefaultAndOffOnlyWhenSwitchedOff(t *testing.T) {
	store := &memConfig{}
	c := NewAttackPathsConfig(store)
	if !c.Enabled() {
		t.Fatal("nothing stored must mean on: the paths send nothing and cost nothing")
	}
	if err := c.Set(false); err != nil || c.Enabled() || !store.Get().AttackPathsOff {
		t.Errorf("off: err=%v enabled=%v", err, c.Enabled())
	}
	if err := c.Set(true); err != nil || !c.Enabled() || store.Get().AttackPathsOff {
		t.Errorf("on again: err=%v enabled=%v", err, c.Enabled())
	}
}

func TestOverviewOf_ListsTheAttackPathsOnlyWhileTheModuleIsOn(t *testing.T) {
	base := models.ScanBaseline{ScannedAt: t0, Findings: []Finding{f(HostDevice, 0, CheckHostSSHPasswordLogin, SeverityLow)}}
	in := OverviewInput{Now: t0, Inventory: apInv(), Baseline: base, AttackPaths: true}
	if got := OverviewOf(in).AttackPaths; len(got) != 1 {
		t.Fatalf("paths = %+v, want one", got)
	}
	in.AttackPaths = false
	if o := OverviewOf(in); o.AttackPaths == nil || len(o.AttackPaths) != 0 || o.AttackPathsOn {
		t.Fatalf("paths = %#v on = %v, want an empty list (never null) and the module reported off", o.AttackPaths, o.AttackPathsOn)
	}
	ack := models.ScanAck{Key: base.Findings[0].Key(), AcceptedAt: t0, ExpiresAt: t0.Add(24 * time.Hour)}
	in.AttackPaths, in.Acks = true, []models.ScanAck{ack}
	got := OverviewOf(in).AttackPaths
	if len(got) != 1 || !got[0].AllAccepted {
		t.Fatalf("paths = %+v, want the accepted finding kept and marked", got)
	}
}

type fixedTouches struct{ list []models.DecoyTouch }

func (f fixedTouches) Read() ([]models.DecoyTouch, error) { return f.list, nil }

func TestRunReporter_AddsTheAttackPathsToTheReportOnlyWhileTheModuleIsOn(t *testing.T) {
	on := true
	r := NewRunReporter(nil).WithAttackPaths(func() bool { return on })
	shown := shownScan{result: Result{Findings: []Finding{f(HostDevice, 0, CheckHostSSHPasswordLogin, SeverityLow)}}}
	note := r.attackPathsNote(shown, apInv())
	if !strings.Contains(note, "Ways in that matter") || !strings.Contains(note, "this computer") {
		t.Fatalf("note = %q", note)
	}
	on = false
	if note := r.attackPathsNote(shown, apInv()); note != "" {
		t.Fatalf("note = %q while the module is off", note)
	}
	if note := NewRunReporter(nil).attackPathsNote(shown, apInv()); note != "" {
		t.Fatalf("a reporter built without the module must add nothing, got %q", note)
	}
}
