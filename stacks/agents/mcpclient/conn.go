package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"llm-proxy-stacks/sdk"
)

// Status is how one server's connection is doing.
type Status string

// The states of a connection. A server that is switched off has no connection at all.
const (
	StatusConnecting Status = "connecting"
	StatusReady      Status = "ready"
	StatusStopped    Status = "stopped"
)

// The transports the client speaks: the current one, and the old one a server from before it may still offer (specification, backwards compatibility).
const (
	TransportStreamable = "streamable_http"
	TransportSSE        = "sse"
)

// Problem is a short id for why a connection is not working. The screen keys its advice on the id; the words are fixed here and never carry text a
// server or a network sent.
type Problem string

// The problems a connection can report.
const (
	ProblemUnreachable Problem = "unreachable"
	ProblemGuard       Problem = "guard"
	ProblemSignIn      Problem = "sign_in"
	ProblemForbidden   Problem = "forbidden"
	ProblemNotMCP      Problem = "not_mcp"
	ProblemTLS         Problem = "tls"
	ProblemTimeout     Problem = "timeout"
	ProblemVersion     Problem = "version"
	ProblemSecret      Problem = "secret"
	ProblemSignInSet   Problem = "sign_in_incomplete"
	ProblemCAFile      Problem = "ca_file"
	ProblemClosed      Problem = "closed"
	ProblemTooMany     Problem = "too_many_tools"
	ProblemInvalid     Problem = "invalid"
)

var problemWords = map[Problem]string{
	ProblemUnreachable: "can't reach the server",
	ProblemGuard:       "the network guard refused this address",
	ProblemSignIn:      "the server wants a sign-in it was not given (401)",
	ProblemForbidden:   "the server refused the sign-in (403)",
	ProblemNotMCP:      "that address does not answer as an MCP server",
	ProblemTLS:         "the server's certificate is not trusted",
	ProblemTimeout:     "the server did not answer in time",
	ProblemVersion:     "the server speaks an MCP version this program does not",
	ProblemSecret:      "the stored sign-in value could not be opened",
	ProblemSignInSet:   "the sign-in is not complete: its value is missing",
	ProblemCAFile:      "the certificate authority file could not be read",
	ProblemClosed:      "the server closed the connection",
	ProblemTooMany:     "the server offers more tools than are shown",
	ProblemInvalid:     "the entry is not valid",
}

// Words is the fixed sentence for a problem; empty for no problem.
func (p Problem) Words() string { return problemWords[p] }

// maxPages bounds the pages of a tool listing: a server that never stops paging cannot hold the client.
const maxPages = 20

// clientName and clientVersion are what the client tells a server it is.
const (
	clientName    = "llm-proxy"
	clientVersion = "1.0"
)

// Timing holds every wait the connection uses; a test shortens them.
type Timing struct {
	Connect, ListPage, Call, Ping time.Duration
	PingEvery, Refresh            time.Duration
	BackoffMin, BackoffMax        time.Duration
}

// DefaultTiming is the timing a running stack uses.
func DefaultTiming() Timing {
	return Timing{
		Connect: 15 * time.Second, ListPage: 20 * time.Second, Call: 60 * time.Second, Ping: 10 * time.Second,
		PingEvery: 30 * time.Second, Refresh: 5 * time.Minute,
		BackoffMin: time.Second, BackoffMax: time.Minute,
	}
}

// Opener opens a sealed secret to the value it holds.
type Opener func(ctx context.Context, sealed string) (string, error)

// ErrNotConnected is the answer to a call or a listing while the server has no ready connection.
var ErrNotConnected = errors.New("the server is not connected")

// ToolInfo is one tool as the server described it, with its text cleaned and cut.
type ToolInfo struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// Info is what a server said about itself when the connection was made, cleaned and cut.
type Info struct {
	Name, Version, Protocol, Transport, Instructions string
}

// State is a copy of one connection's state.
type State struct {
	Status  Status
	Problem Problem
	Tools   []ToolInfo
	Info    Info
}

type conn struct {
	http   stacksdk.HTTP
	open   Opener
	log    stacksdk.Logger
	timing Timing

	mu        sync.RWMutex
	srv       Server
	state     State
	cli       *client.Client
	legacy    bool // the server answered only the old transport; reconnects go straight to it
	lostCh    chan struct{}
	refreshCh chan struct{}
	cancel    context.CancelFunc
	done      chan struct{}
}

