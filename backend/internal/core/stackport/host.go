package stackport

import (
	"context"
	"errors"
	"net"
	"net/netip"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/network"
)

// Dial is the guarded dial function (NetworkTools.DialContext).
type Dial func(ctx context.Context, network, addr string) (net.Conn, error)

// Guard is the SDK's Guard port: the guarded dial, with a refusal as a CodeDenied *Error (a stack cannot tell which rule refused, only that
// one did), and the boundary of an address by the one classifier the guard itself uses.
type Guard struct {
	Dial Dial
}

// DialContext opens a connection through the guard.
func (g Guard) DialContext(ctx context.Context, netw, addr string) (net.Conn, error) {
	conn, err := g.Dial(ctx, netw, addr)
	if errors.Is(err, tools.ErrGuardRefused) {
		return nil, stacksdk.Errorf(stacksdk.CodeDenied, msgDenied)
	}
	return conn, err
}

// Boundary classifies addr as Constitution I.3 does: always blocked, the local network, or the internet.
func (Guard) Boundary(addr netip.Addr) stacksdk.Boundary {
	ip := net.IP(addr.AsSlice())
	switch {
	case network.CheckAlwaysBlocked(ip) != nil:
		return stacksdk.BoundaryBlocked
	case network.IsLAN(ip):
		return stacksdk.BoundaryLAN
	}
	return stacksdk.BoundaryInternet
}

// Log is the SDK's Logger over the service's log.
type Log struct{ Stack string }

// Log writes one line; the stack's name is added so a line says where it came from.
func (l Log) Log(level stacksdk.Level, msg string, fields ...string) {
	args := make([]any, 0, len(fields)+2)
	args = append(args, "stack", l.Stack)
	for _, f := range fields {
		args = append(args, f)
	}
	switch level {
	case stacksdk.LevelDebug:
		logging.Debug(msg, args...)
	case stacksdk.LevelWarn:
		logging.Warn(msg, args...)
	case stacksdk.LevelError:
		logging.Error(msg, args...)
	default:
		logging.Info(msg, args...)
	}
}

// Gateway is the SDK's Gateway port over the core's cached default-gateway reader (the zero address means not known yet).
type Gateway struct {
	Read func() netip.Addr
}

// Default returns the default gateway, or CodeNotFound.
func (g Gateway) Default(context.Context) (string, error) {
	if a := g.Read(); a.IsValid() {
		return a.String(), nil
	}
	return "", stacksdk.Errorf(stacksdk.CodeNotFound, "the default gateway is not known")
}
