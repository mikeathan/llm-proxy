package hostwatch

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Privileger reads the marks of privilege and persistence that live in files any account can read on Linux: who has root's user
// id, who can log in, who is in a group that is root or close to it, which kernel modules are loaded, whether the dynamic loader
// is told to load a library into every program, and which set-user-ID or set-group-ID programs sit outside the packaged
// folders. Like the rest of the package it starts no program and keeps no text from the files except plain names (an account,
// a group, a module) and plain paths, each checked for shape.
type Privileger struct {
	// Passwd is "/etc/passwd", Group "/etc/group", Modules "/proc/modules" and Preload "/etc/ld.so.preload" on a real computer.
	// An empty Modules or Preload leaves that mark out.
	Passwd  string
	Group   string
	Modules string
	Preload string
	// SetIDRoots are the folders walked for set-id files ("/usr/local", "/opt", "/srv"): places nothing packaged writes to.
	// MaxSetIDEntries bounds the walk (defaultSetIDEntries when zero); a walk that reaches it is incomplete.
	SetIDRoots      []string
	MaxSetIDEntries int
}

// Privilege is what one look found.
type Privilege struct {
	// Checked is false on a computer without an account file (macOS keeps its accounts elsewhere): not checked, never "fine".
	Checked bool
	// ExtraRoot are the accounts other than root whose user id is 0, sorted.
	ExtraRoot []string
	// LoginAccounts are the accounts with a login shell, sorted.
	LoginAccounts []string
	// Admins are "group:account" for each member of a group that is root or close to it (adminGroups), sorted.
	Admins []string
	// Modules are the loaded kernel modules, sorted; ModulesRead is false when the list could not be read.
	Modules     []string
	ModulesRead bool
	// Preload is true when the loader's preload file names at least one library; PreloadDigest is a digest of its lines, so a
	// change shows.
	Preload       bool
	PreloadDigest string
	// SetID are the set-user-ID and set-group-ID files under the walked folders, sorted; SetIDComplete is false when the walk was
	// cut short or a folder could not be read.
	SetID         []string
	SetIDComplete bool
	// Odd counts names that were not plain tokens and were left out; Unreadable the files that exist but could not be read.
	Odd        int
	Unreadable int
}

// adminGroups are the groups whose members are root or one step from it: sudo and its older names, adm (reads every log),
// docker and lxd (start a container that mounts the root file system).
var adminGroups = []string{"sudo", "admin", "wheel", "adm", "docker", "lxd"}