func newConn(srv Server, http stacksdk.HTTP, open Opener, log stacksdk.Logger, tm Timing) *conn {
	return &conn{
		http: http, open: open, log: log, timing: tm, srv: srv,
		state:  State{Status: StatusConnecting},
		lostCh: make(chan struct{}, 1), refreshCh: make(chan struct{}, 1),
	}
}

// start runs the connection until ctx ends or stop is called. It returns at once.
func (c *conn) start(ctx context.Context) {
	run, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel, c.done = cancel, make(chan struct{})
	done := c.done
	c.mu.Unlock()
	go func() {
		defer close(done)
		c.run(run)
	}()
}

// stop ends the connection and waits for its goroutine.
func (c *conn) stop() {
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (c *conn) server() Server {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.srv
}

// setServer takes a change that needs no reconnect (the trust, the tools switched off).
func (c *conn) setServer(s Server) {
	c.mu.Lock()
	c.srv = s
	c.mu.Unlock()
}

// snapshot copies the state.
func (c *conn) snapshot() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	st := c.state
	st.Tools = slices.Clone(st.Tools)
	return st
}

func (c *conn) set(status Status, problem Problem) {
	c.mu.Lock()
	c.state.Status, c.state.Problem = status, problem
	if status != StatusReady {
		c.state.Tools = nil
	}
	c.mu.Unlock()
}

func (c *conn) logf(level stacksdk.Level, msg string, kv ...string) {
	if c.log != nil {
		c.log.Log(level, msg, append([]string{"server", c.server().Name}, kv...)...)
	}
}

// run is the connection's life: connect, serve until it is lost, wait, connect again.
func (c *conn) run(ctx context.Context) {
	backoff := c.timing.BackoffMin
	for ctx.Err() == nil {
		c.set(StatusConnecting, "")
		cli, connCancel, err := c.connect(ctx)
		if err != nil {
			c.set(StatusStopped, classify(err))
			c.logf(stacksdk.LevelWarn, "MCP server did not connect", "problem", string(classify(err)))
			if !sleep(ctx, jitter(backoff)) {
				return
			}
			backoff = min(backoff*2, c.timing.BackoffMax)
			continue
		}
		backoff = c.timing.BackoffMin
		c.serve(ctx, cli)
		c.drop(cli, connCancel)
	}
}

