package helpertest

import (
	"context"
	"testing"

	"llm-proxy/internal/platform/privhelper/catalog"
)

func TestTheFakePassesTheWireContract(t *testing.T) {
	f := New(t, func(context.Context, catalog.Prepared) ([]byte, error) { return []byte("udp scan output"), nil })
	Contract(t, f.Path, Sample{Action: "scan_udp", Params: `{"target":"192.168.1.5","ports":[53]}`})
}

func TestTheFakeCapsWhatARunReturns(t *testing.T) {
	big := make([]byte, 100<<10)
	f := New(t, func(context.Context, catalog.Prepared) ([]byte, error) { return big, nil })
	r := exchange(t, f.Path, `{"v":1,"id":"x","action":"os_detect","params":{"target":"192.168.1.5"}}`)
	a, _ := catalog.Lookup("os_detect")
	if r.Result == nil || len(r.Result.Data) != a.Caps.MaxOutputBytes || !r.Result.Truncated {
		t.Errorf("got %+v", r.Result)
	}
}
