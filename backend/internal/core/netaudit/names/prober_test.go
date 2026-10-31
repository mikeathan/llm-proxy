package names

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

// answeringServer is a UDP socket that answers the first question with a reply, given the question that arrived.
func answeringServer(t *testing.T, reply func(question []byte) []byte) Dial {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		if r := reply(buf[:n]); r != nil {
			_, _ = pc.WriteTo(r, from)
		}
	}()
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, pc.LocalAddr().String())
	}
}

func TestProber_ReadsTheNameFromTheDevicesAnswer(t *testing.T) {
	fix := fixture(t, "mdns_ptr_response.hex")
	dial := answeringServer(t, func(q []byte) []byte {
		out := append([]byte{}, fix...)
		copy(out[0:2], q[0:2]) // the device echoes the transaction id
		return out
	})
	got, err := Prober{Dial: dial}.Ask(context.Background(), target)
	if err != nil || got != "example-laptop" {
		t.Errorf("name = %q, %v", got, err)
	}
}

func TestProber_AnAnswerWithAnotherTransactionIdIsNotTaken(t *testing.T) {
	dial := answeringServer(t, func([]byte) []byte { return fixture(t, "mdns_ptr_response.hex") })
	// The fixture carries id 0x1234; a random id differs with a probability of 65535 in 65536. Try until one differs.
	var err error
	for range 5 {
		if _, err = (Prober{Dial: dial}).Ask(context.Background(), target); err != nil {
			return
		}
		dial = answeringServer(t, func([]byte) []byte { return fixture(t, "mdns_ptr_response.hex") })
	}
	t.Error("a reply to a question that was not asked must be refused")
}

func TestProber_ASilentDeviceIsAnErrorAndEndsWithTheContext(t *testing.T) {
	dial := answeringServer(t, func([]byte) []byte { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := (Prober{Dial: dial}).Ask(ctx, target); err == nil {
		t.Fatal("no answer is an error")
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %v: the context's deadline must end the wait", time.Since(start))
	}
}

func TestProber_RefusesWhatThePolicyRefusesAndWhatIsNotIPv4(t *testing.T) {
	refuse := func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("outside the scan scope")
	}
	if _, err := (Prober{Dial: refuse}).Ask(context.Background(), target); err == nil {
		t.Error("a refused dial is an error")
	}
	if _, err := (Prober{Dial: refuse}).Ask(context.Background(), netip.MustParseAddr("fe80::1")); err == nil {
		t.Error("only IPv4 is asked")
	}
}
