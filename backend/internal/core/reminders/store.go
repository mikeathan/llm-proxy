// Package reminders stores user reminders and delivers them verbatim at the
// right time. There is no LLM at fire time: the text the user asked for is the
// text that is sent.
package reminders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/platform/db"
)

// Status is where a reminder is in its life.
type Status string

const (
	StatusScheduled Status = "scheduled"
	StatusDone      Status = "done"
	StatusCancelled Status = "cancelled"
	StatusMissed    Status = "missed"
	StatusFailed    Status = "failed"
	// StatusPaused is a repeating reminder that is silenced: it keeps its place in
	// the list and counts against the open limit, but never fires until resumed.
	StatusPaused Status = "paused"
)

// openStatuses are the reminders a user still manages as "upcoming".
var openStatuses = []Status{StatusScheduled, StatusPaused}

// finishedStatuses can be brought back to life by editing them (reschedule).
var finishedStatuses = []Status{StatusDone, StatusMissed, StatusFailed}

// ErrInvalid wraps every refusal of a reminder the user (or agent) described:
// transport maps it to a 400, as opposed to a storage failure.
var ErrInvalid = errors.New("invalid reminder")

// ErrNotFound is returned when a reminder does not exist or is no longer open.
var ErrNotFound = errors.New("reminder not found")

// Reminder is one stored reminder. Connector is a connector type ("telegram");
// empty means the default reminder channel. Cron is empty for a one-off.
type Reminder struct {
	ID        int64
	Message   string
	FireAt    time.Time
	Cron      string
	Until     *time.Time
	Timezone  string
	Connector string
	// Origin is the connector type the request arrived through. It only informs
	// the channel chosen at creation and is never stored.
	Origin string
	Status Status
	// Rev counts edits made by the user. The delivery loop writes a result back
	// only if Rev is still what it read, so a result computed from an old copy
	// can never overwrite an edit made while it was being sent.
	Rev       int64
	Attempts  int
	LastError string
	CreatedBy string
	CreatedAt time.Time
	// Event is what the last delivery outcome was (an Event constant), and EventAt when, saved in the same write as the
	// outcome so a client that was closed at the time can still be told. Empty and zero until the first outcome.
	Event   string
	EventAt time.Time
}

const migrateSQL = `
CREATE TABLE IF NOT EXISTS reminders (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    message    TEXT NOT NULL,
    fire_at    INTEGER NOT NULL,
    cron       TEXT NOT NULL DEFAULT '',
    until_at   INTEGER,
    timezone   TEXT NOT NULL DEFAULT '',
    connector  TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'scheduled',
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_reminders_due ON reminders(status, fire_at);
`

const columns = `id, message, fire_at, cron, until_at, timezone, connector, status, rev, attempts, last_error, created_by, created_at, event, event_at`

// Store persists reminders in the shared SQLite database. Times are stored as
// UTC unix seconds.
type Store struct {
	db *sql.DB
}

// NewStore creates the reminders table if needed.
func NewStore(p db.Provider) (*Store, error) {
	database := p.DB()
	if _, err := database.Exec(migrateSQL); err != nil {
		return nil, fmt.Errorf("reminders migrate: %w", err)
	}
	// Additive and idempotent: SQLite has no ADD COLUMN IF NOT EXISTS.
	if _, err := database.Exec(`ALTER TABLE reminders ADD COLUMN rev INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return nil, fmt.Errorf("reminders migrate rev: %w", err)
	}
	for _, column := range []string{
		`event TEXT NOT NULL DEFAULT ''`,
		`event_at INTEGER NOT NULL DEFAULT 0`, // unix milliseconds: several outcomes can land inside one second
	} {
		if _, err := database.Exec(`ALTER TABLE reminders ADD COLUMN ` + column); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return nil, fmt.Errorf("reminders migrate %s: %w", column, err)
		}
	}
	return &Store{db: database}, nil
}

// Add inserts a scheduled reminder and returns its id.
func (s *Store) Add(ctx context.Context, r Reminder) (int64, error) {
	var until any
	if r.Until != nil {
		until = r.Until.Unix()
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reminders (message, fire_at, cron, until_at, timezone, connector, status, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'scheduled', ?, ?)`,
		r.Message, r.FireAt.Unix(), r.Cron, until, r.Timezone, r.Connector, r.CreatedBy, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("reminders add: %w", err)
	}
	return res.LastInsertId()
}

