package dnscheck

import (
	"encoding/json"
	"errors"
	"fmt"

	"llm-proxy-stacks/sdk"
)

// LogFile is the file the log of comparisons lives in, in the data folder. Its name and its JSON are what they were before the check
// moved into this stack, so an existing log reads unchanged.
const LogFile = "network-dns-checks.json"

// FileStore keeps the log in one file through the core's storage port. A missing file is an empty log; a file that cannot be read as
// a log is set aside and reported once, and the next read starts empty.
type FileStore struct{ Dir stacksdk.DataDir }

// Read returns the log.
func (s FileStore) Read() ([]Comparison, error) {
	data, err := s.Dir.Read(LogFile)
	if stacksdk.CodeOf(err) == stacksdk.CodeNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the network DNS checks: %w", err)
	}
	var out []Comparison
	if err := json.Unmarshal(data, &out); err != nil {
		err = fmt.Errorf("failed to decode the network DNS checks: %w", err)
		if qerr := s.Dir.Quarantine(LogFile); qerr != nil {
			err = errors.Join(err, qerr)
		}
		return nil, err
	}
	return out, nil
}

// Write replaces the log.
func (s FileStore) Write(items []Comparison) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode the network DNS checks: %w", err)
	}
	if err := s.Dir.Write(LogFile, data); err != nil {
		return fmt.Errorf("failed to save the network DNS checks: %w", err)
	}
	return nil
}
