package sandbox

import "path/filepath"

// FSPermission is an OS-neutral access grant for a profile path. Concrete OS
// mechanisms map it to their own rights (Landlock bits, Seatbelt clauses).
type FSPermission uint8

const (
	PermRead     FSPermission = 1 << iota // read/execute files + list dirs
	PermWrite                             // create/modify/remove entries
	PermTraverse                          // needed on ancestors above a granted path
)

// Rule grants permissions to an absolute path (directories grant their
// subtree). Deny is implicit: anything not granted is refused by
// deny-by-default mechanisms (Landlock) or explicitly denied by allow-default
// mechanisms (Seatbelt deny lists below the allow).
type Rule struct {
	Path string
	Perm FSPermission
}

// Profile is the per-workspace grant set an OS mechanism renders into its own
// rules. All paths absolute.
type Profile struct {
	Rules []Rule
}

// Add appends a rule (no-op for empty paths; relative paths are joined to base
// when provided by builders).
func (p *Profile) Add(path string, perm FSPermission) {
	if path == "" {
		return
	}
	p.Rules = append(p.Rules, Rule{Path: filepath.Clean(path), Perm: perm})
}

// workspaceSandboxDirs are the .sandbox runtime directories every workspace
// shell needs writable (env floor redirects HOME/GOPATH/caches/TMP there).
var workspaceSandboxDirs = []string{
	".sandbox",
	".sandbox/go",
	".sandbox/go-cache",
	".sandbox/tmp",
	".sandbox/cache",
	".sandbox/node_modules/.bin",
}

// DefaultProfile builds the grant set for one workspace (plan §4.7):
//   - workspace root + .sandbox runtime dirs: read+write
//   - toolchain/support roots (from the resolved PATH dirs and system roots):
//     read-only (read+traverse to execute binaries and read libs)
//   - support files (/dev nodes, /etc CA/resolv/hosts, /proc on Linux) passed
//     via supportPaths as read (or read+write for /dev/null)
//
// Deny-by-default handles everything else, including ~/.ssh and the socket
// hatches — they are simply never granted.
func DefaultProfile(workspaceRoot string, toolDirs, supportPaths []string) Profile {
	p := Profile{}
	if workspaceRoot == "" {
		return p
	}
	for _, sub := range workspaceSandboxDirs {
		p.Add(filepath.Join(workspaceRoot, sub), PermRead|PermWrite)
	}
	p.Add(workspaceRoot, PermRead|PermWrite)
	for _, d := range toolDirs {
		if d != "" {
			p.Add(d, PermRead)
		}
	}
	for _, s := range supportPaths {
		if s != "" {
			p.Add(s, PermRead)
		}
	}
	return p
}

// NeedTraverse marks every ancestor of granted paths up to and including the
// filesystem root ("/") with PermTraverse. Landlock (deny-by-default) requires
// traversal rights on path components above a granted subtree; Seatbelt ignores
// this field. Idempotent.
func (p *Profile) NeedTraverse() {
	seen := map[string]bool{}
	for _, r := range p.Rules {
		dir := filepath.Dir(r.Path)
		for {
			if seen[dir] {
				break
			}
			seen[dir] = true
			p.Add(dir, PermTraverse)
			if dir == "/" {
				break
			}
			dir = filepath.Dir(dir)
		}
	}
}
