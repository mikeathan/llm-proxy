package app

import (
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

type memCapture struct {
	got models.CaptureSummary
	err error
}

func (m *memCapture) Read() (models.CaptureSummary, error) { return m.got, m.err }
func (m *memCapture) Write(s models.CaptureSummary) error  { m.got = s; return m.err }

func TestAnEmptyCaptureRequestGetsTheDefaultInterfaceAndNothingElseIsTouched(t *testing.T) {
	d := helperDefaults(func() (string, error) { return "wlp2s0", nil })
	got, err := d("capture_window", nil)
	if err != nil || string(got) != `{"iface":"wlp2s0","seconds":0}` {
		t.Fatalf("got %s, %v", got, err)
	}
	if got, _ := d("capture_window", []byte(`{"iface":"eth0","seconds":5}`)); string(got) != `{"iface":"eth0","seconds":5}` {
		t.Errorf("a body the operator wrote was changed: %s", got)
	}
	if got, _ := d("scan_udp", nil); got != nil {
		t.Errorf("another action's body was changed: %s", got)
	}
}

func TestWithNoDefaultRouteTheOperatorIsToldInFixedWords(t *testing.T) {
	d := helperDefaults(func() (string, error) { return "", errNoDefaultInterface })
	if _, err := d("capture_window", nil); err == nil || !strings.Contains(err.Error(), "no default network interface") {
		t.Fatalf("err = %v", err)
	}
}

func TestTheCaptureOutputIsReadInTheServiceNotTheHelper(t *testing.T) {
	data := wire.AppendCaptureTrailer(nil, wire.CaptureTrailer{Stop: wire.StopInterfaceDown})
	v, err := parseHelperOutput("capture_window", data)
	s, ok := v.(models.CaptureSummary)
	if err != nil || !ok || s.StoppedBy != "interface_down" {
		t.Fatalf("%v %v %v", v, ok, err)
	}
	if _, err := parseHelperOutput("capture_window", wire.AppendCaptureTrailer(nil, wire.CaptureTrailer{Frames: 3})); err == nil {
		t.Fatal("a trailer that disagrees with its records was accepted")
	}
}

func TestAKeptCaptureIsStampedWithWhenAndWhere(t *testing.T) {
	sink := &memCapture{}
	before := time.Now()
	keepCapture(sink, noPrefixes, []byte(`{"iface":"wlp2s0","seconds":60}`), models.CaptureSummary{StoppedBy: "window", Frames: 3})
	if sink.got.Interface != "wlp2s0" || sink.got.At.Before(before) || sink.got.Frames != 3 {
		t.Fatalf("%+v", sink.got)
	}
	keepCapture(sink, noPrefixes, []byte(`nonsense`), models.CaptureSummary{Frames: 9})
	if sink.got.Frames != 3 {
		t.Fatal("a request that cannot be read replaced the kept summary")
	}
	keepCapture(nil, noPrefixes, []byte(`{"iface":"x"}`), models.CaptureSummary{}) // no store: nothing happens, no panic
}

func noPrefixes(string) []netip.Prefix { return nil }

func TestAKeptCaptureMarksAnnouncedAddressesOutsideTheLinksRanges(t *testing.T) {
	sink := &memCapture{}
	link := func(string) []netip.Prefix { return []netip.Prefix{netip.MustParsePrefix("192.168.50.7/24")} }
	keepCapture(sink, link, []byte(`{"iface":"wlp2s0","seconds":60}`), models.CaptureSummary{Claims: []models.CaptureClaim{{MAC: "aa", IP: "192.168.50.220"}, {MAC: "bb", IP: "192.168.101.212"}}})
	if sink.got.OffNetworkAddresses != 1 || sink.got.Claims[0].OffNetwork || !sink.got.Claims[1].OffNetwork {
		t.Fatalf("%+v", sink.got)
	}
}

func TestTheCaptureRouteAnswersNullBeforeTheFirstWindowAndTheSummaryAfter(t *testing.T) {
	sink := &memCapture{}
	rec := httptest.NewRecorder()
	captureWindowHandler(sink)(rec, httptest.NewRequest("GET", "/admin/api/netaudit/capture-window", nil))
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"summary":null}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	sink.got = models.CaptureSummary{At: time.Now(), Interface: "wlp2s0", StoppedBy: "window"}
	rec = httptest.NewRecorder()
	captureWindowHandler(sink)(rec, httptest.NewRequest("GET", "/x", nil))
	if !strings.Contains(rec.Body.String(), `"interface":"wlp2s0"`) {
		t.Fatalf("%s", rec.Body)
	}
	sink.err = errors.New("disk")
	rec = httptest.NewRecorder()
	captureWindowHandler(sink)(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "disk") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
