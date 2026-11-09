package netaudit

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"llm-proxy/models"
)

// The login watch: failed logins to this computer, counted from the system's own record (platform/loginlog) and nothing more. It tells
// the operator, in fixed words, of a burst, and adds a finding to the nightly report. No user name is read, kept or shown (Constitution I.10):
// the finding says how many failures came from this network and from the internet, and names the busiest few sources by their parsed
// address, never by text a sender wrote. Only a program with write access to the record could make one up, which is stronger than the
// router's log, where any device can.
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

// LoginWatchDeps are what the watch needs. Read gives the failed logins after a time, oldest first; Notify sends one
// message through the alert log and fails when it could not be delivered, so it is tried again at the next look.
type LoginWatchDeps struct {
	Read    func(after time.Time) ([]LoginAttempt, error)
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

// LoginAttempt is one failed login: when, and from where when the record says (the zero address when it does not).
type LoginAttempt struct {
	At   time.Time
	From netip.Addr
}

// maxLoginOrigins is how many of the busiest sources a finding names.
const maxLoginOrigins = 3

// LoginOrigin is one source address and how many failures came from it.
type LoginOrigin struct {
	Addr  netip.Addr
	Count int
}

// LoginOrigins says where a day's failures came from. Network counts those from this computer or a private address, Internet those from
// any other, Unknown those with no address recorded. Distinct is how many different addresses there were and Top the busiest few.
type LoginOrigins struct {
	Network, Internet, Unknown, Distinct int
	Top                                  []LoginOrigin
}

// originsOf counts where the attempts came from.
func originsOf(attempts []LoginAttempt) LoginOrigins {
	var o LoginOrigins
	per := map[netip.Addr]int{}
	for _, a := range attempts {
		switch {
		case !a.From.IsValid():
			o.Unknown++
		case a.From.IsPrivate() || a.From.IsLoopback() || a.From.IsLinkLocalUnicast():
			o.Network++
			per[a.From]++
		default:
			o.Internet++
			per[a.From]++
		}
	}
	o.Distinct = len(per)
	for addr, n := range per {
		o.Top = append(o.Top, LoginOrigin{Addr: addr, Count: n})
	}
	slices.SortFunc(o.Top, func(a, b LoginOrigin) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return a.Addr.Compare(b.Addr)
	})
	if len(o.Top) > maxLoginOrigins {
		o.Top = o.Top[:maxLoginOrigins]
	}
	return o
}

// LoginSnapshot is what the nightly report reads: the failed logins of the last day and how many bursts they held.
type LoginSnapshot struct {
	Failures int
	Bursts   int
	// Origins says where the failures came from.
	Origins LoginOrigins
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
	attempts, ok := w.read(now.Add(-loginBurstWindow), now)
	if !ok || len(burstsIn(timesOf(attempts), loginBurstFailures, loginBurstWindow)) == 0 {
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
func (w *LoginWatch) read(after, now time.Time) ([]LoginAttempt, bool) {
	attempts, err := w.deps.Read(after)
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
	kept := attempts[:0:0]
	for _, a := range attempts {
		if !a.At.After(now.Add(loginFutureSlack)) {
			kept = append(kept, a)
		}
	}
	return kept, true
}

func timesOf(attempts []LoginAttempt) []time.Time {
	out := make([]time.Time, len(attempts))
	for i, a := range attempts {
		out[i] = a.At
	}
	return out
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
	attempts, ok := w.read(now.Add(-loginDay), now)
	if !ok {
		return LoginSnapshot{}
	}
	return LoginSnapshot{Failures: len(attempts), Bursts: len(burstsIn(timesOf(attempts), loginBurstFailures, loginBurstWindow)), Origins: originsOf(attempts), Capped: w.deps.Cap > 0 && len(attempts) >= w.deps.Cap}
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
		Evidence: fmt.Sprintf("The record showed %d burst(s) of %d or more failed logins within %d minutes in the last day, %s.", s.Bursts, loginBurstFailures, int(loginBurstWindow.Minutes()), total) + originsText(s.Origins)}}
}

// originsText says where the failures came from, in numbers and parsed addresses: nothing a sender wrote. It is empty when the record
// held no sources (an older record, or one whose address field was never filled).
func originsText(o LoginOrigins) string {
	if o.Network+o.Internet == 0 {
		return ""
	}
	parts := []string{fmt.Sprintf("%d from the internet", o.Internet), fmt.Sprintf("%d from this network", o.Network)}
	if o.Unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d with no address recorded", o.Unknown))
	}
	text := " Where from: " + strings.Join(parts, ", ") + fmt.Sprintf(", %d different address(es).", o.Distinct)
	var busiest []string
	for _, t := range o.Top {
		busiest = append(busiest, fmt.Sprintf("%s (%d)", t.Addr, t.Count))
	}
	return text + " Busiest: " + strings.Join(busiest, ", ") + "."
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
