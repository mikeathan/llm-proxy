package stackport

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy/models"
)

const ownPurpose = "radio_ap_1"

func ownList() []models.WifiNetwork {
	return []models.WifiNetwork{{Name: "home", Addresses: []string{"aa:bb:cc:00:00:01", "bad"}}, {Name: "home-2G"}}
}

// The port answers the owner's names and the confirmed access points as the stack's own tags; no address crosses.
func TestNetworks_GivesNamesAndConfirmedTagsNotAddresses(t *testing.T) {
	ctx := context.Background()
	v := testVault()
	got, err := Networks{Vault: v, List: ownList, Asked: &AskedPurposes{}}.Networks(ctx, ownPurpose)
	if err != nil {
		t.Fatal(err)
	}
	ap, _ := net.ParseMAC("aa:bb:cc:00:00:01")
	want, _ := v.Tag(ctx, ownPurpose, ap)
	if len(got) != 2 || got[0].Name != "home" || len(got[0].Confirmed) != 1 || !bytes.Equal(got[0].Confirmed[0], want) || len(got[1].Confirmed) != 0 {
		t.Fatalf("networks = %+v", got)
	}
	if bytes.Contains(got[0].Confirmed[0], ap) {
		t.Error("an address crossed the port")
	}
}

func sighting(t *testing.T, v stacksdk.Vault, purpose, name, mac string) stacksdk.AirAP {
	t.Helper()
	ctx := context.Background()
	hw, _ := net.ParseMAC(mac)
	tag, _ := v.Tag(ctx, purpose, hw)
	sealed, err := v.Seal(ctx, purpose, hw)
	if err != nil {
		t.Fatal(err)
	}
	return stacksdk.AirAP{Name: name, Tag: tag, Sealed: sealed, Security: "wpa2_wpa3", Channel: 6, Heard: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
}

// A sighting passes only for one of the owner's names, sealed under a purpose the core handed out, as one unicast address whose tag matches.
func TestNetworks_SightingsAreCheckedBeforeTheyAreTrusted(t *testing.T) {
	ctx := context.Background()
	v := testVault()
	n := Networks{Vault: v, List: ownList, Asked: &AskedPurposes{}}
	if _, err := n.Networks(ctx, ownPurpose); err != nil {
		t.Fatal(err)
	}
	good := sighting(t, v, ownPurpose, "home-2G", "aa:bb:cc:00:00:02")
	foreignName := sighting(t, v, ownPurpose, "neighbour", "aa:bb:cc:00:00:03")
	otherPurpose := sighting(t, v, "radio_device_1", "home", "aa:bb:cc:00:00:04")
	multicast := sighting(t, v, ownPurpose, "home", "01:00:5e:00:00:01")
	wrongTag := sighting(t, v, ownPurpose, "home", "aa:bb:cc:00:00:05")
	wrongTag.Tag = good.Tag
	got := n.Sightings(ctx, []stacksdk.AirAP{good, foreignName, otherPurpose, multicast, wrongTag})
	if len(got) != 1 || got[0].BSSID != "aa:bb:cc:00:00:02" || got[0].Name != "home-2G" || got[0].Security != "wpa2" || got[0].Channel != 6 {
		t.Fatalf("sightings = %+v, want only the good one, judged as WPA2", got)
	}
}

// Without the port wired, nothing is answered and nothing passes.
func TestNetworks_UnwiredIsUnavailable(t *testing.T) {
	if _, err := (Networks{}).Networks(context.Background(), ownPurpose); err == nil {
		t.Error("an unwired port must be unavailable")
	}
	if got := (Networks{}).Sightings(context.Background(), []stacksdk.AirAP{{Name: "home"}}); len(got) != 0 {
		t.Errorf("sightings = %v", got)
	}
}
