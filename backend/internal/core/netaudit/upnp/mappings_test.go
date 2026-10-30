package upnp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// soap_no_more_entries.xml is the owner's real RT-AX86U answering "entry 0" of an empty list (2026-10-30): HTTP 500 with
// UPnP error 713. soap_entry_from_spec.xml is NOT a capture (the router had no mappings to show); it is written from the
// UPnP IGD specification, so the success path is only as good as that reading.
func TestParseMapping_TheRealEndOfListReply(t *testing.T) {
	m, end, err := parseEntry(fixture(t, "soap_no_more_entries.xml"))
	if err != nil || !end || m != nil {
		t.Errorf("m=%+v end=%v err=%v", m, end, err)
	}
}

func TestParseMapping_AnEntryFromTheSpecification(t *testing.T) {
	m, end, err := parseEntry(fixture(t, "soap_entry_from_spec.xml"))
	if err != nil || end || m == nil {
		t.Fatalf("m=%+v end=%v err=%v", m, end, err)
	}
	if m.ExternalPort != 2222 || m.InternalPort != 22 || m.Protocol != "tcp" || m.Client != netip.MustParseAddr("192.168.50.77") || !m.Enabled {
		t.Errorf("mapping = %+v", m)
	}
}

func TestParseMapping_RefusesWhatIsNotAMapping(t *testing.T) {
	entry := func(ext, proto, in, client, enabled string) []byte {
		return []byte(`<s:Envelope xmlns:s="x"><s:Body><u:GetGenericPortMappingEntryResponse xmlns:u="y"><NewExternalPort>` + ext + `</NewExternalPort><NewProtocol>` + proto + `</NewProtocol><NewInternalPort>` + in + `</NewInternalPort><NewInternalClient>` + client + `</NewInternalClient><NewEnabled>` + enabled + `</NewEnabled></u:GetGenericPortMappingEntryResponse></s:Body></s:Envelope>`)
	}
	for name, raw := range map[string][]byte{
		"port zero":     entry("0", "TCP", "22", "192.168.50.7", "1"),
		"port too big":  entry("70000", "TCP", "22", "192.168.50.7", "1"),
		"not a number":  entry("x", "TCP", "22", "192.168.50.7", "1"),
		"a name":        entry("80", "TCP", "80", "printer.local", "1"),
		"odd protocol":  entry("80", "ICMP", "80", "192.168.50.7", "1"),
		"ipv6 client":   entry("80", "TCP", "80", "fe80::1", "1"),
		"garbage":       []byte("<<<"),
		"not an answer": []byte(`<root/>`),
	} {
		if m, _, err := parseEntry(raw); err == nil && m != nil {
			t.Errorf("%s must not give a mapping: %+v", name, m)
		}
	}
	// A disabled mapping forwards nothing.
	if m, _, err := parseEntry(entry("80", "TCP", "80", "192.168.50.7", "0")); err != nil || m == nil || m.Enabled {
		t.Errorf("a disabled mapping is read as disabled: %+v, %v", m, err)
	}
}

// controlServer is a router's control URL: it answers entries 0..n-1 and then the real end-of-list fault.
func controlServer(t *testing.T, entries int, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("SOAPAction"), `"urn:schemas-upnp-org:service:WANIPConnection:1#GetGenericPortMappingEntry"`) {
			t.Errorf("method %s action %q", r.Method, r.Header.Get("SOAPAction"))
		}
		index := 0
		for i := 0; i < 70; i++ {
			if strings.Contains(string(body), "<NewPortMappingIndex>"+itoa(i)+"</NewPortMappingIndex>") {
				index = i
			}
		}
		if index >= entries {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write(fixture(t, "soap_no_more_entries.xml"))
			return
		}
		_, _ = w.Write(fixture(t, "soap_entry_from_spec.xml"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestMappings_ListsUntilTheRouterSaysThereAreNoMore(t *testing.T) {
	var calls atomic.Int32
	srv := controlServer(t, 2, &calls)
	c := Client{HTTP: http.DefaultClient}
	got, err := c.Mappings(context.Background(), srv.URL+"/rootDesc.xml", "/ctl/IPConn", "urn:schemas-upnp-org:service:WANIPConnection:1")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if calls.Load() != 3 {
		t.Errorf("two entries and the end-of-list answer = 3 requests, got %d", calls.Load())
	}
}

func TestMappings_AnEmptyListIsOneRequest(t *testing.T) {
	var calls atomic.Int32
	srv := controlServer(t, 0, &calls)
	got, err := Client{HTTP: http.DefaultClient}.Mappings(context.Background(), srv.URL+"/rootDesc.xml", "/ctl/IPConn", "urn:schemas-upnp-org:service:WANIPConnection:1")
	if err != nil || len(got) != 0 || calls.Load() != 1 {
		t.Errorf("got %+v, %v after %d requests", got, err, calls.Load())
	}
}

func TestMappings_AreBounded(t *testing.T) {
	var calls atomic.Int32
	srv := controlServer(t, 1000, &calls)
	got, err := Client{HTTP: http.DefaultClient}.Mappings(context.Background(), srv.URL+"/rootDesc.xml", "/ctl/IPConn", "urn:schemas-upnp-org:service:WANIPConnection:1")
	if err != nil || len(got) != maxMappings || calls.Load() > int32(maxMappings) {
		t.Errorf("got %d mappings after %d requests, %v", len(got), calls.Load(), err)
	}
}

func TestMappings_RefusesAnOddServiceType(t *testing.T) {
	c := Client{HTTP: http.DefaultClient}
	for _, urn := range []string{"", "urn:x", "urn:schemas-upnp-org:service:WANIPConnection:1\r\nX-Evil: 1", "urn:schemas-upnp-org:service:WANIPConnection:9"} {
		if _, err := c.Mappings(context.Background(), "http://192.168.50.1:80/rootDesc.xml", "/ctl/IPConn", urn); err == nil {
			t.Errorf("%q must be refused before anything is sent", urn)
		}
	}
}

func TestMappings_AServerErrorThatIsNotTheEndOfTheListIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()
	if _, err := (Client{HTTP: http.DefaultClient}).Mappings(context.Background(), srv.URL+"/x", "/ctl/IPConn", "urn:schemas-upnp-org:service:WANIPConnection:1"); err == nil {
		t.Error("an unreadable answer is 'could not list', never 'no mappings'")
	}
}
