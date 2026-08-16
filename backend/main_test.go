package main

import (
	"net"
	"testing"
	"time"

	"llm-proxy/internal/boot"
)

func TestCheckPortAvailable_FreePort(t *testing.T) {
	// A wildcard bind lets the kernel pick an unused port and the check must
	// succeed. This exercises the success path deterministically (binding the
	// same previously-used port can hit TIME_WAIT on macOS).
	if err := boot.CheckPortAvailable("127.0.0.1:0"); err != nil {
		t.Fatalf("expected wildcard port to be available, got: %v", err)
	}
}

func TestCheckPortAvailable_InUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to grab ephemeral port: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	done := make(chan error, 1)
	go func() { done <- boot.CheckPortAvailable(addr) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected checkPortAvailable to report %s in use", addr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("checkPortAvailable hung on an in-use port")
	}
}
