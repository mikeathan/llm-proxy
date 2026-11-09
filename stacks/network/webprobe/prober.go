package webprobe

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
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
	// DefaultPace is the wait between two requests to the same service, so a small device is asked gently.
	DefaultPace = 200 * time.Millisecond
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

// Hit is a check that matched on one service. Only the id leaves the stack: the words and the severity of a finding are the core's.
type Hit struct {
	Addr netip.Addr
	Port int
	ID   string
}

// Result is what a run found and whether its time limit ended it before every service was asked.
type Result struct {
	Hits    []Hit
	Partial bool
}

// Dial opens one connection. In the Network stack it is the host's scoped dial, which refuses an address outside the operator's declared
// scan scope and applies the guardrails at the moment of connecting (ADR-0038 rule 2), so every connection of these checks, web or certificate,
// is made by it and a name, a redirect or a second address cannot lead anywhere else.
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// Prober asks the devices' web servers the fixed questions of Checks.
type Prober struct {
	dial Dial
	pace time.Duration
}

// New builds a prober that connects only through dial.
func New(dial Dial) *Prober {
	return &Prober{dial: dial, pace: DefaultPace}
}

// WithPace sets the wait between two requests to the same service; zero means no wait. The default is DefaultPace.
func (p *Prober) WithPace(d time.Duration) *Prober {
	p.pace = d
	return p
}

// dialWithin opens a connection with at most dialTimeout to do it; the connection itself outlives the timeout.
func dialWithin(ctx context.Context, dial Dial, network, address string) (net.Conn, error) {
	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	return dial(dctx, network, address)
}

// validate refuses a run before anything is sent: no target or too many, no time limit, or an address that is not IPv4 or a bad port. The
// scan scope is the caller's to check before it asks (the core does) and the dial's to enforce at every connection. what names the check.
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
	}
	return nil
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
			hits = append(hits, Hit{Addr: t.Addr, Port: t.Port, ID: c.ID})
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

// fetch does one GET and returns the start of the answer, or nil when the device did not answer. It writes one plain request on a connection
// made by the dial and reads one response from it, and so has no way to follow a redirect, use a proxy, keep a connection or connect anywhere
// else: the request names one address and the connection is closed after the answer. Everything read is bounded: the headers and the body
// together cannot pass maxHeaderBytes plus maxBody, whatever the device sends, and the whole exchange ends at requestTimeout. The certificate
// of a secure service is not checked: a device on the home network rarely has one a browser trusts, and nothing is sent that a certificate
// would protect.
func (p *Prober) fetch(ctx context.Context, t Target, path string) *Response {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	conn, err := dialWithin(rctx, p.dial, "tcp", net.JoinHostPort(t.Addr.String(), strconv.Itoa(t.Port)))
	if err != nil {
		return nil
	}
	defer conn.Close()
	stop := context.AfterFunc(rctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := rctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	var wire net.Conn = conn
	if t.TLS {
		secure := tls.Client(conn, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- see the note above: read-only requests to a private address, nothing secret is sent
		if secure.HandshakeContext(rctx) != nil {
			return nil
		}
		wire = secure
	}
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, t.base()+path, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	req.Close = true // sends "Connection: close"
	if req.Write(wire) != nil {
		return nil
	}
	resp, err := http.ReadResponse(bufio.NewReaderSize(io.LimitReader(wire, maxHeaderBytes+maxBody), maxHeaderBytes), req)
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
