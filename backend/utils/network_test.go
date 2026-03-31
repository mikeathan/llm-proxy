package utils_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"llm-proxy/utils"
)

func TestPortReadyAndWaitForPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("sandbox disallows binding to a local port")
		}
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	if !utils.PortReady(port) {
		t.Fatalf("expected port to be ready")
	}
	if err := utils.WaitForPort(port, 200*time.Millisecond); err != nil {
		t.Fatalf("expected WaitForPort success, got %v", err)
	}
}

func TestWaitForPort_TimesOut(t *testing.T) {
	if err := utils.WaitForPort(0, 50*time.Millisecond); err == nil {
		t.Fatalf("expected timeout error")
	}
}
