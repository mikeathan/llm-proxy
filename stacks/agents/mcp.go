package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"llm-proxy-stacks/agents/mcpclient"
	"llm-proxy-stacks/sdk"
)

// The module, the routes and the vault purpose of the Tools layer (ADR-0069).
const (
	MCPModuleID = "mcp_servers"
	// RouteMCPServers lists (GET), adds (POST), changes (PUT, the name in the body) and removes (DELETE, the name in the query) a server. A route that needs the
	// passphrase again has no wildcard in its path, because the core tells those by the exact path (ADR-0033).
	RouteMCPServers = "/admin/api/mcp/servers"
	// RouteMCPTest tries a server once and says what happened: the saved one named in the body, or, when the body holds an address, the entry as typed.
	RouteMCPTest = "/admin/api/mcp/test"
	vaultPurpose = "mcp"
	maxServers   = 16
	maxBody      = 64 * 1024
	// ProblemMCPServer is the fixed phrase the module reports while a server that is switched on cannot be used.
	ProblemMCPServer = "a server could not be reached"
	// settingsTimeout bounds a read of the settings document on a path that must not wait.
	settingsTimeout = 2 * time.Second
)

// errMCPOff is the refusal of a call while the module is switched off.
var errMCPOff = stacksdk.Errorf(stacksdk.CodeUnavailable, "MCP servers are switched off: switch them on under Stacks")

// mcpState is what the Tools layer holds while the stack runs.
type mcpState struct {
	pool *mcpclient.Pool
	host stacksdk.Host
}

// startMCP builds the pool over the host's ports under ctx and connects the servers the owner listed, unless the module is off. A host without the
// ports leaves the layer unavailable and the rest of the stack unchanged.
func (s *Stack) startMCP(ctx context.Context, h stacksdk.Host) {
	if h.HTTP == nil || h.Vault == nil {
		return
	}
	open := func(ctx context.Context, sealed string) (string, error) {
		v, err := h.Vault.Open(ctx, vaultPurpose, []byte(sealed))
		return string(v), err
	}
	pool := mcpclient.NewPool(h.HTTP, open, h.Log, mcpclient.DefaultTiming())
	pool.Start(ctx)
	s.mu.Lock()
	s.mcp = &mcpState{pool: pool, host: h}
	s.mu.Unlock()
	s.reconcileMCP(ctx)
}

// stopMCP ends every connection and waits for them.
func (s *Stack) stopMCP() {
	s.mu.Lock()
	m := s.mcp
	s.mcp = nil
	s.mu.Unlock()
	if m != nil {
		m.pool.Stop()
	}
}

func (s *Stack) mcpPool() *mcpclient.Pool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mcp == nil {
		return nil
	}
	return s.mcp.pool
}

// reconcileMCP makes the connections match the settings, which are read afresh.
func (s *Stack) reconcileMCP(ctx context.Context) {
	pool := s.mcpPool()
	if pool == nil {
		return
	}
	set := s.w.settings(ctx)
	pool.Reconcile(set.MCPServers, !set.MCPOff)
}

// ListTools offers the tools of the ready servers. It answers from the caches and never waits on a server; with the module off, the stack stopped or
// no server ready it offers nothing.
func (s *Stack) ListTools(ctx context.Context) ([]stacksdk.DynamicTool, error) {
	pool := s.mcpPool()
	if pool == nil || s.mcpOff(ctx) {
		return nil, nil
	}
	return pool.Tools(), nil
}

// CallDynamic runs one tool by the name ListTools gave it.
func (s *Stack) CallDynamic(ctx context.Context, name string, args json.RawMessage) (stacksdk.ToolResult, error) {
	pool := s.mcpPool()
	if pool == nil {
		return stacksdk.ToolResult{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the agents stack is not running")
	}
	if s.mcpOff(ctx) {
		return stacksdk.ToolResult{}, errMCPOff
	}
	return pool.Call(ctx, name, args)
}

func (s *Stack) mcpOff(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, settingsTimeout)
	defer cancel()
	return s.w.settings(ctx).MCPOff
}

// mcpStatus is the module's status for the screen: on or off, and one fixed phrase when a server that is switched on is not working.
func (s *Stack) mcpStatus(ctx context.Context) stacksdk.ModuleStatus {
	st := stacksdk.ModuleStatus{ID: MCPModuleID, Enabled: !s.mcpOff(ctx)}
	if pool := s.mcpPool(); pool != nil && st.Enabled {
		for _, v := range pool.Views() {
			if v.Enabled && v.Status == string(mcpclient.StatusStopped) {
				st.Problem = ProblemMCPServer
			}
		}
	}
	return st
}

// serverBody is what a route reads: the fields of an entry, and the sign-in value in the clear, which is sealed at once and never kept or returned.
type serverBody struct {
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Enabled       *bool    `json:"enabled"`
	Trust         string   `json:"trust"`
	AuthKind      string   `json:"auth_kind"`
	AuthHeader    string   `json:"auth_header"`
	Secret        *string  `json:"secret"`
	CACert        string   `json:"ca_cert"`
	DisabledTools []string `json:"disabled_tools"`
}

