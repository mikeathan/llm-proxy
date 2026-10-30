package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

// Real shapes: ssh-hostkey prints one line per key; ssl-cert's output is the one in nmap/testdata/scripts_ssl_cert.xml.
const (
	sshKeysA = "\n  3072 aa:aa:aa:aa:aa:aa:aa:aa:aa:aa:aa:aa:aa:aa:aa:01 (RSA)\n  256 bb:bb:bb:bb:bb:bb:bb:bb:bb:bb:bb:bb:bb:bb:bb:02 (ECDSA)\n  256 cc:cc:cc:cc:cc:cc:cc:cc:cc:cc:cc:cc:cc:cc:cc:03 (ED25519)"
	sshKeysB = "\n  3072 dd:dd:dd:dd:dd:dd:dd:dd:dd:dd:dd:dd:dd:dd:dd:04 (RSA)\n  256 ee:ee:ee:ee:ee:ee:ee:ee:ee:ee:ee:ee:ee:ee:ee:05 (ECDSA)\n  256 ff:ff:ff:ff:ff:ff:ff:ff:ff:ff:ff:ff:ff:ff:ff:06 (ED25519)"
	// One key added to the set the server already had.
	sshKeysAPlus = sshKeysA + "\n  4096 99:99:99:99:99:99:99:99:99:99:99:99:99:99:99:07 (RSA)"
	certOne      = "Subject: commonName=fixture.local\nIssuer: commonName=fixture.local\nPublic Key type: rsa\nPublic Key bits: 2048\nSignature Algorithm: sha256WithRSAEncryption\nNot valid before: 2026-10-26T13:02:37\nNot valid after:  2027-10-28T13:02:37\nMD5:     c292 b300 f990 03b6 2a0f ff6c 0f96 7073\nSHA-1:   931e a230 2fd1 6e69 638c 56c1 3d59 5d6c 3f22 9401\nSHA-256: 2f1c aaea 15c8 db79 0bb4 e518 5065 759f 7c9e 02df 1ee4 d350 1c53 5325 bc8a 2835"
)

func certWith(over ...string) string {
	out := certOne
	for i := 0; i+1 < len(over); i += 2 {
		out = strings.Replace(out, over[i], over[i+1], 1)
	}
	return out
}

func scriptHost(addr string, port int, script, output string) Host {
	return Host{Addr: netip.MustParseAddr(addr), Up: true, Ports: []Port{{Number: port, Proto: "tcp", State: "open", Scripts: []Script{{ID: script, Output: output}}}}}
}

func TestObservedIdentities_ReadsFingerprintsAndNothingElse(t *testing.T) {
	got := ObservedIdentities([]Host{scriptHost("10.0.0.5", 22, "ssh-hostkey", sshKeysA), scriptHost("10.0.0.6", 443, "ssl-cert", certOne)})
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	ssh, tls := got[0], got[1]
	if ssh.Kind != IdentitySSH || len(ssh.Prints) != 3 || ssh.Device != "10.0.0.5" || ssh.Port != 22 {
		t.Errorf("ssh = %+v", ssh)
	}
	if tls.Kind != IdentityTLS || len(tls.Prints) != 1 || tls.Shape == "" {
		t.Errorf("tls = %+v", tls)
	}
	for _, p := range append(ssh.Prints, tls.Prints...) {
		if strings.ContainsAny(p, "\n<>") {
			t.Errorf("a fingerprint is a short token: %q", p)
		}
	}
}

