package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit/testssl"
)

// fakeLooker answers per target; a target it has no answer for fails.
type fakeLooker struct {
	unavailable error
	answers     map[testssl.Target]testssl.Result
	asked       []testssl.Target
}

func (f *fakeLooker) Available(context.Context) error { return f.unavailable }

func (f *fakeLooker) Look(_ context.Context, t testssl.Target) (testssl.Result, error) {
	f.asked = append(f.asked, t)
	if r, ok := f.answers[t]; ok {
		return r, nil
	}
	return testssl.Result{}, errors.New("no answer")
}

func protocols(m map[testssl.Protocol]testssl.Verdict) testssl.Result {
	return testssl.Result{Protocols: m}
}

func ciphers(m map[testssl.CipherClass]testssl.Verdict) testssl.Result {
	return testssl.Result{Ciphers: m}
}

func tgt(addr string, port uint16) testssl.Target {
	return testssl.Target{Addr: netip.MustParseAddr(addr), Port: port}
}

func TestTLSLookFindings_AnOldVersionIsTheDeepChecksOwnFindingInTheSameWords(t *testing.T) {
	got, judged := TLSLookFindings("10.0.0.5", 443, protocols(map[testssl.Protocol]testssl.Verdict{
		testssl.TLS10: testssl.Offered, testssl.TLS11: testssl.Offered, testssl.TLS12: testssl.Offered, testssl.SSLv3: testssl.NotOffered,
	}))
	if len(got) != 1 || !judged.Protocols || judged.Ciphers {
		t.Fatalf("%+v judged %+v", got, judged)
	}
	f := got[0]
	if f.Check != CheckTLSWeakProtocol || f.Severity != SeverityMedium || f.Device != "10.0.0.5" || f.Port != 443 || f.Proto != "tcp" {
		t.Errorf("%+v", f)
	}
	if f.Evidence != "It still accepts old protocol versions: TLS 1.0, TLS 1.1" {
		t.Errorf("the words must match what the nmap check says: %q", f.Evidence)
	}
	high, _ := TLSLookFindings("10.0.0.5", 443, protocols(map[testssl.Protocol]testssl.Verdict{testssl.SSLv3: testssl.Offered}))
	if len(high) != 1 || high[0].Severity != SeverityHigh {
		t.Errorf("SSL 3.0 is high: %+v", high)
	}
}

// A stock server offers the CBC suites testssl.sh calls obsoleted; that alone is not a finding.
func TestTLSLookFindings_OnlyTheWeakCipherCategoriesCountAndObsoleteDoesNot(t *testing.T) {
	none, judged := TLSLookFindings("10.0.0.5", 443, ciphers(map[testssl.CipherClass]testssl.Verdict{testssl.CipherObsolete: testssl.Offered, testssl.CipherNull: testssl.NotOffered}))
	if len(none) != 0 || !judged.Ciphers {
		t.Errorf("%+v judged %+v", none, judged)
	}
	got, _ := TLSLookFindings("10.0.0.5", 443, ciphers(map[testssl.CipherClass]testssl.Verdict{testssl.Cipher3DES: testssl.Offered, testssl.CipherExport: testssl.Offered}))
	if len(got) != 1 || got[0].Check != CheckTLSWeakCipher || got[0].Severity != SeverityHigh {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got[0].Evidence, "export-grade") || !strings.Contains(got[0].Evidence, "3DES") {
		t.Errorf("%q", got[0].Evidence)
	}
	med, _ := TLSLookFindings("10.0.0.5", 443, ciphers(map[testssl.CipherClass]testssl.Verdict{testssl.Cipher3DES: testssl.Offered}))
	if len(med) != 1 || med[0].Severity != SeverityMedium {
		t.Errorf("3DES alone is medium: %+v", med)
	}
}

// What the program could not say is not judged, so the other engine's answer for it stands.
func TestTLSLookFindings_AnUnknownVerdictIsNotJudged(t *testing.T) {
	got, judged := TLSLookFindings("10.0.0.5", 443, testssl.Result{
		Protocols: map[testssl.Protocol]testssl.Verdict{testssl.SSLv3: testssl.Unknown, testssl.TLS10: testssl.NotOffered},
		Ciphers:   map[testssl.CipherClass]testssl.Verdict{testssl.Cipher3DES: testssl.NotOffered},
	})
	if len(got) != 0 || judged.Protocols || !judged.Ciphers {
		t.Errorf("%+v judged %+v", got, judged)
	}
}

