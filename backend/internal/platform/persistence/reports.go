package persistence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"llm-proxy/internal/platform/logging"
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

// reportFileName is the report inside a run's folder; every other file there is one more file of the same run.
const reportFileName = "report.md"

// extraFileNameRe is what a file saved beside a report must look like: a plain name, never a path, never hidden.
var extraFileNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,63}$`)

// runFolderName is a run's folder: its start time (UTC) and id, so folders sort by time and the id reads back from the name.
func runFolderName(at time.Time, runID string) string {
	return at.UTC().Format(reportStampLayout) + "-" + reportPathPart(runID)
}

// WriteReport saves an automation run's final report as reports/<automation>/<start time>-<run id>/report.md in the
// workspace, with any extra files (an audit's typed record) in the same folder, and returns the report's path relative to
// the workspace. Extras are written first and the report last, so a folder with a report is a complete one. Each file is
// written atomically inside the workspace root; writing the same run again replaces them. A later run never overwrites an
// earlier one; only the oldest runs beyond MaxSavedReports are removed (pruneReports), and a failed prune never fails the write.
func (m *WorkspaceManager) WriteReport(workspaceID, automation, runID string, at time.Time, content string, extras ...models.ReportFile) (string, error) {
	folder := path.Join(reportsDirName, reportPathPart(automation), runFolderName(at, runID))
	for _, f := range extras {
		if !extraFileNameRe.MatchString(f.Name) || f.Name == reportFileName {
			return "", fmt.Errorf("failed to save report: %q is not a name for a file beside the report", f.Name)
		}
		if err := m.WriteTaskFile(workspaceID, path.Join(folder, f.Name), f.Content); err != nil {
			return "", fmt.Errorf("failed to save report: %w", err)
		}
	}
	rel := path.Join(folder, reportFileName)
	if err := m.WriteTaskFile(workspaceID, rel, content); err != nil {
		return "", fmt.Errorf("failed to save report: %w", err)
	}
	if err := m.pruneReports(workspaceID, path.Dir(path.Dir(rel)), MaxSavedReports); err != nil {
		logging.Warn("could not prune old saved reports", "workspace", workspaceID, "folder", path.Dir(path.Dir(rel)), "error", err)
	}
	return rel, nil
}

// MaxSavedReports is how many reports an automation keeps; writing one more removes the oldest. A count bounds the
// folder for any schedule (a daily brief keeps about 200 days, a five-minute check about 17 hours), and it equals
// MaxListedReports so every kept report is listed.
const MaxSavedReports = MaxListedReports

// pruneReports removes the oldest reports in dir beyond keep. It touches only files WriteReport could have made
// (reportFromEntry): a note, sub-folder or oddly named file a user put in the folder is never deleted.
func (m *WorkspaceManager) pruneReports(workspaceID, dir string, keep int) error {
	return m.withWorkspaceRoot(workspaceID, false, func(root *os.Root) error {
		reports, err := reportsIn(root, dir)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && len(reports) <= keep) {
			return nil
		}
		if err != nil {
			return err
		}
		var failed []error
		for _, old := range reports[keep:] {
			if rmErr := removeReport(root, old.Path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				failed = append(failed, rmErr)
			}
		}
		return errors.Join(failed...)
	})
}

// removeReport removes one saved report. A report in a run folder goes with the plain files beside it, then the folder if nothing
// else is in it: a sub-folder a person made keeps the folder (and what is in it). A report saved before run folders is one file.
func removeReport(root *os.Root, reportPath string) error {
	if path.Base(reportPath) != reportFileName {
		return root.Remove(reportPath)
	}
	folder := path.Dir(reportPath)
	entries, err := fs.ReadDir(root.FS(), folder)
	if err != nil {
		return err
	}
	var failed []error
	for _, e := range entries {
		if e.Type().IsRegular() {
			if rmErr := root.Remove(path.Join(folder, e.Name())); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				failed = append(failed, rmErr)
			}
		}
	}
	if rmErr := root.Remove(folder); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) && !isNotEmpty(rmErr) {
		failed = append(failed, rmErr)
	}
	return errors.Join(failed...)
}

// isNotEmpty reports whether a Remove failed only because the folder still holds something.
func isNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST)
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
		var readErr error
		list.Reports, readErr = reportsIn(root, dir)
		return readErr
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return models.SavedReports{Reports: []models.SavedReport{}}, nil
		}
		return list, fmt.Errorf("failed to list reports: %w", err)
	}
	if len(list.Reports) > MaxListedReports {
		list.Reports, list.Truncated = list.Reports[:MaxListedReports], true
	}
	return list, nil
}

// reportsIn reads every report WriteReport made in dir, newest first.
func reportsIn(root *os.Root, dir string) ([]models.SavedReport, error) {
	entries, err := fs.ReadDir(root.FS(), dir)
	if err != nil {
		return nil, err
	}
	reports := []models.SavedReport{}
	for _, entry := range entries {
		if report, ok := reportFromEntry(root, dir, entry); ok {
			reports = append(reports, report)
		}
	}
	sort.Slice(reports, func(i, j int) bool {
		if !reports[i].StartedAt.Equal(reports[j].StartedAt) {
			return reports[i].StartedAt.After(reports[j].StartedAt)
		}
		return reports[i].Path > reports[j].Path
	})
	return reports, nil
}

// reportFromEntry reads a report's run id and start time back from its name; it reports false for anything WriteReport did
// not write. A run folder counts only when it holds report.md; a flat file is a report saved before run folders.
func reportFromEntry(root *os.Root, dir string, entry fs.DirEntry) (models.SavedReport, bool) {
	name := entry.Name()
	stampAndRun := name
	reportPath := path.Join(dir, name)
	if entry.IsDir() {
		reportPath = path.Join(dir, name, reportFileName)
	} else if !strings.HasSuffix(name, reportFileSuffix) {
		return models.SavedReport{}, false
	} else {
		stampAndRun = strings.TrimSuffix(name, reportFileSuffix)
	}
	stamp, runID, found := strings.Cut(stampAndRun, "-")
	if !found || runID == "" {
		return models.SavedReport{}, false
	}
	startedAt, err := time.Parse(reportStampLayout, stamp)
	if err != nil {
		return models.SavedReport{}, false
	}
	info, err := root.Stat(reportPath)
	if err != nil || !info.Mode().IsRegular() {
		return models.SavedReport{}, false
	}
	return models.SavedReport{Path: reportPath, RunID: runID, StartedAt: startedAt, SizeBytes: info.Size()}, true
}
