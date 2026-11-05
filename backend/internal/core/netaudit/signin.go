package netaudit

import (
	"context"
	"sync"
	"time"
)

// The sign-in events the operator is told about (Constitution I.5: the admin interface can run code as this program's user, so a
// lockout or an unknown sign-in matters). The message carries no address: the Security page lists the events with theirs.
const (
	SignInLockedOut      = "locked"
	SignInFromNewAddress = "new_address"
)

// signInQueue is how many events may wait for the loop.
const signInQueue = 8

// signInAlertEvery is the least time between two alerts of one kind, so a guessing run raises one alert and not one per lock.
const signInAlertEvery = 5 * time.Minute

var signInMessages = map[string]string{
	SignInLockedOut:      "Security: an address was locked out of the admin sign-in after repeated wrong passphrases. If it was not you, someone on the network is guessing. Open Settings, Sign-in & access to see the sign-in events.",
	SignInFromNewAddress: "Security: someone signed in to the admin interface from an address that has never signed in before. If it was not you, open Settings, Sign-in & access, end that session and change the passphrase.",
}

// SignInAlerts takes the sign-in events from the request path without waiting and sends them from its own loop, paced per kind.
type SignInAlerts struct {
	notify func(ctx context.Context, message string) error
	now    func() time.Time
	queue  chan string

	mu   sync.Mutex
	last map[string]time.Time
}

func NewSignInAlerts(notify func(ctx context.Context, message string) error, now func() time.Time) *SignInAlerts {
	return &SignInAlerts{notify: notify, now: now, queue: make(chan string, signInQueue), last: map[string]time.Time{}}
}

// Tell queues one event and returns at once. An event of a kind told within the window, or one that finds the queue full, is
// dropped: the alert already said it.
func (a *SignInAlerts) Tell(kind string) {
	if _, known := signInMessages[kind]; !known {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if last, ok := a.last[kind]; ok && now.Sub(last) < signInAlertEvery {
		return
	}
	select {
	case a.queue <- kind:
		a.last[kind] = now // paced from the alert that was queued, so a dropped one does not silence the next
	default:
	}
}

// Run sends the queued events until ctx ends. It is started from the one place the Network loops start (Constitution II.2,
// II.14). A send that fails stays in the alert log as not sent, like every other alert.
func (a *SignInAlerts) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case kind := <-a.queue:
			_ = a.notify(ctx, signInMessages[kind])
		}
	}
}
