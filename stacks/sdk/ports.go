package stacksdk

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"time"
)

// Host is what the core hands a stack when it starts it: one adapter per port the manifest requires. A port the manifest did not
// require is nil.
type Host struct {
	Config    Config
	Events    Events
	Log       Logger
	DataDir   DataDir
	Guard     Guard
	Fetch     Fetch
	Gateway   Gateway
	Isolation Isolation
	// Privileged is the request channel to the privileged helper (ADR-0034); nil unless the manifest requires PortPrivileged.
	Privileged Privileged
	// ScopedGuard is the guarded dial that first applies the operator's declared scan scope; nil unless the manifest requires PortScopedGuard.
	ScopedGuard Guard
	// SecretScan is the core's look through the files agents wrote; nil unless the manifest requires PortSecretScan.
	SecretScan SecretScan
	// Vault is the core's keyed tags and sealed values; nil unless the manifest requires PortVault.
	Vault Vault
}

// Stack is what a stack package implements. It is called through this interface, in process, with plain data.
type Stack interface {
	Manifest() Manifest
	// Start runs the stack's background work under ctx until ctx ends or Stop is called.
	Start(ctx context.Context, h Host) error
	Stop(ctx context.Context) error
	// ConfigChanged tells the stack its settings document changed; it re-reads it through Host.Config.
	ConfigChanged(ctx context.Context) error
	// Status reports, for each module that runs background work, whether it is switched on, when it last ran and what stopped it. The core
	// reads it for the modules screen and the stalled-monitor view; a stack with no such module returns nothing.
	Status(ctx context.Context) ([]ModuleStatus, error)
	// CallTool runs the declared tool with JSON arguments and returns JSON. Failures are an *Error.
	CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error)
	// Findings returns what the stack offers for the nightly report, pulled by the core when it builds one (ADR-0036). It must be quick and
	// must not send anything: it reads what the stack already kept. A stack with nothing to offer returns nothing.
	Findings(ctx context.Context) ([]StackFinding, error)
	// Handle answers one request on a route the manifest declares. A failure the caller should see as a refusal is an *Error; the Response
	// carries every ordinary answer, including a 4xx.
	Handle(ctx context.Context, req Request) (Response, error)
}

// The kinds of observation a stack may list in Manifest.Observes. A refusal and an approval come from the agents' tool calls; a fetch is a
// tool call that fetched a web page.
const (
	ObservedRefused  = "tool_refused"
	ObservedApproval = "approval_asked"
	ObservedFetch    = "fetch"
)

// Observation is one thing the service saw an agent do. The tool, automation and host are names the core already vetted as plain tokens; the
// workspace is the id the service made. It has no arguments, no URL path or query, no error text and no model words: the core keeps those out
// before it pushes. Its fields are about agents; a stack that listens to something else declares its own kinds and the fields it needs.
type Observation struct {
	Kind string `json:"kind"`
	// Workspace is the workspace's id, as the service made it (not vetted: the record has always kept it as it was).
	Workspace string `json:"workspace"`
	// Automation names the automation of the run; empty for a chat.
	Automation string `json:"automation,omitempty"`
	// Unattended is true when an automation ran it, with nobody there to be told.
	Unattended bool `json:"unattended,omitempty"`
	// Tool is the tool's name (a refusal or an approval); Host the host of a fetch.
	Tool string `json:"tool,omitempty"`
	Host string `json:"host,omitempty"`
}

// Observer is implemented by a stack that takes in what the service sees (Manifest.Observes). The core pushes each observation as it happens, on
// the goroutine that saw it, so Observe must never block: a stack queues the observation and returns, and counts one it dropped.
type Observer interface {
	Observe(o Observation)
}

// SecretHit is one place an agent left what looks like a credential: the workspace, the workspace-relative path, and the kind of secret in
// fixed words. The secret itself never crosses the port.
type SecretHit struct {
	Workspace string `json:"workspace"`
	Where     string `json:"where"`
	Kind      string `json:"kind"`
}

// SecretScan asks the core to look through the files agents wrote in every workspace for credentials. The core keeps the folder walk, the size
// and file-count bounds and the secret shapes; what comes back is places and kinds only. A failure is an *Error.
type SecretScan interface {
	Scan(ctx context.Context) ([]SecretHit, error)
}

// MaxVaultPurpose and MaxVaultBytes bound what crosses the vault: a purpose is a short plain token and a value is a small identifier or a
// short label, never a document.
const (
	MaxVaultPurpose = 32
	MaxVaultBytes   = 4096
)

