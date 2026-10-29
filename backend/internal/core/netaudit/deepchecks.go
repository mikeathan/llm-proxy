package netaudit

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"llm-proxy/models"
)

// DeepCooldown is how long a device is left alone after a deep check, so a click cannot be repeated into a flood. It is
// judged from the stored time of the last check, so a restart does not reset it.
const DeepCooldown = 10 * time.Minute

// maxDeepDevices bounds the stored results: the newest are kept.
const maxDeepDevices = 256

// CooldownError says a device was checked too recently and how long to wait.
type CooldownError struct{ Remaining time.Duration }

func (e *CooldownError) Error() string {
	return fmt.Sprintf("this device was checked a moment ago; try again in %d minute(s)", int(e.Remaining.Minutes())+1)
}

// DeepScanner is the service that runs a deep check: *Service.
type DeepScanner interface {
	DeepScan(ctx context.Context, device string, ports []int) (Result, error)
}

// DeepBaselines reads an automation's latest scan, which says which ports a device had open.
type DeepBaselines interface {
	ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
}

// DeepStore keeps the last deep check of each device in a workspace.
type DeepStore interface {
	ReadDeepChecks(workspaceID string) ([]models.DeepCheck, error)
	WriteDeepChecks(workspaceID string, items []models.DeepCheck) error
}

// DeepChecks is what the operator's request for a deep check goes through: it picks the ports from the latest scan (the
// request names a device and nothing else), keeps a device to one check per cool-down, runs the check and stores the result.
type DeepChecks struct {
	scanner   DeepScanner
	baselines DeepBaselines
	store     DeepStore
	now       func() time.Time
}

func NewDeepChecks(scanner DeepScanner, baselines DeepBaselines, store DeepStore) *DeepChecks {
	return &DeepChecks{scanner: scanner, baselines: baselines, store: store, now: time.Now}
}

// List returns the workspace's stored deep checks, newest first.
func (d *DeepChecks) List(workspaceID string) ([]models.DeepCheck, error) {
	items, err := d.store.ReadDeepChecks(workspaceID)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(items, func(a, b models.DeepCheck) int { return b.At.Compare(a.At) })
	return items, nil
}

// Run checks one device of an automation's latest scan. The ports are the device's open TCP ports in that scan (the lowest
// MaxDeepPorts of them); a device that scan did not see, or ran out of time on, has none to vouch for and is refused.
// A run that fails is not a result and does not start a cool-down.
func (d *DeepChecks) Run(ctx context.Context, workspaceID, automation, device string) (models.DeepCheck, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(device))
	if err != nil || !addr.Is4() {
		return models.DeepCheck{}, ErrBadDevice
	}
	device = addr.String()
	ports, err := d.openPorts(workspaceID, automation, device)
	if err != nil {
		return models.DeepCheck{}, err
	}
	stored, err := d.store.ReadDeepChecks(workspaceID)
	if err != nil {
		return models.DeepCheck{}, err
	}
	now := d.now()
	if last, ok := lastDeepCheck(stored, device); ok && now.Sub(last.At) < DeepCooldown {
		return models.DeepCheck{}, &CooldownError{Remaining: DeepCooldown - now.Sub(last.At)}
	}
	res, err := d.scanner.DeepScan(ctx, device, ports)
	if err != nil {
		return models.DeepCheck{}, err
	}
	check := models.DeepCheck{Device: device, At: now, Ports: deepPortResults(ports, res.Hosts, addr), Findings: res.Findings}
	if err := d.store.WriteDeepChecks(workspaceID, withDeepCheck(stored, check)); err != nil {
		return models.DeepCheck{}, err
	}
	return check, nil
}

// openPorts is the device's open TCP ports in the automation's latest scan, sorted, at most MaxDeepPorts.
func (d *DeepChecks) openPorts(workspaceID, automation, device string) ([]int, error) {
	base, err := d.baselines.ReadBaseline(workspaceID, automation)
	if err != nil {
		return nil, err
	}
	if slices.Contains(base.Unscanned, device) {
		return nil, ErrNoPorts
	}
	var ports []int
	for _, f := range base.Findings {
		if f.Check == CheckOpenPort && f.Device == device && f.Proto == "tcp" && !slices.Contains(ports, f.Port) {
			ports = append(ports, f.Port)
		}
	}
	if len(ports) == 0 {
		return nil, ErrNoPorts
	}
	slices.Sort(ports)
	return ports[:min(len(ports), MaxDeepPorts)], nil
}

func lastDeepCheck(items []models.DeepCheck, device string) (models.DeepCheck, bool) {
	for _, c := range items {
		if c.Device == device {
			return c, true
		}
	}
	return models.DeepCheck{}, false
}

// deepPortResults says, for each port that was looked at, whether a deep script produced output on it.
func deepPortResults(ports []int, hosts []Host, device netip.Addr) []models.DeepPortResult {
	out := make([]models.DeepPortResult, len(ports))
	for i, port := range ports {
		out[i].Port = port
		for _, h := range hosts {
			if h.Addr != device {
				continue
			}
			for _, p := range h.Ports {
				if p.Number != port || p.Proto != "tcp" {
					continue
				}
				for _, s := range p.Scripts {
					answered := strings.TrimSpace(s.Output) != ""
					out[i].TLS = out[i].TLS || (s.ID == "ssl-enum-ciphers" && answered)
					out[i].SSH = out[i].SSH || (s.ID == "ssh-auth-methods" && answered)
				}
			}
		}
	}
	return out
}

// withDeepCheck is the stored list with this check replacing the device's earlier one, newest first, at most
// maxDeepDevices long.
func withDeepCheck(stored []models.DeepCheck, check models.DeepCheck) []models.DeepCheck {
	out := []models.DeepCheck{check}
	for _, c := range stored {
		if c.Device != check.Device {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b models.DeepCheck) int { return b.At.Compare(a.At) })
	return out[:min(len(out), maxDeepDevices)]
}
