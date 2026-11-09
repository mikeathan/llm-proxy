package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var tlsNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func factsOf(port int, notBefore, notAfter time.Time, selfSigned bool, old ...uint16) TLSFact {
	return TLSFact{Addr: mustAddr("192.168.1.20"), Port: port, NotBefore: notBefore, NotAfter: notAfter, SelfSigned: selfSigned, OldVersions: old}
}

func tlsChecksOf(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Check)
	}
	return out
}

func TestTLSFindings_ExpiredAndExpiringCertificates(t *testing.T) {
	year := 365 * 24 * time.Hour
	got := TLSFindings([]TLSFact{
		factsOf(443, tlsNow.Add(-year), tlsNow.Add(-24*time.Hour), false),
		factsOf(8443, tlsNow.Add(-year), tlsNow.Add(10*24*time.Hour), false),
		factsOf(9443, tlsNow.Add(-year), tlsNow.Add(200*24*time.Hour), false),
	}, tlsNow)
	if strings.Join(tlsChecksOf(got), " ") != CheckCertExpired+" "+CheckCertExpiring {
		t.Fatalf("checks = %v, want one expired and one expiring", tlsChecksOf(got))
	}
	if got[0].Severity != SeverityMedium || got[0].Evidence != "The certificate expired on 2026-11-05" || got[0].Version != "2026-11-05" {
		t.Errorf("expired = %+v", got[0])
	}
	if got[1].Severity != SeverityLow || got[1].Evidence != "The certificate expires on 2026-11-16" || got[1].Port != 8443 || got[1].Proto != "tcp" {
		t.Errorf("expiring = %+v", got[1])
	}
}

func TestTLSFindings_ACertificateTheDeviceRenewsItselfIsNotAboutToExpire(t *testing.T) {
	// Valid for two days, so it is always close to its end: the same rule as the nmap-based check.
	got := TLSFindings([]TLSFact{factsOf(8443, tlsNow.Add(-24*time.Hour), tlsNow.Add(24*time.Hour), false)}, tlsNow)
	if len(got) != 0 {
		t.Fatalf("findings = %+v, want none for a short-lived certificate", got)
	}
}

func TestTLSFindings_SelfSignedAndOldVersions(t *testing.T) {
	got := TLSFindings([]TLSFact{factsOf(443, tlsNow.Add(-time.Hour*24*100), tlsNow.Add(24*time.Hour*300), true, 0x0301, 0x0302)}, tlsNow)
	if strings.Join(tlsChecksOf(got), " ") != CheckSelfSigned+" "+CheckTLSWeakProtocol {
		t.Fatalf("checks = %v", tlsChecksOf(got))
	}
	if got[0].Severity != SeverityLow || got[0].Version != "self-signed" {
		t.Errorf("self-signed = %+v", got[0])
	}
	if got[1].Severity != SeverityMedium || got[1].Evidence != "It still accepts old protocol versions: TLS 1.0, TLS 1.1" || got[1].Version != "TLS 1.0, TLS 1.1" {
		t.Errorf("old versions = %+v", got[1])
	}
}

// The stack sends the protocol versions as the numbers the standard gives them; the words in the evidence and the Version are the core's, so
// an acceptance made on the old wording still holds. A number the core has no words for is dropped.
func TestTLSFindings_VersionsComeAsNumbersAndAnUnknownOneIsDropped(t *testing.T) {
	got := TLSFindings([]TLSFact{factsOf(443, tlsNow.Add(-time.Hour*24*100), tlsNow.Add(24*time.Hour*300), false, 0x0301, 0x9999, 0x0302)}, tlsNow)
	if len(got) != 1 || got[0].Evidence != "It still accepts old protocol versions: TLS 1.0, TLS 1.1" || got[0].Version != "TLS 1.0, TLS 1.1" {
		t.Fatalf("findings = %+v", got)
	}
	if none := TLSFindings([]TLSFact{factsOf(443, tlsNow.Add(-time.Hour*24*100), tlsNow.Add(24*time.Hour*300), false, 0x9999)}, tlsNow); len(none) != 0 {
		t.Errorf("a version with no words made a finding: %+v", none)
	}
}

func TestTLSFindings_EveryNewCheckHasAFixAndAGuide(t *testing.T) {
	if CheckTitle(CheckSelfSigned) == "" || RemediationFor(CheckSelfSigned) == "" {
		t.Fatal("the self-signed check needs a title and a fix")
	}
	if _, ok := guides[CheckSelfSigned]; !ok {
		t.Fatal("the self-signed check needs a fix guide")
	}
}

type fakeTLSWeb struct {
	fakeWeb
	tls    TLSResult
	tlsErr error
	tlsFor []WebTarget
}

func (f *fakeTLSWeb) ProbeTLS(_ context.Context, t []WebTarget, _ time.Duration) (TLSResult, error) {
	f.tlsFor = t
	return f.tls, f.tlsErr
}

