package netaudit

import (
	"fmt"
	"time"

	"llm-proxy/internal/platform/hostwatch"
	"llm-proxy/models"
)

// HostDevice is the Finding.Device of everything the audit finds about the computer it runs on. It is not an address: a
// finding about this computer must not move when its address does, and it is not in any scan's device list.
const HostDevice = "this-computer"

// CheckNewListener is a port that other computers can reach and that was not open when the host watch first looked.
const CheckNewListener = "new_listener"

const hostFileCheckPrefix = "host_changed_"

// HostFileCheck is the check name of a change to one watched place. Each place has its own, so two changes never share a key
// (accepting one must not accept the other).
func HostFileCheck(id string) string { return hostFileCheckPrefix + id }

// hostFileInfo is the fixed wording for one watched place: nothing from the file reaches it.
type hostFileInfo struct{ Title, Fix, Severity string }

// hostFiles is keyed by hostwatch.FileSpec.ID; a test pins that the two lists are the same.
var hostFiles = map[string]hostFileInfo{
	"systemd_units": {"Startup services changed on this computer",
		"Look at the unit files under /etc/systemd/system (ls -lt lists the newest first). If you added or enabled one, accept the finding; if not, find out how it got there before the next reboot.", SeverityMedium},
	"cron": {"Scheduled jobs changed on this computer",
		"Look at /etc/crontab, the /etc/cron.* folders and the users' crontabs. If you added a job, accept the finding; if not, find out who did before the next time it runs.", SeverityMedium},
	"ssh_server_config": {"SSH server settings changed on this computer",
		"Compare /etc/ssh/sshd_config and the files in /etc/ssh/sshd_config.d with what you expect. If you changed them, accept the finding; if not, restore them and find out who did.", SeverityMedium},
	"sudoers": {"Administrator rights changed on this computer",
		"Check /etc/sudoers and /etc/sudoers.d (visudo -c checks the syntax). If you changed them, accept the finding; if not, remove what you do not recognise and find out how it was added.", SeverityHigh},
	"dns_settings": {"DNS settings changed on this computer",
		"Look at /etc/resolv.conf and, on Ubuntu, resolvectl status: they show which servers answer this computer's name lookups. Network changes (a new router, a VPN, a DHCP lease) change them legitimately; if you did not expect it, find out who changed them. Whoever answers name lookups decides where every name leads.", SeverityMedium},
	"authorized_keys": {"SSH login keys changed on this computer",
		"Read the authorized_keys file of each account and remove any key you do not recognise. If you added a key, accept the finding; if not, treat the account as possibly taken over and change its credentials.", SeverityHigh},
	"program": {"The audit program's own file changed on this computer",
		"This program's file was changed while it was running. If you deployed a new build, accept the finding; if not, reinstall it from a build you trust and find out who changed it.", SeverityMedium},
}

func init() {
	for id, info := range hostFiles {
		checkTitles[HostFileCheck(id)] = info.Title
		remediations[HostFileCheck(id)] = info.Fix
	}
	for id, guide := range hostFileGuides {
		guides[HostFileCheck(id)] = guide
	}
	guides[CheckNewListener] = newListenerGuide
	checkTitles[CheckNewListener] = "New open port on this computer"
	remediations[CheckNewListener] = "Find out what is listening (ss -ltnp shows the program for each port). If you started it, accept the finding; if not, stop it and find out how it got there."
}

// Checks for the weak settings of this computer. Unlike the checks above they compare nothing with an earlier look: they are
// there while the setting is weak, and an operator who means it accepts them.
const (
	CheckHostSSHPasswordLogin  = "host_ssh_password_login"
	CheckHostSSHRootLogin      = "host_ssh_root_login"
	CheckHostSSHEmptyPasswords = "host_ssh_empty_passwords"
	CheckHostRPFilterOff       = "host_rp_filter_off"
	CheckHostAutoUpdatesOff    = "host_auto_updates_off"
	CheckHostFirewallOff       = "host_firewall_off"
)

