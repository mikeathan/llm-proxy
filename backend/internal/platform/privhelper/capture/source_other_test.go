//go:build !linux

package capture

import (
	"bytes"
	"testing"

	"llm-proxy/internal/platform/privhelper/wire"
)

func TestWithoutAPacketSocketTheProgramSaysSoAndExitsZero(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Main([]string{"--iface", "en0", "--seconds", "5"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d", code)
	}
	recs, tr, err := wire.DecodeCapture(out.Bytes())
	if err != nil || len(recs) != 0 || tr.Stop != wire.StopUnsupported {
		t.Fatalf("records=%d trailer=%+v err=%v", len(recs), tr, err)
	}
}
