package network

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/network/dnscheck"
	"llm-proxy-stacks/sdk"
)

type memDoc struct {
	mu     sync.Mutex
	fields map[string]json.RawMessage
}

func (m *memDoc) Load(context.Context) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return json.Marshal(m.fields)
}

func (m *memDoc) Save(_ context.Context, doc json.RawMessage) error {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(doc, &in); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fields == nil {
		m.fields = map[string]json.RawMessage{}
	}
	for k, v := range in {
		m.fields[k] = v
	}
	return nil
}

type memDir struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (d *memDir) Read(name string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if b, ok := d.files[name]; ok {
		return b, nil
	}
	return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no file")
}

func (d *memDir) Write(name string, data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.files == nil {
		d.files = map[string][]byte{}
	}
	d.files[name] = data
	return nil
}

func (d *memDir) Quarantine(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.files[name+".corrupt"] = d.files[name]
	delete(d.files, name)
	return nil
}

type noGuard struct{}

func (noGuard) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, stacksdk.Errorf(stacksdk.CodeDenied, "refused")
}
func (noGuard) Boundary(netip.Addr) stacksdk.Boundary { return stacksdk.BoundaryLAN }

type noFetch struct{}

func (noFetch) Get(context.Context, stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	return stacksdk.FetchResponse{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "off")
}

type noGateway struct{}

func (noGateway) Default(context.Context) (string, error) {
	return "", stacksdk.Errorf(stacksdk.CodeNotFound, "unknown")
}

type nullLog struct{}

func (nullLog) Log(stacksdk.Level, string, ...string) {}

func host(doc *memDoc, dir *memDir) stacksdk.Host {
	return stacksdk.Host{Config: doc, DataDir: dir, Guard: noGuard{}, Fetch: noFetch{}, Gateway: noGateway{}, Log: nullLog{}}
}

