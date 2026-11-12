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

// Digest is the rendered digest for the run in ctx, or "" when the run is not an audit (an ordinary chat has no scan settings, so
// no window and no audit name). A part that cannot be read is named in the digest, so the model never reads it as a quiet network.
func (s DigestSource) Digest(ctx context.Context) string {
	settings, ok := models.ScanSettingsFrom(ctx)
	if !ok || s.Store == nil {
		return ""
	}
	ws, automation := models.GetWorkspaceID(ctx), models.GetTaskName(ctx)
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	in := DigestInput{Now: now, Window: settings.WindowDuration(), Automation: automation}
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
	return RenderDigest(d)
}
