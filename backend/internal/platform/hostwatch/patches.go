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
