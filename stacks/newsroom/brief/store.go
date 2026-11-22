package brief

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"llm-proxy-stacks/newsroom/sources"
	"llm-proxy-stacks/sdk"
)

// FileName is the one file the Brief module keeps in the stack's data folder (a manifest Storage entry).
const FileName = "briefs.json"

const fileVersion = 1

type document struct {
	Version   int        `json:"version"`
	NextID    int64      `json:"next_id"`
	NextPool  int64      `json:"next_pool"`
	NextIssue int64      `json:"next_issue"`
	Topics    []Topic    `json:"topics"`
	Editions  []Edition  `json:"editions"`
	Pool      []PoolItem `json:"pool,omitempty"`
	// Issues are the editions made, kept for IssueRetention; Shown records which stories they carried, for the already-shown rule.
	Issues []Issue `json:"issues,omitempty"`
	Shown  []Shown `json:"shown,omitempty"`
}

// Store keeps topics, editions and the pool in one file, written atomically through the stack's DataDir. Every method is safe for concurrent use; each
// change is saved before it returns, so a crash loses nothing the caller was told succeeded. Topics and editions share one id counter.
type Store struct {
	mu  sync.Mutex
	dir stacksdk.DataDir
	doc document
}

func emptyDoc() document { return document{Version: fileVersion, NextID: 1, NextPool: 1, NextIssue: 1} }

// NewStore reads the file, or starts empty when there is none. A file that cannot be understood is set aside (DataDir.Quarantine) and the store starts
// empty, so one bad write never stops the module for good.
func NewStore(dir stacksdk.DataDir) (*Store, error) {
	s := &Store{dir: dir, doc: emptyDoc()}
	raw, err := dir.Read(FileName)
	if err != nil {
		var se *stacksdk.Error
		if errors.As(err, &se) && se.Code == stacksdk.CodeNotFound {
			return s, nil
		}
		return nil, fmt.Errorf("read %s: %w", FileName, err)
	}
	var doc document
	if json.Unmarshal(raw, &doc) != nil || doc.Version != fileVersion {
		if qerr := dir.Quarantine(FileName); qerr != nil {
			return nil, fmt.Errorf("set aside %s: %w", FileName, qerr)
		}
		return s, nil
	}
	doc.NextID, doc.NextPool, doc.NextIssue = max(doc.NextID, 1), max(doc.NextPool, 1), max(doc.NextIssue, 1)
	s.doc = doc
	return s, nil
}

// Erase forgets every topic, edition and story and saves the empty document, for a factory reset.
func (s *Store) Erase() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.doc = emptyDoc()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	raw, err := json.Marshal(s.doc)
	if err != nil {
		return err
	}
	return s.dir.Write(FileName, raw)
}

// Topics returns the topics in id order.
func (s *Store) Topics() []Topic {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.doc.Topics)
}

// Topic returns one topic.
func (s *Store) Topic(id int64) (Topic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.topicIndex(id); i >= 0 {
		return s.doc.Topics[i], nil
	}
	return Topic{}, ErrNotFound
}

func (s *Store) topicIndex(id int64) int {
	return slices.IndexFunc(s.doc.Topics, func(t Topic) bool { return t.ID == id })
}

func (s *Store) editionIndex(id int64) int {
	return slices.IndexFunc(s.doc.Editions, func(e Edition) bool { return e.ID == id })
}

func (s *Store) hasTopic(id int64) bool { return s.topicIndex(id) >= 0 }

