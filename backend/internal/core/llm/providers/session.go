package providers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ConversationSessionID derives a stable session id for a chat request body, for providers that route and cache by
// conversation (ProviderManifest.SessionHeader). Every turn of a conversation re-sends its first system and user
// messages, so hashing them with the model gives the same id for the whole conversation and a different one for the
// next, without the caller having to carry an id. A body that does not parse still gets an id (of the whole body).
func ConversationSessionID(body []byte) string {
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	h := sha256.New()
	if json.Unmarshal(body, &req) != nil {
		h.Write(body)
		return "ses_" + hex.EncodeToString(h.Sum(nil))[:24]
	}
	h.Write([]byte(req.Model))
	seen := map[string]bool{}
	for _, m := range req.Messages {
		if (m.Role == "system" || m.Role == "user") && !seen[m.Role] {
			seen[m.Role] = true
			h.Write([]byte{0})
			h.Write([]byte(m.Role))
			h.Write(m.Content)
		}
	}
	return "ses_" + hex.EncodeToString(h.Sum(nil))[:24]
}

// SessionHeader returns the header this provider wants carrying a conversation id, or "" (see ProviderManifest).
func (p *OpenAICompatibleProvider) SessionHeader() string {
	return p.manifest.SessionHeader
}
