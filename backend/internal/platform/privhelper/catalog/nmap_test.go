package catalog

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func prepare(t *testing.T, id, params string) (Prepared, error) {
	t.Helper()
	a, err := Lookup(id)
	if err != nil {
		t.Fatal(err)
	}
	return a.Prepare([]byte(params))
}

func TestScanUDPBuildsTheMeasuredArguments(t *testing.T) {
	p, err := prepare(t, "scan_udp", `{"target":"192.168.50.10","ports":[161,53]}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--privileged", "-n", "-Pn", "--max-retries", "1", "--host-timeout", "60s", "--max-rate", "20", "-sU", "-p", "53,161", "192.168.50.10"}
	if p.Program != "nmap" || !slices.Equal(p.Args, want) {
		t.Errorf("got %s %v", p.Program, p.Args)
	}
}

func TestThePreparedActionNamesItsTarget(t *testing.T) {
	for _, c := range []struct{ id, params string }{{"scan_udp", `{"target":"192.168.1.5","ports":[53]}`}, {"os_detect", `{"target":"::ffff:10.0.0.9"}`}} {
		p, err := prepare(t, c.id, c.params)
		if err != nil || p.Target == "" || p.Args[len(p.Args)-1] != p.Target {
			t.Errorf("%s: target %q, args %v, %v", c.id, p.Target, p.Args, err)
		}
	}
}

func TestOSDetectBuildsTheMeasuredArguments(t *testing.T) {
	p, err := prepare(t, "os_detect", `{"target":"10.1.2.3"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--privileged", "-n", "-Pn", "--max-retries", "1", "--host-timeout", "60s", "--max-rate", "20", "-O", "-p", "22,80,443", "10.1.2.3"}
	if !slices.Equal(p.Args, want) {
		t.Errorf("got %v", p.Args)
	}
}

func TestTwoSpellingsOfOneRequestHaveOneCanonicalForm(t *testing.T) {
	a, err1 := prepare(t, "scan_udp", `{"target":"192.168.1.5","ports":[161,53,53]}`)
	b, err2 := prepare(t, "scan_udp", `{ "ports": [53, 161], "target": "192.168.1.5" }`)
	c, err3 := prepare(t, "scan_udp", `{"target":"::ffff:192.168.1.5","ports":[53,161]}`)
	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatal(err1, err2, err3)
	}
	if string(a.Canonical) != string(b.Canonical) || string(a.Canonical) != string(c.Canonical) {
		t.Errorf("canonical forms differ:\n%s\n%s\n%s", a.Canonical, b.Canonical, c.Canonical)
	}
}

func TestAnActionRefusesWhatIsOutOfScopeOrMalformed(t *testing.T) {
	cases := []struct {
		name, id, params string
		want             error
	}{
		{"public address", "scan_udp", `{"target":"8.8.8.8","ports":[53]}`, ErrScope},
		{"loopback", "scan_udp", `{"target":"127.0.0.1","ports":[53]}`, ErrScope},
		{"link local", "os_detect", `{"target":"169.254.169.254"}`, ErrScope},
		{"shared (CGNAT) space", "os_detect", `{"target":"100.64.0.1"}`, ErrScope},
		{"ipv6", "scan_udp", `{"target":"fe80::1","ports":[53]}`, ErrScope},
		{"multicast", "scan_udp", `{"target":"224.0.0.251","ports":[5353]}`, ErrScope},
		{"a port off the list", "scan_udp", `{"target":"192.168.1.5","ports":[22]}`, ErrScope},
		{"no ports", "scan_udp", `{"target":"192.168.1.5","ports":[]}`, ErrScope},
		{"too many ports", "scan_udp", `{"target":"192.168.1.5","ports":[53,53,53,53,53,53,53,53,53,53]}`, ErrScope},
		{"a name, not an address", "scan_udp", `{"target":"router.local","ports":[53]}`, ErrInvalid},
		{"an option as the target", "os_detect", `{"target":"-oN /tmp/x"}`, ErrInvalid},
		{"a second host in the target", "os_detect", `{"target":"192.168.1.5 192.168.1.6"}`, ErrInvalid},
		{"a cidr", "os_detect", `{"target":"192.168.1.0/24"}`, ErrInvalid},
		{"an unknown field", "os_detect", `{"target":"192.168.1.5","script":"vuln"}`, ErrInvalid},
		{"a field of another action", "os_detect", `{"target":"192.168.1.5","ports":[22]}`, ErrInvalid},
		{"a wrong type", "scan_udp", `{"target":"192.168.1.5","ports":"53"}`, ErrInvalid},
		{"data after the document", "os_detect", `{"target":"192.168.1.5"}{"target":"8.8.8.8"}`, ErrInvalid},
		{"no parameters", "os_detect", ``, ErrInvalid},
		{"null", "os_detect", `null`, ErrInvalid},
	}
	for _, c := range cases {
		if _, err := prepare(t, c.id, c.params); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

// No argument may come from a caller's text except through a checked value: every argument is a fixed word, a number from a fixed list,
// or the canonical address.
func TestNoCallerTextBecomesAnArgumentExceptTheCheckedValues(t *testing.T) {
	p, err := prepare(t, "scan_udp", `{"target":"192.168.1.5","ports":[53]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range p.Args {
		if strings.HasPrefix(arg, "-") && !slices.Contains(nmapFixed, arg) && !slices.Contains([]string{"-sU", "-O", "-p"}, arg) {
			t.Errorf("argument %q is an option nobody fixed", arg)
		}
	}
	if p.Args[len(p.Args)-1] != "192.168.1.5" {
		t.Errorf("the target must be last and canonical: %v", p.Args)
	}
}
