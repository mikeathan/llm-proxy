package netaudit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"llm-proxy/internal/platform/hostwatch"
	"llm-proxy/models"
)

// HostDevice is the Finding.Device of everything the audit finds about the computer it runs on. It is not an address: a
// finding about this computer must not move when its address does, and it is not in any scan's device list.
const HostDevice = "this-computer"

// CheckNewListener is a port that other computers can reach and that was not open when the host watch first looked.
const CheckNewListener = "new_listener"

// CheckHostFilesModified is a program file that a package installed and that no longer matches the checksum the package shipped.
const CheckHostFilesModified = "host_file_modified"

const hostFileCheckPrefix = "host_changed_"

// HostFileCheck is the check name of a change to one watched place. Each place has its own, so two changes never share a key
// (accepting one must not accept the other).
func HostFileCheck(id string) string { return hostFileCheckPrefix + id }

// hostFileInfo is the fixed wording for one watched place: nothing from the file reaches it.
type hostFileInfo struct{ Title, Fix, Severity string }

// hostFiles is keyed by hostwatch.FileSpec.ID; a test pins that the two lists are the same.
var hostFiles = map[string]hostFileInfo{
	"systemd_units": {"Startup services changed on this computer",
		"Look at the unit files under /etc/systemd/system (ls -lt lists the newest first). The mount units snapd writes for each snap (snap-NAME-REVISION.mount) are not watched, so a snap refresh does not raise this. If you added or enabled one, accept the finding; if not, find out how it got there before the next reboot.", SeverityMedium},
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
	"launch_daemons": {"Programs that start at boot changed on this computer",
		"Look at the files in /Library/LaunchDaemons (ls -lt lists the newest first). Each one starts a program as root when the computer boots. If you installed software that adds one, accept the finding; if not, find out which program the new file starts and how it got there before the next restart.", SeverityHigh},
	"launch_agents": {"Programs that start at login changed on this computer",
		"Look at the files in /Library/LaunchAgents and in ~/Library/LaunchAgents (ls -lt lists the newest first). Each one starts a program when someone logs in. If you installed software that adds one, accept the finding; if not, find out which program the new file starts and how it got there.", SeverityMedium},
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
	guides[CheckHostFilesModified] = filesModifiedGuide
	checkTitles[CheckHostFilesModified] = "A program file on this computer no longer matches its package"
	remediations[CheckHostFilesModified] = "Find which package owns each file (dpkg -S), and reinstall it from the official archive (apt install --reinstall). If you did not change the file yourself, treat the computer as possibly compromised."
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
	CheckHostKeyFilesOpen      = "host_key_files_open"
	CheckHostDiskLow           = "host_disk_low"
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
	CheckHostKeyFilesOpen: {"A private key on this computer can be read by other users",
		"At least one SSH private key file on this computer (a host key or an account's key) can be read by other users of this computer, so any of them could copy it and act as its owner",
		"List the keys (ls -l) and set each private key to owner-only access with chmod 600. If the key may have been read by someone you do not trust, replace it.", SeverityHigh},
	// The severity and evidence of the disk finding depend on how full it is; see diskFinding. The values here are the low ones.
	CheckHostDiskLow: {"A disk on this computer is getting full",
		"A disk on this computer has little room left (under 10% free and under 50 GB). Models, logs and downloads fill a disk quietly, and a full disk stops services and the audit's own records",
		"Find what takes the room (du -xh --max-depth=1 /, ollama list for model files, docker system df) and remove what you no longer need, or move the models to a bigger disk.", SeverityLow},
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
	CheckHostKeyFilesOpen: {
		Steps: []string{
			"List the private keys with their owner and access: the host keys in /etc/ssh (ssh_host_*_key) and the id_* files in each account's .ssh folder. A private key should read -rw------- (600).",
			"Set each one that is more open to owner-only access with chmod 600. A key file that another account could read should be treated as copied: for an account key, make a new key pair and replace it in authorized_keys.",
			"If a distribution deliberately gives a key to a group (some give host keys to an ssh_keys group) and you accept that, accept the finding with a note.",
		},
		Commands: []GuideCommand{{platformLinux, "sudo ls -l /etc/ssh/ssh_host_*_key ~/.ssh/id_*\nchmod 600 ~/.ssh/id_*\nsudo chmod 600 /etc/ssh/ssh_host_*_key"}},
		Risk:     "A tool that runs as another account and needs the key will stop working until it is given its own key.",
		Undo:     "chmod the file back to its earlier mode (the listing above shows it).",
	},
	CheckHostDiskLow: {
		Steps: []string{
			"See which disk is short and what takes the room. Model files, container images and logs are the usual causes on a computer that runs models.",
			"Remove what you do not need: unused models, old container images and build cache, old logs. Look at what a command will delete before you agree.",
			"If the models need more room than the disk has, move them to a bigger disk and point the model folder there.",
		},
		Commands: []GuideCommand{{platformLinux, "df -h /\nsudo du -xh --max-depth=1 / 2>/dev/null | sort -h | tail\ndocker system df\nollama list"}},
		Risk:     "Removing a model or an image means downloading it again; deleting the wrong folder can lose data you wanted.",
		Undo:     "Deleted files are gone unless you have a backup; download models and images again if needed.",
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
	add(h.OpenKeyFiles > 0, CheckHostKeyFilesOpen)
	out = append(out, diskFinding(h.DiskLow)...)
	return out
}

// diskFinding is the disk finding for a level: low when a volume is getting full, medium when it is nearly full.
func diskFinding(level int) []Finding {
	s := hostSettings[CheckHostDiskLow]
	switch level {
	case hostwatch.DiskLow:
	case hostwatch.DiskCritical:
		s.Severity = SeverityMedium
		s.Evidence = "A disk on this computer is nearly full (under 3% free or under 2 GB). Services that write to it, this program included, can start to fail"
	default:
		return nil
	}
	return []Finding{{Device: HostDevice, Check: CheckHostDiskLow, Severity: s.Severity, Evidence: s.Evidence}}
}

const (
	newListenerEvidence = "Something on this computer is waiting for connections on this port, and it was not when the audit first looked. The audit does not say which program it is"
	// dockerListenerNote is added when Docker has a network here: the port may be a container's, and those bypass ufw.
	dockerListenerNote = ". Docker is installed on this computer: a port a container publishes is open to the network even when the computer's firewall is on"
	hostFileEvidence   = "A place that decides what runs on this computer after a reboot changed since the audit first recorded it (edited, added to or removed)"
	// shortHash is how much of a digest goes in a finding's version.
	shortHash = 12
)

// HostSource is what the host watch looks at: *hostwatch.Source.
type HostSource interface {
	Listeners() ([]hostwatch.Listener, error)
	Files() []hostwatch.FileState
	Hardening() hostwatch.Hardening
	Patches() hostwatch.Patches
	DockerBridge() bool
	Integrity(ctx context.Context) hostwatch.IntegrityResult
	Installed() (map[string]string, error)
	Privilege() hostwatch.Privilege
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
	flaws FlawJudge // optional: judges the installed packages against Ubuntu's security notices
}

// WithFlaws makes the watcher judge the installed packages against the downloaded notices. Without it nothing is judged.
func (w *HostWatcher) WithFlaws(j FlawJudge) *HostWatcher {
	w.flaws = j
	return w
}

func NewHostWatcher(src HostSource, store HostStore) *HostWatcher {
	return &HostWatcher{src: src, store: store, now: time.Now}
}

// HostLook is one look: the findings, and how many watched locations the service's account may not read.
type HostLook struct {
	Findings   []Finding
	Unreadable int
	// UnreadableWhere names the directories holding most of the program files that could not be read ("/usr/lib/modules (120)"),
	// and UnreadableWhy the operating system's reason, so the report can say where to look; both are empty when none.
	UnreadableWhere string
	UnreadableWhy   string
	// PortsUnchecked is true when this computer cannot list its own open ports (it has no /proc, as on macOS): the startup files were still compared,
	// no ports baseline was kept and no finding about ports was made, and the report says the ports were not checked. It is never a quiet computer.
	PortsUnchecked bool
	// PrivilegeUnchecked is true when the accounts, groups and kernel modules could not be looked at (no account file, as on
	// macOS), so the report says "not checked" rather than nothing.
	PrivilegeUnchecked bool
	// ModulesUnread and SetIDIncomplete say which part of a privilege look that did run was not done in full, and
	// PrivilegeLearned that this look recorded a starting point for at least one list, so the report says each.
	ModulesUnread    bool
	SetIDIncomplete  bool
	PrivilegeLearned bool
}

// Look takes one look. The first builds the baseline and reports nothing. A look that could not be taken, or whose baseline
// could not be saved, is an error and reports nothing: a computer that could not be read is never reported as a quiet one.
// ctx bounds the slow part (the pass over the program files), so a run or an application that stops does not leave it hashing.
func (w *HostWatcher) Look(ctx context.Context) (HostLook, error) {
	base, err := w.store.Read()
	if err != nil {
		return HostLook{}, fmt.Errorf("failed to read the host watch baseline: %w", err)
	}
	listeners, err := w.src.Listeners()
	var look HostLook
	changed := false
	switch {
	case errors.Is(err, hostwatch.ErrUnavailable):
		look.PortsUnchecked = true
	case err != nil:
		return HostLook{}, fmt.Errorf("failed to list the open ports: %w", err)
	default:
		changed = w.compareListeners(&base, listeners, &look)
	}
	if w.compareFiles(&base, w.src.Files(), &look) {
		changed = true
	}
	hardening := w.src.Hardening()
	look.Unreadable += hardening.Unreadable
	look.Findings = append(look.Findings, hardeningFindings(hardening)...)
	patches := w.src.Patches()
	look.Unreadable += patches.Unreadable
	look.Findings = append(look.Findings, patchFindings(patches)...)
	passCtx, endPass := untilStopped(ctx)
	integrity := w.src.Integrity(passCtx)
	endPass()
	if w.compareIntegrity(&base, integrity, &look) {
		changed = true
	}
	if w.compareFlaws(&base, &look) {
		changed = true
	}
	if comparePrivilege(&base.Privilege, w.src.Privilege(), &look) {
		changed = true
	}
	if changed {
		if err := w.store.Write(base); err != nil {
			return HostLook{}, fmt.Errorf("failed to save the host watch baseline: %w", err)
		}
	}
	return look, nil
}

// untilStopped is ctx without its deadline: it ends when ctx is cancelled (the operator stopped the run, or the application is
// shutting down) but not when the run's time cap passes. The pass over the program files runs last and has its own budget
// (PackagesBudget), so a run that spent most of its cap on the scan and the model must not lose the pass every night. A
// shutdown after the cap has already passed is not seen; the pass's own budget still bounds it.
func untilStopped(ctx context.Context) (context.Context, context.CancelFunc) {
	out, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	stopped := func() {
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			cancel(context.Cause(ctx))
		}
	}
	if ctx.Err() != nil { // already over: AfterFunc would act only later, on its own goroutine
		stopped()
	}
	stop := context.AfterFunc(ctx, stopped)
	return out, func() {
		stop()
		cancel(context.Canceled)
	}
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
	evidence := newListenerEvidence
	if w.src.DockerBridge() {
		evidence += dockerListenerNote
	}
	for _, l := range now {
		if !known[models.HostListener{Proto: l.Proto, Port: l.Port}] {
			look.Findings = append(look.Findings, Finding{
				Device: HostDevice, Port: l.Port, Proto: l.Proto, Check: CheckNewListener, Severity: SeverityMedium, Evidence: evidence,
			})
		}
	}
	return false
}