// Get returns one reminder in any status.
func (s *Store) Get(ctx context.Context, id int64) (Reminder, error) {
	r, err := scan(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM reminders WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Reminder{}, ErrNotFound
	}
	return r, err
}

// ListOpen returns the reminders a user manages as upcoming (scheduled and
// paused), soonest first.
func (s *Store) ListOpen(ctx context.Context) ([]Reminder, error) {
	return s.byStatus(ctx, openStatuses)
}

// ListScheduled returns only the reminders that will fire, soonest first: what
// the delivery timer is armed from.
func (s *Store) ListScheduled(ctx context.Context) ([]Reminder, error) {
	return s.byStatus(ctx, []Status{StatusScheduled})
}

func (s *Store) byStatus(ctx context.Context, statuses []Status) ([]Reminder, error) {
	in, args := inClause(statuses)
	return s.query(ctx, `SELECT `+columns+` FROM reminders WHERE status IN (`+in+`) ORDER BY fire_at, id`, args...)
}

// inClause is the placeholders and arguments for `status IN (…)`.
func inClause(statuses []Status) (string, []any) {
	marks := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, st := range statuses {
		marks[i], args[i] = "?", string(st)
	}
	return strings.Join(marks, ","), args
}

// Due returns scheduled reminders whose fire time is at or before now.
func (s *Store) Due(ctx context.Context, now time.Time) ([]Reminder, error) {
	return s.query(ctx, `SELECT `+columns+` FROM reminders WHERE status = 'scheduled' AND fire_at <= ? ORDER BY fire_at, id`, now.Unix())
}

// Recent returns finished reminders (done, missed, failed) newest first, for the
// in-app notice list.
func (s *Store) Recent(ctx context.Context, limit int) ([]Reminder, error) {
	return s.query(ctx, `SELECT `+columns+` FROM reminders WHERE status IN ('done','missed','failed') ORDER BY fire_at DESC, id DESC LIMIT ?`, limit)
}

// eventMillis is the stored form of an event time: 0 when there is none.
func eventMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// EventsAfter returns the reminders whose last outcome came after the given time (zero means from the start), newest
// first and at most limit: what the web UI has not yet been told about.
func (s *Store) EventsAfter(ctx context.Context, after time.Time, limit int) ([]Reminder, error) {
	return s.query(ctx, `SELECT `+columns+` FROM reminders WHERE event != '' AND event_at > ? ORDER BY event_at DESC, id DESC LIMIT ?`, eventMillis(after), limit)
}

// LatestEventAt is the time of the newest outcome saved on any reminder, zero when there is none: the baseline a
// client starts from so it announces only what happens after it began watching.
func (s *Store) LatestEventAt(ctx context.Context) (time.Time, error) {
	var millis int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(event_at), 0) FROM reminders`).Scan(&millis); err != nil {
		return time.Time{}, fmt.Errorf("reminders latest event: %w", err)
	}
	if millis <= 0 {
		return time.Time{}, nil
	}
	return time.UnixMilli(millis).UTC(), nil
}

// Update writes the result of a delivery attempt back to a reminder that is still
// scheduled and has not been edited since it was read (same Rev). ErrNotFound
// means a cancel, delete or edit got there first: the user's change wins and the
// stale result is dropped.
func (s *Store) Update(ctx context.Context, r Reminder) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reminders SET fire_at = ?, status = ?, attempts = ?, last_error = ?, event = ?, event_at = ? WHERE id = ? AND status = 'scheduled' AND rev = ?`,
		r.FireAt.Unix(), string(r.Status), r.Attempts, r.LastError, r.Event, eventMillis(r.EventAt), r.ID, r.Rev)
	if err != nil {
		return fmt.Errorf("reminders update: %w", err)
	}
	return affected(res)
}

