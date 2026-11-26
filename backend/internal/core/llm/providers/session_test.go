package providers

import (
	"testing"

	"llm-proxy/models"
)

func TestConversationSessionID_StablePerConversation(t *testing.T) {
	turn1 := `{"model":"glm","messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"}]}`
	turn2 := `{"model":"glm","messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"more"}]}`
	other := `{"model":"glm","messages":[{"role":"system","content":"be brief"},{"role":"user","content":"something else"}]}`

	a, b, c := ConversationSessionID([]byte(turn1)), ConversationSessionID([]byte(turn2)), ConversationSessionID([]byte(other))
	if a != b {
		t.Fatalf("two turns of one conversation got %q and %q, want the same id", a, b)
	}
	if a == c {
		t.Fatal("two conversations got the same id")
	}
	if len(a) != len("ses_")+24 || a[:4] != "ses_" {
		t.Fatalf("id %q, want ses_ and 24 hex characters", a)
	}
	if got := ConversationSessionID([]byte("not json")); got[:4] != "ses_" {
		t.Fatalf("unparsable body id = %q", got)
	}
}

func TestOpenCodeGoManifest_AsksForASessionHeader(t *testing.T) {
	m, ok := GetRegistry().Get("opencode-go")
	if !ok {
		t.Fatal("the opencode-go manifest is not registered")
	}
	if m.DefaultBaseURL != "https://opencode.ai/inference/go/openai/v1" || m.Archetype != models.ArchetypeOpenAICompatible {
		t.Fatalf("manifest = %+v", m)
	}
	if got := NewOpenAICompatibleProvider(models.ModelConfig{}, m).SessionHeader(); got != "x-opencode-session" {
		t.Fatalf("SessionHeader() = %q, want x-opencode-session", got)
	}
}