// hostSetting is the fixed wording of one weak setting.
type hostSetting struct{ Title, Evidence, Fix, Severity string }

var hostSettings = map[string]hostSetting{
	CheckHostSSHPasswordLogin: {"SSH accepts passwords on this computer",
		"This computer's SSH server lets anyone who can reach it try passwords. Key login only is much harder to guess",
		"Set up key login for each account that needs SSH, then set PasswordAuthentication no in /etc/ssh/sshd_config.d/ and reload the SSH service. Keep a session open while you test.", SeverityLow},
	CheckHostSSHRootLogin: {"SSH allows the root account to log in with a password",
		"This computer's SSH server lets root log in with a password, the account every attacker tries first",
		"Set PermitRootLogin prohibit-password (or no) in /etc/ssh/sshd_config.d/ and reload the SSH service; log in as a normal account and use sudo.", SeverityHigh},
	CheckHostSSHEmptyPasswords: {"SSH allows accounts with no password",
		"This computer's SSH server accepts accounts that have an empty password",
		"Set PermitEmptyPasswords no in /etc/ssh/sshd_config.d/ and reload the SSH service, then give every account a password or lock it.", SeverityHigh},
	CheckHostRPFilterOff: {"Spoofed-address filtering is off on this computer",
		"The kernel's reverse-path filter is switched off for every interface, so packets with a forged sender address are not dropped",
		"Set net.ipv4.conf.all.rp_filter and net.ipv4.conf.default.rp_filter to 2 (loose, the Ubuntu default) or 1 in a file under /etc/sysctl.d/ and run sysctl --system. Docker and VPNs work with 2.", SeverityMedium},
	CheckHostAutoUpdatesOff: {"This computer does not install security updates by itself",
		"Nothing on this computer is set to install updates automatically, so a fix for a known hole waits until someone installs it by hand",
		"Install the unattended-upgrades package and switch it on (sudo dpkg-reconfigure -plow unattended-upgrades). It installs security updates only and can leave restarts to you.", SeverityLow},
	CheckHostFirewallOff: {"The firewall on this computer is switched off",
		"This computer's own firewall (ufw) is installed but switched off, so only the router stands between its open ports and the network. Ports that Docker publishes bypass ufw even when it is on",
		"Allow what you use first (sudo ufw allow 22/tcp for SSH, plus the service ports you need), then switch it on with sudo ufw enable. Keep a session open while you test.", SeverityLow},
}

