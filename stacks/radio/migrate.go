package radio

import (
	"database/sql"
	"fmt"
)

// The database is versioned with PRAGMA user_version so a file made by an earlier release (the one running on a live computer) is upgraded in
// place and never rebuilt. Version 0 is a file with the first schema (or none); each step below runs once, inside one transaction.
//
//	1 the first schema (devices, beacons, events, meta)
//	6 ADR-0055: the optional per-cycle signal track (off by default; empty unless the owner turns it on)
//	5 R4: the strongest signal an access point was heard at (a hidden one close to the house)
//	4 R4: whether each of the owner's own access points protects its management frames (from its beacon)
//	3 R5: how often a device retried and the channels it was heard on (per day), and the owner's own presence by hour
//	2 R3 part 2: day-level aggregates for the classifier, the owner's own access points, stored documents (the probe, the rules) and the class log
const schemaVersion = 6

var migrations = map[int][]string{
	6: {
		`CREATE TABLE air_track (tag BLOB NOT NULL, at INTEGER NOT NULL, sig INTEGER NOT NULL, channel INTEGER NOT NULL)`,
		`CREATE INDEX air_track_tag_at ON air_track(tag, at)`,
		`CREATE INDEX air_track_at ON air_track(at)`,
	},
	5: {`ALTER TABLE beacons ADD COLUMN sig_max INTEGER`},
	4: {`ALTER TABLE own_aps ADD COLUMN pmf TEXT NOT NULL DEFAULT ''`},
	3: {
		`ALTER TABLE daily ADD COLUMN retries INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE daily ADD COLUMN channels INTEGER NOT NULL DEFAULT 0`,
		// The owner's devices by hour (UTC hours since the epoch): how many cycles heard each, with its signal. Nobody else's device has a row here.
		`CREATE TABLE owner_hours (
			tag BLOB NOT NULL, hour INTEGER NOT NULL, cycles INTEGER NOT NULL, sig_n INTEGER NOT NULL, sig_sum INTEGER NOT NULL, sig_max INTEGER,
			PRIMARY KEY (tag, hour)
		) WITHOUT ROWID`,
		// How many cycles the sensor listened through in an hour: what tells "away" from "nobody was listening".
		`CREATE TABLE air_hours (hour INTEGER PRIMARY KEY, cycles INTEGER NOT NULL) WITHOUT ROWID`,
	},
	2: {
		`ALTER TABLE devices ADD COLUMN random INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE devices ADD COLUMN joined_own INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE devices ADD COLUMN class_ver INTEGER NOT NULL DEFAULT 0`,
		// One row per device and day: nothing finer than a day is kept for anyone (ADR-0053). cycles is how many listening cycles (every channel visited once) heard it.
		`CREATE TABLE daily (
			tag BLOB NOT NULL, day INTEGER NOT NULL, cycles INTEGER NOT NULL, frames INTEGER NOT NULL,
			sig_n INTEGER NOT NULL, sig_sum INTEGER NOT NULL, sig_sq INTEGER NOT NULL, sig_max INTEGER,
			PRIMARY KEY (tag, day)
		) WITHOUT ROWID`,
		// How many full cycles the sensor listened through on a day: the denominator of "present most of the time".
		`CREATE TABLE air_days (day INTEGER PRIMARY KEY, cycles INTEGER NOT NULL) WITHOUT ROWID`,
		// Small documents the stack keeps: the last probe of each adapter, the classifier's rules by version.
		`CREATE TABLE docs (key TEXT PRIMARY KEY, val BLOB NOT NULL, day INTEGER NOT NULL) WITHOUT ROWID`,
		// The owner's own access points: the address sealed (like the owner's devices), the owner's own name for the network, and whether the owner
		// confirmed it. A name match only proposes: anyone can broadcast the same name.
		`CREATE TABLE own_aps (
			tag BLOB PRIMARY KEY, sealed BLOB NOT NULL, name TEXT NOT NULL, confirmed INTEGER NOT NULL,
			channel INTEGER NOT NULL, security TEXT NOT NULL, first_day INTEGER NOT NULL, last_day INTEGER NOT NULL
		) WITHOUT ROWID`,
		// Every change of a device's class: nothing is reclassified silently. Day resolution, a fixed reason word, the rules version that decided.
		`CREATE TABLE class_log (
			day INTEGER NOT NULL, tag BLOB NOT NULL, owner INTEGER NOT NULL, from_class TEXT NOT NULL, to_class TEXT NOT NULL,
			version INTEGER NOT NULL, reason TEXT NOT NULL
		)`,
		`CREATE INDEX class_log_day ON class_log(day)`,
	},
}

// migrate brings the database to schemaVersion.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	var have int
	if err := db.QueryRow("PRAGMA user_version").Scan(&have); err != nil {
		return err
	}
	for v := have + 1; v <= schemaVersion; v++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		for _, q := range migrations[v] {
			if _, err := tx.Exec(q); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("upgrade to version %d: %w", v, err)
			}
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
