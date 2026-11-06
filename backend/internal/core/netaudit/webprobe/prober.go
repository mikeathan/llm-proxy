package webprobe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"syscall"
	"time"
)

const (
	// MaxTargets is the most web services one run is aimed at.
	MaxTargets = 64
	// maxBody is how much of an answer is read; a longer answer is cut, and the cut part is never looked at.
	maxBody = 256 << 10
	// requestTimeout bounds one request, from the first byte sent to the last read.
	requestTimeout = 8 * time.Second
	// dialTimeout bounds opening one connection.
	dialTimeout = 5 * time.Second
	// maxHeaderBytes bounds the headers of an answer.
	maxHeaderBytes = 64 << 10
	// defaultPace is the wait between two requests to the same service, so a small device is asked gently.
	defaultPace = 200 * time.Millisecond
	// userAgent names this program to the device, plainly.
	userAgent = "llm-proxy-network-audit"
)

// Target is one web service on one device: an address already approved by the scan scope, a port, and whether it speaks TLS.
type Target struct {
	Addr netip.Addr
	Port int
	TLS  bool
}

func (t Target) base() string {
	scheme := "http"
	if t.TLS {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(t.Addr.String(), strconv.Itoa(t.Port))
}

// Hit is a check that matched on one service. Title and Severity are this package's words.
type Hit struct {
	Addr     netip.Addr
	Port     int
	ID       string
	Title    string
	Severity string
}

// Result is what a run found and whether its time limit ended it before every service was asked.
type Result struct {
	Hits    []Hit
	Partial bool
}

// Doer sends one request: *http.Client satisfies it.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Guard approves an address for a connection: the scan scope's policy. It is asked before a run and again at every connection.
type Guard func(netip.Addr) error

// Prober asks the devices' web servers the fixed questions of Checks.
type Prober struct {
	doer  Doer
	guard Guard
	pace  time.Duration
}

// New builds a prober that connects through NewClient. The guard is the scan scope's policy.
func New(guard Guard) *Prober {
	return &Prober{doer: NewClient(guard), guard: guard, pace: defaultPace}
}

// NewClient is the only HTTP client these checks use. It never follows a redirect, never uses a proxy, never keeps a connection,
// checks every address it connects to against the guard (so a name or a redirect cannot lead it elsewhere), and does not check
// certificates: a device on the home network rarely has one a browser trusts, and nothing is sent that a certificate would protect.
func NewClient(guard Guard) *http.Client {
	dialer := guardedDialer(guard)
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		TLSClientConfig:        &tls.Config{InsecureSkipVerify: true}, // #nosec G402 -- see NewClient: read-only requests to a private address, nothing secret is sent
		TLSHandshakeTimeout:    dialTimeout,
		ResponseHeaderTimeout:  requestTimeout,
		MaxResponseHeaderBytes: maxHeaderBytes,
		DisableKeepAlives:      true,
	}
	return &http.Client{
		Transport:     transport,
		Timeout:       requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// validate refuses a run before anything is sent: no target or too many, no time limit, an address that is not IPv4 or a bad port, or an
// address the guard does not approve. what names the check in the error.
func (p *Prober) validate(what string, targets []Target, limit time.Duration) error {
	if len(targets) == 0 || len(targets) > MaxTargets {
		return fmt.Errorf("%s: %d targets, want 1 to %d", what, len(targets), MaxTargets)
	}
	if limit <= 0 {
		return fmt.Errorf("%s: a run needs a time limit", what)
	}
	for _, t := range targets {
		if !t.Addr.Is4() || t.Port < 1 || t.Port > 65535 {
			return fmt.Errorf("%s: %s:%d is not an IPv4 address and port", what, t.Addr, t.Port)
		}
		if err := p.guard(t.Addr); err != nil {
			return fmt.Errorf("%s: %s is outside the scan scope: %w", what, t.Addr, err)
		}
	}
	return nil
}

// guardedDialer opens a connection only to an address the guard approves, checked at the moment of connecting, so a name, a redirect
// or a second address cannot lead it elsewhere. Every connection of these checks, web or certificate, is made by it.
func guardedDialer(guard Guard) *net.Dialer {
	return &net.Dialer{
		Timeout: dialTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("failed to read the address %q: %w", address, err)
			}
			addr, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("failed to read the address %q: %w", address, err)
			}
			return guard(addr.Unmap())
		},
	}
}

// Probe asks every target the questions of Checks and returns what matched. limit ends the run: what was found by then is returned
// with Partial set. A target the guard refuses is an error before anything is sent; a device that does not answer is not.
func (p *Prober) Probe(ctx context.Context, targets []Target, limit time.Duration) (Result, error) {
	if err := p.validate("web checks", targets, limit); err != nil {
		return Result{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	var res Result
	for _, t := range targets {
		hits, done := p.probeTarget(runCtx, t)
		res.Hits = append(res.Hits, hits...)
		if !done {
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			res.Partial = true
			break
		}
	}
	return res, nil
}

// probeTarget fetches each distinct path once and applies every check to what came back. It reports false when ctx ended first.
func (p *Prober) probeTarget(ctx context.Context, t Target) ([]Hit, bool) {
	answers := map[string]*Response{}
	for _, c := range Checks {
		for _, path := range c.Paths {
			if _, asked := answers[path]; asked {
				continue
			}
			if len(answers) > 0 && !p.wait(ctx) {
				return nil, false
			}
			answers[path] = p.fetch(ctx, t, path)
			if ctx.Err() != nil {
				return nil, false
			}
		}
	}
	var hits []Hit
	for _, c := range Checks {
		if matched(c, answers) {
			hits = append(hits, Hit{Addr: t.Addr, Port: t.Port, ID: c.ID, Title: c.Title, Severity: c.Severity})
		}
	}
	return hits, true
}

func matched(c Check, answers map[string]*Response) bool {
	return slices.ContainsFunc(c.Paths, func(path string) bool {
		r := answers[path]
		return r != nil && c.Match(*r)
	})
}

// wait pauses between two requests; false when ctx ended during the wait.
func (p *Prober) wait(ctx context.Context) bool {
	if p.pace <= 0 {
		return ctx.Err() == nil
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(p.pace):
		return true
	}
}

// fetch does one GET and returns the start of the answer, or nil when the device did not answer.
func (p *Prober) fetch(ctx context.Context, t Target, path string) *Response {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.base()+path, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	resp, err := p.doer.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil && len(body) == 0 {
		return nil
	}
	return &Response{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}
}
