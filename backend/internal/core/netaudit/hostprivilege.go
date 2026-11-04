package netaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"llm-proxy/internal/platform/hostwatch"
	"llm-proxy/models"
)

// Checks for the marks of privilege and persistence on this computer (Linux only). The first two are there while the mark is:
// a normal computer has neither, so they are reported from the first look. The others compare with the first look, like a new
// open port, and stay until the operator accepts them.
const (
	CheckHostPreload         = "host_preload"
	CheckHostExtraRoot       = "host_extra_root"
	CheckHostNewLoginAccount = "host_new_login_account"
	CheckHostNewAdmin        = "host_new_admin"
	CheckHostNewModule       = "host_new_kernel_module"
	CheckHostNewSetID        = "host_new_setid"
)

// privilegeCheckNames lists them, for the tests and the registries.
var privilegeCheckNames = []string{
	CheckHostPreload, CheckHostExtraRoot, CheckHostNewLoginAccount, CheckHostNewAdmin, CheckHostNewModule, CheckHostNewSetID,
}

// maxNamesShown bounds how many names one finding's evidence lists.
const maxNamesShown = 10

var privilegeInfo = map[string]hostSetting{
	CheckHostPreload: {"Every program on this computer is told to load an extra library",
		"/etc/ld.so.preload names a library that the system loads into every program that starts. Normal systems leave the file absent or empty; hiding tools (rootkits) use it to hide files and processes",
		"Read /etc/ld.so.preload. If you did not add it for a tool you know, treat the computer as compromised: do not trust what it shows about itself, and rebuild it from a trusted installer.", SeverityHigh},
	CheckHostExtraRoot: {"An account other than root has root's rights on this computer",
		"An account other than root has user id 0, which makes it root under another name",
		"Look at /etc/passwd for accounts with user id 0 besides root. If you did not create one on purpose, lock it (sudo usermod -L name), find out how it was added, and treat the computer as possibly taken over.", SeverityHigh},
	CheckHostNewLoginAccount: {"A new account that can log in appeared on this computer",
		"An account with a login shell exists that did not when the audit first looked",
		"Check that you or a package you installed created the account (getent passwd name, and the date in /var/log/auth.log). If not, lock it and find out how it was added.", SeverityMedium},
	CheckHostNewAdmin: {"An account was given administrator rights on this computer",
		"An account is in a group that is root or close to it (sudo, adm, docker or lxd), and was not when the audit first looked. Members of docker and lxd can start a container that reads and writes every file",
		"Check that you added the account to the group (getent group sudo docker lxd). If not, remove it (sudo gpasswd -d name group) and find out who added it.", SeverityHigh},
	CheckHostNewModule: {"A kernel module was loaded on this computer that was not before",
		"A kernel module is loaded that was not when the audit first looked. Plugging in a device or starting a service loads modules too, so this is often harmless; a module is also how the deepest hiding tools get in",
		"Look at the module (modinfo name shows its file and package). If it belongs to hardware or a service you use, accept the finding; if it belongs to no package, treat the computer as possibly compromised.", SeverityLow},
	CheckHostNewSetID: {"A new program that runs with raised rights appeared on this computer",
		"A file with the set-user-ID or set-group-ID bit, which runs with its owner's rights whoever starts it, is in /usr/local, /opt or /srv and was not when the audit first looked. Nothing packaged puts one there; it is a classic way to keep root",
		"Look at the file (ls -l and sha256sum). If you installed it on purpose, accept the finding; if not, remove the bit (sudo chmod u-s,g-s path) and find out how it got there.", SeverityHigh},
}

