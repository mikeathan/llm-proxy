package radio

import (
	"context"
	"database/sql"
	"time"
)

// The rows ADR-0058 adds beside a device's tag: what it says it is, which network it uses and which box it belongs to. Each is written once a cycle
// from what the recorder gathered in memory, in one transaction, and only onto rows that exist (a device not in the picture has nothing to describe).

// IdentityRow is what a device's frames said about the product in one cycle. An empty field leaves the stored one as it was.
type IdentityRow struct {
	Tag          []byte
	Manufacturer string
	Model        string
	Category     int
	Hint         string
	Signature    uint64
}

// LinkRow is the network a device exchanged frames with in one cycle: Net is the tag of the access point's own device row, Own says it is one of
// the owner's confirmed (and not guest) access points.
type LinkRow struct {
	Tag []byte
	Net []byte
	Own bool
}

// BoxRow puts an access point in a box: the radios of one physical router share a box tag.
type BoxRow struct {
	Tag []byte
	Box []byte
}

// inTx runs fn in one write transaction.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	return s.write(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := fn(tx); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// SetIdentities writes the cycle's identity facts.
func (s *Store) SetIdentities(ctx context.Context, rows []IdentityRow) error {
	if len(rows) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx, `UPDATE devices SET
				wps_maker = CASE WHEN ?1 != '' THEN ?1 ELSE wps_maker END,
				model = CASE WHEN ?2 != '' THEN ?2 ELSE model END,
				category = CASE WHEN ?3 != 0 THEN ?3 ELSE category END,
				hint = CASE WHEN ?4 != '' THEN ?4 ELSE hint END,
				sig = CASE WHEN ?5 != 0 THEN ?5 ELSE sig END
				WHERE tag = ?6`, r.Manufacturer, r.Model, r.Category, r.Hint, int64(r.Signature), r.Tag); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetLinks writes the network each device used this cycle. A link to one of the owner's networks also marks the device as having joined it and
// counts one more cycle on it; the mark is evidence the page shows, never a decision that the device is the owner's.
func (s *Store) SetLinks(ctx context.Context, rows []LinkRow) error {
	if len(rows) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, r := range rows {
			own := 0
			if r.Own {
				own = 1
			}
			if _, err := tx.ExecContext(ctx, `UPDATE devices SET net = ?1, joined_own = max(joined_own, ?2), own_cycles = own_cycles + ?2 WHERE tag = ?3`, r.Net, own, r.Tag); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetOwnAPRows marks, on the device row of every access point heard this cycle, whether it is one of the owner's confirmed ones (a guest one
// included: it is the owner's radio). An access point the owner stopped confirming loses the mark the next cycle it is heard.
func (s *Store) SetOwnAPRows(ctx context.Context, own map[string]bool) error {
	if len(own) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for t, yes := range own {
			if _, err := tx.ExecContext(ctx, `UPDATE devices SET own_ap = ? WHERE tag = ?`, yes, []byte(t)); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetBoxes writes which box each access point belongs to.
func (s *Store) SetBoxes(ctx context.Context, rows []BoxRow) error {
	if len(rows) == 0 {
		return nil
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx, `UPDATE devices SET box = ? WHERE tag = ?`, r.Box, r.Tag); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetKind records what a device is. by is "owner" for the owner's word (which the classifier never changes) or "" for the classifier's. A
// classifier's answer never overwrites the owner's.
func (s *Store) SetKind(ctx context.Context, tag []byte, kind DeviceKind, by string) (bool, error) {
	var found bool
	err := s.write(ctx, func() error {
		q := `UPDATE devices SET kind = ?, kind_by = ? WHERE tag = ? AND kind_by != 'owner'`
		if by == kindByOwner {
			q = `UPDATE devices SET kind = ?, kind_by = ? WHERE tag = ?`
		}
		res, err := s.db.ExecContext(ctx, q, string(kind), by, tag)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		found = n > 0
		return nil
	})
	return found, err
}

// SetOwnAPGuest marks one of the owner's access points as a guest network: devices on it are visitors', so using it is no evidence of ownership.
func (s *Store) SetOwnAPGuest(ctx context.Context, tag []byte, guest bool) (bool, error) {
	var found bool
	err := s.write(ctx, func() error {
		res, err := s.db.ExecContext(ctx, `UPDATE own_aps SET guest = ? WHERE tag = ?`, guest, tag)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		found = n > 0
		return nil
	})
	return found, err
}

// clearReferences drops every reference from a row to another device (its network and its box) after the neighbours are forgotten: the rows they
// pointed at are gone, and the next cycle makes them again under the new epoch.
func clearReferences(ctx context.Context, ex interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}) error {
	_, err := ex.ExecContext(ctx, `UPDATE devices SET net = NULL, box = NULL`)
	return err
}

// CanCalibrate says whether a device may calibrate the distance model: one of the owner's devices, or one of the owner's access points.
func (s *Store) CanCalibrate(ctx context.Context, tag []byte) (bool, error) {
	var owner, ownAP bool
	err := s.db.QueryRowContext(ctx, `SELECT owner, own_ap FROM devices WHERE tag = ?`, tag).Scan(&owner, &ownAP)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return owner || ownAP, err
}

// TagsByShortID lists every device whose short id (the first bytes of its tag, in hex) is id: more than one means the id names neither.
func (s *Store) TagsByShortID(ctx context.Context, id string) ([][]byte, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tag FROM devices WHERE lower(substr(hex(tag), 1, ?)) = lower(?)`, len(id), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var t []byte
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetOwnerClass records the owner's word on a device's class (it then trains the classifier and is never changed by it), or with "" hands the
// device back to the classifier. The change is logged like any other.
func (s *Store) SetOwnerClass(ctx context.Context, tag []byte, class Class, at time.Time) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var from string
		var owner bool
		if err := tx.QueryRowContext(ctx, `SELECT class, owner FROM devices WHERE tag = ?`, tag).Scan(&from, &owner); err != nil {
			return err
		}
		ver := 0
		if class == "" {
			ver = -1 // not the owner's word and not any rules version: the next cycle decides it again
		}
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET class = ?, class_ver = ?, pending = '', pending_n = 0 WHERE tag = ?`, string(class), ver, tag); err != nil {
			return err
		}
		if class == "" || string(class) == from {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO class_log(day, tag, owner, from_class, to_class, version, reason) VALUES(?,?,?,?,?,0,?)`,
			dayNumber(at), tag, owner, from, string(class), string(WhyOwnerSaid))
		return err
	})
}

// DeviceClassLog is one device's own class changes, newest first: shown on that device's page only.
func (s *Store) DeviceClassLog(ctx context.Context, tag []byte, limit int) ([]ClassChange, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT day, from_class, to_class, version, reason FROM class_log WHERE tag = ? ORDER BY day DESC, rowid DESC LIMIT ?`, tag, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ClassChange{}
	for rows.Next() {
		var c ClassChange
		var d int64
		if err := rows.Scan(&d, &c.From, &c.To, &c.Version, &c.Reason); err != nil {
			return nil, err
		}
		c.Day, c.N = dayString(d), 1
		out = append(out, c)
	}
	return out, rows.Err()
}
