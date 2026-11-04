package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/hostwatch"
	"llm-proxy/models"
)

type fakeHost struct {
	listeners    []hostwatch.Listener
	lerr         error
	files        []hostwatch.FileState
	hard         hostwatch.Hardening
	patches      hostwatch.Patches
	docker       bool
	integrity    hostwatch.IntegrityResult
	integrityRan bool  // whether the integrity pass was asked for
	integrityErr error // the state of its context when it was
	packages     map[string]string
	packagesErr  error
	privilege    hostwatch.Privilege
}

func (f *fakeHost) Listeners() ([]hostwatch.Listener, error) { return f.listeners, f.lerr }
func (f *fakeHost) Files() []hostwatch.FileState             { return f.files }
func (f *fakeHost) Hardening() hostwatch.Hardening           { return f.hard }
func (f *fakeHost) Patches() hostwatch.Patches               { return f.patches }
func (f *fakeHost) DockerBridge() bool                       { return f.docker }
func (f *fakeHost) Integrity(ctx context.Context) hostwatch.IntegrityResult {
	f.integrityRan, f.integrityErr = true, ctx.Err()
	return f.integrity
}
func (f *fakeHost) Installed() (map[string]string, error) { return f.packages, f.packagesErr }
func (f *fakeHost) Privilege() hostwatch.Privilege        { return f.privilege }

type memHostStore struct {
	w      models.HostWatch
	writes int
	werr   error
}

func (m *memHostStore) Read() (models.HostWatch, error) { return m.w, nil }
func (m *memHostStore) Write(w models.HostWatch) error {
	m.writes++
	if m.werr != nil {
		return m.werr
	}
	m.w = w
	return nil
}

func newWatcher(src *fakeHost, store *memHostStore) *HostWatcher {
	w := NewHostWatcher(src, store)
	w.now = func() time.Time { return time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC) }
	return w
}

var ssh = hostwatch.Listener{Proto: "tcp", Port: 22}