// CreateTopic validates and stores a new topic and returns it with its id.
func (s *Store) CreateTopic(t Topic, now time.Time) (Topic, error) {
	t, err := t.Normalize()
	if err != nil {
		return Topic{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.doc.Topics) >= MaxTopics {
		return Topic{}, fmt.Errorf("%w: at most %d topics", ErrInvalid, MaxTopics)
	}
	t.ID, t.CreatedAt = s.doc.NextID, now.UTC()
	t.LastCollected, t.LastNew, t.LastProblems = time.Time{}, 0, nil
	s.doc.NextID++
	s.doc.Topics = append(s.doc.Topics, t)
	return t, s.saveLocked()
}

// UpdateTopic replaces what the owner may edit; the collection record and creation time stay.
func (s *Store) UpdateTopic(id int64, t Topic) (Topic, error) {
	t, err := t.Normalize()
	if err != nil {
		return Topic{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.topicIndex(id)
	if i < 0 {
		return Topic{}, ErrNotFound
	}
	old := s.doc.Topics[i]
	t.ID, t.CreatedAt, t.LastCollected, t.LastNew, t.LastProblems = id, old.CreatedAt, old.LastCollected, old.LastNew, old.LastProblems
	s.doc.Topics[i] = t
	return t, s.saveLocked()
}

// DeleteTopic removes a topic and the pool's link to it (a story no topic wants any more goes). An edition that still lists it refuses: ErrInUse.
func (s *Store) DeleteTopic(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.topicIndex(id)
	if i < 0 {
		return ErrNotFound
	}
	for _, e := range s.doc.Editions {
		if slices.ContainsFunc(e.Sections, func(sec Section) bool { return sec.TopicID == id }) {
			return fmt.Errorf("%w: the edition %q still lists it", ErrInUse, e.Name)
		}
	}
	s.doc.Topics = slices.Delete(s.doc.Topics, i, i+1)
	kept := s.doc.Pool[:0:0]
	for _, p := range s.doc.Pool {
		p.Topics = slices.DeleteFunc(slices.Clone(p.Topics), func(t int64) bool { return t == id })
		if len(p.Topics) > 0 {
			kept = append(kept, p)
		}
	}
	s.doc.Pool = kept
	return s.saveLocked()
}

// Editions returns the editions in id order.
func (s *Store) Editions() []Edition {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.doc.Editions)
}

// Edition returns one edition.
func (s *Store) Edition(id int64) (Edition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.editionIndex(id); i >= 0 {
		return s.doc.Editions[i], nil
	}
	return Edition{}, ErrNotFound
}

// CreateEdition validates and stores a new edition and returns it with its id.
func (s *Store) CreateEdition(e Edition, now time.Time) (Edition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := e.Normalize(s.hasTopic)
	if err != nil {
		return Edition{}, err
	}
	if len(s.doc.Editions) >= MaxEditions {
		return Edition{}, fmt.Errorf("%w: at most %d editions", ErrInvalid, MaxEditions)
	}
	e.ID, e.CreatedAt = s.doc.NextID, now.UTC()
	s.doc.NextID++
	s.doc.Editions = append(s.doc.Editions, e)
	return e, s.saveLocked()
}

// UpdateEdition replaces what the owner may edit.
func (s *Store) UpdateEdition(id int64, e Edition) (Edition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.editionIndex(id)
	if i < 0 {
		return Edition{}, ErrNotFound
	}
	e, err := e.Normalize(s.hasTopic)
	if err != nil {
		return Edition{}, err
	}
	e.ID, e.CreatedAt = id, s.doc.Editions[i].CreatedAt
	s.doc.Editions[i] = e
	return e, s.saveLocked()
}

// DeleteEdition removes an edition.
func (s *Store) DeleteEdition(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.editionIndex(id)
	if i < 0 {
		return ErrNotFound
	}
	s.doc.Editions = slices.Delete(s.doc.Editions, i, i+1)
	return s.saveLocked()
}

// RecordCollection puts what one collection of a topic found into the pool and writes the topic's collection record: when, how many stories were new,
// and which sources could not be read. It returns how many stories were new.
func (s *Store) RecordCollection(topicID int64, items []sources.Item, problems []string, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.topicIndex(topicID)
	if i < 0 {
		return 0, ErrNotFound
	}
	pool, next, added := merge(s.doc.Pool, s.doc.NextPool, s.doc.Topics[i], items, now.UTC())
	s.doc.Pool, s.doc.NextPool = pool, next
	t := &s.doc.Topics[i]
	t.LastCollected, t.LastNew, t.LastProblems = now.UTC(), added, slices.Clone(problems)
	return added, s.saveLocked()
}

// Pool returns the collected stories, newest first (by first seen, then id, so the order never depends on the file).
func (s *Store) Pool() []PoolItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := slices.Clone(s.doc.Pool)
	slices.SortStableFunc(out, func(a, b PoolItem) int {
		if c := b.FirstSeen.Compare(a.FirstSeen); c != 0 {
			return c
		}
		return int(b.ID - a.ID)
	})
	return out
}
