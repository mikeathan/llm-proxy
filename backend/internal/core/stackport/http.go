package stackport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"strings"
	"time"

	"llm-proxy-stacks/sdk"
)

// Fixed sentences a stack is told: never the standard library's own wording, which can quote a certificate or an address.
const (
	msgNoCert        = "the certificate authority file holds no certificate"
	msgTooManyRedirs = "too many redirects"
	msgRedirectHost  = "the server redirected to another host"
	msgRedirectPlain = "the server redirected from https to http"
)

// The limits of the client a stack is given.
const (
	httpMaxRedirects    = 5
	httpTLSHandshake    = 10 * time.Second
	httpIdleConnTimeout = 90 * time.Second
	httpIdlePerHost     = 2
	httpExpectContinue  = time.Second
)

// HTTP is the SDK's HTTP port: a client whose every connection is dialled through the guard (NetworkTools.DialContext), so DNS-rebinding protection
// and the LAN-versus-internet boundary apply at the socket as they do for every other outbound connection (Constitution I.1 to I.3). It never uses the
// environment's proxy (the guard must see the real destination), follows a redirect only to the same host and never from https to http, and has no
// overall timeout because a streamed answer must not be cut: the caller bounds each call with a context.
type HTTP struct {
	Dial Dial
}

// Client builds a client; it is cheap, and each connection of a stack makes its own so one server's keep-alives never reach another's.
func (h HTTP) Client(opts stacksdk.HTTPOptions) (*http.Client, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if len(opts.RootCAPEM) > 0 && !pool.AppendCertsFromPEM(opts.RootCAPEM) {
		return nil, stacksdk.Errorf(stacksdk.CodeInvalid, msgNoCert)
	}
	transport := &http.Transport{
		DialContext:           h.Dial,
		Proxy:                 nil, // the guard checks the destination itself, never a proxy standing in for it
		TLSClientConfig:       &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   httpTLSHandshake,
		IdleConnTimeout:       httpIdleConnTimeout,
		MaxIdleConnsPerHost:   httpIdlePerHost,
		ExpectContinueTimeout: httpExpectContinue,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Transport: transport, CheckRedirect: sameHostRedirect}, nil
}

// sameHostRedirect follows a redirect only to the host (and port) the request went to, and never from https down to http.
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= httpMaxRedirects {
		return errors.New(msgTooManyRedirs)
	}
	first := via[0]
	if !strings.EqualFold(req.URL.Host, first.URL.Host) {
		return errors.New(msgRedirectHost)
	}
	if first.URL.Scheme == "https" && req.URL.Scheme != "https" {
		return errors.New(msgRedirectPlain)
	}
	return nil
}

var _ stacksdk.HTTP = HTTP{}