// compareIntegrity turns a pass over the packaged program files into a finding. Only a complete pass can say a finding is gone:
// a pass cut short keeps the earlier finding, and a computer where nothing could be compared (no dpkg lists) says nothing at
// all. It returns true when the stored outcome changed.
func (w *HostWatcher) compareIntegrity(base *models.HostWatch, res hostwatch.IntegrityResult, look *HostLook) bool {
	look.Unreadable += res.Unreadable
	look.UnreadableWhere, look.UnreadableWhy = strings.Join(res.UnreadableIn, ", "), res.UnreadableWhy
	modified, paths := res.Modified, res.Paths
	stored := false
	switch {
	case res.Complete:
		if base.Integrity.Modified != modified || !slices.Equal(base.Integrity.Paths, paths) {
			base.Integrity = models.HostIntegrity{Modified: modified, Paths: slices.Clone(paths), At: w.now()}
			stored = true
		}
	case modified == 0:
		modified, paths = base.Integrity.Modified, base.Integrity.Paths
	}
	if modified > 0 {
		look.Findings = append(look.Findings, Finding{Device: HostDevice, Check: CheckHostFilesModified, Severity: SeverityHigh, Evidence: integrityEvidence(modified), Details: integrityDetails(modified, paths)})
	}
	return stored
}

