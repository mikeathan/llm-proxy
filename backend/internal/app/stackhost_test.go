package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/network"
	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/stack"
	"llm-proxy/internal/core/stackport"
	"llm-proxy/internal/core/tools"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

type portsOnly struct {
	manifest stacksdk.Manifest
	stacksdk.Stack
}

func (p portsOnly) Manifest() stacksdk.Manifest { return p.manifest }

func baseManifest(requires ...string) stacksdk.Manifest {
	return stacksdk.Manifest{
		ID: "demo", Title: "Demo", Layers: []stacksdk.Layer{{ID: "l", Title: "L"}},
		Modules:  []stacksdk.ModuleSpec{{ID: "m", Title: "M", Layer: "l", Sends: stacksdk.SendsNothing}},
		Settings: stacksdk.SettingsSchema{Key: "demo", Version: 1}, Requires: requires,
	}
}

type nopLog struct{}

func (nopLog) Log(stacksdk.Level, string, ...string) {}

// A stack gets exactly the ports its manifest declares: one it did not ask for is a hole, one it asked for and lacks is a crash later.
func TestNewStackHost_TheHostGivesExactlyThePortsTheManifestRequires(t *testing.T) {
	if _, err := newStackHost(portsOnly{manifest: baseManifest(stacksdk.PortLog)}, stacksdk.Host{Log: nopLog{}}); err != nil {
		t.Fatalf("a matching host: %v", err)
	}
	if _, err := newStackHost(portsOnly{manifest: baseManifest(stacksdk.PortLog, stacksdk.PortFetch)}, stacksdk.Host{Log: nopLog{}}); err == nil {
		t.Error("a required port that is missing must be refused")
	}
	if _, err := newStackHost(portsOnly{manifest: baseManifest(stacksdk.PortLog)}, stacksdk.Host{Log: nopLog{}, Fetch: fetchStub{}}); err == nil {
		t.Error("a port the manifest did not ask for must be refused")
	}
	bad := baseManifest(stacksdk.PortLog)
	bad.ID = "Not Valid"
	if _, err := newStackHost(portsOnly{manifest: bad}, stacksdk.Host{Log: nopLog{}}); err == nil {
		t.Error("an invalid manifest must be refused")
	}
}

type fetchStub struct{}

func (fetchStub) Get(context.Context, stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	return stacksdk.FetchResponse{}, nil
}

func TestClaimedFiles_TwoStacksCannotClaimOneFile(t *testing.T) {
	a, b := baseManifest(), baseManifest()
	a.ID, b.ID = "a", "b"
	a.Storage, b.Storage = []string{"one.json"}, []string{"two.json"}
	if got, err := claimedFiles(a, b); err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	b.Storage = []string{"one.json"}
	if _, err := claimedFiles(a, b); err == nil {
		t.Error("a file claimed twice must be refused")
	}
}

// The registry (core) and the manifest (stack) both describe the DNS check module; the manifest is the source and the registry must not
// drift from it.
func TestNetworkStack_TheRegistryModuleMatchesTheManifestSpec(t *testing.T) {
	spec := networkStack().Manifest().Modules[0]
	if spec.ID != netaudit.MonitorDNS {
		t.Errorf("the module id %q is the monitor id %q the core watches", spec.ID, netaudit.MonitorDNS)
	}
	var found bool
	for _, m := range allModules(t, newTestStacks(t).reg) {
		if m.ID != spec.ID {
			continue
		}
		found = true
		if m.Title != spec.Title || m.Layer != spec.Layer || string(m.Sends) != string(spec.Sends) || m.StepUp != spec.StepUp || m.Stack != stack.Network {
			t.Errorf("registry %+v differs from manifest %+v", m, spec)
		}
	}
	if !found {
		t.Errorf("module %s is not in the registry", spec.ID)
	}
}

type offOpener struct{}

func (offOpener) OpenAccepting(context.Context, string, string) (*http.Response, error) {
	return nil, tools.ErrInternetBlocked
}

