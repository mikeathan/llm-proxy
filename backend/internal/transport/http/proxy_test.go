package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fromPeer(peer string, headers ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/admin/api/auth/status", nil)
	r.RemoteAddr = peer
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Add(headers[i], headers[i+1])
	}
	return r
}

// X-Forwarded-Proto is believed only from a listed proxy: anyone can send it, and believing it from a stranger would
// make the cookie Secure (or "encrypted" true) on a plain connection.
func TestTrustedProxies_IsHTTPS(t *testing.T) {
	trusted := []string{"127.0.0.1", "10.1.0.0/16", "::1"}
	cases := []struct {
		name string
		r    *http.Request
		list []string
		want bool
	}{
		{"plain request", fromPeer("203.0.113.9:1"), trusted, false},
		{"header from a stranger", fromPeer("203.0.113.9:1", "X-Forwarded-Proto", "https"), trusted, false},
		{"header from a listed address", fromPeer("127.0.0.1:1", "X-Forwarded-Proto", "https"), trusted, true},
		{"header from inside a listed range", fromPeer("10.1.4.4:1", "X-Forwarded-Proto", "https"), trusted, true},
		{"header from outside the range", fromPeer("10.2.4.4:1", "X-Forwarded-Proto", "https"), trusted, false},
		{"IPv6 proxy", fromPeer("[::1]:1", "X-Forwarded-Proto", "https"), trusted, true},
		{"IPv4 proxy seen as IPv4-mapped IPv6", fromPeer("[::ffff:127.0.0.1]:1", "X-Forwarded-Proto", "https"), trusted, true},
		{"proxy says http", fromPeer("127.0.0.1:1", "X-Forwarded-Proto", "http"), trusted, false},
		{"proxy sends nothing", fromPeer("127.0.0.1:1"), trusted, false},
		{"case and spaces", fromPeer("127.0.0.1:1", "X-Forwarded-Proto", " HTTPS "), trusted, true},
		{"nearest proxy's value wins over a client's", fromPeer("127.0.0.1:1", "X-Forwarded-Proto", "https, http"), trusted, false},
		{"client forged https, nearest proxy says http", fromPeer("127.0.0.1:1", "X-Forwarded-Proto", "https", "X-Forwarded-Proto", "http"), trusted, false},
		{"client said http, nearest proxy says https", fromPeer("127.0.0.1:1", "X-Forwarded-Proto", "http, https"), trusted, true},
		{"nothing is trusted by default", fromPeer("127.0.0.1:1", "X-Forwarded-Proto", "https"), nil, false},
		{"a bad entry trusts nothing", fromPeer("127.0.0.1:1", "X-Forwarded-Proto", "https"), []string{"not-an-address", "10.0.0.0/99"}, false},
		{"an unparseable peer is a stranger", fromPeer("garbage", "X-Forwarded-Proto", "https"), trusted, false},
	}
	for _, tc := range cases {
		list := tc.list
		if got := IsHTTPS(tc.r, func() []string { return list }); got != tc.want {
			t.Errorf("%s: IsHTTPS = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsHTTPS_TLSAndNoList(t *testing.T) {
	r := fromPeer("203.0.113.9:1")
	r.TLS = &tls.ConnectionState{}
	if !IsHTTPS(r, nil) {
		t.Error("a request that arrived over TLS is encrypted whatever the list")
	}
	if IsHTTPS(fromPeer("127.0.0.1:1", "X-Forwarded-Proto", "https"), nil) {
		t.Error("no list function trusts no proxy")
	}
}

func TestInvalidProxyEntries(t *testing.T) {
	got := InvalidProxyEntries([]string{"127.0.0.1", "10.0.0.0/8", "::1", "nope", "10.0.0.0/99", ""})
	if len(got) != 3 || got[0] != "nope" || got[1] != "10.0.0.0/99" || got[2] != "" {
		t.Errorf("InvalidProxyEntries = %q", got)
	}
}
