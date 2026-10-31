package persistence

import "llm-proxy/models"

// devicePortsFileName holds the ports each device had open at its last completed scan, one file in the data folder.
const devicePortsFileName = "device-ports.json"

// DevicePortStore keeps the port record in one file.
type DevicePortStore struct{ file rootFile[models.DevicePorts] }

func NewDevicePortStore(dir string) *DevicePortStore {
	return &DevicePortStore{file: newRootFile[models.DevicePorts](dir, devicePortsFileName, "device port record")}
}

// Read returns the record; a missing file is an empty record.
func (s *DevicePortStore) Read() (models.DevicePorts, error) { return s.file.read() }

// Write replaces the record atomically.
func (s *DevicePortStore) Write(r models.DevicePorts) error { return s.file.write(r) }

// Delete removes the file and any copy set aside as corrupt.
func (s *DevicePortStore) Delete() error { return s.file.remove() }
