package radio

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"llm-proxy-stacks/sdk"

	_ "modernc.org/sqlite"
)

// The store (ADR-0053): one SQLite file in the stack's data folder. It holds tags, sealed addresses and counts, never a frame and never a raw
// address; the caller hands it tags, and the types here have no field for an address. Other people's rows have a day's resolution and expire
// in 14 days; the owner's keep until deleted or 30 days (events). The file cannot pass its cap: SQLite's own page limit is the hard stop and
// the lowest-value rows of other people go first, before it is reached.
const (
	// DBFile is the file the manifest lists in Storage.
	DBFile = "radio.db"
	// DefaultCapBytes is the cap when none is given (the plan's 20 MB).
	DefaultCapBytes = 20 << 20
	// OthersKeep is how long a row about someone else's device, access point or event is kept after it was last seen; OwnerKeep is the same for
	// the owner's events.
	OthersKeep = 14 * 24 * time.Hour
	OwnerKeep  = 30 * 24 * time.Hour

	pageSize   = 4096
	evictAbove = 90 // percent of the cap at which the lowest-value rows start to go
	evictBatch = 500
)

// ErrFull is returned when only the owner's rows are left to evict and the cap is still reached.
var ErrFull = errors.New("radio: the store is full of the owner's own rows")

const schema = `
CREATE TABLE IF NOT EXISTS devices (
	tag BLOB PRIMARY KEY, owner INTEGER NOT NULL, sealed BLOB, label TEXT NOT NULL DEFAULT '', class TEXT NOT NULL DEFAULT '',
	visits INTEGER NOT NULL, first_day INTEGER NOT NULL, last_day INTEGER NOT NULL, sig_min INTEGER, sig_max INTEGER
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS beacons (
	tag BLOB PRIMARY KEY, own INTEGER NOT NULL, name TEXT NOT NULL DEFAULT '', name_len INTEGER NOT NULL, name_hash BLOB NOT NULL,
	security TEXT NOT NULL, channel INTEGER NOT NULL, visits INTEGER NOT NULL, first_day INTEGER NOT NULL, last_day INTEGER NOT NULL
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS events (
	day INTEGER NOT NULL, kind TEXT NOT NULL, owner INTEGER NOT NULL, tag BLOB NOT NULL, n INTEGER NOT NULL,
	PRIMARY KEY (day, kind, owner, tag)
) WITHOUT ROWID;`

// Store is the Radio stack's database.
type Store struct {
	db       *sql.DB
	capBytes int64
}

