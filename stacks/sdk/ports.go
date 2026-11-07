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
	Isolation Isolation
	// Privileged is the request channel to the privileged helper (ADR-0034); nil unless the manifest requires PortPrivileged.
	Privileged Privileged
}

// Stack is what a stack package implements. It is called through this interface, in process, with plain data.
type Stack interface {
	Manifest() Manifest
	// Start runs the stack's background work under ctx until ctx ends or Stop is called.
	Start(ctx context.Context, h Host) error
	Stop(ctx context.Context) error
	// ConfigChanged tells the stack its settings document changed; it re-reads it through Host.Config.
	ConfigChanged(ctx context.Context) error
	// CallTool runs the declared tool with JSON arguments and returns JSON. Failures are an *Error.
	CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error)
	// Handle answers one request on a route the manifest declares. A failure the caller should see as a refusal is an *Error; the Response
	// carries every ordinary answer, including a 4xx.
	Handle(ctx context.Context, req Request) (Response, error)
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
	// Save replaces the stack's settings document.
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

// DataDir names the directory the stack owns for its storage. The core never reads it except through the stack's interface.
type DataDir interface {
	Path() string
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
	return []any{Manifest{}, Event{}, RunRequest{}, RunResult{}, Error{}, ToolSpec{}, RouteSpec{}, Request{}, Response{}, PrivilegedRequest{}, PrivilegedResult{}}
}