// Vault gives a stack tags and sealed values for data it must keep about people or things it does not own. The key never crosses the port: the
// core derives a subkey for each purpose from its master key, so one purpose's tags cannot be matched to another's. Failures are an *Error:
// CodeInvalid for a purpose that is not a clean token or a value that is empty or too large, CodeDenied for a sealed value that does not open
// (altered, or from before the key was rotated), CodeUnavailable when the core has no key yet.
type Vault interface {
	// Tag returns the same bytes for the same purpose and value for as long as the purpose's key lives, and a value that cannot be turned back
	// into the input. Rotating the purpose's key makes every old tag stop matching.
	Tag(ctx context.Context, purpose string, value []byte) ([]byte, error)
	// Seal returns the value encrypted so that only Open can read it; the result differs on every call.
	Seal(ctx context.Context, purpose string, value []byte) ([]byte, error)
	// Open returns what Seal sealed for the same purpose.
	Open(ctx context.Context, purpose string, sealed []byte) ([]byte, error)
}

// StorageUse is how much of its data folder one file of a stack uses and the most it may use. It is counts and a time only, no names of
// what is inside, so the data management page can show it for every stack the same way.
type StorageUse struct {
	// File is a file the manifest lists in Storage.
	File string `json:"file"`
	// Bytes is its size now; CapBytes the most the stack lets it reach (zero when the stack sets no cap).
	Bytes    int64 `json:"bytes"`
	CapBytes int64 `json:"cap_bytes,omitempty"`
	// Rows is how many records it holds (zero when it is not a table of records).
	Rows int `json:"rows,omitempty"`
	// Oldest is the time of the oldest record, zero when there is none; KeepFor how long the stack keeps a record before it removes it.
	Oldest  time.Time     `json:"oldest,omitempty"`
	KeepFor time.Duration `json:"keep_for,omitempty"`
}

// StorageReporter is implemented by a stack that keeps data (Manifest.Storage). The core asks it for the data management page. It must be quick
// and must not write.
type StorageReporter interface {
	StorageUse(ctx context.Context) ([]StorageUse, error)
}

// EventAlert is the Event type of an alert. Its Data is an Alert; the core words it from its own table for ID and sends it to the operator.
const EventAlert = "alert"

// Alert is what a stack publishes when the operator should be told: an id from the stack's fixed list, and the vetted tokens it is about. The
// core drops an id it does not know and words the rest, so no sentence is built from text the stack kept.
type Alert struct {
	ID         string `json:"id"`
	Workspace  string `json:"workspace,omitempty"`
	Automation string `json:"automation,omitempty"`
	Tool       string `json:"tool,omitempty"`
	Host       string `json:"host,omitempty"`
	// Count is how many places or entries the alert is about (the secrets alert).
	Count int `json:"count,omitempty"`
}

// ModuleStatus is what one module's background work has done. Problem is a fixed phrase (never text from the network), or empty.
type ModuleStatus struct {
	ID string `json:"id"`
	// Enabled is true while the module is on and has something to do.
	Enabled bool `json:"enabled"`
	// Every is how often a cycle should run (zero when it does not run on an interval).
	Every   time.Duration `json:"every,omitempty"`
	LastRun time.Time     `json:"last_run,omitempty"`
	Problem string        `json:"problem,omitempty"`
	// Recent is what the module did over the last 24 hours, in counts only (ADR-0049); the zero value when it did nothing in that time or
	// keeps no tally. It has no free text: the core words it from its own table, so nothing a device or a resolver sent can reach a report
	// through it.
	Recent Tally `json:"recent,omitzero"`
}

// Tally is what a module's background work did over a period: how many times it looked, how many looks found something worth a second
// look, and how many could not be judged (the other side gave no usable answer). Counts only.
type Tally struct {
	Done    int `json:"done"`
	Flagged int `json:"flagged"`
	Unclear int `json:"unclear"`
}

// StackFinding is one finding a stack offers (ADR-0036). It is typed fields only, with no free-text field: the core words the finding
// from its own table for ID, so nothing a device or a resolver sent can reach the report. The core drops a finding whose ID the module
// did not declare in ModuleSpec.Finds, whose Subject is not a device it scanned, or whose Names are not host names.
type StackFinding struct {
	ID string `json:"id"`
	// Subject is the device the finding is about.
	Subject netip.Addr `json:"subject"`
	// Count is how many times, or in a row, the stack saw it.
	Count int `json:"count,omitempty"`
	// Names are host names the operator chose to watch.
	Names []string `json:"names,omitempty"`
	// Since is when it was first seen in this run of sightings.
	Since time.Time `json:"since,omitempty"`
}

// MaxLookAddrs bounds how many addresses one Look request carries; the core sends no more and a stack refuses more.
const MaxLookAddrs = 1024

