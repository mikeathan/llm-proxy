package persistence

import "llm-proxy/models"

// deviceInventoryFileName is who has been seen on the network, one file in the application's data folder: the network
// belongs to the host, not to a workspace, and a run cannot write it.
const deviceInventoryFileName = "devices.json"

// DeviceInventoryStore keeps the device inventory in one file.
type DeviceInventoryStore struct{ file rootFile[models.DeviceInventory] }

// NewDeviceInventoryStore keeps the inventory in dir. Nothing is read or created until Read or Write.
func NewDeviceInventoryStore(dir string) *DeviceInventoryStore {
	return &DeviceInventoryStore{file: newRootFile[models.DeviceInventory](dir, deviceInventoryFileName, "device inventory")}
}

// Read returns the stored inventory. A missing file is an empty inventory, not an error; an undecodable one is quarantined
// as *.corrupt and reported as an error.
func (s *DeviceInventoryStore) Read() (models.DeviceInventory, error) { return s.file.read() }

// Write replaces the stored inventory atomically.
func (s *DeviceInventoryStore) Write(inv models.DeviceInventory) error { return s.file.write(inv) }

// Delete removes the stored inventory, and any copy set aside as corrupt. There being none is not an error.
func (s *DeviceInventoryStore) Delete() error { return s.file.remove() }
