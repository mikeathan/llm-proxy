package app

import (
	"context"
	"strings"
	"testing"

	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

type recordingConnector struct{ sent []string }

func (c *recordingConnector) Name() string { return "Recorder" }
func (c *recordingConnector) Send(_ context.Context, m string) error {
	c.sent = append(c.sent, m)
	return nil
}

func TestConnectorNotifier(t *testing.T) {
	comm := tools.NewCommunicationTools()
	rec := &recordingConnector{}
	comm.AddConnector("my-tg", models.ConnectorTypeTelegram, rec)
	n := connectorNotifier{comm: comm}

	if err := n.Notify(context.Background(), "my-tg", "hello"); err != nil || len(rec.sent) != 1 || rec.sent[0] != "hello" {
		t.Fatalf("Notify = %v, sent %q; want delivery to the named connector", err, rec.sent)
	}
	err := n.Notify(context.Background(), "missing", "hello")
	if err == nil || !strings.Contains(err.Error(), `"missing"`) {
		t.Fatalf("an unknown connector must name itself in the error, got %v", err)
	}
	if got := communicationTools(nil); got != nil {
		t.Fatalf("communicationTools(nil) = %v, want nil", got)
	}
}
