package netaudit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"llm-proxy/models"
)

// digestEvery is how often the summary is sent, and digestLookback how far back "a week ago" is looked for in the history.
const (
	digestEvery    = 7 * 24 * time.Hour
	digestLookback = 8
)

// DigestMessage is the weekly summary: counts and fixed words only. It names no device and no address, and nothing a
// device sent, so it can go to any connector.
func DigestMessage(inv models.DeviceInventory, series []models.NetworkDay, now time.Time) string {
	known, fresh, unseen, unnamed := 0, 0, 0, 0
	for _, d := range inv.Devices {
		known++
		if inv.Baselined && now.Sub(d.FirstSeen) <= digestEvery {
			fresh++
		}
		if now.Sub(d.LastSeen) > digestEvery {
			unseen++
		}
		if d.Name == "" {
			unnamed++
		}
	}
	parts := []string{fmt.Sprintf("%s known (%d new this week, %d not seen this week), %d without a name.", plural(known, "device", "devices"), fresh, unseen, unnamed)}
	parts = append(parts, digestFindings(series, now))
	return "Network summary for the week: " + strings.Join(parts, " ") + " Open the Network page for details."
}

// digestFindings says how many findings are open at the latest audit and, if the history reaches back a week, how many were.
func digestFindings(series []models.NetworkDay, now time.Time) string {
	var latest *models.NetworkDay
	for i := len(series) - 1; i >= 0; i-- {
		if series[i].Audited {
			latest = &series[i]
			break
		}
	}
	if latest == nil {
		return "No network audit has run yet."
	}
	out := fmt.Sprintf("Open findings: %d", latest.Findings)
	weekAgo := now.AddDate(0, 0, -7).Format(historyDateLayout)
	for _, d := range series {
		if d.Audited && d.Date <= weekAgo && d.Date != latest.Date {
			out += fmt.Sprintf(" (was %d a week ago)", d.Findings)
			break
		}
	}
	return out + fmt.Sprintf(", %d serious.", latest.Serious)
}

// DigestHistory is the part of *History the summary uses: the daily counts and when the last summary was sent.
type DigestHistory interface {
	Series(n int) ([]models.NetworkDay, error)
	LastDigest() (time.Time, error)
	MarkDigest(at time.Time) error
}

// DigestDeps are what the summary needs; the application wires them in one place.
type DigestDeps struct {
	Enabled func() bool
	View    func() (models.DeviceInventory, error)
	History DigestHistory
	Notify  func(ctx context.Context, message string) error
	Now     func() time.Time
	OnError func(error)
}

// WeeklyDigest sends the weekly summary. It makes no network request of its own and adds no probe: it reads what the
// presence check and the audit already keep.
type WeeklyDigest struct {
	deps DigestDeps
	last string // the trouble reported last, so a repeating failure is reported once
}

// NewWeeklyDigest builds the summary. Nothing runs until Tick.
func NewWeeklyDigest(deps DigestDeps) *WeeklyDigest {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &WeeklyDigest{deps: deps}
}

// Tick sends the summary if it is switched on and a week has passed since the last one. A summary that could not be sent
// is not marked as sent, so it goes out as soon as it can.
func (d *WeeklyDigest) Tick(ctx context.Context) {
	if !d.deps.Enabled() {
		return
	}
	now := d.deps.Now()
	last, err := d.deps.History.LastDigest()
	if err != nil {
		d.trouble(err)
		return
	}
	if !last.IsZero() && now.Sub(last) < digestEvery {
		return
	}
	inv, err := d.deps.View()
	if err != nil {
		d.trouble(err)
		return
	}
	series, err := d.deps.History.Series(maxHistoryDays)
	if err != nil {
		d.trouble(err)
		return
	}
	if err := d.deps.Notify(ctx, DigestMessage(inv, series, now)); err != nil {
		d.trouble(fmt.Errorf("weekly summary: could not send: %w", err))
		return
	}
	d.last = ""
	if err := d.deps.History.MarkDigest(now); err != nil {
		d.trouble(err)
	}
}

func (d *WeeklyDigest) trouble(err error) {
	if err.Error() == d.last {
		return
	}
	d.last = err.Error()
	if d.deps.OnError != nil {
		d.deps.OnError(err)
	}
}

// SummaryConfig reads and changes whether the weekly summary is on.
type SummaryConfig struct{ store PolicyStore }

func NewSummaryConfig(store PolicyStore) *SummaryConfig { return &SummaryConfig{store: store} }

// Enabled reports whether the weekly summary is switched on.
func (c *SummaryConfig) Enabled() bool { return c.store.Get().WeeklySummary }

// Set switches the weekly summary on or off.
func (c *SummaryConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.WeeklySummary = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the weekly summary setting: %w", err)
	}
	return nil
}
