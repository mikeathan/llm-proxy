package nearby

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/upnp"
	"llm-proxy/internal/platform/mcast"
	"llm-proxy/models"
)

type memStore struct{ s models.NearbyState }

func (m *memStore) Read() (models.NearbyState, error) { return m.s, nil }
func (m *memStore) Write(s models.NearbyState) error  { m.s = s; return nil }

type rig struct {
	store     *memStore
	c         *Collector
	enabled   bool
	lan       bool
	replies   []mcast.Datagram
	exchanged int
	sent      []mcast.Question
	identity  upnp.Identity
	idErr     error
	now       time.Time
}

func newRig() *rig {
	r := &rig{store: &memStore{}, enabled: true, lan: true, now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	r.c = New(Deps{
		Enabled:    func() bool { return r.enabled },
		LANAllowed: func() bool { return r.lan },
		Interfaces: func() ([]mcast.Interface, error) {
			return []mcast.Interface{{Name: "wlan0", Addr: netip.MustParseAddr("192.168.50.60")}}, nil
		},
		Exchange: func(_ context.Context, _ mcast.Interface, qs []mcast.Question, _ mcast.Limits, accept func(netip.Addr) bool) ([]mcast.Datagram, error) {
			r.exchanged++
			r.sent = qs
			var out []mcast.Datagram
			for _, d := range r.replies {
				if accept(d.From.Addr()) {
					out = append(out, d)
				}
			}
			return out, nil
		},
		Accept:   func(a netip.Addr) bool { return a.String() != "192.168.50.60" && a.As4()[0] == 192 },
		Identify: func(context.Context, string, netip.Addr) (upnp.Identity, error) { return r.identity, r.idErr },
		Store:    r.store,
		Now:      func() time.Time { return r.now },
	})
	return r
}

func from(ip string) netip.AddrPort { return netip.AddrPortFrom(netip.MustParseAddr(ip), 5353) }

func mdnsReply(ip, svc string) mcast.Datagram {
	return mcast.Datagram{From: from(ip), Data: newMsg().rec("_services._dns-sd._udp.local", 12, name(svc+".local")).bytes()}
}

func ssdpReply(ip string) mcast.Datagram {
	return mcast.Datagram{From: from(ip), Data: []byte("HTTP/1.1 200 OK\r\nLOCATION: http://" + ip + ":49152/desc.xml\r\nST: upnp:rootdevice\r\n\r\n")}
}

func TestLook_OffSendsNothing(t *testing.T) {
	r := newRig()
	r.enabled = false
	out, err := r.c.look(context.Background())
	if out != OutcomeOff || err != nil || r.exchanged != 0 {
		t.Fatalf("%q %v exchanged=%d", out, err, r.exchanged)
	}
}

func TestLook_LocalNetworkOffSendsNothing(t *testing.T) {
	r := newRig()
	r.lan = false
	if out, _ := r.c.look(context.Background()); out != OutcomeLANOff || r.exchanged != 0 {
		t.Fatalf("%q exchanged=%d", out, r.exchanged)
	}
}

func TestLook_SendsExactlyTheTwoFixedQuestions(t *testing.T) {
	r := newRig()
	r.c.look(context.Background())
	if len(r.sent) != 2 || r.sent[0].To != MDNSGroup || r.sent[1].To != SSDPGroup || string(r.sent[1].Payload) != string(SSDPSearch) {
		t.Fatalf("sent %+v", r.sent)
	}
}

func TestLook_KeepsWhatWasHeardAndDropsWhatIsNotAccepted(t *testing.T) {
	r := newRig()
	r.replies = []mcast.Datagram{mdnsReply("192.168.50.20", "_ipp._tcp"), mdnsReply("203.0.113.9", "_ipp._tcp"), mdnsReply("192.168.50.60", "_ipp._tcp")}
	out, _ := r.c.look(context.Background())
	st, _ := r.c.State()
	if out != OutcomeHeard || len(st.Devices) != 1 || st.Devices[0].IP != "192.168.50.20" || st.Devices[0].Services[0] != "_ipp._tcp" {
		t.Fatalf("%q %+v", out, st)
	}
}

func TestLook_SilenceIsNotAbsence(t *testing.T) {
	r := newRig()
	r.replies = []mcast.Datagram{mdnsReply("192.168.50.20", "_ipp._tcp")}
	r.c.look(context.Background())
	r.replies, r.now = nil, r.now.Add(time.Hour)
	out, _ := r.c.look(context.Background())
	st, _ := r.c.State()
	if out != OutcomeNothingHeard || len(st.Devices) != 1 || !st.Devices[0].LastHeard.Before(r.now) {
		t.Fatalf("a quiet look removed or refreshed a device: %q %+v", out, st)
	}
}

func TestLook_ADeviceUnheardForAMonthExpires(t *testing.T) {
	r := newRig()
	r.replies = []mcast.Datagram{mdnsReply("192.168.50.20", "_ipp._tcp")}
	r.c.look(context.Background())
	r.replies, r.now = nil, r.now.Add(Keep+time.Hour)
	r.c.look(context.Background())
	if st, _ := r.c.State(); len(st.Devices) != 0 {
		t.Fatalf("kept %+v", st.Devices)
	}
}

func TestLook_AGatewayPageSetsAndLaterClearsTheClaim(t *testing.T) {
	r := newRig()
	r.replies = []mcast.Datagram{ssdpReply("192.168.50.7")}
	r.identity = upnp.Identity{DeviceType: "urn:schemas-upnp-org:device:InternetGatewayDevice:1", Gateway: true, Manufacturer: "Example‮", ModelName: "EN", ModelNumber: "300"}
	r.c.look(context.Background())
	st, _ := r.c.State()
	if !st.Devices[0].Gateway || st.Devices[0].UPnPType != "InternetGatewayDevice:1" || st.Devices[0].Model != "EN 300" || st.Devices[0].Maker != "Example" {
		t.Fatalf("%+v", st.Devices[0])
	}
	r.now = r.now.Add(time.Minute)
	r.identity = upnp.Identity{DeviceType: "urn:schemas-upnp-org:device:MediaRenderer:1"}
	r.c.look(context.Background())
	if st, _ = r.c.State(); st.Devices[0].Gateway {
		t.Fatal("a page that no longer says gateway must clear the claim")
	}
}

func TestLook_APageThatCannotBeReadKeepsTheClaim(t *testing.T) {
	r := newRig()
	r.replies = []mcast.Datagram{ssdpReply("192.168.50.7")}
	r.identity = upnp.Identity{DeviceType: "urn:schemas-upnp-org:device:InternetGatewayDevice:1", Gateway: true}
	r.c.look(context.Background())
	r.now, r.idErr = r.now.Add(time.Minute), errors.New("timeout")
	r.c.look(context.Background())
	if st, _ := r.c.State(); !st.Devices[0].Gateway {
		t.Fatal("a failed page read is silence, not evidence")
	}
}

func TestLook_ASecondLookTooSoonOrWhileRunningIsRefused(t *testing.T) {
	r := newRig()
	r.c.look(context.Background())
	if _, err := r.c.look(context.Background()); !errors.Is(err, ErrTooSoon) {
		t.Fatalf("%v", err)
	}
	r.now = r.now.Add(MinGap + time.Second)
	if _, err := r.c.look(context.Background()); err != nil {
		t.Fatalf("%v", err)
	}
	r.c.running = true
	r.now = r.now.Add(time.Hour)
	if _, err := r.c.look(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("%v", err)
	}
}

func TestLook_ARefusedSocketIsAFixedOutcome(t *testing.T) {
	r := newRig()
	r.c.deps.Exchange = func(context.Context, mcast.Interface, []mcast.Question, mcast.Limits, func(netip.Addr) bool) ([]mcast.Datagram, error) {
		return nil, mcast.ErrRefused
	}
	if out, _ := r.c.look(context.Background()); out != OutcomeRefused {
		t.Fatalf("%q", out)
	}
	r.now = r.now.Add(time.Hour)
	r.c.deps.Exchange = func(context.Context, mcast.Interface, []mcast.Question, mcast.Limits, func(netip.Addr) bool) ([]mcast.Datagram, error) {
		return nil, mcast.ErrUnavailable
	}
	if out, _ := r.c.look(context.Background()); out != OutcomeUnavailable {
		t.Fatalf("%q", out)
	}
}

func TestLook_ReadsAtMostThirtyTwoPages(t *testing.T) {
	r := newRig()
	for i := range 50 {
		r.replies = append(r.replies, ssdpReply("192.168.50."+itoa(100+i)))
	}
	pages := 0
	r.c.deps.Identify = func(context.Context, string, netip.Addr) (upnp.Identity, error) { pages++; return upnp.Identity{}, nil }
	r.c.look(context.Background())
	if pages != MaxDescriptions {
		t.Fatalf("fetched %d pages", pages)
	}
}

func TestLook_ALocationOnAnotherHostIsNeverFetched(t *testing.T) {
	r := newRig()
	r.replies = []mcast.Datagram{{From: from("192.168.50.7"), Data: []byte("HTTP/1.1 200 OK\r\nLOCATION: http://192.168.50.99/d.xml\r\n\r\n")}}
	fetched := false
	r.c.deps.Identify = func(context.Context, string, netip.Addr) (upnp.Identity, error) {
		fetched = true
		return upnp.Identity{}, nil
	}
	r.c.look(context.Background())
	if fetched {
		t.Fatal("fetched a page that is not on the replier")
	}
}

// look is a synchronous look for the tests: the same guards as Request, then the same work Run does.
func (c *Collector) look(ctx context.Context) (string, error) {
	if !c.deps.Enabled() {
		return OutcomeOff, nil
	}
	if err := c.begin(); err != nil {
		return "", err
	}
	defer c.end()
	return c.execute(ctx), nil
}

func itoa(i int) string { return netip.AddrFrom4([4]byte{0, 0, 0, byte(i)}).String()[6:] }

func TestRequest_RunsOneLookInTheBackgroundAndRefusesAnotherAtOnce(t *testing.T) {
	r := newRig()
	r.replies = []mcast.Datagram{mdnsReply("192.168.50.20", "_ipp._tcp")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.c.Run(ctx); close(done) }()
	if err := r.c.Request(); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Request(); !errors.Is(err, ErrBusy) && !errors.Is(err, ErrTooSoon) {
		t.Fatalf("a second request was accepted: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for r.c.Running() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if st, _ := r.c.State(); len(st.Devices) != 1 {
		t.Fatalf("the look did not finish: %+v", st)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop with its context")
	}
}

func TestRequest_OffIsRefusedAndSendsNothing(t *testing.T) {
	r := newRig()
	r.enabled = false
	if err := r.c.Request(); !errors.Is(err, ErrOff) || r.exchanged != 0 {
		t.Fatalf("%v", err)
	}
}
