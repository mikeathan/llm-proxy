package routerhistory

import (
	"errors"
	"testing"
	"time"

	"llm-proxy/models"
)

type memStore struct {
	state models.RouterHistory
	err   error
	wrote int
}

func (m *memStore) Read() (models.RouterHistory, error) { return m.state, m.err }
func (m *memStore) Write(h models.RouterHistory) error  { m.state = h; m.wrote++; return m.err }

var t0 = time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC)

func owner(asn uint32, name, country string) *models.TrafficOwner {
	return &models.TrafficOwner{ASN: asn, Name: name, Country: country}
}

func read(ip string, places ...models.RouterPlace) *models.RouterConnections {
	return &models.RouterConnections{Devices: []models.RouterDevice{{Address: ip, Total: len(places), Places: places}}}
}

func place(addr string, o *models.TrafficOwner) models.RouterPlace {
	return models.RouterPlace{Address: addr, Port: 443, Proto: "tcp", Count: 1, Owner: o}
}

func byIP(ip string) string { return "ip:" + ip }

func TestEverythingInTheFirstDayIsLearnedNotNew(t *testing.T) {
	h := NewEmpty(&memStore{})
	r1 := read("192.168.50.9", place("1.1.1.1", owner(13335, "Cloudflare", "US")))
	if err := h.Observe(r1, byIP, t0); err != nil {
		t.Fatal(err)
	}
	r2 := read("192.168.50.9", place("8.8.8.8", owner(15169, "Google LLC", "US")))
	if err := h.Observe(r2, byIP, t0.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if r1.Devices[0].Places[0].New || r2.Devices[0].Places[0].New {
		t.Fatal("a destination seen in the device's first day must not be new")
	}
	if got := h.Signals(t0.Add(4 * time.Hour)); len(got) != 0 {
		t.Fatalf("signals during the learning day = %v", got)
	}
}

func TestACompanyAfterTheLearningDayIsNewForADayThenNormal(t *testing.T) {
	h := NewEmpty(&memStore{})
	_ = h.Observe(read("192.168.50.9", place("1.1.1.1", owner(13335, "Cloudflare", "US"))), byIP, t0)
	later := t0.Add(30 * time.Hour)
	r := read("192.168.50.9", place("1.1.1.1", owner(13335, "Cloudflare", "US")), place("47.89.80.209", owner(45102, "Alibaba (US)", "DE")))
	_ = h.Observe(r, byIP, later)
	if r.Devices[0].Places[0].New {
		t.Fatal("a known company must not be new")
	}
	if !r.Devices[0].Places[1].New {
		t.Fatal("a company first seen after the learning day must be new")
	}
	sig := h.Signals(later.Add(time.Hour))
	if len(sig) != 2 || sig[0].Kind != NewCompany || sig[0].Names[0] != "Alibaba (US)" || sig[1].Kind != NewCountry || sig[1].Names[0] != "DE" {
		t.Fatalf("signals = %+v", sig)
	}
	if sig[0].IP != "192.168.50.9" || sig[0].Key != "ip:192.168.50.9" {
		t.Fatalf("signal device = %+v", sig[0])
	}
	if got := h.Signals(later.Add(NewFor + time.Minute)); len(got) != 0 {
		t.Fatalf("after the window the company is normal, got %v", got)
	}
}

func TestADeviceIsKeyedByWhatTheCallerSaysSoAChangedAddressKeepsItsHistory(t *testing.T) {
	h := NewEmpty(&memStore{})
	mac := func(string) string { return "aa:bb" }
	_ = h.Observe(read("192.168.50.9", place("1.1.1.1", owner(13335, "Cloudflare", "US"))), mac, t0)
	_ = h.Observe(read("192.168.50.77", place("47.89.80.209", owner(45102, "Alibaba (US)", "US"))), mac, t0.Add(30*time.Hour))
	sig := h.Signals(t0.Add(31 * time.Hour))
	if len(sig) != 1 || sig[0].IP != "192.168.50.77" || sig[0].Key != "aa:bb" {
		t.Fatalf("signals = %+v", sig)
	}
}

func TestAPlaceWithNoOwnerIsNotJudged(t *testing.T) {
	h := NewEmpty(&memStore{})
	r := read("192.168.50.9", place("1.1.1.1", nil))
	_ = h.Observe(r, byIP, t0)
	_ = h.Observe(read("192.168.50.9", place("2.2.2.2", nil)), byIP, t0.Add(48*time.Hour))
	if len(h.state.Devices["ip:192.168.50.9"].Companies) != 0 || len(h.Signals(t0.Add(49*time.Hour))) != 0 {
		t.Fatal("a place with no owner has no company to remember")
	}
}

func TestBoundsPushTheLeastRecentlySeenOut(t *testing.T) {
	h := NewEmpty(&memStore{})
	var places []models.RouterPlace
	for i := 0; i < MaxCompanies+5; i++ {
		places = append(places, place("9.9.9.9", owner(uint32(1000+i), "Co", "US")))
	}
	_ = h.Observe(read("192.168.50.9", places...), byIP, t0)
	if n := len(h.state.Devices["ip:192.168.50.9"].Companies); n != MaxCompanies {
		t.Fatalf("companies kept = %d, want %d", n, MaxCompanies)
	}
	for i := 0; i < MaxDevices+3; i++ {
		_ = h.Observe(read("10.0.0."+string(rune('a'+i%26))+string(rune('a'+i/26)), place("1.1.1.1", owner(1, "A", "US"))), byIP, t0.Add(time.Duration(i)*time.Second))
	}
	if n := len(h.state.Devices); n != MaxDevices {
		t.Fatalf("devices kept = %d, want %d", n, MaxDevices)
	}
}

func TestAHistoryThatCannotBeReadIsAnErrorNeverAnEmptyOne(t *testing.T) {
	if _, err := New(&memStore{err: errors.New("corrupt")}); err == nil {
		t.Fatal("an unreadable history must be an error, so it never reads as nothing new")
	}
}

func TestAFailedWriteIsReportedAndMemoryKeepsTheRead(t *testing.T) {
	s := &memStore{err: errors.New("disk")}
	h := NewEmpty(s)
	if err := h.Observe(read("192.168.50.9", place("1.1.1.1", owner(1, "A", "US"))), byIP, t0); err == nil {
		t.Fatal("a failed write must be returned")
	}
	if len(h.state.Devices) != 1 {
		t.Fatal("memory must keep the read")
	}
}

func TestStoredHistorySurvivesARestart(t *testing.T) {
	s := &memStore{}
	h := NewEmpty(s)
	_ = h.Observe(read("192.168.50.9", place("1.1.1.1", owner(13335, "Cloudflare", "US"))), byIP, t0)
	again, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	r := read("192.168.50.9", place("47.89.80.209", owner(45102, "Alibaba (US)", "US")))
	_ = again.Observe(r, byIP, t0.Add(30*time.Hour))
	if !r.Devices[0].Places[0].New {
		t.Fatal("history read back must still judge a new company")
	}
}
