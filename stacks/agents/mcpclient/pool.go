package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"
)

// ToolView is one tool of a server as the screen shows it.
type ToolView struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

// View is one server as the screen shows it: what the owner set (never the sign-in value) and how the connection is doing.
type View struct {
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Enabled       bool       `json:"enabled"`
	Trust         Trust      `json:"trust"`
	AuthKind      AuthKind   `json:"auth_kind"`
	AuthHeader    string     `json:"auth_header,omitempty"`
	HasSecret     bool       `json:"has_secret"`
	CACert        string     `json:"ca_cert,omitempty"`
	Status        string     `json:"status"`
	Problem       string     `json:"problem,omitempty"`
	ProblemCode   string     `json:"problem_code,omitempty"`
	ServerName    string     `json:"server_name,omitempty"`
	ServerVersion string     `json:"server_version,omitempty"`
	Protocol      string     `json:"protocol,omitempty"`
	Transport     string     `json:"transport,omitempty"`
	Instructions  string     `json:"instructions,omitempty"`
	Tools         []ToolView `json:"tools"`
	// DisabledTools are the tools the owner switched off, kept here so a screen can send them back while the server is down and lists none.
	DisabledTools []string `json:"disabled_tools"`
}

// StatusOff is the status of a server the owner switched off: it has no connection.
const StatusOff = "off"

// Pool keeps one connection for each enabled server.
type Pool struct {
	http   stacksdk.HTTP
	open   Opener
	log    stacksdk.Logger
	timing Timing

	mu     sync.Mutex
	root   context.Context
	conns  map[string]*conn
	all    []Server // every listed server, switched on or off, for the screen
	active bool     // false while the module is switched off
}

// NewPool returns a pool that opens no connection until Start and Reconcile.
func NewPool(h stacksdk.HTTP, open Opener, log stacksdk.Logger, tm Timing) *Pool {
	return &Pool{http: h, open: open, log: log, timing: tm, conns: map[string]*conn{}}
}

// Start gives the pool the context every connection lives under: when it ends, they all end.
func (p *Pool) Start(ctx context.Context) {
	p.mu.Lock()
	p.root = ctx
	p.mu.Unlock()
}

// Stop ends every connection and waits for them.
func (p *Pool) Stop() {
	p.mu.Lock()
	stopping := p.takeAll()
	p.root = nil
	p.mu.Unlock()
	stopAll(stopping)
}

func (p *Pool) takeAll() []*conn {
	out := make([]*conn, 0, len(p.conns))
	for name, c := range p.conns {
		out = append(out, c)
		delete(p.conns, name)
	}
	return out
}

func stopAll(list []*conn) {
	var wg sync.WaitGroup
	for _, c := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.stop()
		}()
	}
	wg.Wait()
}

// Reconcile makes the connections match the listed servers while the module is active (when it is not, nothing is dialled and every server shows as off): a new enabled server gets one, a removed or switched-off one loses it, a changed
// address or sign-in reconnects, and a change of trust or of the tools switched off takes effect without reconnecting. A server that is not valid is
// left without a connection.
func (p *Pool) Reconcile(servers []Server, active bool) {
	p.mu.Lock()
	if p.root == nil {
		p.mu.Unlock()
		return
	}
	p.all, p.active = slices.Clone(servers), active
	stopping, starting := p.diffLocked(wanted(servers, active))
	root := p.root
	p.mu.Unlock()
	stopAll(stopping)
	for _, c := range starting {
		c.start(root)
	}
}

// wanted is the entries that should have a connection: valid, switched on, and only while the module is active.
func wanted(servers []Server, active bool) map[string]Server {
	want := map[string]Server{}
	for _, s := range servers {
		s = s.Normalize()
		if active && s.Enabled && s.Validate() == nil {
			want[s.Name] = s
		}
	}
	return want
}

// diffLocked makes p.conns match want and returns the connections to stop and the new ones to start. A change of trust or of the tools switched off
// is applied in place; a change of address, sign-in or switch replaces the connection. The caller holds p.mu.
func (p *Pool) diffLocked(want map[string]Server) (stopping, starting []*conn) {
	for name, c := range p.conns {
		next, ok := want[name]
		if ok && c.server().sameConnection(next) {
			c.setServer(next)
			continue
		}
		stopping = append(stopping, c)
		delete(p.conns, name)
	}
	for name, s := range want {
		if _, ok := p.conns[name]; !ok {
			c := newConn(s, p.http, p.open, p.log, p.timing)
			p.conns[name] = c
			starting = append(starting, c)
		}
	}
	return stopping, starting
}

func (p *Pool) sorted() []*conn {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := make([]*conn, 0, len(p.conns))
	for _, c := range p.conns {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].server().Name < list[j].server().Name })
	return list
}

// Tools lists the tools of every ready server that the owner has not switched off, with the names the model sees. It answers from the caches and
// never waits on a server.
func (p *Pool) Tools() []stacksdk.DynamicTool {
	var out []stacksdk.DynamicTool
	seen := map[string]bool{}
	for _, c := range p.sorted() {
		st := c.snapshot()
		if st.Status != StatusReady {
			continue
		}
		srv := c.server()
		for _, t := range st.Tools {
			name := ToolName(srv.Name, t.Name, stacksdk.MaxDynamicToolName)
			if slices.Contains(srv.DisabledTools, t.Name) || seen[name] || len(out) >= stacksdk.MaxDynamicTools {
				continue
			}
			seen[name] = true
			out = append(out, stacksdk.DynamicTool{
				Name: name, Title: Text(t.Name, 120), Source: srv.Name, Description: t.Description, Schema: t.Schema, Ask: srv.Trust != TrustAllow,
			})
		}
	}
	return out
}

