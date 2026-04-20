package storage

import (
	"llm-proxy/models"
	"path/filepath"
)

type PathResolver struct {
	workspacesDir string
}

func NewPathResolver(workspacesDir string) *PathResolver {
	return &PathResolver{workspacesDir: workspacesDir}
}

func (r *PathResolver) WorkspaceDir(id string) string {
	return filepath.Join(r.workspacesDir, id)
}

func (r *PathResolver) Config(id string) string {
	return filepath.Join(r.InternalDir(id), models.ConfigFilename)
}

func (r *PathResolver) State(id string) string {
	return filepath.Join(r.InternalDir(id), models.StateFilename)
}

func (r *PathResolver) Heartbeat(id string) string {
	return filepath.Join(r.WorkspaceDir(id), models.HeartbeatFilename)
}

// InternalDir returns the path to the hidden metadata folder (.internal).
func (r *PathResolver) InternalDir(id string) string {
	return filepath.Join(r.WorkspaceDir(id), models.InternalDirName)
}

func (r *PathResolver) Lock(id string) string {
	return filepath.Join(r.WorkspaceDir(id), models.LockFilename)
}

func (r *PathResolver) ProcessLog(id string) string {
	return filepath.Join(r.InternalDir(id), models.ProcessLogFilename)
}

func (r *PathResolver) TaskFile(id, filename string) string {
	return filepath.Join(r.WorkspaceDir(id), filename)
}

func (r *PathResolver) WorkspacesRoot() string {
	return r.workspacesDir
}
