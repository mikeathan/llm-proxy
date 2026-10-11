// Package reminders stores user reminders and delivers them verbatim at the
// right time. There is no LLM at fire time: the text the user asked for is the
// text that is sent.
package reminders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
)

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
	Status    Status
	Attempts  int
	LastError string
	CreatedBy string
	CreatedAt time.Time
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

const columns = `id, message, fire_at, cron, until_at, timezone, connector, status, attempts, last_error, created_by, created_at`

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

// ListOpen returns scheduled reminders, soonest first.
func (s *Store) ListOpen(ctx context.Context) ([]Reminder, error) {
	return s.query(ctx, `SELECT `+columns+` FROM reminders WHERE status = 'scheduled' ORDER BY fire_at, id`)
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

// Update writes the mutable fields of a reminder.
func (s *Store) Update(ctx context.Context, r Reminder) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reminders SET fire_at = ?, status = ?, attempts = ?, last_error = ? WHERE id = ?`,
		r.FireAt.Unix(), string(r.Status), r.Attempts, r.LastError, r.ID)
	if err != nil {
		return fmt.Errorf("reminders update: %w", err)
	}
	return nil
}

// Cancel marks a scheduled reminder cancelled; ErrNotFound when it is not open.
func (s *Store) Cancel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE reminders SET status = 'cancelled' WHERE id = ? AND status = 'scheduled'`, id)
	if err != nil {
		return fmt.Errorf("reminders cancel: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountOpen is the number of scheduled reminders.
func (s *Store) CountOpen(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reminders WHERE status = 'scheduled'`).Scan(&n)
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
		until             sql.NullInt64
		status            string
	)
	if err := row.Scan(&r.ID, &r.Message, &fireAt, &r.Cron, &until, &r.Timezone, &r.Connector,
		&status, &r.Attempts, &r.LastError, &r.CreatedBy, &createdAt); err != nil {
		return Reminder{}, err
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