var hostSettingGuides = map[string]FixGuide{
	CheckHostSSHPasswordLogin: {
		Steps: []string{
			"Make sure every account that needs SSH has a public key in its authorized_keys file, and that you can log in with it from another computer.",
			"Add a file such as /etc/ssh/sshd_config.d/10-no-passwords.conf containing the line PasswordAuthentication no. Files in that folder are read before the main file, and the first value wins.",
			"Check the configuration and reload the SSH service, and open a second session to test before closing the first.",
			"If you want passwords on purpose (for instance on a home network with no outside access), accept the finding with a note.",
		},
		Commands: []GuideCommand{{platformLinux, "sudo sshd -T | grep -i passwordauthentication\necho 'PasswordAuthentication no' | sudo tee /etc/ssh/sshd_config.d/10-no-passwords.conf\nsudo sshd -t && sudo systemctl reload ssh"}},
		Risk:     "If no key works yet you are locked out over SSH and need the console to get back in.",
		Undo:     "Delete the drop-in file and reload the SSH service.",
	},
	CheckHostSSHRootLogin: {
		Steps: []string{
			"Make sure you can log in as a normal account and use sudo.",
			"Add a file such as /etc/ssh/sshd_config.d/10-root.conf containing PermitRootLogin prohibit-password, or no.",
			"Check the configuration, reload the SSH service, and test from a second session.",
		},
		Commands: []GuideCommand{{platformLinux, "sudo sshd -T | grep -i permitrootlogin\necho 'PermitRootLogin prohibit-password' | sudo tee /etc/ssh/sshd_config.d/10-root.conf\nsudo sshd -t && sudo systemctl reload ssh"}},
		Risk:     "Anything that logs in as root over SSH with a password stops working.",
		Undo:     "Delete the drop-in file and reload the SSH service.",
	},
	CheckHostSSHEmptyPasswords: {
		Steps: []string{
			"Find accounts with an empty password and give them a password or lock them.",
			"Add a file such as /etc/ssh/sshd_config.d/10-empty.conf containing PermitEmptyPasswords no, check the configuration, and reload the SSH service.",
		},
		Commands: []GuideCommand{{platformLinux, "sudo awk -F: '($2==\"\"){print $1}' /etc/shadow\necho 'PermitEmptyPasswords no' | sudo tee /etc/ssh/sshd_config.d/10-empty.conf\nsudo sshd -t && sudo systemctl reload ssh"}},
		Risk:     "An account that relied on an empty password can no longer log in over SSH.",
		Undo:     "Delete the drop-in file and reload the SSH service.",
	},
	CheckHostRPFilterOff: {
		Steps: []string{
			"Read the current values, then set them to 2 (loose) or 1 (strict) in a file under /etc/sysctl.d/.",
			"Apply them and check that routing, Docker and any VPN still work. Strict (1) can break setups where replies come back on another interface; loose (2) rarely does.",
		},
		Commands: []GuideCommand{{platformLinux, "sysctl net.ipv4.conf.all.rp_filter net.ipv4.conf.default.rp_filter\nprintf 'net.ipv4.conf.all.rp_filter=2\\nnet.ipv4.conf.default.rp_filter=2\\n' | sudo tee /etc/sysctl.d/60-rp-filter.conf\nsudo sysctl --system"}},
		Risk:     "Strict filtering drops traffic on setups with more than one route to the same network.",
		Undo:     "Delete the file you added and run sudo sysctl --system, or set the values back to 0.",
	},
	CheckHostAutoUpdatesOff: {
		Steps: []string{
			"Install the unattended-upgrades package and switch it on. By default it installs only security updates.",
			"It does not restart the computer by itself unless you ask it to; the 'waiting for a restart' finding still tells you when one is due.",
			"If you install updates by hand on a schedule and would rather keep it that way, accept the finding with a note.",
		},
		Commands: []GuideCommand{{platformLinux, "sudo apt install unattended-upgrades\nsudo dpkg-reconfigure -plow unattended-upgrades\ncat /etc/apt/apt.conf.d/20auto-upgrades"}},
		Risk:     "An update can change how a service behaves. Security-only updates rarely do, but a service may need a restart afterwards.",
		Undo:     "Set APT::Periodic::Unattended-Upgrade to \"0\" in /etc/apt/apt.conf.d/20auto-upgrades.",
	},
	CheckHostFirewallOff: {
		Steps: []string{
			"List what listens on this computer, and decide which of those ports should be reachable from the network.",
			"Allow SSH first if you use it, then each service port you need, and only then switch the firewall on. Switching it on without an SSH rule locks you out over SSH.",
			"Docker publishes its ports through its own rules, which ufw does not control. A published port stays reachable even with ufw on; bind it to 127.0.0.1 in the container settings if it should not be.",
			"If the router already blocks everything you do not forward and you accept that, accept the finding with a note.",
		},
		Commands: []GuideCommand{{platformLinux, "sudo ss -ltn\nsudo ufw allow 22/tcp\nsudo ufw enable\nsudo ufw status verbose"}},
		Risk:     "Anything you did not allow stops being reachable from other computers, and a missing SSH rule locks you out over SSH.",
		Undo:     "sudo ufw disable",
	},
}

func init() {
	for check, s := range hostSettings {
		checkTitles[check] = s.Title
		remediations[check] = s.Fix
		guides[check] = hostSettingGuides[check]
	}
}

