package netaudit

import (
	"errors"
	"testing"
	"time"

	"llm-proxy/models"
)

type baselineReader struct{ b models.ScanBaseline }

func (r baselineReader) ReadBaseline(string, string) (models.ScanBaseline, error) { return r.b, nil }

func newAckService(findings ...Finding) (*Acks, *memAcks) {
	store := &memAcks{}
	svc := NewAcks(store, baselineReader{models.ScanBaseline{ScannedAt: ackNow, Findings: findings}})
	svc.now = func() time.Time { return ackNow }
	return svc, store
}

func TestAcks_AcceptTakesTheVersionTheLatestScanShows(t *testing.T) {
	f := finding("10.0.0.1", 22, "OpenSSH 9.6")
	svc, store := newAckService(f, finding("10.0.0.2", 80, ""))

	ack, err := svc.Accept("ws", "nightly", f.Key(), "jump host", 30)
	if err != nil {
		t.Fatal(err)
	}
	if ack.AcceptedVersion != "OpenSSH 9.6" || len(store.acks) != 1 || store.acks[0].Note != "jump host" {
		t.Errorf("ack = %+v, stored %+v", ack, store.acks)
	}
	if _, err := svc.Accept("ws", "nightly", "10.0.0.9|tcp|1|open_port", "", 0); !errors.Is(err, ErrNoSuchFinding) {
		t.Errorf("a finding the scan did not show: %v", err)
	}
	if _, err := svc.Accept("ws", "nightly", f.Key(), "", 9999); !errors.Is(err, ErrBadAck) {
		t.Errorf("a bad length: %v", err)
	}
}

func TestAcks_RevokeAndList(t *testing.T) {
	f := finding("10.0.0.1", 22, "")
	svc, _ := newAckService(f)
	if _, err := svc.Accept("ws", "nightly", f.Key(), "", 0); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.List("ws"); len(got) != 1 {
		t.Fatalf("list = %+v", got)
	}
	if ok, err := svc.Revoke("ws", f.Key()); !ok || err != nil {
		t.Errorf("revoke = %v, %v", ok, err)
	}
	if ok, _ := svc.Revoke("ws", f.Key()); ok {
		t.Error("revoking twice reports nothing to revoke")
	}
	if got, _ := svc.List("ws"); len(got) != 0 {
		t.Errorf("list after revoke = %+v", got)
	}
}