func started(t *testing.T, doc *memDoc, dir *memDir) *Stack {
	t.Helper()
	s := New()
	if err := s.Start(context.Background(), host(doc, dir)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s
}

func call(t *testing.T, s *Stack, method, body string) stacksdk.Response {
	t.Helper()
	resp, err := s.Handle(context.Background(), stacksdk.Request{Method: method, Path: RoutePath, ContentType: "application/json", Body: []byte(body)})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestManifest_IsValidAndTheWriteNeedsTheStepUp(t *testing.T) {
	m := New().Manifest()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(m.Storage) != 1 || m.Storage[0] != "network-dns-checks.json" || m.Settings.Key != "netaudit" {
		t.Errorf("the file and the settings key are the ones from before the move: %+v %+v", m.Storage, m.Settings)
	}
	levels := map[string]stacksdk.Access{}
	for _, r := range m.Routes {
		levels[r.Method] = r.Level()
	}
	if levels[http.MethodGet] != stacksdk.AccessAdmin || levels[http.MethodPut] != stacksdk.AccessStepUp || len(m.Routes) != 2 {
		t.Errorf("routes = %+v", m.Routes)
	}
}

func TestHandle_GetShowsSettingsStatusAndTheSummaryToBringBack(t *testing.T) {
	doc := &memDoc{fields: map[string]json.RawMessage{"dns_check_enabled": json.RawMessage("true"), "dns_check_names": json.RawMessage(`["dns.google"]`), "dns_check_interval_minutes": json.RawMessage("0")}}
	log := `[{"at":"2026-11-01T02:00:00Z","name":"dns.google","outcome":"match","router":["8.8.8.8"],"outside":["8.8.8.8"]},` +
		`{"at":"2026-11-01T03:00:00Z","name":"dns.google","outcome":"differ","router":["1.2.3.4"],"outside":["8.8.8.8"]}]`
	dir := &memDir{files: map[string][]byte{dnscheck.LogFile: []byte(log)}}
	s := started(t, doc, dir)
	resp := call(t, s, http.MethodGet, "")
	if resp.Status != 200 || resp.ContentType != "application/json" {
		t.Fatalf("%d %s", resp.Status, resp.ContentType)
	}
	for _, want := range []string{`"enabled":true`, `"names":["dns.google"]`, `"interval_minutes":60`, `"comparisons":2`, `"differs":1`, `"status":{`, `"summary":{`} {
		if !strings.Contains(string(resp.Body), want) {
			t.Errorf("missing %q in %s", want, resp.Body)
		}
	}
}

func TestHandle_AnUnreadableLogIsAnErrorNotAnEmptySummary(t *testing.T) {
	dir := &memDir{files: map[string][]byte{dnscheck.LogFile: []byte("{not a log")}}
	resp := call(t, started(t, &memDoc{}, dir), http.MethodGet, "")
	if resp.Status != http.StatusInternalServerError || !strings.Contains(string(resp.Body), `"error"`) {
		t.Errorf("%d %s", resp.Status, resp.Body)
	}
	if _, set := dir.files[dnscheck.LogFile+".corrupt"]; !set {
		t.Error("the unreadable log must be set aside so the next read starts empty")
	}
}

func TestHandle_PutChangesOnlyWhatIsSent(t *testing.T) {
	doc := &memDoc{fields: map[string]json.RawMessage{"presence_check": json.RawMessage("true")}}
	s := started(t, doc, &memDir{})
	if resp := call(t, s, http.MethodPut, `{"enabled":true}`); resp.Status != 200 || string(doc.fields["dns_check_enabled"]) != "true" || doc.fields["dns_check_names"] != nil {
		t.Errorf("enabled only: %d %v", resp.Status, doc.fields)
	}
	body := `{"interval_minutes":30,"names":["a.example"],"resolver_url":"https://r.example/q","server":"192.168.1.53"}`
	if resp := call(t, s, http.MethodPut, body); resp.Status != 200 {
		t.Fatalf("%d %s", resp.Status, resp.Body)
	}
	if string(doc.fields["dns_check_interval_minutes"]) != "30" || string(doc.fields["dns_check_names"]) != `["a.example"]` ||
		string(doc.fields["dns_check_resolver_url"]) != `"https://r.example/q"` || string(doc.fields["dns_check_server"]) != `"192.168.1.53"` {
		t.Errorf("document = %v", doc.fields)
	}
	if string(doc.fields["presence_check"]) != "true" {
		t.Error("another module's setting was touched")
	}
}

// A refused setting is a 400 and the switch is not turned on in the same request.
func TestHandle_APutWithABadSettingRefusesAndSwitchesNothingOn(t *testing.T) {
	doc := &memDoc{}
	s := started(t, doc, &memDir{})
	resp := call(t, s, http.MethodPut, `{"enabled":true,"names":["bad name"]}`)
	if resp.Status != http.StatusBadRequest || doc.fields["dns_check_enabled"] != nil {
		t.Errorf("%d %s %v", resp.Status, resp.Body, doc.fields)
	}
	if resp := call(t, s, http.MethodPut, `{not json`); resp.Status != http.StatusBadRequest {
		t.Errorf("bad body: %d", resp.Status)
	}
	r, _ := s.Handle(context.Background(), stacksdk.Request{Method: http.MethodPut, Path: RoutePath, ContentType: "text/plain", Body: []byte(`{}`)})
	if r.Status != http.StatusUnsupportedMediaType {
		t.Errorf("content type: %d", r.Status)
	}
}

func TestHandle_AnotherRouteIsNotFoundAndAStackNotStartedIsUnavailable(t *testing.T) {
	s := started(t, &memDoc{}, &memDir{})
	if _, err := s.Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: "/admin/api/netaudit/other"}); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("code = %q", stacksdk.CodeOf(err))
	}
	if _, err := s.Handle(context.Background(), stacksdk.Request{Method: http.MethodDelete, Path: RoutePath}); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("a method the route does not declare: code = %q", stacksdk.CodeOf(err))
	}
	if _, err := New().Handle(context.Background(), stacksdk.Request{Method: http.MethodGet, Path: RoutePath}); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("not started: code = %q", stacksdk.CodeOf(err))
	}
}