// drop closes a connection that ended and forgets it.
func (c *conn) drop(cli *client.Client, cancel context.CancelFunc) {
	c.mu.Lock()
	if c.cli == cli {
		c.cli = nil
	}
	c.mu.Unlock()
	cancel()
	_ = cli.Close()
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// jitter spreads a wait over its half and a half again, so several servers that were lost together do not retry together.
func jitter(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

// connect opens a connection, tries the old transport once if the server does not speak the current one, and checks the version the server chose.
// The returned cancel ends the connection's own context; the client's streams live on it, so it must not be a short one.
func (c *conn) connect(ctx context.Context) (*client.Client, context.CancelFunc, error) {
	srv := c.server()
	headers, hc, err := c.prepare(ctx, srv)
	if err != nil {
		return nil, nil, err
	}
	connCtx, cancel := context.WithCancel(ctx)
	c.mu.RLock()
	legacy := c.legacy
	c.mu.RUnlock()
	var cli *client.Client
	var res *mcp.InitializeResult
	if !legacy {
		cli, res, err = c.dial(connCtx, srv, headers, hc, false)
		if err != nil && legacyCandidate(err) {
			cli, res, err = c.dial(connCtx, srv, headers, hc, true)
			if err == nil {
				c.mu.Lock()
				c.legacy = true
				c.mu.Unlock()
			}
		}
	} else {
		cli, res, err = c.dial(connCtx, srv, headers, hc, true)
	}
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if !slices.Contains(mcp.ValidProtocolVersions, res.ProtocolVersion) {
		cancel()
		_ = cli.Close()
		return nil, nil, errVersion
	}
	c.mu.Lock()
	c.cli = cli
	c.state.Info = infoOf(res, transportName(c.legacy))
	c.mu.Unlock()
	return cli, cancel, nil
}

var errVersion = errors.New("unsupported protocol version")

func transportName(legacy bool) string {
	if legacy {
		return TransportSSE
	}
	return TransportStreamable
}

func infoOf(res *mcp.InitializeResult, transport string) Info {
	return Info{
		Name: Text(res.ServerInfo.Name, 80), Version: Text(res.ServerInfo.Version, 40), Protocol: res.ProtocolVersion, Transport: transport,
		Instructions: Text(res.Instructions, 2048),
	}
}

// prepare opens the sign-in value and builds the HTTP client: everything a connection needs from outside, none of it held between connections.
func (c *conn) prepare(ctx context.Context, srv Server) (map[string]string, httpClient, error) {
	secret := ""
	if srv.Auth.NeedsSecret() {
		if srv.Auth.Sealed == "" {
			return nil, nil, errSignInSet
		}
		v, err := c.open(ctx, srv.Auth.Sealed)
		if err != nil {
			return nil, nil, errSecret
		}
		secret = v
	}
	headers, err := srv.Auth.Headers(secret)
	if err != nil {
		return nil, nil, errSignInSet
	}
	opts := stacksdk.HTTPOptions{}
	if srv.CACert != "" {
		pem, err := os.ReadFile(srv.CACert)
		if err != nil {
			return nil, nil, errCAFile
		}
		opts.RootCAPEM = pem
	}
	hc, err := c.http.Client(opts)
	if err != nil {
		return nil, nil, errCAFile
	}
	return headers, hc, nil
}

var (
	errSecret    = errors.New("the sealed sign-in did not open")
	errSignInSet = errors.New("the sign-in is incomplete")
	errCAFile    = errors.New("the certificate authority file did not load")
)

// dial makes one client over one transport and runs the initialize exchange; a failure closes the client.
func (c *conn) dial(connCtx context.Context, srv Server, headers map[string]string, hc httpClient, legacy bool) (*client.Client, *mcp.InitializeResult, error) {
	var cli *client.Client
	var err error
	if legacy {
		cli, err = client.NewSSEMCPClient(srv.URL, transport.WithHTTPClient(hc), transport.WithHeaders(headers))
	} else {
		cli, err = client.NewStreamableHttpClient(srv.URL, transport.WithHTTPBasicClient(hc), transport.WithHTTPHeaders(headers), transport.WithContinuousListening())
	}
	if err != nil {
		return nil, nil, err
	}
	cli.OnConnectionLost(func(error) { signal(c.lostCh) })
	cli.OnNotification(func(n mcp.JSONRPCNotification) {
		if n.Method == mcp.MethodNotificationToolsListChanged {
			signal(c.refreshCh)
		}
	})
	if err := cli.Start(connCtx); err != nil {
		_ = cli.Close()
		return nil, nil, err
	}
	initCtx, cancel := context.WithTimeout(connCtx, c.timing.Connect)
	defer cancel()
	req := mcp.InitializeRequest{}
	req.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcp.Implementation{Name: clientName, Version: clientVersion}
	res, err := cli.Initialize(initCtx, req)
	if err != nil {
		_ = cli.Close()
		return nil, nil, err
	}
	return cli, res, nil
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// serve keeps a connection healthy: it lists the tools, then refreshes them when the server says they changed and now and then regardless (many
// servers never say), and pings. It returns when the connection is lost, the ping fails, or ctx ends.
func (c *conn) serve(ctx context.Context, cli *client.Client) {
	drain(c.lostCh)
	if err := c.refresh(ctx, cli); err != nil {
		c.set(StatusStopped, classify(err))
		c.logf(stacksdk.LevelWarn, "MCP tools could not be listed", "problem", string(classify(err)))
		return
	}
	ping, refresh := time.NewTicker(c.timing.PingEvery), time.NewTicker(c.timing.Refresh)
	defer ping.Stop()
	defer refresh.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.lostCh:
			c.set(StatusConnecting, ProblemClosed)
			return
		case <-c.refreshCh:
			c.refreshOrEnd(ctx, cli)
		case <-refresh.C:
			c.refreshOrEnd(ctx, cli)
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, c.timing.Ping)
			err := cli.Ping(pctx)
			cancel()
			if err != nil && ctx.Err() == nil {
				c.set(StatusConnecting, classify(err))
				return
			}
		}
	}
}

func drain(ch chan struct{}) {
	select {
	case <-ch:
	default:
	}
}

// refreshOrEnd refreshes the tool list while serving; a failure keeps the list the client already has, except a session that ended, which ends the
// connection so it is made again.
func (c *conn) refreshOrEnd(ctx context.Context, cli *client.Client) {
	if err := c.refresh(ctx, cli); err != nil && ctx.Err() == nil {
		c.logf(stacksdk.LevelWarn, "MCP tools could not be refreshed", "problem", string(classify(err)))
		if errors.Is(err, transport.ErrSessionTerminated) {
			signal(c.lostCh)
		}
	}
}