// hardeningFindings turns the weak settings that were read into findings in fixed words.
func hardeningFindings(h hostwatch.Hardening) []Finding {
	var out []Finding
	add := func(on bool, check string) {
		if on {
			s := hostSettings[check]
			out = append(out, Finding{Device: HostDevice, Check: check, Severity: s.Severity, Evidence: s.Evidence})
		}
	}
	add(h.PasswordLogin, CheckHostSSHPasswordLogin)
	add(h.RootLogin, CheckHostSSHRootLogin)
	add(h.EmptyPasswords, CheckHostSSHEmptyPasswords)
	add(h.RPFilterOff, CheckHostRPFilterOff)
	add(h.AutoUpdatesOff, CheckHostAutoUpdatesOff)
	add(h.FirewallOff, CheckHostFirewallOff)
	return out
}

const (
	newListenerEvidence = "Something on this computer is waiting for connections on this port, and it was not when the audit first looked. The audit does not say which program it is"
	hostFileEvidence    = "A place that decides what runs on this computer after a reboot changed since the audit first recorded it (edited, added to or removed). The audit does not show what changed"
	// shortHash is how much of a digest goes in a finding's version.
	shortHash = 12
)

// HostSource is what the host watch looks at: *hostwatch.Source.
type HostSource interface {
	Listeners() ([]hostwatch.Listener, error)
	Files() []hostwatch.FileState
	Hardening() hostwatch.Hardening
	Patches() hostwatch.Patches
}

// HostStore keeps the first look the later ones are compared with: *persistence.HostWatchStore.
type HostStore interface {
	Read() (models.HostWatch, error)
	Write(models.HostWatch) error
}

// HostWatcher compares this computer's open ports and the files that survive a reboot with its own first look. It reads files
// the service's account can read, starts no program and sends nothing; findings are fixed words and a port number.
type HostWatcher struct {
	src   HostSource
	store HostStore
	now   func() time.Time
}

func NewHostWatcher(src HostSource, store HostStore) *HostWatcher {
	return &HostWatcher{src: src, store: store, now: time.Now}
}

// HostLook is one look: the findings, and how many watched locations the service's account may not read.
type HostLook struct {
	Findings   []Finding
	Unreadable int
}

// Look takes one look. The first builds the baseline and reports nothing. A look that could not be taken, or whose baseline
// could not be saved, is an error and reports nothing: a computer that could not be read is never reported as a quiet one.
func (w *HostWatcher) Look() (HostLook, error) {
	base, err := w.store.Read()
	if err != nil {
		return HostLook{}, fmt.Errorf("failed to read the host watch baseline: %w", err)
	}
	listeners, err := w.src.Listeners()
	if err != nil {
		return HostLook{}, fmt.Errorf("failed to list the open ports: %w", err)
	}
	var look HostLook
	changed := w.compareListeners(&base, listeners, &look)
	if w.compareFiles(&base, w.src.Files(), &look) {
		changed = true
	}
	hardening := w.src.Hardening()
	look.Unreadable += hardening.Unreadable
	look.Findings = append(look.Findings, hardeningFindings(hardening)...)
	patches := w.src.Patches()
	look.Unreadable += patches.Unreadable
	look.Findings = append(look.Findings, patchFindings(patches)...)
	if changed {
		if err := w.store.Write(base); err != nil {
			return HostLook{}, fmt.Errorf("failed to save the host watch baseline: %w", err)
		}
	}
	return look, nil
}

func (w *HostWatcher) compareListeners(base *models.HostWatch, now []hostwatch.Listener, look *HostLook) bool {
	if base.ListenersAt.IsZero() {
		base.ListenersAt = w.now()
		base.Listeners = nil
		for _, l := range now {
			base.Listeners = append(base.Listeners, models.HostListener{Proto: l.Proto, Port: l.Port})
		}
		return true
	}
	known := make(map[models.HostListener]bool, len(base.Listeners))
	for _, l := range base.Listeners {
		known[l] = true
	}
	for _, l := range now {
		if !known[models.HostListener{Proto: l.Proto, Port: l.Port}] {
			look.Findings = append(look.Findings, Finding{
				Device: HostDevice, Port: l.Port, Proto: l.Proto, Check: CheckNewListener, Severity: SeverityMedium, Evidence: newListenerEvidence,
			})
		}
	}
	return false
}