// Pair is a device address and a port, as typed numbers: the answer to "which of these did you ask, and which answered".
type Pair struct {
	Addr netip.Addr `json:"addr"`
	Port int        `json:"port"`
}

// LookRequest asks a sensor module to look at the devices the nightly scan found. The addresses are those the core already judged
// worth asking (live, scanned, IPv4); the stack asks no other.
type LookRequest struct {
	// Module is the id of a module the manifest lists in Looks.
	Module string       `json:"module"`
	Addrs  []netip.Addr `json:"addrs"`
}

// LookResult is what a sensor learned, as typed pairs and nothing a device said. Asked is every pair the stack sent a question to, and Answered
// those that replied in the expected form; a pair asked and not answered is unknown, never fixed.
type LookResult struct {
	Asked    []Pair `json:"asked"`
	Answered []Pair `json:"answered"`
}

// Looker is implemented by a stack with a sensor module (Manifest.Looks). Unlike Findings it sends: it asks devices through the host's
// ScopedGuard during the nightly report. It is separate from Stack so a stack with no sensor implements nothing extra. A failure is an *Error.
type Looker interface {
	Look(ctx context.Context, req LookRequest) (LookResult, error)
}

// The bounds of a web sensor's request, held by the host and by the stack alike.
const (
	// MaxWebTargets is how many web services one request names.
	MaxWebTargets = 64
	// MaxLookSeconds is the longest time limit a request may ask for.
	MaxLookSeconds = 900
)

// WebTarget is one web service on one device: an address the core has already checked against the declared scan scope, a port, and whether
// it speaks TLS.
type WebTarget struct {
	Addr netip.Addr `json:"addr"`
	Port int        `json:"port"`
	TLS  bool       `json:"tls,omitempty"`
}

// WebRequest asks a web sensor module to look at web services. LimitSeconds ends the run: what was found by then comes back with Partial set.
type WebRequest struct {
	// Module is the id of a module the manifest lists in Looks.
	Module       string      `json:"module"`
	Targets      []WebTarget `json:"targets"`
	LimitSeconds int         `json:"limit_seconds"`
}

// WebHit is one fixed check that matched on one service. ID is a check id from the stack's fixed list; the core owns the words and the
// severity for each id and drops an id it does not know, so nothing a device said reaches a finding.
type WebHit struct {
	Addr netip.Addr `json:"addr"`
	Port int        `json:"port"`
	ID   string     `json:"id"`
}

// WebResult is what a run of the plain web checks found and whether its time limit ended it before every service was asked.
type WebResult struct {
	Hits    []WebHit `json:"hits"`
	Partial bool     `json:"partial,omitempty"`
}

// CertFact is what one secure service showed in a handshake: the dates of the certificate it presented, whether it signed itself, and the
// protocol versions older than 1.2 that it accepted, as the numbers the TLS standard gives them (0x0301 is 1.0). The certificate's own text is
// never read out.
type CertFact struct {
	Addr        netip.Addr `json:"addr"`
	Port        int        `json:"port"`
	NotBefore   time.Time  `json:"not_before"`
	NotAfter    time.Time  `json:"not_after"`
	SelfSigned  bool       `json:"self_signed,omitempty"`
	OldVersions []uint16   `json:"old_versions,omitempty"`
}

// CertResult is what a run of the handshakes found and whether its time limit ended it before every service was asked.
type CertResult struct {
	Facts   []CertFact `json:"facts"`
	Partial bool       `json:"partial,omitempty"`
}

// WebLooker is implemented by a stack with a web sensor module (Manifest.Looks). Like Looker it sends, through the host's ScopedGuard only.
// LookWeb makes the fixed plain GET requests; LookCertificates makes the handshakes with the secure services among the targets. A failure
// is an *Error.
type WebLooker interface {
	LookWeb(ctx context.Context, req WebRequest) (WebResult, error)
	LookCertificates(ctx context.Context, req WebRequest) (CertResult, error)
}

// MaxRouterMappings bounds how many forwarded ports one router answer carries; the host drops what comes beyond it.
const MaxRouterMappings = 64

// RouterRequest asks a router sensor module to look at the router at Gateway: whether it answers UPnP, which ports it forwards, and, when
// WantAddress is set, the address of its internet side. The core has already checked Gateway against the declared scan scope.
type RouterRequest struct {
	// Module is the id of a module the manifest lists in Looks.
	Module  string     `json:"module"`
	Gateway netip.Addr `json:"gateway"`
	// WantAddress asks for the router's own statement of its internet address. The core sets it only when the operator switched the outside
	// lookup on, so a router is not asked for it otherwise.
	WantAddress bool `json:"want_address,omitempty"`
}

