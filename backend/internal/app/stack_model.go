package app

import (
	"context"
	"errors"
	"strings"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/assistant/reasoning"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/stackport"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// What a stack's model call asks of the model (ADR-0071): a short, plain answer and no thinking, as the memory review's call does.
const (
	stackModelMaxTokens   = 2000
	stackModelTemperature = 0.2
)

// stackModelPort builds the Model port a stack with the given id is handed: the owner's configured model answers one tool-free question, while the
// module's switch is on, up to the daily limit the owner set (limit; nil means the default). Only a stack whose manifest requires PortModel is given it (checkPorts).
func (s *AppServices) stackModelPort(stackID string, enabled func() bool, limit func() int) *stackport.Model {
	return &stackport.Model{
		Stack: stackID, Enabled: enabled, Limit: limit, Ask: s.stackModelAsk,
		// The call's own time (stackport.ModelTimeout) starts after a cold local model has loaded, so the outer bound allows for that wait too.
		Timeout: models.ModelStartWaitTimeout + stackport.ModelTimeout,
		Log:     func(msg string, fields ...string) { logging.Info(msg, stringsToAny(fields)...) },
	}
}

func stringsToAny(fields []string) []any {
	out := make([]any, len(fields))
	for i, f := range fields {
		out[i] = f
	}
	return out
}

// stackModelAsk is the one-shot completion behind the port: the model the owner chose for the assistant (primary, else fallback), waited for if it is a
// local model still loading, asked once with no tools, no memory and no history, and not admitted through the run lane (it is background work and must
// not take a chat's place). It never falls back to a model the owner did not choose.
func (s *AppServices) stackModelAsk(ctx context.Context, system, user string) (stackport.ModelAnswer, error) {
	if s.clientProvider == nil || s.AppCtx == nil {
		return stackport.ModelAnswer{}, stackport.ErrNoModel
	}
	// SelectModels already gives the fallback when no primary is set, and never an arbitrary catalogue entry.
	model, _ := s.SelectModels()
	if model == "" {
		return stackport.ModelAnswer{}, stackport.ErrNoModel
	}
	client, err := proxy.WaitForClient(ctx, func() (proxy.Client, error) { return s.GetClientForModel(ctx, model) }, proxy.ModelWait{
		ModelName: model, PollInterval: models.ModelStartPollInterval, Timeout: models.ModelStartWaitTimeout,
	})
	if err != nil {
		return stackport.ModelAnswer{}, err
	}
	req := proxy.ChatRequest{
		Model:       model,
		Messages:    []proxy.Message{{Role: proxy.SystemRole, Content: system}, {Role: proxy.UserRole, Content: user}},
		MaxTokens:   stackModelMaxTokens,
		Temperature: stackModelTemperature,
	}
	if client.ReasoningField() == proxy.ReasoningFieldThinkTokens {
		reasoning.DisableThinking(&req)
	}
	// The model has loaded: only the call itself is held to the port's own time, so a slow start cannot eat it.
	callCtx, cancel := context.WithTimeout(ctx, stackport.ModelTimeout)
	defer cancel()
	resp, err := client.Chat(callCtx, req)
	if err != nil {
		return stackport.ModelAnswer{}, err
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return stackport.ModelAnswer{}, errors.New("the model returned no answer")
	}
	return stackport.ModelAnswer{Text: resp.Choices[0].Message.Content, Local: s.modelIsLocal(model)}, nil
}

var _ stacksdk.Model = (*stackport.Model)(nil)
