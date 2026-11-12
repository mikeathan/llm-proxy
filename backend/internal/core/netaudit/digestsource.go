package netaudit

import (
	"context"
	"time"

	"llm-proxy/models"
)

// digestStore is what the digest reads of a workspace. *persistence.WorkspaceManager satisfies it.
type digestStore interface {
	ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
	ReadScanLog(workspaceID string) ([]models.ScanEntry, error)
	ReadLife(workspaceID string) ([]models.FindingLife, error)
	ReadAcks(workspaceID string) ([]models.ScanAck, error)
}

// DigestSource builds the window digest for an audit run from the records the program keeps. It reads, and starts nothing: no
// scan, no probe, no router read. Inventory, Alerts and Series are optional; a part that is absent adds nothing.
type DigestSource struct {
	Store     digestStore
	Inventory interface {
		View() (models.DeviceInventory, error)
	}
	Alerts interface {
		After(id int64, limit int) ([]models.NetworkAlert, error)
	}
	Series interface {
		Series(n int) ([]models.NetworkDay, error)
	}
	Now func() time.Time
}

// seriesDays covers the longest window (a week) with a day to spare.
const seriesDays = 8

// Build reads the records of one audit of one workspace and returns the digest, checked against current (the scan being made, nil when
// there is none). The window comes from the run's scan settings in ctx; ok is false when the run is not an audit (an ordinary chat has
// no scan settings, so no window) or there is no store. The workspace and the audit are passed, not read from ctx: the run's own context
// does not carry the workspace (a tool call adds it). A part that cannot be read is named in the digest's Unread, so it is never read as
// a quiet network.
func (s DigestSource) Build(ctx context.Context, workspaceID, automation string, current *CurrentScan) (WindowDigest, bool) {
	settings, ok := models.ScanSettingsFrom(ctx)
	if !ok || s.Store == nil {
		return WindowDigest{}, false
	}
	ws := workspaceID
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	in := DigestInput{Now: now, Window: settings.WindowDuration(), Automation: automation, Current: current}
	var unread []string
	note := func(name string, err error) {
		if err != nil {
			unread = append(unread, name)
		}
	}
	var err error
	in.Baseline, err = s.Store.ReadBaseline(ws, automation)
	note("latest saved scan", err)
	in.Log, err = s.Store.ReadScanLog(ws)
	note("scan log", err)
	in.Life, err = s.Store.ReadLife(ws)
	note("finding history", err)
	acks, err := s.Store.ReadAcks(ws)
	note("accepted findings", err)
	in.Acks = make(map[string]models.ScanAck, len(acks))
	for _, a := range acks {
		in.Acks[a.Key] = a
	}
	if s.Inventory != nil {
		in.Inventory, err = s.Inventory.View()
		note("device inventory", err)
	}
	if s.Alerts != nil {
		in.Alerts, err = s.Alerts.After(0, MaxActivityRows)
		note("alert log", err)
	}
	if s.Series != nil {
		in.Series, err = s.Series.Series(seriesDays)
		note("daily history", err)
	}
	d := DigestOf(in)
	d.Unread = unread
	return d, true
}

// Digest is the rendered digest, fitted to budget characters (see RenderDigest), or "" when the run is not an audit.
func (s DigestSource) Digest(ctx context.Context, current *CurrentScan, budget int) string {
	d, ok := s.Build(ctx, models.GetWorkspaceID(ctx), models.GetTaskName(ctx), current)
	if !ok {
		return ""
	}
	return RenderDigest(d, budget)
}
