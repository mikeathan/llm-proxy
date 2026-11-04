package hostwatch

import (
	"bufio"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
)

// Limits of one read of the package list.
const (
	// MaxPackages is how many installed packages are listed; a computer has a few thousand.
	MaxPackages = 50_000
	// maxStatusBytes is the largest status file read (a real one is about 2 MB).
	maxStatusBytes = 64 << 20
	maxLineBytes   = 1 << 20
)

var (
	statusPackageName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]{0,99}$`)
	statusVersion     = regexp.MustCompile(`^[A-Za-z0-9.+~:-]{1,100}$`)
	releaseWord       = regexp.MustCompile(`^[a-z]{3,20}$`)
)

// Packages reads what this computer has installed and which Ubuntu release it is, from two files every account can read. It
// starts no program and keeps no text: a package becomes a name and a version that passed a plain-text check.
type Packages struct {
	// StatusFile is "/var/lib/dpkg/status" and OSRelease "/etc/os-release" on Ubuntu.
	StatusFile string
	OSRelease  string
}

// Installed lists the installed packages (dpkg's "installed" state, held ones included) with their versions. A line that
// continues a description (it starts with a space) is never a field. A package with a name or a version that is not plain text
// is left out. A file that cannot be read, or holds no package, is an error, never an empty list: nothing is judged "up to
// date" because the list could not be read.
func (p Packages) Installed() (map[string]string, error) {
	if p.StatusFile == "" {
		return nil, errors.New("no package list is configured")
	}
	f, err := os.Open(p.StatusFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(io.LimitReader(f, maxStatusBytes))
	sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	var cur stanza
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "Package: ") {
			cur.finish(out)
			cur = stanza{}
		}
		cur.read(line)
	}
	cur.finish(out)
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("the package list holds no installed package")
	}
	return out, nil
}

// Release is the codename of this computer's Ubuntu release ("noble"), or "" when it is not Ubuntu (a derivative is not), the
// file cannot be read or the codename is not a plain word.
func (p Packages) Release() string {
	f, err := os.Open(p.OSRelease)
	if err != nil {
		return ""
	}
	defer f.Close()
	var id, codename string
	sc := bufio.NewScanner(io.LimitReader(f, 1<<16))
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "ID":
			id = value
		case "VERSION_CODENAME":
			codename = value
		}
	}
	if id != "ubuntu" || !releaseWord.MatchString(codename) {
		return ""
	}
	return codename
}

// stanza is one package's entry in the status file, collected a line at a time.
type stanza struct {
	name, version string
	installed     bool
}

// read takes one line of the entry: its name, its state and its version. A line that continues a description (it starts with a
// space) is never a field.
func (s *stanza) read(line string) {
	switch {
	case line == "" || line[0] == ' ' || line[0] == '\t':
	case strings.HasPrefix(line, "Package: "):
		if n := strings.TrimSpace(line[len("Package: "):]); statusPackageName.MatchString(n) {
			s.name = n
		}
	case strings.HasPrefix(line, "Status: "):
		s.installed = strings.HasSuffix(strings.TrimSpace(line), " installed")
	case strings.HasPrefix(line, "Version: "):
		if v := strings.TrimSpace(line[len("Version: "):]); statusVersion.MatchString(v) {
			s.version = v
		}
	}
}

// finish adds the entry to out when it is an installed package with a plain name and version. The first entry of a name is
// kept (a multi-arch copy has the same version).
func (s stanza) finish(out map[string]string) {
	if s.name == "" || !s.installed || s.version == "" || len(out) >= MaxPackages {
		return
	}
	if _, seen := out[s.name]; !seen {
		out[s.name] = s.version
	}
}