// compareFlaws turns the installed packages into a finding when they are behind a fix. Only a judgement that was made can say the
// finding is gone: a package list that could not be read keeps the earlier finding (and counts as an unreadable place), and a
// computer with nothing to judge against (no data downloaded, or data for another release) says nothing and keeps nothing.
// It returns true when the stored outcome changed.
func (w *HostWatcher) compareFlaws(base *models.HostWatch, look *HostLook) bool {
	if w.flaws == nil {
		return false
	}
	installed, err := w.src.Installed()
	if err != nil {
		look.Unreadable++
		if base.Flaws.Packages > 0 {
			look.Findings = append(look.Findings, flawFinding(base.Flaws))
		}
		return false
	}
	res, ok := w.flaws.Judge(installed)
	now := models.HostFlaws{}
	if ok {
		now = flawsOf(res)
	}
	if now.Packages > 0 {
		look.Findings = append(look.Findings, flawFinding(now))
	}
	if reflect.DeepEqual(base.Flaws, now) {
		return false
	}
	base.Flaws = now
	return true
}

// integrityEvidence is the finding's words: how many files and what the audit cannot tell. The paths are the finding's details.
func integrityEvidence(modified int) string {
	verb := "no longer match"
	if modified == 1 {
		verb = "no longer matches"
	}
	return plural(modified, "program file", "program files") + " that packages installed on this computer " + verb + " the checksum the package shipped. A change by hand, by an attack or by a failing disk looks the same; the audit cannot tell which"
}

