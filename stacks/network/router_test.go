package network

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// routerDevice is a stand-in router: it answers the discovery datagram with the owner's real router answer (its LOCATION names 192.168.50.1:37027)
// and serves the description page and the control address over HTTP. routerGuard sends every connection to it, as the scoped dial would.
type routerDevice struct {
	udp     string
	web     string
	mapping int // how many forwarded ports the control address lists before the end-of-list answer
}

func fixtureOf(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("upnp/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func startRouterDevice(t *testing.T, answerSSDP bool, entries int) routerDevice {
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
			if answerSSDP && strings.HasPrefix(string(buf[:n]), "M-SEARCH * HTTP/1.1") {
				_, _ = pc.WriteTo([]byte(fixtureOf(t, "ssdp_asus_response.txt")), from)
			}
		}
	}()
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rootDesc.xml":
			_, _ = w.Write([]byte(fixtureOf(t, "rootdesc_asus.xml")))
		case r.Method == http.MethodPost && r.URL.Path == "/ctl/IPConn":
			body, _ := io.ReadAll(r.Body)
			switch {
			case strings.Contains(r.Header.Get("SOAPAction"), "GetExternalIPAddress"):
				_, _ = w.Write([]byte(fixtureOf(t, "soap_external_ip.xml")))
			case strings.Contains(string(body), "<NewPortMappingIndex>"+itoa(entries)+"</NewPortMappingIndex>") || entries == 0:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(fixtureOf(t, "soap_no_more_entries.xml")))
			default:
				_, _ = w.Write([]byte(fixtureOf(t, "soap_entry_from_spec.xml")))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(web.Close)
	return routerDevice{udp: pc.LocalAddr().String(), web: web.Listener.Addr().String(), mapping: entries}
}

// routerGuard is a scoped dial that connects every datagram and every page request to the stand-in router, and records what was asked for.
type routerGuard struct {
	mu     sync.Mutex
	dialed []string
	dev    routerDevice
}

func (g *routerGuard) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	g.mu.Lock()
	g.dialed = append(g.dialed, network+" "+addr)
	g.mu.Unlock()
	target := g.dev.web
	if network == "udp" {
		target = g.dev.udp
	}
	var d net.Dialer
	return d.DialContext(ctx, network, target)
}
func (*routerGuard) Boundary(netip.Addr) stacksdk.Boundary { return stacksdk.BoundaryLAN }

func routerStack(t *testing.T, g stacksdk.Guard) *Stack {
	t.Helper()
	h := host(&memDoc{}, &memDir{})
	h.ScopedGuard, h.Guard = g, failingGuard{t}
	s := New()
	s.upnpWait = 2 * time.Second
	if err := s.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s
}

var testRouter = netip.MustParseAddr("192.168.50.1")

func TestLookRouter_ReportsTheRoutersFactsThroughTheScopedDialAndNeverThePlainGuard(t *testing.T) {
	g := &routerGuard{dev: startRouterDevice(t, true, 2)}
	s := routerStack(t, g)
	got, err := s.LookRouter(context.Background(), stacksdk.RouterRequest{Module: RouterModuleID, Gateway: testRouter, WantAddress: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Answered || !got.Gateway || !got.HasService || got.Problem != "" || got.MappingsProblem != "" {
		t.Fatalf("result = %+v", got)
	}
	if len(got.Mappings) != 2 || got.Mappings[0] != (stacksdk.RouterMapping{ExternalPort: 2222, InternalPort: 22, Protocol: "tcp", Client: netip.MustParseAddr("192.168.50.77"), Enabled: true}) {
		t.Errorf("mappings = %+v", got.Mappings)
	}
	if got.ExternalAddr != netip.MustParseAddr("203.0.113.7") {
		t.Errorf("external address = %v", got.ExternalAddr)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.dialed) < 3 || g.dialed[0] != "udp 192.168.50.1:1900" {
		t.Errorf("every connection goes through the scoped dial, the datagram first: %v", g.dialed)
	}
	for _, d := range g.dialed[1:] {
		if d != "tcp 192.168.50.1:37027" {
			t.Errorf("a connection to somewhere other than the router's own page address: %q", d)
		}
	}
}

func TestLookRouter_TheInternetAddressIsReadOnlyWhenAskedFor(t *testing.T) {
	g := &routerGuard{dev: startRouterDevice(t, true, 0)}
	got, err := routerStack(t, g).LookRouter(context.Background(), stacksdk.RouterRequest{Module: RouterModuleID, Gateway: testRouter})
	if err != nil || !got.Gateway || got.ExternalAddr.IsValid() {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestLookRouter_NoAnswerIsAResultNotAnError(t *testing.T) {
	g := &routerGuard{dev: startRouterDevice(t, false, 0)}
	s := routerStack(t, g)
	s.upnpWait = 150 * time.Millisecond
	got, err := s.LookRouter(context.Background(), stacksdk.RouterRequest{Module: RouterModuleID, Gateway: testRouter})
	if err != nil || got.Answered || got.Problem != "" {
		t.Errorf("a router with UPnP off says nothing: %+v, %v", got, err)
	}
}

type refusingScoped struct{}

func (refusingScoped) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, stacksdk.Errorf(stacksdk.CodeDenied, "outside the declared scan scope")
}
func (refusingScoped) Boundary(netip.Addr) stacksdk.Boundary { return stacksdk.BoundaryBlocked }

func TestLookRouter_ARefusedAddressIsAProblemIdAndNeverNothingFound(t *testing.T) {
	got, err := routerStack(t, refusingScoped{}).LookRouter(context.Background(), stacksdk.RouterRequest{Module: RouterModuleID, Gateway: netip.MustParseAddr("203.0.113.1")})
	if err != nil || got.Answered || got.Problem != "unreachable" {
		t.Errorf("a refusal must read as 'could not look', got %+v, %v", got, err)
	}
}

func TestLookRouter_RefusesAnotherModuleNoAddressAndAStackWithNoScopedDial(t *testing.T) {
	g := &routerGuard{dev: startRouterDevice(t, true, 0)}
	s := routerStack(t, g)
	if _, err := s.LookRouter(context.Background(), stacksdk.RouterRequest{Module: UDPModuleID, Gateway: testRouter}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("another module: code = %q", stacksdk.CodeOf(err))
	}
	if _, err := s.LookRouter(context.Background(), stacksdk.RouterRequest{Module: RouterModuleID}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("no address: code = %q", stacksdk.CodeOf(err))
	}
	g.mu.Lock()
	dialed := len(g.dialed)
	g.mu.Unlock()
	if dialed != 0 {
		t.Errorf("a refused request must send nothing, dialled %d", dialed)
	}
	if _, err := New().LookRouter(context.Background(), stacksdk.RouterRequest{Module: RouterModuleID, Gateway: testRouter}); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("not started: code = %q", stacksdk.CodeOf(err))
	}
}

func TestManifest_TheRouterSensorIsDeclaredAndMatchesTheLimit(t *testing.T) {
	m := New().Manifest()
	if !strings.Contains(strings.Join(m.Looks, ","), RouterModuleID) {
		t.Fatalf("looks = %v", m.Looks)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	var _ stacksdk.RouterLooker = New()
	if stacksdk.MaxRouterMappings != 64 {
		t.Error("the SDK bound is the stack's list bound")
	}
}
