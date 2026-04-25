package storage

import (
	"llm-proxy/models"
	"path/filepath"
)

// Resolver defines the interface for mapping workspace and system identifiers to filesystem paths.
type Resolver interface {
	RootDir() string
	WorkspacesRoot() string
	MetadataRoot() string
	WorkspaceDir(id string) string
	InternalDir(id string) string
	Config(id string) string
	State(id string) string
	Lock(id string) string
	ProcessLog(id string) string
	TaskFile(id, filename string) string
	Heartbeat(id string) string
}

type PathResolver struct {
	rootDir       string
	workspacesDir string
	metadataDir   string
}

func NewPathResolver(rootDir, workspacesDir, metadataDir string) *PathResolver {
	return &PathResolver{
		rootDir:       rootDir,
		workspacesDir: workspacesDir,
		metadataDir:   metadataDir,
	}
}

func (r *PathResolver) RootDir() string {
	return r.rootDir
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

// InternalDir returns the path to the hidden metadata folder for a workspace.
// This is now located in the central metadata directory, not inside the workspace.
func (r *PathResolver) InternalDir(id string) string {
	return filepath.Join(r.metadataDir, id)
}

func (r *PathResolver) Lock(id string) string {
	return filepath.Join(r.InternalDir(id), models.LockFilename)
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

func (r *PathResolver) MetadataRoot() string {
	return r.metadataDir
}
