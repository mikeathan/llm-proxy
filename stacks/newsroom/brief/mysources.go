package brief

import (
	"fmt"
	"slices"
	"strings"

	"llm-proxy-stacks/newsroom/sources"
)

// The owner's own list of sources, kept beside the built-in one (sources.Catalog) so the topic form offers both. A topic copies what it picks, so
// removing an entry here never changes a topic.
const (
	// MaxMySources bounds the list.
	MaxMySources = 100
	// MySourcesCategory is the group the form shows the owner's entries under.
	MySourcesCategory = "My sources"
)

// MySource is one source the owner added to their list.
type MySource struct {
	ID     int64        `json:"id"`
	Label  string       `json:"label"`
	Kind   sources.Kind `json:"kind"`
	Target string       `json:"target"`
}

func sameSource(kind sources.Kind, target string, other sources.Spec) bool {
	return kind == other.Kind && strings.EqualFold(target, other.Target)
}

// MySources returns the owner's list in the order it was made.
func (s *Store) MySources() []MySource {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.doc.MySources)
}

// AddMySource checks and stores one source for the owner's list. The kind must be one a topic reads from a list (a feed or a news search), the target
// passes the same check as in a topic, and a source already in either list is refused.
func (s *Store) AddMySource(label string, kind sources.Kind, target string) (MySource, error) {
	label = strings.TrimSpace(label)
	if label == "" || len([]rune(label)) > MaxNameRunes {
		return MySource{}, fmt.Errorf("%w: a source needs a name of 1 to %d characters", ErrInvalid, MaxNameRunes)
	}
	spec := sources.Spec{Kind: kind, Label: label, Target: target}.Canonical()
	if kind != sources.KindFeed && kind != sources.KindNews {
		return MySource{}, fmt.Errorf("%w: a source in the list is a feed or a news search", ErrInvalid)
	}
	if err := spec.Validate(); err != nil {
		return MySource{}, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	for _, e := range sources.Catalog() {
		if sameSource(spec.Kind, spec.Target, e.Spec()) {
			return MySource{}, fmt.Errorf("%w: %s is already in the built-in list", ErrInvalid, e.Label)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.doc.MySources) >= MaxMySources {
		return MySource{}, fmt.Errorf("%w: at most %d sources of your own", ErrInvalid, MaxMySources)
	}
	for _, m := range s.doc.MySources {
		if sameSource(spec.Kind, spec.Target, sources.Spec{Kind: m.Kind, Target: m.Target}) {
			return MySource{}, fmt.Errorf("%w: %s is already in your list", ErrInvalid, m.Label)
		}
	}
	m := MySource{ID: s.doc.NextID, Label: label, Kind: spec.Kind, Target: spec.Target}
	s.doc.NextID++
	s.doc.MySources = append(s.doc.MySources, m)
	return m, s.saveLocked()
}

// DeleteMySource removes one source from the owner's list. Topics that read it keep their own copy.
func (s *Store) DeleteMySource(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.doc.MySources, func(m MySource) bool { return m.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	s.doc.MySources = slices.Delete(s.doc.MySources, i, i+1)
	return s.saveLocked()
}
