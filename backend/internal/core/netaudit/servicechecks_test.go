package netaudit

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
)

func serverHost(t *testing.T, srv *httptest.Server) (Host, int) {
	t.Helper()
	ap := netip.MustParseAddrPort(strings.TrimPrefix(srv.URL, "http://"))
	return Host{Addr: ap.Addr(), Up: true, Ports: []Port{{Number: int(ap.Port()), Proto: "tcp", State: "open"}}}, int(ap.Port())
}

func checks(probe serviceProbe) *ServiceChecks {
	return &ServiceChecks{deps: ServiceDeps{HTTP: http.DefaultClient, Dial: (&net.Dialer{}).DialContext}, probes: []serviceProbe{probe}}
}

func TestServiceCheck_OllamaOpenProtectedAndUnreadable(t *testing.T) {
	status, body := http.StatusOK, `{"models":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("asked for %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("a credential was sent")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	host, port := serverHost(t, srv)
	sc := checks(httpProbe("Ollama", CheckAIAPIExposed, SeverityHigh, []int{port}, "/api/tags", jsonHasKey("models"), "words"))

	if got := sc.Findings(context.Background(), []Host{host}); len(got) != 1 || got[0].Check != CheckAIAPIExposed || got[0].Evidence != "words" {
		t.Fatalf("open: %+v", got)
	}
	status = http.StatusUnauthorized
	if got := sc.Findings(context.Background(), []Host{host}); len(got) != 0 || len(sc.unknown) != 0 {
		t.Fatalf("protected is neither a finding nor unknown: %+v %v", got, sc.unknown)
	}
	status, body = http.StatusOK, `<html>some other service</html>`
	if got := sc.Findings(context.Background(), []Host{host}); len(got) != 0 || len(sc.unknown) != 1 {
		t.Fatalf("an unreadable answer is unknown, not a finding: %+v %v", got, sc.unknown)
	}
}

func TestServiceCheck_AFindingIsKeptThroughAnUnreadableNightAndGoesWhenProtected(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	defer srv.Close()
	host, port := serverHost(t, srv)
	sc := checks(httpProbe("x", CheckAIAPIExposed, SeverityHigh, []int{port}, "/", func([]byte) bool { return true }, "w"))
	earlier := sc.Findings(context.Background(), []Host{host})
	status = http.StatusInternalServerError
	sc.Findings(context.Background(), []Host{host})
	if kept := sc.Carry(earlier); len(kept) != 1 {
		t.Fatalf("an unknown answer must keep the earlier finding: %+v", kept)
	}
	status = http.StatusForbidden
	sc.Findings(context.Background(), []Host{host})
	if kept := sc.Carry(earlier); len(kept) != 0 {
		t.Fatalf("a protected answer resolves it: %+v", kept)
	}
}

func TestServiceCheck_ARedirectIsNotFollowed(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("followed a redirect") }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer srv.Close()
	host, port := serverHost(t, srv)
	sc := checks(httpProbe("x", CheckAIAPIExposed, SeverityHigh, []int{port}, "/", func([]byte) bool { return true }, "w"))
	if got := sc.Findings(context.Background(), []Host{host}); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestServiceCheck_OnlyOpenPortsOfTheTableAreProbed(t *testing.T) {
	asked := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked = true }))
	defer srv.Close()
	host, port := serverHost(t, srv)
	sc := checks(httpProbe("x", CheckAIAPIExposed, SeverityHigh, []int{port + 1}, "/", func([]byte) bool { return true }, "w"))
	sc.Findings(context.Background(), []Host{host})
	host.Ports[0].State = "closed"
	sc.probes[0].ports = []int{port}
	sc.Findings(context.Background(), []Host{host})
	if asked {
		t.Fatal("probed a port that is not in the table or not open")
	}
}

func TestServiceChecks_TheBodyReadersAreStrict(t *testing.T) {
	if jsonHasKey("data")([]byte(`[1]`)) || !jsonHasKey("data")([]byte(`{"data":[]}`)) {
		t.Error("jsonHasKey")
	}
	if !bodyIsBraces([]byte(" {}\n")) || bodyIsBraces([]byte(`{"errors":[]}`)) {
		t.Error("bodyIsBraces")
	}
	if !bodyIsQuotedString([]byte(`"10.0.0.1:8300"`)) || bodyIsQuotedString([]byte(`{}`)) {
		t.Error("bodyIsQuotedString")
	}
}

// fakeRTSP answers OPTIONS and DESCRIBE with the given status codes.
func fakeRTSP(t *testing.T, options, describe string) (Host, *ServiceChecks, *[]string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var seen []string
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 1024)
		for _, code := range []string{options, describe} {
			n, err := c.Read(buf)
			if err != nil {
				return
			}
			seen = append(seen, string(buf[:n]))
			_, _ = c.Write([]byte("RTSP/1.0 " + code + "\r\nCSeq: 1\r\n\r\n"))
		}
	}()
	ap := netip.MustParseAddrPort(ln.Addr().String())
	p := serviceProbe{name: "RTSP", check: CheckRTSPNoAuth, severity: SeverityHigh, ports: []int{int(ap.Port())}, ask: askRTSP, words: "w"}
	return Host{Addr: ap.Addr(), Ports: []Port{{Number: int(ap.Port()), State: "open"}}}, checks(p), &seen
}

func TestServiceCheck_RTSPReadableWithoutAPasswordIsAFinding(t *testing.T) {
	host, sc, seen := fakeRTSP(t, "200 OK", "200 OK")
	got := sc.Findings(context.Background(), []Host{host})
	if len(got) != 1 || got[0].Check != CheckRTSPNoAuth {
		t.Fatalf("%+v", got)
	}
	if len(*seen) != 2 || !strings.HasPrefix((*seen)[0], "OPTIONS rtsp://") || !strings.HasPrefix((*seen)[1], "DESCRIBE rtsp://") || strings.Contains(strings.ToLower(strings.Join(*seen, "")), "authorization") {
		t.Fatalf("the requests were %q", *seen)
	}
}

func TestServiceCheck_RTSPWithALoginOrANeededPathIsNotAFinding(t *testing.T) {
	for _, tc := range []struct {
		opt, desc string
		unknown   bool
	}{
		{"200 OK", "401 Unauthorized", false}, {"401 Unauthorized", "401 Unauthorized", false}, {"200 OK", "404 Not Found", true},
	} {
		host, sc, _ := fakeRTSP(t, tc.opt, tc.desc)
		got := sc.Findings(context.Background(), []Host{host})
		if len(got) != 0 || (len(sc.unknown) == 1) != tc.unknown {
			t.Errorf("%v: %+v unknown=%v", tc, got, sc.unknown)
		}
	}
}

func TestServiceCheck_ANonRTSPAnswerIsUnknown(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			_, _ = c.Write([]byte("SSH-2.0-x\r\n"))
			c.Close()
		}
	}()
	ap := netip.MustParseAddrPort(ln.Addr().String())
	sc := checks(serviceProbe{check: CheckRTSPNoAuth, ports: []int{int(ap.Port())}, ask: askRTSP})
	if got := sc.Findings(context.Background(), []Host{{Addr: ap.Addr(), Ports: []Port{{Number: int(ap.Port()), State: "open"}}}}); len(got) != 0 || len(sc.unknown) != 1 {
		t.Fatalf("%+v %v", got, sc.unknown)
	}
}

func TestServiceProbePorts_AreInTheScanLists(t *testing.T) {
	for _, p := range ServiceProbePorts() {
		if !slicesContains(DefaultPorts, p) || !slicesContains(ThoroughPorts, p) {
			t.Errorf("port %s is checked but not scanned by default or thoroughly", strconv.Itoa(p))
		}
	}
}

func TestServiceCheck_ThePortCapHolds(t *testing.T) {
	sc := checks(serviceProbe{check: CheckAIAPIExposed, ports: []int{1}, ask: func(context.Context, ServiceDeps, netip.Addr, int) verdict { return verdictUnknown }})
	var hosts []Host
	for i := range 200 {
		hosts = append(hosts, Host{Addr: netip.AddrFrom4([4]byte{10, 0, 0, byte(i)}), Ports: []Port{{Number: 1, State: "open"}}})
	}
	sc.Findings(context.Background(), hosts)
	if len(sc.unknown) != maxServiceProbes {
		t.Fatalf("probed %d", len(sc.unknown))
	}
}

func slicesContains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
