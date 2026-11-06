package netaudit

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"llm-proxy/models"
)

// The login watch: failed logins to this computer, counted from the system's own record (platform/loginlog) and nothing more. It tells
// the operator, in fixed words, of a burst, and adds a finding to the nightly report. No user name and no address is read, kept or
// shown (Constitution I.10, proposed): a burst is only a number of failures within a time. Only a program with write access to the
// record could make one up, which is stronger than the router's log, where any device can.
const (
	// loginBurstFailures failed logins within loginBurstWindow are a burst. The figure is a starting point with no attack data behind it:
	// the owner's real record (2026-11-06) holds two failures, both of their own. Tune it after a week of real data.
	loginBurstFailures = 10
	loginBurstWindow   = 10 * time.Minute
	// loginAlertGap is the least time between two messages; a burst seen meanwhile is told when it has passed.
	loginAlertGap = time.Hour
	// loginWatchEvery is how often the watch looks.
	loginWatchEvery = time.Minute
	// loginDay is how far back the nightly count looks.
	loginDay = 24 * time.Hour
	// loginFutureSlack is how far past now a record's time may be and still count (clocks differ a little); anything later is ignored.
	loginFutureSlack = 5 * time.Minute

	CheckHostLoginFailures = "host_login_failures"
)

func init() {
	checkTitles[CheckHostLoginFailures] = "Someone failed to log in to this computer again and again"
	remediations[CheckHostLoginFailures] = "If that was not you, someone is guessing a password for this computer. Turn off password login for SSH and use keys only, and keep the SSH port off the internet. The finding for SSH password login, if there is one, has the steps."
	guides[CheckHostLoginFailures] = FixGuide{
		Steps: []string{
			"Check whether you or a program of yours was the one failing to log in (a wrong password or an old saved one).",
			"If not, turn off password login in the SSH settings (PasswordAuthentication no) and log in with a key instead.",
			"Make sure the router does not forward the SSH port to the internet, and look at the Internet view for a forwarded port.",
		},
		Risk: "Turning off password login locks out anyone who has no key yet, so set up your key and test it in a second window first.",
		Undo: "Set PasswordAuthentication back to yes and restart SSH, or accept this finding to say it is understood.",
	}
}

// LoginWatchDeps are what the watch needs. Read gives the times of the failed logins after a time, oldest first; Notify sends one
// message through the alert log and fails when it could not be delivered, so it is tried again at the next look.
type LoginWatchDeps struct {
	Read    func(after time.Time) ([]time.Time, error)
	Enabled func() bool
	Notify  func(ctx context.Context, message string) error
	OnError func(error)
	Now     func() time.Time
	// Cap is the most records one read looks at (the reader's bound); a day's count that reaches it is "at least" that many.
	Cap int
}

// LoginWatch looks at the system's record of failed logins while it is switched on. It keeps only the moment it last told, when it last
// read the record, and the last error: memory does not grow with the number of failures.
type LoginWatch struct {
	deps LoginWatchDeps

	mu       sync.Mutex
	told     time.Time
	lastGood time.Time
	failed   bool
	lastErr  string
}

func NewLoginWatch(deps LoginWatchDeps) *LoginWatch {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &LoginWatch{deps: deps}
}

// LoginSnapshot is what the nightly report reads: the failed logins of the last day and how many bursts they held.
type LoginSnapshot struct {
	Failures int
	Bursts   int
	// Capped says the read stopped at its bound, so Failures is a lower limit.
	Capped bool
}

// Run looks every minute until ctx ends.
func (w *LoginWatch) Run(ctx context.Context) {
	tick := time.NewTicker(loginWatchEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			w.Tick(ctx)
		}
	}
}

// Tick reads the last few minutes of the record and tells of a burst in them, unless one was told within the gap. The burst is marked
// told only after the message was delivered. A record that cannot be read is a failed part, said once, never a burst.
func (w *LoginWatch) Tick(ctx context.Context) {
	if !w.deps.Enabled() {
		return
	}
	now := w.deps.Now()
	times, ok := w.read(now.Add(-loginBurstWindow), now)
	if !ok || len(burstsIn(times, loginBurstFailures, loginBurstWindow)) == 0 {
		return
	}
	w.mu.Lock()
	held := !w.told.IsZero() && now.Sub(w.told) < loginAlertGap
	w.mu.Unlock()
	if held || ctx.Err() != nil {
		return
	}
	if err := w.deps.Notify(ctx, loginBurstMessage()); err != nil {
		w.say(fmt.Errorf("failed to send the login alert: %w", err))
		return
	}
	w.mu.Lock()
	w.told = now
	w.mu.Unlock()
}