// A switched-on check with no router address known logs a "cannot compare" row and says when it ran: the whole path from the settings
// document through the ports to the stored log.
func TestRun_ASwitchedOnCheckLogsARowAndReportsStatus(t *testing.T) {
	doc := &memDoc{fields: map[string]json.RawMessage{"dns_check_enabled": json.RawMessage("true"), "dns_check_names": json.RawMessage(`["dns.google"]`)}}
	dir := &memDir{}
	s := started(t, doc, dir)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, _ := s.Status(context.Background()); len(st) == 1 && !st[0].LastRun.IsZero() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	st, _ := s.Status(context.Background())
	if len(st) != 1 || st[0].ID != ModuleID || !st[0].Enabled || st[0].Every != time.Hour || st[0].LastRun.IsZero() {
		t.Fatalf("status = %+v", st)
	}
	var rows []dnscheck.Comparison
	data, _ := dir.Read(dnscheck.LogFile)
	if err := json.Unmarshal(data, &rows); err != nil || len(rows) != 1 || rows[0].Outcome != dnscheck.CannotCompare || rows[0].Reason != dnscheck.ReasonNoRouter {
		t.Errorf("log = %s (%v)", data, err)
	}
}

func TestStart_TwiceIsRefusedAndStopIsSafeAnytime(t *testing.T) {
	s := New()
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("stopping a stack that never started: %v", err)
	}
	if st, err := s.Status(context.Background()); st != nil || err != nil {
		t.Errorf("status before start: %v %v", st, err)
	}
	h := host(&memDoc{}, &memDir{})
	if err := s.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), h); err == nil {
		t.Error("a second start must be refused")
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), h); err != nil {
		t.Errorf("start after stop: %v", err)
	}
	_ = s.Stop(context.Background())
}

func TestCallTool_ThereAreNoToolsYet(t *testing.T) {
	if _, err := New().CallTool(context.Background(), "x", nil); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("code = %q", stacksdk.CodeOf(err))
	}
}

type fixedGateway string

func (g fixedGateway) Default(context.Context) (string, error) { return string(g), nil }

