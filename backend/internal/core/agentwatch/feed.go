// Package agentwatch is the core's side of the Agents stack's watch (stacks/agents). The stack keeps the agent security record; this package
// owns what only the core can do for it: it reads the agent events the event bus already carries and pushes the stack observations that hold
// vetted plain tokens only (a tool name, a host name), it looks through the files agents wrote for credentials and reports only where and what
// kind, and it words every alert the stack publishes.
//
// Nothing that crosses to the stack holds arguments, a URL path or query, an error text, a model's words or a secret.
package agentwatch

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/models"
)

// unknownTool stands for a tool name that is not a plain token (an MCP tool may be named by its server).
const unknownTool = "another tool"

var (
	plainTool = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	plainHost = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)
	plainName = regexp.MustCompile(`^[A-Za-z0-9 _.-]{1,64}$`)
)

// Feed turns the agent events the bus publishes into observations for the stack. Push hands one over and must not block (the stack's Observe
// queues it); ActivityOn is the operator's switch for the record, read on every event and never cached, and nil means on.
type Feed struct {
	Push       func(stacksdk.Observation)
	ActivityOn func() bool
}

// Observe is the bus tap. It runs on the goroutine that published the event, so it does a cheap typed read of the switch and never waits:
// an event that is not one the record uses is dropped here.
func (f Feed) Observe(workspaceID string, ev assistant.AgentEvent) {
	if f.Push == nil || (f.ActivityOn != nil && !f.ActivityOn()) {
		return
	}
	if o, ok := observationOf(workspaceID, ev); ok {
		f.Push(o)
	}
}

// observationOf is the observation an event stands for, or false for any event the record does not use.
func observationOf(workspace string, ev assistant.AgentEvent) (stacksdk.Observation, bool) {
	o := stacksdk.Observation{Workspace: workspace, Automation: automationOf(ev), Unattended: ev.Channel == assistant.ChannelAutomation}
	switch ev.Type {
	case assistant.EventGuardrailViolation:
		payload, _ := ev.Payload.(map[string]string)
		o.Kind, o.Tool = stacksdk.ObservedRefused, toolName(payload["tool"])
	case assistant.EventGuardrailBlocked:
		p, ok := ev.Payload.(assistant.GuardrailBlockedPayload)
		if !ok {
			return stacksdk.Observation{}, false
		}
		o.Kind, o.Tool = stacksdk.ObservedApproval, toolName(p.Tool)
	case assistant.EventToolCall:
		host := fetchHost(ev)
		if o.Automation == "" || host == "" {
			return stacksdk.Observation{}, false
		}
		o.Kind, o.Host = stacksdk.ObservedFetch, host
	default:
		return stacksdk.Observation{}, false
	}
	return o, true
}

// fetchHost is the host of a fetch_url call an automation made, or "" for anything else.
func fetchHost(ev assistant.AgentEvent) string {
	if ev.Channel != assistant.ChannelAutomation {
		return ""
	}
	call, ok := ev.Payload.(models.ToolCall)
	if !ok || call.Function.Name != models.ToolNetworkFetch {
		return ""
	}
	return hostOf(call.Function.Arguments)
}

// toolName keeps a tool's name when it is a plain token.
func toolName(name string) string {
	if plainTool.MatchString(name) {
		return name
	}
	return unknownTool
}

// automationOf is the event's automation name when it is a plain one.
func automationOf(ev assistant.AgentEvent) string {
	if plainName.MatchString(ev.Automation) {
		return ev.Automation
	}
	return ""
}

// hostOf is the lower-cased host name (or address) of a fetch_url call's url, or "" when the arguments hold no http(s) URL with a plain host.
func hostOf(arguments string) string {
	var args struct {
		URL string `json:"url"`
	}
	if json.Unmarshal([]byte(arguments), &args) != nil {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(args.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if !plainHost.MatchString(host) {
		return ""
	}
	return host
}
