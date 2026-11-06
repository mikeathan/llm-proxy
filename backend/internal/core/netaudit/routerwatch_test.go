package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
)

type routerRig struct {
	w     *RouterWatch
	now   time.Time
	sent  []string
	on    bool
	fails bool
}

func newRouterRig() *routerRig {
	r := &routerRig{now: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC), on: true}
	r.w = NewRouterWatch(RouterWatchDeps{
		Enabled: func() bool { return r.on },
		Notify: func(_ context.Context, m string) error {
			if r.fails {
				return errors.New("no connector")
			}
			r.sent = append(r.sent, m)
			return nil
		},
		Now: func() time.Time { return r.now },
	})
	return r
}

func (r *routerRig) add(kind routerlog.WatchedKind, n int, every time.Duration) {
	for i := 0; i < n; i++ {
		r.w.Add(routerlog.Watched{Kind: kind, Reason: 3}, r.now)
		r.now = r.now.Add(every)
	}
}

func TestRouterWatch_FiveFailedSignInsInTenMinutesAreToldOnce(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.SignInFailed, routerSignInFailures, time.Minute)
	r.w.Tick(context.Background())
	if len(r.sent) != 1 || !strings.Contains(r.sent[0], "sign-in") || !strings.Contains(r.sent[0], "5") {
		t.Fatalf("sent = %q", r.sent)
	}
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Errorf("the same burst was told twice: %q", r.sent)
	}
}

func TestRouterWatch_FailuresSpreadOverMoreThanTheWindowAreNotABurst(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.SignInFailed, 20, 3*time.Minute) // one every three minutes: four in ten
	r.w.Tick(context.Background())
	if len(r.sent) != 0 {
		t.Errorf("sent = %q", r.sent)
	}
}

func TestRouterWatch_TwentyDisconnectsInAMinuteAreToldAndAFewAreNot(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.Deauth, routerDeauthBurst-1, time.Second) // the rate of a phone toggling its Wi-Fi, and more
	r.w.Tick(context.Background())
	if len(r.sent) != 0 {
		t.Fatalf("nineteen is not a flood: %q", r.sent)
	}
	r.add(routerlog.Deauth, 1, time.Second)
	r.w.Tick(context.Background())
	if len(r.sent) != 1 || !strings.Contains(r.sent[0], "Wi-Fi") {
		t.Fatalf("sent = %q", r.sent)
	}
}

func TestRouterWatch_ASecondBurstWithinTheGapWaitsAndIsToldAfterIt(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.SignInFailed, routerSignInFailures, time.Second)
	r.w.Tick(context.Background())
	r.add(routerlog.SignInFailed, routerSignInFailures, time.Second)
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent = %q", r.sent)
	}
	r.now = r.now.Add(routerAlertGap)
	r.w.Tick(context.Background())
	if len(r.sent) != 2 {
		t.Errorf("the held burst was never told: %q", r.sent)
	}
}

func TestRouterWatch_AMessageThatCouldNotBeSentIsTriedAgain(t *testing.T) {
	r := newRouterRig()
	r.fails = true
	r.add(routerlog.SignInFailed, routerSignInFailures, time.Second)
	r.w.Tick(context.Background())
	r.fails = false
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Errorf("sent = %q", r.sent)
	}
}

func TestRouterWatch_WhileSwitchedOffNothingIsKeptOrSent(t *testing.T) {
	r := newRouterRig()
	r.on = false
	r.add(routerlog.SignInFailed, routerSignInFailures*2, time.Second)
	r.on = true
	r.w.Tick(context.Background())
	if len(r.sent) != 0 {
		t.Errorf("sent = %q", r.sent)
	}
}

func TestRouterWatch_ItsMemoryStaysTheSizeOfTheBurstWhateverIsSent(t *testing.T) {
	r := newRouterRig()
	r.add(routerlog.Deauth, 100000, time.Millisecond)
	r.add(routerlog.SignInFailed, 100000, time.Millisecond)
	if n := len(r.w.deauths) + len(r.w.fails); n > routerDeauthBurst+routerSignInFailures {
		t.Errorf("%d times kept", n)
	}
}
