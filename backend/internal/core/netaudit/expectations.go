package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"llm-proxy/models"
)

// Checks an expectation produces. They are judged from what the scan already found: nothing is probed for them.
const (
	CheckUnexpectedPort     = "unexpected_port"
	CheckExpectedPortClosed = "expected_port_closed"
)

const (
	maxExpectedPorts   = 64
	maxExpectedDevices = 256
)

// ErrBadExpectation means a request to set a device's expectation is not valid.
var ErrBadExpectation = errors.New("invalid expectation")

// NewExpectation checks and tidies what the operator says a device should offer: a plain IPv4 address, up to
// maxExpectedPorts distinct TCP ports (sorted), and a short note. An empty list is allowed: it says nothing should answer.
func NewExpectation(device string, ports []int, note string, now time.Time) (models.Expectation, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(device))
	if err != nil || !addr.Is4() {
		return models.Expectation{}, fmt.Errorf("%w: the device must be an IPv4 address", ErrBadExpectation)
	}
	clean := make([]int, 0, len(ports))
	for _, p := range ports {
		if p < 1 || p > 65535 {
			return models.Expectation{}, fmt.Errorf("%w: port %d is not between 1 and 65535", ErrBadExpectation, p)
		}
		if !slices.Contains(clean, p) {
			clean = append(clean, p)
		}
	}
	if len(clean) > maxExpectedPorts {
		return models.Expectation{}, fmt.Errorf("%w: at most %d ports per device", ErrBadExpectation, maxExpectedPorts)
	}
	slices.Sort(clean)
	note = strings.TrimSpace(note)
	if len([]rune(note)) > maxNoteLen {
		return models.Expectation{}, fmt.Errorf("%w: the note is longer than %d characters", ErrBadExpectation, maxNoteLen)
	}
	return models.Expectation{Device: addr.String(), Ports: clean, Note: note, SetAt: now}, nil
}

// ExpectationFindings compares what the scan saw on each device that has an expectation with that expectation. A device
// the scan did not see, did not finish, or found down is left alone: its ports are unknown, and unknown is not "none".
// The wording is fixed text; nothing the device sent is in it.
func ExpectationFindings(expectations []models.Expectation, hosts []Host) []Finding {
	byDevice := make(map[string]models.Expectation, len(expectations))
	for _, e := range expectations {
		byDevice[e.Device] = e
	}
	var out []Finding
	for _, h := range hosts {
		exp, ok := byDevice[h.Addr.String()]
		if !ok || !h.Up || h.Unscanned {
			continue
		}
		out = append(out, judgeDevice(exp, h)...)
	}
	return out
}

// judgeDevice lists the open ports the expectation does not name, then the named ports that did not answer, each in port order.
func judgeDevice(exp models.Expectation, h Host) []Finding {
	var open []int
	for _, p := range h.Ports {
		if p.State == "open" && p.Proto == "tcp" {
			open = append(open, p.Number)
		}
	}
	slices.Sort(open)
	var out []Finding
	for _, n := range open {
		if !slices.Contains(exp.Ports, n) {
			out = append(out, Finding{Device: exp.Device, Port: n, Proto: "tcp", Check: CheckUnexpectedPort, Severity: SeverityMedium,
				Evidence: fmt.Sprintf("Port %d is open but is not on this device's expected list", n)})
		}
	}
	listed := slices.Clone(exp.Ports)
	slices.Sort(listed)
	for _, n := range listed {
		if !slices.Contains(open, n) {
			out = append(out, Finding{Device: exp.Device, Port: n, Proto: "tcp", Check: CheckExpectedPortClosed, Severity: SeverityLow,
				Evidence: fmt.Sprintf("Port %d is on this device's expected list but did not answer", n)})
		}
	}
	return out
}

// ExpectationStore keeps a workspace's device expectations. *persistence.WorkspaceManager satisfies it.
type ExpectationStore interface {
	ReadExpectations(workspaceID string) ([]models.Expectation, error)
	WriteExpectations(workspaceID string, items []models.Expectation) error
}

// TargetChecker says which targets the scan policy would refuse. *Scope satisfies it.
type TargetChecker interface {
	CheckTargets(ctx context.Context, targets []string) []TargetProblem
}

// Expectations is what the operator does with device expectations: set one, remove one, list them. An expectation only
// changes what is reported; it never widens a scan.
type Expectations struct {
	store   ExpectationStore
	targets TargetChecker
	now     func() time.Time
}

func NewExpectations(store ExpectationStore, targets TargetChecker) *Expectations {
	return &Expectations{store: store, targets: targets, now: time.Now}
}

// List returns the workspace's expectations.
func (e *Expectations) List(workspaceID string) ([]models.Expectation, error) {
	return e.store.ReadExpectations(workspaceID)
}

// Set records what a device should offer, replacing any earlier entry for it. The device must be one a scan may look at.
func (e *Expectations) Set(ctx context.Context, workspaceID, device string, ports []int, note string) (models.Expectation, error) {
	exp, err := NewExpectation(device, ports, note, e.now())
	if err != nil {
		return models.Expectation{}, err
	}
	if problems := e.targets.CheckTargets(ctx, []string{exp.Device}); len(problems) > 0 {
		return models.Expectation{}, fmt.Errorf("%w: %s %s", ErrBadExpectation, exp.Device, problems[0].Reason)
	}
	items, err := e.store.ReadExpectations(workspaceID)
	if err != nil {
		return models.Expectation{}, err
	}
	items = slices.DeleteFunc(items, func(x models.Expectation) bool { return x.Device == exp.Device })
	if len(items) >= maxExpectedDevices {
		return models.Expectation{}, fmt.Errorf("%w: at most %d devices", ErrBadExpectation, maxExpectedDevices)
	}
	if err := e.store.WriteExpectations(workspaceID, append(items, exp)); err != nil {
		return models.Expectation{}, err
	}
	return exp, nil
}

// Remove takes a device's expectation back; ok is false when there was none.
func (e *Expectations) Remove(workspaceID, device string) (bool, error) {
	items, err := e.store.ReadExpectations(workspaceID)
	if err != nil {
		return false, err
	}
	left := slices.DeleteFunc(slices.Clone(items), func(x models.Expectation) bool { return x.Device == device })
	if len(left) == len(items) {
		return false, nil
	}
	return true, e.store.WriteExpectations(workspaceID, left)
}
