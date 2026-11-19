package stackport

import (
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

func notifyOver(sent *[]string, fail error) Notify {
	return Notify{
		Types: func() []string { return []string{"telegram", "slack"} },
		Deliver: func(_ context.Context, channel, text string) error {
			*sent = append(*sent, channel+":"+text)
			return fail
		},
	}
}

func TestNotify_SendsTheTextUnchangedToAnEnabledConnector(t *testing.T) {
	var sent []string
	n := notifyOver(&sent, nil)
	text := "call <b>Dr</b> Smith"
	if err := n.Send(context.Background(), "telegram", text); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != "telegram:"+text {
		t.Fatalf("sent %v", sent)
	}
}

func TestNotify_RefusesWhatIsNotAnEnabledConnectorOrNotText(t *testing.T) {
	var sent []string
	n := notifyOver(&sent, nil)
	for name, tc := range map[string]struct{ channel, text string }{
		"unknown channel":           {"whatsapp", "x"},
		"in-app is not a connector": {"in-app", "x"},
		"empty channel":             {"", "x"},
		"empty text":                {"telegram", "  "},
		"too long":                  {"telegram", strings.Repeat("a", stacksdk.MaxNotifyBytes+1)},
	} {
		if err := n.Send(context.Background(), tc.channel, tc.text); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
			t.Errorf("%s: %v, want invalid", name, err)
		}
	}
	if len(sent) != 0 {
		t.Fatalf("something was sent: %v", sent)
	}
}

func TestNotify_AConnectorFailureIsUnavailableWithItsWords(t *testing.T) {
	var sent []string
	err := notifyOver(&sent, errors.New("telegram is down")).Send(context.Background(), "telegram", "x")
	if stacksdk.CodeOf(err) != stacksdk.CodeUnavailable || !strings.Contains(err.Error(), "telegram is down") {
		t.Fatalf("got %v", err)
	}
}

func TestNotify_ChannelsAreACopy(t *testing.T) {
	var sent []string
	got, _ := notifyOver(&sent, nil).Channels(context.Background())
	got[0] = "changed"
	again, _ := notifyOver(&sent, nil).Channels(context.Background())
	if again[0] != "telegram" {
		t.Fatalf("a stack changed the core's list: %v", again)
	}
}
