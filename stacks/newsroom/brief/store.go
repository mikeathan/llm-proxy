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

// FileName is the Brief module's settings and written editions, and PoolFileName the stories collected for them, in the stack's data folder (manifest
// Storage entries). The pool is the large part and changes with every collection, so it has its own file: reading a brief or changing a topic rewrites
// only the small one.
const (
	FileName     = "briefs.json"
	PoolFileName = "briefs-pool.json"
)

const fileVersion = 1

type document struct {
	Version   int       `json:"version"`
	NextID    int64     `json:"next_id"`
	NextPool  int64     `json:"next_pool"`
	NextIssue int64     `json:"next_issue"`
	Topics    []Topic   `json:"topics"`
	Editions  []Edition `json:"editions"`
	// Issues are the editions made, kept for IssueRetention; Shown records which stories they carried, for the already-shown rule.
	Issues []Issue `json:"issues,omitempty"`
	Shown  []Shown `json:"shown,omitempty"`
	// MySources is the owner's own list of sources the topic form offers beside the built-in one.
	MySources []MySource `json:"my_sources,omitempty"`
}

// poolDocument is the pool's own file.
type poolDocument struct {
	Version int        `json:"version"`
	Pool    []PoolItem `json:"pool"`
}

// Store keeps topics and editions in one file and the pool in another, written atomically through the stack's DataDir. Every method is safe for concurrent use; each
// change is saved before it returns, so a crash loses nothing the caller was told succeeded. Topics and editions share one id counter.
type Store struct {
	mu   sync.Mutex
	dir  stacksdk.DataDir
	doc  document
	pool []PoolItem
}

func emptyDoc() document { return document{Version: fileVersion, NextID: 1, NextPool: 1, NextIssue: 1} }

// NewStore reads both files, or starts empty when there are none. A file that cannot be understood is set aside (DataDir.Quarantine) and that part starts
// empty, so one bad write never stops the module for good. A briefs.json written before the pool had its own file carries its pool inline; that copy is
// not read (the pool is a cache of headlines) and goes with the next save, and every topic is marked never read so the next pass refills it.
func NewStore(dir stacksdk.DataDir) (*Store, error) {
	s := &Store{dir: dir, doc: emptyDoc()}
	var doc document
	found, err := readFile(dir, FileName, &doc, func() bool { return doc.Version == fileVersion })
	if err != nil {
		return nil, err
	}
	if found {
		doc.NextID, doc.NextPool, doc.NextIssue = max(doc.NextID, 1), max(doc.NextPool, 1), max(doc.NextIssue, 1)
		s.doc = doc
	}
	var pool poolDocument
	if found, err = readFile(dir, PoolFileName, &pool, func() bool { return pool.Version == fileVersion }); err != nil {
		return nil, err
	}
	if found {
		s.pool = pool.Pool
	} else {
		// No pool to go with the topics (a briefs.json from before the pool had its own file, or a pool file set aside): every topic is due, so the
		// next pass reads them all instead of an edition being written from nothing until each topic's interval comes round.
		for i := range s.doc.Topics {
			s.doc.Topics[i].LastCollected = time.Time{}
		}
	}
	return s, nil
}

// readFile reads one file into v and says whether it was there and understood (ok checks what was read); a file that is not is set aside.
func readFile(dir stacksdk.DataDir, name string, v any, ok func() bool) (bool, error) {
	raw, err := dir.Read(name)
	if err != nil {
		var se *stacksdk.Error
		if errors.As(err, &se) && se.Code == stacksdk.CodeNotFound {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", name, err)
	}
	if json.Unmarshal(raw, v) != nil || !ok() {
		if qerr := dir.Quarantine(name); qerr != nil {
			return false, fmt.Errorf("set aside %s: %w", name, qerr)
		}
		return false, nil
	}
	return true, nil
}

// Erase forgets every topic, edition and story and saves the empty document, for a factory reset.
func (s *Store) Erase() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.doc, s.pool = emptyDoc(), nil
	return errors.Join(s.saveLocked(), s.savePoolLocked())
}

// saveLocked writes the topics, editions and written editions; savePoolLocked writes the pool.
func (s *Store) saveLocked() error {
	raw, err := json.Marshal(s.doc)
	if err != nil {
		return err
	}
	return s.dir.Write(FileName, raw)
}

func (s *Store) savePoolLocked() error {
	raw, err := json.Marshal(poolDocument{Version: fileVersion, Pool: s.pool})
	if err != nil {
		return err
	}
	return s.dir.Write(PoolFileName, raw)
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
	kept := s.pool[:0:0]
	for _, p := range s.pool {
		p.Topics = slices.DeleteFunc(slices.Clone(p.Topics), func(t int64) bool { return t == id })
		if len(p.Topics) > 0 {
			kept = append(kept, p)
		}
	}
	s.pool = kept
	return errors.Join(s.saveLocked(), s.savePoolLocked())
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
	pool, next, added := merge(s.pool, s.doc.NextPool, s.doc.Topics[i], items, now.UTC())
	s.pool, s.doc.NextPool = pool, next
	t := &s.doc.Topics[i]
	t.LastCollected, t.LastNew, t.LastProblems = now.UTC(), added, slices.Clone(problems)
	// The pool first: a crash between the two leaves the topic due again, never marked read with its stories lost.
	if err := s.savePoolLocked(); err != nil {
		return 0, err
	}
	return added, s.saveLocked()
}

// Pool returns the collected stories, newest first (by first seen, then id, so the order never depends on the file).
func (s *Store) Pool() []PoolItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := slices.Clone(s.pool)
	slices.SortStableFunc(out, func(a, b PoolItem) int {
		if c := b.FirstSeen.Compare(a.FirstSeen); c != 0 {
			return c
		}
		return int(b.ID - a.ID)
	})
	return out
}
