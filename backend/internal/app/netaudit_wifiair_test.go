package app

import (
	"context"
	"testing"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

type memRadioPolicy struct{ cfg models.RadioConfig }

func (p *memRadioPolicy) Get() models.RadioConfig { return p.cfg }
func (p *memRadioPolicy) Update(f func(*models.RadioConfig) error) error {
	return f(&p.cfg)
}

// The names typed on the Radio page move into the Network page's one list once, the Radio settings lose them, and a second start moves
// nothing; access points are not moved, so a stack can never add to what the operator confirmed.
func TestMoveRadioNetworkNames_MovesTheNamesOnceIntoTheOneList(t *testing.T) {
	radio := &memRadioPolicy{cfg: models.RadioConfig{OwnNetworks: []string{"home", "home-2G"}}}
	store := &joinedPolicy{cfg: models.NetauditConfig{WifiNetworks: []models.WifiNetwork{{Name: "home", Addresses: []string{"aa:bb:cc:00:00:01"}}}}}
	moveRadioNetworkNames(radio, store)
	got := netaudit.NewWifiConfig(store).Networks()
	if len(got) != 2 || got[0].Name != "home" || len(got[0].Addresses) != 1 || got[1].Name != "home-2G" || len(got[1].Addresses) != 0 {
		t.Fatalf("networks = %+v", got)
	}
	if len(radio.cfg.OwnNetworks) != 0 {
		t.Errorf("the Radio settings keep %v", radio.cfg.OwnNetworks)
	}
	moveRadioNetworkNames(radio, store)
	if n := len(netaudit.NewWifiConfig(store).Networks()); n != 2 {
		t.Errorf("a second start added names: %d", n)
	}
}

// Without the Radio stack the look-alike check has no Radio source, and says so, so it works from this computer's own Wi-Fi alone.
func TestWifiAir_NothingWithoutTheRadioStack(t *testing.T) {
	if got, ok := (&AppServices{}).wifiAir(context.Background()); ok || got != nil {
		t.Errorf("wifiAir = %v, %v", got, ok)
	}
}
