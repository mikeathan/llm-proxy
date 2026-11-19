package mcpclient

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"llm-proxy-stacks/sdk"
)

func TestPool_EveryPageOfTheToolListIsRead(t *testing.T) {
	f := startStreamable(t, nil, server.WithPaginationLimit(2))
	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("files", f.URL)}, true)
	settled(t, p)
	if got := len(p.Tools()); got != 4 {
		t.Fatalf("with two tools to a page all four must be listed, got %d", got)
	}
}

// guardedOut is an HTTP port whose guard refuses every address, as the core's does for one the owner has not allowed.
type guardedOut struct{}

func (guardedOut) Client(stacksdk.HTTPOptions) (*http.Client, error) {
	return &http.Client{Transport: &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return nil, stacksdk.Errorf(stacksdk.CodeDenied, "refused")
	}}}, nil
}

func TestPool_ARefusalByTheGuardIsSaidAsSuch(t *testing.T) {
	f := startStreamable(t, nil)
	p := newTestPool(t, guardedOut{})
	p.Reconcile([]Server{entry("files", f.URL)}, true)
	settled(t, p)
	if v := byName(p.Views(), "files"); v.ProblemCode != string(ProblemGuard) {
		t.Fatalf("view: %+v", v)
	}
	if len(f.Auths()) != 0 {
		t.Error("a refused address is never reached")
	}
}

func TestPool_ACertificateAuthorityFileMakesATLSServerTrusted(t *testing.T) {
	f := &fixture{seen: map[string]bool{}, dead: map[string]bool{}}
	f.mcp = newMCPServer(f)
	tls := httptest.NewTLSServer(server.NewStreamableHTTPServer(f.mcp))
	t.Cleanup(tls.Close)
	url := tls.URL + "/mcp"

	p := newTestPool(t, testHTTP{})
	p.Reconcile([]Server{entry("secure", url)}, true)
	settled(t, p)
	if v := byName(p.Views(), "secure"); v.ProblemCode != string(ProblemTLS) {
		t.Fatalf("an untrusted certificate is said so: %+v", v)
	}

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	block := &pem.Block{Type: "CERTIFICATE", Bytes: tls.Certificate().Raw}
	if err := os.WriteFile(caFile, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := x509.ParseCertificate(tls.Certificate().Raw); err != nil {
		t.Fatal(err)
	}
	s := entry("secure", url)
	s.CACert = caFile
	p.Reconcile([]Server{s}, true)
	settled(t, p)
	if v := byName(p.Views(), "secure"); v.Status != string(StatusReady) {
		t.Fatalf("with the authority file the server connects: %+v", v)
	}

	s.CACert = filepath.Join(t.TempDir(), "missing.pem")
	p.Reconcile([]Server{s}, true)
	settled(t, p)
	if v := byName(p.Views(), "secure"); v.ProblemCode != string(ProblemCAFile) {
		t.Fatalf("a missing file is said so: %+v", v)
	}
}
