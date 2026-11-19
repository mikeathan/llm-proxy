package persistence

import "llm-proxy/models"

// dashboardFileName holds the dashboard layout, one file in the application's data folder.
const dashboardFileName = "dashboard.json"

// DashboardStore keeps the owner's dashboard layout in one file.
type DashboardStore struct {
	file rootFile[models.Dashboard]
}

func NewDashboardStore(dir string) *DashboardStore {
	return &DashboardStore{file: newRootFile[models.Dashboard](dir, dashboardFileName, "dashboard layout")}
}

// Read returns the layout. A missing file is an empty layout; an undecodable one is quarantined and reported as an error.
func (s *DashboardStore) Read() (models.Dashboard, error) { return s.file.read() }

// Write replaces the layout atomically.
func (s *DashboardStore) Write(d models.Dashboard) error { return s.file.write(d) }

// Delete removes the file and any copy set aside as corrupt.
func (s *DashboardStore) Delete() error { return s.file.remove() }
