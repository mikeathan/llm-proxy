package netaudit

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"llm-proxy/internal/platform/scanscope"
	"llm-proxy/models"
)

// MaxScanTargets bounds how many entries an audit's target list may hold.
const MaxScanTargets = 32

const maxTargetLen = 253

var hostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

// ValidateScanSettings refuses settings a scan could never use, before they are saved. A target is checked for being a
// scannable address, block or range, or a plausible host name (names are not looked up here: the network may change
// before the audit runs). Whether a target is inside the allowed networks is decided when the audit runs.
func ValidateScanSettings(s models.ScanSettings) error {
	var errs []error
	if s.Depth != "" && s.Depth != models.ScanDepthQuick && s.Depth != models.ScanDepthThorough {
		errs = append(errs, fmt.Errorf("depth must be %q or %q", models.ScanDepthQuick, models.ScanDepthThorough))
	}
	if s.Ports != "" {
		if _, err := ValidatePorts(s.Ports); err != nil {
			errs = append(errs, fmt.Errorf("ports: %w", err))
		}
	}
	if len(s.Targets) > MaxScanTargets {
		errs = append(errs, fmt.Errorf("targets: at most %d entries", MaxScanTargets))
	} else {
		for _, entry := range s.Targets {
			if err := validateTarget(entry); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func validateTarget(entry string) error {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return errors.New("targets: an entry is empty")
	}
	if len(entry) > maxTargetLen {
		return fmt.Errorf("targets: %q is too long", entry[:40])
	}
	_, err := scanscope.Parse(context.Background(), nil, entry)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, scanscope.ErrBadEntry) && hostName.MatchString(entry):
		return nil
	}
	return fmt.Errorf("targets: %w", err)
}

// RequestFromSettings is the scan an audit's settings stand for. The audit's own settings decide the targets, the
// depth, the ports and whether the checks run: nothing the model passes to the scan tool changes them.
func RequestFromSettings(s models.ScanSettings) Request {
	mode := ModeServices
	if s.ChecksOn() {
		mode = ModeChecks
	}
	return Request{Targets: s.Targets, Mode: mode, Ports: s.Ports, Thorough: s.Thorough()}
}