// The router's DNS server is the one the operator set, or else the default gateway, read each time.
func TestRouterAddress_IsTheOneSetOrElseTheGateway(t *testing.T) {
	doc := &memDoc{}
	cfg := dnscheck.Config{Doc: doc}
	gateway := netip.MustParseAddr("192.0.2.1")
	ctx := context.Background()
	if got := routerAddress(ctx, cfg, fixedGateway("192.0.2.1")); got != gateway {
		t.Errorf("by default the gateway: %v", got)
	}
	if err := cfg.SetServer(ctx, "192.168.1.53"); err != nil {
		t.Fatal(err)
	}
	if got := routerAddress(ctx, cfg, fixedGateway("192.0.2.1")); got.String() != "192.168.1.53" {
		t.Errorf("the one set: %v", got)
	}
	if err := cfg.SetServer(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if got := routerAddress(ctx, cfg, fixedGateway("192.0.2.1")); got != gateway {
		t.Errorf("empty goes back to the gateway: %v", got)
	}
	if got := routerAddress(ctx, cfg, noGateway{}); got.IsValid() {
		t.Errorf("neither known: %v", got)
	}
	if got := routerAddress(ctx, cfg, fixedGateway("not an address")); got.IsValid() {
		t.Errorf("a gateway that is not an address: %v", got)
	}
}

// ---- findings (ADR-0036) ----

func differLog(now time.Time) *memDir {
	row := func(ago time.Duration, outcome string) string {
		return `{"at":"` + now.Add(-ago).UTC().Format(time.RFC3339) + `","name":"dns.google","outcome":"` + outcome + `","router":["1.2.3.4"],"outside":["8.8.8.8"]}`
	}
	return &memDir{files: map[string][]byte{dnscheck.LogFile: []byte("[" + row(2*time.Hour, "differ") + "," + row(time.Hour, "differ") + "]")}}
}

func findingDoc(fields map[string]string) *memDoc {
	doc := &memDoc{fields: map[string]json.RawMessage{}}
	for k, v := range fields {
		doc.fields[k] = json.RawMessage(v)
	}
	return doc
}

func TestFindings_OffUntilTheOperatorSwitchesTheFindingOn(t *testing.T) {
	doc := findingDoc(map[string]string{"dns_check_enabled": "true", "dns_check_server": `"192.168.50.1"`})
	s := started(t, doc, differLog(time.Now()))
	got, err := s.Findings(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("the finding is off by default: %v %v", got, err)
	}
	if err := doc.Save(context.Background(), json.RawMessage(`{"dns_check_finding_enabled":true}`)); err != nil {
		t.Fatal(err)
	}
	got, err = s.Findings(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("switched on: %v %v", got, err)
	}
	f := got[0]
	if f.ID != "dns_router_differs" || f.Subject != netip.MustParseAddr("192.168.50.1") || f.Count != 2 || len(f.Names) != 1 || f.Names[0] != "dns.google" {
		t.Errorf("finding = %+v", f)
	}
}

func TestFindings_NothingWhenTheCheckIsOffOrTheRouterIsUnknown(t *testing.T) {
	off := findingDoc(map[string]string{"dns_check_finding_enabled": "true", "dns_check_server": `"192.168.50.1"`})
	if got, _ := started(t, off, differLog(time.Now())).Findings(context.Background()); len(got) != 0 {
		t.Errorf("a switched-off check offers nothing: %v", got)
	}
	unknown := findingDoc(map[string]string{"dns_check_enabled": "true", "dns_check_finding_enabled": "true"})
	if got, _ := started(t, unknown, differLog(time.Now())).Findings(context.Background()); len(got) != 0 {
		t.Errorf("with no router address there is no subject: %v", got)
	}
}

func TestManifest_TheDNSModuleDeclaresItsFinding(t *testing.T) {
	m := New().Manifest()
	if len(m.Modules) != 1 || len(m.Modules[0].Finds) != 1 || m.Modules[0].Finds[0] != "dns_router_differs" {
		t.Fatalf("finds = %+v", m.Modules)
	}
	if strings.Contains(m.Modules[0].About.Never, "Raises a finding") {
		t.Errorf("the About text still says it never raises a finding: %q", m.Modules[0].About.Never)
	}
}

// ---- the lying-resolver test, end to end (ADR-0036) ----

// lyingDNS answers every A question over UDP on the loopback with one fixed address, as a router whose resolver has been tampered with
// would. It stands in for the router; the stack's Guard hands the real DNS client a connection to it.
type lyingDNS struct {
	conn   *net.UDPConn
	answer [4]byte
}

func startLyingDNS(t *testing.T, answer netip.Addr) *lyingDNS {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	l := &lyingDNS{conn: conn, answer: answer.As4()}
	t.Cleanup(func() { _ = conn.Close() })
	go l.serve()
	return l
}

const (
	dnsHeaderLen   = 12
	dnsResponseOK  = 0x8180 // a response, recursion desired and available, no error
	dnsTypeA       = 1
	dnsClassIN     = 1
	dnsAnswerTTL   = 60
	dnsNamePointer = 0xc00c // the answer's name points at the question's, at byte 12
)

// serve replies to each question with the question copied back and one A record.
func (l *lyingDNS) serve() {
	buf := make([]byte, 512)
	for {
		n, from, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if n <= dnsHeaderLen {
			continue
		}
		end := questionEnd(buf[:n])
		if end < 0 {
			continue
		}
		// Only the header and the question are echoed: the client's extra EDNS record would sit between them and the answer.
		reply := append([]byte(nil), buf[:end]...)
		binary.BigEndian.PutUint16(reply[2:], dnsResponseOK)
		binary.BigEndian.PutUint16(reply[6:], 1)  // one answer
		binary.BigEndian.PutUint16(reply[8:], 0)  // no authority records
		binary.BigEndian.PutUint16(reply[10:], 0) // no additional records
		reply = binary.BigEndian.AppendUint16(reply, dnsNamePointer)
		reply = binary.BigEndian.AppendUint16(reply, dnsTypeA)
		reply = binary.BigEndian.AppendUint16(reply, dnsClassIN)
		reply = binary.BigEndian.AppendUint32(reply, dnsAnswerTTL)
		reply = binary.BigEndian.AppendUint16(reply, uint16(len(l.answer)))
		reply = append(reply, l.answer[:]...)
		_, _ = l.conn.WriteToUDP(reply, from)
	}
}

// questionEnd is the offset just past the first question of a query, or -1 when the query is cut short.
func questionEnd(q []byte) int {
	const typeAndClassLen = 4
	i := dnsHeaderLen
	for i < len(q) && q[i] != 0 {
		i += int(q[i]) + 1
	}
	if end := i + 1 + typeAndClassLen; i < len(q) && end <= len(q) {
		return end
	}
	return -1
}

// toLoopback is a Guard that lets the router lookup through, to the stand-in router instead of port 53 of the real address.
type toLoopback struct{ to net.Addr }

func (g toLoopback) DialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, g.to.String())
}
func (toLoopback) Boundary(netip.Addr) stacksdk.Boundary { return stacksdk.BoundaryLAN }

