package stackport

import (
	"context"
	"slices"
	"strings"

	"llm-proxy-stacks/sdk"
)

// Fixed sentences a stack is told about a refused send.
const (
	msgNotifyEmpty   = "there is nothing to send"
	msgNotifyTooLong = "the text is longer than a message may be"
	msgNotifyChannel = "that channel is not an enabled connector"
)

// Notify is the SDK's Notify port over the core's connectors: a stack hands it the owner's own text and a connector type, and the core sends it
// unchanged and as plain text to that connector only if the owner enabled it (ADR-0068, which amends ADR-0036). The stack never sees an address,
// a token or a chat; what it may name is a connector type, and one that is not enabled is refused here, not trusted.
type Notify struct {
	// Types lists the connector types that are configured and enabled now.
	Types func() []string
	// Deliver sends text as plain text (never parsed as markup) to one connector type.
	Deliver func(ctx context.Context, channel, text string) error
}

// Channels lists the enabled connector types.
func (n Notify) Channels(context.Context) ([]string, error) {
	return slices.Clone(n.Types()), nil
}

// Send checks the text and the channel, then sends. A failure of the connector itself is reported as unavailable with the connector's own words,
// which go only to the owner's own screen (the reminder's last error).
func (n Notify) Send(ctx context.Context, channel, text string) error {
	switch {
	case strings.TrimSpace(text) == "":
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgNotifyEmpty)
	case len(text) > stacksdk.MaxNotifyBytes:
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgNotifyTooLong)
	case !slices.Contains(n.Types(), channel):
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgNotifyChannel)
	}
	if err := n.Deliver(ctx, channel, text); err != nil {
		return stacksdk.Errorf(stacksdk.CodeUnavailable, err.Error())
	}
	return nil
}
