//go:build !linux

package capture

import "llm-proxy/internal/platform/privhelper/wire"

// OpenHost has no packet socket on this host.
func OpenHost(string) (HostSource, wire.CaptureStop, error) {
	return nil, wire.StopUnsupported, errNotOpened
}