func (w *HostWatcher) compareFiles(base *models.HostWatch, now []hostwatch.FileState, look *HostLook) bool {
	changed := false
	for _, f := range now {
		look.Unreadable += f.Unreadable
		info, watched := hostFiles[f.ID]
		if !watched {
			continue
		}
		i := indexOfHostFile(base.Files, f.ID)
		switch {
		case i < 0: // a place with no earlier look
			base.Files = append(base.Files, models.HostFile{ID: f.ID, Hash: f.Hash, Tag: f.Tag})
			changed = true
		case base.Files[i].Tag != f.Tag: // the program was restarted, perhaps after an upgrade
			base.Files[i] = models.HostFile{ID: f.ID, Hash: f.Hash, Tag: f.Tag}
			changed = true
		case base.Files[i].Hash != f.Hash:
			look.Findings = append(look.Findings, Finding{
				Device: HostDevice, Check: HostFileCheck(f.ID), Severity: info.Severity, Version: f.Hash[:min(shortHash, len(f.Hash))],
				Evidence: hostFileEvidence,
			})
		}
	}
	return changed
}

func indexOfHostFile(files []models.HostFile, id string) int {
	for i, f := range files {
		if f.ID == id {
			return i
		}
	}
	return -1
}

// acceptHint ends every guide's steps: the audit does not know whether a change was the operator's own.
const acceptHint = "If the change was yours, accept the finding with a note. An accepted finding comes back if the same place changes again, or after the acceptance runs out."

