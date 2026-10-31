package persistence

import "llm-proxy/models"

// nameSuggestionsFileName is what devices called themselves when asked, one small file in the application's data folder.
const nameSuggestionsFileName = "device-names.json"

// NameSuggestionStore keeps the name suggestions in one file.
type NameSuggestionStore struct {
	file rootFile[models.NameSuggestions]
}

// NewNameSuggestionStore keeps the suggestions in dir. Nothing is read or created until Read or Write.
func NewNameSuggestionStore(dir string) *NameSuggestionStore {
	return &NameSuggestionStore{file: newRootFile[models.NameSuggestions](dir, nameSuggestionsFileName, "device name suggestions")}
}

// Read returns the suggestions; a missing file is an empty list.
func (s *NameSuggestionStore) Read() (models.NameSuggestions, error) { return s.file.read() }

// Write replaces the suggestions atomically.
func (s *NameSuggestionStore) Write(n models.NameSuggestions) error { return s.file.write(n) }

// Delete removes the file (a factory reset clears it).
func (s *NameSuggestionStore) Delete() error { return s.file.remove() }
