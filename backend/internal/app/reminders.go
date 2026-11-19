package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/toolregistry"
	"llm-proxy/internal/platform/logging"
)

// initReminders attaches the hosted Reminders stack to the assistant's tools and to /remind. It is a no-op (reminders unavailable, everything else
// unaffected) when the stack could not be hosted or the tool registry is missing. The stack's own loop is started by the app lifecycle with the
// other stacks, not here.
func (c *Container) initReminders(s *AppServices) {
	registry := localRegistry(s.toolProvider)
	if s.netaudit == nil || s.netaudit.Reminders == nil || registry == nil {
		logging.Warn("reminders disabled: the stack or the tool registry is unavailable")
		return
	}
	port := &remindersPort{h: s.netaudit.Reminders}
	s.Reminders = port
	registry.WithReminders(port, func() bool { return !s.AppCtx.GetSettings().Reminders.Effective().Disabled })
}

// stageLegacyReminders reads the reminders the service kept in its main database before the Reminders stack existed and stages them on the stack, which
// copies them into its own database in its Start, once. It runs before the stack is started. The old table is left as it was (nothing deletes it), and a
// factory reset keeps it, as it always kept orchestrator.db. A read that fails is logged and tried again at the next start.
func (s *AppServices) stageLegacyReminders(ctx context.Context, h *stackHost) {
	provider := s.AppCtx.DBProvider()
	if provider == nil {
		return
	}
	rows, err := readLegacyReminders(ctx, provider.DB())
	if err != nil {
		logging.Warn("reminders: the old reminders could not be read; they will be tried again at the next start", "error", err)
		return
	}
	if len(rows) == 0 {
		return // nothing to copy: the stack creates its database when the first reminder is set
	}
	if err := stageLegacyReminders(h, rows); err != nil {
		logging.Warn("reminders: the old reminders could not be handed to the stack", "error", err)
	}
}

// legacyColumns are the columns of the old table, in the order readLegacyReminders scans them.
const legacyColumns = `id, message, fire_at, cron, until_at, timezone, connector, status, rev, attempts, last_error, created_by, created_at, event, event_at`

// readLegacyReminders reads every row of the old reminders table, or none when the table does not exist (a new install, or an install that never
// had one). Times are stored as unix seconds, and an outcome's time as unix milliseconds.
func readLegacyReminders(ctx context.Context, db *sql.DB) ([]legacyReminder, error) {
	var name string
	switch err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'reminders'`).Scan(&name); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+legacyColumns+` FROM reminders ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyReminder
	for rows.Next() {
		var (
			r                          legacyReminder
			fireAt, createdAt, eventAt int64
			until                      sql.NullInt64
		)
		if err := rows.Scan(&r.ID, &r.Message, &fireAt, &r.Cron, &until, &r.Timezone, &r.Connector, &r.Status, &r.Rev, &r.Attempts, &r.LastError,
			&r.CreatedBy, &createdAt, &r.Event, &eventAt); err != nil {
			return nil, err
		}
		r.FireAt, r.CreatedAt = time.Unix(fireAt, 0).UTC(), time.Unix(createdAt, 0).UTC()
		if until.Valid {
			u := time.Unix(until.Int64, 0).UTC()
			r.Until = &u
		}
		if eventAt > 0 {
			e := time.UnixMilli(eventAt).UTC()
			r.EventAt = &e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// localRegistry finds the local tool registry inside the tool provider chain.
func localRegistry(tp assistantPkg.ToolProvider) *toolregistry.LocalToolRegistry {
	mtp, ok := tp.(*toolregistry.MultiToolProvider)
	if !ok {
		return nil
	}
	for _, p := range mtp.Providers {
		if ltr, ok := p.(*toolregistry.LocalToolRegistry); ok {
			return ltr
		}
	}
	return nil
}

// eraseReminders removes every reminder for a factory reset: the stack's own, and the rows still in the old table of the main database (which the
// reset keeps), so none can be copied in later and none is left behind.
func (s *AppServices) eraseReminders(ctx context.Context, h *stackHost) error {
	if err := eraseStackReminders(ctx, h); err != nil {
		return err
	}
	provider := s.AppCtx.DBProvider()
	if provider == nil {
		return nil
	}
	return clearLegacyReminders(ctx, provider.DB())
}

// clearLegacyReminders empties the old reminders table; a database without it has nothing to clear.
func clearLegacyReminders(ctx context.Context, db *sql.DB) error {
	var name string
	switch err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'reminders'`).Scan(&name); {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return err
	}
	_, err := db.ExecContext(ctx, `DELETE FROM reminders`)
	return err
}
