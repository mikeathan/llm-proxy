package upnp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

var router = netip.MustParseAddr("192.168.50.1")

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The answer and the description page are the owner's real router (ASUS RT-AX86U, MiniUPnPd 2.3.11), with the hardware
// address and everything derived from it replaced.
func TestParseSSDP_TheRealRouterAnswer(t *testing.T) {
	loc, err := ParseSSDP(fixture(t, "ssdp_asus_response.txt"), router)
	if err != nil {
		t.Fatal(err)
	}
	if loc != "http://192.168.50.1:37027/rootDesc.xml" {
		t.Errorf("location = %q", loc)
	}
}

func TestParseSSDP_RefusesALocationThatIsNotTheRouter(t *testing.T) {
	answer := func(location string) []byte {
		return []byte("HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\nLOCATION: " + location + "\r\n\r\n")
	}
	for name, location := range map[string]string{
		"another host": "http://192.168.50.77:80/rootDesc.xml",
		"the internet": "http://203.0.113.9/rootDesc.xml",
		"a name":       "http://router.local/rootDesc.xml",
		"https":        "https://192.168.50.1/rootDesc.xml",
		"userinfo":     "http://user@192.168.50.1/rootDesc.xml",
		"no path":      "http://192.168.50.1:80",
		"a credential": "http://192.168.50.1:80/a?token=1",
		"too long":     "http://192.168.50.1/" + strings.Repeat("a", 600),
		"file":         "file:///etc/passwd",
		"empty":        "",
	} {
		if _, err := ParseSSDP(answer(location), router); err == nil {
			t.Errorf("%s: %q must be refused", name, location)
		}
	}
}

func TestParseSSDP_RefusesAnAnswerThatIsNotOK(t *testing.T) {
	for _, raw := range []string{"", "garbage", "HTTP/1.1 404 Not Found\r\nLOCATION: http://192.168.50.1/x\r\n\r\n", "NOTIFY * HTTP/1.1\r\nLOCATION: http://192.168.50.1/x\r\n\r\n"} {
		if _, err := ParseSSDP([]byte(raw), router); err == nil {
			t.Errorf("%q must be refused", raw)
		}
	}
}

func TestParseDescription_TheRealRouterPage(t *testing.T) {
	d, err := ParseDescription(strings.NewReader(string(fixture(t, "rootdesc_asus.xml"))))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Gateway || d.Service != ServiceWANIP || d.ControlPath != "/ctl/IPConn" {
		t.Errorf("description = %+v", d)
	}
}

func TestParseDescription_FindsAPPPConnectionToo(t *testing.T) {
	page := `<root xmlns="urn:schemas-upnp-org:device-1-0"><device><deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType><deviceList><device><deviceList><device><serviceList><service><serviceType>urn:schemas-upnp-org:service:WANPPPConnection:1</serviceType><controlURL>/ctl/PPPConn</controlURL></service></serviceList></device></deviceList></device></deviceList></device></root>`
	d, err := ParseDescription(strings.NewReader(page))
	if err != nil || d.Service != ServiceWANPPP || d.ControlPath != "/ctl/PPPConn" {
		t.Errorf("description = %+v, %v", d, err)
	}
}

func TestParseDescription_ARouterWithoutAnInternetServiceIsStillAGateway(t *testing.T) {
	page := `<root xmlns="urn:schemas-upnp-org:device-1-0"><device><deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType></device></root>`
	d, err := ParseDescription(strings.NewReader(page))
	if err != nil || !d.Gateway || d.Service != "" {
		t.Errorf("description = %+v, %v", d, err)
	}
}

func TestParseDescription_NotAGatewayAndHostileInputs(t *testing.T) {
	media := `<root xmlns="urn:schemas-upnp-org:device-1-0"><device><deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType></device></root>`
	if d, err := ParseDescription(strings.NewReader(media)); err != nil || d.Gateway {
		t.Errorf("a media server is not a gateway: %+v, %v", d, err)
	}
	if _, err := ParseDescription(strings.NewReader("<root><unclosed>")); err == nil {
		t.Error("broken XML must be an error")
	}
	huge := "<root>" + strings.Repeat("<a>x</a>", maxDescriptionBytes) + "</root>"
	if _, err := ParseDescription(strings.NewReader(huge)); err == nil {
		t.Error("an oversized page must be refused")
	}
	// A control URL is a path on the router, never another host.
	evil := `<root xmlns="urn:schemas-upnp-org:device-1-0"><device><deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType><serviceList><service><serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType><controlURL>http://203.0.113.9/ctl</controlURL></service></serviceList></device></root>`
	if d, err := ParseDescription(strings.NewReader(evil)); err != nil || d.Service != "" {
		t.Errorf("an absolute control URL must be ignored: %+v, %v", d, err)
	}
}