// RouterMapping is one port a router forwards from its internet side to a device, as typed numbers. Protocol is "tcp" or "udp".
type RouterMapping struct {
	ExternalPort int        `json:"external_port"`
	InternalPort int        `json:"internal_port"`
	Protocol     string     `json:"protocol"`
	Client       netip.Addr `json:"client"`
	Enabled      bool       `json:"enabled,omitempty"`
}

// RouterResult is what a router sensor learned, as flags, numbers and ids: nothing the router said. Problem is the id of why the router could
// not be read at all, and MappingsProblem why its list of forwarded ports could not be (the rest then still stands); the core owns the words
// for each id and treats an id it does not know as an unspecified failure. An address is never carried when the router did not state one.
type RouterResult struct {
	Answered        bool            `json:"answered,omitempty"`
	Gateway         bool            `json:"gateway,omitempty"`
	HasService      bool            `json:"has_service,omitempty"`
	Problem         string          `json:"problem,omitempty"`
	Mappings        []RouterMapping `json:"mappings,omitempty"`
	MappingsProblem string          `json:"mappings_problem,omitempty"`
	// ExternalAddr is the address of the router's internet side, valid only when WantAddress was set and the router stated one.
	ExternalAddr netip.Addr `json:"external_addr,omitempty"`
}

// RouterLooker is implemented by a stack with a router sensor module (Manifest.Looks). Like Looker it sends, through the host's ScopedGuard only.
// A failure is an *Error.
type RouterLooker interface {
	LookRouter(ctx context.Context, req RouterRequest) (RouterResult, error)
}

// Request is one HTTP request to a stack route, as plain data. The core has already signed the caller in and checked the route's access.
type Request struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// Query is the raw query string, without the "?".
	Query string `json:"query,omitempty"`
	// Params are the values of the route's {name} wildcards.
	Params      map[string]string `json:"params,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	Body        []byte            `json:"body,omitempty"`
	// Grant is set only on a route whose RouteSpec names an action, after the core has checked step-up and decoded the body as the
	// action's parameters. It is the one-use permission the stack passes back through Host.Privileged.
	Grant string `json:"grant,omitempty"`
}

// Response is a stack's answer to a Request.
type Response struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"`
}

// Config gives a stack its settings document as JSON. The stack re-reads it when the core calls Stack.ConfigChanged and never reaches
// into the core's global settings.
type Config interface {
	// Load returns the stack's settings document (the SettingsSchema key).
	Load(ctx context.Context) (json.RawMessage, error)
	// Save merges the document over the stored one: a field it names replaces the stored field, a field it leaves out is kept, and a
	// document that does not fit the settings saves nothing (an *Error with CodeInvalid).
	Save(ctx context.Context, doc json.RawMessage) error
}

