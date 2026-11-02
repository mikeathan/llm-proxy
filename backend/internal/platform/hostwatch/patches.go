package hostwatch

import (
	"io"
	"os"
	"regexp"
	"strconv"
	"time"
)

// Patcher reads whether this computer is waiting for a restart and how many updates it has not installed, from the two files
// Ubuntu keeps for its login message. Like the rest of the package it reads files the service's account can read, starts no
// program and keeps no text: the files become a few numbers.
type Patcher struct {
	// RebootFile is "/var/run/reboot-required" and UpdatesFile "/var/lib/update-notifier/updates-available" on Ubuntu. An empty
	// path leaves that part out.
	RebootFile  string
	UpdatesFile string
	// AutoUpgradeLog is "/var/log/unattended-upgrades/unattended-upgrades.log" on Ubuntu (readable by every account). Only the
	// date of the last "Starting unattended upgrades script" line is used. Zone is the time zone its timestamps are written in
	// (the computer's own; time.Local when nil).
	AutoUpgradeLog string
	Zone           *time.Location
	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

// Patches is what one look found.
type Patches struct {
	// RebootRequired is true when the restart marker exists, and RebootDays how many whole days ago it was made.
	RebootRequired bool
	RebootDays     int
	// UpdatesRead is true only when the updates file was read. A computer without the file (not Ubuntu, or the message service
	// is off) has an unknown update state, which is never reported as "up to date".
	UpdatesRead bool
	// Updates is the number of updates waiting, SecurityUpdates how many of them the file calls standard security updates,
	// and ESMUpdates the extra security updates that only a paid Ubuntu Pro subscription can install.
	Updates         int
	SecurityUpdates int
	ESMUpdates      int
	// AutoRunRead is true only when the log held a start line, and AutoRunDays is the whole days since the last one. A missing,
	// empty or just-rotated log is unknown, never "stalled".
	AutoRunRead bool
	AutoRunDays int
	// Unreadable counts the files that exist but this account may not read.
	Unreadable int
}

// maxUpdatesBytes caps how much of the updates file is read: the real one is a few hundred bytes.
const maxUpdatesBytes = 64 << 10

var (
	// The wording is Ubuntu's own (the capture in testdata/updates_available_ubuntu). The security line is written from the
	// package's documentation, not from a capture: the captured file had no such line because none was waiting.
	updatesWaiting  = regexp.MustCompile(`(?m)^\s*(\d{1,6}) updates? can be applied immediately`)
	securityWaiting = regexp.MustCompile(`(?m)^\s*(\d{1,6}) of these updates? (?:is|are) (?:a )?standard security updates?`)
	esmWaiting      = regexp.MustCompile(`(?m)^\s*(\d{1,6}) additional security updates? can be applied with ESM`)
)

// Check reads the files.
func (p Patcher) Check() Patches {
	var out Patches
	if p.RebootFile != "" {
		p.reboot(&out)
	}
	if p.UpdatesFile != "" {
		p.updates(&out)
	}
	if p.AutoUpgradeLog != "" {
		p.autoRun(&out)
	}
	return out
}

func (p Patcher) reboot(out *Patches) {
	info, err := os.Stat(p.RebootFile)
	if err != nil {
		if !os.IsNotExist(err) {
			out.Unreadable++
		}
		return
	}
	out.RebootRequired = true
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	if age := now().Sub(info.ModTime()); age > 0 {
		out.RebootDays = int(age / (24 * time.Hour))
	}
}

func (p Patcher) updates(out *Patches) {
	f, err := os.Open(p.UpdatesFile)
	if err != nil {
		if !os.IsNotExist(err) {
			out.Unreadable++
		}
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxUpdatesBytes))
	if err != nil {
		out.Unreadable++
		return
	}
	text := string(raw)
	out.Updates, out.SecurityUpdates, out.ESMUpdates = firstNumber(updatesWaiting, text), firstNumber(securityWaiting, text), firstNumber(esmWaiting, text)
	// A file with none of the three lines is not a statement that nothing is waiting: the wording may have changed.
	out.UpdatesRead = updatesWaiting.MatchString(text) || securityWaiting.MatchString(text) || esmWaiting.MatchString(text)
}

// maxLogTail is how much of the end of the unattended-upgrades log is read: a month of runs is a few kilobytes.
const maxLogTail = 256 << 10

// The wording and the date layout are the package's own (the capture in testdata/unattended_upgrades_log_ubuntu).
var autoRunStart = regexp.MustCompile(`(?m)^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}),\d{3} INFO Starting unattended upgrades script$`)

func (p Patcher) autoRun(out *Patches) {
	f, err := os.Open(p.AutoUpgradeLog)
	if err != nil {
		if !os.IsNotExist(err) {
			out.Unreadable++
		}
		return
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() > maxLogTail {
		if _, err := f.Seek(info.Size()-maxLogTail, io.SeekStart); err != nil {
			out.Unreadable++
			return
		}
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxLogTail))
	if err != nil {
		out.Unreadable++
		return
	}
	matches := autoRunStart.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		return
	}
	zone, now := p.Zone, time.Now
	if zone == nil {
		zone = time.Local
	}
	if p.Now != nil {
		now = p.Now
	}
	last, err := time.ParseInLocation("2006-01-02 15:04:05", matches[len(matches)-1][1], zone)
	if err != nil {
		return
	}
	out.AutoRunRead = true
	if age := now().Sub(last); age > 0 {
		out.AutoRunDays = int(age / (24 * time.Hour))
	}
}

// firstNumber is the number in the first match of re, or 0 when there is none or it does not fit.
func firstNumber(re *regexp.Regexp, text string) int {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}