// listAnswer is the answer of the list route: the module's switch, so the page can say why nothing runs, and the servers.
type listAnswer struct {
	Enabled bool             `json:"enabled"`
	Servers []mcpclient.View `json:"servers"`
}

type mcpRoute func(context.Context, *mcpclient.Pool, stacksdk.Request) stacksdk.Response

// handleMCP answers the Tools layer's routes. found is false when the request is none of them.
func (s *Stack) handleMCP(ctx context.Context, req stacksdk.Request) (resp stacksdk.Response, found bool) {
	fn := s.mcpRouteFor(req)
	if fn == nil {
		return stacksdk.Response{}, false
	}
	pool := s.mcpPool()
	if pool == nil {
		return failure(http.StatusServiceUnavailable, "the agents stack is not running"), true
	}
	return fn(ctx, pool, req), true
}

func (s *Stack) mcpRouteFor(req stacksdk.Request) mcpRoute {
	switch req.Method + " " + req.Path {
	case http.MethodGet + " " + RouteMCPServers:
		return s.mcpList
	case http.MethodPost + " " + RouteMCPServers:
		return s.mcpAdd
	case http.MethodPut + " " + RouteMCPServers:
		return s.mcpChange
	case http.MethodDelete + " " + RouteMCPServers:
		return s.mcpRemove
	case http.MethodPost + " " + RouteMCPTest:
		return s.mcpTest
	}
	return nil
}

func (s *Stack) mcpList(ctx context.Context, pool *mcpclient.Pool, _ stacksdk.Request) stacksdk.Response {
	return ok(listAnswer{Enabled: !s.mcpOff(ctx), Servers: pool.Views()})
}

func decodeBody(req stacksdk.Request) (serverBody, string) {
	var b serverBody
	if len(req.Body) > maxBody {
		return b, "the request is too large"
	}
	if err := json.Unmarshal(req.Body, &b); err != nil {
		return b, "the request must be a JSON object"
	}
	return b, ""
}

// entryOf builds the entry a body describes, without the sign-in value, which the caller seals.
func (b serverBody) entryOf() mcpclient.Server {
	enabled := true
	if b.Enabled != nil {
		enabled = *b.Enabled
	}
	return mcpclient.Server{
		Name: b.Name, URL: b.URL, Enabled: enabled, Trust: mcpclient.Trust(b.Trust),
		Auth:   mcpclient.Auth{Kind: mcpclient.AuthKind(b.AuthKind), Header: b.AuthHeader},
		CACert: b.CACert, DisabledTools: b.DisabledTools,
	}.Normalize()
}

func (s *Stack) mcpAdd(ctx context.Context, pool *mcpclient.Pool, req stacksdk.Request) stacksdk.Response {
	b, msg := decodeBody(req)
	if msg != "" {
		return failure(http.StatusBadRequest, msg)
	}
	srv := b.entryOf()
	if err := srv.Validate(); err != nil {
		return failure(http.StatusBadRequest, messageOf(err))
	}
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	servers := s.w.settings(ctx).MCPServers
	switch {
	case slices.ContainsFunc(servers, func(o mcpclient.Server) bool { return o.Name == srv.Name }):
		return failure(http.StatusConflict, "a server with that name is already listed")
	case len(servers) >= maxServers:
		return failure(http.StatusBadRequest, "at most 16 servers can be listed")
	}
	sealed, msg := s.sealSecret(ctx, srv.Auth, b.Secret)
	if msg != "" {
		return failure(http.StatusBadRequest, msg)
	}
	srv.Auth.Sealed = sealed
	if err := s.saveServers(ctx, append(slices.Clone(servers), srv)); err != nil {
		return failure(http.StatusInternalServerError, "failed to save the server")
	}
	return s.afterWrite(ctx, pool, srv.Name, http.StatusCreated)
}

func (s *Stack) mcpChange(ctx context.Context, pool *mcpclient.Pool, req stacksdk.Request) stacksdk.Response {
	b, msg := decodeBody(req)
	if msg != "" {
		return failure(http.StatusBadRequest, msg)
	}
	name := b.Name
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	servers := s.w.settings(ctx).MCPServers
	at := slices.IndexFunc(servers, func(o mcpclient.Server) bool { return o.Name == name })
	if at < 0 {
		return failure(http.StatusNotFound, "no server with that name")
	}
	old := servers[at]
	srv := b.entryOf()
	if err := srv.Validate(); err != nil {
		return failure(http.StatusBadRequest, messageOf(err))
	}
	switch {
	case b.Secret != nil:
		sealed, msg := s.sealSecret(ctx, srv.Auth, b.Secret)
		if msg != "" {
			return failure(http.StatusBadRequest, msg)
		}
		srv.Auth.Sealed = sealed
	case !srv.Auth.NeedsSecret():
		srv.Auth.Sealed = ""
	case old.Auth.Kind != srv.Auth.Kind || !sameHost(old.URL, srv.URL):
		// A stored value goes only to the host it was given for: a new address, or a new way of signing in, needs the value again.
		return failure(http.StatusBadRequest, "the address or the sign-in changed, so enter the sign-in value again: a stored value is never sent to another address")
	default:
		srv.Auth.Sealed = old.Auth.Sealed
	}
	servers = slices.Clone(servers)
	servers[at] = srv
	if err := s.saveServers(ctx, servers); err != nil {
		return failure(http.StatusInternalServerError, "failed to save the server")
	}
	return s.afterWrite(ctx, pool, name, http.StatusOK)
}

