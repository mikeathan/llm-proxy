package upnp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
)

func TestParseIdentity_RealRouterPage(t *testing.T) {
	f, err := os.Open("testdata/rootdesc_asus.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	id, err := ParseIdentity(f)
	if err != nil || !id.Gateway || id.Manufacturer != "ASUSTeK Computer Inc." || id.ModelNumber != "RT-AX86U" {
		t.Fatalf("got %+v %v", id, err)
	}
}

func TestParseIdentity_AMediaDeviceIsNotAGateway(t *testing.T) {
	id, err := ParseIdentity(strings.NewReader(`<root><device><deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType><friendlyName>TV</friendlyName></device></root>`))
	if err != nil || id.Gateway || id.FriendlyName != "TV" {
		t.Fatalf("got %+v %v", id, err)
	}
}

func TestParseIdentity_CutsALongFieldAndRefusesAHugePage(t *testing.T) {
	long := strings.Repeat("a", 5000)
	id, _ := ParseIdentity(strings.NewReader(`<root><device><friendlyName>` + long + `</friendlyName></device></root>`))
	if len(id.FriendlyName) != maxFieldLen {
		t.Fatalf("kept %d bytes", len(id.FriendlyName))
	}
	if _, err := ParseIdentity(strings.NewReader("<root><device><friendlyName>" + strings.Repeat("a", maxDescriptionBytes) + "</friendlyName></device></root>")); err == nil {
		t.Fatal("a page over the limit must be refused")
	}
}

func TestIdentify_RefusesALocationOnAnotherHostAndNeverFollowsARedirect(t *testing.T) {
	c := Client{HTTP: http.DefaultClient}
	if _, err := c.Identify(context.Background(), "http://192.0.2.9/desc.xml", netip.MustParseAddr("192.0.2.1")); err == nil {
		t.Fatal("a location on another host must be refused")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://192.0.2.9/x", http.StatusFound)
	}))
	defer srv.Close()
	addr := netip.MustParseAddr("127.0.0.1")
	if _, err := c.Identify(context.Background(), srv.URL+"/d.xml", addr); err == nil {
		t.Fatal("a redirect must not be followed")
	}
}
