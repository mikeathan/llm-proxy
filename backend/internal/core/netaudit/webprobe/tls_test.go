package webprobe

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// certFor makes a certificate and key. With a nil parent it is signed by itself.
func certFor(t *testing.T, subject string, notAfter time.Time, parent *tls.Certificate) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: subject},
		NotBefore:    notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true, BasicConstraintsValid: true,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	signer, signerCert := key, tmpl
	if parent != nil {
		signer = parent.PrivateKey.(*ecdsa.PrivateKey)
		signerCert = parent.Leaf
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signerCert, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

// tlsServer answers handshakes only, counting the connections it was given, and returns its target.
func tlsServer(t *testing.T, cfg *tls.Config) (Target, *atomic.Int32) {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var conns atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns.Add(1)
			go func() {
				defer c.Close()
				_ = c.(*tls.Conn).Handshake()
			}()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	n, _ := strconv.Atoi(port)
	return Target{Addr: netip.MustParseAddr("127.0.0.1"), Port: n, TLS: true}, &conns
}

func TestProbeTLS_ReadsTheExpiryOfASelfSignedCertificate(t *testing.T) {
	end := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
	target, _ := tlsServer(t, &tls.Config{Certificates: []tls.Certificate{certFor(t, "device", end, nil)}})
	res, err := prober(allowAll).ProbeTLS(context.Background(), []Target{target}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Facts) != 1 {
		t.Fatalf("facts = %d, want 1", len(res.Facts))
	}
	f := res.Facts[0]
	if !f.NotAfter.Equal(end) || f.NotBefore.IsZero() || !f.SelfSigned || len(f.OldVersions) != 0 {
		t.Fatalf("facts = %+v, want expiry %v, self-signed, no old versions", f, end)
	}
}

func TestProbeTLS_ACertificateSignedByAnotherIsNotSelfSigned(t *testing.T) {
	ca := certFor(t, "ca", time.Now().Add(365*24*time.Hour), nil)
	leaf := certFor(t, "device", time.Now().Add(30*24*time.Hour), &ca)
	target, _ := tlsServer(t, &tls.Config{Certificates: []tls.Certificate{leaf}})
	res, err := prober(allowAll).ProbeTLS(context.Background(), []Target{target}, time.Minute)
	if err != nil || len(res.Facts) != 1 {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	if res.Facts[0].SelfSigned {
		t.Fatal("a certificate issued by another was called self-signed")
	}
}

func TestProbeTLS_SaysWhichOldVersionsAreAccepted(t *testing.T) {
	cert := certFor(t, "old", time.Now().Add(100*24*time.Hour), nil)
	target, _ := tlsServer(t, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS11}) // #nosec G402 -- a deliberately old test server
	res, err := prober(allowAll).ProbeTLS(context.Background(), []Target{target}, time.Minute)
	if err != nil || len(res.Facts) != 1 {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	got := res.Facts[0].OldVersions
	if !slices.Contains(got, "TLS 1.0") && !slices.Contains(got, "TLS 1.1") {
		t.Fatalf("old versions = %v, want TLS 1.0 or 1.1", got)
	}
	if res.Facts[0].NotAfter.IsZero() {
		t.Fatal("a server that speaks only an old version still has a certificate to read")
	}
}

func TestProbeTLS_ADeviceThatDoesNotSpeakTLSGivesNoFacts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("HTTP/1.1 400 plain\r\n\r\n"))
			_ = c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	n, _ := strconv.Atoi(port)
	res, err := prober(allowAll).ProbeTLS(context.Background(), []Target{{Addr: netip.MustParseAddr("127.0.0.1"), Port: n, TLS: true}}, time.Minute)
	if err != nil || len(res.Facts) != 0 {
		t.Fatalf("res = %+v err = %v, want no facts and no error", res, err)
	}
}

func TestProbeTLS_ARefusedAddressIsAnErrorBeforeAnythingIsSent(t *testing.T) {
	target, conns := tlsServer(t, &tls.Config{Certificates: []tls.Certificate{certFor(t, "d", time.Now().Add(time.Hour), nil)}})
	refuse := func(netip.Addr) error { return errors.New("outside the scope") }
	if _, err := prober(refuse).ProbeTLS(context.Background(), []Target{target}, time.Minute); err == nil {
		t.Fatal("a refused address was probed")
	}
	if conns.Load() != 0 {
		t.Fatalf("%d connections were opened to a refused address", conns.Load())
	}
}

func TestProbeTLS_TheGuardIsAskedAtEveryConnection(t *testing.T) {
	target, conns := tlsServer(t, &tls.Config{Certificates: []tls.Certificate{certFor(t, "d", time.Now().Add(time.Hour), nil)}})
	var asked atomic.Int32
	// Approves the address when the run starts and refuses it at each connection: the dial itself must still be checked.
	guard := func(netip.Addr) error {
		if asked.Add(1) == 1 {
			return nil
		}
		return errors.New("refused at the connection")
	}
	res, err := prober(guard).ProbeTLS(context.Background(), []Target{target}, time.Minute)
	if err != nil || len(res.Facts) != 0 {
		t.Fatalf("res = %+v err = %v, want no facts", res, err)
	}
	if conns.Load() != 0 {
		t.Fatalf("%d connections got through a guard that refuses at the dial", conns.Load())
	}
}

