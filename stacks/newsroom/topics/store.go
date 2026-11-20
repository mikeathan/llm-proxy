package topics

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy-stacks/newsroom/sources"
)

// FileName is the one file the Topics module keeps in the stack's data folder (the manifest's Storage entry).
const FileName = "topics.json"

const fileVersion = 1

// SeenEntry is one reported item in a topic's ledger.
type SeenEntry struct {
	Title string    `json:"title,omitempty"`
	At    time.Time `json:"at"`
}

type document struct {
	Version int   `json:"version"`
	NextID  int64 `json:"next_id"`
	// Entries is the feed's journal (journal.go); NextEntry numbers the next one.
	Entries   []Entry                        `json:"entries,omitempty"`
	NextEntry int64                          `json:"next_entry,omitempty"`
	Topics    []Topic                        `json:"topics"`
	Seen      map[int64]map[string]SeenEntry `json:"seen,omitempty"`
}

// Store keeps topics and each topic's seen ledger in one file, written atomically through the stack's DataDir. Every method is safe for concurrent
// use; each change is saved before it returns, so a crash loses nothing the caller was told succeeded.
type Store struct {
	mu  sync.Mutex
	dir stacksdk.DataDir
	doc document
}

// NewStore reads the file, or starts empty when there is none. A file that cannot be understood is set aside (DataDir.Quarantine) and the store
// starts empty, so one bad write never stops the module for good.
func NewStore(dir stacksdk.DataDir) (*Store, error) {
	s := &Store{dir: dir, doc: document{Version: fileVersion, NextID: 1, NextEntry: 1, Seen: map[int64]map[string]SeenEntry{}}}
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
	if doc.Seen == nil {
		doc.Seen = map[int64]map[string]SeenEntry{}
	}
	if doc.NextID < 1 {
		doc.NextID = 1
	}
	if doc.NextEntry < 1 {
		doc.NextEntry = 1
	}
	s.doc = doc
	return s, nil
}

// Erase forgets every topic, ledger entry and feed entry and saves the empty document, for a factory reset: the file alone cannot be removed from
// under a store that holds the document in memory, because the next change would write it back.
func (s *Store) Erase() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.doc = document{Version: fileVersion, NextID: 1, NextEntry: 1, Seen: map[int64]map[string]SeenEntry{}}
	return s.saveLocked()
}

