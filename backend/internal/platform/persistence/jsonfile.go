package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"llm-proxy/internal/platform/storage"
)

// corruptSuffix marks an undecodable metadata file set aside by readJSONFile.
const corruptSuffix = ".corrupt"

// readJSONFile decodes the JSON file at path into a zero T. A missing file yields the zero value and no error; an
// undecodable one is quarantined as *.corrupt (the next write would otherwise overwrite it) and reported as an error.
// what names the data in error messages.
func readJSONFile[T any](path, what string) (T, error) {
	var out T
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, fmt.Errorf("failed to read %s: %w", what, err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		if renameErr := os.Rename(path, path+corruptSuffix); renameErr != nil {
			err = errors.Join(err, renameErr)
		}
		var zero T
		return zero, fmt.Errorf("failed to decode %s: %w", what, err)
	}
	return out, nil
}

// writeJSONFile replaces the file at path atomically. tmpPattern is the temp-file pattern the write goes through.
func writeJSONFile(path, tmpPattern, what string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode %s: %w", what, err)
	}
	return storage.WriteAtomic(path, tmpPattern, data, storage.ClassUserContent)
}

// removeJSONFile deletes the file at path and any quarantined copy of it. A missing file is not an error.
func removeJSONFile(path, what string) error {
	for _, p := range []string{path, path + corruptSuffix} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", what, err)
		}
	}
	return nil
}
