package stackport

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/storage"
)

const (
	msgNotYours = "that file is not one the stack's manifest lists"
	msgNoFile   = "the file does not exist yet"
	msgIO       = "the file could not be read or written"
	corruptExt  = ".corrupt"
)

// DataDir is the SDK's DataDir over a folder and the names a stack's manifest lists. Only those names are reachable: a name with a
// separator, or one not listed, is refused before the disk is touched.
type DataDir struct {
	Dir   string
	Files []string
}

func (d DataDir) path(name string) (string, error) {
	if !slices.Contains(d.Files, name) || strings.ContainsAny(name, `/\`) {
		return "", stacksdk.Errorf(stacksdk.CodeDenied, msgNotYours)
	}
	return filepath.Join(d.Dir, name), nil
}

// Read returns the file's bytes.
func (d DataDir) Read(name string) ([]byte, error) {
	p, err := d.path(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, stacksdk.Errorf(stacksdk.CodeNotFound, msgNoFile)
	case err != nil:
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, msgIO)
	}
	return data, nil
}

// Write replaces the file atomically, with the permissions of the core's own user content.
func (d DataDir) Write(name string, data []byte) error {
	p, err := d.path(name)
	if err != nil {
		return err
	}
	pattern := strings.TrimSuffix(name, filepath.Ext(name)) + "-*.json.tmp"
	if err := storage.WriteAtomic(p, pattern, data, storage.ClassUserContent); err != nil {
		return stacksdk.Errorf(stacksdk.CodeInternal, msgIO)
	}
	return nil
}

// Quarantine sets the file aside as name + ".corrupt".
func (d DataDir) Quarantine(name string) error {
	p, err := d.path(name)
	if err != nil {
		return err
	}
	if err := os.Rename(p, p+corruptExt); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return stacksdk.Errorf(stacksdk.CodeInternal, msgIO)
	}
	return nil
}

// Remove deletes the named files, and any copy set aside as corrupt, from dir: what a factory reset does for the files a stack's manifest
// lists. A file that is not there is not an error.
func Remove(dir string, files []string) error {
	var errs []error
	for _, name := range files {
		for _, p := range []string{filepath.Join(dir, name), filepath.Join(dir, name) + corruptExt} {
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("failed to delete %s: %w", name, err))
			}
		}
	}
	return errors.Join(errs...)
}
