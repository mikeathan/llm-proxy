package stackport

import (
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

type onceADay struct{}

func (onceADay) Next(t time.Time) time.Time { return t.Add(24 * time.Hour) }

func TestSchedule_AnswersWithTheParsersNext(t *testing.T) {
	s := Schedule{Parse: func(expr string) (interface{ Next(time.Time) time.Time }, error) { return onceADay{}, nil }}
	at := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	got, err := s.Next("0 9 * * *", at)
	if err != nil || !got.Equal(at.Add(24*time.Hour)) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestSchedule_ABadExpressionIsInvalidWithTheGrammarsOwnWords(t *testing.T) {
	s := Schedule{Parse: func(expr string) (interface{ Next(time.Time) time.Time }, error) {
		return nil, errString("expected exactly 5 fields, found 1: " + expr)
	}}
	_, err := s.Next("nope", time.Now())
	var coded *stacksdk.Error
	if err == nil || stacksdk.CodeOf(err) != stacksdk.CodeInvalid || !strings.Contains(err.Error(), "expected exactly 5 fields") {
		t.Fatalf("got %v %v", err, coded)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