var privilegeGuides = map[string]FixGuide{
	CheckHostPreload: {
		Steps: []string{
			"Read the file and the library it names. A library you installed for a known tool (some memory debuggers do this) is the only good reason.",
			"If you do not recognise it, do not delete it and carry on: a hiding tool hides itself, so what the computer shows can no longer be trusted. Copy what you need from it, and reinstall the system from a trusted installer.",
		},
		Commands: []GuideCommand{{platformLinux, "cat /etc/ld.so.preload\nls -l $(cat /etc/ld.so.preload)\ndpkg -S $(cat /etc/ld.so.preload)"}},
		Risk:     "Removing a library a tool needs stops that tool; removing a hiding tool's line does not remove the tool.",
		Undo:     "Put the line back in /etc/ld.so.preload.",
	},
	CheckHostExtraRoot: {
		Steps: []string{
			"List the accounts with user id 0.",
			"If one is not yours, lock it at once, then look in /var/log/auth.log for when and from where it was used.",
			"Treat the computer as possibly taken over: change the passwords and keys of the real accounts from a computer you trust.",
		},
		Commands: []GuideCommand{{platformLinux, "awk -F: '$3 == 0 {print $1}' /etc/passwd\nsudo usermod -L <name>\nsudo grep <name> /var/log/auth.log"}},
		Risk:     "Locking an account a service really uses stops that service.",
		Undo:     "sudo usermod -U <name>",
	},
	CheckHostNewLoginAccount: {
		Steps: []string{
			"See the account and when it was made: packages that create service accounts normally give them no login shell.",
			"If you made it, accept the finding. If not, lock it and look for how it was added.",
		},
		Commands: []GuideCommand{{platformLinux, "getent passwd | awk -F: '$7 !~ /(nologin|false)$/'\nsudo grep -E 'useradd|adduser' /var/log/auth.log\nsudo usermod -L <name>"}},
		Risk:     "Locking an account stops its logins.",
		Undo:     "sudo usermod -U <name>",
	},
	CheckHostNewAdmin: {
		Steps: []string{
			"See who is in sudo, adm, docker and lxd.",
			"If you added the account, accept the finding. If not, remove it from the group and look for how it was added.",
		},
		Commands: []GuideCommand{{platformLinux, "getent group sudo adm docker lxd\nsudo gpasswd -d <name> <group>"}},
		Risk:     "Removing an account from docker or sudo stops what it ran with those rights.",
		Undo:     "sudo usermod -aG <group> <name>",
	},
	CheckHostNewModule: {
		Steps: []string{
			"Look at each new module: modinfo names its file, and dpkg -S says which package owns that file.",
			"A module of hardware you plugged in or a service you started (a VPN, Docker, a USB disk) is normal; accept the finding.",
			"A module no package owns, or whose file is missing, is a warning sign: treat the computer as possibly compromised.",
		},
		Commands: []GuideCommand{{platformLinux, "lsmod | head -50\nmodinfo <name> | head\ndpkg -S $(modinfo -n <name>)"}},
		Risk:     "None from looking. Unloading a module the system uses can break that hardware.",
		Undo:     "Nothing to undo.",
	},
	CheckHostNewSetID: {
		Steps: []string{
			"Look at each file: who owns it, when it changed, and what it is.",
			"If you installed it on purpose, accept the finding. If not, remove the bit and find out how it got there.",
		},
		Commands: []GuideCommand{{platformLinux, "sudo find /usr/local /opt /srv -xdev -perm /6000 -type f -exec ls -l {} +\nsudo chmod u-s,g-s <path>"}},
		Risk:     "A tool that needs the raised rights stops working.",
		Undo:     "sudo chmod u+s <path> (or g+s for a set-group-ID file).",
	},
}

func init() {
	for check, s := range privilegeInfo {
		checkTitles[check] = s.Title
		remediations[check] = s.Fix
		guides[check] = privilegeGuides[check]
	}
}

// comparePrivilege turns one look at the privilege marks into findings and learns each list the first time it was read in full.
// A list that could not be read neither learns nor reports. It returns true when the baseline changed.
func comparePrivilege(base *models.HostPrivilege, p hostwatch.Privilege, look *HostLook) bool {
	look.Unreadable += p.Unreadable
	if !p.Checked {
		look.PrivilegeUnchecked = true
		return false
	}
	if p.Preload {
		look.Findings = append(look.Findings, privilegeFinding(CheckHostPreload, "", shortDigest(p.PreloadDigest)))
	}
	if len(p.ExtraRoot) > 0 {
		look.Findings = append(look.Findings, privilegeFinding(CheckHostExtraRoot, namesPhrase("Accounts", p.ExtraRoot), digestOf(p.ExtraRoot)))
	}
	changed := false
	if !base.AccountsSeen {
		base.AccountsSeen, base.LoginAccounts, base.Admins = true, slices.Clone(p.LoginAccounts), slices.Clone(p.Admins)
		changed = true
	} else {
		look.Findings = append(look.Findings, added(CheckHostNewLoginAccount, "New accounts", base.LoginAccounts, p.LoginAccounts)...)
		look.Findings = append(look.Findings, added(CheckHostNewAdmin, "New members (group:account)", base.Admins, p.Admins)...)
	}
	if p.ModulesRead {
		if !base.ModulesSeen {
			base.ModulesSeen, base.Modules = true, slices.Clone(p.Modules)
			changed = true
		} else {
			look.Findings = append(look.Findings, added(CheckHostNewModule, "New modules", base.Modules, p.Modules)...)
		}
	}
	switch {
	case base.SetIDSeen:
		look.Findings = append(look.Findings, added(CheckHostNewSetID, "New files", base.SetID, p.SetID)...)
	case p.SetIDComplete:
		base.SetIDSeen, base.SetID = true, slices.Clone(p.SetID)
		changed = true
	}
	return changed
}

// added is one finding listing what now has that the first look did not, or none.
func added(check, label string, first, now []string) []Finding {
	var fresh []string
	for _, n := range now {
		if !slices.Contains(first, n) {
			fresh = append(fresh, n)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	return []Finding{privilegeFinding(check, namesPhrase(label, fresh), digestOf(fresh))}
}

// privilegeFinding is a finding about this computer in fixed words, with the vetted names after them. Version is a digest of the
// names, so an accepted finding holds until the set changes.
func privilegeFinding(check, names, version string) Finding {
	evidence := privilegeInfo[check].Evidence
	if names != "" {
		evidence += ". " + names
	}
	return Finding{Device: HostDevice, Check: check, Severity: privilegeInfo[check].Severity, Evidence: evidence, Version: version}
}

func namesPhrase(label string, names []string) string {
	shown := names[:min(len(names), maxNamesShown)]
	out := label + ": " + strings.Join(shown, ", ")
	if len(names) > len(shown) {
		out += ", and others"
	}
	return out
}

func digestOf(names []string) string {
	sum := sha256.Sum256([]byte(strings.Join(names, "\x00")))
	return hex.EncodeToString(sum[:])[:shortHash]
}

func shortDigest(d string) string { return d[:min(len(d), shortHash)] }
