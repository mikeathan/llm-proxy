package radio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

// What the classifier reads, kept at the resolution ADR-0053 allows: a row per device and day, never finer.

// agg is what one device did in one listening cycle: the frames it sent and their signal. A cycle visits every channel of the plan once.
type agg struct {
	frames, n int
	retries   int
	channels  uint64 // a bit for each channel heard on (channelBit)
	sum, sq   int64
	max       int
	hasMax    bool
	random    bool
}

// channelBit is the bit a channel has in a channel mask: its place in validChannels (0 for a channel the plan cannot visit).
func channelBit(ch int) uint64 {
	if i := slices.Index(validChannels, ch); i >= 0 {
		return 1 << uint(i)
	}
	return 0
}

// channelsOf lists the channels a mask holds, in the order of validChannels.
func channelsOf(mask uint64) []int {
	var out []int
	for i, ch := range validChannels {
		if mask&(1<<uint(i)) != 0 {
			out = append(out, ch)
		}
	}
	return out
}

func (a *agg) add(f Frame) {
	a.frames++
	if f.Retry {
		a.retries++
	}
	a.channels |= channelBit(channelOf(int(f.FreqMHz)))
	signal, ok := f.Signal, f.SignalOK
	if !ok {
		return
	}
	a.n++
	a.sum += int64(signal)
	a.sq += int64(signal) * int64(signal)
	if !a.hasMax || signal > a.max {
		a.max, a.hasMax = signal, true
	}
}

// FlushCycle writes one finished cycle: a daily row for every device heard (one more cycle, its frames and signal) and one more listened cycle
// for the day. It is one transaction, so a cycle is counted whole or not at all.
func (s *Store) FlushCycle(ctx context.Context, at time.Time, heard map[string]*agg) error {
	day := dayNumber(at)
	return s.write(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(ctx, `INSERT INTO air_days(day, cycles) VALUES(?,1) ON CONFLICT(day) DO UPDATE SET cycles = cycles + 1`, day); err != nil {
			return err
		}
		hour := hourNumber(at)
		if _, err := tx.ExecContext(ctx, `INSERT INTO air_hours(hour, cycles) VALUES(?,1) ON CONFLICT(hour) DO UPDATE SET cycles = cycles + 1`, hour); err != nil {
			return err
		}
		for tag, a := range heard {
			var mx any
			if a.hasMax {
				mx = a.max
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO daily(tag, day, cycles, frames, sig_n, sig_sum, sig_sq, sig_max, retries, channels) VALUES(?,?,1,?,?,?,?,?,?,?)
				ON CONFLICT(tag, day) DO UPDATE SET cycles = cycles + 1, frames = frames + excluded.frames, sig_n = sig_n + excluded.sig_n,
				sig_sum = sig_sum + excluded.sig_sum, sig_sq = sig_sq + excluded.sig_sq, retries = retries + excluded.retries, channels = channels | excluded.channels,
				sig_max = CASE WHEN excluded.sig_max IS NULL THEN sig_max WHEN sig_max IS NULL THEN excluded.sig_max ELSE max(sig_max, excluded.sig_max) END`,
				[]byte(tag), day, a.frames, a.n, a.sum, a.sq, mx, a.retries, int64(a.channels)); err != nil {
				return err
			}
			if err := flushOwnerHour(ctx, tx, []byte(tag), hour, a, mx); err != nil {
				return err
			}
			if a.random {
				if _, err := tx.ExecContext(ctx, `UPDATE devices SET random = 1 WHERE tag = ?`, []byte(tag)); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	})
}

// hourNumber is the hour since the Unix epoch, in UTC: the resolution of the owner's own presence rows. Nothing finer is kept, and nothing at all
// of this kind is kept for anyone else (ADR-0053).
func hourNumber(t time.Time) int64 { return t.UTC().Unix() / 3600 }

// flushOwnerHour adds one cycle to the hour's row of a device of the owner's. A device of anyone else writes nothing here.
func flushOwnerHour(ctx context.Context, tx *sql.Tx, tag []byte, hour int64, a *agg, sigMax any) error {
	var owner bool
	if err := tx.QueryRowContext(ctx, `SELECT owner FROM devices WHERE tag = ?`, tag).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if !owner {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO owner_hours(tag, hour, cycles, sig_n, sig_sum, sig_max) VALUES(?,?,1,?,?,?)
		ON CONFLICT(tag, hour) DO UPDATE SET cycles = cycles + 1, sig_n = sig_n + excluded.sig_n, sig_sum = sig_sum + excluded.sig_sum,
		sig_max = CASE WHEN excluded.sig_max IS NULL THEN sig_max WHEN sig_max IS NULL THEN excluded.sig_max ELSE max(sig_max, excluded.sig_max) END`,
		tag, hour, a.n, a.sum, sigMax)
	return err
}

// MarkJoinedOwn records that a device asked to join one of the owner's confirmed access points.
func (s *Store) MarkJoinedOwn(ctx context.Context, tag []byte) error {
	return s.write(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `UPDATE devices SET joined_own = 1 WHERE tag = ?`, tag)
		return err
	})
}

