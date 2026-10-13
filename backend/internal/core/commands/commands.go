// Package commands holds the slash commands a person can type instead of asking
// the assistant ("/remind tomorrow 9am call the bank"). A command is plain code:
// it runs without a model, so it is instant and costs nothing. The registry is
// independent of the channel; the web chat and every connector call the same
// Match/Execute/Preview.
package commands

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Env is what a channel knows about the person typing.
type Env struct {
	Zone string    // IANA zone the person's times are in (reminders.ChatZone)
	Now  time.Time // zero means time.Now()
}

// Request is a command's arguments in that environment.
type Request struct {
	Args string
	Zone string
	Now  time.Time
}

// Command is one slash command. Preview says what Run would do without doing it;
// both return an error whose text is meant to be shown to the person.
type Command struct {
	Name     string
	Usage    string
	Summary  string
	Examples []string
	Preview  func(ctx context.Context, req Request) (string, error)
	Run      func(ctx context.Context, req Request) (string, error)
}

// Info describes a command to a UI that offers it.
type Info struct {
	Name     string   `json:"name"`
	Usage    string   `json:"usage"`
	Summary  string   `json:"summary"`
	Examples []string `json:"examples"`
}

// Preview is what a command would do with the text typed so far.
type Preview struct {
	Command string `json:"command"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Registry looks commands up by name, in registration order.
type Registry struct {
	order  []string
	byName map[string]Command
}

// New builds a registry of the commands.
func New(cmds ...Command) *Registry {
	r := &Registry{byName: make(map[string]Command, len(cmds))}
	for _, c := range cmds {
		r.order = append(r.order, c.Name)
		r.byName[c.Name] = c
	}
	return r
}

// Match reads a message as a command: "/name args", with Telegram's "/name@bot"
// form accepted. ok is false for plain text and for an unknown name, which are not
// commands and go to the assistant.
func (r *Registry) Match(message string) (Command, string, bool) {
	message = strings.TrimSpace(message)
	if r == nil {
		return Command{}, "", false
	}
	if !strings.HasPrefix(message, "/") {
		return Command{}, "", false
	}
	head, args := message[1:], ""
	if i := strings.IndexAny(head, " \t\r\n"); i >= 0 {
		head, args = head[:i], head[i:]
	}
	if i := strings.IndexByte(head, '@'); i >= 0 {
		head = head[:i]
	}
	cmd, ok := r.byName[strings.ToLower(head)]
	if !ok {
		return Command{}, "", false
	}
	return cmd, strings.TrimSpace(args), true
}

func (e Env) request(args string) Request {
	now := e.Now
	if now.IsZero() {
		now = time.Now()
	}
	return Request{Args: args, Zone: e.Zone, Now: now}
}

// Reply is what a command answers: Text to show, and Failed when the command was
// refused (Text then carries the reason and the usage).
type Reply struct {
	Text   string
	Failed bool
}

// Execute runs the message if it is a command and returns the reply to show;
// handled is false when it is not one.
func (r *Registry) Execute(ctx context.Context, message string, env Env) (reply Reply, handled bool) {
	cmd, args, ok := r.Match(message)
	if !ok {
		return Reply{}, false
	}
	text, err := cmd.Run(ctx, env.request(args))
	if err != nil {
		return Reply{Text: fmt.Sprintf("%s\nUsage: %s", err, cmd.Usage), Failed: true}, true
	}
	return Reply{Text: text}, true
}

// Preview says what the message would do, for a UI that shows it while typing.
func (r *Registry) Preview(ctx context.Context, message string, env Env) (Preview, bool) {
	cmd, args, ok := r.Match(message)
	if !ok {
		return Preview{}, false
	}
	p := Preview{Command: cmd.Name}
	if cmd.Preview == nil {
		return p, true
	}
	text, err := cmd.Preview(ctx, env.request(args))
	if err != nil {
		p.Error = err.Error()
		return p, true
	}
	p.Text = text
	return p, true
}

// List describes the commands in registration order.
func (r *Registry) List() []Info {
	if r == nil {
		return []Info{}
	}
	infos := make([]Info, 0, len(r.order))
	for _, name := range r.order {
		c := r.byName[name]
		infos = append(infos, Info{Name: c.Name, Usage: c.Usage, Summary: c.Summary, Examples: c.Examples})
	}
	return infos
}
