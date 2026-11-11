package findingsview

import (
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

// The four reads behind a report. Each is what the operator's stores already offer, so a handler and a tool hand the same
// stores in and neither keeps a reader of its own.
type (
	// Baselines reads an automation's latest scan. *persistence.WorkspaceManager satisfies it.
	Baselines interface {
		ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
	}
	// Acks lists a workspace's accepted findings.
	Acks interface {
		List(workspaceID string) ([]models.ScanAck, error)
	}
	// Fixes lists a workspace's fix records.
	Fixes interface {
		List(workspaceID string) ([]models.FixRecord, error)
	}
	// Life lists the history of each finding.
	Life interface {
		ReadLife(workspaceID string) ([]models.FindingLife, error)
	}
	// Deep lists the last deep check of each device.
	Deep interface {
		List(workspaceID string) ([]models.DeepCheck, error)
	}
)

// Reader reads a report. Acks, Fixes and Deep are optional; one that is absent or unreadable adds nothing, so a finding is
// shown without it rather than not at all.
type Reader struct {
	Baselines Baselines
	Acks      Acks
	Fixes     Fixes
	Deep      Deep
	Life      Life
	Guide     func(string) (netaudit.FixGuide, bool)
}

// Read builds the report of one audit. Only the baseline can fail it.
func (r Reader) Read(workspaceID, automation string) (Report, error) {
	b, err := r.Baselines.ReadBaseline(workspaceID, automation)
	if err != nil {
		return Report{}, err
	}
	return Of(Input{Baseline: b, Acks: r.acks(workspaceID), Fixes: r.fixes(workspaceID), DeepAt: r.deepAt(workspaceID), Life: r.life(workspaceID), Guide: r.Guide, Now: time.Now()}), nil
}

func (r Reader) acks(workspaceID string) map[string]models.ScanAck {
	out := map[string]models.ScanAck{}
	if r.Acks == nil {
		return out
	}
	list, err := r.Acks.List(workspaceID)
	if err != nil {
		return out
	}
	for _, a := range list {
		out[a.Key] = a
	}
	return out
}

func (r Reader) fixes(workspaceID string) map[string]models.FixRecord {
	out := map[string]models.FixRecord{}
	if r.Fixes == nil {
		return out
	}
	list, err := r.Fixes.List(workspaceID)
	if err != nil {
		return out
	}
	for _, f := range list {
		out[f.Key] = f
	}
	return out
}

func (r Reader) deepAt(workspaceID string) map[string]time.Time {
	if r.Deep == nil {
		return nil
	}
	list, err := r.Deep.List(workspaceID)
	if err != nil {
		return nil
	}
	out := make(map[string]time.Time, len(list))
	for _, c := range list {
		out[c.Device] = c.At
	}
	return out
}

func (r Reader) life(workspaceID string) map[string]models.FindingLife {
	if r.Life == nil {
		return nil
	}
	list, err := r.Life.ReadLife(workspaceID)
	if err != nil {
		return nil
	}
	out := make(map[string]models.FindingLife, len(list))
	for _, l := range list {
		out[l.Key] = l
	}
	return out
}
