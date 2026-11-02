package netaudit

import (
	"fmt"

	"llm-proxy/internal/platform/hostwatch"
)

// Checks for how up to date this computer is. Like the weak settings they compare nothing with an earlier look: they are there
// while an update or a restart is waiting, and an operator who means it accepts them.
const (
	CheckHostRebootPending  = "host_reboot_pending"
	CheckHostUpdatesPending = "host_updates_pending"
)

// rebootStaleDays is how long a waiting restart can wait before it is a medium finding: an update that needs a restart has been
// installed, but the old, flawed version keeps running until then.
const rebootStaleDays = 14

func init() {
	checkTitles[CheckHostRebootPending] = "This computer is waiting for a restart"
	remediations[CheckHostRebootPending] = "Restart this computer at a quiet time so the updates that were installed start to work. Everything running on it, including this program, stops for a minute or two."
	guides[CheckHostRebootPending] = rebootPendingGuide
	checkTitles[CheckHostUpdatesPending] = "Updates are waiting on this computer"
	remediations[CheckHostUpdatesPending] = "Install the waiting updates (sudo apt update, then sudo apt upgrade). Security updates close holes that are already known to attackers."
	guides[CheckHostUpdatesPending] = updatesPendingGuide
}

var rebootPendingGuide = FixGuide{
	Steps: []string{
		"A restart is needed after updates to the kernel, the C library or similar parts. Until then the old, flawed version keeps running.",
		"Pick a quiet time. A restart stops everything on this computer for a minute or two, this program and the model servers included.",
		"After the restart, check that your services came back. The restart marker goes away by itself, and so does this finding at the next audit.",
	},
	Commands: []GuideCommand{{platformLinux, "cat /var/run/reboot-required.pkgs\nsudo reboot"}},
	Risk:     "Anything running on this computer stops, and a service that does not start by itself at boot stays down until you start it.",
	Undo:     "A restart cannot be undone; start any service that did not come back.",
}

var updatesPendingGuide = FixGuide{
	Steps: []string{
		"List what is waiting and read the names. Updates to the kernel or to a service you run are the ones that may need a restart afterwards.",
		"Install them, then look at whether a restart is now waiting (the other finding).",
		"If the finding mentions extra security updates for Ubuntu Pro: these are fixes for packages outside Ubuntu's main collection, and Ubuntu Pro is free for personal use on a few machines. Enabling it is your decision; the audit never does it.",
		"Where you cannot install now, accept the finding with a note and a date to look again.",
	},
	Commands: []GuideCommand{{platformLinux, "apt list --upgradable\nsudo apt update && sudo apt upgrade\npro status"}},
	Risk:     "An update can change how a service behaves or need a restart; a held-back package may need apt full-upgrade, which can remove packages, so read what it plans before agreeing.",
	Undo:     "Most upgrades cannot be undone with one command; take a snapshot or backup first if the computer matters.",
}

// patchFindings turns what was read about updates into findings in fixed words and numbers read from the files. A computer
// whose update file could not be read says nothing about updates: unknown is never reported as up to date, nor as behind.
func patchFindings(p hostwatch.Patches) []Finding {
	var out []Finding
	if p.RebootRequired {
		severity := SeverityLow
		if p.RebootDays >= rebootStaleDays {
			severity = SeverityMedium
		}
		out = append(out, Finding{Device: HostDevice, Check: CheckHostRebootPending, Severity: severity, Evidence: rebootEvidence(p.RebootDays)})
	}
	if p.UpdatesRead && (p.Updates > 0 || p.SecurityUpdates > 0 || p.ESMUpdates > 0) {
		severity := SeverityLow
		if p.SecurityUpdates > 0 {
			severity = SeverityMedium
		}
		out = append(out, Finding{Device: HostDevice, Check: CheckHostUpdatesPending, Severity: severity, Evidence: updatesEvidence(p)})
	}
	return out
}

func rebootEvidence(days int) string {
	when := "since today"
	switch {
	case days == 1:
		when = "for 1 day"
	case days > 1:
		when = fmt.Sprintf("for %d days", days)
	}
	return "An update that needs a restart was installed on this computer, and the restart has been waiting " + when +
		". Until then the old version of that software keeps running"
}

func updatesEvidence(p hostwatch.Patches) string {
	text := fmt.Sprintf("%d updates are waiting to be installed on this computer", p.Updates)
	if p.Updates == 1 {
		text = "1 update is waiting to be installed on this computer"
	}
	if p.SecurityUpdates > 0 {
		text += fmt.Sprintf(", %d of them security updates", p.SecurityUpdates)
	}
	if p.ESMUpdates > 0 {
		text += fmt.Sprintf(". %d more security updates are offered only with Ubuntu Pro", p.ESMUpdates)
	}
	return text
}
