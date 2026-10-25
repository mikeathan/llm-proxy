package netaudit

import (
	"context"
	"net/netip"
	"testing"
)

func TestCollector_MergesScansOfOneRun(t *testing.T) {
	ctx, c := WithCollector(context.Background())
	if CollectorFrom(ctx) != c || CollectorFrom(context.Background()) != nil {
		t.Fatal("the collector must travel in the context")
	}
	if _, ok := c.Merged(); ok {
		t.Error("nothing scanned yet")
	}
	a, b := netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2")
	c.Add(Result{Engine: "portprobe", Warnings: []string{"limited"}, Hosts: []Host{{Addr: a, Up: true}, {Addr: b, Up: false}}})
	c.Add(Result{Engine: "nmap", Warnings: []string{"limited"}, Hosts: []Host{{Addr: b, Up: true, Ports: []Port{{Number: 22, Proto: "tcp", State: "open"}}}}})

	got, ok := c.Merged()
	if !ok || got.Engine != "portprobe, nmap" || len(got.Warnings) != 1 {
		t.Fatalf("merged = %+v", got)
	}
	if len(got.Hosts) != 2 || !got.Hosts[1].Up || len(got.Findings) != 1 || got.Findings[0].Device != "10.0.0.2" {
		t.Errorf("a later scan of the same address replaces the earlier one: %+v", got)
	}
}
