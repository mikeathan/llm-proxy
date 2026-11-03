package netaudit

import (
	"net/netip"
	"strings"
	"testing"

	"llm-proxy/models"
)

var router = netip.MustParseAddr("192.168.50.1")

func TestRogueGateway_OnlyAGatewayThatIsNotTheRouter(t *testing.T) {
	state := models.NearbyState{Devices: []models.NearbyDevice{
		{IP: "192.168.50.1", Gateway: true},
		{IP: "192.168.50.7", Gateway: true, Maker: "Example‮"},
		{IP: "192.168.50.8"},
	}}
	got := RogueGatewayFindings(state, router)
	if len(got) != 1 || got[0].Device != "192.168.50.7" || got[0].Check != CheckRogueGateway || got[0].Severity != SeverityMedium {
		t.Fatalf("%+v", got)
	}
	if strings.Contains(got[0].Evidence, "Example") {
		t.Fatal("device text reached a finding")
	}
}

func TestRogueGateway_NoKnownRouterJudgesNothing(t *testing.T) {
	state := models.NearbyState{Devices: []models.NearbyDevice{{IP: "192.168.50.7", Gateway: true}}}
	if got := RogueGatewayFindings(state, netip.Addr{}); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestRogueGateway_AQuietLookKeepsTheFinding(t *testing.T) {
	state := models.NearbyState{LastOutcome: "nothing_heard", Devices: []models.NearbyDevice{{IP: "192.168.50.7", Gateway: true}}}
	if got := RogueGatewayFindings(state, router); len(got) != 1 {
		t.Fatalf("a quiet look resolved the finding: %+v", got)
	}
}

func TestNearbyConfig_OffUntilSwitchedOn(t *testing.T) {
	store := &memPolicy{}
	c := NewNearbyConfig(store)
	if c.Enabled() {
		t.Fatal("on by default")
	}
	if err := c.Set(true); err != nil || !c.Enabled() {
		t.Fatalf("%v", err)
	}
}
