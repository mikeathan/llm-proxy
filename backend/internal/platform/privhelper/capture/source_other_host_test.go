//go:build !linux

package capture

import (
	"errors"
	"testing"

	"llm-proxy/internal/platform/privhelper/wire"
)

func TestTheHostLookIsUnsupportedWhereThereIsNoPacketSocket(t *testing.T) {
	if _, stop, err := OpenHost("en0"); !errors.Is(err, errNotOpened) || stop != wire.StopUnsupported {
		t.Fatalf("stop=%d err=%v", stop, err)
	}
}