// Event is a typed message a stack publishes. The core decides where it goes.
type Event struct {
	Type    string          `json:"type"`
	Subject string          `json:"subject,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	At      time.Time       `json:"at"`
}

// Events publishes a stack's events. The stack never uses the core's event bus directly.
type Events interface {
	Publish(ctx context.Context, e Event) error
}

// Level is a log level.
type Level string

// Log levels.
const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Logger writes one structured line. Fields are alternating key, value strings.
type Logger interface {
	Log(level Level, msg string, fields ...string)
}

// DataDir is a stack's storage: the files its manifest lists in Storage, and no others. A name outside that list is refused with
// CodeDenied, so what a stack may write is data the core checks, not a convention (D10). The core writes atomically, with the
// permissions it gives its own user content.
type DataDir interface {
	// Read returns the file's bytes; a file that does not exist yet fails with CodeNotFound.
	Read(name string) ([]byte, error)
	// Write replaces the file atomically.
	Write(name string, data []byte) error
	// Quarantine sets a file that could not be understood aside (as name + ".corrupt") so the stack can start again from an empty one.
	Quarantine(name string) error
}

// Boundary classifies a destination the way Constitution I.3 does.
type Boundary string

// The boundary of a destination.
const (
	BoundaryLAN      Boundary = "lan"
	BoundaryInternet Boundary = "internet"
	BoundaryBlocked  Boundary = "blocked" // always refused (loopback, link-local metadata and the like)
)

// Guard is the only way a stack opens an outbound connection (Constitution I.1 to I.3). DialContext applies DNS-rebinding protection
// and the LAN-versus-internet boundary at the socket; a stack that dials any other way fails the source scan in internal/archtest.
// It is a capability, not a data call: it returns a live connection.
type Guard interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	Boundary(addr netip.Addr) Boundary
}

// FetchRequest asks the core for one guarded HTTPS GET. It is plain data and has no field for a method, a header, a client or a
// redirect rule: the core sends a GET with the Accept value, follows only the redirects the guard allows, and checks the internet
// boundary, the status and the size before the body comes back.
type FetchRequest struct {
	URL    string `json:"url"`
	Accept string `json:"accept,omitempty"`
	// MaxBytes caps the body read; the core applies its own ceiling when this is larger or zero.
	MaxBytes int `json:"max_bytes,omitempty"`
}

// FetchResponse is what a successful (HTTP 200) fetch returned; any other status is an *Error.
type FetchResponse struct {
	Body      []byte `json:"body,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Fetch makes one guarded GET (Constitution I.1 to I.3) so a stack does not re-implement the boundary, redirect, size and status checks.
// Failures are an *Error: CodeUnavailable when network or internet access is switched off (the stack can say so and stop), CodeDenied
// when a guard refused the address, a blocked domain or a redirect, CodeTimeout when it ran out of time, CodeInvalid for a URL the core
// will not send, and CodeInternal for any other failure, including an answer that was not HTTP 200.
type Fetch interface {
	Get(ctx context.Context, req FetchRequest) (FetchResponse, error)
}

// Gateway tells a stack the address of this network's default gateway, which only the core reads (the neighbour table). It is plain data.
// Default fails with CodeNotFound when the gateway is not known yet.
type Gateway interface {
	Default(ctx context.Context) (string, error)
}

// RunRequest asks the core to run a known program under the isolation library. It is plain data and has no field that relaxes the
// sandbox: no environment, no Unix-socket switch, no way to turn the jail off (ADR-0033). The core builds the clean environment.
type RunRequest struct {
	// Program names a program the core knows how to run (the core holds the table; a stack cannot name an arbitrary binary).
	Program string `json:"program"`
	// Args are the code-built arguments.
	Args []string `json:"args"`
	// Root is the directory the program runs in and may write to.
	Root string `json:"root"`
	// ReadPaths are extra directories it may read.
	ReadPaths []string `json:"read_paths,omitempty"`
	// NoNetwork denies the program new TCP connections where the kernel can.
	NoNetwork bool `json:"no_network,omitempty"`
	// TimeoutSeconds kills the program when exceeded.
	TimeoutSeconds int `json:"timeout_seconds"`
	// MaxOutputBytes caps the stdout kept.
	MaxOutputBytes int `json:"max_output_bytes"`
}

// RunResult is what a run produced.
type RunResult struct {
	Stdout    []byte `json:"stdout"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Isolation runs known programs under the sandbox. Failures are an *Error: CodeUnavailable when the host cannot confine the program,
// CodeTimeout when it ran out of time (the partial stdout is still returned), CodeNotFound when the program is not installed.
type Isolation interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

// PrivilegedRequest asks the core to run one catalogue action in the privileged helper. The core checks that the grant is one it minted,
// unused and unexpired, and that it was minted for exactly this action and these parameters, before it calls the helper; a stack cannot
// mint a grant or change the parameters it was given.
type PrivilegedRequest struct {
	Grant  string          `json:"grant"`
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

// PrivilegedResult is what the helper returned: raw bounded bytes the stack must parse itself (the helper never does).
type PrivilegedResult struct {
	Kind      string `json:"kind"`
	Data      []byte `json:"data"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Privileged runs a granted action in the privileged helper. Failures are an *Error: CodeDenied when the grant is not valid for this
// request or the helper refused it, CodeUnavailable when the helper is not installed or the host cannot confine the service's children,
// CodeInvalid for parameters the catalogue refuses, CodeTimeout when it ran out of time.
type Privileged interface {
	Run(ctx context.Context, req PrivilegedRequest) (PrivilegedResult, error)
}

// PortTypes are the request, result and event types that cross a port. A test checks that each is plain data and survives a JSON
// round trip (Constitution IV.6).
func PortTypes() []any {
	return []any{Manifest{}, Event{}, FetchRequest{}, FetchResponse{}, ModuleStatus{}, Tally{}, RunRequest{}, RunResult{}, Error{}, ToolSpec{}, RouteSpec{}, Request{}, Response{}, PrivilegedRequest{}, PrivilegedResult{}, StackFinding{}, LookRequest{}, LookResult{}, Pair{}, WebTarget{}, WebRequest{}, WebHit{}, WebResult{}, CertFact{}, CertResult{}, RouterRequest{}, RouterMapping{}, RouterResult{}, Observation{}, SecretHit{}, Alert{}, StorageUse{}}
}