func (s *Stack) mcpRemove(ctx context.Context, _ *mcpclient.Pool, req stacksdk.Request) stacksdk.Response {
	query, err := url.ParseQuery(req.Query)
	name := query.Get("name")
	if err != nil || name == "" {
		return failure(http.StatusBadRequest, "the name of the server to remove is required")
	}
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	servers := s.w.settings(ctx).MCPServers
	at := slices.IndexFunc(servers, func(o mcpclient.Server) bool { return o.Name == name })
	if at < 0 {
		return failure(http.StatusNotFound, "no server with that name")
	}
	if err := s.saveServers(ctx, slices.Delete(slices.Clone(servers), at, at+1)); err != nil {
		return failure(http.StatusInternalServerError, "failed to remove the server")
	}
	s.reconcileMCP(ctx)
	return stacksdk.Response{Status: http.StatusNoContent}
}

// mcpTest tries a server once. A body with no address names a saved server; a body with an address is an entry that is not saved, tried with the
// sign-in value the person typed, sealed in memory for the one attempt and then forgotten.
func (s *Stack) mcpTest(ctx context.Context, pool *mcpclient.Pool, req stacksdk.Request) stacksdk.Response {
	b, msg := decodeBody(req)
	if msg != "" {
		return failure(http.StatusBadRequest, msg)
	}
	if b.URL == "" {
		return s.mcpTestSaved(ctx, pool, b.Name)
	}
	if b.Name == "" {
		b.Name = "test"
	}
	srv := b.entryOf()
	srv.Enabled = true
	if srv.Auth.NeedsSecret() {
		sealed, msg := s.sealSecret(ctx, srv.Auth, b.Secret)
		if msg != "" {
			return failure(http.StatusBadRequest, msg)
		}
		srv.Auth.Sealed = sealed
	}
	return ok(pool.Test(ctx, srv))
}

func (s *Stack) mcpTestSaved(ctx context.Context, pool *mcpclient.Pool, name string) stacksdk.Response {
	servers := s.w.settings(ctx).MCPServers
	at := slices.IndexFunc(servers, func(o mcpclient.Server) bool { return o.Name == name })
	if at < 0 {
		return failure(http.StatusNotFound, "no server with that name")
	}
	srv := servers[at]
	srv.Enabled = true
	return ok(pool.Test(ctx, srv))
}

// sealSecret seals the typed value for a sign-in that carries one. A sign-in with none gives no value; one that needs it and got none is refused.
func (s *Stack) sealSecret(ctx context.Context, auth mcpclient.Auth, secret *string) (sealed, msg string) {
	if !auth.NeedsSecret() {
		return "", ""
	}
	if secret == nil || strings.TrimSpace(*secret) == "" {
		return "", "the sign-in value is required"
	}
	if len(*secret) > stacksdk.MaxVaultBytes {
		return "", "the sign-in value is too long"
	}
	s.mu.Lock()
	m := s.mcp
	s.mu.Unlock()
	out, err := m.host.Vault.Seal(ctx, vaultPurpose, []byte(strings.TrimSpace(*secret)))
	if err != nil {
		return "", "the sign-in value could not be sealed"
	}
	return string(out), ""
}

// saveServers writes the list into the stack's settings; the Config port merges it over the other fields.
func (s *Stack) saveServers(ctx context.Context, servers []mcpclient.Server) error {
	doc, err := json.Marshal(map[string][]mcpclient.Server{"mcp_servers": servers})
	if err != nil {
		return err
	}
	return s.w.config.Save(ctx, doc)
}

// afterWrite connects what changed and answers with the entry as the screen shows it.
func (s *Stack) afterWrite(ctx context.Context, pool *mcpclient.Pool, name string, status int) stacksdk.Response {
	s.reconcileMCP(ctx)
	for _, v := range pool.Views() {
		if v.Name == name {
			resp := ok(v)
			resp.Status = status
			return resp
		}
	}
	return failure(http.StatusInternalServerError, "the server was saved but could not be read back")
}

// sameHost reports whether two addresses name the same scheme, host and port.
func sameHost(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(ua.Host, ub.Host) && ua.Scheme == ub.Scheme
}

// messageOf is the words of a validation error without the package's prefix.
func messageOf(err error) string {
	return strings.TrimPrefix(err.Error(), mcpclient.ErrInvalid.Error()+": ")
}

var _ stacksdk.ToolSource = (*Stack)(nil)
