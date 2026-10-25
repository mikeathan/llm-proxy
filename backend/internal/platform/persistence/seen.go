package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"

	"llm-proxy/models"
)

// seenDirName is the metadata subfolder holding per-automation seen ledgers. It
// lives beside state.json in the workspace's metadata folder, outside the
// agent's workspace jail, so a run cannot edit what it is told to skip.
const seenDirName = "seen"

var unsafeNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// seenPath names one ledger file.
func (m *WorkspaceManager) seenPath(workspaceID, automation string) string {
	return m.automationFilePath(workspaceID, seenDirName, automation, ".json")
}

// automationFilePath names one per-automation file in a metadata subfolder: the
// name is sanitised and suffixed with a short hash so two names that sanitise
// alike never share a file, and a traversing name stays inside the folder.
func (m *WorkspaceManager) automationFilePath(workspaceID, dir, automation, ext string) string {
	sum := sha256.Sum256([]byte(automation))
	name := unsafeNameRe.ReplaceAllString(automation, "_") + "-" + hex.EncodeToString(sum[:4]) + ext
	return filepath.Join(m.resolver.InternalDir(workspaceID), dir, name)
}

// seenLedgerLabel names the ledger in the helper's error messages.
const seenLedgerLabel = "seen ledger"

// ReadSeen returns an automation's seen ledger. A missing file is an empty
// ledger, not an error; an undecodable one is quarantined as *.corrupt and
// reported as an error.
func (m *WorkspaceManager) ReadSeen(workspaceID, automation string) (models.SeenLedger, error) {
	ledger, err := readJSONFile[models.SeenLedger](m.seenPath(workspaceID, automation), seenLedgerLabel)
	if err != nil {
		return nil, err
	}
	if ledger == nil {
		ledger = models.SeenLedger{}
	}
	return ledger, nil
}

// WriteSeen replaces an automation's seen ledger atomically.
func (m *WorkspaceManager) WriteSeen(workspaceID, automation string, ledger models.SeenLedger) error {
	return writeJSONFile(m.seenPath(workspaceID, automation), "seen-*.json.tmp", seenLedgerLabel, ledger)
}

// DeleteSeen removes an automation's seen ledger, and any quarantined copy of
// it. A missing file is not an error.
func (m *WorkspaceManager) DeleteSeen(workspaceID, automation string) error {
	return removeJSONFile(m.seenPath(workspaceID, automation), seenLedgerLabel)
}
