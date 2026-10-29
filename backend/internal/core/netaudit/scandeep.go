package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"llm-proxy/models"
)

// DeepBudget is the longest a deep check of one device may take: the engine's per-device limit and its script allowance,
// with a little margin. The check runs under this deadline whatever the engine does.
const DeepBudget = 6 * time.Minute

// MaxDeepPorts is the most ports one deep check looks at, so a device that shows many open ports cannot make it long.
const MaxDeepPorts = 16

var (
	// ErrBadDevice means the deep check was asked for something that is not one IPv4 address.
	ErrBadDevice = errors.New("a deep check is of one device, named by its IPv4 address")
	// ErrNoPorts means there is nothing to look at: the device has no open port to check.
	ErrNoPorts = errors.New("there is no open port to check on this device")
)

// DeepScan runs the deep check (ModeDeep) on one device and the ports given, and returns the deep findings. It is the only
// way to run an intrusive mode (Constitution II.3, "Intrusive runs"): the ordinary Scan refuses them.
//
// It refuses before the network is touched when the run is unattended, the device is not one IPv4 address, there are no
// ports or too many, the device is outside the scan scope right now, another scan is running, or nmap cannot be used (the
// limited engine never stands in: it cannot do this). There is no sweep: the device is known, and it is the only thing
// touched. Findings are the deep checks only; open ports and the nightly checks belong to the audit's own scan.
func (s *Service) DeepScan(ctx context.Context, device string, ports []int) (Result, error) {
	if models.IsUnattendedRun(ctx) {
		return Result{}, fmt.Errorf("%w: a deep check is started by the operator, never by a scheduled run", ErrIntrusiveRefused)
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(device))
	if err != nil || !addr.Is4() {
		return Result{}, ErrBadDevice
	}
	portList, err := deepPortList(ports)
	if err != nil {
		return Result{}, err
	}
	addrs, err := s.authorizer.Authorize(ctx, []string{addr.String()})
	if err != nil {
		return Result{}, err
	}
	if len(addrs) != 1 || addrs[0] != addr {
		return Result{}, ErrBadDevice
	}
	if !s.running.TryLock() {
		return Result{}, ErrScanBusy
	}
	defer s.running.Unlock()

	if unavailable := s.primary.Available(ctx); unavailable != nil || !s.primary.Supports(ModeDeep) {
		if unavailable == nil {
			unavailable = fmt.Errorf("%s does not support a deep check", s.primary.Name())
		}
		return Result{}, fmt.Errorf("%w: %v", ErrNmapRequired, unavailable)
	}
	scan := ScanRequest{Targets: addrs, Mode: ModeDeep, Ports: portList, Timeout: DeepBudget}
	if s.deviceTimeout != nil {
		scan.HostTimeout = s.deviceTimeout()
	}
	runCtx, cancel := context.WithTimeout(ctx, DeepBudget)
	defer cancel()
	res, err := s.primary.Scan(runCtx, scan)
	if err != nil {
		return Result{}, err
	}
	res.Engine = s.primary.Name()
	res.Findings = DeepFindingsOf(res.Hosts)
	return res, nil
}

// deepPortList checks the ports and writes them as the comma list nmap takes, sorted and without repeats.
func deepPortList(ports []int) (string, error) {
	if len(ports) == 0 {
		return "", ErrNoPorts
	}
	clean := slices.Clone(ports)
	slices.Sort(clean)
	clean = slices.Compact(clean)
	if len(clean) > MaxDeepPorts {
		return "", fmt.Errorf("%w: at most %d ports at a time", ErrBadPorts, MaxDeepPorts)
	}
	parts := make([]string, len(clean))
	for i, p := range clean {
		if p < 1 || p > maxPort {
			return "", fmt.Errorf("%w: %d is outside 1-%d", ErrBadPorts, p, maxPort)
		}
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ","), nil
}