// find resolves a model-facing name to the connection and the server's own tool name.
func (p *Pool) find(name string) (*conn, string, bool) {
	for _, c := range p.sorted() {
		st := c.snapshot()
		if st.Status != StatusReady {
			continue
		}
		srv := c.server()
		for _, t := range st.Tools {
			if ToolName(srv.Name, t.Name, stacksdk.MaxDynamicToolName) == name && !slices.Contains(srv.DisabledTools, t.Name) {
				return c, t.Name, true
			}
		}
	}
	return nil, "", false
}

// maxErrorText bounds the server's own words in an error that is handed on.
const maxErrorText = 300

// Call runs one tool by the name Tools gave it.
func (p *Pool) Call(ctx context.Context, name string, args json.RawMessage) (stacksdk.ToolResult, error) {
	c, tool, ok := p.find(name)
	if !ok {
		return stacksdk.ToolResult{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no such tool is offered now: "+oneLine(name, 80))
	}
	var arguments map[string]any
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return stacksdk.ToolResult{}, stacksdk.Errorf(stacksdk.CodeInvalid, "the arguments must be a JSON object")
		}
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	res, err := c.call(ctx, tool, arguments)
	if err != nil {
		return stacksdk.ToolResult{}, callError(ctx, err)
	}
	return Result(res), nil
}

// callError turns a failed call into an *Error with a plain sentence; a server's own words are cleaned and cut.
func callError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotConnected):
		return stacksdk.Errorf(stacksdk.CodeUnavailable, "the server is not connected")
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		return stacksdk.Errorf(stacksdk.CodeTimeout, "the tool did not answer in time")
	case errors.Is(err, context.Canceled):
		return stacksdk.Errorf(stacksdk.CodeUnavailable, "the call was cancelled")
	}
	if phrase := classify(err); phrase != ProblemUnreachable && phrase != ProblemNotMCP {
		return stacksdk.Errorf(stacksdk.CodeUnavailable, phrase.Words())
	}
	return stacksdk.Errorf(stacksdk.CodeInternal, "the server answered with an error: "+oneLine(err.Error(), maxErrorText))
}

// Views describes every listed server, in the order they were listed.
func (p *Pool) Views() []View {
	p.mu.Lock()
	all, active := slices.Clone(p.all), p.active
	conns := make(map[string]*conn, len(p.conns))
	for k, v := range p.conns {
		conns[k] = v
	}
	p.mu.Unlock()
	out := make([]View, 0, len(all))
	for _, s := range all {
		s = s.Normalize()
		v := viewOf(s)
		if c, ok := conns[s.Name]; ok {
			v.apply(c.snapshot(), s)
		} else if s.Enabled && active {
			v.Status, v.Problem, v.ProblemCode = string(StatusStopped), problemOfInvalid(s), string(ProblemInvalid)
		} else {
			v.Status = StatusOff
		}
		out = append(out, v)
	}
	return out
}

func problemOfInvalid(s Server) string {
	if err := s.Validate(); err != nil {
		return "the entry is not valid: " + oneLine(err.Error(), 200)
	}
	return ""
}

func viewOf(s Server) View {
	return View{
		Name: s.Name, URL: s.URL, Enabled: s.Enabled, Trust: s.Trust, AuthKind: s.Auth.Kind, AuthHeader: s.Auth.Header,
		HasSecret: s.Auth.Sealed != "", CACert: s.CACert, Tools: []ToolView{}, DisabledTools: append([]string{}, s.DisabledTools...),
	}
}

func (v *View) apply(st State, s Server) {
	v.Status, v.Problem, v.ProblemCode = string(st.Status), st.Problem.Words(), string(st.Problem)
	v.ServerName, v.ServerVersion, v.Protocol, v.Transport, v.Instructions = st.Info.Name, st.Info.Version, st.Info.Protocol, st.Info.Transport, st.Info.Instructions
	for _, t := range st.Tools {
		v.Tools = append(v.Tools, ToolView{Name: t.Name, Description: t.Description, Disabled: slices.Contains(s.DisabledTools, t.Name)})
	}
}

// Test connects once, lists the tools and closes: it answers "does this work" for an entry that may not be saved. It holds no connection afterwards.
func (p *Pool) Test(ctx context.Context, s Server) View {
	s = s.Normalize()
	v := viewOf(s)
	if err := s.Validate(); err != nil {
		v.Status, v.Problem, v.ProblemCode = string(StatusStopped), problemOfInvalid(s), string(ProblemInvalid)
		return v
	}
	c := newConn(s, p.http, p.open, nil, p.timing)
	ctx, cancel := context.WithTimeout(ctx, 2*p.timing.Connect+p.timing.ListPage)
	defer cancel()
	cli, closeConn, err := c.connect(ctx)
	if err != nil {
		v.Status, v.Problem, v.ProblemCode = string(StatusStopped), classify(err).Words(), string(classify(err))
		return v
	}
	defer c.drop(cli, closeConn)
	if err := c.refresh(ctx, cli); err != nil {
		v.Status, v.Problem, v.ProblemCode = string(StatusStopped), classify(err).Words(), string(classify(err))
		return v
	}
	v.apply(c.snapshot(), s)
	return v
}

// defaultWait is how long a caller waits for a first connection when it asked for one.
const defaultWait = 5 * time.Second

// WaitReady waits until every enabled server has settled (ready or stopped), at most d; a test and the screen's refresh use it.
func (p *Pool) WaitReady(ctx context.Context, d time.Duration) {
	if d <= 0 {
		d = defaultWait
	}
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		settled := true
		for _, c := range p.sorted() {
			if c.snapshot().Status == StatusConnecting {
				settled = false
			}
		}
		if settled {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-tick.C:
		}
	}
}
