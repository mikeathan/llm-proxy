package dnscheck

import (
	"context"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func addrs(t *testing.T, in ...string) []netip.Addr {
	t.Helper()
	var out []netip.Addr
	for _, s := range in {
		out = append(out, netip.MustParseAddr(s))
	}
	return out
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Real replies from Cloudflare (2026-11-01): the order is the resolver's, not the router's, so only sets are compared.
func TestParseDoH_ReadsTheAddressesOfARealReply(t *testing.T) {
	cases := map[string][]string{
		"doh_dns.google.json":      {"8.8.4.4", "8.8.8.8"},
		"doh_one.one.one.one.json": {"1.0.0.1", "1.1.1.1"},
		"doh_dns.quad9.net.json":   {"149.112.112.112", "9.9.9.9"},
	}
	for file, want := range cases {
		got, err := ParseDoH(fixture(t, file))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var text []string
		for _, a := range got {
			text = append(text, a.String())
		}
		slices.Sort(text)
		if !slices.Equal(text, want) {
			t.Errorf("%s: got %v, want %v", file, text, want)
		}
	}
}

func TestParseDoH_KeepsOnlyAddressRecordsAndRefusesAnythingElse(t *testing.T) {
	good := `{"Status":0,"Answer":[` +
		`{"type":5,"data":"198.51.100.5"},{"type":1,"data":"2001:db8::2"},{"type":5,"data":"alias.example."},` +
		`{"type":1,"data":"192.0.2.1"},{"type":1,"data":"not an address"},{"type":28,"data":"2001:db8::1"}]}`
	got, err := ParseDoH([]byte(good))
	if err != nil || len(got) != 1 || got[0].String() != "192.0.2.1" {
		t.Errorf("only the type-1 record that is an address is kept: %v %v", got, err)
	}
	for name, raw := range map[string]string{
		"not json":       `<html>`,
		"name not found": `{"Status":3}`,
		"server failure": `{"Status":2,"Answer":[{"type":1,"data":"192.0.2.1"}]}`,
		"no status":      `{"Answer":[{"type":1,"data":"192.0.2.1"}]}`,
		"no address":     `{"Status":0,"Answer":[{"type":5,"data":"alias.example."}]}`,
		"empty":          `{"Status":0}`,
	} {
		if got, err := ParseDoH([]byte(raw)); err == nil {
			t.Errorf("%s: want an error, got %v", name, got)
		}
	}
}

func TestParseDoH_BoundsTheAddressesRead(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"Status":0,"Answer":[`)
	for i := 1; i <= 40; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		b.WriteString(`{"type":1,"data":"192.0.2.` + strconv.Itoa(i) + `"}`)
	}
	b.WriteString(`]}`)
	got, err := ParseDoH([]byte(b.String()))
	if err != nil || len(got) != MaxAddresses {
		t.Errorf("got %d addresses, err %v; want %d", len(got), err, MaxAddresses)
	}
}

func TestCompare(t *testing.T) {
	a := addrs(t, "8.8.8.8", "8.8.4.4")
	cases := []struct {
		name          string
		router, out   []netip.Addr
		same, overlap bool
	}{
		{"same set, other order", a, addrs(t, "8.8.4.4", "8.8.8.8"), true, true},
		{"repeats do not matter", addrs(t, "8.8.8.8", "8.8.8.8", "8.8.4.4"), a, true, true},
		{"one shared, one different", addrs(t, "8.8.8.8", "203.0.113.9"), a, false, true},
		{"router holds fewer", addrs(t, "8.8.8.8"), a, false, true},
		{"router holds more", a, addrs(t, "8.8.8.8"), false, true},
		{"nothing shared", addrs(t, "203.0.113.9"), a, false, false},
	}
	for _, c := range cases {
		got := Compare(c.router, c.out)
		if got.Same != c.same || got.Overlap != c.overlap {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}

func TestValidateNames(t *testing.T) {
	got, err := ValidateNames([]string{" DNS.Google ", "one.one.one.one.", "dns.google"})
	if err != nil || !slices.Equal(got, []string{"dns.google", "one.one.one.one"}) {
		t.Errorf("tidy, lower-case, no trailing dot, each once: %v %v", got, err)
	}
	bad := map[string][]string{
		"none":          {},
		"blank only":    {"  "},
		"a space":       {"bad name.example"},
		"a slash":       {"example.com/x"},
		"an address":    {"192.0.2.1"},
		"a leading dot": {".example.com"},
		"empty label":   {"a..example"},
		"a long label":  {strings.Repeat("a", 64) + ".example"},
		"too long":      {strings.Repeat("abcdefghi.", 26) + "com"},
		"too many":      {"a.example", "b.example", "c.example", "d.example", "e.example", "f.example"},
	}
	for name, in := range bad {
		if got, err := ValidateNames(in); err == nil {
			t.Errorf("%s: want an error, got %v", name, got)
		}
	}
}

// answerDNS serves one UDP question with the given IPv4 answers, built to the wire format by hand: the real router reply was
// observed with dig, which reads the same bytes.
func answerDNS(t *testing.T, pc net.PacketConn, ips []string) {
	t.Helper()
	buf := make([]byte, 512)
	n, from, err := pc.ReadFrom(buf)
	if err != nil {
		return
	}
	q := buf[:n]
	end := 12
	for q[end] != 0 {
		end += int(q[end]) + 1
	}
	end += 5 // the root label, type and class
	reply := append([]byte{q[0], q[1], 0x81, 0x80, 0, 1, 0, byte(len(ips)), 0, 0, 0, 0}, q[12:end]...)
	for _, ip := range ips {
		reply = append(reply, 0xC0, 0x0C, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
		reply = append(reply, net.ParseIP(ip).To4()...)
	}
	pc.WriteTo(reply, from)
}

func TestRouter_AsksOnlyTheRoutersAddressOnPort53(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback socket: %v", err)
	}
	defer pc.Close()
	go answerDNS(t, pc, []string{"8.8.8.8", "8.8.4.4"})

	var dialed []string
	r := Router{
		Server: func() netip.Addr { return netip.MustParseAddr("192.0.2.1") },
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialed = append(dialed, network+" "+address)
			return (&net.Dialer{}).DialContext(ctx, network, pc.LocalAddr().String())
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := r.LookupA(ctx, "dns.google")
	if err != nil {
		t.Fatal(err)
	}
	if !Compare(got, addrs(t, "8.8.4.4", "8.8.8.8")).Same {
		t.Errorf("got %v", got)
	}
	if len(dialed) == 0 || dialed[0] != "udp 192.0.2.1:53" {
		t.Errorf("only the router's port 53 may be dialed, got %v", dialed)
	}
}

func TestRouter_NoAddressForTheRouterIsAnErrorAndDialsNothing(t *testing.T) {
	r := Router{
		Server: func() netip.Addr { return netip.Addr{} },
		Dial: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("nothing may be dialed")
			return nil, nil
		},
	}
	if _, err := r.LookupA(context.Background(), "dns.google"); err != ErrNoRouter {
		t.Errorf("err = %v, want ErrNoRouter", err)
	}
}
