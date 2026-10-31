package netaudit

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"llm-proxy/models"
)

// ErrNoConnector is what sending an alert returns when no connector is set up, so the alert stays pending and the log can
// say so.
var ErrNoConnector = errors.New("no connector is set up to send alerts")

const (
	maxAlerts = 50
	// repeatWindow is how long a repeating decoy alert for the same sources keeps updating one entry.
	repeatWindow = time.Hour
)

// AlertStore keeps the log, oldest first. A missing file is none.
type AlertStore interface {
	Read() ([]models.NetworkAlert, error)
	Write([]models.NetworkAlert) error
}

// AlertLog is the bounded log of the alerts the Network module raises, so the web UI shows them whether or not a connector
// exists. It holds the exact text sent to connectors and nothing else.
type AlertLog struct {
	store   AlertStore
	now     func() time.Time
	onError func(error)
	mu      sync.Mutex
}

// NewAlertLog builds the log. onError (optional) hears a log failure, which never stops an alert from being sent.
func NewAlertLog(store AlertStore, now func() time.Time, onError func(error)) *AlertLog {
	return &AlertLog{store: store, now: now, onError: onError}
}

// Send logs the alert before anything is sent, hands it to send, and records what became of it, so a missing connector no
// longer hides the alert from the UI. Retrying an alert that was not sent reuses its entry, and a decoy alert repeating for
// the same sources within an hour updates one entry (a count and the latest time) rather than adding rows. A log that
// cannot be read or written is reported and never stops the send. The error is send's own.
func (l *AlertLog) Send(ctx context.Context, kind models.NetworkAlertKind, message string, send func(context.Context, string) error) error {
	id := l.record(kind, message)
	err := send(ctx, message)
	if id != 0 {
		l.settle(id, deliveryOf(err))
	}
	return err
}

func deliveryOf(err error) models.NetworkAlertDelivery {
	switch {
	case err == nil:
		return models.NetworkAlertSent
	case errors.Is(err, ErrNoConnector):
		return models.NetworkAlertNoConnector
	}
	return models.NetworkAlertFailed
}

// record adds the entry for an alert about to be sent, or finds the one to reuse, and returns its ID (0 when the log
// could not be written).
func (l *AlertLog) record(kind models.NetworkAlertKind, message string) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	items, err := l.store.Read()
	if err != nil {
		l.report(err)
		return 0
	}
	now := l.now()
	var id int64
	if i := lastOfKind(items, kind); i >= 0 && items[i].Message == message {
		prev := &items[i]
		switch {
		case prev.Delivery != models.NetworkAlertSent:
			// A retry of what was never sent: the same alert, not a new one.
			return prev.ID
		case kind == models.NetworkAlertDecoy && now.Sub(prev.At) < repeatWindow:
			prev.Count++
			prev.At, prev.Delivery, prev.ID = now, models.NetworkAlertSending, nextAlertID(items, now)
			id = prev.ID
		}
	}
	if id == 0 {
		id = nextAlertID(items, now)
		items = append(items, models.NetworkAlert{ID: id, Kind: kind, Message: message, At: now, Delivery: models.NetworkAlertSending, Count: 1})
	}
	l.save(items)
	return id
}

// settle records the outcome. An outcome that differs from the entry's gives it a new ID, so a client whose cursor passed
// the entry sees it again with the outcome (an entry still being sent is not shown).
func (l *AlertLog) settle(id int64, delivery models.NetworkAlertDelivery) {
	l.mu.Lock()
	defer l.mu.Unlock()
	items, err := l.store.Read()
	if err != nil {
		l.report(err)
		return
	}
	i := slices.IndexFunc(items, func(a models.NetworkAlert) bool { return a.ID == id })
	if i < 0 || items[i].Delivery == delivery {
		return
	}
	items[i].Delivery, items[i].ID = delivery, nextAlertID(items, l.now())
	l.save(items)
}

// save keeps the newest maxAlerts entries in ID order.
func (l *AlertLog) save(items []models.NetworkAlert) {
	slices.SortFunc(items, func(a, b models.NetworkAlert) int { return cmp.Compare(a.ID, b.ID) })
	if len(items) > maxAlerts {
		items = items[len(items)-maxAlerts:]
	}
	if err := l.store.Write(items); err != nil {
		l.report(err)
	}
}

func (l *AlertLog) report(err error) {
	if l.onError != nil {
		l.onError(fmt.Errorf("network alert log: %w", err))
	}
}

// lastOfKind is the index of the newest entry of a kind, or -1.
func lastOfKind(items []models.NetworkAlert, kind models.NetworkAlertKind) int {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Kind == kind {
			return i
		}
	}
	return -1
}

// nextAlertID is the time in unix milliseconds, pushed past the highest ID so IDs rise strictly even when two alerts share
// a millisecond or the clock steps back.
func nextAlertID(items []models.NetworkAlert, now time.Time) int64 {
	id := now.UnixMilli()
	for _, a := range items {
		id = max(id, a.ID+1)
	}
	return id
}

// After returns the finished entries with an ID above id, newest first, at most limit (the newest when more exist).
func (l *AlertLog) After(id int64, limit int) ([]models.NetworkAlert, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	items, err := l.store.Read()
	if err != nil {
		return nil, fmt.Errorf("network alert log: %w", err)
	}
	out := []models.NetworkAlert{}
	for i := len(items) - 1; i >= 0 && len(out) < limit; i-- {
		if items[i].ID > id && items[i].Delivery != models.NetworkAlertSending {
			out = append(out, items[i])
		}
	}
	return out, nil
}

// Latest is the newest finished entry's ID, or 0 for an empty log: the cursor a client that has just opened starts from.
func (l *AlertLog) Latest() (int64, error) {
	items, err := l.After(0, 1)
	if err != nil || len(items) == 0 {
		return 0, err
	}
	return items[0].ID, nil
}
