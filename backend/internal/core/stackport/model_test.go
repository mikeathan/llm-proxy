package stackport

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

type modelRig struct {
	m     *Model
	logs  []string
	asked int
	now   time.Time
	reply func(ctx context.Context, system, user string) (ModelAnswer, error)
}

func newModelRig() *modelRig {
	r := &modelRig{now: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)}
	r.reply = func(context.Context, string, string) (ModelAnswer, error) {
		return ModelAnswer{Text: "answer", Local: true}, nil
	}
	r.m = &Model{
		Stack: "newsroom", Enabled: func() bool { return true }, Now: func() time.Time { return r.now },
		Ask: func(ctx context.Context, s, u string) (ModelAnswer, error) { r.asked++; return r.reply(ctx, s, u) },
		Log: func(msg string, fields ...string) { r.logs = append(r.logs, msg+" "+strings.Join(fields, " ")) },
	}
	return r
}

func ask(m *Model, user string) (stacksdk.ModelResponse, error) {
	return m.Complete(context.Background(), stacksdk.ModelRequest{System: "sys", User: user})
}

func TestModel_AnswersAndLogsOnlyMetadata(t *testing.T) {
	r := newModelRig()
	const secret = "the-private-prompt-text"
	r.reply = func(context.Context, string, string) (ModelAnswer, error) {
		return ModelAnswer{Text: "the-private-answer-text", Local: true}, nil
	}
	got, err := ask(r.m, secret)
	if err != nil || got.Text != "the-private-answer-text" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(r.logs) != 1 || !strings.Contains(r.logs[0], "model_class local") || !strings.Contains(r.logs[0], "result ok") {
		t.Fatalf("logs = %v", r.logs)
	}
	for _, l := range r.logs {
		if strings.Contains(l, "private") || strings.Contains(l, "sys") {
			t.Errorf("a log line carries the text: %q", l)
		}
	}
}

func TestModel_RefusesWhatTheADRForbidsWithoutCallingTheModel(t *testing.T) {
	r := newModelRig()
	cases := map[string]struct {
		req  stacksdk.ModelRequest
		code stacksdk.Code
	}{
		"empty":    {stacksdk.ModelRequest{System: "s", User: "  "}, stacksdk.CodeInvalid},
		"too long": {stacksdk.ModelRequest{System: "s", User: strings.Repeat("x", stacksdk.MaxModelPromptBytes)}, stacksdk.CodeInvalid},
	}
	for name, c := range cases {
		if _, err := r.m.Complete(context.Background(), c.req); stacksdk.CodeOf(err) != c.code {
			t.Errorf("%s: %v", name, err)
		}
	}
	if r.asked != 0 {
		t.Errorf("a refused question reached the model %d times", r.asked)
	}
}

func TestModel_RefusesWhileTheModuleIsOff(t *testing.T) {
	r := newModelRig()
	r.m.Enabled = func() bool { return false }
	if _, err := ask(r.m, "q"); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable || r.asked != 0 {
		t.Errorf("err = %v, asked = %d", err, r.asked)
	}
}

func TestModel_AnAnswerOverTheLimitIsRefusedNotTruncated(t *testing.T) {
	r := newModelRig()
	r.reply = func(context.Context, string, string) (ModelAnswer, error) {
		return ModelAnswer{Text: strings.Repeat("a", stacksdk.MaxModelAnswerBytes+1)}, nil
	}
	got, err := ask(r.m, "q")
	if stacksdk.CodeOf(err) != stacksdk.CodeInvalid || got.Text != "" {
		t.Errorf("got %d bytes, %v", len(got.Text), err)
	}
	if !strings.Contains(r.logs[0], "model_class cloud") || !strings.Contains(r.logs[0], "answer_too_long") {
		t.Errorf("log = %v", r.logs)
	}
}

