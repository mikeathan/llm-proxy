package hostwatch

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Limits on what one look reads for the two protection settings.
const (
	maxProtectionBytes = 64 << 10
	maxAptFiles        = 64
)

var (
	// The wording is apt's and ufw's own (the captures in testdata/20auto-upgrades_ubuntu and ufw_conf_ubuntu_disabled).
	autoUpgrade = regexp.MustCompile(`(?i)^\s*APT::Periodic::Unattended-Upgrade\s+"([^"]*)"`)
	ufwEnabled  = regexp.MustCompile(`(?i)^\s*ENABLED\s*=\s*"?([a-z]+)"?\s*$`)
)

// protection reads whether this computer installs security updates by itself and whether its own firewall is switched on.
// Both settings are weak only when they were read and say so: a missing or unreadable file is never reported as weak.
func (h Hardener) protection(out *Hardening) {
	if h.AptDir != "" {
		out.AutoUpdatesOff = h.autoUpdatesOff(out)
	}
	if h.UfwConf != "" {
		out.FirewallOff = h.firewallOff(out)
	}
}

// autoUpdatesOff is true on a computer that has an apt configuration folder when the last Unattended-Upgrade setting, in the
// order apt reads the files, is 0 or empty, or when there is none at all.
func (h Hardener) autoUpdatesOff(out *Hardening) bool {
	entries, err := os.ReadDir(h.AptDir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			out.Unreadable++
		}
		return false // not an apt computer, or the folder cannot be read: nothing is known
	}
	value, set := "", false
	for i, e := range entries {
		if i >= maxAptFiles || !e.Type().IsRegular() {
			continue
		}
		text, ok := readCapped(filepath.Join(h.AptDir, e.Name()), out)
		if !ok {
			return false // one file could not be read, so the effective value is unknown
		}
		if v, found := lastAutoUpgrade(text); found {
			value, set = v, true
		}
	}
	return !set || value == "" || value == "0"
}

// lastAutoUpgrade is the value of the last Unattended-Upgrade setting in one file's text.
func lastAutoUpgrade(text string) (value string, found bool) {
	for _, line := range strings.Split(text, "\n") {
		if m := autoUpgrade.FindStringSubmatch(line); m != nil {
			value, found = strings.TrimSpace(m[1]), true
		}
	}
	return value, found
}

// firewallOff is true when ufw's configuration says ENABLED=no (the last such line wins, as in a shell).
func (h Hardener) firewallOff(out *Hardening) bool {
	text, ok := readCapped(h.UfwConf, out)
	if !ok {
		return false
	}
	value := ""
	for _, line := range strings.Split(text, "\n") {
		if m := ufwEnabled.FindStringSubmatch(line); m != nil {
			value = strings.ToLower(m[1])
		}
	}
	return value == "no"
}

// readCapped reads at most maxProtectionBytes of a file. A file that does not exist is not read and not counted; one that
// exists and cannot be read is counted.
func readCapped(path string, out *Hardening) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			out.Unreadable++
		}
		return "", false
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxProtectionBytes))
	if err != nil {
		out.Unreadable++
		return "", false
	}
	return string(raw), true
}
