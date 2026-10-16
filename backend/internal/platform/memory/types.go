// Package memory provides a SQLite-backed persistent memory store for
// agent facts, decisions, and preferences.  It uses FTS5 for full-text
// search and shares the same database file as the ledger package.
package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type MemoryType string

const (
	LongTerm    MemoryType = "long_term"
	Daily       MemoryType = "daily"
	Session     MemoryType = "session"
	UserProfile MemoryType = "user_profile"
)

// ── Three-tier value objects ──────────────────────────────────────────────

type Scope string

const (
	ScopeUser      Scope = "user"
	ScopeWorkspace Scope = "workspace"
)

func (s Scope) Validate() error {
	if s != ScopeUser && s != ScopeWorkspace {
		return fmt.Errorf("invalid scope: %s", s)
	}
	return nil
}

type Mode string

const (
	ModeAlways   Mode = "always"
	ModeOnDemand Mode = "on_demand"
)

func (m Mode) Validate() error {
	if m != ModeAlways && m != ModeOnDemand {
		return fmt.Errorf("invalid mode: %s", m)
	}
	return nil
}

type Keep string

const (
	KeepPermanent Keep = "permanent"
	KeepSession   Keep = "session"
)

func (k Keep) Validate() error {
	if k != KeepPermanent && k != KeepSession {
		return fmt.Errorf("invalid keep: %s", k)
	}
	return nil
}

// Kind says what a fact is about, so a reader can weigh it: a fact about the user, feedback on how to work, a project
// fact, or a pointer to where something lives. Empty means unspecified (every fact saved before kinds existed).
type Kind string

const (
	KindUser      Kind = "user"
	KindFeedback  Kind = "feedback"
	KindProject   Kind = "project"
	KindReference Kind = "reference"
)

// Kinds lists the valid kinds, in the order the UI offers them.
var Kinds = []Kind{KindUser, KindFeedback, KindProject, KindReference}

// MaxWhyChars bounds the one-line reason a fact may carry.
const MaxWhyChars = 200

var (
	ErrInvalidKind = fmt.Errorf("invalid kind: must be one of user, feedback, project, reference")
	ErrInvalidWhy  = fmt.Errorf("invalid why: must be one line of at most %d characters", MaxWhyChars)
)

// Validate accepts the empty kind (unspecified) and the four named kinds.
func (k Kind) Validate() error {
	if k == "" || slices.Contains(Kinds, k) {
		return nil
	}
	return ErrInvalidKind
}

// ValidateTyping checks a fact's kind and reason: a known (or empty) kind, and a reason that is a single line within
// MaxWhyChars. Writers call it so a bad value is refused at the edge instead of stored.
func ValidateTyping(kind Kind, why string) error {
	if err := kind.Validate(); err != nil {
		return err
	}
	if strings.ContainsAny(why, "\r\n") || utf8.RuneCountInString(why) > MaxWhyChars {
		return ErrInvalidWhy
	}
	return nil
}

// ExpiryDayLayout is how a day is written wherever an expiry is typed or shown.
const ExpiryDayLayout = "2006-01-02"

// ExpiryAfterDay is the instant a fact whose last useful day is the given YYYY-MM-DD (UTC) expires: the start of the
// next day.
func ExpiryAfterDay(day string) (time.Time, error) {
	d, err := time.Parse(ExpiryDayLayout, day)
	if err != nil {
		return time.Time{}, fmt.Errorf("expires must be a date written YYYY-MM-DD")
	}
	return d.AddDate(0, 0, 1), nil
}

// LastDay is the last calendar day the fact is still valid ("" and false when it never expires). An expiry stored as the
// start of a day (a dated fact) reads as the day before it, while a mid-day instant (a session fact) reads as its own day.
func (e MemoryEntry) LastDay() (string, bool) {
	t, err := time.Parse(time.RFC3339, e.ExpiresAt)
	if err != nil {
		return "", false
	}
	return t.Add(-time.Second).UTC().Format(ExpiryDayLayout), true
}

