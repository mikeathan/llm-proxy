package hostwatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// FileSpec is one entry of the closed list of places that decide what runs on this computer after a reboot. Paths and globs
// are written in code; no operator, model or file chooses them. A path may be a file or a folder.
type FileSpec struct {
	ID    string
	Paths []string
	Globs []string
	// Program marks the running program's own file, which is replaced at every upgrade: it is compared only within one run of
	// the program (see Files.ProgramTag), so an upgrade followed by a restart is not a change.
	Program bool
	// SkipNames are path.Match patterns for file and link names (not folders) that are left out of the digest wherever they
	// appear under a folder in Paths or Globs. They exist for files a program writes by itself and routinely, such as snapd's
	// snap-<name>-<revision>.mount units; the patterns are written in code and must be as narrow as the noise they remove.
	SkipNames []string
	// MaxBytes caps how much of one file is read (the default is DefaultMaxBytes).
	MaxBytes int64
}

// Limits that keep one look short whatever is on the disk.
const (
	DefaultMaxBytes = 4 << 20
	programMaxBytes = 1 << 30
	maxEntries      = 4096 // files and folders looked at under one FileSpec
	maxGlobMatches  = 64
	maxDepth        = 2 // a folder, and the folders inside it: systemctl enable puts a link one folder down
)

// FileState is what one look at one FileSpec found. Hash is a digest only: no byte of any file is kept or shown.
type FileState struct {
	ID   string
	Hash string
	// Tag identifies the run of the program for the Program entry, empty otherwise.
	Tag string
	// Unreadable counts the paths this account may not read. They add a fixed token to the hash, so they never look like a
	// change, and the count lets the screen say what is not being watched.
	Unreadable int
}

// Files looks at a list of FileSpecs.
type Files struct {
	Specs []FileSpec
	// ProgramTag identifies this run of the program, for example its start time.
	ProgramTag string
}

// Observe hashes every spec. It never fails as a whole: a path it cannot read is counted, not fatal.
func (f Files) Observe() []FileState {
	out := make([]FileState, 0, len(f.Specs))
	for _, spec := range f.Specs {
		out = append(out, f.observe(spec))
	}
	return out
}

func (f Files) observe(spec FileSpec) FileState {
	limit := spec.MaxBytes
	if limit == 0 {
		limit = DefaultMaxBytes
	}
	if spec.Program {
		limit = programMaxBytes
	}
	h := sha256.New()
	st := FileState{ID: spec.ID}
	paths := slices.Clone(spec.Paths)
	for _, g := range spec.Globs {
		matches, _ := filepath.Glob(g) // only a malformed pattern errors, and the patterns are constants
		if len(matches) > maxGlobMatches {
			matches = matches[:maxGlobMatches]
		}
		paths = append(paths, matches...)
	}
	slices.Sort(paths)
	budget := maxEntries
	for _, p := range paths {
		walk(h, p, limit, 0, &budget, &st, spec.SkipNames)
	}
	st.Hash = hex.EncodeToString(h.Sum(nil))
	if spec.Program {
		st.Tag = f.ProgramTag
	}
	return st
}

// walk adds one path to the digest: its name, what it is, and for a file its content (up to limit bytes and its size), for
// a symlink its target text (the link is not followed), for a folder its entries in name order.
func walk(h io.Writer, path string, limit int64, depth int, budget *int, st *FileState, skip []string) {
	fmt.Fprintf(h, "%s\x00", path)
	if *budget <= 0 {
		io.WriteString(h, "over-budget\x00")
		return
	}
	*budget--
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		io.WriteString(h, "missing\x00")
		return
	case err != nil:
		st.Unreadable++
		io.WriteString(h, "unreadable\x00")
		return
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			st.Unreadable++
			io.WriteString(h, "unreadable\x00")
			return
		}
		fmt.Fprintf(h, "link\x00%s\x00", target)
	case info.IsDir():
		walkDir(h, path, limit, depth, budget, st, skip)
	case info.Mode().IsRegular():
		hashFile(h, path, info.Size(), limit, st)
	default:
		io.WriteString(h, "other\x00")
	}
}

func walkDir(h io.Writer, path string, limit int64, depth int, budget *int, st *FileState, skip []string) {
	io.WriteString(h, "dir\x00")
	entries, err := os.ReadDir(path)
	if err != nil {
		st.Unreadable++
		io.WriteString(h, "unreadable\x00")
		return
	}
	for _, e := range entries { // ReadDir returns them sorted by name
		if !e.IsDir() && skipped(skip, e.Name()) {
			continue
		}
		child := filepath.Join(path, e.Name())
		if e.IsDir() && depth+1 >= maxDepth { // deeper folders are named but not opened
			fmt.Fprintf(h, "%s\x00dir-not-opened\x00", child)
			continue
		}
		walk(h, child, limit, depth+1, budget, st, skip)
	}
}