// read returns the failures after a time that are not from the future, and records whether the record could be read.
func (w *LoginWatch) read(after, now time.Time) ([]time.Time, bool) {
	times, err := w.deps.Read(after)
	if err != nil {
		w.mu.Lock()
		w.failed = true
		w.mu.Unlock()
		w.say(w.explain(err))
		return nil, false
	}
	w.mu.Lock()
	w.failed, w.lastGood, w.lastErr = false, now, ""
	w.mu.Unlock()
	kept := times[:0:0]
	for _, at := range times {
		if !at.After(now.Add(loginFutureSlack)) {
			kept = append(kept, at)
		}
	}
	return kept, true
}

// explain says what a read error means in the operator's words.
func (w *LoginWatch) explain(err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return errors.New("this program may not read the system's record of failed logins; the service needs the utmp group (see the docs for the unit)")
	}
	return err
}

// say reports an error once, until a different one or a good read.
func (w *LoginWatch) say(err error) {
	w.mu.Lock()
	same := w.lastErr == err.Error()
	w.lastErr = err.Error()
	w.mu.Unlock()
	if !same && w.deps.OnError != nil {
		w.deps.OnError(err)
	}
}

// Snapshot is the last day's failures and bursts, read from the record itself so a restart loses nothing. It is empty while the watch is
// switched off or the record cannot be read.
func (w *LoginWatch) Snapshot(now time.Time) LoginSnapshot {
	if !w.deps.Enabled() {
		return LoginSnapshot{}
	}
	times, ok := w.read(now.Add(-loginDay), now)
	if !ok {
		return LoginSnapshot{}
	}
	return LoginSnapshot{Failures: len(times), Bursts: len(burstsIn(times, loginBurstFailures, loginBurstWindow)), Capped: w.deps.Cap > 0 && len(times) >= w.deps.Cap}
}

// Reading is the part's health for the monitor: failed while the record cannot be read, working once it has been.
func (w *LoginWatch) Reading() MonitorReading {
	w.mu.Lock()
	defer w.mu.Unlock()
	return MonitorReading{On: w.deps.Enabled(), LastGood: w.lastGood, Limit: 15 * time.Minute, Failed: w.failed}
}

// burstsIn returns the moment each burst ended: n failures within the window, the next burst taking n new failures. times are oldest first.
func burstsIn(times []time.Time, n int, window time.Duration) []time.Time {
	var out []time.Time
	for start := 0; start+n <= len(times); {
		if times[start+n-1].Sub(times[start]) <= window {
			out = append(out, times[start+n-1])
			start += n
			continue
		}
		start++
	}
	return out
}

func loginBurstMessage() string {
	return fmt.Sprintf("This computer's record shows %d or more failed logins within %d minutes. If that was not you, someone is guessing a password: turn off password login for SSH and check that its port is not open to the internet.", loginBurstFailures, int(loginBurstWindow.Minutes()))
}

// LoginFindings turns a snapshot into the nightly finding, keyed by this computer. A day of single failures is not a finding: a
// mistyped password is one record, so only a burst is.
func LoginFindings(s LoginSnapshot) []Finding {
	if s.Bursts == 0 {
		return nil
	}
	total := fmt.Sprintf("%d failures in all", s.Failures)
	if s.Capped {
		total = fmt.Sprintf("at least %d failures in all", s.Failures)
	}
	return []Finding{{Device: HostDevice, Check: CheckHostLoginFailures, Severity: SeverityMedium,
		Evidence: fmt.Sprintf("The record showed %d burst(s) of %d or more failed logins within %d minutes in the last day, %s.", s.Bursts, loginBurstFailures, int(loginBurstWindow.Minutes()), total)}}
}

// LoginSource is what the reporter reads of the login watch.
type LoginSource interface {
	Snapshot(now time.Time) LoginSnapshot
}

// LoginWatchConfig reads and changes whether the login watch is on. It is off until the operator turns it on; it opens no socket
// and sends nothing, so switching it needs no passphrase.
type LoginWatchConfig struct{ store PolicyStore }

func NewLoginWatchConfig(store PolicyStore) *LoginWatchConfig { return &LoginWatchConfig{store: store} }

// Enabled reports whether the watch is on.
func (c *LoginWatchConfig) Enabled() bool { return c.store.Get().LoginWatchEnabled }

// Set switches the watch on or off.
func (c *LoginWatchConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.LoginWatchEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the login watch setting: %w", err)
	}
	return nil
}
