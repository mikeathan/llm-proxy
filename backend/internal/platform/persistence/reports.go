package persistence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"llm-proxy/models"
)

// reportsDirName is the workspace folder holding each automation's saved final reports. It sits inside the workspace,
// not the metadata folder, so the Files page and the agent's read_file can open a report like any other file.
const reportsDirName = "reports"

// reportStampLayout orders reports by run start time and has no character a file name cannot hold.
const reportStampLayout = "20060102T150405Z"

// reportPathPart makes a name safe as one path segment: anything outside letters, digits, dot, dash and underscore
// becomes "_", and leading dots go so the segment can never be "." or "..". An empty result is "unnamed".
func reportPathPart(name string) string {
	part := strings.TrimLeft(unsafeNameRe.ReplaceAllString(name, "_"), ".")
	if part == "" {
		return "unnamed"
	}
	return part
}

// WriteReport saves an automation run's final report as reports/<automation>/<start time>-<run id>.md in the
// workspace and returns that path relative to the workspace. The file is written atomically inside the workspace
// root; writing the same run again replaces it. Nothing here deletes a report: a later run never overwrites an
// earlier one, and only an operator removes them.
func (m *WorkspaceManager) WriteReport(workspaceID, automation, runID string, at time.Time, content string) (string, error) {
	name := at.UTC().Format(reportStampLayout) + "-" + reportPathPart(runID) + reportFileSuffix
	rel := path.Join(reportsDirName, reportPathPart(automation), name)
	if err := m.WriteTaskFile(workspaceID, rel, content); err != nil {
		return "", fmt.Errorf("failed to save report: %w", err)
	}
	return rel, nil
}

// MaxListedReports bounds ListReports so a long-running automation cannot make the list unbounded; the newest are kept.
const MaxListedReports = 200

// reportFileSuffix ends every file WriteReport makes.
const reportFileSuffix = ".md"

// ListReports lists an automation's saved reports, newest first, at most MaxListedReports. Only files WriteReport
// could have made are listed: a note a user put in the folder, a sub-folder or a file with a malformed name is skipped.
// An automation with no reports (or a workspace that does not exist) has an empty, non-nil list.
func (m *WorkspaceManager) ListReports(workspaceID, automation string) (models.SavedReports, error) {
	list := models.SavedReports{Reports: []models.SavedReport{}}
	dir := path.Join(reportsDirName, reportPathPart(automation))
	err := m.withWorkspaceRoot(workspaceID, false, func(root *os.Root) error {
		entries, readErr := fs.ReadDir(root.FS(), dir)
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			if report, ok := reportFromEntry(dir, entry); ok {
				list.Reports = append(list.Reports, report)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return list, nil
		}
		return list, fmt.Errorf("failed to list reports: %w", err)
	}
	sort.Slice(list.Reports, func(i, j int) bool {
		if !list.Reports[i].StartedAt.Equal(list.Reports[j].StartedAt) {
			return list.Reports[i].StartedAt.After(list.Reports[j].StartedAt)
		}
		return list.Reports[i].Path > list.Reports[j].Path
	})
	if len(list.Reports) > MaxListedReports {
		list.Reports, list.Truncated = list.Reports[:MaxListedReports], true
	}
	return list, nil
}

// reportFromEntry reads a report's run id and start time back from its file name; it reports false for anything
// WriteReport did not write.
func reportFromEntry(dir string, entry fs.DirEntry) (models.SavedReport, bool) {
	name := entry.Name()
	if entry.IsDir() || !strings.HasSuffix(name, reportFileSuffix) {
		return models.SavedReport{}, false
	}
	stamp, runID, found := strings.Cut(strings.TrimSuffix(name, reportFileSuffix), "-")
	if !found || runID == "" {
		return models.SavedReport{}, false
	}
	startedAt, err := time.Parse(reportStampLayout, stamp)
	if err != nil {
		return models.SavedReport{}, false
	}
	info, err := entry.Info()
	if err != nil || !info.Mode().IsRegular() {
		return models.SavedReport{}, false
	}
	return models.SavedReport{Path: path.Join(dir, name), RunID: runID, StartedAt: startedAt, SizeBytes: info.Size()}, true
}
