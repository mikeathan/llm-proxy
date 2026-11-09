package webprobe

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"time"
)

// The certificate checks: one handshake to each secure web service, and a second that offers only the old protocol versions. A
// handshake sends no data and reads nothing but the certificate the device presents, which is why it does not check that
// certificate: a device on the home network rarely has one a browser trusts, and nothing secret is sent that a certificate would
// protect. What is kept is dates, versions and one yes-or-no; none of the certificate's text is.

// handshakeTimeout bounds one handshake, from the first byte sent to the last read.
const handshakeTimeout = 5 * time.Second

// allSuites are every cipher suite this program's TLS library knows, the broken ones included. Old embedded devices often offer only
// RSA key exchange with AES-CBC or 3DES, which the library leaves out unless asked; a probe that did not ask would see such a device
// fail the handshake, report nothing and never flag its old protocol. The probe sends no data, so offering a weak suite exposes
// nothing; the point is to find out what the device accepts.
var allSuites = func() []uint16 {
	var ids []uint16
	for _, s := range tls.CipherSuites() {
		ids = append(ids, s.ID)
	}
	for _, s := range tls.InsecureCipherSuites() {
		ids = append(ids, s.ID)
	}
	return ids
}()

// oldVersions are the protocol versions that are offered one at a time in the second handshake. They leave the stack as the numbers the
// standard gives them; the core has the words.
var oldVersions = []uint16{tls.VersionTLS10, tls.VersionTLS11}

// TLSFacts is what one secure web service showed. OldVersions lists, as protocol numbers, the versions older than 1.2 that it accepted.
type TLSFacts struct {
	Addr        netip.Addr
	Port        int
	NotBefore   time.Time
	NotAfter    time.Time
	SelfSigned  bool
	OldVersions []uint16
}

// TLSResult is what a run found and whether its time limit ended it before every service was asked.
type TLSResult struct {
	Facts   []TLSFacts
	Partial bool
}

// ProbeTLS handshakes with every target that speaks TLS and returns what each showed. A device that does not answer, or does not speak
// TLS, adds nothing and is not an error; an address the guard refuses is an error before anything is sent. limit ends the run, and what
// was found by then is returned with Partial set.
func (p *Prober) ProbeTLS(ctx context.Context, targets []Target, limit time.Duration) (TLSResult, error) {
	if err := p.validate("certificate checks", targets, limit); err != nil {
		return TLSResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	var res TLSResult
	asked := false
	for _, t := range targets {
		if !t.TLS {
			continue
		}
		if asked && !p.wait(runCtx) {
			return finish(ctx, res)
		}
		asked = true
		if facts, ok := p.handshakes(runCtx, t); ok {
			res.Facts = append(res.Facts, facts)
		}
		if runCtx.Err() != nil {
			return finish(ctx, res)
		}
	}
	return res, nil
}

// finish ends a run that was cut short: the caller's own cancellation is an error, the run's time limit is a partial result.
func finish(parent context.Context, res TLSResult) (TLSResult, error) {
	if parent.Err() != nil {
		return TLSResult{}, parent.Err()
	}
	res.Partial = true
	return res, nil
}

// handshakes reads the certificate a service presents and finds out which old versions it accepts. It reports false when the service
// gave no certificate at all.
func (p *Prober) handshakes(ctx context.Context, t Target) (TLSFacts, bool) {
	facts := TLSFacts{Addr: t.Addr, Port: t.Port}
	var leaf *x509.Certificate
	if state, ok := handshake(ctx, p.dial, t, tls.VersionTLS12, 0); ok {
		leaf = leafOf(state)
	}
	for _, v := range oldVersions {
		if !p.wait(ctx) { // a small device is asked gently: a pause between its handshakes too
			return facts, false
		}
		if state, ok := handshake(ctx, p.dial, t, v, v); ok {
			facts.OldVersions = append(facts.OldVersions, v)
			if leaf == nil {
				leaf = leafOf(state)
			}
		}
	}
	if leaf == nil {
		return facts, false
	}
	facts.NotBefore, facts.NotAfter, facts.SelfSigned = leaf.NotBefore, leaf.NotAfter, selfSigned(leaf)
	return facts, true
}

func leafOf(s tls.ConnectionState) *x509.Certificate {
	if len(s.PeerCertificates) == 0 {
		return nil
	}
	return s.PeerCertificates[0]
}

// selfSigned says whether a certificate names itself as its issuer and was signed by its own key. The signature is checked with the
// certificate's own public key directly (CheckSignature), not through CheckSignatureFrom: that call also demands that the parent be a CA
// allowed to sign certificates, which the leaf certificate most devices make for themselves is not, so it would never recognise the
// common case. A certificate signed with an algorithm this program's TLS library refuses to verify (SHA-1, which old devices use) is
// taken as its own when it names itself; one that names itself but carries another key's signature is not.
func selfSigned(c *x509.Certificate) bool {
	if !bytes.Equal(c.RawIssuer, c.RawSubject) {
		return false
	}
	err := c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature)
	var insecure x509.InsecureAlgorithmError
	return err == nil || errors.As(err, &insecure)
}

// handshake opens one connection, offers the versions from min to max (max 0 is the newest this program speaks), and closes it as soon
// as the handshake ends. Nothing is written afterwards.
func handshake(ctx context.Context, dial Dial, t Target, minVersion, maxVersion uint16) (tls.ConnectionState, bool) {
	hsCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	conn, err := dialWithin(hsCtx, dial, "tcp", net.JoinHostPort(t.Addr.String(), strconv.Itoa(t.Port)))
	if err != nil {
		return tls.ConnectionState{}, false
	}
	defer conn.Close()
	cfg := &tls.Config{MinVersion: minVersion, MaxVersion: maxVersion, CipherSuites: allSuites, InsecureSkipVerify: true} // #nosec G402 -- only the certificate is read, nothing is sent: see the note at the top of this file
	client := tls.Client(conn, cfg)
	if err := client.HandshakeContext(hsCtx); err != nil {
		return tls.ConnectionState{}, false
	}
	return client.ConnectionState(), true
}
