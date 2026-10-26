package nmap

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/netip"
	"os/exec"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit"
)

// expiredCertListener serves TLS with a certificate that expired a month ago, on a free local port.
func expiredCertListener(t *testing.T) int {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "expired.fixture"},
		NotBefore: time.Now().Add(-90 * 24 * time.Hour), NotAfter: time.Now().Add(-30 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _ = c.(*tls.Conn).Handshake(); c.Close() }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// The whole chain on real nmap output: the exact arguments a checks scan uses, nmap's own XML, the parser and the check
// evaluators. It needs nmap on the machine and skips where there is none.
func TestChecks_RealNmapFindsAnExpiredCertificate(t *testing.T) {
	if testing.Short() {
		t.Skip("starts nmap")
	}
	bin, err := exec.LookPath("nmap")
	if err != nil {
		t.Skip("nmap is not installed")
	}
	port := expiredCertListener(t)
	args, err := Args(netaudit.ScanRequest{
		Targets: []netip.Addr{netip.MustParseAddr("127.0.0.1")}, Mode: netaudit.ModeChecks, Ports: itoa(port), Prefiltered: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		t.Fatalf("nmap: %v", err)
	}
	hosts, err := parseBytes(out)
	if err != nil {
		t.Fatal(err)
	}
	var checks []string
	for _, f := range netaudit.FindingsOf(hosts) {
		if f.Check != netaudit.CheckOpenPort {
			checks = append(checks, f.Check)
		}
	}
	if len(checks) != 1 || checks[0] != netaudit.CheckCertExpired {
		t.Errorf("checks = %v, want only %s", checks, netaudit.CheckCertExpired)
	}
}

func itoa(n int) string { return big.NewInt(int64(n)).String() }

func parseBytes(b []byte) ([]netaudit.Host, error) { return ParseXML(bytes.NewReader(b)) }