// List returns the topics in id order.
func (s *Store) List() []Topic {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Topic(nil), s.doc.Topics...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns one topic.
func (s *Store) Get(id int64) (Topic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.index(id); i >= 0 {
		return s.doc.Topics[i], nil
	}
	return Topic{}, ErrNotFound
}

// Create validates and stores a new topic and returns it with its id.
func (s *Store) Create(t Topic, now time.Time) (Topic, error) {
	t, err := Normalize(t)
	if err != nil {
		return Topic{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.doc.Topics) >= MaxTopics {
		return Topic{}, fmt.Errorf("%w: at most %d topics", ErrInvalid, MaxTopics)
	}
	t.ID, t.CreatedAt = s.doc.NextID, now.UTC()
	t.Baselined, t.LastRun, t.LastStatus, t.LastDetail, t.LastNew, t.State = false, time.Time{}, "", "", 0, ""
	s.doc.NextID++
	s.doc.Topics = append(s.doc.Topics, t)
	return t, s.saveLocked()
}

// Update replaces what the owner may edit (name, sources, interval, enabled, search, paper) and keeps the run record and the ledger. A change of
// sources keeps the ledger: items already reported stay reported.
func (s *Store) Update(id int64, edit Topic) (Topic, error) {
	edit, err := Normalize(edit)
	if err != nil {
		return Topic{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 {
		return Topic{}, ErrNotFound
	}
	cur := s.doc.Topics[i]
	edit.ID, edit.CreatedAt = cur.ID, cur.CreatedAt
	edit.Baselined, edit.LastRun, edit.LastStatus, edit.LastDetail, edit.LastNew, edit.State = cur.Baselined, cur.LastRun, cur.LastStatus, cur.LastDetail, cur.LastNew, cur.State
	s.doc.Topics[i] = edit
	return edit, s.saveLocked()
}

// Delete removes a topic and its ledger.
func (s *Store) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 {
		return ErrNotFound
	}
	s.doc.Topics = append(s.doc.Topics[:i], s.doc.Topics[i+1:]...)
	delete(s.doc.Seen, id)
	kept := s.doc.Entries[:0]
	for _, e := range s.doc.Entries {
		if e.TopicID != id {
			kept = append(kept, e)
		}
	}
	s.doc.Entries = kept
	return s.saveLocked()
}

// Unseen returns the items whose identity is not in the topic's ledger (inside the retention window), once each, in the order given. Items already
// seen are dropped in code: the model never sees them.
func (s *Store) Unseen(id int64, items []sources.Item, now time.Time) []sources.Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	ledger, cutoff := s.doc.Seen[id], now.Add(-SeenRetention)
	in := map[string]bool{}
	var out []sources.Item
	for _, it := range items {
		key := it.Identity()
		if e, ok := ledger[key]; (ok && e.At.After(cutoff)) || in[key] {
			continue
		}
		in[key] = true
		out = append(out, it)
	}
	return out
}

// Record adds items to the topic's ledger at now, drops entries past the retention window and keeps the newest MaxSeen. Call it only after the items
// were delivered (or deliberately baselined): an item recorded but never sent is an item the owner never hears about.
func (s *Store) Record(id int64, items []sources.Item, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index(id) < 0 {
		return ErrNotFound
	}
	ledger := s.doc.Seen[id]
	if ledger == nil {
		ledger = map[string]SeenEntry{}
	}
	cutoff := now.Add(-SeenRetention)
	for k, e := range ledger {
		if !e.At.After(cutoff) {
			delete(ledger, k)
		}
	}
	for _, it := range items {
		ledger[it.Identity()] = SeenEntry{Title: sources.Line(it.Title, 100), At: now.UTC()}
	}
	s.doc.Seen[id] = capLedger(ledger)
	return s.saveLocked()
}

// SetRun writes a run's outcome onto the topic.
func (s *Store) SetRun(id int64, at time.Time, status Status, detail string, newItems int, baselined bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 {
		return ErrNotFound
	}
	t := &s.doc.Topics[i]
	t.LastRun, t.LastStatus, t.LastDetail, t.LastNew = at.UTC(), status, sources.Line(detail, 300), newItems
	t.Baselined = t.Baselined || baselined
	return s.saveLocked()
}

// Recent returns the titles of the n most recently reported items, newest first. Entries with no title are skipped. The model step shows them to the
// model, numbered, so a repeat can cite what it repeats.
func (s *Store) Recent(id int64, n int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	type kv struct {
		title string
		at    time.Time
	}
	var all []kv
	for _, e := range s.doc.Seen[id] {
		if e.Title != "" {
			all = append(all, kv{e.Title, e.At})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].at.After(all[j].at) || (all[i].at.Equal(all[j].at) && all[i].title < all[j].title)
	})
	if len(all) > n {
		all = all[:n]
	}
	out := make([]string, len(all))
	for i, e := range all {
		out[i] = e.title
	}
	return out
}

// SetState writes a topic's last-known-state note. It is cleaned and bounded here, whoever calls: the note is model output that is fed back into
// every later prompt, so it is never stored raw.
func (s *Store) SetState(id int64, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.index(id)
	if i < 0 {
		return ErrNotFound
	}
	s.doc.Topics[i].State = sources.Plain(state, MaxStateRunes)
	return s.saveLocked()
}

// SeenCount is the size of a topic's ledger.
func (s *Store) SeenCount(id int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.doc.Seen[id])
}

func (s *Store) index(id int64) int {
	for i := range s.doc.Topics {
		if s.doc.Topics[i].ID == id {
			return i
		}
	}
	return -1
}

func (s *Store) saveLocked() error {
	raw, err := json.Marshal(s.doc)
	if err != nil {
		return err
	}
	return s.dir.Write(FileName, raw)
}

// capLedger keeps the newest MaxSeen entries.
func capLedger(l map[string]SeenEntry) map[string]SeenEntry {
	if len(l) <= MaxSeen {
		return l
	}
	type kv struct {
		k string
		e SeenEntry
	}
	all := make([]kv, 0, len(l))
	for k, e := range l {
		all = append(all, kv{k, e})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].e.At.After(all[j].e.At) })
	out := make(map[string]SeenEntry, MaxSeen)
	for _, p := range all[:MaxSeen] {
		out[p.k] = p.e
	}
	return out
}
