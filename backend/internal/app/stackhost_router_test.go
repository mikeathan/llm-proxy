package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/network"
	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/stackport"
	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

// routerFixture reads one of the owner's real router captures, which live with the stack's sensor.
func routerFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../../stacks/network/upnp/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// fakeRouter answers the discovery datagram with the real answer (its LOCATION names 192.168.50.1:37027), serves the description page and answers the
// control address with one forwarded port (2222 to 22 on 192.168.50.77) and the internet address 203.0.113.7.
func fakeRouter(t *testing.T) (udp, web string) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if strings.HasPrefix(string(buf[:n]), "M-SEARCH * HTTP/1.1") {
				_, _ = pc.WriteTo([]byte(routerFixture(t, "ssdp_asus_response.txt")), from)
			}
		}
	}()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rootDesc.xml":
			_, _ = w.Write([]byte(routerFixture(t, "rootdesc_asus.xml")))
		case strings.Contains(r.Header.Get("SOAPAction"), "GetExternalIPAddress"):
			_, _ = w.Write([]byte(routerFixture(t, "soap_external_ip.xml")))
		case strings.Contains(string(body), "<NewPortMappingIndex>0<"):
			_, _ = w.Write([]byte(routerFixture(t, "soap_entry_from_spec.xml")))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(routerFixture(t, "soap_no_more_entries.xml")))
		}
	}))
	t.Cleanup(srv.Close)
	return pc.LocalAddr().String(), srv.Listener.Addr().String()
}