// Replace overwrites what the user can edit (message, time, recurrence, end,
// channel) and starts delivery afresh: a scheduled or finished reminder becomes
// scheduled (so a missed or failed one can be rescheduled), a paused one stays
// paused. Identity (zone, creator, creation time) is kept and Rev is bumped.
// ErrNotFound when the reminder is cancelled or does not exist.
func (s *Store) Replace(ctx context.Context, r Reminder) error {
	var until any
	if r.Until != nil {
		until = r.Until.Unix()
	}
	editable := append(append([]Status{}, openStatuses...), finishedStatuses...)
	in, args := inClause(editable)
	args = append([]any{r.Message, r.FireAt.Unix(), r.Cron, until, r.Connector, r.ID}, args...)
	res, err := s.db.ExecContext(ctx,
		`UPDATE reminders SET message = ?, fire_at = ?, cron = ?, until_at = ?, connector = ?, attempts = 0, last_error = '',
		   status = CASE WHEN status = 'paused' THEN 'paused' ELSE 'scheduled' END, rev = rev + 1
		 WHERE id = ? AND status IN (`+in+`)`, args...)
	if err != nil {
		return fmt.Errorf("reminders replace: %w", err)
	}
	return affected(res)
}

// Transition applies a state change the user asked for (pause, resume, skip):
// it writes the reminder's status, time and failure record, bumps Rev, and only
// if the reminder is currently in one of the from states; otherwise ErrNotFound.
func (s *Store) Transition(ctx context.Context, r Reminder, from ...Status) error {
	in, args := inClause(from)
	args = append([]any{r.FireAt.Unix(), string(r.Status), r.Attempts, r.LastError, r.ID}, args...)
	res, err := s.db.ExecContext(ctx,
		`UPDATE reminders SET fire_at = ?, status = ?, attempts = ?, last_error = ?, rev = rev + 1 WHERE id = ? AND status IN (`+in+`)`, args...)
	if err != nil {
		return fmt.Errorf("reminders transition: %w", err)
	}
	return affected(res)
}

// affected is ErrNotFound when a guarded write matched no row.
func affected(res sql.Result) error {
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a reminder in any status; ErrNotFound when there is none.
func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM reminders WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("reminders delete: %w", err)
	}
	return affected(res)
}

// Cancel marks a scheduled reminder cancelled; ErrNotFound when it is not open.
func (s *Store) Cancel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE reminders SET status = 'cancelled', rev = rev + 1 WHERE id = ? AND status IN ('scheduled','paused')`, id)
	if err != nil {
		return fmt.Errorf("reminders cancel: %w", err)
	}
	return affected(res)
}

// CountOpen is the number of scheduled reminders.
func (s *Store) CountOpen(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reminders WHERE status IN ('scheduled','paused')`).Scan(&n)
	return n, err
}

func (s *Store) query(ctx context.Context, q string, args ...any) ([]Reminder, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("reminders query: %w", err)
	}
	defer rows.Close()
	var out []Reminder
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scan(row scanner) (Reminder, error) {
	var (
		r                 Reminder
		fireAt, createdAt int64
		eventAt           int64
		until             sql.NullInt64
		status            string
	)
	if err := row.Scan(&r.ID, &r.Message, &fireAt, &r.Cron, &until, &r.Timezone, &r.Connector,
		&status, &r.Rev, &r.Attempts, &r.LastError, &r.CreatedBy, &createdAt, &r.Event, &eventAt); err != nil {
		return Reminder{}, err
	}
	if eventAt > 0 {
		r.EventAt = time.UnixMilli(eventAt).UTC()
	}
	r.FireAt = time.Unix(fireAt, 0).UTC()
	r.CreatedAt = time.Unix(createdAt, 0).UTC()
	r.Status = Status(status)
	if until.Valid {
		u := time.Unix(until.Int64, 0).UTC()
		r.Until = &u
	}
	return r, nil
}
