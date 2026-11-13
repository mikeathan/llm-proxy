package port

import (
	"testing"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/platform/privhelper/grant"
	"llm-proxy/internal/platform/privhelper/session"
)

const listen = `{"iface":"wlx1","channel":60,"seconds":20}`

func sessionRig(t *testing.T) (*SessionPort, *session.Store, *recordingHelper, *bool) {
	t.Helper()
	sessions := session.New(session.Options{})
	h := &recordingHelper{}
	on := true
	p := newPort(grant.NewStore(grant.Options{}), h).ForStack("radio", sessions, func(string) bool { return on })
	return p, sessions, h, &on
}

func approve(t *testing.T, s *session.Store, stack, action, canonical string) session.View {
	t.Helper()
	v, err := s.Begin(session.Spec{Stack: stack, Label: "test", TTL: time.Hour, MaxUses: 5, Requests: []session.Request{{Action: action, Canonical: []byte(canonical)}}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAStackRunsAnApprovedRequestWithNoGrantWhileTheSessionLives(t *testing.T) {
	p, sessions, h, _ := sessionRig(t)
	v := approve(t, sessions, "radio", "radio_listen", listen)
	if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Action: "radio_listen", Params: []byte(listen)}); err != nil || h.calls != 1 {
		t.Fatalf("err %v, helper calls %d", err, h.calls)
	}
	// Spelling the parameters another way is the same request: the canonical form is what was approved.
	if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Action: "radio_listen", Params: []byte(`{"seconds":20,"channel":60,"iface":"wlx1"}`)}); err != nil {
		t.Errorf("the same request spelled differently: %v", err)
	}
	if open, _ := sessions.Open(); len(open) != 1 || open[0].Uses != 2 || open[0].ID != v.ID {
		t.Errorf("each run counts one use: %+v", open)
	}
	if err := sessions.End(v.ID); err != nil {
		t.Fatal(err)
	}
	_, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Action: "radio_listen", Params: []byte(listen)})
	if stacksdk.CodeOf(err) != stacksdk.CodeDenied || h.calls != 2 {
		t.Errorf("after the session ends: %v, helper calls %d", err, h.calls)
	}
}

func TestARequestTheSessionDoesNotListIsDeniedAndNeverReachesTheHelper(t *testing.T) {
	p, sessions, h, _ := sessionRig(t)
	approve(t, sessions, "radio", "radio_listen", listen)
	for name, params := range map[string]string{
		"another channel": `{"iface":"wlx1","channel":1,"seconds":20}`,
		"a longer window": `{"iface":"wlx1","channel":60,"seconds":30}`,
		"another adapter": `{"iface":"wlx2","channel":60,"seconds":20}`,
	} {
		if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Action: "radio_listen", Params: []byte(params)}); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
			t.Errorf("%s: %v", name, err)
		}
	}
	if h.calls != 0 {
		t.Errorf("the helper was called %d times", h.calls)
	}
}

func TestAnotherStacksSessionIsNotUsable(t *testing.T) {
	p, sessions, h, _ := sessionRig(t)
	approve(t, sessions, "network", "radio_listen", listen)
	if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Action: "radio_listen", Params: []byte(listen)}); stacksdk.CodeOf(err) != stacksdk.CodeDenied || h.calls != 0 {
		t.Errorf("%v, helper calls %d", err, h.calls)
	}
}

func TestASessionNeverServesAnActionThatIsNotStanding(t *testing.T) {
	p, sessions, h, _ := sessionRig(t)
	approve(t, sessions, "radio", "scan_udp", `{"target":"192.168.50.10","ports":[53,161]}`)
	if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Action: "scan_udp", Params: []byte(params)}); stacksdk.CodeOf(err) != stacksdk.CodeDenied || h.calls != 0 {
		t.Errorf("a scan needs a person's step-up each time: %v, helper calls %d", err, h.calls)
	}
}

func TestSwitchingTheActionOffStopsASessionAtOnce(t *testing.T) {
	p, sessions, h, on := sessionRig(t)
	approve(t, sessions, "radio", "radio_listen", listen)
	*on = false
	if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Action: "radio_listen", Params: []byte(listen)}); stacksdk.CodeOf(err) != stacksdk.CodeDenied || h.calls != 0 {
		t.Errorf("%v, helper calls %d", err, h.calls)
	}
}

func TestASessionStillMeetsTheHourlyCap(t *testing.T) {
	p, sessions, h, _ := sessionRig(t)
	v, err := sessions.Begin(session.Spec{Stack: "radio", Label: "x", TTL: time.Hour, MaxUses: session.MaxUses, Requests: []session.Request{{Action: "radio_listen", Canonical: []byte(listen)}}})
	if err != nil {
		t.Fatal(err)
	}
	_ = v
	for i := 0; i < 241; i++ {
		_, err = p.Run(t.Context(), stacksdk.PrivilegedRequest{Action: "radio_listen", Params: []byte(listen)})
	}
	if stacksdk.CodeOf(err) != stacksdk.CodeUnavailable || h.calls != 240 {
		t.Errorf("the 241st request in an hour: %v, helper calls %d", err, h.calls)
	}
}

func TestARequestThatCarriesAGrantTakesTheGrantPath(t *testing.T) {
	sessions := session.New(session.Options{})
	store, token := minted(t)
	h := &recordingHelper{}
	p := newPort(store, h).ForStack("radio", sessions, func(string) bool { return true })
	if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Grant: token, Action: "scan_udp", Params: []byte(params)}); err != nil || h.calls != 1 {
		t.Errorf("%v, helper calls %d", err, h.calls)
	}
	if _, err := p.Run(t.Context(), stacksdk.PrivilegedRequest{Grant: "forged", Action: "scan_udp", Params: []byte(params)}); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
		t.Errorf("a bad grant is not rescued by a session: %v", err)
	}
}