// fakeRouter answers one SSDP datagram with the fixture (its LOCATION pointing at a local test server) and serves the
// description page.
func fakeRouter(t *testing.T, serve bool, web http.Handler) (addr netip.Addr, port int, dial Dial) {
	t.Helper()
	return startRouter(t, serve, httptest.NewServer(web))
}

func pageOf(page []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(page) })
}

func startRouter(t *testing.T, serve bool, web *httptest.Server) (addr netip.Addr, port int, dial Dial) {
	t.Helper()
	t.Cleanup(web.Close)
	webPort := web.Listener.Addr().(*net.TCPAddr).Port
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		n, from, err := pc.ReadFrom(buf)
		if err != nil || !serve || !strings.HasPrefix(string(buf[:n]), "M-SEARCH * HTTP/1.1") {
			return
		}
		answer := strings.Replace(string(fixture(t, "ssdp_asus_response.txt")), "192.168.50.1:37027", "127.0.0.1:"+strconv.Itoa(webPort), 1)
		_, _ = pc.WriteTo([]byte(answer), from)
	}()
	udpPort := pc.LocalAddr().(*net.UDPAddr).Port
	d := &net.Dialer{}
	return netip.MustParseAddr("127.0.0.1"), udpPort, func(ctx context.Context, network, address string) (net.Conn, error) {
		return d.DialContext(ctx, network, address)
	}
}

func TestProbe_AnAnsweringRouterIsReportedWithItsInternetService(t *testing.T) {
	addr, port, dial := fakeRouter(t, true, pageOf(fixture(t, "rootdesc_asus.xml")))
	c := Client{Dial: dial, HTTP: http.DefaultClient, SSDPPort: port, Wait: 2 * time.Second}
	got, err := c.Probe(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Answered || !got.Gateway || got.Service != ServiceWANIP {
		t.Errorf("report = %+v", got)
	}
}

func TestProbe_NoAnswerIsNotAnError(t *testing.T) {
	addr, port, dial := fakeRouter(t, false, pageOf(nil))
	got, err := Client{Dial: dial, HTTP: http.DefaultClient, SSDPPort: port, Wait: 150 * time.Millisecond}.Probe(context.Background(), addr)
	if err != nil || got.Answered {
		t.Errorf("a router with UPnP off says nothing: %+v, %v", got, err)
	}
}

func TestProbe_ADialTheGuardRefusesIsAnError(t *testing.T) {
	refuse := func(context.Context, string, string) (net.Conn, error) { return nil, net.ErrClosed }
	if _, err := (Client{Dial: refuse, HTTP: http.DefaultClient, Wait: time.Second}).Probe(context.Background(), router); err == nil {
		t.Error("a refused dial is 'could not check', not 'UPnP is off'")
	}
}

func TestProbe_NeverFollowsARedirectToAnotherHost(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("the redirect target was contacted") }))
	defer other.Close()
	addr, port, dial := fakeRouter(t, true, http.RedirectHandler(other.URL, http.StatusFound))
	c := Client{Dial: dial, HTTP: http.DefaultClient, SSDPPort: port, Wait: 2 * time.Second}
	if got, err := c.Probe(context.Background(), addr); err == nil && got.Gateway {
		t.Errorf("a description that cannot be read must not look like a gateway: %+v", got)
	}
}

func TestParseDescription_KeepsTheFullServiceTypeOfTheRealRouter(t *testing.T) {
	d, err := ParseDescription(strings.NewReader(string(fixture(t, "rootdesc_asus.xml"))))
	if err != nil || d.ServiceType != "urn:schemas-upnp-org:service:WANIPConnection:1" {
		t.Errorf("description = %+v, %v", d, err)
	}
}
