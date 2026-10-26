package netaudit

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"
)

var checkNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

// Each fixture is the documented output of the nmap script (nmap 7.99), so the parsers are tested against what the
// script really prints, not against what they were written to expect.
func hitsOf(t *testing.T, script, output string) []checkHit {
	t.Helper()
	eval, ok := checkTable[script]
	if !ok {
		t.Fatalf("no check for script %q", script)
	}
	return eval(output, checkNow)
}

func onlyHit(t *testing.T, script, output, wantCheck, wantSeverity string) checkHit {
	t.Helper()
	hits := hitsOf(t, script, output)
	if len(hits) != 1 || hits[0].Check != wantCheck || hits[0].Severity != wantSeverity {
		t.Fatalf("%s: hits = %+v, want one %s/%s", script, hits, wantCheck, wantSeverity)
	}
	return hits[0]
}

func TestCheck_SMBv1(t *testing.T) {
	onlyHit(t, "smb-protocols", "\n  dialects: \n    NT LM 0.12 (SMBv1) [dangerous, but default]\n    2.0.2\n    2.1\n    3.1.1", CheckSMBv1, SeverityHigh)
	if hits := hitsOf(t, "smb-protocols", "\n  dialects: \n    2.0.2\n    2.1\n    3.0\n    3.1.1"); len(hits) != 0 {
		t.Errorf("a server that only speaks SMB2/3 has nothing to report: %+v", hits)
	}
}

func TestCheck_SMBSigning(t *testing.T) {
	onlyHit(t, "smb2-security-mode", "\n  3.1.1: \n    Message signing enabled but not required", CheckSMBSigning, SeverityMedium)
	if hits := hitsOf(t, "smb2-security-mode", "\n  3.1.1: \n    Message signing enabled and required"); len(hits) != 0 {
		t.Errorf("required signing is fine: %+v", hits)
	}
}

func TestCheck_AnonymousFTP(t *testing.T) {
	out := "Anonymous FTP login allowed (FTP code 230)\n-rw-r--r--   1 1170     924            31 Mar 28  2001 .banner\nd--x--x--x   2 root     root         1024 Jan 14  2002 bin"
	hit := onlyHit(t, "ftp-anon", out, CheckFTPAnonymous, SeverityHigh)
	if strings.Contains(hit.Evidence, "banner") || strings.Contains(hit.Evidence, "root") {
		t.Errorf("a directory listing from the device must never reach the evidence: %q", hit.Evidence)
	}
	if hits := hitsOf(t, "ftp-anon", ""); len(hits) != 0 {
		t.Errorf("no output, no finding: %+v", hits)
	}
}

func TestCheck_VNCWithoutAPassword(t *testing.T) {
	onlyHit(t, "vnc-info", "\n  Protocol version: 3.8\n  Security types:\n    None (1)\n    VNC Authentication (2)", CheckVNCNoAuth, SeverityCritical)
	if hits := hitsOf(t, "vnc-info", "\n  Protocol version: 3.8\n  Security types:\n    VNC Authentication (2)\n    Mac OS X security type (30)"); len(hits) != 0 {
		t.Errorf("a server that asks for a password is fine: %+v", hits)
	}
}

func TestCheck_RedisThatAnswersWithoutAPassword(t *testing.T) {
	out := "\n  Version            2.2.11\n  Architecture       64 bits\n  Bind addresses:\n    192.168.121.101\n  Client connections:\n    192.168.171.101"
	hit := onlyHit(t, "redis-info", out, CheckRedisNoAuth, SeverityCritical)
	if strings.Contains(hit.Evidence, "192.168") {
		t.Errorf("addresses Redis reported must not be copied: %q", hit.Evidence)
	}
	if hits := hitsOf(t, "redis-info", ""); len(hits) != 0 {
		t.Errorf("a Redis that wants a password prints nothing: %+v", hits)
	}
}

func TestCheck_NFSExports(t *testing.T) {
	hit := onlyHit(t, "nfs-showmount", "\n  /home/storage/backup 10.46.200.0/255.255.255.0\n  /home 1.2.3.4/255.255.255.255 10.46.200.0/255.255.255.0", CheckNFSExports, SeverityMedium)
	if !strings.Contains(hit.Evidence, "2") {
		t.Errorf("evidence should count the exports: %q", hit.Evidence)
	}
	if strings.Contains(hit.Evidence, "storage") {
		t.Errorf("export paths are not copied: %q", hit.Evidence)
	}
}