func reporterWith(web WebChecker, health *RunHealth) *RunReporter {
	r := NewRunReporter(nil).WithWebChecks(func() bool { return true }, web).WithRunHealth(health)
	r.now = func() time.Time { return tlsNow }
	return r
}

func TestRunReporter_AsksOnlyTheSecureServicesForACertificate(t *testing.T) {
	web := &fakeTLSWeb{tls: TLSResult{Facts: []TLSFact{factsOf(443, tlsNow.Add(-time.Hour*24*100), tlsNow.Add(24*time.Hour*300), true)}}}
	result := Result{Findings: []Finding{openPortOf("192.168.1.20", 80, "http"), openPortOf("192.168.1.20", 443, "https")}}
	reporterWith(web, NewRunHealth()).addWebChecks(context.Background(), &result)
	if len(web.tlsFor) != 1 || web.tlsFor[0].Port != 443 || !web.tlsFor[0].TLS {
		t.Fatalf("certificate targets = %+v, want only the https port", web.tlsFor)
	}
	var self bool
	for _, f := range result.Findings {
		self = self || f.Check == CheckSelfSigned
	}
	if !self {
		t.Fatalf("findings = %+v, want a self-signed finding", result.Findings)
	}
}

func TestRunReporter_TheNmapFindingOfTheSameCertificateIsKept(t *testing.T) {
	nmap := Finding{Device: "192.168.1.20", Port: 443, Proto: "tcp", Check: CheckCertExpired, Severity: SeverityMedium, Evidence: "The certificate expired on 2026-03-01"}
	web := &fakeTLSWeb{tls: TLSResult{Facts: []TLSFact{factsOf(443, tlsNow.Add(-time.Hour*24*400), tlsNow.Add(-24*time.Hour*250), false)}}}
	result := Result{Findings: []Finding{openPortOf("192.168.1.20", 443, "https"), nmap}}
	reporterWith(web, NewRunHealth()).addWebChecks(context.Background(), &result)
	n := 0
	for _, f := range result.Findings {
		if f.Check == CheckCertExpired {
			n++
			if f.Evidence != nmap.Evidence {
				t.Errorf("evidence = %q, want the nmap finding kept", f.Evidence)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d expired findings, want 1", n)
	}
}

func TestRunReporter_AFreshHandshakeReplacesACarriedOldVersionFinding(t *testing.T) {
	carried := Finding{Device: "192.168.1.20", Port: 443, Proto: "tcp", Check: CheckTLSWeakProtocol, Severity: SeverityMedium, Evidence: "It still accepts old protocol versions: TLS 1.0 (from the deep check of 2026-11-01)"}
	web := &fakeTLSWeb{tls: TLSResult{Facts: []TLSFact{factsOf(443, tlsNow.Add(-time.Hour*24*10), tlsNow.Add(24*time.Hour*300), false)}}}
	result := Result{Findings: []Finding{openPortOf("192.168.1.20", 443, "https"), carried}}
	reporterWith(web, NewRunHealth()).addWebChecks(context.Background(), &result)
	for _, f := range result.Findings {
		if f.Check == CheckTLSWeakProtocol {
			t.Fatalf("finding %+v survived a handshake that accepted no old version", f)
		}
	}
}

func TestRunReporter_AFailedCertificateRunIsRecordedAndSaid(t *testing.T) {
	web := &fakeTLSWeb{tlsErr: errors.New("outside the scan scope")}
	health := NewRunHealth()
	result := Result{Findings: []Finding{openPortOf("192.168.1.20", 443, "https")}}
	note := reporterWith(web, health).addWebChecks(context.Background(), &result)
	if !strings.Contains(note, "certificate checks could not run") {
		t.Fatalf("note = %q", note)
	}
	if got := health.Reading(MonitorWebChecks, true, time.Hour); !got.Failed {
		t.Fatalf("reading = %+v, want the failure recorded", got)
	}
}

func TestDeepChecks_AddTheCertificateFindingsOfTheDevice(t *testing.T) {
	base := baselineWith(openPortOf("192.168.1.20", 443, "https"))
	d := newDeepChecks(&fakeDeepScanner{}, base, &memDeepStore{})
	d.WithWebChecks(&fakeTLSWeb{tls: TLSResult{Facts: []TLSFact{factsOf(443, tlsNow.Add(-time.Hour*24*100), tlsNow.Add(24*time.Hour*300), true)}}})
	check, err := d.Run(context.Background(), "ws", "audit", "192.168.1.20")
	if err != nil {
		t.Fatal(err)
	}
	var self bool
	for _, f := range check.Findings {
		self = self || f.Check == CheckSelfSigned
	}
	if !self {
		t.Fatalf("findings = %+v, want the self-signed finding in the deep check", check.Findings)
	}
}
