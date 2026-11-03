package netaudit

import (
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestBuildNearbyView_JoinsTheDeviceListAndPutsProblemsFirst(t *testing.T) {
	now := time.Now()
	state := models.NearbyState{Devices: []models.NearbyDevice{
		{IP: "192.168.50.20", LastHeard: now},
		{IP: "192.168.50.7", Gateway: true, LastHeard: now.Add(-time.Hour)},
		{IP: "192.168.50.44", LastHeard: now.Add(-time.Minute)},
	}}
	devices := []DeviceView{{MAC: "aa:bb", IP: "192.168.50.44", Name: "Kitchen", New: true}}
	v := BuildNearbyView(true, false, state, devices, router)
	if v.Devices[0].IP != "192.168.50.7" || v.Devices[0].Problem == nil || v.Devices[0].Problem.Guide == nil {
		t.Fatalf("the gateway claim must come first with its guide: %+v", v.Devices[0])
	}
	if v.Devices[1].IP != "192.168.50.44" || !v.Devices[1].Known || v.Devices[1].MAC != "aa:bb" || !v.Devices[1].New {
		t.Fatalf("a new known device second: %+v", v.Devices[1])
	}
	if v.Devices[2].Known {
		t.Fatal("a device the list has not seen must say so")
	}
	if strings.Contains(v.Devices[0].Problem.Text, "192.168") {
		t.Fatal("the sentence is fixed text")
	}
}
