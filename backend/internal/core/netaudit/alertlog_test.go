package netaudit

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"llm-proxy/models"
)

type memAlertStore struct {
	items    []models.NetworkAlert
	writeErr error
}

func (s *memAlertStore) Read() ([]models.NetworkAlert, error) {
	return append([]models.NetworkAlert(nil), s.items...), nil
}
func (s *memAlertStore) Write(items []models.NetworkAlert) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.items = append([]models.NetworkAlert(nil), items...)
	return nil
}

type alertRig struct {
	log   *AlertLog
	store *memAlertStore
	now   time.Time
	errs  []error
}

func newAlertRig() *alertRig {
	r := &alertRig{store: &memAlertStore{}, now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	r.log = NewAlertLog(r.store, func() time.Time { return r.now }, func(err error) { r.errs = append(r.errs, err) })
	return r
}

func ok(context.Context, string) error { return nil }

func TestAlertLog_Send(t *testing.T) {
	ctx := context.Background()
	t.Run("each kind adds one entry holding the exact text and a sent mark", func(t *testing.T) {
		r := newAlertRig()
		for _, k := range []models.NetworkAlertKind{models.NetworkAlertPresence, models.NetworkAlertDecoy, models.NetworkAlertSummary} {
			if err := r.log.Send(ctx, k, "text "+string(k), ok); err != nil {
				t.Fatal(err)
			}
		}
		if len(r.store.items) != 3 {
			t.Fatalf("want 3 entries, got %d", len(r.store.items))
		}
		for _, a := range r.store.items {
			if a.Message != "text "+string(a.Kind) || a.Delivery != models.NetworkAlertSent || a.Count != 1 || !a.At.Equal(r.now) {
				t.Errorf("unexpected entry %+v", a)
			}
		}
	})
	t.Run("the entry exists and says so when no connector is set up", func(t *testing.T) {
		r := newAlertRig()
		err := r.log.Send(ctx, models.NetworkAlertDecoy, "m", func(context.Context, string) error { return fmt.Errorf("wrapped: %w", ErrNoConnector) })
		if !errors.Is(err, ErrNoConnector) {
			t.Fatalf("the send's error must reach the caller, got %v", err)
		}
		if len(r.store.items) != 1 || r.store.items[0].Delivery != models.NetworkAlertNoConnector {
			t.Fatalf("got %+v", r.store.items)
		}
	})
	t.Run("another failure is marked failed and keeps no reason", func(t *testing.T) {
		r := newAlertRig()
		_ = r.log.Send(ctx, models.NetworkAlertPresence, "m", func(context.Context, string) error { return errors.New("secret detail") })
		if r.store.items[0].Delivery != models.NetworkAlertFailed {
			t.Fatalf("got %+v", r.store.items[0])
		}
	})
	t.Run("a retry of an unsent alert reuses its entry and the success updates it", func(t *testing.T) {
		r := newAlertRig()
		_ = r.log.Send(ctx, models.NetworkAlertPresence, "m", func(context.Context, string) error { return ErrNoConnector })
		r.now = r.now.Add(time.Minute)
		_ = r.log.Send(ctx, models.NetworkAlertPresence, "m", func(context.Context, string) error { return ErrNoConnector })
		if len(r.store.items) != 1 {
			t.Fatalf("a retry must not add a row, got %d", len(r.store.items))
		}
		_ = r.log.Send(ctx, models.NetworkAlertPresence, "m", ok)
		if len(r.store.items) != 1 || r.store.items[0].Delivery != models.NetworkAlertSent || r.store.items[0].Count != 1 {
			t.Fatalf("got %+v", r.store.items)
		}
	})
	t.Run("a larger pending set is a new entry", func(t *testing.T) {
		r := newAlertRig()
		_ = r.log.Send(ctx, models.NetworkAlertPresence, "1 new device", func(context.Context, string) error { return ErrNoConnector })
		_ = r.log.Send(ctx, models.NetworkAlertPresence, "2 new devices", func(context.Context, string) error { return ErrNoConnector })
		if len(r.store.items) != 2 {
			t.Fatalf("got %d", len(r.store.items))
		}
	})
	t.Run("a sent presence alert with the same text later is a new row", func(t *testing.T) {
		r := newAlertRig()
		_ = r.log.Send(ctx, models.NetworkAlertPresence, "1 new device", ok)
		_ = r.log.Send(ctx, models.NetworkAlertPresence, "1 new device", ok)
		if len(r.store.items) != 2 {
			t.Fatalf("got %d", len(r.store.items))
		}
	})
	t.Run("a log that cannot be written is reported and the alert is still sent", func(t *testing.T) {
		r := newAlertRig()
		r.store.writeErr = errors.New("disk full")
		sent := false
		err := r.log.Send(ctx, models.NetworkAlertSummary, "m", func(context.Context, string) error { sent = true; return nil })
		if err != nil || !sent || len(r.errs) == 0 {
			t.Fatalf("err=%v sent=%v reported=%v", err, sent, r.errs)
		}
	})
}

func TestAlertLog_RepeatingDecoySource(t *testing.T) {
	ctx := context.Background()
	t.Run("the same sources within the hour update one entry with a count and a newer id", func(t *testing.T) {
		r := newAlertRig()
		_ = r.log.Send(ctx, models.NetworkAlertDecoy, "decoy 192.168.1.9", ok)
		first := r.store.items[0]
		r.now = r.now.Add(5 * time.Minute)
		_ = r.log.Send(ctx, models.NetworkAlertDecoy, "decoy 192.168.1.9", ok)
		if len(r.store.items) != 1 {
			t.Fatalf("got %d rows", len(r.store.items))
		}
		got := r.store.items[0]
		if got.Count != 2 || got.ID <= first.ID || !got.At.Equal(r.now) || got.Delivery != models.NetworkAlertSent {
			t.Fatalf("got %+v after %+v", got, first)
		}
	})
	t.Run("an hour later it is a new row", func(t *testing.T) {
		r := newAlertRig()
		_ = r.log.Send(ctx, models.NetworkAlertDecoy, "decoy 192.168.1.9", ok)
		r.now = r.now.Add(time.Hour + time.Minute)
		_ = r.log.Send(ctx, models.NetworkAlertDecoy, "decoy 192.168.1.9", ok)
		if len(r.store.items) != 2 {
			t.Fatalf("got %d rows", len(r.store.items))
		}
	})
	t.Run("different sources are a new row", func(t *testing.T) {
		r := newAlertRig()
		_ = r.log.Send(ctx, models.NetworkAlertDecoy, "decoy 192.168.1.9", ok)
		_ = r.log.Send(ctx, models.NetworkAlertDecoy, "decoy 192.168.1.10", ok)
		if len(r.store.items) != 2 {
			t.Fatalf("got %d rows", len(r.store.items))
		}
	})
}

func TestAlertLog_BoundsAndOrder(t *testing.T) {
	ctx := context.Background()
	r := newAlertRig()
	for i := range maxAlerts + 5 {
		_ = r.log.Send(ctx, models.NetworkAlertPresence, fmt.Sprintf("alert %d", i), ok)
	}
	if len(r.store.items) != maxAlerts {
		t.Fatalf("the log is capped at %d, got %d", maxAlerts, len(r.store.items))
	}
	if r.store.items[0].Message != "alert 5" {
		t.Errorf("the oldest must go first, kept %q", r.store.items[0].Message)
	}
	for i := 1; i < len(r.store.items); i++ {
		if r.store.items[i].ID <= r.store.items[i-1].ID {
			t.Fatalf("ids must rise strictly with a frozen clock: %d then %d", r.store.items[i-1].ID, r.store.items[i].ID)
		}
	}
	t.Run("After returns newer entries newest first, bounded by limit", func(t *testing.T) {
		cut := r.store.items[maxAlerts-4].ID
		got, err := r.log.After(cut, 20)
		if err != nil || len(got) != 3 || got[0].Message != "alert 54" || got[2].Message != "alert 52" {
			t.Fatalf("err=%v got=%+v", err, got)
		}
		got, _ = r.log.After(0, 2)
		if len(got) != 2 || got[0].Message != "alert 54" || got[1].Message != "alert 53" {
			t.Fatalf("the limit keeps the newest, got %+v", got)
		}
	})
	t.Run("Latest is the newest id, and 0 when empty", func(t *testing.T) {
		if id, _ := r.log.Latest(); id != r.store.items[maxAlerts-1].ID {
			t.Errorf("got %d", id)
		}
		if id, _ := newAlertRig().log.Latest(); id != 0 {
			t.Errorf("got %d", id)
		}
	})
}

func TestAlertLog_VisibilityWhileSending(t *testing.T) {
	r := newAlertRig()
	var during []models.NetworkAlert
	_ = r.log.Send(context.Background(), models.NetworkAlertPresence, "m", func(context.Context, string) error {
		during, _ = r.log.After(0, 20)
		return ErrNoConnector
	})
	if len(during) != 0 {
		t.Errorf("an entry still being sent must not be shown, got %+v", during)
	}
	after, _ := r.log.After(0, 20)
	if len(after) != 1 || after[0].Delivery != models.NetworkAlertNoConnector {
		t.Fatalf("got %+v", after)
	}
	seen := after[0].ID
	_ = r.log.Send(context.Background(), models.NetworkAlertPresence, "m", func(context.Context, string) error { return ErrNoConnector })
	if again, _ := r.log.After(seen, 20); len(again) != 0 {
		t.Errorf("a retry that fails the same way must not show a second time, got %+v", again)
	}
	_ = r.log.Send(context.Background(), models.NetworkAlertPresence, "m", ok)
	if again, _ := r.log.After(seen, 20); len(again) != 1 || again[0].Delivery != models.NetworkAlertSent {
		t.Errorf("a changed outcome must reach a client whose cursor passed the entry, got %+v", again)
	}
}