// OpenStore opens (creating it if needed) the database at path. capBytes <= 0 means DefaultCapBytes.
func OpenStore(path string, capBytes int64) (*Store, error) {
	if capBytes <= 0 {
		capBytes = DefaultCapBytes
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=auto_vacuum(2)" +
		"&_pragma=max_page_count(" + strconv.FormatInt(capBytes/pageSize, 10) + ")"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("radio store: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("radio store schema: %w", err)
	}
	var ps int
	if err := db.QueryRow("PRAGMA page_size").Scan(&ps); err != nil || ps != pageSize {
		_ = db.Close()
		return nil, fmt.Errorf("radio store: page size %d, want %d: %v", ps, pageSize, err)
	}
	return &Store{db: db, capBytes: capBytes}, nil
}

// Close closes the database, which checkpoints the log.
func (s *Store) Close() error { return s.db.Close() }

func dayNumber(t time.Time) int64 { return t.UTC().Unix() / 86400 }

func dayTime(d int64) time.Time { return time.Unix(d*86400, 0).UTC() }

// Seen is one sighting of a device. Tag is the vault's tag of its address. A row of the owner's (Owner) carries the sealed address and a
// label; a row of anyone else carries neither.
type Seen struct {
	Tag      []byte
	Owner    bool
	Sealed   []byte
	Label    string
	Class    string
	Signal   int
	SignalOK bool
	At       time.Time
}

func (v Seen) check() error {
	switch {
	case len(v.Tag) == 0:
		return errors.New("radio store: a device needs a tag")
	case !v.Owner && (v.Sealed != nil || v.Label != ""):
		return errors.New("radio store: only the owner's devices carry a sealed address or a label")
	case v.Owner && len(v.Sealed) == 0:
		return errors.New("radio store: the owner's device needs a sealed address")
	}
	return nil
}

const upsertDevice = `
INSERT INTO devices(tag, owner, sealed, label, class, visits, first_day, last_day, sig_min, sig_max) VALUES(?,?,?,?,?,1,?,?,?,?)
ON CONFLICT(tag) DO UPDATE SET
	visits = visits + (CASE WHEN last_day < excluded.last_day THEN 1 ELSE 0 END),
	first_day = min(first_day, excluded.first_day),
	last_day = max(last_day, excluded.last_day),
	owner = max(owner, excluded.owner),
	sealed = coalesce(excluded.sealed, sealed),
	label = CASE WHEN excluded.label != '' THEN excluded.label ELSE label END,
	class = CASE WHEN excluded.class != '' THEN excluded.class ELSE class END,
	sig_min = CASE WHEN excluded.sig_min IS NULL THEN sig_min WHEN sig_min IS NULL THEN excluded.sig_min ELSE min(sig_min, excluded.sig_min) END,
	sig_max = CASE WHEN excluded.sig_max IS NULL THEN sig_max WHEN sig_max IS NULL THEN excluded.sig_max ELSE max(sig_max, excluded.sig_max) END`

// SeenDevice records a sighting: a new row, or the existing row's visits (one a day), last day and signal range.
func (s *Store) SeenDevice(ctx context.Context, v Seen) error {
	if err := v.check(); err != nil {
		return err
	}
	var sig any
	if v.SignalOK {
		sig = v.Signal
	}
	d := dayNumber(v.At)
	return s.write(ctx, func() error {
		_, err := s.db.ExecContext(ctx, upsertDevice, v.Tag, v.Owner, v.Sealed, v.Label, v.Class, d, d, sig, sig)
		return err
	})
}

// TouchOwner records a sighting of a device only when its row is the owner's, and says whether it was; the owner's sealed address is not
// sent again for every frame.
func (s *Store) TouchOwner(ctx context.Context, tag []byte, signal int, signalOK bool, at time.Time) (bool, error) {
	var sig any
	if signalOK {
		sig = signal
	}
	d := dayNumber(at)
	var touched bool
	err := s.write(ctx, func() error {
		res, err := s.db.ExecContext(ctx, `UPDATE devices SET
			visits = visits + (CASE WHEN last_day < ?1 THEN 1 ELSE 0 END), last_day = max(last_day, ?1),
			sig_min = CASE WHEN ?2 IS NULL THEN sig_min WHEN sig_min IS NULL THEN ?2 ELSE min(sig_min, ?2) END,
			sig_max = CASE WHEN ?2 IS NULL THEN sig_max WHEN sig_max IS NULL THEN ?2 ELSE max(sig_max, ?2) END
			WHERE tag = ?3 AND owner = 1`, d, sig, tag)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		touched = n > 0
		return nil
	})
	return touched, err
}

// Device is a stored device row.
type Device struct {
	Tag                  []byte
	Owner                bool
	Sealed               []byte
	Label, Class         string
	Visits               int
	FirstDay, LastDay    int64
	SignalMin, SignalMax int
	HasSignal            bool
}

// Device returns the row for tag.
func (s *Store) Device(ctx context.Context, tag []byte) (Device, bool, error) {
	var d Device
	var lo, hi sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT tag, owner, sealed, label, class, visits, first_day, last_day, sig_min, sig_max FROM devices WHERE tag = ?`, tag).
		Scan(&d.Tag, &d.Owner, &d.Sealed, &d.Label, &d.Class, &d.Visits, &d.FirstDay, &d.LastDay, &lo, &hi)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, false, nil
	}
	if err != nil {
		return Device{}, false, err
	}
	d.HasSignal, d.SignalMin, d.SignalMax = lo.Valid && hi.Valid, int(lo.Int64), int(hi.Int64)
	return d, true, nil
}

// Beacon is one sighting of an access point. Only the owner's own access point (Own) keeps its name; for any other the store keeps the
// name's length and a short hash.
type Beacon struct {
	Tag      []byte
	Own      bool
	Name     string
	Security string
	Channel  int
	At       time.Time
}

const upsertBeacon = `
INSERT INTO beacons(tag, own, name, name_len, name_hash, security, channel, visits, first_day, last_day) VALUES(?,?,?,?,?,?,?,1,?,?)
ON CONFLICT(tag) DO UPDATE SET
	visits = visits + (CASE WHEN last_day < excluded.last_day THEN 1 ELSE 0 END),
	last_day = max(last_day, excluded.last_day), own = max(own, excluded.own), name = excluded.name, name_len = excluded.name_len,
	name_hash = excluded.name_hash, security = excluded.security, channel = excluded.channel`

// SeenBeacon records a sighting of an access point.
func (s *Store) SeenBeacon(ctx context.Context, b Beacon) error {
	if len(b.Tag) == 0 {
		return errors.New("radio store: a beacon needs a tag")
	}
	name := ""
	if b.Own {
		name = b.Name
	}
	h := sha256.Sum256([]byte(b.Name))
	d := dayNumber(b.At)
	return s.write(ctx, func() error {
		_, err := s.db.ExecContext(ctx, upsertBeacon, b.Tag, b.Own, name, len(b.Name), h[:8], b.Security, b.Channel, d, d)
		return err
	})
}

// AddEvent counts one event of a kind on the day. An event of the owner's device keeps the device's tag; any other is a plain count.
func (s *Store) AddEvent(ctx context.Context, kind string, tag []byte, owner bool, at time.Time) error {
	if !owner || tag == nil {
		tag = []byte{}
	}
	return s.write(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `INSERT INTO events(day, kind, owner, tag, n) VALUES(?,?,?,?,1)
			ON CONFLICT(day, kind, owner, tag) DO UPDATE SET n = n + 1`, dayNumber(at), kind, owner, tag)
		return err
	})
}

// write runs one change, then keeps the file under its cap.
func (s *Store) write(ctx context.Context, fn func() error) error {
	if err := fn(); err != nil {
		if !isFull(err) {
			return err
		}
		if err := s.evict(ctx, 0); err != nil {
			return err
		}
		if err := fn(); err != nil {
			return err
		}
	}
	return s.evict(ctx, evictAbove)
}

func isFull(err error) bool {
	m := err.Error()
	return strings.Contains(m, "database or disk is full") || strings.Contains(m, "SQLITE_FULL")
}

// used is the bytes in pages that hold data.
func (s *Store) used(ctx context.Context) (int64, error) {
	var pages, free int64
	if err := s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return 0, err
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&free); err != nil {
		return 0, err
	}
	return (pages - free) * pageSize, nil
}

var evictions = []string{
	`DELETE FROM events WHERE (day, kind, owner, tag) IN (SELECT day, kind, owner, tag FROM events WHERE owner = 0 ORDER BY day ASC, n ASC LIMIT ?)`,
	`DELETE FROM beacons WHERE tag IN (SELECT tag FROM beacons WHERE own = 0 ORDER BY visits ASC, last_day ASC LIMIT ?)`,
	`DELETE FROM devices WHERE tag IN (SELECT tag FROM devices WHERE owner = 0 ORDER BY visits ASC, last_day ASC LIMIT ?)`,
}

// evict removes other people's lowest-value rows (fewest visits, then oldest) until the data is at or below percent of the cap; percent 0 means
// room is made after a write was refused as full. The owner's rows are never removed: when nothing else is left and the cap is reached it
// returns ErrFull.
func (s *Store) evict(ctx context.Context, percent int64) error {
	limit := s.capBytes * percent / 100
	if percent == 0 {
		limit = s.capBytes * (evictAbove - 10) / 100
	}
	for {
		u, err := s.used(ctx)
		if err != nil {
			return err
		}
		if u <= limit {
			return nil
		}
		removed, err := s.evictBatch(ctx)
		if err != nil {
			return err
		}
		if removed == 0 {
			if percent == 0 || u >= s.capBytes {
				return ErrFull
			}
			return nil
		}
	}
}

func (s *Store) evictBatch(ctx context.Context) (int64, error) {
	removed := int64(0)
	for _, q := range evictions {
		res, err := s.db.ExecContext(ctx, q, evictBatch)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		removed += n
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA incremental_vacuum"); err != nil {
		return 0, err
	}
	return removed, nil
}

// Prune removes what is past its time as of now and gives the freed pages back; it returns the rows removed.
func (s *Store) Prune(ctx context.Context, now time.Time) (int, error) {
	today := dayNumber(now)
	other, owner := today-int64(OthersKeep/(24*time.Hour)), today-int64(OwnerKeep/(24*time.Hour))
	total := 0
	for _, q := range []struct {
		sql string
		arg int64
	}{
		{`DELETE FROM devices WHERE owner = 0 AND last_day < ?`, other},
		{`DELETE FROM beacons WHERE own = 0 AND last_day < ?`, other},
		{`DELETE FROM events WHERE owner = 0 AND day < ?`, other},
		{`DELETE FROM events WHERE owner = 1 AND day < ?`, owner},
	} {
		res, err := s.db.ExecContext(ctx, q.sql, q.arg)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	_, err := s.db.ExecContext(ctx, "PRAGMA incremental_vacuum")
	return total, err
}

// ForgetOthers removes every row about someone else's devices, access points and events; the owner's stay. (Rotating the tag key, once the vault
// supports it, makes them unmatchable as well; this removes them outright.)
func (s *Store) ForgetOthers(ctx context.Context) error {
	for _, q := range []string{`DELETE FROM devices WHERE owner = 0`, `DELETE FROM beacons WHERE own = 0`, `DELETE FROM events WHERE owner = 0`} {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, "PRAGMA incremental_vacuum")
	return err
}

func (s *Store) count(table string) (int, error) {
	var n int
	err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n)
	return n, err
}

// StorageUse reports the file for the data management page: counts and a time, no names.
func (s *Store) StorageUse(ctx context.Context) ([]stacksdk.StorageUse, error) {
	u, err := s.used(ctx)
	if err != nil {
		return nil, err
	}
	rows := 0
	for _, t := range []string{"devices", "beacons", "events"} {
		n, err := s.count(t)
		if err != nil {
			return nil, err
		}
		rows += n
	}
	use := stacksdk.StorageUse{File: DBFile, Bytes: u, CapBytes: s.capBytes, Rows: rows, KeepFor: OthersKeep}
	var oldest sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT min(d) FROM (SELECT min(first_day) d FROM devices UNION ALL SELECT min(first_day) FROM beacons UNION ALL SELECT min(day) FROM events)`).Scan(&oldest); err != nil {
		return nil, err
	}
	if oldest.Valid {
		use.Oldest = dayTime(oldest.Int64)
	}
	return []stacksdk.StorageUse{use}, nil
}
