package sandbox

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultProfileGrantsWorkspaceAndToolchains(t *testing.T) {
	root := "/data/workspaces/ws-1"
	toolDirs := []string{"/usr/bin", "/usr/lib", "/opt/homebrew"}
	support := []string{"/etc/ssl/certs", "/etc/resolv.conf", "/dev/null"}
	p := DefaultProfile(root, toolDirs, support)

	got := map[string]FSPermission{}
	for _, r := range p.Rules {
		got[r.Path] |= r.Perm
	}

	if got[root]&(PermRead|PermWrite) != PermRead|PermWrite {
		t.Errorf("workspace root must be read+write: %+v", got[root])
	}
	for _, sub := range []string{".sandbox", ".sandbox/go-cache", ".sandbox/node_modules/.bin"} {
		path := filepath.Join(root, sub)
		if got[path]&(PermRead|PermWrite) != PermRead|PermWrite {
			t.Errorf("%s must be read+write: got perm %v", path, got[path])
		}
	}
	for _, d := range toolDirs {
		if got[d]&PermRead == 0 {
			t.Errorf("toolchain dir %s must be granted read/exec", d)
		}
		if got[d]&PermWrite != 0 {
			t.Errorf("toolchain dir %s must NOT be writable", d)
		}
	}
	for _, s := range support {
		if got[s]&PermRead == 0 {
			t.Errorf("support path %s must be readable", s)
		}
	}
}

func TestDefaultProfileDeniesByDefault(t *testing.T) {
	p := DefaultProfile("/w", []string{"/usr/bin"}, nil)
	for _, r := range p.Rules {
		if strings.Contains(r.Path, ".ssh") || strings.Contains(r.Path, "docker.sock") {
			t.Errorf("secret/socket paths must never be granted: %s", r.Path)
		}
	}
	// The grant set is small and targeted — no blanket home grants.
	for _, r := range p.Rules {
		if r.Path == "/" || (strings.HasPrefix(r.Path, "/home/") && r.Perm&PermWrite != 0) {
			t.Errorf("unexpected broad rule: %s perm=%v", r.Path, r.Perm)
		}
	}
}

func TestNeedTraverseAnnotatesAncestors(t *testing.T) {
	p := DefaultProfile("/data/workspaces/ws-1", []string{"/usr/bin"}, nil)
	p.NeedTraverse()

	perms := map[string]FSPermission{}
	for _, r := range p.Rules {
		perms[r.Path] |= r.Perm
	}
	for _, anc := range []string{"/", "/data", "/data/workspaces"} {
		if perms[anc]&PermTraverse == 0 {
			t.Errorf("ancestor %s must be marked traverse", anc)
		}
	}
}