// plainName is the shape a kept name must have: what useradd and the kernel accept, and nothing a message could misread.
var plainName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,31}$`)

// Limits on what one look reads.
const (
	maxAccountFileBytes = 1 << 20
	defaultSetIDEntries = 200_000
	maxSetIDKept        = 50
)

// noLoginShells end a shell path that does not give a login.
var noLoginShells = []string{"nologin", "false", "sync", "shutdown", "halt"}

// Check reads the marks.
func (p Privileger) Check() Privilege {
	var out Privilege
	gids, ok := p.accounts(&out)
	if !ok {
		return Privilege{}
	}
	out.Checked = true
	p.groups(&out, gids)
	p.modules(&out)
	p.preload(&out)
	p.setID(&out)
	return out
}

// lines reads a small text file line by line; a missing file is reported as such, so the caller can tell absent from unreadable.
func lines(path string, out *Privilege) ([]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			out.Unreadable++
		}
		return nil, false
	}
	defer f.Close()
	var all []string
	sc := bufio.NewScanner(io.LimitReader(f, maxAccountFileBytes))
	sc.Buffer(make([]byte, 0, 4096), 64<<10)
	for sc.Scan() {
		all = append(all, sc.Text())
	}
	return all, true
}

// keep adds name to list when it is a plain token, and counts it as odd otherwise.
func keep(list *[]string, name string, out *Privilege) {
	if plainName.MatchString(name) {
		*list = append(*list, name)
		return
	}
	out.Odd++
}

// accounts reads the account file and returns each account's own group id, for the primary-group members of an admin group.
func (p Privileger) accounts(out *Privilege) (map[string][]string, bool) {
	rows, ok := lines(p.Passwd, out)
	if !ok {
		return nil, false
	}
	byGID := map[string][]string{}
	for _, line := range rows {
		f := strings.Split(line, ":")
		if len(f) < 7 || strings.HasPrefix(line, "#") {
			continue
		}
		name, uid, gid, shell := f[0], f[2], f[3], f[6]
		if !plainName.MatchString(name) {
			out.Odd++
			continue
		}
		if uid == "0" && name != "root" {
			out.ExtraRoot = append(out.ExtraRoot, name)
		}
		if isLoginShell(shell) {
			out.LoginAccounts = append(out.LoginAccounts, name)
		}
		byGID[gid] = append(byGID[gid], name)
	}
	slices.Sort(out.ExtraRoot)
	slices.Sort(out.LoginAccounts)
	return byGID, true
}

func isLoginShell(shell string) bool {
	if shell == "" {
		return false
	}
	base := filepath.Base(shell)
	return !slices.Contains(noLoginShells, base)
}

// groups lists the members of the admin groups, named in the group file or by their own group id.
func (p Privileger) groups(out *Privilege, byGID map[string][]string) {
	rows, ok := lines(p.Group, out)
	if !ok {
		return
	}
	seen := map[string]bool{}
	add := func(group, member string) {
		if member == "" {
			return
		}
		entry := group + ":" + member
		if !plainName.MatchString(member) {
			out.Odd++
			return
		}
		if !seen[entry] {
			seen[entry] = true
			out.Admins = append(out.Admins, entry)
		}
	}
	for _, line := range rows {
		f := strings.Split(line, ":")
		if len(f) < 4 || !slices.Contains(adminGroups, f[0]) {
			continue
		}
		for _, m := range strings.Split(f[3], ",") {
			add(f[0], strings.TrimSpace(m))
		}
		for _, m := range byGID[f[2]] {
			add(f[0], m)
		}
	}
	slices.Sort(out.Admins)
}

// modules lists the loaded kernel modules: the first field of each line of /proc/modules.
func (p Privileger) modules(out *Privilege) {
	if p.Modules == "" {
		return
	}
	rows, ok := lines(p.Modules, out)
	if !ok {
		return
	}
	out.ModulesRead = true
	for _, line := range rows {
		if name, _, _ := strings.Cut(line, " "); name != "" {
			keep(&out.Modules, name, out)
		}
	}
	slices.Sort(out.Modules)
}

// preload reads the loader's preload file. Only whether it names a library and a digest of its lines are kept, never the lines.
func (p Privileger) preload(out *Privilege) {
	if p.Preload == "" {
		return
	}
	rows, ok := lines(p.Preload, out)
	if !ok {
		return
	}
	h := sha256.New()
	for _, line := range rows {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out.Preload = true
		io.WriteString(h, line+"\x00")
	}
	if out.Preload {
		out.PreloadDigest = hex.EncodeToString(h.Sum(nil))
	}
}

// setID walks the roots for regular files with the set-user-ID or set-group-ID bit, without following links, and keeps up to
// maxSetIDKept plain paths.
func (p Privileger) setID(out *Privilege) {
	if len(p.SetIDRoots) == 0 {
		return
	}
	w := setIDWalk{out: out, limit: p.MaxSetIDEntries}
	if w.limit <= 0 {
		w.limit = defaultSetIDEntries
	}
	out.SetIDComplete = true
	for _, root := range p.SetIDRoots {
		w.root = root
		if errors.Is(filepath.WalkDir(root, w.visit), errWalkLimit) {
			out.SetIDComplete = false
			break
		}
	}
	slices.Sort(out.SetID)
}

// errWalkLimit stops a walk that reached its entry limit.
var errWalkLimit = errors.New("set-id walk limit reached")

// setIDWalk is one walk over the set-id roots.
type setIDWalk struct {
	out     *Privilege
	root    string
	limit   int
	entries int
}

// visit looks at one entry. A missing root is not trouble; a root that cannot be read leaves the walk incomplete. A folder below a
// root that this account may not open (a container runtime's state, under /opt) is counted as unreadable and skipped: what is
// inside is not seen, which the report's count of unreadable places says, and treating it as an unfinished walk would leave the
// list never learned on a computer that has one.
func (w *setIDWalk) visit(path string, d fs.DirEntry, err error) error {
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case path == w.root:
			w.out.SetIDComplete = false
		default:
			w.out.Unreadable++
		}
		if d != nil && d.IsDir() {
			return fs.SkipDir
		}
		return nil
	}
	if w.entries++; w.entries > w.limit {
		return errWalkLimit
	}
	if !d.Type().IsRegular() || !isSetID(d) {
		return nil
	}
	if !safeListedPath.MatchString(path) {
		w.out.Odd++
	} else if len(w.out.SetID) < maxSetIDKept {
		w.out.SetID = append(w.out.SetID, path)
	}
	return nil
}

func isSetID(d fs.DirEntry) bool {
	info, err := d.Info()
	return err == nil && info.Mode()&(fs.ModeSetuid|fs.ModeSetgid) != 0
}
