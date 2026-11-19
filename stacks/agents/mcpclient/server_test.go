package mcpclient

import (
	"errors"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

func goodServer() Server {
	return Server{Name: "files", URL: "https://mcp.example.com/mcp", Enabled: true}
}

func TestServer_ValidateAcceptsAGoodEntry(t *testing.T) {
	if err := goodServer().Validate(); err != nil {
		t.Fatalf("a good entry: %v", err)
	}
	withHeader := goodServer()
	withHeader.Auth = Auth{Kind: AuthHeader, Header: "X-Api-Key"}
	withHeader.CACert = "/etc/ssl/own-ca.pem"
	withHeader.DisabledTools = []string{"delete_all"}
	if err := withHeader.Validate(); err != nil {
		t.Fatalf("an entry with a header, a CA file and a switched-off tool: %v", err)
	}
}

func TestServer_ValidateRefusesWhatCannotBeUsed(t *testing.T) {
	cases := map[string]func(*Server){
		"empty name":                    func(s *Server) { s.Name = "" },
		"upper-case name":               func(s *Server) { s.Name = "Files" },
		"name with underscore":          func(s *Server) { s.Name = "my_files" },
		"name that starts with a digit": func(s *Server) { s.Name = "1files" },
		"name too long":                 func(s *Server) { s.Name = strings.Repeat("a", maxNameLen+1) },
		"no address":                    func(s *Server) { s.URL = "" },
		"ftp address":                   func(s *Server) { s.URL = "ftp://x/y" },
		"address without host":          func(s *Server) { s.URL = "https:///mcp" },
		"user in the address":           func(s *Server) { s.URL = "https://user:pw@x/mcp" },
		"fragment":                      func(s *Server) { s.URL = "https://x/mcp#a" },
		"unknown trust":                 func(s *Server) { s.Trust = "sometimes" },
		"unknown sign-in":               func(s *Server) { s.Auth.Kind = "oauth" },
		"header without name":           func(s *Server) { s.Auth = Auth{Kind: AuthHeader} },
		"header the client owns":        func(s *Server) { s.Auth = Auth{Kind: AuthHeader, Header: "Mcp-Session-Id"} },
		"header with a space":           func(s *Server) { s.Auth = Auth{Kind: AuthHeader, Header: "X Key"} },
		"header name on bearer":         func(s *Server) { s.Auth = Auth{Kind: AuthBearer, Header: "X-Key"} },
		"relative CA path":              func(s *Server) { s.CACert = "ca.pem" },
		"empty disabled tool":           func(s *Server) { s.DisabledTools = []string{""} },
	}
	for name, mutate := range cases {
		s := goodServer()
		mutate(&s)
		err := s.Validate()
		if err == nil {
			t.Errorf("%s: must be refused", name)
		} else if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: the refusal must be an ErrInvalid, got %v", name, err)
		}
	}
}

func TestServer_NormalizeDefaultsToAskAndNoSignIn(t *testing.T) {
	n := Server{Trust: "whatever"}.Normalize()
	if n.Trust != TrustAsk || n.Auth.Kind != AuthNone {
		t.Fatalf("an unknown trust reads as ask and an unset sign-in as none, got %+v", n)
	}
	if (Server{Trust: TrustAllow}).Normalize().Trust != TrustAllow {
		t.Error("allow must stay allow")
	}
}

func TestAuth_HeadersNeverGoOutHalfSet(t *testing.T) {
	if h, err := (Auth{Kind: AuthNone}).Headers(""); err != nil || h != nil {
		t.Errorf("none: %v %v", h, err)
	}
	h, err := (Auth{Kind: AuthBearer}).Headers("tok")
	if err != nil || h["Authorization"] != "Bearer tok" {
		t.Errorf("bearer: %v %v", h, err)
	}
	h, err = (Auth{Kind: AuthHeader, Header: "X-Api-Key"}).Headers("v")
	if err != nil || h["X-Api-Key"] != "v" {
		t.Errorf("header: %v %v", h, err)
	}
	for _, a := range []Auth{{Kind: AuthBearer}, {Kind: AuthHeader, Header: "X-Api-Key"}} {
		if _, err := a.Headers(""); err == nil {
			t.Errorf("%+v with no value must be an error, not an unauthenticated request", a)
		}
	}
}

func TestToolName_PrefixedUniqueAndWithinTheLimit(t *testing.T) {
	limit := stacksdk.MaxDynamicToolName
	if got := ToolName("files", "read", limit); got != "mcp__files__read" {
		t.Errorf("plain name: %q", got)
	}
	cases := []struct{ server, tool string }{
		{"files", "read file"}, {"files", "read.file"}, {"files", strings.Repeat("t", 200)},
		{"a-very-long-server-nam", strings.Repeat("t", 70)}, {"files", "ünï"},
	}
	seen := map[string]string{}
	for _, c := range cases {
		got := ToolName(c.server, c.tool, limit)
		if !stacksdk.ValidDynamicToolName(got) {
			t.Errorf("%q → %q is not a valid tool name", c.tool, got)
		}
		if len(got) > limit {
			t.Errorf("%q → %q is longer than %d", c.tool, got, limit)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("%q and %q share the name %q", c.tool, other, got)
		}
		seen[got] = c.tool
		if got != ToolName(c.server, c.tool, limit) {
			t.Errorf("the name must be the same every time")
		}
	}
	if ToolName("files", "read file", limit) == ToolName("files", "read.file", limit) {
		t.Error("two tools that clean to the same text must still get different names")
	}
	if ToolName("a", "x", limit) == ToolName("b", "x", limit) {
		t.Error("the same tool on two servers must get different names")
	}
}
