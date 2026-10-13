package commands

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func echoCommand(name string) Command {
	return Command{
		Name: name, Usage: "/" + name + " <text>", Summary: "echo",
		Run: func(_ context.Context, req Request) (string, error) { return "ran:" + req.Args, nil },
		Preview: func(_ context.Context, req Request) (string, error) {
			if req.Args == "bad" {
				return "", errors.New("nope")
			}
			return "would:" + req.Args, nil
		},
	}
}

func TestRegistry_Match(t *testing.T) {
	r := New(echoCommand("remind"))
	tests := []struct {
		message  string
		wantArgs string
		wantOK   bool
	}{
		{"/remind tomorrow 9am call", "tomorrow 9am call", true},
		{"  /REMIND  x  ", "x", true},
		{"/remind@MyBot x", "x", true},
		{"/remind", "", true},
		{"/reminder x", "", false},
		{"remind me tomorrow", "", false},
		{"/unknown x", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		_, args, ok := r.Match(tc.message)
		if ok != tc.wantOK || args != tc.wantArgs {
			t.Errorf("Match(%q) = (%q, %v), want (%q, %v)", tc.message, args, ok, tc.wantArgs, tc.wantOK)
		}
	}
}

func TestRegistry_Execute(t *testing.T) {
	r := New(echoCommand("remind"))
	if _, handled := r.Execute(context.Background(), "hello", Env{}); handled {
		t.Error("plain text must not be handled")
	}
	reply, handled := r.Execute(context.Background(), "/remind x", Env{Zone: "UTC"})
	if !handled || reply.Text != "ran:x" || reply.Failed {
		t.Errorf("got (%+v, %v), want (ran:x, true)", reply, handled)
	}
}

func TestRegistry_ExecuteErrorShowsUsage(t *testing.T) {
	r := New(Command{Name: "remind", Usage: "/remind <when> <message>",
		Run: func(context.Context, Request) (string, error) { return "", errors.New("when should I remind you?") }})
	reply, handled := r.Execute(context.Background(), "/remind", Env{})
	if !handled || !reply.Failed || !strings.Contains(reply.Text, "when should I remind you?") || !strings.Contains(reply.Text, "/remind <when> <message>") {
		t.Errorf("reply %+v should carry the problem and the usage", reply)
	}
}

func TestRegistry_Preview(t *testing.T) {
	r := New(echoCommand("remind"))
	got, ok := r.Preview(context.Background(), "/remind x", Env{})
	if !ok || got.Command != "remind" || got.Text != "would:x" || got.Error != "" {
		t.Errorf("got %+v ok=%v", got, ok)
	}
	got, ok = r.Preview(context.Background(), "/remind bad", Env{})
	if !ok || got.Error != "nope" {
		t.Errorf("got %+v, want the error carried", got)
	}
	if _, ok := r.Preview(context.Background(), "/other", Env{}); ok {
		t.Error("unknown command has no preview")
	}
}

func TestRegistry_List(t *testing.T) {
	r := New(echoCommand("remind"), echoCommand("run"))
	list := r.List()
	if len(list) != 2 || list[0].Name != "remind" || list[1].Name != "run" {
		t.Errorf("List() = %+v, want registration order", list)
	}
}
