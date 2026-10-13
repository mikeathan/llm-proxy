package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/commands"
)

// zoneEcho is a command that reports the zone it ran in.
func zoneEcho() commands.Command {
	return commands.Command{
		Name: "remind", Usage: "/remind <when> <message>", Summary: "s", Examples: []string{"e"},
		Run: func(_ context.Context, r commands.Request) (string, error) {
			return "ran " + r.Zone + " " + r.Args, nil
		},
		Preview: func(_ context.Context, r commands.Request) (string, error) { return "would " + r.Zone, nil },
	}
}

func postCommand(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/api/commands/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestCommandHandlers_RunUsesOwnerZoneBeforeBrowser(t *testing.T) {
	h := NewCommandHandlers(commands.New(zoneEcho()), func() string { return "Europe/London" })
	rec := postCommand(t, h.Run, `{"message":"/remind tomorrow x","timezone":"Atlantic/Canary"}`)
	var got commandRunResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Handled || got.Reply != "ran Europe/London tomorrow x" {
		t.Errorf("got %+v", got)
	}
}

func TestCommandHandlers_RunUsesBrowserZoneWithoutOwnerSetting(t *testing.T) {
	h := NewCommandHandlers(commands.New(zoneEcho()), func() string { return "" })
	var got commandRunResponse
	_ = json.Unmarshal(postCommand(t, h.Run, `{"message":"/remind x","timezone":"Atlantic/Canary"}`).Body.Bytes(), &got)
	if got.Reply != "ran Atlantic/Canary x" {
		t.Errorf("got %+v", got)
	}
}

func TestCommandHandlers_RunLeavesPlainTextToTheAssistant(t *testing.T) {
	h := NewCommandHandlers(commands.New(zoneEcho()), nil)
	var got commandRunResponse
	_ = json.Unmarshal(postCommand(t, h.Run, `{"message":"remind me tomorrow"}`).Body.Bytes(), &got)
	if got.Handled || got.Reply != "" {
		t.Errorf("plain text must not be handled, got %+v", got)
	}
}

func TestCommandHandlers_Preview(t *testing.T) {
	h := NewCommandHandlers(commands.New(zoneEcho()), nil)
	var got commandPreviewResponse
	_ = json.Unmarshal(postCommand(t, h.Preview, `{"message":"/remind x","timezone":"UTC"}`).Body.Bytes(), &got)
	if !got.Matched || got.Command != "remind" || got.Text != "would UTC" {
		t.Errorf("got %+v", got)
	}
	got = commandPreviewResponse{}
	_ = json.Unmarshal(postCommand(t, h.Preview, `{"message":"hello"}`).Body.Bytes(), &got)
	if got.Matched {
		t.Errorf("plain text must not match, got %+v", got)
	}
}

func TestCommandHandlers_List(t *testing.T) {
	h := NewCommandHandlers(commands.New(zoneEcho()), nil)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, "/admin/api/commands", nil))
	var got struct {
		Commands []commands.Info `json:"commands"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Commands) != 1 || got.Commands[0].Name != "remind" {
		t.Errorf("got %+v err=%v", got, err)
	}
}