// skipped reports whether name matches one of the SkipNames patterns. A pattern that does not parse never matches.
func skipped(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

func hashFile(h io.Writer, path string, size, limit int64, st *FileState) {
	file, err := os.Open(path)
	if err != nil {
		st.Unreadable++
		io.WriteString(h, "unreadable\x00")
		return
	}
	defer file.Close()
	io.WriteString(h, "file\x00"+strconv.FormatInt(size, 10)+"\x00")
	if _, err := io.Copy(h, io.LimitReader(file, limit)); err != nil {
		st.Unreadable++
		io.WriteString(h, "unreadable\x00")
	}
}

// DefaultFileSpecs is the closed list of what is watched: the places a change to survives a reboot. home is the service
// account's home folder and executable the running program's file; either may be empty, which leaves its entry out.
func DefaultFileSpecs(home, executable string) []FileSpec {
	keys := FileSpec{ID: "authorized_keys", Paths: []string{"/root/.ssh/authorized_keys"}, Globs: []string{"/home/*/.ssh/authorized_keys"}}
	if home != "" {
		keys.Paths = append(keys.Paths, filepath.Join(home, ".ssh", "authorized_keys"))
	}
	specs := []FileSpec{
		// snapd writes a mount unit (and a link enabling it) for every snap revision, so each refresh would otherwise raise a
		// finding; only that exact name shape is skipped, so a service, a timer or any other mount is still caught.
		{ID: "systemd_units", Paths: []string{"/etc/systemd/system"}, SkipNames: []string{"snap-*.mount"}},
		{ID: "cron", Paths: []string{
			"/etc/crontab", "/etc/cron.d", "/etc/cron.hourly", "/etc/cron.daily", "/etc/cron.weekly", "/etc/cron.monthly",
			"/var/spool/cron/crontabs",
		}},
		{ID: "ssh_server_config", Paths: []string{"/etc/ssh/sshd_config", "/etc/ssh/sshd_config.d"}},
		{ID: "sudoers", Paths: []string{"/etc/sudoers", "/etc/sudoers.d"}},
		// Where this computer sends its name lookups. A changed list is how traffic is quietly redirected. The systemd file lists
		// the real servers behind the 127.0.0.53 stub that /etc/resolv.conf names.
		{ID: "dns_settings", Paths: []string{
			"/etc/resolv.conf", "/run/systemd/resolve/resolv.conf", "/etc/systemd/resolved.conf", "/etc/systemd/resolved.conf.d",
		}},
		keys,
	}
	if executable != "" {
		specs = append(specs, FileSpec{ID: "program", Paths: []string{executable}, Program: true})
	}
	return specs
}

// Source is what the host watch reads: this computer's open ports and its watched files.
type Source struct {
	Proc    Proc
	Watch   Files
	Harden  Hardener
	Patcher Patcher
	Docker  Docker
	// Packages compares the program files with their packages' checksums; PackagesBudget is how long one pass may take.
	Packages       Integrity
	PackagesBudget time.Duration
	// Pkgs reads the installed packages and the Ubuntu release.
	Pkgs Packages
}

// Hardening returns the few weak settings this computer may have.
func (s Source) Hardening() Hardening { return s.Harden.Check() }

// Patches returns whether this computer is behind on updates or waiting for a restart.
func (s Source) Patches() Patches { return s.Patcher.Check() }

// Integrity compares the packaged program files with their packages' checksums within PackagesBudget. A pass that runs out of
// time is returned incomplete, never as a clean one.
func (s Source) Integrity() IntegrityResult {
	ctx := context.Background()
	if s.PackagesBudget > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.PackagesBudget)
		defer cancel()
	}
	return s.Packages.Check(ctx)
}

// Installed lists the installed packages with their versions.
func (s Source) Installed() (map[string]string, error) { return s.Pkgs.Installed() }

// DockerBridge returns whether Docker has a network on this computer.
func (s Source) DockerBridge() bool { return s.Docker.Bridge() }

// Listeners returns the ports open to other computers.
func (s Source) Listeners() ([]Listener, error) { return s.Proc.Listeners() }

// Files returns the digest of each watched place.
func (s Source) Files() []FileState { return s.Watch.Observe() }
