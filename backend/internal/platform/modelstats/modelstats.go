// Package modelstats counts the calls this service makes to language models, so the owner can see how much each local and each cloud model was used.
// It keeps numbers only: per day and per model, how many calls, how many failed, how many tokens went in and out and how fast a local server answered. It never
// sees a prompt, a reply, a key or an address, and it sends nothing anywhere: the one file lives in the data folder and is served only to the signed-in admin
// (ADR-0085).
package modelstats

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// RetainDays is how many days are kept; an older day is dropped when the next call is recorded.
	RetainDays = 14
	// MaxModelsPerDay bounds one day's table: a model beyond it is counted under OtherModels, so a flood of made-up names cannot grow the file.
	MaxModelsPerDay = 64
	// OtherModels is the row that holds the calls of models beyond MaxModelsPerDay.
	OtherModels = "other models"
	// UnknownModel names a call that came with no model name.
	UnknownModel = "unknown"
	// maxModelName clips a model name to a length a table cell can hold.
	maxModelName = 120
	// saveEvery is the shortest time between two writes of the file while calls keep coming; Flush writes at once.
	saveEvery = time.Minute
	fileMode  = 0o600
	dayLayout = "2006-01-02"
	// FileName is the file in the data folder.
	FileName    = "model-stats.json"
	fileVersion = 1
)

// Call is one finished call to a model. Tokens are what the provider reported (zero when it said nothing); TokensPerSec is the speed a local server
// reported for its reply (zero when it did not, which is every cloud call).
type Call struct {
	Model            string
	Local            bool
	PromptTokens     int
	CompletionTokens int
	TokensPerSec     float64
	Failed           bool
}

// entry is one model's totals for one day. The speed is kept as a sum and a count so a day's average can be taken without keeping every call.
type entry struct {
	Model            string  `json:"model"`
	Local            bool    `json:"local"`
	Calls            int     `json:"calls"`
	Failed           int     `json:"failed"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	SpeedSum         float64 `json:"speed_sum,omitempty"`
	SpeedCount       int     `json:"speed_count,omitempty"`
}

// document is the file: a version and each day's rows.
type document struct {
	Version int                `json:"version"`
	Days    map[string][]entry `json:"days"`
}

// DayTotal is one day's call counts, local and cloud apart.
type DayTotal struct {
	Date  string `json:"date"`
	Local int    `json:"local"`
	Cloud int    `json:"cloud"`
}

// ModelUse is one model's use today.
type ModelUse struct {
	Model            string  `json:"model"`
	Local            bool    `json:"local"`
	Calls            int     `json:"calls"`
	Failed           int     `json:"failed"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TokensPerSec     float64 `json:"tokens_per_sec,omitempty"`
}

// Report is what the screen reads: the last days oldest first (a day with no call is a row of zeros) and today's models, busiest first.
type Report struct {
	Days  []DayTotal `json:"days"`
	Today []ModelUse `json:"today"`
}

// Store holds the counts in memory and the file on disk. It starts no goroutine: a write happens inside Record at most once every saveEvery, and Flush writes the
// rest when the service stops.
type Store struct {
	mu       sync.Mutex
	path     string
	now      func() time.Time
	days     map[string]map[string]*entry
	dirty    bool
	lastSave time.Time
	// gen numbers each snapshot taken for writing and written is the newest one on disk, so a snapshot that was taken earlier but reaches the disk later (two
	// writers racing, one slow) is skipped instead of replacing newer counts. writeMu serialises the writes themselves.
	gen     int
	written int
	writeMu sync.Mutex
}

// Open reads the file at path, or starts empty when there is none. A file that cannot be read or understood is not an error for the service: Open starts
// empty and returns the reason, for the caller to log, and the next write replaces the file.
func Open(path string, now func() time.Time) (*Store, error) {
	s := &Store{path: path, now: now, days: map[string]map[string]*entry{}, lastSave: now()}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("read model stats: %w", err)
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Version != fileVersion {
		return s, fmt.Errorf("model stats file is not readable, starting empty: %v", err)
	}
	for date, rows := range doc.Days {
		if _, err := time.Parse(dayLayout, date); err != nil {
			continue
		}
		table := map[string]*entry{}
		for i := range rows {
			if len(table) < MaxModelsPerDay+1 {
				table[rows[i].Model] = &rows[i]
			}
		}
		s.days[date] = table
	}
	return s, nil
}

