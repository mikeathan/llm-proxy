package scanscope

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"
)

type fakeResolver map[string][]string

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	raw, ok := f[host]
	if !ok {
		return nil, fmt.Errorf("no such host %s", host)
	}
	var out []netip.Addr
	for _, s := range raw {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

func policy(cidrs ...string) Policy {
	var p Policy
	for _, c := range cidrs {
		p.Allowed = append(p.Allowed, netip.MustParsePrefix(c))
	}
	return p
}

func TestParse(t *testing.T) {
	r := fakeResolver{"nas": {"192.168.1.20"}, "mixed": {"192.168.1.30", "8.8.8.8"}, "v6": {"fd00::1"}}
	tests := []struct {
		entry   string
		want    []string
		wantErr error
	}{
		{"192.168.1.5", []string{"192.168.1.5/32"}, nil},
		{"192.168.1.0/24", []string{"192.168.1.0/24"}, nil},
		{"192.168.1.9/24", []string{"192.168.1.0/24"}, nil},
		{"10.0.0.5-10.0.0.9", []string{"10.0.0.5/32", "10.0.0.6/31", "10.0.0.8/31"}, nil},
		{"nas", []string{"192.168.1.20/32"}, nil},
		{"::ffff:192.168.1.7", []string{"192.168.1.7/32"}, nil},
		{"8.8.8.8", nil, ErrNotScannable},
		{"100.64.0.5", nil, ErrNotScannable},
		{"127.0.0.1", nil, ErrNotScannable},
		{"169.254.169.254", nil, ErrNotScannable},
		{"192.168.0.0/15", nil, ErrNotScannable},
		{"mixed", nil, ErrNotScannable},
		{"v6", nil, ErrNotScannable},
		{"unknown-host", nil, ErrBadEntry},
		{"10.0.0.9-10.0.0.5", nil, ErrBadEntry},
		{"", nil, ErrBadEntry},
	}
	for _, tt := range tests {
		t.Run(tt.entry, func(t *testing.T) {
			got, err := Parse(context.Background(), r, tt.entry)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			var gotStr []string
			for _, p := range got {
				gotStr = append(gotStr, p.String())
			}
			if fmt.Sprint(gotStr) != fmt.Sprint(tt.want) {
				t.Errorf("prefixes = %v, want %v", gotStr, tt.want)
			}
		})
	}
}

func TestAuthorize_EmptyPolicyIsOff(t *testing.T) {
	if _, err := (Policy{}).Authorize(context.Background(), nil, []string{"192.168.1.5"}); !errors.Is(err, ErrScanningOff) {
		t.Errorf("err = %v, want ErrScanningOff", err)
	}
}

func TestAuthorize_Scope(t *testing.T) {
	r := fakeResolver{"nas": {"192.168.1.20"}, "other": {"192.168.2.20"}}
	p := policy("192.168.1.0/24")
	tests := []struct {
		name    string
		targets []string
		want    int
		wantErr error
	}{
		{"one address", []string{"192.168.1.5"}, 1, nil},
		{"a name inside", []string{"nas"}, 1, nil},
		{"a name outside", []string{"other"}, 0, ErrOutOfScope},
		{"address outside", []string{"192.168.2.5"}, 0, ErrOutOfScope},
		{"block wider than the range", []string{"192.168.0.0/16"}, 0, ErrOutOfScope},
		{"one outside refuses all", []string{"192.168.1.5", "192.168.2.5"}, 0, ErrOutOfScope},
		{"duplicates collapse", []string{"192.168.1.5", "192.168.1.0/29", "192.168.1.5"}, 6, nil},
		{"block drops network and broadcast", []string{"192.168.1.0/24"}, 254, nil},
		{"a /31 has both addresses", []string{"192.168.1.4/31"}, 2, nil},
		{"no targets means every allowed range", nil, 254, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Authorize(context.Background(), r, tt.targets)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if len(got) != tt.want {
				t.Errorf("got %d addresses, want %d", len(got), tt.want)
			}
		})
	}
}

func TestAuthorize_NoTargetsScansEveryRangeButNothingElse(t *testing.T) {
	got, err := policy("192.168.1.0/30", "10.0.0.0/30").Authorize(context.Background(), nil, nil)
	if err != nil || len(got) != 4 {
		t.Fatalf("got %d addresses, err %v; want the two usable addresses of each range", len(got), err)
	}
	if _, err := (Policy{}).Authorize(context.Background(), nil, nil); !errors.Is(err, ErrScanningOff) {
		t.Errorf("no ranges allowed and no targets = %v, want ErrScanningOff", err)
	}
}

func TestAuthorize_HostCap(t *testing.T) {
	p := policy("10.0.0.0/16")
	p.MaxHosts = 254
	if _, err := p.Authorize(context.Background(), nil, []string{"10.0.1.0/24"}); err != nil {
		t.Errorf("exactly the cap must pass: %v", err)
	}
	p.MaxHosts = 253
	if _, err := p.Authorize(context.Background(), nil, []string{"10.0.1.0/24"}); !errors.Is(err, ErrTooManyHosts) {
		t.Errorf("one over the cap must be refused, got %v", err)
	}
	p.MaxHosts = 10
	if _, err := p.Authorize(context.Background(), nil, []string{"10.0.1.1-10.0.1.8", "10.0.1.20-10.0.1.30"}); !errors.Is(err, ErrTooManyHosts) {
		t.Errorf("the cap applies across targets, got %v", err)
	}
	p.MaxHosts = 0
	if _, err := p.Authorize(context.Background(), nil, []string{"10.0.0.0/16"}); !errors.Is(err, ErrTooManyHosts) {
		t.Errorf("a zero MaxHosts must fall back to the default cap, got %v", err)
	}
}

func TestPolicy_CheckIsTheLastLineOfDefence(t *testing.T) {
	p := policy("192.168.1.0/24")
	if err := p.Check(netip.MustParseAddr("192.168.1.9")); err != nil {
		t.Errorf("an address in range: %v", err)
	}
	if err := p.Check(netip.MustParseAddr("::ffff:192.168.1.9")); err != nil {
		t.Errorf("an IPv4-mapped address in range: %v", err)
	}
	if err := p.Check(netip.MustParseAddr("192.168.2.9")); !errors.Is(err, ErrOutOfScope) {
		t.Errorf("outside the range: %v", err)
	}
	if err := p.Check(netip.MustParseAddr("8.8.8.8")); !errors.Is(err, ErrNotScannable) {
		t.Errorf("a public address: %v", err)
	}
	if err := (Policy{}).Check(netip.MustParseAddr("192.168.1.9")); !errors.Is(err, ErrOutOfScope) {
		t.Errorf("no declared range allows nothing: %v", err)
	}
}