// integrityDetails lists up to ten plain paths the reader already vetted, one row each, and counts the rest.
func integrityDetails(modified int, paths []string) []models.FindingDetail {
	out := make([]models.FindingDetail, 0, len(paths)+1)
	for _, p := range paths {
		out = append(out, models.FindingDetail{Label: "Changed file", Value: p})
	}
	if more := modified - len(paths); more > 0 && len(paths) > 0 {
		out = append(out, models.FindingDetail{Label: "Not listed", Value: fmt.Sprintf("%d more", more)})
	}
	return out
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
			base.Files = append(base.Files, w.newHostFile(f))
			changed = true
		case base.Files[i].Tag != f.Tag: // the program was restarted, perhaps after an upgrade
			base.Files[i] = w.newHostFile(f)
			changed = true
		case base.Files[i].Hash != f.Hash:
			details := w.fileDetails(&base.Files[i], f, &changed)
			look.Findings = append(look.Findings, Finding{
				Device: HostDevice, Check: HostFileCheck(f.ID), Severity: info.Severity, Version: f.Hash[:min(shortHash, len(f.Hash))],
				Evidence: hostFileEvidence, Details: details,
			})
		}
	}
	return changed
}

// newHostFile is the first look at a place: its digest and the list of its files.
func (w *HostWatcher) newHostFile(f hostwatch.FileState) models.HostFile {
	out := models.HostFile{ID: f.ID, Hash: f.Hash, Tag: f.Tag, Entries: hostEntries(f.Entries)}
	if len(out.Entries) > 0 {
		out.ListedAt = w.now()
	}
	return out
}

func hostEntries(in []hostwatch.FileEntry) []models.HostFileEntry {
	out := make([]models.HostFileEntry, len(in))
	for i, e := range in {
		out[i] = models.HostFileEntry{Path: e.Path, Sum: e.Sum}
	}
	return out
}

const (
	// maxNamedFiles is how many file names one detail row lists; the rest are counted.
	maxNamedFiles = 10
	// listLimitValue says why a place with no list cannot name a change.
	listLimitValue = "The audit could not list the files of this place (too many), so it knows the place changed but not which file"
	fileDateLayout = "2 Jan 2006"
)

// fileDetails names what changed in a place against the list kept at the first look. A place first recorded before lists were kept
// gets its list now (reported through changed) and says what it cannot name; a place too big to list says that.
func (w *HostWatcher) fileDetails(rec *models.HostFile, f hostwatch.FileState, changed *bool) []models.FindingDetail {
	const label = "Which file"
	if len(f.Entries) == 0 {
		return []models.FindingDetail{{Label: label, Value: listLimitValue}}
	}
	if len(rec.Entries) == 0 {
		rec.Entries, rec.ListedAt = hostEntries(f.Entries), w.now()
		*changed = true
		return []models.FindingDetail{{Label: label, Value: "This place was first recorded as a digest only, so what changed before " + w.now().Format(fileDateLayout) + " cannot be named. Files are named from this look on"}}
	}
	added, edited, removed := diffEntries(rec.Entries, f.Entries)
	var out []models.FindingDetail
	for _, row := range []struct {
		label string
		paths []string
	}{{"Added", added}, {"Edited", edited}, {"Removed", removed}} {
		if len(row.paths) > 0 {
			out = append(out, models.FindingDetail{Label: row.label, Value: namedFiles(row.paths)})
		}
	}
	if len(out) == 0 { // the digest moved but no listed file did: a folder or link that is not listed, or names that were left out as not plain
		out = []models.FindingDetail{{Label: label, Value: "The change is in something the audit does not list by name (a folder, or a file whose name is not a plain path)"}}
	}
	if !rec.ListedAt.IsZero() {
		out = append(out, models.FindingDetail{Label: "Compared with", Value: "the list kept on " + rec.ListedAt.Format(fileDateLayout)})
	}
	return out
}

