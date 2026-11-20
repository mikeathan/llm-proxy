package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"llm-proxy/internal/core/toolregistry"
	"llm-proxy/models"

	stacksdk "llm-proxy-stacks/sdk"
)

// newsroomToolModule offers the assistant the Newsroom stack's two declared tools, newsroom_follow and newsroom_topics. They are scoped to attended runs:
// a topic makes the service read its sources from then on, and an unattended automation may be steered by content it read. They are offered only while
// the Topics module is on, so the model is never shown a tool that can only refuse. The stack answers with the topic as JSON, or refuses in its own words.
func newsroomToolModule(h *stackHost) toolregistry.Module {
	// The stack's tool names are the core's (models.ToolNewsroom*); a test pins that the stack declares exactly these. The module is the stack's one.
	moduleID := h.stack.Manifest().Modules[0].ID
	available := func(ctx context.Context) bool {
		if models.IsUnattendedRun(ctx) {
			return false
		}
		st, ok := h.Module(ctx, moduleID)
		return ok && st.Enabled
	}
	call := func(name string) toolregistry.ToolHandler {
		return func(ctx context.Context, rawArgs string) (any, error) {
			// A module's Available hides its tools from the model; it does not stop a call that names one, so the handler checks it too.
			if !available(ctx) {
				return nil, toolregistry.ErrToolNotInternal
			}
			if rawArgs == "" {
				rawArgs = "{}"
			}
			out, err := h.stack.CallTool(ctx, name, json.RawMessage(rawArgs))
			if err != nil {
				var coded *stacksdk.Error
				if errors.As(err, &coded) {
					return nil, errors.New(coded.Message)
				}
				return nil, fmt.Errorf("the Newsroom stack could not answer: %w", err)
			}
			return string(out), nil
		}
	}
	return toolregistry.Module{
		Handlers: map[string]toolregistry.ToolHandler{
			models.ToolNewsroomFollow: call(models.ToolNewsroomFollow),
			models.ToolNewsroomTopics: call(models.ToolNewsroomTopics),
		},
		Available: available,
	}
}
