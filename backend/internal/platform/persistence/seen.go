package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// seenDirName is the metadata subfolder holding per-automation seen ledgers. It
// lives beside state.json in the workspace's metadata folder, outside the
// agent's workspace jail, so a run cannot edit what it is told to skip.
const seenDirName = "seen"

var unsafeNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// seenPath names one ledger file. The automation name is sanitised and suffixed
// with a short hash so two names that sanitise alike never share a file.
func (m *WorkspaceManager) seenPath(workspaceID, automation string) string {
	sum := sha256.Sum256([]byte(automation))
	name := unsafeNameRe.ReplaceAllString(automation, "_") + "-" + hex.EncodeToString(sum[:4]) + ".json"
	return filepath.Join(m.resolver.InternalDir(workspaceID), seenDirName, name)
}

// ReadSeen returns an automation's seen ledger. A missing file is an empty
// ledger, not an error.
func (m *WorkspaceManager) ReadSeen(workspaceID, automation string) (models.SeenLedger, error) {
	data, err := os.ReadFile(m.seenPath(workspaceID, automation))
	if err != nil {
		if os.IsNotExist(err) {
			return models.SeenLedger{}, nil
		}
		return nil, fmt.Errorf("failed to read seen ledger: %w", err)
	}
	ledger := models.SeenLedger{}
	if err := json.Unmarshal(data, &ledger); err != nil {
		return nil, fmt.Errorf("failed to decode seen ledger: %w", err)
	}
	return ledger, nil
}

// WriteSeen replaces an automation's seen ledger atomically.
func (m *WorkspaceManager) WriteSeen(workspaceID, automation string, ledger models.SeenLedger) error {
	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode seen ledger: %w", err)
	}
	return storage.WriteAtomic(m.seenPath(workspaceID, automation), "seen-*.json.tmp", data, storage.ClassUserContent)
}
