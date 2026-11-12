package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

type fakeDigestStore struct {
	base    models.ScanBaseline
	log     []models.ScanEntry
	life    []models.FindingLife
	acks    []models.ScanAck
	logErr  error
	baseErr error
}

func (f fakeDigestStore) ReadBaseline(string, string) (models.ScanBaseline, error) {
	return f.base, f.baseErr
}
func (f fakeDigestStore) ReadScanLog(string) ([]models.ScanEntry, error) { return f.log, f.logErr }
func (f fakeDigestStore) ReadLife(string) ([]models.FindingLife, error)  { return f.life, nil }
func (f fakeDigestStore) ReadAcks(string) ([]models.ScanAck, error)      { return f.acks, nil }

func auditCtx(window string) context.Context {
	ctx := models.WithWorkspaceID(context.Background(), "ws")
	ctx = models.WithTaskName(ctx, "network-audit")
	return models.WithScanSettings(ctx, models.ScanSettings{Window: window})
}

func newSource(store fakeDigestStore) DigestSource {
	return DigestSource{Store: store, Now: func() time.Time { return wdNow }}
}

func TestDigestSource_AnOrdinaryChatScanGetsNoDigest(t *testing.T) {
	ctx := models.WithWorkspaceID(context.Background(), "ws")
	if got := newSource(fakeDigestStore{}).Digest(ctx); got != "" {
		t.Errorf("only an audit run has settings, so only it gets a digest: %q", got)
	}
}

func TestDigestSource_ReadsTheRunsOwnAuditAndWindow(t *testing.T) {
	store := fakeDigestStore{log: []models.ScanEntry{
		{Automation: "network-audit", At: wdAgo(30), DevicesUp: 1},
		{Automation: "network-audit", At: wdAgo(2), DevicesUp: 15},
	}}
	day := newSource(store).Digest(auditCtx(models.ScanWindowDay))
	week := newSource(store).Digest(auditCtx(models.ScanWindowWeek))
	if !strings.Contains(day, "last 24h): 1 scan(s)") || !strings.Contains(week, "last 7d): 2 scan(s)") {
		t.Errorf("window not applied:\n%s\n%s", day, week)
	}
}

func TestDigestSource_APartThatCannotBeReadIsSaidNotLeftOut(t *testing.T) {
	got := newSource(fakeDigestStore{logErr: errors.New("corrupt")}).Digest(auditCtx(""))
	if !strings.Contains(got, `"unread":["scan log"]`) {
		t.Errorf("an unreadable log must be named, never read as a quiet network: %s", got)
	}
}
