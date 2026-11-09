package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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
