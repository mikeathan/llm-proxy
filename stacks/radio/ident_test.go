package radio

import "testing"

// A class change the classifier is waiting to be sure of is said, so the page never shows the old class at a confidence of nought.
func TestTheIdentitySaysWhichClassAChangeIsWaitingFor(t *testing.T) {
	c := newIdentCtx(nil, nil)
	got := c.of(FeatureRow{Class: string(ClassInfrastructure), Pending: string(ClassPersonal), PendingN: 1}, nil)
	if got.Pending != ClassPersonal {
		t.Errorf("pending: %q", got.Pending)
	}
	if c.of(FeatureRow{Class: string(ClassInfrastructure)}, nil).Pending != "" {
		t.Error("nothing pending, nothing said")
	}
}

// The name a router's radio has on the network is the router's, so it never decides the radio's kind.
func TestARouterRadiosNetworkNameDoesNotDecideItsKind(t *testing.T) {
	row := FeatureRow{Tag: []byte("r1"), WifiGen: 6}
	c := newIdentCtx(nil, map[string]LANInfo{"r1": {Name: "Kitchen TV", Router: true}})
	if got := c.of(row, nil); got.Kind != DeviceRouter || got.KindSource != KindFromBeacons {
		t.Errorf("%s from %s", got.Kind, got.KindSource)
	}
}