// FeatureRow is what the daily rows add up to for one device over the days asked for.
type FeatureRow struct {
	Tag          []byte
	Owner        bool
	Label, Class string
	ClassVer     int
	Random       bool
	JoinedOwn    bool
	FirstDay     int64 // the first day with a row
	LastDay      int64 // the last day with a row (the device's own last day when it has none)
	Days         int
	Cycles       int
	Frames       int
	Retries      int
	Channels     uint64
	SigN         int64
	SigSum       int64
	SigSq        int64
	SigMax       int
	HasMax       bool
}

// FeatureRows adds up every device's daily rows from sinceDay on, and returns them with the number of cycles the sensor listened through on each
// of those days (the denominator of "present most of the time").
func (s *Store) FeatureRows(ctx context.Context, sinceDay int64) ([]FeatureRow, map[int64]int, error) {
	// A device with no row in the window is still a row here (it has been heard, and has not yet been through a finished cycle), so every
	// device is sorted and the card and the list agree. The owner's devices are listed only once they have a row.
	rows, err := s.db.QueryContext(ctx, `SELECT d.tag, d.owner, d.label, d.class, d.class_ver, d.random, d.joined_own,
			COALESCE(min(x.day), d.first_day), COALESCE(max(x.day), d.last_day), count(x.day), COALESCE(sum(x.cycles), 0), COALESCE(sum(x.frames), 0), COALESCE(sum(x.retries), 0),
			COALESCE(group_concat(x.channels), ''), COALESCE(sum(x.sig_n), 0), COALESCE(sum(x.sig_sum), 0), COALESCE(sum(x.sig_sq), 0), max(x.sig_max)
		FROM devices d LEFT JOIN daily x ON x.tag = d.tag AND x.day >= ?1
		WHERE d.owner = 0 OR x.day IS NOT NULL GROUP BY d.tag`, sinceDay)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []FeatureRow
	for rows.Next() {
		var r FeatureRow
		var mx sql.NullInt64
		var masks string
		if err := rows.Scan(&r.Tag, &r.Owner, &r.Label, &r.Class, &r.ClassVer, &r.Random, &r.JoinedOwn,
			&r.FirstDay, &r.LastDay, &r.Days, &r.Cycles, &r.Frames, &r.Retries, &masks, &r.SigN, &r.SigSum, &r.SigSq, &mx); err != nil {
			return nil, nil, err
		}
		r.Channels = orMasks(masks)
		r.SigMax, r.HasMax = int(mx.Int64), mx.Valid
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	air, err := s.airDays(ctx, sinceDay)
	return out, air, err
}

// orMasks joins the channel masks of a device's days (a comma-separated list, as group_concat gives it) into one.
func orMasks(list string) uint64 {
	var out uint64
	for _, f := range strings.Split(list, ",") {
		if n, err := strconv.ParseInt(f, 10, 64); err == nil {
			out |= uint64(n)
		}
	}
	return out
}

func (s *Store) airDays(ctx context.Context, sinceDay int64) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT day, cycles FROM air_days WHERE day >= ?`, sinceDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var d int64
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		out[d] = n
	}
	return out, rows.Err()
}

// SetClass gives a device a computed class under a rules version and logs the change. A device whose class the owner set (class_ver 0 and a
// class) is never touched. It reports whether the class changed.
func (s *Store) SetClass(ctx context.Context, tag []byte, owner bool, from, to string, version int, reason string, at time.Time) error {
	return s.write(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		res, err := tx.ExecContext(ctx, `UPDATE devices SET class = ?, class_ver = ? WHERE tag = ? AND NOT (class != '' AND class_ver = 0)`, to, version, tag)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		if from != to {
			if _, err := tx.ExecContext(ctx, `INSERT INTO class_log(day, tag, owner, from_class, to_class, version, reason) VALUES(?,?,?,?,?,?,?)`,
				dayNumber(at), tag, owner, from, to, version, reason); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// ClassCounts counts the devices of other people by class.
func (s *Store) ClassCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT class, count(*) FROM devices WHERE owner = 0 GROUP BY class`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var c string
		var n int
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		out[c] = n
	}
	return out, rows.Err()
}

// ClassChange is one row of the class log as shown: a count of changes of one kind on a day, never a device of someone else.
type ClassChange struct {
	Day     string `json:"day"`
	From    string `json:"from"`
	To      string `json:"to"`
	Version int    `json:"version"`
	Reason  string `json:"reason"`
	N       int    `json:"n"`
}

