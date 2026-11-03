//go:build unix

package mcast

import (
	"context"
	"net"
	"net/netip"
	"runtime"
	"testing"
	"time"
)

var loop = netip.MustParseAddr("127.0.0.1")

// responder listens on loopback and answers every datagram with `replies` copies of `body`, from `senders` source ports.
func responder(t *testing.T, replies int, body []byte) netip.AddrPort {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: loop.AsSlice()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 64)
		for {
			_, from, err := conn.ReadFromUDPAddrPort(buf)
			if err != nil {
				return
			}
			for range replies {
				_, _ = conn.WriteToUDPAddrPort(body, from)
			}
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).AddrPort()
}

func ask(ctx context.Context, to netip.AddrPort, lim Limits, accept func(netip.Addr) bool) ([]Datagram, error) {
	return Exchange(ctx, Interface{Name: "lo", Addr: loop}, []Question{{To: to, Payload: []byte("hi")}}, lim, accept)
}

func yes(netip.Addr) bool { return true }

func TestExchange_ReadsTheReplyToTheQuestion(t *testing.T) {
	to := responder(t, 1, []byte("answer"))
	got, err := ask(context.Background(), to, Limits{Window: 300 * time.Millisecond}, yes)
	if err != nil || len(got) != 1 || string(got[0].Data) != "answer" || got[0].From.Addr() != loop {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestExchange_CapsDatagramsFromOneSender(t *testing.T) {
	to := responder(t, 100, []byte("x"))
	got, _ := ask(context.Background(), to, Limits{Window: 300 * time.Millisecond}, yes)
	if len(got) != MaxPerSender {
		t.Fatalf("kept %d datagrams from one sender, want %d", len(got), MaxPerSender)
	}
}

func TestExchange_CapsTheTotal(t *testing.T) {
	to := responder(t, 100, []byte("x"))
	got, _ := ask(context.Background(), to, Limits{Datagrams: 5, PerSender: 32, Window: 300 * time.Millisecond}, yes)
	if len(got) != 5 {
		t.Fatalf("kept %d, want 5", len(got))
	}
}

func TestExchange_DropsASenderTheCallerDoesNotAccept(t *testing.T) {
	to := responder(t, 1, []byte("x"))
	got, _ := ask(context.Background(), to, Limits{Window: 300 * time.Millisecond}, func(netip.Addr) bool { return false })
	if len(got) != 0 {
		t.Fatalf("kept %d datagrams from a refused sender", len(got))
	}
}

func TestExchange_DropsAnOversizeDatagram(t *testing.T) {
	to := responder(t, 1, make([]byte, MaxDatagramSize+10))
	got, _ := ask(context.Background(), to, Limits{Window: 300 * time.Millisecond}, yes)
	if len(got) != 0 {
		t.Fatalf("kept an oversize datagram")
	}
}

func TestExchange_AWindowAboveTheMaximumIsLowered(t *testing.T) {
	if got := (Limits{Window: time.Hour}).clamped().Window; got != MaxWindow {
		t.Fatalf("window %v", got)
	}
}

func TestExchange_CancelEndsTheLookAndLeavesNoGoroutine(t *testing.T) {
	to := responder(t, 0, nil)
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _, _ = ask(ctx, to, Limits{Window: MaxWindow}, yes); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the look did not stop when cancelled")
	}
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+1 {
		t.Fatalf("goroutines %d -> %d", before, after)
	}
}

func TestInterfaces_NeverReturnsLoopbackOrMoreThanTheBound(t *testing.T) {
	got, err := Interfaces(yes)
	if err != nil || len(got) > MaxInterfaces {
		t.Fatalf("%v %v", got, err)
	}
	for _, i := range got {
		if i.Addr.IsLoopback() {
			t.Fatalf("loopback returned: %v", i)
		}
	}
}
