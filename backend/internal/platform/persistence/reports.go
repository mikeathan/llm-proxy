package persistence

import (
	"fmt"
	"path"
	"strings"
	"time"
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
	name := at.UTC().Format(reportStampLayout) + "-" + reportPathPart(runID) + ".md"
	rel := path.Join(reportsDirName, reportPathPart(automation), name)
	if err := m.WriteTaskFile(workspaceID, rel, content); err != nil {
		return "", fmt.Errorf("failed to save report: %w", err)
	}
	return rel, nil
}
