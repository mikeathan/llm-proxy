package radio

import (
	"database/sql"
	"fmt"
)

// The database is versioned with PRAGMA user_version so a file made by an earlier release (the one running on a live computer) is upgraded in
// place and never rebuilt. Version 0 is a file with the first schema (or none); each step below runs once, inside one transaction.
//
//	1 the first schema (devices, beacons, events, meta)
//	2 R3 part 2: day-level aggregates for the classifier, the owner's own access points, stored documents (the probe, the rules) and the class log
const schemaVersion = 2

var migrations = map[int][]string{
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
