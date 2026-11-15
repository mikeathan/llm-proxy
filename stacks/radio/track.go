package radio

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"llm-proxy-stacks/sdk"
)

// The optional signal track on disk (ADR-0055): OFF by default, 24 or 48 hours when the owner turns it on. A point is the keyed tag, the time of the
// cycle, the mean signal and the first channel, never an address or a name. Turning it off and "forget neighbours" delete every point.
const (
	// KeepPath reads (GET) and sets (PUT) how long the track is kept, and deletes the stored points (DELETE).
	KeepPath = "/admin/api/radio/keeping"

	// MaxTrackHours is the ceiling in code: a longer age needs a new ADR, it is not a setting.
	MaxTrackHours = 48
	// MaxTrackRows is the most points the table holds; the oldest go first.
	MaxTrackRows = 300000
)

// trackHourChoices are the only values the setting takes: off, one day, two days.
var trackHourChoices = []int{0, 24, 48}

// trackRow is one device in one cycle.
type trackRow struct {
	tag     []byte
	sig     int
	channel int
}

// AddTrack writes one cycle's points in one transaction.
func (s *Store) AddTrack(ctx context.Context, at time.Time, rows []trackRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	st, err := tx.PrepareContext(ctx, `INSERT INTO air_track(tag, at, sig, channel) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	for _, r := range rows {
		if _, err := st.ExecContext(ctx, r.tag, at.Unix(), r.sig, r.channel); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TrackPoints is one device's points since a time, oldest first.
func (s *Store) TrackPoints(ctx context.Context, tag []byte, since time.Time) ([]trackPt, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT at, sig, channel FROM air_track WHERE tag = ? AND at >= ? ORDER BY at ASC`, tag, since.Unix())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []trackPt
	for rows.Next() {
		var p trackPt
		var sig int
		if err := rows.Scan(&p.at, &sig, &p.channel); err != nil {
			return nil, err
		}
		p.signal = float64(sig)
		out = append(out, p)
	}
	return out, rows.Err()
}

// PruneTracks removes the points past their age (never more than MaxTrackHours) and, when the table is over its row cap, the oldest ones.
func (s *Store) PruneTracks(ctx context.Context, now time.Time, hours int) error {
	hours = min(max(hours, 0), MaxTrackHours)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM air_track WHERE at < ?`, now.Add(-time.Duration(hours)*time.Hour).Unix()); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM air_track WHERE rowid IN (SELECT rowid FROM air_track ORDER BY at ASC
		LIMIT max((SELECT count(*) FROM air_track) - ?, 0))`, s.trackRowCap)
	return err
}

// DeleteTracks removes every stored point and gives the pages back.
func (s *Store) DeleteTracks(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM air_track`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "PRAGMA incremental_vacuum")
	return err
}

// trackUse is how many points are kept and the time of the oldest (zero when none).
func (s *Store) trackUse(ctx context.Context) (int, time.Time, error) {
	var n int
	var oldest sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT count(*), min(at) FROM air_track`).Scan(&n, &oldest); err != nil {
		return 0, time.Time{}, err
	}
	if !oldest.Valid {
		return n, time.Time{}, nil
	}
	return n, time.Unix(oldest.Int64, 0).UTC(), nil
}

// TagByShortID is the tag of the device a short id stands for, or nil.
func (s *Store) TagByShortID(ctx context.Context, id string) ([]byte, error) {
	b, err := hex.DecodeString(id)
	if err != nil || len(b) != shortIDBytes {
		return nil, nil
	}
	var tag []byte
	err = s.db.QueryRowContext(ctx, `SELECT tag FROM devices WHERE substr(tag, 1, ?) = ? LIMIT 1`, shortIDBytes, b).Scan(&tag)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return tag, err
}

// SetTrackHours tells the recorder how long the owner keeps tracks; 0 stops writing them.
func (r *Recorder) SetTrackHours(h int) { r.trackHours.Store(int32(h)) }

