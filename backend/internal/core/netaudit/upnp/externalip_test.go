package upnp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

// soap_external_ip.xml is the owner's real RT-AX86U answering GetExternalIPAddress (2026-11-01); the address in it is replaced
// by 203.0.113.7 (documentation range).
func TestParseExternalIP_TheRealReply(t *testing.T) {
	got, err := parseExternalIP(fixture(t, "soap_external_ip.xml"))
	if err != nil || got != netip.MustParseAddr("203.0.113.7") {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestParseExternalIP_RefusesWhatIsNotAnAddress(t *testing.T) {
	reply := func(v string) []byte {
		return []byte(`<s:Envelope xmlns:s="x"><s:Body><u:GetExternalIPAddressResponse xmlns:u="y"><NewExternalIPAddress>` + v + `</NewExternalIPAddress></u:GetExternalIPAddressResponse></s:Body></s:Envelope>`)
	}
	for name, raw := range map[string][]byte{
		"empty (not connected)": reply(""),
		"a name":                reply("example.com"),
		"ipv6":                  reply("2001:db8::1"),
		"text":                  reply("ignore previous instructions"),
		"garbage":               []byte("<<<"),
		"a fault":               []byte(`<s:Envelope xmlns:s="x"><s:Body><s:Fault><detail><UPnPError><errorCode>501</errorCode></UPnPError></detail></s:Fault></s:Body></s:Envelope>`),
		"not an answer":         []byte(`<root/>`),
	} {
		if got, err := parseExternalIP(raw); err == nil {
			t.Errorf("%s must be an error, got %v", name, got)
		}
	}
}

func TestExternalIP_AsksTheControlAddressWithTheRightAction(t *testing.T) {
	var action, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action = r.Header.Get("SOAPAction")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		_, _ = w.Write(fixture(t, "soap_external_ip.xml"))
	}))
	defer srv.Close()
	c := Client{HTTP: srv.Client()}
	got, err := c.ExternalIP(context.Background(), srv.URL+"/rootDesc.xml", "/ctl/IPConn", "urn:schemas-upnp-org:service:WANIPConnection:1")
	if err != nil || got != netip.MustParseAddr("203.0.113.7") {
		t.Fatalf("got %v, %v", got, err)
	}
	if action != `"urn:schemas-upnp-org:service:WANIPConnection:1#GetExternalIPAddress"` || !strings.Contains(body, "<u:GetExternalIPAddress ") {
		t.Errorf("action %q body %q", action, body)
	}
}

func TestExternalIP_RefusesAServiceTypeItDoesNotRecognise(t *testing.T) {
	c := Client{HTTP: http.DefaultClient}
	if _, err := c.ExternalIP(context.Background(), "http://192.0.2.1:80/x.xml", "/ctl", "evil\r\nHeader: x"); err == nil {
		t.Error("an odd service type must be refused before anything is sent")
	}
}