func TestHostWatch_FirstLookBuildsABaselineAndReportsNothing(t *testing.T) {
	src := &fakeHost{listeners: []hostwatch.Listener{ssh, {Proto: "udp", Port: 5353}}, files: []hostwatch.FileState{{ID: "sudoers", Hash: "aa"}}}
	store := &memHostStore{}
	look, err := newWatcher(src, store).Look(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(look.Findings) != 0 {
		t.Fatalf("a first look must report nothing, got %+v", look.Findings)
	}
	if len(store.w.Listeners) != 2 || len(store.w.Files) != 1 || store.w.ListenersAt.IsZero() {
		t.Errorf("baseline not stored: %+v", store.w)
	}
}

func TestHostWatch_ANewListenerIsOneFindingAndAnUnchangedOneIsNone(t *testing.T) {
	src := &fakeHost{listeners: []hostwatch.Listener{ssh}}
	store := &memHostStore{}
	w := newWatcher(src, store)
	if _, err := w.Look(t.Context()); err != nil {
		t.Fatal(err)
	}
	src.listeners = []hostwatch.Listener{ssh, {Proto: "tcp", Port: 8080}}
	look, err := w.Look(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(look.Findings) != 1 {
		t.Fatalf("want exactly one finding, got %+v", look.Findings)
	}
	f := look.Findings[0]
	if f.Device != HostDevice || f.Check != CheckNewListener || f.Port != 8080 || f.Proto != "tcp" || f.Severity != SeverityMedium {
		t.Errorf("finding = %+v", f)
	}
	again, _ := w.Look(t.Context())
	if len(again.Findings) != 1 || again.Findings[0].Key() != f.Key() {
		t.Errorf("an open listener stays one finding with the same key until it is accepted or gone: %+v", again.Findings)
	}
	src.listeners = []hostwatch.Listener{ssh}
	if gone, _ := w.Look(t.Context()); len(gone.Findings) != 0 {
		t.Errorf("a listener that closed is not a finding: %+v", gone.Findings)
	}
}

func TestHostWatch_ANewListenerNamesDockerOnlyWhereDockerIsInstalled(t *testing.T) {
	for _, docker := range []bool{false, true} {
		src := &fakeHost{listeners: []hostwatch.Listener{ssh}, docker: docker}
		w := newWatcher(src, &memHostStore{})
		if _, err := w.Look(t.Context()); err != nil {
			t.Fatal(err)
		}
		src.listeners = []hostwatch.Listener{ssh, {Proto: "tcp", Port: 8080}}
		look, _ := w.Look(t.Context())
		if len(look.Findings) != 1 {
			t.Fatalf("docker=%v: want one finding, got %+v", docker, look.Findings)
		}
		got := strings.Contains(look.Findings[0].Evidence, "Docker")
		if got != docker {
			t.Errorf("docker=%v: evidence mentions Docker = %v: %q", docker, got, look.Findings[0].Evidence)
		}
		if docker && !strings.Contains(look.Findings[0].Evidence, "firewall") {
			t.Errorf("the note must say the computer's firewall does not cover a published port: %q", look.Findings[0].Evidence)
		}
	}
}

func TestHostWatch_AChangedFileIsOneFindingWithItsOwnCheckAndKey(t *testing.T) {
	src := &fakeHost{files: []hostwatch.FileState{{ID: "sudoers", Hash: "aaaaaaaaaaaaaaaaaaaa"}, {ID: "cron", Hash: "cc"}}}
	store := &memHostStore{}
	w := newWatcher(src, store)
	if _, err := w.Look(t.Context()); err != nil {
		t.Fatal(err)
	}
	src.files = []hostwatch.FileState{{ID: "sudoers", Hash: "bbbbbbbbbbbbbbbbbbbb"}, {ID: "cron", Hash: "cc"}}
	look, err := w.Look(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(look.Findings) != 1 {
		t.Fatalf("want exactly one finding, got %+v", look.Findings)
	}
	f := look.Findings[0]
	if f.Check != HostFileCheck("sudoers") || f.Device != HostDevice || f.Port != 0 || f.Severity != SeverityHigh {
		t.Errorf("finding = %+v", f)
	}
	if f.Version != "bbbbbbbbbbbb" {
		t.Errorf("Version = %q: a short digest, so a further change brings back an accepted finding", f.Version)
	}
	if strings.Contains(f.Evidence, "bbbb") {
		t.Errorf("evidence must be fixed words: %q", f.Evidence)
	}
}

func TestHostWatch_TwoChangedFilesDoNotShareAKey(t *testing.T) {
	src := &fakeHost{files: []hostwatch.FileState{{ID: "sudoers", Hash: "a"}, {ID: "cron", Hash: "a"}}}
	w := newWatcher(src, &memHostStore{})
	w.Look(t.Context())
	src.files = []hostwatch.FileState{{ID: "sudoers", Hash: "b"}, {ID: "cron", Hash: "b"}}
	look, _ := w.Look(t.Context())
	if len(look.Findings) != 2 || look.Findings[0].Key() == look.Findings[1].Key() {
		t.Errorf("accepting one must not accept the other: %+v", look.Findings)
	}
}

func TestHostWatch_AnUpgradeOfTheProgramIsNotAChange(t *testing.T) {
	src := &fakeHost{files: []hostwatch.FileState{{ID: "program", Hash: "old", Tag: "0.31.0"}}}
	store := &memHostStore{}
	w := newWatcher(src, store)
	w.Look(t.Context())
	src.files = []hostwatch.FileState{{ID: "program", Hash: "new", Tag: "0.32.0"}}
	look, _ := w.Look(t.Context())
	if len(look.Findings) != 0 {
		t.Fatalf("a new version replaces the program's file on purpose: %+v", look.Findings)
	}
	if store.w.Files[0].Hash != "new" {
		t.Errorf("the baseline must move to the new version: %+v", store.w.Files)
	}
	src.files = []hostwatch.FileState{{ID: "program", Hash: "tampered", Tag: "0.32.0"}}
	if look, _ := w.Look(t.Context()); len(look.Findings) != 1 {
		t.Errorf("a change inside one version is a finding: %+v", look.Findings)
	}
}

func TestHostWatch_ANewlyWatchedPlaceIsRecordedNotReported(t *testing.T) {
	src := &fakeHost{files: []hostwatch.FileState{{ID: "cron", Hash: "a"}}}
	w := newWatcher(src, &memHostStore{})
	w.Look(t.Context())
	src.files = []hostwatch.FileState{{ID: "cron", Hash: "a"}, {ID: "sudoers", Hash: "x"}}
	if look, _ := w.Look(t.Context()); len(look.Findings) != 0 {
		t.Errorf("a place added to the list in a later version has no earlier look to differ from: %+v", look.Findings)
	}
}

func TestHostWatch_ACannotCheckIsAnErrorAndNeverABaseline(t *testing.T) {
	src := &fakeHost{lerr: hostwatch.ErrUnavailable, files: []hostwatch.FileState{{ID: "cron", Hash: "a"}}}
	store := &memHostStore{}
	_, err := newWatcher(src, store).Look(t.Context())
	if !errors.Is(err, hostwatch.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if store.writes != 0 {
		t.Error("a look that could not be taken must not become the baseline")
	}
}

func TestHostWatch_AnUnsavedBaselineIsAnErrorSoNothingIsClaimed(t *testing.T) {
	store := &memHostStore{werr: errors.New("disk full")}
	if _, err := newWatcher(&fakeHost{listeners: []hostwatch.Listener{ssh}}, store).Look(t.Context()); err == nil {
		t.Error("expected the failed save to be reported")
	}
}

func TestHostWatch_CountsWhatItCannotRead(t *testing.T) {
	src := &fakeHost{files: []hostwatch.FileState{{ID: "sudoers", Hash: "a", Unreadable: 2}, {ID: "cron", Hash: "b", Unreadable: 1}}}
	look, _ := newWatcher(src, &memHostStore{}).Look(t.Context())
	if look.Unreadable != 3 {
		t.Errorf("Unreadable = %d, want 3", look.Unreadable)
	}
}

func TestHostFileTable_CoversEverythingTheReaderWatchesAndNothingElse(t *testing.T) {
	for _, spec := range hostwatch.DefaultFileSpecs("/home/svc", "/usr/bin/llm-proxy") {
		info, ok := hostFiles[spec.ID]
		if !ok {
			t.Errorf("watched place %q has no title, severity or fix", spec.ID)
			continue
		}
		check := HostFileCheck(spec.ID)
		if RemediationFor(check) == "" || CheckTitle(check) == "" {
			t.Errorf("check %q is not registered", check)
		}
		if info.Severity != SeverityMedium && info.Severity != SeverityHigh {
			t.Errorf("%q severity %q", spec.ID, info.Severity)
		}
	}
	if len(hostFiles) != len(hostwatch.DefaultFileSpecs("/home/svc", "/usr/bin/llm-proxy")) {
		t.Error("the finding table lists a place the reader does not watch")
	}
	if RemediationFor(CheckNewListener) == "" || CheckTitle(CheckNewListener) == "" {
		t.Error("new_listener is not registered")
	}
}

func TestHostWatch_WeakSettingsAreFindingsFromTheFirstLookWithFixedWords(t *testing.T) {
	src := &fakeHost{hard: hostwatch.Hardening{SSHFound: true, PasswordLogin: true, RootLogin: true, EmptyPasswords: true, RPFilterOff: true, AutoUpdatesOff: true, FirewallOff: true, OpenKeyFiles: 2}}
	look, err := newWatcher(src, &memHostStore{}).Look(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		CheckHostSSHPasswordLogin: SeverityLow, CheckHostSSHRootLogin: SeverityHigh, CheckHostSSHEmptyPasswords: SeverityHigh, CheckHostRPFilterOff: SeverityMedium,
		CheckHostAutoUpdatesOff: SeverityLow, CheckHostFirewallOff: SeverityLow, CheckHostKeyFilesOpen: SeverityHigh,
	}
	if len(look.Findings) != len(want) {
		t.Fatalf("got %+v", look.Findings)
	}
	for _, f := range look.Findings {
		if f.Device != HostDevice || f.Port != 0 || want[f.Check] != f.Severity || f.Evidence == "" {
			t.Errorf("finding = %+v", f)
		}
	}
}

func TestHostWatch_AGoodConfigurationHasNoSettingFindingsAndUnreadableIsCounted(t *testing.T) {
	src := &fakeHost{hard: hostwatch.Hardening{SSHFound: true, Unreadable: 2}}
	look, _ := newWatcher(src, &memHostStore{}).Look(t.Context())
	if len(look.Findings) != 0 || look.Unreadable != 2 {
		t.Errorf("got %+v", look)
	}
}

func TestHostSettings_EveryCheckIsRegistered(t *testing.T) {
	for check := range hostSettings {
		if RemediationFor(check) == "" || CheckTitle(check) == "" {
			t.Errorf("%q is not registered", check)
		}
		if g, ok := GuideFor(check); !ok || len(g.Steps) == 0 || g.Risk == "" || g.Undo == "" {
			t.Errorf("%q has no complete guide", check)
		}
	}
}

func TestHostWatch_FullDiskIsLowThenMediumInFixedWords(t *testing.T) {
	for level, want := range map[int]string{hostwatch.DiskLow: SeverityLow, hostwatch.DiskCritical: SeverityMedium} {
		look, err := newWatcher(&fakeHost{hard: hostwatch.Hardening{DiskLow: level}}, &memHostStore{}).Look(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(look.Findings) != 1 || look.Findings[0].Check != CheckHostDiskLow || look.Findings[0].Severity != want || look.Findings[0].Evidence == "" {
			t.Errorf("level %d: %+v", level, look.Findings)
		}
	}
	if look, _ := newWatcher(&fakeHost{hard: hostwatch.Hardening{}}, &memHostStore{}).Look(t.Context()); len(look.Findings) != 0 {
		t.Errorf("a disk with room is not a finding: %+v", look.Findings)
	}
}

func integrityLook(t *testing.T, src *fakeHost, store *memHostStore) HostLook {
	t.Helper()
	look, err := newWatcher(src, store).Look(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return look
}

func integrityFindings(look HostLook) []Finding {
	var out []Finding
	for _, f := range look.Findings {
		if f.Check == CheckHostFilesModified {
			out = append(out, f)
		}
	}
	return out
}

func TestHostWatch_AChangedProgramFileIsOneHighFindingWithTheCountAndThePaths(t *testing.T) {
	src := &fakeHost{integrity: hostwatch.IntegrityResult{Complete: true, Checked: 9000, Modified: 12, Paths: []string{"/usr/sbin/sshd", "/usr/bin/sudo"}}}
	got := integrityFindings(integrityLook(t, src, &memHostStore{}))
	if len(got) != 1 {
		t.Fatalf("want one finding, got %+v", got)
	}
	f := got[0]
	if f.Device != HostDevice || f.Severity != SeverityHigh || f.Port != 0 {
		t.Errorf("finding = %+v", f)
	}
	for _, want := range []string{"12 program files", "/usr/sbin/sshd", "/usr/bin/sudo", "and 10 more"} {
		if !strings.Contains(f.Evidence, want) {
			t.Errorf("evidence lacks %q: %s", want, f.Evidence)
		}
	}
	if _, ok := GuideFor(CheckHostFilesModified); !ok || CheckTitle(CheckHostFilesModified) == "" {
		t.Error("the check needs a title and a fix guide")
	}
}

func TestHostWatch_ACleanCompletePassIsNoFindingAndOneFileIsSingular(t *testing.T) {
	if got := integrityFindings(integrityLook(t, &fakeHost{integrity: hostwatch.IntegrityResult{Complete: true, Checked: 9000}}, &memHostStore{})); len(got) != 0 {
		t.Errorf("a clean pass is not a finding: %+v", got)
	}
	one := integrityFindings(integrityLook(t, &fakeHost{integrity: hostwatch.IntegrityResult{Complete: true, Checked: 5, Modified: 1, Paths: []string{"/usr/bin/x"}}}, &memHostStore{}))
	if len(one) != 1 || !strings.Contains(one[0].Evidence, "1 program file ") || strings.Contains(one[0].Evidence, "more") {
		t.Errorf("one file: %+v", one)
	}
}

func TestHostWatch_APassCutShortKeepsTheEarlierFindingAndNeverResolvesIt(t *testing.T) {
	store := &memHostStore{}
	src := &fakeHost{integrity: hostwatch.IntegrityResult{Complete: true, Checked: 9000, Modified: 2, Paths: []string{"/usr/sbin/sshd"}}}
	integrityLook(t, src, store)
	src.integrity = hostwatch.IntegrityResult{Complete: false, Checked: 300}
	got := integrityFindings(integrityLook(t, src, store))
	if len(got) != 1 || !strings.Contains(got[0].Evidence, "2 program files") {
		t.Errorf("an incomplete pass resolved the finding: %+v", got)
	}
	src.integrity = hostwatch.IntegrityResult{Complete: true, Checked: 9000}
	if gone := integrityFindings(integrityLook(t, src, store)); len(gone) != 0 {
		t.Errorf("a complete clean pass resolves it: %+v", gone)
	}
	src.integrity = hostwatch.IntegrityResult{Complete: false, Checked: 10}
	if again := integrityFindings(integrityLook(t, src, store)); len(again) != 0 {
		t.Errorf("after a clean pass an incomplete one has nothing to carry: %+v", again)
	}
}

func TestHostWatch_AComputerWithoutDpkgSaysNothingAboutItsProgramFiles(t *testing.T) {
	store := &memHostStore{}
	if got := integrityFindings(integrityLook(t, &fakeHost{}, store)); len(got) != 0 {
		t.Errorf("nothing was checked and nothing is reported: %+v", got)
	}
}

func TestHostWatch_FilesThatCouldNotBeReadAreCountedWithTheOtherUnreadablePlaces(t *testing.T) {
	look := integrityLook(t, &fakeHost{integrity: hostwatch.IntegrityResult{Complete: true, Checked: 5, Unreadable: 3}}, &memHostStore{})
	if look.Unreadable != 3 {
		t.Errorf("unreadable = %d", look.Unreadable)
	}
}

func TestHostWatcher_LookGivesItsContextToTheProgramFilePass(t *testing.T) {
	src := &fakeHost{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := newWatcher(src, &memHostStore{}).Look(ctx); err != nil {
		t.Fatal(err)
	}
	if !src.integrityRan || src.integrityErr == nil {
		t.Fatal("the pass over the program files must run under the look's context, so a stopped run stops it")
	}
}

// The pass over the program files runs at the end of an audit, after the scan and the model. A run that used most of its time
// cap must still get the pass's own budget: the run's deadline is not a reason to stop it, only a stop or a shutdown is.
func TestHostWatcher_ARunNearItsTimeCapStillGetsTheWholePass(t *testing.T) {
	src := &fakeHost{}
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	<-ctx.Done() // the run's cap has passed
	if _, err := newWatcher(src, &memHostStore{}).Look(ctx); err != nil {
		t.Fatal(err)
	}
	if !src.integrityRan || src.integrityErr != nil {
		t.Fatalf("the run's time cap cut the pass short: %v", src.integrityErr)
	}
}