func TestObservedIdentities_APortWithoutAFingerprintTellsNothing(t *testing.T) {
	// A script that printed no keys (or a device that sent junk) is "unknown", never "changed to nothing".
	if got := ObservedIdentities([]Host{scriptHost("10.0.0.5", 22, "ssh-hostkey", "no keys here"), scriptHost("10.0.0.6", 443, "ssl-cert", "ERROR: Script execution failed")}); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestObservedIdentities_HostileTextIsBoundedAndNeverKept(t *testing.T) {
	hostile := "\n  3072 " + strings.Repeat("a", 5000) + " (RSA)"
	for _, o := range ObservedIdentities([]Host{scriptHost("10.0.0.5", 22, "ssh-hostkey", hostile)}) {
		for _, p := range o.Prints {
			if len(p) > maxPrintLen {
				t.Errorf("a fingerprint longer than %d was kept: %d", maxPrintLen, len(p))
			}
		}
	}
}

func seenSSH(keys string) []ObservedIdentity {
	return ObservedIdentities([]Host{scriptHost("10.0.0.5", 22, "ssh-hostkey", keys)})
}
func seenTLS(cert string) []ObservedIdentity {
	return ObservedIdentities([]Host{scriptHost("10.0.0.6", 443, "ssl-cert", cert)})
}

func TestTrackIdentities_TheFirstLookIsRememberedNotReported(t *testing.T) {
	stored := TrackIdentities(nil, seenSSH(sshKeysA), t0)
	if len(stored) != 1 || stored[0].Change != nil {
		t.Fatalf("stored = %+v", stored)
	}
	if got := IdentityFindings(stored, t0); len(got) != 0 {
		t.Errorf("a baseline has no findings: %+v", got)
	}
}

func TestTrackIdentities_AllKeysReplacedIsAChange(t *testing.T) {
	stored := TrackIdentities(nil, seenSSH(sshKeysA), t0)
	later := t0.Add(24 * time.Hour)
	stored = TrackIdentities(stored, seenSSH(sshKeysB), later)
	got := IdentityFindings(stored, later)
	if len(got) != 1 || got[0].Check != CheckHostKeyChanged || got[0].Severity != SeverityMedium || got[0].Device != "10.0.0.5" || got[0].Port != 22 || got[0].Proto != "tcp" {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].Key() != "10.0.0.5|tcp|22|hostkey_changed" {
		t.Errorf("key %q", got[0].Key())
	}
	// The next night sees the same keys: the change is still reported, not forgotten after one night.
	stored = TrackIdentities(stored, seenSSH(sshKeysB), later.Add(24*time.Hour))
	if got := IdentityFindings(stored, later.Add(24*time.Hour)); len(got) != 1 {
		t.Errorf("a change stays in the report until accepted or aged out: %+v", got)
	}
}

func TestTrackIdentities_AKeyAddedOrDroppedIsNotAnAttack(t *testing.T) {
	stored := TrackIdentities(nil, seenSSH(sshKeysA), t0)
	stored = TrackIdentities(stored, seenSSH(sshKeysAPlus), t0.Add(time.Hour))
	if got := IdentityFindings(stored, t0.Add(time.Hour)); len(got) != 0 {
		t.Errorf("a server that added a key still has the old ones: %+v", got)
	}
	if stored[0].Prints[0] == "" || len(stored[0].Prints) != 4 {
		t.Errorf("the new set is remembered: %+v", stored[0].Prints)
	}
}

func TestTrackIdentities_ARenewedCertificateIsQuiet_ADifferentOneIsNot(t *testing.T) {
	stored := TrackIdentities(nil, seenTLS(certOne), t0)
	renewed := certWith("SHA-256: 2f1c aaea", "SHA-256: 0000 1111", "Not valid after:  2027-10-28", "Not valid after:  2028-10-28")
	later := t0.Add(24 * time.Hour)
	r := TrackIdentities(stored, seenTLS(renewed), later)
	got := IdentityFindings(r, later)
	if len(got) != 1 || got[0].Check != CheckCertChanged || got[0].Severity != SeverityLow || !strings.Contains(got[0].Evidence, "renewal") {
		t.Fatalf("a renewal is a low note: %+v", got)
	}
	other := certWith("SHA-256: 2f1c aaea", "SHA-256: 0000 1111", "Subject: commonName=fixture.local", "Subject: commonName=evil.example")
	d := TrackIdentities(stored, seenTLS(other), later)
	got = IdentityFindings(d, later)
	if len(got) != 1 || got[0].Severity != SeverityMedium || strings.Contains(got[0].Evidence, "renewal") {
		t.Errorf("a certificate for another name is medium and not called a renewal: %+v", got)
	}
	if strings.Contains(got[0].Evidence, "evil.example") {
		t.Errorf("text from the certificate reached the finding: %q", got[0].Evidence)
	}
}

func TestTrackIdentities_APortNotLookedAtThisTimeIsLeftAlone(t *testing.T) {
	stored := TrackIdentities(nil, append(seenSSH(sshKeysA), seenTLS(certOne)...), t0)
	stored = TrackIdentities(stored, seenSSH(sshKeysA), t0.Add(time.Hour))
	if len(stored) != 2 {
		t.Errorf("a scan without the TLS script must not forget the certificate: %+v", stored)
	}
}

func TestIdentityFindings_AChangeAgesOutAndOldEntriesAreDropped(t *testing.T) {
	stored := TrackIdentities(nil, seenSSH(sshKeysA), t0)
	stored = TrackIdentities(stored, seenSSH(sshKeysB), t0.Add(time.Hour))
	if got := IdentityFindings(stored, t0.Add(time.Hour+ChangeWindow+time.Hour)); len(got) != 0 {
		t.Errorf("an old change is no longer reported: %+v", got)
	}
	gone := TrackIdentities(stored, nil, t0.Add(identityKeepFor+24*time.Hour))
	if len(gone) != 0 {
		t.Errorf("a device not seen for months is forgotten: %+v", gone)
	}
}

type memIdentities struct {
	items  []models.DeviceIdentity
	err    error
	writes int
}

func (m *memIdentities) Read() ([]models.DeviceIdentity, error) { return m.items, m.err }
func (m *memIdentities) Write(v []models.DeviceIdentity) error {
	m.items = v
	m.writes++
	return nil
}

func TestIdentities_TrackSavesAndReturnsFindings(t *testing.T) {
	st := &memIdentities{}
	v := NewIdentities(st)
	v.now = func() time.Time { return t0 }
	if f, err := v.Track(context.Background(), []Host{scriptHost("10.0.0.5", 22, "ssh-hostkey", sshKeysA)}); err != nil || len(f) != 0 {
		t.Fatalf("first look: %v %+v", err, f)
	}
	v.now = func() time.Time { return t0.Add(time.Hour) }
	f, err := v.Track(context.Background(), []Host{scriptHost("10.0.0.5", 22, "ssh-hostkey", sshKeysB)})
	if err != nil || len(f) != 1 || f[0].Check != CheckHostKeyChanged {
		t.Fatalf("second look: %v %+v", err, f)
	}
}

func TestIdentities_ATrouble_ReadingIsAnErrorAndNothingIsOverwritten(t *testing.T) {
	st := &memIdentities{err: errors.New("corrupt")}
	if _, err := NewIdentities(st).Track(context.Background(), []Host{scriptHost("10.0.0.5", 22, "ssh-hostkey", sshKeysA)}); err == nil || st.writes != 0 {
		t.Errorf("err=%v writes=%d: a store that cannot be read must not be overwritten", err, st.writes)
	}
}