func TestProbeTLS_CancelledRunReturnsTheContextError(t *testing.T) {
	target, _ := tlsServer(t, &tls.Config{Certificates: []tls.Certificate{certFor(t, "d", time.Now().Add(time.Hour), nil)}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prober(allowAll).ProbeTLS(ctx, []Target{target}, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestProbeTLS_BoundsTheTargetsAndNeedsALimit(t *testing.T) {
	p := prober(allowAll)
	if _, err := p.ProbeTLS(context.Background(), nil, time.Minute); err == nil {
		t.Error("no targets was accepted")
	}
	many := make([]Target, MaxTargets+1)
	for i := range many {
		many[i] = Target{Addr: netip.MustParseAddr("127.0.0.1"), Port: 443, TLS: true}
	}
	if _, err := p.ProbeTLS(context.Background(), many, time.Minute); err == nil {
		t.Error("more than the most targets was accepted")
	}
	if _, err := p.ProbeTLS(context.Background(), many[:1], 0); err == nil {
		t.Error("a run with no time limit was accepted")
	}
}

func TestProbeTLS_OnlyTLSTargetsAreAsked(t *testing.T) {
	target, conns := tlsServer(t, &tls.Config{Certificates: []tls.Certificate{certFor(t, "d", time.Now().Add(time.Hour), nil)}})
	target.TLS = false
	res, err := prober(allowAll).ProbeTLS(context.Background(), []Target{target}, time.Minute)
	if err != nil || len(res.Facts) != 0 {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	if conns.Load() != 0 {
		t.Fatal("a target that does not speak TLS was sent a handshake")
	}
}

// rsaCert is a self-signed RSA certificate, which the RSA key-exchange suites of an old device need.
func rsaCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "old device"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(100 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

// An old embedded server often offers only RSA key exchange with AES-CBC or 3DES, which Go's client leaves out unless asked. A probe
// that did not ask would see every such device fail the handshake, report nothing and so never flag TLS 1.0.
func TestProbeTLS_SeesAnOldDeviceThatOffersOnlyRSAKeyExchange(t *testing.T) {
	for _, suite := range []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA, tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA} {
		cfg := &tls.Config{Certificates: []tls.Certificate{rsaCert(t)}, MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS10, CipherSuites: []uint16{suite}} // #nosec G402 -- a deliberately old test server
		target, _ := tlsServer(t, cfg)
		res, err := prober(allowAll).ProbeTLS(context.Background(), []Target{target}, time.Minute)
		if err != nil || len(res.Facts) != 1 {
			t.Fatalf("suite %#x: res = %+v err = %v, want the device seen", suite, res, err)
		}
		if !slices.Contains(res.Facts[0].OldVersions, "TLS 1.0") || res.Facts[0].NotAfter.IsZero() {
			t.Fatalf("suite %#x: facts = %+v, want TLS 1.0 flagged and the certificate read", suite, res.Facts[0])
		}
	}
}

// selfSignedLeaf is what most devices carry: a certificate that names itself as issuer, is not a CA, and may only sign data. Go's own
// CheckSignatureFrom refuses it as a parent ("parent certificate cannot sign this kind of certificate"), so a check built on that call
// never recognised the common case (found on vertex, 2026-11-06, by testing it against a leaf, not a CA).
func selfSignedLeaf(t *testing.T, algo x509.SignatureAlgorithm) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "device"}, SignatureAlgorithm: algo,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true, IsCA: false,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Skipf("this Go will not sign with %v: %v", algo, err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSelfSigned_RecognisesALeafThatSignedItself(t *testing.T) {
	if !selfSigned(selfSignedLeaf(t, x509.SHA256WithRSA)) {
		t.Fatal("a self-signed leaf certificate (CA:FALSE) was not recognised as self-signed")
	}
}

// An old device's certificate may be signed with SHA-1, which this program's TLS library will not verify: it still names itself as
// issuer, so it is the device's own.
func TestSelfSigned_RecognisesAnOldSHA1CertificateThatNamesItself(t *testing.T) {
	if !selfSigned(selfSignedLeaf(t, x509.SHA1WithRSA)) {
		t.Fatal("a self-signed SHA-1 certificate was not recognised as self-signed")
	}
}

// Naming itself is not enough: a certificate that copies its own name into the issuer field but is signed by someone else's key is
// not self-signed, and is exactly what a forger would make.
func TestSelfSigned_ANameThatMatchesWithAnotherKeysSignatureIsNot(t *testing.T) {
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mine, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(8), Subject: pkix.Name{CommonName: "device"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	// The parent has the same subject (so the issuer field reads the same) but is signed with a different key.
	parent := &x509.Certificate{SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "device"}, NotBefore: tmpl.NotBefore, NotAfter: tmpl.NotAfter, KeyUsage: x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &mine.PublicKey, other)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	if selfSigned(c) {
		t.Fatal("a certificate signed by another key was called self-signed because its issuer field matched its subject")
	}
}

func TestSelfSigned_ACertificateIssuedByAnotherNameIsNot(t *testing.T) {
	ca := certFor(t, "ca", time.Now().Add(365*24*time.Hour), nil)
	leaf := certFor(t, "device", time.Now().Add(24*time.Hour), &ca)
	if selfSigned(leaf.Leaf) {
		t.Fatal("a certificate issued by a CA was called self-signed")
	}
}

// The same case through the real handshake, which is what the audit does.
func TestProbeTLS_ASelfSignedLeafIsReportedAsSelfSigned(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(11), Subject: pkix.Name{CommonName: "device"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(100 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, BasicConstraintsValid: true, IsCA: false,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := tlsServer(t, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
	res, err := prober(allowAll).ProbeTLS(context.Background(), []Target{target}, time.Minute)
	if err != nil || len(res.Facts) != 1 || !res.Facts[0].SelfSigned {
		t.Fatalf("res = %+v err = %v, want one self-signed fact", res, err)
	}
}