// The whole path with real adapters: the stack reads and writes the core's typed settings through the Config port, keeps its log in the
// data folder through the DataDir port, and its routes answer through the core's router.
func TestNetworkStack_RunsOverTheRealAdaptersAndTheCoreSeesItsSettings(t *testing.T) {
	// Already switched on when the stack starts: a switched-off collector looks at its settings again only every 30 seconds, as before.
	store := &memPolicy{cfg: models.NetauditConfig{DNSCheckEnabled: true, DNSCheckIntervalMinutes: 30, DNSCheckNames: []string{"a.example"}}}
	dir := t.TempDir()
	st := networkStack()
	host := stacksdk.Host{
		Config:  stackport.Config[models.NetauditConfig]{Store: store},
		Log:     nopLog{},
		DataDir: stackport.DataDir{Dir: dir, Files: st.Manifest().Storage},
		Guard:   stackport.Guard{Dial: func(context.Context, string, string) (net.Conn, error) { return nil, tools.ErrLANBlocked }},
		Fetch:   stackport.Fetch{Opener: offOpener{}},
		Gateway: stackport.Gateway{Read: func() netip.Addr { return netip.Addr{} }},
		// The manifest lists a sensor, so the host must give the scoped dial.
		ScopedGuard: stackport.ScopedGuard{Check: func(netip.Addr) error { return nil }, Dial: func(context.Context, string, string) (net.Conn, error) { return nil, tools.ErrLANBlocked }},
	}
	h, err := newStackHost(st, host)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.Run(ctx)
	defer cancel()

	// Switched on with no router address known, the first cycle logs a "cannot compare" row in the stack's own file.
	deadline := time.Now().Add(3 * time.Second)
	var st2 stacksdk.ModuleStatus
	for time.Now().Before(deadline) {
		if got, ok := h.Module(context.Background(), netaudit.MonitorDNS); ok && !got.LastRun.IsZero() {
			st2 = got
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	router := api.NewRouter()
	if err := api.MountStackRoutes(router, st, nil); err != nil {
		t.Fatal(err)
	}
	do := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/admin/api/netaudit/dns-check", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	rec := do(http.MethodGet, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"interval_minutes":30`) || !strings.Contains(rec.Body.String(), `"names":["a.example"]`) {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPut, `{"names":["b.example"],"server":"192.168.1.53"}`); rec.Code != 200 {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	if c := store.cfg; !c.DNSCheckEnabled || c.DNSCheckIntervalMinutes != 30 || !slices.Equal(c.DNSCheckNames, []string{"b.example"}) || c.DNSCheckServer != "192.168.1.53" {
		t.Errorf("the core's typed settings = %+v", c)
	}
	if !st2.Enabled || st2.Every != 30*time.Minute {
		t.Errorf("status = %+v", st2)
	}
	if _, err := os.Stat(filepath.Join(dir, "network-dns-checks.json")); err != nil {
		t.Errorf("the log file is not where it always was: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.yml")); err == nil {
		t.Error("the stack wrote outside its files")
	}
	if err := stackport.Remove(dir, st.Manifest().Storage); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "network-dns-checks.json")); err == nil {
		t.Error("a factory reset must remove the stack's file")
	}
}

