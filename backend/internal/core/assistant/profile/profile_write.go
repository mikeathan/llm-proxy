package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"gopkg.in/yaml.v3"
)

var (
	// ErrInvalid wraps every reason a profile cannot be saved because of what it says; the message tells the user what to fix.
	ErrInvalid = errors.New("invalid profile")
	// ErrBuiltin is a refusal to change a built-in: it is duplicated under a new id instead.
	ErrBuiltin = errors.New("built-in profile")
	// ErrNotFound is a delete of a profile that has no user file and is not built in.
	ErrNotFound = errors.New("profile not found")
)

const tempFilePrefix = ".saving-"

// Marshal writes the profile as a file: the inverse of Parse. The source is not stored, and a memory_save left at its
// default is left out.
func Marshal(p Profile) []byte {
	fm := frontMatter{
		Name: p.Name, Summary: p.Summary, AnswerFirst: p.AnswerFirst, MayAsk: p.MayAsk, MaxSteps: p.MaxSteps,
		Tools: p.Tools, Memory: p.Memory, Reminders: p.Reminders, Delegates: p.Delegates, LocalOnly: p.LocalOnly,
	}
	if p.MemorySave.Effective() != SaveModel {
		fm.MemorySave = p.MemorySave
	}
	header, _ := yaml.Marshal(fm) // a struct of strings, bools, ints and string lists cannot fail to marshal
	return []byte(frontMatterFence + "\n" + string(header) + frontMatterFence + "\n\n" + p.Instructions + "\n")
}

// Save writes p as the user's own profile file. What is written is what Parse accepts, so a saved profile always loads.
// A built-in is never overwritten: its id is taken, and the user duplicates it under another.
func (c *Catalog) Save(p Profile) error {
	data := Marshal(p)
	if _, err := Parse(p.ID, User, data); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if len(data) > maxFileBytes {
		return fmt.Errorf("%w: the profile is too large (%d bytes, at most %d)", ErrInvalid, len(data), maxFileBytes)
	}
	for _, id := range p.Delegates {
		if _, ok := c.Get(id); !ok {
			return fmt.Errorf("%w: delegates: there is no profile %q", ErrInvalid, id)
		}
	}
	if c.userDir == "" {
		return errors.New("save profile: no profiles folder is configured")
	}
	if err := c.checkRoomFor(p.ID); err != nil {
		return err
	}
	return writeAtomic(c.userDir, p.ID+profileExt, data)
}

// checkRoomFor refuses a new file for a built-in's id and a new file past the limit; editing an existing user file is
// always allowed.
func (c *Catalog) checkRoomFor(id string) error {
	if _, err := os.Stat(filepath.Join(c.userDir, id+profileExt)); err == nil {
		return nil
	}
	if _, builtin := c.builtin[id]; builtin {
		return fmt.Errorf("%w: %q cannot be changed. Duplicate it under a new id", ErrBuiltin, id)
	}
	if len(c.userIDs()) >= maxUserProfiles {
		return fmt.Errorf("%w: at most %d of your own profiles", ErrInvalid, maxUserProfiles)
	}
	return nil
}

// writeAtomic writes through a temp file in the same folder, so a crash never leaves a half-written profile.
func writeAtomic(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	tmp, err := os.CreateTemp(dir, tempFilePrefix+"*")
	if err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("save profile: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("save profile: %w", err)
	}
	return nil
}

// Delete removes the user's own file for id. A built-in the file replaced comes back; a built-in with no user file is
// refused.
func (c *Catalog) Delete(id string) error {
	if !ValidID(id) {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	if c.userDir != "" {
		err := os.Remove(filepath.Join(c.userDir, id+profileExt))
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete profile: %w", err)
		}
	}
	if _, builtin := c.builtin[id]; builtin {
		return fmt.Errorf("%w: %q cannot be deleted", ErrBuiltin, id)
	}
	return fmt.Errorf("%w: %q", ErrNotFound, id)
}

// HasBuiltin reports whether a built-in profile has this id, whether or not a user file replaces it.
func (c *Catalog) HasBuiltin(id string) bool {
	_, ok := c.builtin[id]
	return ok
}

// DelegatingTo lists the profiles that name id as a helper, sorted.
func (c *Catalog) DelegatingTo(id string) []string {
	var out []string
	for _, p := range c.List() {
		if slices.Contains(p.Delegates, id) {
			out = append(out, p.ID)
		}
	}
	return out
}