func TestSpeaksTLS_NamesAndPortsButNotClosedOrUDP(t *testing.T) {
	for _, tc := range []struct {
		p    Port
		want bool
	}{
		{Port{Number: 443, Proto: "tcp", State: "open", Service: "https"}, true},
		{Port{Number: 8443, Proto: "tcp", State: "open", Service: "unknown"}, true},
		{Port{Number: 9000, Proto: "tcp", State: "open", Service: "ssl/foo"}, true},
		{Port{Number: 993, Proto: "tcp", State: "open", Service: "imaps"}, true},
		{Port{Number: 80, Proto: "tcp", State: "open", Service: "http"}, false},
		{Port{Number: 443, Proto: "tcp", State: "closed", Service: "https"}, false},
		{Port{Number: 443, Proto: "udp", State: "open", Service: "https"}, false},
	} {
		if got := speaksTLS(tc.p); got != tc.want {
			t.Errorf("%+v: got %v", tc.p, got)
		}
	}
}

func tlsDeepHost(addr string, ports ...int) []Host {
	var ps []Port
	for _, n := range ports {
		ps = append(ps, Port{Number: n, Proto: "tcp", State: "open", Service: "https", Scripts: []Script{{ID: "ssl-enum-ciphers", Output: oldTLS}}})
	}
	return []Host{{Addr: netip.MustParseAddr(addr), Up: true, Ports: ps}}
}

func tlsDeepService(hosts []Host, l TLSLooker) *Service {
	dev := netip.MustParseAddr("10.0.0.5")
	primary := &fakeEngine{name: "nmap", modes: []Mode{ModeDeep}, result: Result{Hosts: hosts}}
	return NewService(&fixedAuth{addrs: []netip.Addr{dev}}, primary, nil).WithTLSLooker(l)
}

func checksFor(res Result, port int) []string {
	var out []string
	for _, f := range res.Findings {
		if f.Port == port {
			out = append(out, f.Check)
		}
	}
	return out
}

// With no looker nothing changes: nmap's own answer is the finding.
func TestDeepScan_WithoutATLSLookerNmapsAnswerStands(t *testing.T) {
	res, err := tlsDeepService(tlsDeepHost("10.0.0.5", 443), nil).DeepScan(t.Context(), "10.0.0.5", []int{443})
	if err != nil || !slices.Equal(checksFor(res, 443), []string{CheckTLSWeakProtocol}) {
		t.Errorf("%v %+v", err, res.Findings)
	}
}

// One finding per fact: the program's answer replaces nmap's for the same port, with the same check and so the same key.
func TestDeepScan_TheTLSLookReplacesNmapsAnswerForThePortItJudged(t *testing.T) {
	l := &fakeLooker{answers: map[testssl.Target]testssl.Result{
		tgt("10.0.0.5", 443): {
			Protocols: map[testssl.Protocol]testssl.Verdict{testssl.TLS10: testssl.NotOffered, testssl.TLS11: testssl.NotOffered, testssl.SSLv3: testssl.NotOffered, testssl.SSLv2: testssl.NotOffered},
			Ciphers:   map[testssl.CipherClass]testssl.Verdict{testssl.Cipher3DES: testssl.Offered},
		},
	}}
	res, err := tlsDeepService(tlsDeepHost("10.0.0.5", 443), l).DeepScan(t.Context(), "10.0.0.5", []int{443})
	if err != nil {
		t.Fatal(err)
	}
	if got := checksFor(res, 443); !slices.Equal(got, []string{CheckTLSWeakCipher}) {
		t.Errorf("nmap's protocol finding is replaced by the program's clean verdict and its cipher finding added: %v", got)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("%v", res.Warnings)
	}
}

func TestDeepScan_ALookThatFailedKeepsNmapsAnswerAndSaysSo(t *testing.T) {
	res, err := tlsDeepService(tlsDeepHost("10.0.0.5", 443), &fakeLooker{}).DeepScan(t.Context(), "10.0.0.5", []int{443})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(checksFor(res, 443), []string{CheckTLSWeakProtocol}) {
		t.Errorf("%+v", res.Findings)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "443") {
		t.Errorf("the reader is told the closer look did not work, naming the port: %v", res.Warnings)
	}
}

func TestDeepScan_AProgramThatIsNotInstalledIsSilent(t *testing.T) {
	l := &fakeLooker{unavailable: errors.New("not installed")}
	res, err := tlsDeepService(tlsDeepHost("10.0.0.5", 443), l).DeepScan(t.Context(), "10.0.0.5", []int{443})
	if err != nil || len(l.asked) != 0 || len(res.Warnings) != 0 || !slices.Equal(checksFor(res, 443), []string{CheckTLSWeakProtocol}) {
		t.Errorf("%v asked %v %v %+v", err, l.asked, res.Warnings, res.Findings)
	}
}

func TestDeepScan_AtMostThreeTLSPortsAreLookedAt(t *testing.T) {
	l := &fakeLooker{}
	_, err := tlsDeepService(tlsDeepHost("10.0.0.5", 443, 8443, 993, 995, 465), l).DeepScan(t.Context(), "10.0.0.5", []int{443, 465, 993, 995, 8443})
	if err != nil || len(l.asked) != maxDeepTLSPorts {
		t.Errorf("%v: %d looks, cap %d", err, len(l.asked), maxDeepTLSPorts)
	}
}