// Record counts one finished call. A write of the file may follow, at most once in saveEvery.
func (s *Store) Record(c Call) {
	s.mu.Lock()
	now := s.now()
	table := s.table(now.Format(dayLayout))
	name := cleanModel(c.Model)
	e, ok := table[name]
	if !ok && len(table) >= MaxModelsPerDay {
		name = OtherModels
		e = table[name]
	}
	if e == nil {
		e = &entry{Model: name, Local: c.Local}
		table[name] = e
	}
	e.Calls++
	if c.Failed {
		e.Failed++
	}
	e.PromptTokens += max(0, c.PromptTokens)
	e.CompletionTokens += max(0, c.CompletionTokens)
	if c.TokensPerSec > 0 {
		e.SpeedSum += c.TokensPerSec
		e.SpeedCount++
	}
	s.dirty = true
	var snapshot []byte
	var gen int
	if now.Sub(s.lastSave) >= saveEvery {
		snapshot, gen = s.encodeLocked(now)
	}
	s.mu.Unlock()
	if snapshot != nil {
		_ = s.write(snapshot, gen)
	}
}

// table is the rows of a day, made on first use; it also drops the days older than RetainDays.
func (s *Store) table(date string) map[string]*entry {
	t, ok := s.days[date]
	if !ok {
		t = map[string]*entry{}
		s.days[date] = t
		s.pruneLocked(date)
	}
	return t
}

func (s *Store) pruneLocked(today string) {
	day, err := time.Parse(dayLayout, today)
	if err != nil {
		return
	}
	oldest := day.AddDate(0, 0, -(RetainDays - 1)).Format(dayLayout)
	for d := range s.days {
		if d < oldest {
			delete(s.days, d)
		}
	}
}

// Snapshot is the report for the last `days` days (at least 1, at most RetainDays) ending today.
func (s *Store) Snapshot(days int) Report {
	days = min(max(days, 1), RetainDays)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	rep := Report{Days: make([]DayTotal, 0, days), Today: []ModelUse{}}
	for i := days - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format(dayLayout)
		total := DayTotal{Date: date}
		for _, e := range s.days[date] {
			if e.Local {
				total.Local += e.Calls
			} else {
				total.Cloud += e.Calls
			}
		}
		rep.Days = append(rep.Days, total)
	}
	for _, e := range s.days[now.Format(dayLayout)] {
		use := ModelUse{Model: e.Model, Local: e.Local, Calls: e.Calls, Failed: e.Failed, PromptTokens: e.PromptTokens, CompletionTokens: e.CompletionTokens}
		if e.SpeedCount > 0 {
			use.TokensPerSec = e.SpeedSum / float64(e.SpeedCount)
		}
		rep.Today = append(rep.Today, use)
	}
	slices.SortFunc(rep.Today, func(a, b ModelUse) int {
		if a.Calls != b.Calls {
			return b.Calls - a.Calls
		}
		return strings.Compare(a.Model, b.Model)
	})
	return rep
}

// Flush writes any counts not yet on disk.
func (s *Store) Flush() error {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	snapshot, gen := s.encodeLocked(s.now())
	s.mu.Unlock()
	return s.write(snapshot, gen)
}

// encodeLocked marks the counts as saved and returns the file's bytes; the caller writes them without the lock, so a slow disk never holds up a call.
func (s *Store) encodeLocked(now time.Time) ([]byte, int) {
	doc := document{Version: fileVersion, Days: map[string][]entry{}}
	for date, table := range s.days {
		rows := make([]entry, 0, len(table))
		for _, e := range table {
			rows = append(rows, *e)
		}
		slices.SortFunc(rows, func(a, b entry) int { return strings.Compare(a.Model, b.Model) })
		doc.Days[date] = rows
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, 0
	}
	s.dirty = false
	s.lastSave = now
	s.gen++
	return raw, s.gen
}

// write replaces the file in one step (a temporary file in the same folder, then a rename), so a stop in the middle never leaves half a file.
func (s *Store) write(raw []byte, gen int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if gen <= s.written {
		return nil
	}
	if err := s.replaceFile(raw); err != nil {
		return err
	}
	s.written = gen
	return nil
}

func (s *Store) replaceFile(raw []byte) error {
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write model stats: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(fileMode); err != nil {
		tmp.Close()
		return fmt.Errorf("write model stats: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write model stats: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write model stats: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("write model stats: %w", err)
	}
	return nil
}

// cleanModel makes a model name one short plain line.
func cleanModel(name string) string {
	name = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if r < ' ' && r != '\t' && r != '\n' {
			return -1
		}
		return r
	}, name)), " ")
	if name == "" {
		return UnknownModel
	}
	if utf8.RuneCountInString(name) > maxModelName {
		name = string([]rune(name)[:maxModelName])
	}
	return name
}
