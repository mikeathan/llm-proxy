package profile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/platform/logging"
)

// Kind is what a run is for; it picks the default profile when nothing chose one.
type Kind string

const (
	KindAssistant  Kind = "assistant"  // an interactive chat
	KindAutomation Kind = "automation" // a scheduled or manual task run (the heartbeat names its own profile)
)

// defaultIDs is the built-in profile each kind starts with.
var defaultIDs = map[Kind]string{
	KindAssistant:  "general",
	KindAutomation: "task-runner",
}

const (
	profileExt      = ".md"
	builtinDir      = "profiles"
	maxFileBytes    = 64 * 1024 // a profile is a page of instructions; this guards the prompt, not the disk
	maxUserProfiles = 50
)

// Catalog finds profiles: the built-ins, then the user's files, which add to them and may replace one by using its id.
// User files are read on demand, so an edit applies to the next run without a restart.
type Catalog struct {
	userDir string
	log     logging.Logger
	builtin map[string]Profile
}

// NewCatalog parses the embedded built-ins (a malformed one is a build bug, so it fails fast). userDir may not exist.
func NewCatalog(userDir string, log logging.Logger) (*Catalog, error) {
	entries, err := fs.ReadDir(prompts.BuiltinProfileFiles, builtinDir)
	if err != nil {
		return nil, fmt.Errorf("read built-in profiles: %w", err)
	}
	builtin := make(map[string]Profile, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, profileExt) {
			continue
		}
		data, err := fs.ReadFile(prompts.BuiltinProfileFiles, builtinDir+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("read built-in profile %s: %w", name, err)
		}
		p, err := Parse(strings.TrimSuffix(name, profileExt), Builtin, data)
		if err != nil {
			return nil, err
		}
		builtin[p.ID] = p
	}
	for kind, id := range defaultIDs {
		if _, ok := builtin[id]; !ok {
			return nil, fmt.Errorf("the default profile %q for %s runs is not built in", id, kind)
		}
	}
	return &Catalog{userDir: userDir, log: log, builtin: builtin}, nil
}

// MustCatalog is NewCatalog for startup: the only way it fails is a malformed embedded profile, a build defect that
// TestBuiltinsAreTheShippedSix catches, so a failure here stops the program rather than being handled.
func MustCatalog(userDir string, log logging.Logger) *Catalog {
	c, err := NewCatalog(userDir, log)
	if err != nil {
		panic("profile: " + err.Error())
	}
	return c
}

// DefaultFor lists the kinds of run the profile with this id is the default for, sorted (empty, never nil).
func DefaultFor(id string) []Kind {
	kinds := []Kind{}
	for kind, defaultID := range defaultIDs {
		if defaultID == id {
			kinds = append(kinds, kind)
		}
	}
	slices.Sort(kinds)
	return kinds
}

// Get finds a profile by id: the user's file first, then the built-in.
func (c *Catalog) Get(id string) (Profile, bool) {
	if !ValidID(id) {
		return Profile{}, false
	}
	if p, ok := c.readUser(id); ok {
		return p, true
	}
	p, ok := c.builtin[id]
	return p, ok
}

// List returns every profile, sorted by id; a user file replaces the built-in with the same id.
func (c *Catalog) List() []Profile {
	byID := make(map[string]Profile, len(c.builtin))
	for id, p := range c.builtin {
		byID[id] = p
	}
	for _, id := range c.userIDs() {
		if p, ok := c.readUser(id); ok {
			byID[id] = p
		}
	}
	out := make([]Profile, 0, len(byID))
	for _, p := range byID {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Profile) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Resolve picks the profile for a run: the first requested id that exists (a message's choice, then the conversation's,
// then the workspace's), else the kind's default. An id that no longer exists — a deleted user file — is skipped.
func (c *Catalog) Resolve(kind Kind, requested ...string) Profile {
	for _, id := range requested {
		if id == "" {
			continue
		}
		if p, ok := c.Get(id); ok {
			return p
		}
	}
	p, _ := c.Get(defaultIDs[kind])
	return p
}

// userIDs lists the valid profile ids among the user's files, capped.
func (c *Catalog) userIDs() []string {
	entries, err := os.ReadDir(c.userDir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, entry := range entries {
		id, isProfile := strings.CutSuffix(entry.Name(), profileExt)
		if !isProfile || entry.IsDir() || !ValidID(id) {
			continue
		}
		if len(ids) == maxUserProfiles {
			c.log.Warn("too many user profiles; the rest are ignored", "dir", c.userDir, "max", maxUserProfiles)
			break
		}
		ids = append(ids, id)
	}
	return ids
}

// readUser reads and validates one user file; an unreadable or invalid one is skipped with a warning.
func (c *Catalog) readUser(id string) (Profile, bool) {
	path := filepath.Join(c.userDir, id+profileExt)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Profile{}, false
	}
	if info.Size() > maxFileBytes {
		c.log.Warn("profile file is too large and is ignored", "file", path, "bytes", info.Size(), "max", maxFileBytes)
		return Profile{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.log.Warn("profile file could not be read", "file", path, "error", err)
		return Profile{}, false
	}
	p, err := Parse(id, User, data)
	if err != nil {
		c.log.Warn("profile file is invalid and is ignored", "file", path, "error", err)
		return Profile{}, false
	}
	return p, true
}
