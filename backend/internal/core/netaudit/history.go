package netaudit

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"llm-proxy/models"
)

// maxHistoryDays bounds the history: about 13 months of one small entry per day.
const maxHistoryDays = 400

const historyDateLayout = "2006-01-02"

// HistoryStore keeps the history. A missing file is an empty history.
type HistoryStore interface {
	Read() (models.NetworkHistory, error)
	Write(models.NetworkHistory) error
}

// History records the daily counts the Network page draws its trends from. It is fed by the looks the host already takes
// (the presence check and the audit); it never probes anything itself, and it writes only when a day's numbers change.
type History struct {
	mu    sync.Mutex
	store HistoryStore
	now   func() time.Time
}

// NewHistory builds the recorder over a store.
func NewHistory(store HistoryStore) *History { return &History{store: store, now: time.Now} }

// RecordLook notes one look at the network: how many devices answered, how many the inventory knows, how many of those
// have no name.
func (h *History) RecordLook(up, devices, unnamed int) error {
	return h.edit(func(d *models.NetworkDay) {
		d.DevicesUp = max(d.DevicesUp, up)
		d.Devices, d.Unnamed = devices, unnamed
	})
}

// RecordAudit notes the open findings of an audit that just finished.
func (h *History) RecordAudit(findings []models.ScanFinding) error {
	serious := 0
	for _, f := range findings {
		if f.Severity == SeverityHigh || f.Severity == SeverityCritical {
			serious++
		}
	}
	return h.edit(func(d *models.NetworkDay) {
		d.Findings, d.Serious, d.Audited = len(findings), serious, true
	})
}

// Series returns the last n days that have an entry, oldest first.
func (h *History) Series(n int) ([]models.NetworkDay, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hist, err := h.store.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read the network history: %w", err)
	}
	days := hist.Days
	if n >= 0 && len(days) > n {
		days = days[len(days)-n:]
	}
	return slices.Clone(days), nil
}

// edit applies a change to today's entry, creating it, and writes only if the entry changed.
func (h *History) edit(change func(*models.NetworkDay)) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	hist, err := h.store.Read()
	if err != nil {
		return fmt.Errorf("failed to read the network history: %w", err)
	}
	today := h.now().Format(historyDateLayout)
	i := slices.IndexFunc(hist.Days, func(d models.NetworkDay) bool { return d.Date == today })
	created := i < 0
	if created {
		hist.Days = append(hist.Days, models.NetworkDay{Date: today})
		slices.SortFunc(hist.Days, func(a, b models.NetworkDay) int { return compareDates(a.Date, b.Date) })
		i = slices.IndexFunc(hist.Days, func(d models.NetworkDay) bool { return d.Date == today })
	}
	before := hist.Days[i]
	change(&hist.Days[i])
	if !created && hist.Days[i] == before {
		return nil
	}
	if len(hist.Days) > maxHistoryDays {
		hist.Days = hist.Days[len(hist.Days)-maxHistoryDays:]
	}
	return h.store.Write(hist)
}

func compareDates(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// LastDigest is when the last weekly summary was sent; the zero time if none was.
func (h *History) LastDigest() (time.Time, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hist, err := h.store.Read()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read the network history: %w", err)
	}
	return hist.DigestAt, nil
}

// MarkDigest records that a weekly summary was sent at the given time.
func (h *History) MarkDigest(at time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	hist, err := h.store.Read()
	if err != nil {
		return fmt.Errorf("failed to read the network history: %w", err)
	}
	hist.DigestAt = at
	return h.store.Write(hist)
}
