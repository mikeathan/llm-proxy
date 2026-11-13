//go:build !linux

package capture

import "llm-proxy/internal/platform/privhelper/wire"

// OpenRadio has no packet socket on this host.
func OpenRadio(string) (Source, wire.CaptureStop, error) {
	return nil, wire.StopUnsupported, errNotOpened
}
