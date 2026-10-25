package netaudit

import (
	"errors"
	"testing"
	"time"

	"llm-proxy/models"
)

var ackNow = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func TestNewAck(t *testing.T) {
	f := finding("10.0.0.1", 22, "OpenSSH 9.6")
	a, err := NewAck(f, "  jump host  ", 0, ackNow)
	if err != nil {
		t.Fatal(err)
	}
	if a.Key != f.Key() || a.AcceptedVersion != "OpenSSH 9.6" || a.Note != "jump host" || !a.ExpiresAt.Equal(ackNow.AddDate(0, 0, DefaultAckDays)) {
		t.Errorf("ack = %+v", a)
	}
	for _, days := range []int{-1, MaxAckDays + 1} {
		if _, err := NewAck(f, "", days, ackNow); !errors.Is(err, ErrBadAck) {
			t.Errorf("days %d: %v", days, err)
		}
	}
	long := make([]rune, maxNoteLen+1)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := NewAck(f, string(long), 1, ackNow); !errors.Is(err, ErrBadAck) {
		t.Errorf("a long note: %v", err)
	}
}

func TestStateFor(t *testing.T) {
	f := finding("10.0.0.1", 22, "OpenSSH 9.6")
	a, _ := NewAck(f, "", 10, ackNow)
	tests := []struct {
		name string
		f    Finding
		now  time.Time
		want AckState
	}{
		{"covered", f, ackNow.AddDate(0, 0, 5), AckActive},
		{"on the expiry instant it is over", f, a.ExpiresAt, AckExpired},
		{"after expiry", f, a.ExpiresAt.Add(time.Hour), AckExpired},
		{"the version changed", finding("10.0.0.1", 22, "OpenSSH 9.7"), ackNow.AddDate(0, 0, 5), AckVersionChanged},
	}
	for _, tt := range tests {
		if got := StateFor(a, tt.f, tt.now); got != tt.want {
			t.Errorf("%s: %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestApplyAcks_HidesOnlyWhatIsCoveredAndAccounts(t *testing.T) {
	ssh, db, web := finding("10.0.0.1", 22, "OpenSSH 9.6"), finding("10.0.0.2", 3306, "MySQL 8"), finding("10.0.0.3", 80, "")
	sshAck, _ := NewAck(ssh, "ok", 30, ackNow.AddDate(0, 0, -1))
	dbAck, _ := NewAck(finding("10.0.0.2", 3306, "MySQL 5"), "old", 30, ackNow.AddDate(0, 0, -1)) // accepted another version

	got := ApplyAcks([]Finding{ssh, db, web}, []models.ScanAck{sshAck, dbAck}, ackNow)
	if len(got.Accepted) != 1 || got.Accepted[0].Key() != ssh.Key() {
		t.Errorf("accepted = %+v", got.Accepted)
	}
	if len(got.Visible) != 2 || got.Visible[0].Key() != db.Key() || got.Visible[1].Key() != web.Key() {
		t.Errorf("visible = %+v (a changed version comes back, with the rest in order)", got.Visible)
	}
	for _, a := range got.Acks {
		if !a.LastSeen.Equal(ackNow) {
			t.Errorf("a finding that was seen refreshes its acceptance: %+v", a)
		}
	}
}

func TestApplyAcks_ExpiredComesBackAndGoneForAMonthIsDropped(t *testing.T) {
	f := finding("10.0.0.1", 22, "v1")
	expired, _ := NewAck(f, "", 1, ackNow.AddDate(0, 0, -5))
	if got := ApplyAcks([]Finding{f}, []models.ScanAck{expired}, ackNow); len(got.Visible) != 1 || len(got.Accepted) != 0 {
		t.Errorf("an expired acceptance does not hide the finding: %+v", got)
	}

	gone, _ := NewAck(finding("10.0.0.9", 21, ""), "", 365, ackNow.AddDate(0, 0, -40))
	recent, _ := NewAck(finding("10.0.0.8", 21, ""), "", 365, ackNow.AddDate(0, 0, -10))
	got := ApplyAcks(nil, []models.ScanAck{gone, recent}, ackNow)
	if len(got.Acks) != 1 || got.Acks[0].Key != recent.Key {
		t.Errorf("an acceptance for something no scan has shown for over 30 days is dropped: %+v", got.Acks)
	}
}

func TestApplyAcks_DoesNotChangeTheCallersList(t *testing.T) {
	f := finding("10.0.0.1", 22, "")
	a, _ := NewAck(f, "", 30, ackNow.AddDate(0, 0, -2))
	before := a.LastSeen
	acks := []models.ScanAck{a}
	ApplyAcks([]Finding{f}, acks, ackNow)
	if !acks[0].LastSeen.Equal(before) {
		t.Error("ApplyAcks must work on a copy")
	}
}

func TestUpsertAndRemoveAck(t *testing.T) {
	f := finding("10.0.0.1", 22, "v1")
	first, _ := NewAck(f, "first", 10, ackNow)
	second, _ := NewAck(f, "second", 20, ackNow)
	other, _ := NewAck(finding("10.0.0.2", 80, ""), "", 10, ackNow)

	acks := UpsertAck(UpsertAck(nil, first), other)
	acks = UpsertAck(acks, second)
	if len(acks) != 2 || acks[len(acks)-1].Note != "second" {
		t.Errorf("a second acceptance of the same finding replaces the first: %+v", acks)
	}
	acks, ok := RemoveAck(acks, f.Key())
	if !ok || len(acks) != 1 || acks[0].Key != other.Key {
		t.Errorf("remove = %+v, %v", acks, ok)
	}
	if _, ok := RemoveAck(acks, "nope"); ok {
		t.Error("removing nothing reports nothing")
	}
}

func TestHideAccepted_TheModelNeverSeesWhatTheOperatorAccepted(t *testing.T) {
	ssh, telnet := finding("10.0.0.1", 22, "OpenSSH 9"), finding("10.0.0.1", 23, "")
	res := Result{Engine: "nmap", Hosts: hostsWith(ssh, telnet), Findings: []Finding{ssh, telnet}, Warnings: []string{"w"}}
	ack, _ := NewAck(ssh, "", 30, ackNow.AddDate(0, 0, -1))

	got, hidden := HideAccepted(res, []models.ScanAck{ack}, ackNow)
	if hidden != 1 || len(got.Hosts[0].Ports) != 1 || got.Hosts[0].Ports[0].Number != 23 {
		t.Errorf("hosts = %+v, hidden %d", got.Hosts, hidden)
	}
	if len(got.Findings) != 1 || got.Findings[0].Port != 23 || got.Engine != "nmap" || len(got.Warnings) != 1 {
		t.Errorf("findings and the rest of the result follow: %+v", got)
	}
	if len(res.Hosts[0].Ports) != 2 || len(res.Findings) != 2 {
		t.Error("the input result must not be changed (the report is built from the full one)")
	}

	if same, n := HideAccepted(res, nil, ackNow); n != 0 || len(same.Hosts[0].Ports) != 2 {
		t.Error("no acceptances hide nothing")
	}
	changed := finding("10.0.0.1", 22, "OpenSSH 10")
	res2 := Result{Hosts: hostsWith(changed), Findings: []Finding{changed}}
	if _, n := HideAccepted(res2, []models.ScanAck{ack}, ackNow); n != 0 {
		t.Error("an acceptance of another version hides nothing")
	}
}