// diffEntries returns the paths in now but not in then (added), in both with a different sum (edited), and in then but not now (removed).
func diffEntries(then []models.HostFileEntry, now []hostwatch.FileEntry) (added, edited, removed []string) {
	before := make(map[string]string, len(then))
	for _, e := range then {
		before[e.Path] = e.Sum
	}
	seen := make(map[string]bool, len(now))
	for _, e := range now {
		seen[e.Path] = true
		sum, ok := before[e.Path]
		switch {
		case !ok:
			added = append(added, e.Path)
		case sum != e.Sum:
			edited = append(edited, e.Path)
		}
	}
	for _, e := range then {
		if !seen[e.Path] {
			removed = append(removed, e.Path)
		}
	}
	return added, edited, removed
}

// namedFiles joins up to maxNamedFiles paths and counts the rest.
func namedFiles(paths []string) string {
	if len(paths) <= maxNamedFiles {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(paths[:maxNamedFiles], ", "), len(paths)-maxNamedFiles)
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
	"launch_daemons": {
		Steps: []string{
			"List the files by age, newest first, and read any you do not recognise. Each file names the program it starts as root when the computer boots (the ProgramArguments entry).",
			"A file that starts a program from /tmp, a hidden folder or a user's home folder is the one to worry about. Software you installed (a VPN, a driver, an updater) adds its own file here.",
			"Unload a file you did not add and move it away (do not delete it yet); keep the program it started for inspection.",
			acceptHint,
		},
		Commands: []GuideCommand{
			{platformMac, "ls -lt /Library/LaunchDaemons | head -20\nplutil -p /Library/LaunchDaemons/NAME.plist\nsudo launchctl list | head -40"},
		},
		Risk: "Unloading a daemon stops what it provides; one you remove by mistake can stop a driver or a service the computer depends on at the next restart.",
		Undo: "Move the file back into /Library/LaunchDaemons and load it again with sudo launchctl bootstrap system /Library/LaunchDaemons/NAME.plist.",
	},
	"launch_agents": {
		Steps: []string{
			"List the files by age, newest first, in /Library/LaunchAgents and in your own Library/LaunchAgents, and read any you do not recognise. Each one starts a program when someone logs in.",
			"A file that starts a program from /tmp, a hidden folder or a download is the one to worry about.",
			"Unload a file you did not add and move it away (do not delete it yet); keep the program it started for inspection.",
			acceptHint,
		},
		Commands: []GuideCommand{
			{platformMac, "ls -lt /Library/LaunchAgents ~/Library/LaunchAgents | head -30\nplutil -p ~/Library/LaunchAgents/NAME.plist\nlaunchctl list | head -40"},
		},
		Risk: "Unloading an agent stops what it does at login, such as a menu bar program or a sync tool.",
		Undo: "Move the file back into its folder and load it again with launchctl bootstrap gui/$(id -u) PATH-TO-THE-FILE.",
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
		"If the port belongs to a container, the firewall does not close it: Docker publishes ports through its own rules. Publish it on 127.0.0.1 only (-p 127.0.0.1:8080:80) or stop publishing it.",
	},
	Commands: []GuideCommand{
		{platformLinux, "sudo ss -ltnup\ndocker ps --format '{{.Names}}  {{.Ports}}'"},
	},
	Risk: "Stopping a program stops what it serves; a port closed by the firewall can no longer be reached from other computers.",
	Undo: "Start the program again, or remove the firewall rule.",
}

var filesModifiedGuide = FixGuide{
	Steps: []string{
		"List the files that differ (dpkg --verify). Each line names a file; a 5 in the third column means its contents changed.",
		"If you patched a file yourself or an update was cut off, reinstall its package (apt install --reinstall) and run the check again, or accept the finding with a note.",
		"If you cannot explain the change, do not trust this computer: disconnect it, copy off what you need, and reinstall from media you trust. A changed system program can hide itself from the tools you would use to look.",
		"Change the passwords and keys that were used on this computer.",
	},
	Commands: []GuideCommand{
		{platformLinux, "dpkg --verify 2>/dev/null | grep -v ' c /'\ndpkg -S /usr/sbin/sshd\nsudo apt install --reinstall openssh-server"},
	},
	Risk: "Reinstalling a package restarts nothing by itself, but a running service keeps using the old file until it is restarted.",
	Undo: "None needed: a reinstall puts back the file the package ships.",
}