func TestCheck_CertificateDates(t *testing.T) {
	cert := func(after string) string {
		return "Subject: commonName=x\n| Not valid before: 2024-02-08T00:00:00\n|_Not valid after:  " + after
	}
	onlyHit(t, "ssl-cert", cert("2025-02-08T23:59:59"), CheckCertExpired, SeverityMedium)
	onlyHit(t, "ssl-cert", cert("2026-11-10T00:00:00"), CheckCertExpiring, SeverityLow)
	if hits := hitsOf(t, "ssl-cert", cert("2028-01-01T00:00:00")); len(hits) != 0 {
		t.Errorf("a certificate valid for years is fine: %+v", hits)
	}
	if hits := hitsOf(t, "ssl-cert", "Subject: commonName=x"); len(hits) != 0 {
		t.Errorf("no date, no finding: %+v", hits)
	}
	// A certificate issued for a couple of days is renewed by the device itself (Chromecasts and similar do this
	// constantly): "expires tomorrow" is how it always looks, not a problem. An expired one still is.
	short := "Subject: commonName=cast\nNot valid before: 2026-10-25T10:00:00\nNot valid after:  2026-10-27T10:00:00"
	if hits := hitsOf(t, "ssl-cert", short); len(hits) != 0 {
		t.Errorf("a short-lived certificate close to its renewal is not a finding: %+v", hits)
	}
	onlyHit(t, "ssl-cert", "Subject: commonName=cast\nNot valid before: 2026-10-20T10:00:00\nNot valid after:  2026-10-22T10:00:00", CheckCertExpired, SeverityMedium)
	// Self-signed certificates are what almost every home device uses; they are not reported.
	if hits := hitsOf(t, "ssl-cert", "Subject: commonName=router\nIssuer: commonName=router\nNot valid after:  2030-01-01T00:00:00"); len(hits) != 0 {
		t.Errorf("self-signed alone is not a finding: %+v", hits)
	}
}

func TestCheck_WeakSSHAlgorithms(t *testing.T) {
	weak := "\n  kex_algorithms (2)\n      curve25519-sha256\n      diffie-hellman-group1-sha1\n  encryption_algorithms (3)\n      aes128-ctr\n      arcfour\n      3des-cbc\n  mac_algorithms (1)\n      hmac-md5"
	hit := onlyHit(t, "ssh2-enum-algos", weak, CheckSSHWeakAlgorithms, SeverityLow)
	for _, want := range []string{"arcfour", "3des-cbc", "diffie-hellman-group1-sha1"} {
		if !strings.Contains(hit.Evidence, want) {
			t.Errorf("evidence %q should name %s", hit.Evidence, want)
		}
	}
	if hits := hitsOf(t, "ssh2-enum-algos", "\n  kex_algorithms (1)\n      curve25519-sha256\n  encryption_algorithms (1)\n      chacha20-poly1305@openssh.com\n  mac_algorithms (1)\n      hmac-sha2-256-etm@openssh.com"); len(hits) != 0 {
		t.Errorf("modern algorithms only: %+v", hits)
	}
}

func TestCheck_RDPWithoutNetworkLevelAuthentication(t *testing.T) {
	good := "\n  Security layer\n    CredSSP (NLA): SUCCESS\n    SSL: SUCCESS\n  RDP Encryption level: High"
	if hits := hitsOf(t, "rdp-enum-encryption", good); len(hits) != 0 {
		t.Errorf("NLA offered is fine: %+v", hits)
	}
	onlyHit(t, "rdp-enum-encryption", "\n  Security layer\n    CredSSP (NLA): FAILURE\n    Native RDP: SUCCESS", CheckRDPNoNLA, SeverityMedium)
}

func TestCheckScriptNames_AreTheFixedSortedList(t *testing.T) {
	want := []string{"ftp-anon", "nfs-showmount", "rdp-enum-encryption", "redis-info", "smb-protocols", "smb2-security-mode", "ssh2-enum-algos", "ssl-cert", "vnc-info"}
	got := CheckScriptNames()
	if !slices.Equal(got, want) {
		t.Errorf("the scripts a scan may run are a reviewed list, got %v want %v", got, want)
	}
}

func TestFindingsOf_AddsCheckFindingsBesideTheOpenPort(t *testing.T) {
	clock = func() time.Time { return checkNow }
	t.Cleanup(func() { clock = time.Now })
	hosts := []Host{{Addr: netip.MustParseAddr("192.0.2.10"), Up: true, Ports: []Port{
		{Number: 445, Proto: "tcp", State: "open", Service: "microsoft-ds", Scripts: []Script{
			{ID: "smb-protocols", Output: "\n  dialects: \n    NT LM 0.12 (SMBv1) [dangerous, but default]"},
			{ID: "smb2-security-mode", Output: "\n  3.1.1: \n    Message signing enabled but not required"},
			{ID: "not-a-check", Output: "NT LM 0.12 (SMBv1)"},
		}},
	}}}
	got := FindingsOf(hosts)
	if len(got) != 3 {
		t.Fatalf("one open port + two checks, got %+v", got)
	}
	if got[0].Check != CheckOpenPort || got[1].Check != CheckSMBv1 || got[2].Check != CheckSMBSigning {
		t.Errorf("order is the port, then its checks by script name: %+v", got)
	}
	keys := map[string]bool{}
	for _, f := range got {
		if f.Device != "192.0.2.10" || f.Port != 445 || f.Proto != "tcp" {
			t.Errorf("a check finding is about the same device and port: %+v", f)
		}
		keys[f.Key()] = true
	}
	if len(keys) != 3 {
		t.Errorf("every finding needs its own key so each can be compared and accepted on its own: %v", keys)
	}
}

func TestModeChecks_NeedsNmapAndIsNotIntrusive(t *testing.T) {
	spec := ModeChecks.Spec()
	if !ModeChecks.Valid() || !spec.NeedsNmap || spec.Intrusive {
		t.Errorf("checks = %+v: it reads from devices, never falls back to the limited engine and is not intrusive", spec)
	}
}
