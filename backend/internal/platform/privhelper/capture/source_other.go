//go:build !linux

package capture

import "llm-proxy/internal/platform/privhelper/wire"

// Open has no packet socket on this host.
func Open(string) (Source, wire.CaptureStop, error) {
	return nil, wire.StopUnsupported, errNotOpened
}