func TestModel_AFailureIsAFixedSentenceNotTheProvidersWords(t *testing.T) {
	r := newModelRig()
	r.reply = func(context.Context, string, string) (ModelAnswer, error) {
		return ModelAnswer{}, errors.New("401 for key sk-secret echoing the-private-prompt")
	}
	_, err := ask(r.m, "q")
	var e *stacksdk.Error
	if !errors.As(err, &e) || e.Code != stacksdk.CodeUnavailable || strings.Contains(e.Message, "sk-") || strings.Contains(r.logs[0], "sk-") {
		t.Errorf("err = %v, log = %v", err, r.logs)
	}
}

// No model chosen is a refusal, not an outage: a stack carries on without a comparison instead of retrying, and the sentence is the core's.
func TestModel_NoModelChosenIsDeniedWithAFixedSentence(t *testing.T) {
	r := newModelRig()
	r.reply = func(context.Context, string, string) (ModelAnswer, error) { return ModelAnswer{}, ErrNoModel }
	_, err := ask(r.m, "q")
	var e *stacksdk.Error
	if !errors.As(err, &e) || e.Code != stacksdk.CodeDenied || e.Message != "no model is chosen" {
		t.Errorf("err = %v", err)
	}
}

func TestModel_ATimeoutIsATimeoutCode(t *testing.T) {
	r := newModelRig()
	r.reply = func(context.Context, string, string) (ModelAnswer, error) {
		return ModelAnswer{}, context.DeadlineExceeded
	}
	if _, err := ask(r.m, "q"); stacksdk.CodeOf(err) != stacksdk.CodeTimeout {
		t.Errorf("err = %v", err)
	}
}

func TestModel_TheTimeoutIsTheCoresToSetSoAColdStartCanBeAllowedFor(t *testing.T) {
	r := newModelRig()
	r.reply = func(ctx context.Context, _, _ string) (ModelAnswer, error) {
		select {
		case <-time.After(40 * time.Millisecond):
			return ModelAnswer{Text: "late but in time"}, nil
		case <-ctx.Done():
			return ModelAnswer{}, ctx.Err()
		}
	}
	r.m.Timeout = 10 * time.Millisecond
	if _, err := ask(r.m, "q"); stacksdk.CodeOf(err) != stacksdk.CodeTimeout {
		t.Errorf("a short timeout must time out, got %v", err)
	}
	r.m.Timeout = time.Second
	if got, err := ask(r.m, "q"); err != nil || got.Text != "late but in time" {
		t.Errorf("a longer timeout waits, got %+v, %v", got, err)
	}
}

func TestModel_OnlyOneCallRunsAtATime(t *testing.T) {
	r := newModelRig()
	started, release := make(chan struct{}), make(chan struct{})
	r.reply = func(context.Context, string, string) (ModelAnswer, error) {
		close(started)
		<-release
		return ModelAnswer{Text: "late"}, nil
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _, _ = ask(r.m, "first") }()
	<-started
	if _, err := ask(r.m, "second"); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("a second call while one runs must be refused, got %v", err)
	}
	close(release)
	wg.Wait()
	r.reply = func(context.Context, string, string) (ModelAnswer, error) { return ModelAnswer{Text: "ok"}, nil }
	if _, err := ask(r.m, "third"); err != nil && stacksdk.CodeOf(err) == stacksdk.CodeUnavailable {
		t.Errorf("the slot is free again, got %v", err)
	}
}

func TestModel_TheDailyLimitIsSpentThenFreedTomorrow(t *testing.T) {
	r := newModelRig()
	r.m.Limit = 2
	for range 2 {
		if _, err := ask(r.m, "q"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ask(r.m, "q"); stacksdk.CodeOf(err) != stacksdk.CodeDenied {
		t.Errorf("the third call is over the limit, got %v", err)
	}
	r.now = r.now.Add(24 * time.Hour)
	if _, err := ask(r.m, "q"); err != nil {
		t.Errorf("a new day has a new limit, got %v", err)
	}
}
