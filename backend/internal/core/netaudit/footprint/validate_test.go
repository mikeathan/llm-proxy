package footprint

import "testing"

func TestValidateDomain(t *testing.T) {
	good := map[string]string{"Example.COM": "example.com", " mail.example.co.uk ": "mail.example.co.uk", "xn--bcher-kva.de": "xn--bcher-kva.de"}
	for in, want := range good {
		if got, err := ValidateDomain(in); err != nil || got != want {
			t.Errorf("%q = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "localhost", "10.0.0.1", "::1", "example.com.", "-a.com", "a..com", "exa mple.com", "http://example.com", "example.com/path", "a@b.com", "*.example.com", "x.local", "x.internal", "1.2.3.4.in-addr.arpa"} {
		if _, err := ValidateDomain(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestValidateEmail(t *testing.T) {
	if got, err := ValidateEmail(" Jo.Smith+x@Example.COM "); err != nil || got != "jo.smith+x@example.com" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "no-at", "a@b", "a@@b.com", "a b@c.com", "@c.com", "a@", "a@10.0.0.1", "<x>@c.com"} {
		if _, err := ValidateEmail(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestMask(t *testing.T) {
	if got := Mask("jo.smith@example.com"); got != "j***@example.com" {
		t.Errorf("got %q", got)
	}
}
