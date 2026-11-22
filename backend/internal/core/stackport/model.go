package stackport

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"
)

// Fixed sentences a stack is told about a refused or failed model call: never the provider's own words, which can quote the prompt.
const (
	msgModelOff      = "the module is switched off"
	msgModelEmpty    = "there is nothing to ask"
	msgModelTooLong  = "the question is longer than a model call may carry"
	msgModelAnswer   = "the answer is longer than a model call may return"
	msgModelBusy     = "a model call is already running for this stack"
	msgModelCap      = "the stack's limit of model calls for today is spent"
	msgModelNone     = "no model is chosen"
	msgModelDown     = "no model could answer"
	msgModelTimedOut = "the model ran out of time"
)

// Bounds of the model port (ADR-0071). The daily limit is the owner's setting, kept between 1 and ModelDailyLimitMax; ModelDailyLimit is what it is
// until they set one.
const (
	ModelTimeout       = 120 * time.Second
	ModelDailyLimit    = 200
	ModelDailyLimitMax = 2000
	modelLogCallDone   = "stack model call"
)

// ModelAnswer is what the core's own completion gives back: the text, and whether the model that wrote it runs on this computer.
type ModelAnswer struct {
	Text  string
	Local bool
}

// Model is the SDK's Model port over the core's configured model (ADR-0071). The core chooses the model; the stack cannot name one, and the call
// carries no tools, memory or history (Ask is a one-shot completion the core builds). It enforces the bounds the ADR sets: sizes in and out, a
// timeout, one call in flight, a daily limit and the module's switch. It logs the fact of a call and never its text.
type Model struct {
	// Stack is the stack's id, for the log line.
	Stack string
	// Enabled says whether the stack's module is switched on now; the port refuses while it is off.
	Enabled func() bool
	// Ask asks the owner's configured model one question, with no tools, memory or history.
	Ask func(ctx context.Context, system, user string) (ModelAnswer, error)
	// Log writes one structured line; nil logs nothing.
	Log func(msg string, fields ...string)
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Timeout bounds one call from start to answer; zero means ModelTimeout. A core whose Ask also waits for a cold model to load adds that wait.
	Timeout time.Duration
	// Limit gives the daily limit of calls, read at every call so a change the owner makes applies at once; nil, or an answer of zero or less, means
	// ModelDailyLimit, and an answer above ModelDailyLimitMax is held to it.
	Limit func() int

	mu     sync.Mutex
	busy   bool
	day    string
	called int
}

func (m *Model) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

// admit checks the switch and the limits and claims the one call in flight; the caller must release it.
func (m *Model) admit() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Enabled != nil && !m.Enabled() {
		return stacksdk.Errorf(stacksdk.CodeUnavailable, msgModelOff)
	}
	if m.busy {
		return stacksdk.Errorf(stacksdk.CodeUnavailable, msgModelBusy)
	}
	today := m.now().Format("2006-01-02")
	if today != m.day {
		m.day, m.called = today, 0
	}
	limit := m.dailyLimit()
	if m.called >= limit {
		return stacksdk.Errorf(stacksdk.CodeDenied, msgModelCap)
	}
	m.busy = true
	m.called++
	return nil
}

// dailyLimit is today's limit of calls: the owner's setting held to the bounds, or the default.
func (m *Model) dailyLimit() int {
	limit := 0
	if m.Limit != nil {
		limit = m.Limit()
	}
	switch {
	case limit <= 0:
		return ModelDailyLimit
	case limit > ModelDailyLimitMax:
		return ModelDailyLimitMax
	}
	return limit
}

func (m *Model) release() {
	m.mu.Lock()
	m.busy = false
	m.mu.Unlock()
}

// ErrNoModel is what Ask returns when the owner has chosen no model; the port answers it with CodeDenied and a fixed sentence.
var ErrNoModel = errors.New("no model is chosen")

// Complete checks the question, makes the call and checks the answer.
func (m *Model) Complete(ctx context.Context, req stacksdk.ModelRequest) (stacksdk.ModelResponse, error) {
	switch {
	case strings.TrimSpace(req.User) == "":
		return stacksdk.ModelResponse{}, stacksdk.Errorf(stacksdk.CodeInvalid, msgModelEmpty)
	case len(req.System)+len(req.User) > stacksdk.MaxModelPromptBytes:
		return stacksdk.ModelResponse{}, stacksdk.Errorf(stacksdk.CodeInvalid, msgModelTooLong)
	}
	if err := m.admit(); err != nil {
		return stacksdk.ModelResponse{}, err
	}
	defer m.release()
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = ModelTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	ans, err := m.Ask(ctx, req.System, req.User)
	return m.finish(req, ans, err, time.Since(start))
}

// finish classifies the outcome, logs its metadata and returns the answer or the fixed refusal.
func (m *Model) finish(req stacksdk.ModelRequest, ans ModelAnswer, err error, took time.Duration) (stacksdk.ModelResponse, error) {
	result, out := "ok", stacksdk.ModelResponse{Text: ans.Text}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result, err = "timeout", stacksdk.Errorf(stacksdk.CodeTimeout, msgModelTimedOut)
	case errors.Is(err, ErrNoModel):
		// Denied, as for a spent limit: a stack may carry on without a comparison, and a retry cannot help until the owner chooses a model.
		result, err = "no_model", stacksdk.Errorf(stacksdk.CodeDenied, msgModelNone)
	case err != nil:
		result, err = "error", stacksdk.Errorf(stacksdk.CodeUnavailable, msgModelDown)
	case len(ans.Text) > stacksdk.MaxModelAnswerBytes:
		result, out, err = "answer_too_long", stacksdk.ModelResponse{}, stacksdk.Errorf(stacksdk.CodeInvalid, msgModelAnswer)
	}
	if m.Log != nil {
		class := "cloud"
		if ans.Local {
			class = "local"
		}
		m.Log(modelLogCallDone, "stack", m.Stack, "in_bytes", strconv.Itoa(len(req.System)+len(req.User)), "out_bytes", strconv.Itoa(len(ans.Text)),
			"millis", strconv.FormatInt(took.Milliseconds(), 10), "model_class", class, "result", result)
	}
	if err != nil {
		return stacksdk.ModelResponse{}, err
	}
	return out, nil
}
