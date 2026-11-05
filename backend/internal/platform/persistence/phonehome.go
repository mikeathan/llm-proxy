package persistence

import "llm-proxy/models"

// phoneHomeFileName holds the phone-home watch's names, one file in the application's data folder.
const phoneHomeFileName = "phone-home.json"

// PhoneHomeStore keeps the names each device asked for in one file.
type PhoneHomeStore struct {
	file rootFile[models.PhoneHome]
}

func NewPhoneHomeStore(dir string) *PhoneHomeStore {
	return &PhoneHomeStore{file: newRootFile[models.PhoneHome](dir, phoneHomeFileName, "phone-home names")}
}

// Read returns the stored names. A missing file is an empty record; an undecodable one is quarantined and reported as an error.
func (s *PhoneHomeStore) Read() (models.PhoneHome, error) { return s.file.read() }

// Write replaces the stored names atomically.
func (s *PhoneHomeStore) Write(r models.PhoneHome) error { return s.file.write(r) }

// Delete removes the file and any copy set aside as corrupt.
func (s *PhoneHomeStore) Delete() error { return s.file.remove() }
