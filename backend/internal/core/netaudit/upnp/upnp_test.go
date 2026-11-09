package upnp

import (
	"net/netip"
	"os"
	"strings"
	"testing"
)

var router = netip.MustParseAddr("192.168.50.1")

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The answer and the description page are the owner's real router (ASUS RT-AX86U, MiniUPnPd 2.3.11), with the hardware
// address and everything derived from it replaced.
func TestParseSSDP_TheRealRouterAnswer(t *testing.T) {
	loc, err := ParseSSDP(fixture(t, "ssdp_asus_response.txt"), router)
	if err != nil {
		t.Fatal(err)
	}
	if loc != "http://192.168.50.1:37027/rootDesc.xml" {
		t.Errorf("location = %q", loc)
	}
}

func TestParseSSDP_RefusesALocationThatIsNotTheRouter(t *testing.T) {
	answer := func(location string) []byte {
		return []byte("HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\nLOCATION: " + location + "\r\n\r\n")
	}
	for name, location := range map[string]string{
		"another host": "http://192.168.50.77:80/rootDesc.xml",
		"the internet": "http://203.0.113.9/rootDesc.xml",
		"a name":       "http://router.local/rootDesc.xml",
		"https":        "https://192.168.50.1/rootDesc.xml",
		"userinfo":     "http://user@192.168.50.1/rootDesc.xml",
		"no path":      "http://192.168.50.1:80",
		"a credential": "http://192.168.50.1:80/a?token=1",
		"too long":     "http://192.168.50.1/" + strings.Repeat("a", 600),
		"file":         "file:///etc/passwd",
		"empty":        "",
	} {
		if _, err := ParseSSDP(answer(location), router); err == nil {
			t.Errorf("%s: %q must be refused", name, location)
		}
	}
}

func TestParseSSDP_RefusesAnAnswerThatIsNotOK(t *testing.T) {
	for _, raw := range []string{"", "garbage", "HTTP/1.1 404 Not Found\r\nLOCATION: http://192.168.50.1/x\r\n\r\n", "NOTIFY * HTTP/1.1\r\nLOCATION: http://192.168.50.1/x\r\n\r\n"} {
		if _, err := ParseSSDP([]byte(raw), router); err == nil {
			t.Errorf("%q must be refused", raw)
		}
	}
}