// routerHost hosts the real Network stack over the real declared scan scope and a scoped dial that connects every permitted datagram and page
// request to the stand-in router. dialed records every address the stack asked to connect to.
func routerHost(t *testing.T, ranges []string, udp, web string) (*stackHost, func() []string) {
	t.Helper()
	store := &memPolicy{cfg: models.NetauditConfig{ScanRanges: ranges}}
	scope := netaudit.NewScope(store, net.DefaultResolver, nil)
	var mu sync.Mutex
	var asked []string
	toRouter := func(ctx context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		asked = append(asked, network+" "+addr)
		mu.Unlock()
		target := web
		if network == "udp" {
			target = udp
		}
		var d net.Dialer
		return d.DialContext(ctx, network, target)
	}
	st := networkStack()
	h, err := newStackHost(st, stacksdk.Host{
		Config:      stackport.Config[models.NetauditConfig]{Store: store},
		Log:         nopLog{},
		DataDir:     stackport.DataDir{Dir: t.TempDir(), Files: st.Manifest().Storage},
		Guard:       stackport.Guard{Dial: func(context.Context, string, string) (net.Conn, error) { return nil, tools.ErrLANBlocked }},
		Fetch:       stackport.Fetch{Opener: offOpener{}},
		Gateway:     stackport.Gateway{Read: func() netip.Addr { return netip.Addr{} }},
		ScopedGuard: stackport.ScopedGuard{Check: func(a netip.Addr) error { return scope.Policy().Check(a) }, Dial: toRouter},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.Run(ctx)
	t.Cleanup(cancel)
	return h, func() []string { mu.Lock(); defer mu.Unlock(); return slices.Clone(asked) }
}

var routerAddr = netip.MustParseAddr("192.168.50.1")

// fixedPorts is the outside scan database: it lists two ports as open, one the router forwards through UPnP and one nothing accounts for.
type fixedPorts struct{}

func (fixedPorts) OpenPorts(context.Context, netip.Addr) ([]int, error) {
	return []int{2222, 9999}, nil
}

// Parity (ADR-0038 rule 1): the owner's real router, answered through the whole new path (the stack, the host, the adapter and the core's check),
// becomes exactly the findings the core made when it ran the probe itself, in every field, so no acknowledgement or fix keyed on them is lost on
// the first night. The expected findings are what the code before the move produced for the same router (run from the committed tree against the
// same fixtures), including both wordings of the outside look's finding: a port UPnP forwards and one nothing accounts for.
func TestNetworkStack_ARouterThroughTheSensorIsTheSameFindingsAsBefore(t *testing.T) {
	udp, web := fakeRouter(t)
	h, _ := routerHost(t, []string{"192.168.50.0/24"}, udp, web)
	exposure := netaudit.NewExposure(func() bool { return true }, fixedPorts{}, time.Now)
	check := netaudit.NewRouterCheck(stackRouterProber{host: h, module: networkRouterModule},
		func() (netip.Addr, error) { return routerAddr, nil }, func(netip.Addr) error { return nil }).WithExposure(exposure)
	got, err := check.Findings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []netaudit.Finding{
		{Device: "192.168.50.1", Check: netaudit.CheckUPnPEnabled, Severity: netaudit.SeverityLow,
			Evidence: "The router offers UPnP, which lets any device on the network open a port to the internet by itself, without asking you. Malware and some apps use this to make a device reachable from outside"},
		{Device: "192.168.50.77", Port: 22, Proto: "tcp", Check: netaudit.CheckUPnPForward, Severity: netaudit.SeverityHigh,
			Evidence: "The router forwards port 2222 from the internet to this device's port 22 (tcp) because something asked it to through UPnP. Anyone on the internet can try to connect to it"},
		{Device: "192.168.50.1", Port: 2222, Check: netaudit.CheckInternetExposedPort, Severity: netaudit.SeverityMedium,
			Evidence: "An outside scan database lists port 2222 as open on this network's public address. Your router forwards this port because something asked it to through UPnP. Anyone on the internet can try to connect to it. The database's information can be up to a week old"},
		{Device: "192.168.50.1", Port: 9999, Check: netaudit.CheckInternetExposedPort, Severity: netaudit.SeverityMedium,
			Evidence: "An outside scan database lists port 9999 as open on this network's public address. No UPnP mapping on your router accounts for it: check the router's own port-forwarding list and any device in front of the router. Anyone on the internet can try to connect to it. The database's information can be up to a week old"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findings = %+v\nwant     %+v", got, want)
	}
}

// The router's own statement of its internet address reaches the outside lookup through the sensor, and only when the lookup asked for it.
func TestNetworkStack_TheRoutersInternetAddressComesOnlyWhenAskedFor(t *testing.T) {
	udp, web := fakeRouter(t)
	h, _ := routerHost(t, []string{"192.168.50.0/24"}, udp, web)
	prober := stackRouterProber{host: h, module: networkRouterModule}
	asked, err := prober.Probe(context.Background(), routerAddr, true)
	if err != nil || asked.ExternalAddr != netip.MustParseAddr("203.0.113.7") || !asked.HasService {
		t.Fatalf("asked: %+v, %v", asked, err)
	}
	quiet, err := prober.Probe(context.Background(), routerAddr, false)
	if err != nil || quiet.ExternalAddr.IsValid() || !quiet.Gateway {
		t.Fatalf("not asked: %+v, %v", quiet, err)
	}
}

// ADR-0038 rule 2: even if the core's own pre-check were bypassed, the scoped dial refuses a router outside the declared scan scope at the moment
// of connecting. Nothing is dialled, and the report says it could not look: it is never read as "UPnP is off".
func TestNetworkStack_TheRouterSensorNeverDialsAnAddressOutsideTheDeclaredScope(t *testing.T) {
	udp, web := fakeRouter(t)
	h, dialed := routerHost(t, []string{"192.168.1.0/24"}, udp, web)
	_, err := stackRouterProber{host: h, module: networkRouterModule}.Probe(context.Background(), routerAddr, true)
	if err == nil || !strings.Contains(err.Error(), "could not reach the router") {
		t.Fatalf("a refused router is an error the report states, got %v", err)
	}
	if got := dialed(); len(got) != 0 {
		t.Fatalf("an address outside the declared scope was dialled: %v", got)
	}
}

// The stack's problem ids and the core's words are one list: an id with no words would be said as "could not be read", and a word with no id
// could never be used.
func TestNetworkStack_TheRouterProblemIdsAreTheOnesTheCoreHasWordsFor(t *testing.T) {
	stack, core := network.RouterProblems(), netaudit.RouterProblemIDs()
	slices.Sort(stack)
	slices.Sort(core)
	if !slices.Equal(stack, core) {
		t.Errorf("stack ids %v, core ids %v", stack, core)
	}
}

// routing is a router sensor with a canned answer.
type routing struct {
	sensing
	got    stacksdk.RouterRequest
	result stacksdk.RouterResult
	err    error
	wait   time.Duration
}

func (r *routing) LookRouter(ctx context.Context, req stacksdk.RouterRequest) (stacksdk.RouterResult, error) {
	r.got = req
	select {
	case <-time.After(r.wait):
	case <-ctx.Done():
		return stacksdk.RouterResult{}, ctx.Err()
	}
	return r.result, r.err
}

func routingHost(t *testing.T, r *routing) *stackHost {
	t.Helper()
	r.manifest = baseManifest(stacksdk.PortLog, stacksdk.PortScopedGuard)
	r.manifest.Looks = []string{networkRouterModule}
	h, err := newStackHost(r, stacksdk.Host{Log: nopLog{}, ScopedGuard: stackport.ScopedGuard{Check: func(netip.Addr) error { return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// ADR-0038 rule 3: the core checks what crosses the port. A mapping with a port out of range, another protocol or a client that is not IPv4 is
// dropped, the list is bounded, an address is kept only when asked for and public-shaped IPv4, and a problem id the core does not know is said plainly.
func TestStackRouterProber_TheCoreChecksWhatCrossesThePort(t *testing.T) {
	good := stacksdk.RouterMapping{ExternalPort: 80, InternalPort: 8080, Protocol: "tcp", Client: netip.MustParseAddr("192.168.50.7"), Enabled: true}
	bad := []stacksdk.RouterMapping{
		{ExternalPort: 0, InternalPort: 80, Protocol: "tcp", Client: good.Client},
		{ExternalPort: 80, InternalPort: 80, Protocol: "<b>x</b>", Client: good.Client},
		{ExternalPort: 80, InternalPort: 80, Protocol: "tcp", Client: netip.MustParseAddr("fe80::1")},
	}
	many := make([]stacksdk.RouterMapping, 0, netaudit.MaxRouterMappings+10)
	for range netaudit.MaxRouterMappings + 10 {
		many = append(many, good)
	}
	r := &routing{result: stacksdk.RouterResult{Answered: true, Gateway: true, HasService: true, Mappings: append(bad, many...),
		MappingsProblem: "<script>", ExternalAddr: netip.MustParseAddr("203.0.113.7")}}
	got, err := stackRouterProber{host: routingHost(t, r), module: networkRouterModule}.Probe(context.Background(), routerAddr, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Mappings) != netaudit.MaxRouterMappings || got.Mappings[0].InternalPort != 8080 {
		t.Errorf("mappings = %d, first %+v", len(got.Mappings), got.Mappings[0])
	}
	if got.ExternalAddr.IsValid() {
		t.Error("an address nobody asked for is dropped")
	}
	if got.MappingsErr != "the router could not be read" || strings.Contains(got.MappingsErr, "script") {
		t.Errorf("an id the core does not know must not be echoed: %q", got.MappingsErr)
	}
	r.result = stacksdk.RouterResult{Problem: "not_router"}
	if _, err := (stackRouterProber{host: routingHost(t, r), module: networkRouterModule}).Probe(context.Background(), routerAddr, false); err == nil || err.Error() != netaudit.RouterProblemWords("not_router") {
		t.Errorf("a problem is an error in the core's words, got %v", err)
	}
	if _, err := (stackRouterProber{}).Probe(context.Background(), routerAddr, false); err == nil {
		t.Error("an unhosted stack cannot look")
	}
}

func TestStackHost_LookRouterGivesUpOnASlowSensorAndRefusesAStackThatIsNoSensor(t *testing.T) {
	old := routerLookTimeout
	routerLookTimeout = 20 * time.Millisecond
	t.Cleanup(func() { routerLookTimeout = old })
	req := stacksdk.RouterRequest{Module: networkRouterModule, Gateway: routerAddr}
	if _, err := routingHost(t, &routing{wait: time.Second}).LookRouter(context.Background(), req); err == nil {
		t.Error("a slow sensor must time out")
	}
	if _, err := routingHost(t, &routing{}).LookRouter(context.Background(), stacksdk.RouterRequest{Module: "other", Gateway: routerAddr}); err == nil {
		t.Error("a module the manifest does not list must be refused")
	}
	if _, err := offeringHost(t, offering{}).LookRouter(context.Background(), req); err == nil {
		t.Error("a stack that is no router sensor cannot be asked")
	}
}
