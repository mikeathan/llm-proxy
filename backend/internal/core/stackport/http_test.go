package stackport

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"llm-proxy-stacks/sdk"
)

func countingDial(n *atomic.Int32) Dial {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		n.Add(1)
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
}

func TestHTTP_EveryConnectionIsDialledThroughTheGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	var dials atomic.Int32
	c, err := HTTP{Dial: countingDial(&dials)}.Client(stacksdk.HTTPOptions{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if dials.Load() == 0 {
		t.Fatal("the request did not go through the guard's dial")
	}
}

func TestHTTP_AGuardRefusalReachesTheCallerAsTheGuardsError(t *testing.T) {
	refuse := Dial(func(context.Context, string, string) (net.Conn, error) {
		return nil, stacksdk.Errorf(stacksdk.CodeDenied, "no")
	})
	c, _ := HTTP{Dial: refuse}.Client(stacksdk.HTTPOptions{})
	_, err := c.Get("http://203.0.113.9/")
	if stacksdk.CodeOf(err) != stacksdk.CodeDenied {
		t.Fatalf("the refusal must stay a CodeDenied error through the transport's wrapping, got %v", err)
	}
}

func TestHTTP_TheEnvironmentsProxyIsNeverUsed(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	var dials atomic.Int32
	c, _ := HTTP{Dial: countingDial(&dials)}.Client(stacksdk.HTTPOptions{})
	if tr, ok := c.Transport.(*http.Transport); !ok || tr.Proxy != nil {
		t.Fatal("the guard must see the real destination, so no proxy may stand in for it")
	}
}

func TestHTTP_ARedirectGoesOnlyToTheSameHost(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("other")) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "/landed", http.StatusFound)
		case "/landed":
			_, _ = w.Write([]byte("landed"))
		default:
			http.Redirect(w, r, other.URL, http.StatusFound)
		}
	}))
	defer srv.Close()
	var dials atomic.Int32
	c, _ := HTTP{Dial: countingDial(&dials)}.Client(stacksdk.HTTPOptions{})
	if resp, err := c.Get(srv.URL + "/same"); err != nil {
		t.Fatalf("a redirect on the same host is followed: %v", err)
	} else {
		resp.Body.Close()
	}
	if _, err := c.Get(srv.URL + "/away"); err == nil {
		t.Fatal("a redirect to another host must be refused")
	}
}

func TestHTTP_ACertificateAuthorityMakesATLSServerTrusted(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("secure")) }))
	defer srv.Close()
	var dials atomic.Int32
	plain, _ := HTTP{Dial: countingDial(&dials)}.Client(stacksdk.HTTPOptions{})
	if _, err := plain.Get(srv.URL); err == nil {
		t.Fatal("an unknown certificate authority must not be trusted")
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	trusting, err := HTTP{Dial: countingDial(&dials)}.Client(stacksdk.HTTPOptions{RootCAPEM: pemBytes})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := trusting.Get(srv.URL)
	if err != nil {
		t.Fatalf("with the authority it is trusted: %v", err)
	}
	resp.Body.Close()
	if tr := trusting.Transport.(*http.Transport); tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Error("TLS 1.2 is the least")
	}
	if _, err := (HTTP{Dial: countingDial(&dials)}).Client(stacksdk.HTTPOptions{RootCAPEM: []byte("not a certificate")}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("a file with no certificate is CodeInvalid: %v", err)
	}
}

func TestHTTP_ThereIsNoOverallTimeoutSoAStreamIsNotCut(t *testing.T) {
	c, _ := HTTP{Dial: countingDial(new(atomic.Int32))}.Client(stacksdk.HTTPOptions{})
	if c.Timeout != 0 {
		t.Fatalf("timeout = %v", c.Timeout)
	}
}
