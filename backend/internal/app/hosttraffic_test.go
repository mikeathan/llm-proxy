package app

import (
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
	"llm-proxy/models"
)

type memTraffic struct {
	got models.HostTraffic
	err error
}

func (m *memTraffic) Read() (models.HostTraffic, error) { return m.got, m.err }
func (m *memTraffic) Write(t models.HostTraffic) error  { m.got = t; return m.err }

func TestTheHostTrafficOutputIsReadInTheServiceNotTheHelper(t *testing.T) {
	data := wire.AppendHostTrailer(nil, wire.HostTrailer{Stop: wire.StopInterfaceDown, Sample: 1})
	v, err := parseHelperOutput("capture_host", data)
	s, ok := v.(models.HostTraffic)
	if err != nil || !ok || s.StoppedBy != "interface_down" {
		t.Fatalf("%v %v %v", v, ok, err)
	}
	if _, err := parseHelperOutput("capture_host", wire.AppendHostTrailer(nil, wire.HostTrailer{Frames: 3})); err == nil {
		t.Fatal("a trailer that disagrees with its records was accepted")
	}
}

func TestAHostTrafficWindowTellsDirectionByThisComputersAddresses(t *testing.T) {
	old := ownAddresses
	ownAddresses = func() []netip.Addr { return []netip.Addr{netip.MustParseAddr("192.168.50.20")} }
	t.Cleanup(func() { ownAddresses = old })
	f := make([]byte, 14+20+20)
	binary.BigEndian.PutUint16(f[12:], 0x0800)
	f[14] = 0x45
	binary.BigEndian.PutUint16(f[16:], 1000)
	f[14+9] = 6
	copy(f[14+12:], []byte{192, 168, 50, 20})
	copy(f[14+16:], []byte{93, 184, 216, 34})
	binary.BigEndian.PutUint16(f[14+20+2:], 443)
	f[14+20+12] = 5 << 4
	data := wire.AppendHostTrailer(wire.AppendHostRecord(nil, 5, 1, f), wire.HostTrailer{Frames: 1, ElapsedMs: 1000, Sample: 1})
	v, err := parseHelperOutput("capture_host", data)
	if err != nil || v.(models.HostTraffic).BytesOut != 1000 {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestAKeptHostTrafficSummaryIsStampedWithWhenAndWhere(t *testing.T) {
	sink := &memTraffic{}
	before := time.Now()
	keepHostTraffic(sink, []byte(`{"iface":"wlp2s0","seconds":60}`), models.HostTraffic{StoppedBy: "window", Frames: 3})
	if sink.got.Interface != "wlp2s0" || sink.got.At.Before(before) || sink.got.Frames != 3 {
		t.Fatalf("%+v", sink.got)
	}
	keepHostTraffic(sink, []byte(`nonsense`), models.HostTraffic{Frames: 9})
	if sink.got.Frames != 3 {
		t.Fatal("a request that cannot be read replaced the kept summary")
	}
	keepHostTraffic(nil, []byte(`{"iface":"x"}`), models.HostTraffic{}) // no store: nothing happens, no panic
}

func TestTheKeeperRoutesAHostTrafficSummaryToItsOwnStore(t *testing.T) {
	traffic := &memTraffic{}
	keep := keepHelperResult(helperSinks{Traffic: traffic})
	keep("capture_host", []byte(`{"iface":"eth0","seconds":5}`), models.HostTraffic{Frames: 4})
	if traffic.got.Frames != 4 || traffic.got.Interface != "eth0" {
		t.Fatalf("%+v", traffic.got)
	}
}

func TestTheHostTrafficRouteAnswersNullBeforeTheFirstLookAndTheSummaryAfter(t *testing.T) {
	sink := &memTraffic{}
	rec := httptest.NewRecorder()
	hostTrafficHandler(sink)(rec, httptest.NewRequest("GET", "/admin/api/netaudit/host-traffic", nil))
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"summary":null}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	sink.got = models.HostTraffic{At: time.Now(), Interface: "wlp2s0", StoppedBy: "window"}
	rec = httptest.NewRecorder()
	hostTrafficHandler(sink)(rec, httptest.NewRequest("GET", "/x", nil))
	if !strings.Contains(rec.Body.String(), `"interface":"wlp2s0"`) {
		t.Fatalf("%s", rec.Body)
	}
	sink.err = errors.New("disk")
	rec = httptest.NewRecorder()
	hostTrafficHandler(sink)(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "disk") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
