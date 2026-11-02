package hostwatch

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Hardener reads the few settings of this computer that decide how easily it is taken over. Like the rest of the package it
// reads files the service's account can read, starts no program, and keeps no text from them: each setting becomes a bool.
type Hardener struct {
	// SSHDir is "/etc/ssh" and SysDir "/proc/sys" on a real computer.
	SSHDir string
	SysDir string
	// AptDir is "/etc/apt/apt.conf.d" and UfwConf "/etc/ufw/ufw.conf". An empty path leaves that setting out.
	AptDir  string
	UfwConf string
	// KeyGlobs match private key files to look at: the host keys under SSHDir and each account's id_* files. Only the access
	// mode of a file is read, never its content. An empty list leaves the check out.
	KeyGlobs []string
	// DiskPaths are folders whose volume's free space is checked ("/" and the service account's home). An empty list leaves the
	// check out. Space reads one volume's free and total bytes; the real statfs when nil.
	DiskPaths []string
	Space     func(path string) (free, total uint64, err error)
}

// Hardening is what one look found. A bool is true only when the setting was read and is the weak one; a setting that could
// not be read is false, so an unreadable file is never reported as a weak one.
type Hardening struct {
	// SSHFound is true when an SSH server's main configuration file exists.
	SSHFound bool
	// PasswordLogin is sshd's effective PasswordAuthentication (default yes), EmptyPasswords PermitEmptyPasswords (default no)
	// and RootLogin whether PermitRootLogin is plain yes (the default, prohibit-password, needs a key).
	PasswordLogin  bool
	RootLogin      bool
	EmptyPasswords bool
	// RPFilterOff is true when reverse-path filtering is 0 for both "all" and "default". The value 2 (loose) is Ubuntu's own
	// default.
	RPFilterOff bool
	// AutoUpdatesOff is true on a computer that uses apt when nothing there installs updates by itself, and FirewallOff when
	// ufw is installed and switched off. Another firewall (nftables, firewalld) is not looked for, so its absence is not a
	// finding on its own.
	AutoUpdatesOff bool
	FirewallOff    bool
	// OpenKeyFiles counts private key files that group or other users can access. The file names are not kept.
	OpenKeyFiles int
	// DiskLow is 0, DiskLow (a volume is getting full) or DiskCritical (nearly full), for the fullest volume looked at.
	DiskLow int
	// Unreadable counts the files that exist but this account may not read.
	Unreadable int
}

// Limits on what one look reads.
const (
	maxSSHConfigBytes = 256 << 10
	maxIncludeDepth   = 4
	maxIncludeFiles   = 64
)

// Check reads the settings.
func (h Hardener) Check() Hardening {
	var out Hardening
	if h.SSHDir != "" {
		h.sshd(&out)
	}
	if h.SysDir != "" {
		out.RPFilterOff = h.rpFilterOff()
	}
	h.protection(&out)
	h.keyFiles(&out)
	h.diskLow(&out)
	return out
}

// sshdValues are the first value seen for each keyword sshd has been asked about.
type sshdValues map[string]string

func (h Hardener) sshd(out *Hardening) {
	main := filepath.Join(h.SSHDir, "sshd_config")
	if _, err := os.Stat(main); errors.Is(err, fs.ErrNotExist) {
		return
	}
	out.SSHFound = true
	r := &sshReader{h: h, vals: sshdValues{}}
	ok := r.file(main, 0)
	out.Unreadable += r.unreadable
	if !ok {
		return // the main file could not be read: nothing is known, so nothing is reported
	}
	out.PasswordLogin = r.vals.value("passwordauthentication", "yes") == "yes"
	out.EmptyPasswords = r.vals.value("permitemptypasswords", "no") == "yes"
	out.RootLogin = r.vals.value("permitrootlogin", "prohibit-password") == "yes"
}

func (v sshdValues) value(keyword, fallback string) string {
	if got, ok := v[keyword]; ok {
		return got
	}
	return fallback
}

// sshReader reads sshd_config and the files it includes.
type sshReader struct {
	h          Hardener
	vals       sshdValues
	files      int
	unreadable int
}

// file adds a file's settings. sshd keeps the first value of a keyword, an Include is read where it stands, and everything
// after a Match line applies to some connections only, so it is not the computer's default. It reports whether the file
// could be read.
func (r *sshReader) file(path string, depth int) bool {
	if depth > maxIncludeDepth || r.files >= maxIncludeFiles {
		return true
	}
	r.files++
	f, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			r.unreadable++
		}
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	read := 0
	for sc.Scan() {
		if read += len(sc.Bytes()) + 1; read > maxSSHConfigBytes {
			break
		}
		keyword, arg := splitSSHLine(sc.Text())
		if r.line(keyword, arg, depth) {
			break
		}
	}
	return true
}

// line applies one line and reports whether the rest of the file is conditional (a Match line).
func (r *sshReader) line(keyword, arg string, depth int) (stop bool) {
	switch keyword {
	case "":
	case "match":
		return true
	case "include":
		for _, pattern := range strings.Fields(arg) {
			r.include(pattern, depth)
		}
	default:
		if _, seen := r.vals[keyword]; !seen {
			r.vals[keyword] = strings.ToLower(firstField(arg))
		}
	}
	return false
}

// include reads the files a pattern names, in name order. It stays inside the SSH folder: a path that leaves it is ignored,
// because a configuration file must not be able to send this reader to any other file.
func (r *sshReader) include(pattern string, depth int) {
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(r.h.SSHDir, pattern)
	}
	pattern = r.h.contain(pattern)
	if pattern == "" {
		return
	}
	matches, _ := filepath.Glob(pattern) // sorted; only a malformed pattern errors
	for _, m := range matches {
		r.file(m, depth+1)
	}
}

// contain returns the cleaned path when it is inside the SSH folder, and "" otherwise. A real configuration names
// /etc/ssh/..., so an absolute pattern is accepted only under the folder this reader was given.
func (h Hardener) contain(path string) string {
	clean := filepath.Clean(path)
	if rel, err := filepath.Rel(filepath.Clean(h.SSHDir), clean); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return h.realFolder(clean)
	}
	return clean
}

// realFolder accepts an absolute /etc/ssh path when the reader was pointed at another folder (as tests do), by mapping it
// onto that folder. Anything else is outside.
func (h Hardener) realFolder(clean string) string {
	const real = "/etc/ssh"
	if rel, ok := strings.CutPrefix(clean, real+"/"); ok && !strings.Contains(rel, "..") {
		return filepath.Join(h.SSHDir, rel)
	}
	return ""
}

// splitSSHLine returns the lower-case keyword and the rest of a line, which sshd separates by space, tab or "=".
// Comments and blank lines give an empty keyword.
func splitSSHLine(line string) (keyword, arg string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", ""
	}
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return strings.ToLower(line), ""
	}
	return strings.ToLower(line[:i]), strings.TrimLeft(line[i:], " \t=")
}

func firstField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func (h Hardener) rpFilterOff() bool {
	for _, which := range []string{"all", "default"} {
		raw, err := os.ReadFile(filepath.Join(h.SysDir, "net", "ipv4", "conf", which, "rp_filter"))
		if err != nil || strings.TrimSpace(string(raw)) != "0" {
			return false
		}
	}
	return true
}