// RecentClassChanges lists the changes of the last days, newest first, grouped so no neighbour's device is singled out.
func (s *Store) RecentClassChanges(ctx context.Context, sinceDay int64, limit int) ([]ClassChange, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT day, from_class, to_class, version, reason, count(*) FROM class_log WHERE day >= ?
		GROUP BY day, from_class, to_class, version, reason ORDER BY day DESC, count(*) DESC LIMIT ?`, sinceDay, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ClassChange{}
	for rows.Next() {
		var c ClassChange
		var d int64
		if err := rows.Scan(&d, &c.From, &c.To, &c.Version, &c.Reason, &c.N); err != nil {
			return nil, err
		}
		c.Day = dayTime(d).Format("2006-01-02")
		out = append(out, c)
	}
	return out, rows.Err()
}

// PutDoc and GetDoc keep a small named document (the last probe of an adapter, the classifier's rules by version).
func (s *Store) PutDoc(ctx context.Context, key string, v any, at time.Time) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.write(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `INSERT INTO docs(key, val, day) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET val = excluded.val, day = excluded.day`, key, raw, dayNumber(at))
		return err
	})
}

// GetDoc reads a document into v and says whether there was one.
func (s *Store) GetDoc(ctx context.Context, key string, v any) (bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT val FROM docs WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(raw, v)
}

// MetaInt reads a number from the meta table (0 when absent), and SetMetaInt writes one.
func (s *Store) MetaInt(ctx context.Context, key string) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT val FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

func (s *Store) SetMetaInt(ctx context.Context, key string, v int) error {
	return s.write(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `INSERT INTO meta(key, val) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET val = excluded.val`, key, v)
		return err
	})
}

// OwnAP is one of the owner's access points as the store keeps it.
type OwnAP struct {
	Tag       []byte
	Sealed    []byte
	Name      string
	Confirmed bool
	Channel   int
	Security  string
	// PMF is whether the access point protects its management frames: off, capable or required ("" until a beacon said).
	PMF      string
	FirstDay int64
	LastDay  int64
}

// ProposeOwnAP records an access point that broadcasts one of the owner's network names. It stays a proposal until the owner confirms it: anyone
// can broadcast the same name. A confirmed one is never turned back into a proposal by a later sighting.
func (s *Store) ProposeOwnAP(ctx context.Context, a OwnAP, at time.Time) error {
	d := dayNumber(at)
	return s.write(ctx, func() error {
		_, err := s.db.ExecContext(ctx, `INSERT INTO own_aps(tag, sealed, name, confirmed, channel, security, pmf, first_day, last_day) VALUES(?,?,?,0,?,?,?,?,?)
			ON CONFLICT(tag) DO UPDATE SET last_day = max(last_day, excluded.last_day), channel = excluded.channel, security = excluded.security, pmf = excluded.pmf`,
			a.Tag, a.Sealed, a.Name, a.Channel, a.Security, a.PMF, d, d)
		return err
	})
}

// OwnAPs lists the owner's access points, confirmed first.
func (s *Store) OwnAPs(ctx context.Context) ([]OwnAP, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tag, sealed, name, confirmed, channel, security, pmf, first_day, last_day FROM own_aps ORDER BY confirmed DESC, name, first_day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OwnAP
	for rows.Next() {
		var a OwnAP
		if err := rows.Scan(&a.Tag, &a.Sealed, &a.Name, &a.Confirmed, &a.Channel, &a.Security, &a.PMF, &a.FirstDay, &a.LastDay); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetOwnAPConfirmed confirms (or, with false, withdraws) one access point and says whether there was one.
func (s *Store) SetOwnAPConfirmed(ctx context.Context, tag []byte, yes bool) (bool, error) {
	var found bool
	err := s.write(ctx, func() error {
		res, err := s.db.ExecContext(ctx, `UPDATE own_aps SET confirmed = ? WHERE tag = ?`, yes, tag)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		found = n > 0
		return nil
	})
	return found, err
}

// DeleteOwnAP removes an access point the owner says is not theirs.
func (s *Store) DeleteOwnAP(ctx context.Context, tag []byte) (bool, error) {
	var found bool
	err := s.write(ctx, func() error {
		res, err := s.db.ExecContext(ctx, `DELETE FROM own_aps WHERE tag = ?`, tag)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		found = n > 0
		return nil
	})
	return found, err
}

// firstChannelOf is the first channel a device has been heard on in the days kept (0 when none), which the distance model needs to pick a band.
func (s *Store) firstChannelOf(ctx context.Context, tag []byte) (int, error) {
	var mask sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT channels FROM daily WHERE tag = ? ORDER BY day DESC LIMIT 1`, tag).Scan(&mask)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return firstChannel(uint64(mask.Int64)), err
}