// keepTrack writes the finished cycle's points when the owner keeps tracks, and prunes what is past its age.
func (r *Recorder) keepTrack(ctx context.Context, at time.Time, heard map[string]*agg) error {
	hours := int(r.trackHours.Load())
	if hours <= 0 {
		return nil
	}
	rows := make([]trackRow, 0, len(heard))
	for tag, a := range heard {
		if a.n > 0 {
			rows = append(rows, trackRow{tag: []byte(tag), sig: int(float64(a.sum) / float64(a.n)), channel: firstChannel(a.channels)})
		}
	}
	if err := r.store.AddTrack(ctx, at, rows); err != nil {
		return err
	}
	return r.store.PruneTracks(ctx, at, hours)
}

// KeepView is the answer of KeepPath.
type KeepView struct {
	// TrackHours is how long a track is kept: 0 is off. Choices are the only values accepted.
	TrackHours   int       `json:"track_hours"`
	Choices      []int     `json:"choices"`
	StoredPoints int       `json:"stored_points"`
	Oldest       time.Time `json:"oldest,omitzero"`
}

func (s *Stack) keepView(ctx context.Context, h stacksdk.Host, store *Store) (stacksdk.Response, error) {
	n, oldest, err := store.trackUse(ctx)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the stored tracks could not be read")
	}
	return jsonResponse(http.StatusOK, KeepView{TrackHours: s.settings(ctx, h).TrackHours, Choices: trackHourChoices, StoredPoints: n, Oldest: oldest})
}

// setKeep saves the choice; off deletes every stored point in the same request.
func (s *Stack) setKeep(ctx context.Context, h stacksdk.Host, rec *Recorder, store *Store, body []byte) (stacksdk.Response, error) {
	var b struct {
		TrackHours int `json:"track_hours"`
	}
	if err := strictDecode(body, &b); err != nil {
		return plainError(http.StatusBadRequest, "send track_hours: 0, 24 or 48")
	}
	ok := false
	for _, c := range trackHourChoices {
		ok = ok || c == b.TrackHours
	}
	if !ok {
		return plainError(http.StatusBadRequest, "tracks are kept for 24 or 48 hours, or not at all")
	}
	if h.Config == nil {
		return plainError(http.StatusConflict, "the settings are not available to the Radio stack")
	}
	doc, _ := jsonBytes(map[string]any{"track_hours": b.TrackHours})
	if err := h.Config.Save(ctx, doc); err != nil {
		return plainError(http.StatusInternalServerError, "the choice could not be saved")
	}
	rec.SetTrackHours(b.TrackHours)
	if b.TrackHours == 0 {
		if err := store.DeleteTracks(ctx); err != nil {
			return plainError(http.StatusInternalServerError, "the stored tracks could not be deleted")
		}
	} else if err := store.PruneTracks(ctx, s.now(), b.TrackHours); err != nil {
		return plainError(http.StatusInternalServerError, "the stored tracks could not be pruned")
	}
	return s.keepView(ctx, h, store)
}

// dropTracks deletes the stored points and leaves the choice as it is.
func (s *Stack) dropTracks(ctx context.Context, h stacksdk.Host, store *Store) (stacksdk.Response, error) {
	if err := store.DeleteTracks(ctx); err != nil {
		return plainError(http.StatusInternalServerError, "the stored tracks could not be deleted")
	}
	return s.keepView(ctx, h, store)
}

// pruneEvery is how often retention runs: the stack has no goroutine of its own, so it runs when the page is read or a run starts.
const pruneEvery = time.Hour

// pruneIfDue applies the age limits (ADR-0053's 14 and 30 days, ADR-0055's track age) at most once an hour, so data does not outlive its limit
// while nothing is listening. A failure is logged by the caller's next read; it never blocks an answer.
func (s *Stack) pruneIfDue(ctx context.Context, store *Store, hours int) {
	s.mu.Lock()
	due := s.now().Sub(s.lastPrune) >= pruneEvery
	if due {
		s.lastPrune = s.now()
	}
	s.mu.Unlock()
	if !due {
		return
	}
	_, _ = store.Prune(ctx, s.now())
	_ = store.PruneTracks(ctx, s.now(), hours)
}