// honestOutside answers the DNS-over-HTTPS question with the real, fixed address of dns.google.
type honestOutside struct{}

func (honestOutside) Get(context.Context, stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	body := `{"Status":0,"Answer":[{"name":"dns.google.","type":1,"TTL":60,"data":"8.8.8.8"}]}`
	return stacksdk.FetchResponse{Body: []byte(body)}, nil
}

// A router that keeps giving another address for a name the outside resolver knows is found by the real collector (the real Router
// lookup over UDP, the real comparison, the stored log) on two checks in a row, and only then does the stack offer the finding.
func TestFindings_ALyingRouterIsFoundByTheRealCollectorOnTwoChecksInARow(t *testing.T) {
	fake := startLyingDNS(t, netip.MustParseAddr("203.0.113.9"))
	doc := findingDoc(map[string]string{
		"dns_check_enabled": "true", "dns_check_finding_enabled": "true",
		"dns_check_server": `"192.168.50.1"`, "dns_check_names": `["dns.google"]`,
	})
	dir := &memDir{}
	h := host(doc, dir)
	h.Guard, h.Fetch = toLoopback{to: fake.conn.LocalAddr()}, honestOutside{}
	s := New()
	if err := s.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })

	// Start runs one cycle at once; wait for it so the checks counted below are the ones this test asks for.
	waitForLog(t, dir, 1)
	if got, err := s.Findings(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("one differing check is a blip, not a finding: %v %v", got, err)
	}
	s.collector.Tick(context.Background())
	got, err := s.Findings(context.Background())
	if err != nil || len(got) != 1 {
		data, _ := dir.Read(dnscheck.LogFile)
		t.Fatalf("two checks in a row: %v %v; log %s", got, err, data)
	}
	f := got[0]
	if f.ID != FindingID || f.Subject != netip.MustParseAddr("192.168.50.1") || f.Count != 2 || len(f.Names) != 1 || f.Names[0] != "dns.google" {
		t.Errorf("finding = %+v", f)
	}

	// The same stack against an honest router offers nothing: the finding needs a real disagreement.
	honest := startLyingDNS(t, netip.MustParseAddr("8.8.8.8"))
	h2 := host(doc, &memDir{})
	h2.Guard, h2.Fetch = toLoopback{to: honest.conn.LocalAddr()}, honestOutside{}
	s2 := New()
	if err := s2.Start(context.Background(), h2); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s2.Stop(context.Background()) })
	s2.collector.Tick(context.Background())
	s2.collector.Tick(context.Background())
	if got, err := s2.Findings(context.Background()); err != nil || len(got) != 0 {
		t.Errorf("an honest router: %v %v", got, err)
	}
}

// waitForLog waits until the stored log holds at least n rows.
func waitForLog(t *testing.T, dir *memDir, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var rows []dnscheck.Comparison
		if data, err := dir.Read(dnscheck.LogFile); err == nil && json.Unmarshal(data, &rows) == nil && len(rows) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the log never reached %d rows", n)
}