// hostFileGuides are the longer forms of the fixes above. The commands only look; nothing here changes a file.
var hostFileGuides = map[string]FixGuide{
	"systemd_units": {
		Steps: []string{
			"List the unit files by age, newest first, and read any you do not recognise. A unit that starts a program from /tmp, a hidden folder or a home folder is the one to worry about.",
			"List the units that start at boot, and compare with what you installed.",
			"Stop and disable a unit you did not add, move its file away (do not delete it yet) and keep the program it started for inspection.",
			acceptHint,
		},
		Commands: []GuideCommand{
			{platformLinux, "ls -lt /etc/systemd/system | head -20\nsystemctl list-unit-files --state=enabled\nsystemctl cat NAME.service"},
		},
		Risk: "Disabling a unit stops what it provides; one you disable by mistake can stop a service the computer depends on at the next boot.",
		Undo: "Move the unit file back, run systemctl daemon-reload and enable it again with systemctl enable.",
	},
	"cron": {
		Steps: []string{
			"Read the system crontab and the folders of scheduled jobs, and each account's own crontab.",
			"Look for a job you did not add, especially one that downloads something, runs from /tmp, or runs as root.",
			"Remove a job you did not add from the file it is in, and keep a copy of the line.",
			acceptHint,
		},
		Commands: []GuideCommand{
			{platformLinux, "cat /etc/crontab\nls -l /etc/cron.d /etc/cron.hourly /etc/cron.daily /etc/cron.weekly /etc/cron.monthly\nsudo ls -l /var/spool/cron/crontabs"},
		},
		Risk: "Removing a job that something relied on stops that task, such as a backup or a certificate renewal.",
		Undo: "Put the saved line back in the same file.",
	},
	"ssh_server_config": {
		Steps: []string{
			"Read the SSH server's settings and the extra files it includes. Check PermitRootLogin, PasswordAuthentication, AuthorizedKeysFile and any Match block.",
			"Check the settings are valid before you apply them, then reload the SSH service. Keep your current session open while you test a new one.",
			acceptHint,
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo sshd -T | grep -Ei 'permitrootlogin|passwordauthentication|authorizedkeysfile'\nls -l /etc/ssh/sshd_config.d\nsudo sshd -t && sudo systemctl reload ssh"},
		},
		Risk: "A wrong setting can lock you out of the computer over SSH.",
		Undo: "Restore the previous file (keep a copy before editing), check it with sshd -t, and reload the SSH service.",
	},
	"dns_settings": {
		Steps: []string{
			"See which servers this computer asks for names, and compare with the ones you set (your router, or a service such as AdGuard DNS).",
			"A DHCP lease from your router, a VPN or a new network can change the list legitimately. A server you do not recognise, or a changed list on a computer that has not moved, is the one to worry about.",
			"If a setting was changed by something you did not do, put it back in the place that owns it (the network settings or /etc/systemd/resolved.conf.d) and find out how it was changed.",
			acceptHint,
		},
		Commands: []GuideCommand{
			{platformLinux, "resolvectl status | head -40\ncat /etc/resolv.conf\nls -l /etc/systemd/resolved.conf.d"},
		},
		Risk: "A wrong DNS server can make every name lookup fail, so the computer seems offline.",
		Undo: "Put the previous servers back in the same place and run sudo systemctl restart systemd-resolved.",
	},
	"sudoers": {
		Steps: []string{
			"Read the administrator rules and the files in the sudoers.d folder. Look for a user or group you do not know, and for NOPASSWD.",
			"Remove a rule you did not add using visudo, which checks the syntax before it saves.",
			"If you cannot explain the change, treat the computer as possibly compromised: change the passwords of the accounts with administrator rights and look at who is logged in.",
			acceptHint,
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cat /etc/sudoers\nsudo ls -l /etc/sudoers.d\nsudo visudo -c\nwho; last -n 10"},
		},
		Risk: "A broken sudoers file can remove everyone's administrator rights; always edit it with visudo.",
		Undo: "Restore the previous file with visudo, or from your copy.",
	},
	"authorized_keys": {
		Steps: []string{
			"Read the authorized_keys file of each account that can log in over SSH, including root. Every key should be one you recognise; the comment at the end of a line usually names the owner.",
			"Remove any key you do not recognise, and keep a copy of the line.",
			"If a key you do not know was there, treat the account as possibly taken over: change its password, check its recent logins and look for other changes (startup services, scheduled jobs).",
			acceptHint,
		},
		Commands: []GuideCommand{
			{platformLinux, "cat ~/.ssh/authorized_keys\nsudo cat /root/.ssh/authorized_keys\nlast -n 10"},
		},
		Risk: "Removing a key that someone or something legitimately uses (a backup job, a colleague) stops their login.",
		Undo: "Add the saved key line back to the file.",
	},
	"program": {
		Steps: []string{
			"This program's own file was changed while the program was running. Check whether you copied a new build over it before restarting; that is the usual reason.",
			"If you cannot explain the change, reinstall the program from a build you trust and compare the result.",
			acceptHint,
		},
		Commands: []GuideCommand{
			{platformLinux, "ls -l $(readlink -f /proc/$(pgrep -o llm-proxy)/exe)"},
		},
		Risk: "Reinstalling restarts the program, which interrupts running work.",
		Undo: "Install the previous build again.",
	},
}

var newListenerGuide = FixGuide{
	Steps: []string{
		"List what is listening, with the program behind each port, and find the port named in the finding.",
		"If you started it (a development server, a database, a new service), accept the finding with a note. A program you did not start should be stopped.",
		"If you cannot explain it, stop the program, find out how it started (check the startup services and scheduled jobs), and treat the computer as possibly compromised.",
		"Close ports that nothing needs with the computer's firewall, even for programs you trust.",
	},
	Commands: []GuideCommand{
		{platformLinux, "sudo ss -ltnup"},
	},
	Risk: "Stopping a program stops what it serves; a port closed by the firewall can no longer be reached from other computers.",
	Undo: "Start the program again, or remove the firewall rule.",
}