// NewEntry is a fact to insert. A zero ExpiresAt means the fact never expires.
type NewEntry struct {
	WorkspaceID string
	MemoryType  MemoryType
	Title       string
	Content     string
	Tags        []string
	Source      string
	Kind        Kind
	Why         string
	ExpiresAt   time.Time
}

// Edit is the new text, tags and typing of an existing fact. It replaces them all: a caller that means to keep the kind,
// reason or expiry passes the existing values. A zero ExpiresAt clears the expiry.
type Edit struct {
	Title     string
	Content   string
	Tags      []string
	TagMode   UpdateTags
	Kind      Kind
	Why       string
	ExpiresAt time.Time
}

// ── Search options ────────────────────────────────────────────────────────

// SearchOption carries optional filters for Store.Search(). Tags filters by
// exact tag match (all must be present). MemType replaces the old variadic.
// WorkspaceID overrides the workspace filter when set (e.g., for global scope).
type SearchOption struct {
	Tags                []string
	MemType             MemoryType
	WorkspaceID         string
	SearchAllWorkspaces bool
}

// MemoryEntry stores datetime as string to match go-sqlite3 scan behavior.
// The existing ledger code (store.go:233-242) follows the same pattern:
// scan DATETIME columns as string, parse with time.Parse when needed.
type MemoryEntry struct {
	ID          int64      `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	MemoryType  MemoryType `json:"memory_type"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	Tags        []string   `json:"tags"`
	Source      string     `json:"source"`
	// Kind and Why type the fact (see Kind); ExpiresAt, when set, is when the reaper removes it ("" = never).
	Kind      Kind   `json:"kind"`
	Why       string `json:"why"`
	ExpiresAt string `json:"expires_at"`
	// Priority orders hot facts when the budget cuts the tail: higher survives
	// longer. Default PriorityNormal; only the operator changes it.
	Priority int `json:"priority"`
	// Usage counters (usage.go): runs the fact was sent in, times memory_search
	// returned it, and when it was last used ("" = never).
	InjectedCount int    `json:"injected_count"`
	SearchedCount int    `json:"searched_count"`
	LastUsedAt    string `json:"last_used_at"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// scanMemoryEntry is a shared row scanner for all Store methods to avoid
// duplicating the 16-column Scan call at every query site.
func scanMemoryEntry(row interface{ Scan(dest ...any) error }) (MemoryEntry, error) {
	var e MemoryEntry
	var tagsStr string
	var lastUsed, expires sql.NullString
	if err := row.Scan(&e.ID, &e.WorkspaceID, &e.MemoryType, &e.Title,
		&e.Content, &tagsStr, &e.Source, &e.Priority, &e.InjectedCount, &e.SearchedCount, &lastUsed,
		&e.CreatedAt, &e.UpdatedAt, &e.Kind, &e.Why, &expires); err != nil {
		return MemoryEntry{}, err
	}
	e.LastUsedAt = lastUsed.String
	e.ExpiresAt = expires.String
	if tagsStr != "" && tagsStr != "[]" {
		json.Unmarshal([]byte(tagsStr), &e.Tags)
	}
	return e, nil
}

func (e *MemoryEntry) CreatedAtTime() (time.Time, error) {
	return parseMemoryTime(e.CreatedAt)
}

func (e *MemoryEntry) UpdatedAtTime() (time.Time, error) {
	return parseMemoryTime(e.UpdatedAt)
}

// parseMemoryTime handles both SQLite datetime('now') format
// (2006-01-02 15:04:05) and ISO 8601 (2006-01-02T15:04:05Z).
func parseMemoryTime(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err == nil {
		return t, nil
	}
	t, err = time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q: SQLite and RFC3339 both failed", s)
}

// Priority levels for hot facts (0–2). High facts are injected first, so they are
// the last to be cut when a small window leaves little room; Low ones go first.
const (
	PriorityLow    = 0
	PriorityNormal = 1
	PriorityHigh   = 2
)

// ValidPriority reports whether p is a defined priority level.
func ValidPriority(p int) bool {
	return p >= PriorityLow && p <= PriorityHigh
}