// A file a stack lists in its manifest is the stack's: no core store may use the name (D10). The core's own source is searched for the
// quoted name, so a store that kept the old file under the same name after the move would fail here.
func TestNetworkStack_NoCoreStoreUsesAFileTheManifestClaims(t *testing.T) {
	files := networkStack().Manifest().Storage
	if len(files) == 0 {
		t.Fatal("the stack claims no file: this test would check nothing")
	}
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, name := range files {
			if strings.Contains(string(src), `"`+name+`"`) {
				t.Errorf("%s names %q, a file the Network stack's manifest claims", path, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// ---- findings a stack offers (ADR-0036) ----

type offering struct {
	portsOnly
	items []stacksdk.StackFinding
	err   error
	wait  time.Duration
}

func (o offering) Findings(ctx context.Context) ([]stacksdk.StackFinding, error) {
	select {
	case <-time.After(o.wait):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return o.items, o.err
}

func offeringHost(t *testing.T, o offering) *stackHost {
	t.Helper()
	o.manifest = baseManifest(stacksdk.PortLog)
	o.manifest.Modules[0].Finds = []string{"declared_one"}
	h, err := newStackHost(o, stacksdk.Host{Log: nopLog{}})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// The host passes on only the ids the module declared, so a stack cannot offer a finding it did not announce.
func TestStackHost_OfferedKeepsOnlyDeclaredFindings(t *testing.T) {
	a := netip.MustParseAddr("10.0.0.1")
	h := offeringHost(t, offering{items: []stacksdk.StackFinding{
		{ID: "declared_one", Subject: a, Count: 2, Names: []string{"a.example"}},
		{ID: "undeclared", Subject: a, Count: 2, Names: []string{"a.example"}},
	}})
	got, err := h.Offered(context.Background())
	if err != nil || len(got) != 1 || got[0].ID != "declared_one" || got[0].Subject != a || got[0].Count != 2 || got[0].Names[0] != "a.example" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// A stack that is slow or fails must not stall or break the nightly report.
func TestStackHost_OfferedGivesUpOnASlowStackAndReportsAFailure(t *testing.T) {
	old := offerTimeout
	offerTimeout = 20 * time.Millisecond
	t.Cleanup(func() { offerTimeout = old })
	if _, err := offeringHost(t, offering{wait: time.Second}).Offered(context.Background()); err == nil {
		t.Error("a slow stack must time out")
	}
	if _, err := offeringHost(t, offering{err: stacksdk.Errorf(stacksdk.CodeInternal, "boom")}).Offered(context.Background()); err == nil {
		t.Error("a failing stack is an error the report states")
	}
}

// What the module's own screen says it finds is the core's wording, and the Network stack's finding is one of them.
func TestNetworkStack_TheDNSCheckIsFoundByTheDNSModule(t *testing.T) {
	if !slices.Contains(moduleFinds["dns_check"], netaudit.CheckDNSRouterDiffers) {
		t.Error("the DNS check module must list the finding it offers")
	}
	for _, m := range networkStack().Manifest().Modules {
		for _, id := range m.Finds {
			if netaudit.CheckTitle(id) == "" {
				t.Errorf("the stack offers %q, which the core has no title for", id)
			}
		}
	}
}

// ---- the sensor path (ADR-0038) ----

// sensing is a stack with one sensor module that answers Look as told.
type sensing struct {
	portsOnly
	got    stacksdk.LookRequest
	result stacksdk.LookResult
	err    error
	wait   time.Duration
}

func (l *sensing) Look(ctx context.Context, req stacksdk.LookRequest) (stacksdk.LookResult, error) {
	l.got = req
	select {
	case <-time.After(l.wait):
	case <-ctx.Done():
		return stacksdk.LookResult{}, ctx.Err()
	}
	return l.result, l.err
}

func sensingHost(t *testing.T, l *sensing) *stackHost {
	t.Helper()
	l.manifest = baseManifest(stacksdk.PortLog, stacksdk.PortScopedGuard)
	l.manifest.Looks = []string{"sense"}
	h, err := newStackHost(l, stacksdk.Host{Log: nopLog{}, ScopedGuard: stackport.ScopedGuard{Check: func(netip.Addr) error { return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// The host asks a sensor only for a module the manifest lists and only for a bounded list of addresses, and the stack gets the addresses it was given.
func TestStackHost_LookKeepsToTheDeclaredModuleAndTheBound(t *testing.T) {
	a := netip.MustParseAddr("192.168.1.30")
	l := &sensing{result: stacksdk.LookResult{Asked: []stacksdk.Pair{{Addr: a, Port: 161}}}}
	h := sensingHost(t, l)
	got, err := h.Look(context.Background(), "sense", []netip.Addr{a})
	if err != nil || len(got.Asked) != 1 || l.got.Module != "sense" || len(l.got.Addrs) != 1 || l.got.Addrs[0] != a {
		t.Fatalf("got %+v, %v; the stack saw %+v", got, err, l.got)
	}
	l.got = stacksdk.LookRequest{}
	if _, err := h.Look(context.Background(), "other", []netip.Addr{a}); err == nil || l.got.Module != "" {
		t.Errorf("a module the manifest does not list must be refused before the stack is called: %v", err)
	}
	if _, err := h.Look(context.Background(), "sense", make([]netip.Addr, stacksdk.MaxLookAddrs+1)); err == nil || l.got.Module != "" {
		t.Errorf("more addresses than the bound must be refused before the stack is called: %v", err)
	}
}

// A sensor that fails or is slow must not stall or break the nightly report.
func TestStackHost_LookGivesUpOnASlowStackAndReportsAFailure(t *testing.T) {
	old := lookTimeout
	lookTimeout = 20 * time.Millisecond
	t.Cleanup(func() { lookTimeout = old })
	a := []netip.Addr{netip.MustParseAddr("192.168.1.30")}
	if _, err := sensingHost(t, &sensing{wait: time.Second}).Look(context.Background(), "sense", a); err == nil {
		t.Error("a slow sensor must time out")
	}
	if _, err := sensingHost(t, &sensing{err: stacksdk.Errorf(stacksdk.CodeInternal, "boom")}).Look(context.Background(), "sense", a); err == nil {
		t.Error("a failing sensor is an error the report states")
	}
	plain := offeringHost(t, offering{})
	if _, err := plain.Look(context.Background(), "sense", a); err == nil {
		t.Error("a stack that is no sensor cannot be asked to look")
	}
}

// The UDP checks reach the stack through the host, in the core's own types.
func TestStackUDPAsker_GivesTheCoreItsOwnTypesAndAnUnhostedStackCannotLook(t *testing.T) {
	a := netip.MustParseAddr("192.168.1.30")
	l := &sensing{result: stacksdk.LookResult{
		Asked:    []stacksdk.Pair{{Addr: a, Port: 161}, {Addr: a, Port: 1900}},
		Answered: []stacksdk.Pair{{Addr: a, Port: 1900}},
	}}
	l.manifest = baseManifest(stacksdk.PortLog, stacksdk.PortScopedGuard)
	l.manifest.Looks = []string{networkUDPModule}
	h, err := newStackHost(l, stacksdk.Host{Log: nopLog{}, ScopedGuard: stackport.ScopedGuard{Check: func(netip.Addr) error { return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := stackUDPAsker{host: h}.Look(context.Background(), []netip.Addr{a})
	if err != nil || l.got.Module != networkUDPModule {
		t.Fatalf("got %+v, %v; module %q", got, err, l.got.Module)
	}
	if want := (netaudit.UDPLooked{Asked: []netaudit.UDPPair{{Addr: a, Port: 161}, {Addr: a, Port: 1900}}, Answered: []netaudit.UDPPair{{Addr: a, Port: 1900}}}); !slices.Equal(got.Asked, want.Asked) || !slices.Equal(got.Answered, want.Answered) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if _, err := (stackUDPAsker{}).Look(context.Background(), []netip.Addr{a}); err == nil {
		t.Error("an asker with no hosted stack must say it cannot look")
	}
}

// The gate of ADR-0038 rule 2, through the whole path: the real Network stack, the real scoped dial and the real declared scan scope. A device
// outside the operator's declared ranges is never dialled, exactly as the core's own probes never dialled it.
func TestNetworkStack_TheUDPSensorNeverDialsAnAddressOutsideTheDeclaredScope(t *testing.T) {
	store := &memPolicy{cfg: models.NetauditConfig{ScanRanges: []string{"192.168.1.0/24"}}}
	scope := netaudit.NewScope(store, net.DefaultResolver, nil)
	var dialed []string
	refuse := func(_ context.Context, _, addr string) (net.Conn, error) {
		dialed = append(dialed, addr)
		return nil, tools.ErrLANBlocked
	}
	st := networkStack()
	h, err := newStackHost(st, stacksdk.Host{
		Config:      stackport.Config[models.NetauditConfig]{Store: store},
		Log:         nopLog{},
		DataDir:     stackport.DataDir{Dir: t.TempDir(), Files: st.Manifest().Storage},
		Guard:       stackport.Guard{Dial: refuse},
		Fetch:       stackport.Fetch{Opener: offOpener{}},
		Gateway:     stackport.Gateway{Read: func() netip.Addr { return netip.Addr{} }},
		ScopedGuard: stackport.ScopedGuard{Check: func(a netip.Addr) error { return scope.Policy().Check(a) }, Dial: refuse},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.Run(ctx)
	defer cancel()

	inside, outside := netip.MustParseAddr("192.168.1.30"), netip.MustParseAddr("192.168.9.9")
	got, err := h.Look(context.Background(), networkUDPModule, []netip.Addr{inside, outside})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dialed {
		if strings.HasPrefix(d, outside.String()+":") {
			t.Errorf("an address outside the declared scope was dialled: %s", d)
		}
	}
	if len(dialed) != 2 || !strings.HasPrefix(dialed[0], inside.String()+":") {
		t.Errorf("only the declared device is asked (two questions): %v", dialed)
	}
	if len(got.Answered) != 0 || len(got.Asked) != 4 {
		t.Errorf("both devices were asked in intent and none answered: %+v", got)
	}
}

// Parity (ADR-0038 rule 1): a device that answers through the real sensor path, the stack and the host and the core's checks, becomes exactly the
// finding the core made before the sensor moved, in every field, so no acknowledgement or fix keyed on it is lost on the first night.
func TestNetworkStack_AnAnswerThroughTheSensorIsTheSameFindingAsBefore(t *testing.T) {
	device, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = device.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			_, from, err := device.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = device.WriteToUDP([]byte("HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\n\r\n"), from)
		}
	}()
	toDevice := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !strings.HasSuffix(addr, ":1900") {
			return nil, tools.ErrLANBlocked
		}
		var d net.Dialer
		return d.DialContext(ctx, network, device.LocalAddr().String())
	}
	store := &memPolicy{cfg: models.NetauditConfig{ScanRanges: []string{"192.168.1.0/24"}}}
	scope := netaudit.NewScope(store, net.DefaultResolver, nil)
	st := networkStack()
	h, err := newStackHost(st, stacksdk.Host{
		Config:      stackport.Config[models.NetauditConfig]{Store: store},
		Log:         nopLog{},
		DataDir:     stackport.DataDir{Dir: t.TempDir(), Files: st.Manifest().Storage},
		Guard:       stackport.Guard{Dial: toDevice},
		Fetch:       stackport.Fetch{Opener: offOpener{}},
		Gateway:     stackport.Gateway{Read: func() netip.Addr { return netip.Addr{} }},
		ScopedGuard: stackport.ScopedGuard{Check: func(a netip.Addr) error { return scope.Policy().Check(a) }, Dial: toDevice},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.Run(ctx)
	defer cancel()

	host := netaudit.Host{Addr: netip.MustParseAddr("192.168.1.30"), Up: true}
	res := (&netaudit.UDPChecks{Asker: stackUDPAsker{host: h}}).Findings(context.Background(), []netaudit.Host{host})
	want := netaudit.Finding{Device: "192.168.1.30", Port: 1900, Proto: "udp", Service: "UPnP", Check: netaudit.CheckUDPService, Severity: netaudit.SeverityLow,
		Evidence: "UPnP answered a check from this computer. UPnP lets other devices on the network ask this one to open ports to the internet"}
	if len(res.Findings) != 1 || !reflect.DeepEqual(res.Findings[0], want) || res.Findings[0].Key() != want.Key() {
		t.Fatalf("findings = %+v, want exactly %+v", res.Findings, want)
	}
	// The port that stayed silent is unknown, so an earlier SNMP finding for this device is carried, not resolved.
	earlier := netaudit.Finding{Device: "192.168.1.30", Port: 161, Proto: "udp", Service: "SNMP", Check: netaudit.CheckUDPService, Severity: netaudit.SeverityMedium}
	if got := res.Carry([]netaudit.Finding{earlier}); len(got) != 1 {
		t.Errorf("silence on port 161 must keep the earlier finding: %+v", got)
	}
}

// ---- the web sensor path (ADR-0038) ----

// webSensing is a stack with a web sensor module that answers as told.
type webSensing struct {
	portsOnly
	gotWeb  stacksdk.WebRequest
	gotCert stacksdk.WebRequest
	web     stacksdk.WebResult
	certs   stacksdk.CertResult
	err     error
	wait    time.Duration
}

func (w *webSensing) pause(ctx context.Context) error {
	select {
	case <-time.After(w.wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *webSensing) LookWeb(ctx context.Context, req stacksdk.WebRequest) (stacksdk.WebResult, error) {
	w.gotWeb = req
	if err := w.pause(ctx); err != nil {
		return stacksdk.WebResult{}, err
	}
	return w.web, w.err
}

func (w *webSensing) LookCertificates(ctx context.Context, req stacksdk.WebRequest) (stacksdk.CertResult, error) {
	w.gotCert = req
	if err := w.pause(ctx); err != nil {
		return stacksdk.CertResult{}, err
	}
	return w.certs, w.err
}

func webSensingHost(t *testing.T, w *webSensing) *stackHost {
	t.Helper()
	w.manifest = baseManifest(stacksdk.PortLog, stacksdk.PortScopedGuard)
	w.manifest.Looks = []string{"web"}
	h, err := newStackHost(w, stacksdk.Host{Log: nopLog{}, ScopedGuard: stackport.ScopedGuard{Check: func(netip.Addr) error { return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func targetsOf(addrs ...string) []stacksdk.WebTarget {
	var out []stacksdk.WebTarget
	for _, a := range addrs {
		out = append(out, stacksdk.WebTarget{Addr: netip.MustParseAddr(a), Port: 80})
	}
	return out
}

// The host holds a web request to what the manifest announced and to the bounds, and waits as long as the run may take plus a short grace, not
// the fixed minutes of the UDP look: the nightly run of the web checks is allowed ten.
func TestStackHost_LookWebKeepsToTheManifestTheBoundsAndTheRunsOwnLimit(t *testing.T) {
	w := &webSensing{web: stacksdk.WebResult{Hits: []stacksdk.WebHit{{Addr: netip.MustParseAddr("192.168.1.30"), Port: 80, ID: "x"}}}}
	h := webSensingHost(t, w)
	req := stacksdk.WebRequest{Module: "web", Targets: targetsOf("192.168.1.30"), LimitSeconds: 60}
	if got, err := h.LookWeb(context.Background(), req); err != nil || len(got.Hits) != 1 || w.gotWeb.Module != "web" {
		t.Fatalf("got %+v, %v", got, err)
	}
	w.gotWeb = stacksdk.WebRequest{}
	for name, bad := range map[string]stacksdk.WebRequest{
		"another module":   {Module: "other", Targets: req.Targets, LimitSeconds: 60},
		"no targets":       {Module: "web", LimitSeconds: 60},
		"too many targets": {Module: "web", Targets: make([]stacksdk.WebTarget, stacksdk.MaxWebTargets+1), LimitSeconds: 60},
		"no limit":         {Module: "web", Targets: req.Targets},
		"limit too long":   {Module: "web", Targets: req.Targets, LimitSeconds: stacksdk.MaxLookSeconds + 1},
	} {
		if _, err := h.LookWeb(context.Background(), bad); err == nil {
			t.Errorf("%s: must be refused", name)
		}
		if _, err := h.LookCertificates(context.Background(), bad); err == nil {
			t.Errorf("%s: certificates must be refused", name)
		}
	}
	if w.gotWeb.Module != "" || w.gotCert.Module != "" {
		t.Error("a refused request must not reach the stack")
	}
	if _, err := offeringHost(t, offering{}).LookWeb(context.Background(), req); err == nil {
		t.Error("a stack that is no web sensor cannot be asked")
	}
}

func TestStackHost_LookWebGivesUpAfterTheRunsLimitAndAGraceAndReportsAFailure(t *testing.T) {
	old := webLookGrace
	webLookGrace = 20 * time.Millisecond
	t.Cleanup(func() { webLookGrace = old })
	req := stacksdk.WebRequest{Module: "web", Targets: targetsOf("192.168.1.30"), LimitSeconds: 1}
	start := time.Now()
	if _, err := webSensingHost(t, &webSensing{wait: 5 * time.Second}).LookWeb(context.Background(), req); err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("a stack that overruns its limit must be cut off after the limit and the grace: %v after %v", err, time.Since(start))
	}
	if _, err := webSensingHost(t, &webSensing{err: stacksdk.Errorf(stacksdk.CodeInternal, "boom")}).LookCertificates(context.Background(), req); err == nil {
		t.Error("a failing stack is an error the report states")
	}
}

func openGuardrails() (bool, string) { return true, "" }
func lanOff() (bool, string)         { return false, blockedNoLAN }

func inScope(allowed ...string) func(netip.Addr) error {
	return func(a netip.Addr) error {
		if slices.Contains(allowed, a.String()) {
			return nil
		}
		return errors.New("outside the declared scan scope")
	}
}

// The core refuses a run before it asks the stack, as the prober did: a target outside the declared scope, too many targets, no limit, or the
// guardrails not allowing the local network. Each is an error the report states (never "nothing found"), and nothing reaches the stack.
func TestStackWebChecker_RefusesBeforeAskingTheStack(t *testing.T) {
	w := &webSensing{}
	h := webSensingHost(t, w)
	good := []netaudit.WebTarget{{Addr: netip.MustParseAddr("192.168.1.30"), Port: 80}}
	checker := func(scope func(netip.Addr) error, ready func() (bool, string)) stackWebChecker {
		return stackWebChecker{host: h, scope: scope, ready: ready, module: "web"}
	}
	cases := map[string]func() error{
		"a target outside the scope": func() error {
			_, err := checker(inScope("192.168.1.99"), openGuardrails).Probe(context.Background(), good, time.Minute)
			return err
		},
		"no local network in the guardrails": func() error {
			_, err := checker(inScope("192.168.1.30"), lanOff).Probe(context.Background(), good, time.Minute)
			return err
		},
		"too many targets": func() error {
			_, err := checker(inScope("192.168.1.30"), openGuardrails).Probe(context.Background(), make([]netaudit.WebTarget, netaudit.MaxWebTargets+1), time.Minute)
			return err
		},
		"no targets": func() error {
			_, err := checker(inScope("192.168.1.30"), openGuardrails).Probe(context.Background(), nil, time.Minute)
			return err
		},
		"no limit": func() error {
			_, err := checker(inScope("192.168.1.30"), openGuardrails).Probe(context.Background(), good, 0)
			return err
		},
		"an IPv6 address": func() error {
			_, err := checker(func(netip.Addr) error { return nil }, openGuardrails).Probe(context.Background(), []netaudit.WebTarget{{Addr: netip.MustParseAddr("fe80::1"), Port: 80}}, time.Minute)
			return err
		},
		"certificates outside the scope": func() error {
			_, err := checker(inScope("192.168.1.99"), openGuardrails).ProbeTLS(context.Background(), good, time.Minute)
			return err
		},
	}
	for name, run := range cases {
		if run() == nil {
			t.Errorf("%s: must be refused with an error", name)
		}
	}
	if w.gotWeb.Module != "" || w.gotCert.Module != "" {
		t.Error("a refused run reached the stack")
	}
	_, err := checker(inScope("192.168.1.30"), lanOff).Probe(context.Background(), good, time.Minute)
	if err == nil || !strings.Contains(err.Error(), blockedNoLAN) {
		t.Errorf("the report must say why: %v", err)
	}
	if _, err := (stackWebChecker{ready: openGuardrails, scope: inScope("192.168.1.30")}).Probe(context.Background(), good, time.Minute); err == nil {
		t.Error("an unhosted stack cannot look, and must say so")
	}
}

// The core's types go to the stack and come back: ids and numbers, with the limit in seconds.
func TestStackWebChecker_ConvertsBothWaysAndPassesTheLimit(t *testing.T) {
	a := netip.MustParseAddr("192.168.1.30")
	when := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	w := &webSensing{
		web:   stacksdk.WebResult{Hits: []stacksdk.WebHit{{Addr: a, Port: 8080, ID: "grafana_login"}}, Partial: true},
		certs: stacksdk.CertResult{Facts: []stacksdk.CertFact{{Addr: a, Port: 443, NotBefore: when.Add(-time.Hour), NotAfter: when, SelfSigned: true, OldVersions: []uint16{0x0301}}}},
	}
	c := stackWebChecker{host: webSensingHost(t, w), scope: inScope("192.168.1.30"), ready: openGuardrails, module: "web"}
	targets := []netaudit.WebTarget{{Addr: a, Port: 8080}, {Addr: a, Port: 443, TLS: true}}
	res, err := c.Probe(context.Background(), targets, 90*time.Second)
	if err != nil || !res.Partial || len(res.Hits) != 1 || res.Hits[0] != (netaudit.WebHit{Addr: a, Port: 8080, ID: "grafana_login"}) {
		t.Fatalf("res = %+v, %v", res, err)
	}
	if w.gotWeb.LimitSeconds != 90 || len(w.gotWeb.Targets) != 2 || w.gotWeb.Targets[1] != (stacksdk.WebTarget{Addr: a, Port: 443, TLS: true}) {
		t.Errorf("request = %+v", w.gotWeb)
	}
	tlsRes, err := c.ProbeTLS(context.Background(), targets, 90*time.Second)
	want := netaudit.TLSFact{Addr: a, Port: 443, NotBefore: when.Add(-time.Hour), NotAfter: when, SelfSigned: true, OldVersions: []uint16{0x0301}}
	if err != nil || len(tlsRes.Facts) != 1 || !reflect.DeepEqual(tlsRes.Facts[0], want) {
		t.Fatalf("tls = %+v, %v", tlsRes, err)
	}
}

// The core's words for a check are held equal to the stack's list: the stack owns the paths and the tests, the core the title and the
// severity, and a check added on one side only would be dropped or worded wrongly.
func TestWebChecks_TheCoresTableMatchesTheStacksList(t *testing.T) {
	stackList, coreList := network.WebChecks(), netaudit.WebChecks()
	if len(stackList) != len(coreList) {
		t.Fatalf("the stack has %d checks, the core words %d", len(stackList), len(coreList))
	}
	for i, s := range stackList {
		c := coreList[i]
		if s.ID != c.ID || s.Title != c.Title || s.Severity != c.Severity {
			t.Errorf("check %d: stack %+v, core %+v", i, s, c)
		}
	}
	if netaudit.MaxWebTargets != stacksdk.MaxWebTargets {
		t.Errorf("the core's bound is %d and the SDK's %d", netaudit.MaxWebTargets, stacksdk.MaxWebTargets)
	}
}

// netHostWith hosts the real Network stack over a scoped dial that connects every permitted address to one stand-in device, with the real
// declared scan scope. dialed records the addresses the stack asked to connect to.
func netHostWith(t *testing.T, ranges []string, device string) (h *stackHost, scope *netaudit.Scope, dialed func() []string) {
	t.Helper()
	store := &memPolicy{cfg: models.NetauditConfig{ScanRanges: ranges}}
	scope = netaudit.NewScope(store, net.DefaultResolver, nil)
	var mu sync.Mutex
	var asked []string
	toDevice := func(ctx context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		asked = append(asked, addr)
		mu.Unlock()
		var d net.Dialer
		return d.DialContext(ctx, network, device)
	}
	st := networkStack()
	h, err := newStackHost(st, stacksdk.Host{
		Config:      stackport.Config[models.NetauditConfig]{Store: store},
		Log:         nopLog{},
		DataDir:     stackport.DataDir{Dir: t.TempDir(), Files: st.Manifest().Storage},
		Guard:       stackport.Guard{Dial: func(context.Context, string, string) (net.Conn, error) { return nil, tools.ErrLANBlocked }},
		Fetch:       stackport.Fetch{Opener: offOpener{}},
		Gateway:     stackport.Gateway{Read: func() netip.Addr { return netip.Addr{} }},
		ScopedGuard: stackport.ScopedGuard{Check: func(a netip.Addr) error { return scope.Policy().Check(a) }, Dial: toDevice},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.Run(ctx)
	t.Cleanup(cancel)
	return h, scope, func() []string { mu.Lock(); defer mu.Unlock(); return slices.Clone(asked) }
}

// Parity (ADR-0038 rule 1): a web page answered through the whole new path, the stack, the host, the adapter and the core's words, becomes
// exactly the finding the core made before the web checks moved. The stack asks every fixed path at its gentle pace, so this takes a few seconds.
func TestNetworkStack_AWebPageThroughTheSensorIsTheSameFindingAsBefore(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte("<title>Home Assistant</title>"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	h, scope, _ := netHostWith(t, []string{"192.168.1.0/24"}, srv.Listener.Addr().String())
	checker := stackWebChecker{
		host: h, module: networkWebModule,
		scope: func(a netip.Addr) error { return scope.Policy().Check(a) },
		ready: openGuardrails,
	}
	a := netip.MustParseAddr("192.168.1.30")
	res, err := checker.Probe(context.Background(), []netaudit.WebTarget{{Addr: a, Port: 8123}}, 2*time.Minute)
	if err != nil || res.Partial {
		t.Fatalf("res = %+v, %v", res, err)
	}
	got := netaudit.WebFindings(res.Hits)
	want := netaudit.Finding{Device: "192.168.1.30", Port: 8123, Proto: "tcp", Check: netaudit.CheckWebExposure, Severity: netaudit.SeverityInfo,
		Evidence: "The web checks found: The page of Home Assistant", Version: got[0].Version}
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) || got[0].Version == "" {
		t.Fatalf("findings = %+v, want %+v", got, want)
	}
}

// Even if the core's own pre-check were bypassed, the stack's scoped dial refuses an address outside the declared scope at the moment of
// connecting: nothing is dialled and nothing is found.
func TestNetworkStack_TheWebSensorNeverDialsAnAddressOutsideTheDeclaredScope(t *testing.T) {
	h, _, dialed := netHostWith(t, []string{"192.168.1.0/24"}, "127.0.0.1:1")
	out := netip.MustParseAddr("192.168.9.9")
	req := stacksdk.WebRequest{Module: networkWebModule, Targets: []stacksdk.WebTarget{{Addr: out, Port: 80, TLS: false}, {Addr: out, Port: 443, TLS: true}}, LimitSeconds: 1}
	web, err := h.LookWeb(context.Background(), req)
	if err != nil || len(web.Hits) != 0 {
		t.Fatalf("web = %+v, %v", web, err)
	}
	certs, err := h.LookCertificates(context.Background(), req)
	if err != nil || len(certs.Facts) != 0 {
		t.Fatalf("certs = %+v, %v", certs, err)
	}
	if got := dialed(); len(got) != 0 {
		t.Fatalf("addresses outside the declared scope were dialled: %v", got)
	}
}
