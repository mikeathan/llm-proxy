package netaudit

import (
	"fmt"
	"time"

	"llm-proxy/models"
)

// CheckDecoyTouched is a device connecting to a decoy service: a fake service on a high port that nothing legitimate uses.
const CheckDecoyTouched = "decoy_touched"

// DecoyReportFor is how long a touch stays in the report after the last connection. The operator can accept it sooner.
const DecoyReportFor = 30 * 24 * time.Hour

// DecoyFindings turns the stored touches into findings, keyed by the source address and the decoy's port. The wording is
// fixed: only the count and the dates, which this program wrote, are in it.
func DecoyFindings(touches []models.DecoyTouch, now time.Time) []Finding {
	var out []Finding
	for _, t := range touches {
		if now.Sub(t.Last) > DecoyReportFor {
			continue
		}
		times := "once"
		if t.Count > 1 {
			times = fmt.Sprintf("%d times", t.Count)
		}
		out = append(out, Finding{
			Device: t.Source, Port: t.Port, Proto: "tcp", Check: CheckDecoyTouched, Severity: SeverityHigh,
			Evidence: fmt.Sprintf("This device connected to a decoy service on port %d, a fake service nothing legitimate uses (%s, last %s). It is probing the network for services", t.Port, times, t.Last.Format(dateLayout)),
		})
	}
	return out
}

// DecoyMessage is the alert for devices that connected to a decoy: how many, and their addresses, which are the connection's
// remote address as the kernel reports it. Nothing the visitor sent, and no name, is in it.
func DecoyMessage(sources []string) string {
	return "Network audit: " + plural(len(sources), "device", "devices") + " connected to a decoy service (a fake service nothing legitimate uses)" +
		listed(sources) + ". Open the network audit to review."
}

// DecoyReader reads the stored touches: *persistence.DecoyTouchStore.
type DecoyReader interface {
	Read() ([]models.DecoyTouch, error)
}

// DecoyConfig reads and changes whether the decoy services run.
type DecoyConfig struct{ store PolicyStore }

func NewDecoyConfig(store PolicyStore) *DecoyConfig { return &DecoyConfig{store: store} }

// Enabled reports whether the decoys are switched on.
func (c *DecoyConfig) Enabled() bool { return c.store.Get().DecoysEnabled }

// Set switches the decoys on or off.
func (c *DecoyConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.DecoysEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the decoy setting: %w", err)
	}
	return nil
}