// refresh lists every page of tools and replaces the cache. The status becomes ready on the first success.
func (c *conn) refresh(ctx context.Context, cli *client.Client) error {
	var tools []ToolInfo
	more := false
	cursor := mcp.Cursor("")
	for page := 0; page < maxPages; page++ {
		pctx, cancel := context.WithTimeout(ctx, c.timing.ListPage)
		req := mcp.ListToolsRequest{}
		req.Params.Cursor = cursor
		res, err := cli.ListTools(pctx, req)
		cancel()
		if err != nil {
			return err
		}
		for _, t := range res.Tools {
			if len(tools) >= stacksdk.MaxDynamicTools {
				more = true
				break
			}
			tools = append(tools, toolInfo(t))
		}
		if res.NextCursor == "" || more {
			break
		}
		cursor = res.NextCursor
		if page == maxPages-1 {
			more = true
		}
	}
	var problem Problem
	if more {
		problem = ProblemTooMany
	}
	c.mu.Lock()
	c.state.Status, c.state.Problem, c.state.Tools = StatusReady, problem, tools
	c.mu.Unlock()
	return nil
}

// toolInfo cleans one tool's text and renders its schema; a tool that carries none gets an empty object schema so the model can still call it.
func toolInfo(t mcp.Tool) ToolInfo {
	schema := t.RawInputSchema
	if len(schema) == 0 {
		schema, _ = json.Marshal(t.InputSchema)
	}
	if len(schema) == 0 || len(schema) > stacksdk.MaxDynamicToolSchema || !json.Valid(schema) {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return ToolInfo{Name: t.Name, Description: Text(t.Description, stacksdk.MaxDynamicToolDescription), Schema: schema}
}

// call runs one tool. A session the server ended (a 404 for a session it issued) is made again and the call is tried once more, as the
// specification requires of a client.
func (c *conn) call(ctx context.Context, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	res, err := c.callOnce(ctx, tool, args)
	if errors.Is(err, transport.ErrSessionTerminated) {
		signal(c.lostCh)
		if !c.waitReady(ctx, c.timing.Connect) {
			return nil, ErrNotConnected
		}
		res, err = c.callOnce(ctx, tool, args)
	}
	return res, err
}

func (c *conn) callOnce(ctx context.Context, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	c.mu.RLock()
	cli, ready := c.cli, c.state.Status == StatusReady
	c.mu.RUnlock()
	if cli == nil || !ready {
		return nil, ErrNotConnected
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timing.Call)
	defer cancel()
	req := mcp.CallToolRequest{}
	req.Params.Name = tool
	req.Params.Arguments = args
	return cli.CallTool(callCtx, req)
}

// waitReady waits for the connection to be ready again, at most d.
func (c *conn) waitReady(ctx context.Context, d time.Duration) bool {
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	time.Sleep(5 * time.Millisecond) // let the loop see the signal before the old state is read as ready
	for {
		c.mu.RLock()
		ready := c.state.Status == StatusReady && c.cli != nil
		c.mu.RUnlock()
		if ready {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-tick.C:
		}
	}
}

var statusPattern = regexp.MustCompile(`status (\d{3})`)

// statusOf reads the HTTP status out of the transport's error text, which has no type of its own; 0 when it names none.
func statusOf(err error) int {
	m := statusPattern.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// legacyCandidate reports whether a failed first request may mean the server speaks only the old transport: the specification names 400, 404 and 405.
func legacyCandidate(err error) bool {
	switch statusOf(err) {
	case 400, 404, 405:
		return true
	}
	return false
}

// classify turns any connection failure into one of the fixed phrases.
func classify(err error) Problem {
	var coded *stacksdk.Error
	text := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, errSecret):
		return ProblemSecret
	case errors.Is(err, errSignInSet):
		return ProblemSignInSet
	case errors.Is(err, errCAFile):
		return ProblemCAFile
	case errors.Is(err, errVersion):
		return ProblemVersion
	case errors.As(err, &coded) && coded.Code == stacksdk.CodeDenied:
		return ProblemGuard
	case errors.Is(err, context.DeadlineExceeded):
		return ProblemTimeout
	case statusOf(err) == 401:
		return ProblemSignIn
	case statusOf(err) == 403:
		return ProblemForbidden
	case statusOf(err) != 0, strings.Contains(text, "unexpected content type"), strings.Contains(text, "failed to decode"), strings.Contains(text, "invalid character"):
		return ProblemNotMCP
	case strings.Contains(text, "x509"), strings.Contains(text, "certificate"):
		return ProblemTLS
	}
	return ProblemUnreachable
}
