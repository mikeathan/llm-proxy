package orchestrator

// slot_manager persists llama.cpp KV-cache slots to disk so the system
// prompt doesn't need re-processing on every request.  The cache key
// includes temperature, top_p, and presence_penalty — sampling-parameter
// mismatches would corrupt the attention state, so each parameter
// combination gets an independent slot.  The manager calls POST /slots/{n}
// ?action=save|restore on the llama.cpp server.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"llm-proxy/internal/platform/ledger"
)

// SlotManager persists llama.cpp KV-cache slots to skip prompt
// re-processing on repeated requests with the same system prompt and
// sampling parameters.  The cache key includes temperature, top_p, and
// presence_penalty to prevent KV-cache state corruption.
type SlotManager struct {
	store *ledger.Store
}

func NewSlotManager(store *ledger.Store) *SlotManager {
	return &SlotManager{store: store}
}

type SlotParams struct {
	ModelName       string
	SystemPrompt    string
	FirstUserMsg    string
	Temperature     float64
	TopP            float64
	PresencePenalty float64
	Host            string
	Port            int
	TTL             time.Duration
}

func (m *SlotManager) cacheKey(p SlotParams) string {
	raw := fmt.Sprintf("%s|%s|%f|%f|%f",
		p.SystemPrompt, p.FirstUserMsg, p.Temperature, p.TopP, p.PresencePenalty)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash)
}

func (m *SlotManager) baseURL(host string, port int) string {
	return fmt.Sprintf("http://%s:%d", host, port)
}

// RestoreIfCached checks the ledger for a matching slot and, if found,
// tells llama.cpp to restore it via POST /slots/{n}?action=restore.
// Returns true if a slot was restored (prompt re-processing was skipped).
func (m *SlotManager) RestoreIfCached(ctx context.Context, p SlotParams) (bool, error) {
	if m == nil || m.store == nil {
		return false, nil
	}
	key := m.cacheKey(p)
	slot, err := m.store.GetActiveSlot(ctx, p.ModelName, p.Host, p.Port, key)
	if err != nil {
		return false, fmt.Errorf("slot lookup: %w", err)
	}
	if slot == nil {
		return false, nil
	}
	restoreURL := fmt.Sprintf("%s/slots/%d?action=restore", m.baseURL(p.Host, p.Port), slot.SlotID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, restoreURL, nil)
	if err != nil {
		return false, fmt.Errorf("slot restore request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("slot restore: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	return true, nil
}

// SaveAfterResponse scans llama.cpp for idle slots and persists the first
// one via POST /slots/{n}?action=save, recording the mapping in the ledger.
// The cache key includes sampling params so temperature changes create
// separate slots (preventing KV-cache state corruption).
func (m *SlotManager) SaveAfterResponse(ctx context.Context, p SlotParams) error {
	if m == nil || m.store == nil {
		return nil
	}
	slotsURL := m.baseURL(p.Host, p.Port) + "/slots"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, slotsURL, nil)
	if err != nil {
		return fmt.Errorf("slot list request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("slot list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slot list returned %d", resp.StatusCode)
	}

	var slots []struct {
		ID    int  `json:"id"`
		Idle  bool `json:"idle"`
		State int  `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&slots); err != nil {
		return fmt.Errorf("slot list parse: %w", err)
	}

	for _, s := range slots {
		if s.Idle && s.State >= 0 {
			saveURL := fmt.Sprintf("%s/slots/%d?action=save", m.baseURL(p.Host, p.Port), s.ID)
			saveReq, err := http.NewRequestWithContext(ctx, http.MethodPost, saveURL, nil)
			if err != nil {
				continue
			}
			saveResp, err := http.DefaultClient.Do(saveReq)
			if err != nil {
				continue
			}
			saveResp.Body.Close()
			if saveResp.StatusCode != http.StatusOK {
				continue
			}
			key := m.cacheKey(p)
			return m.store.SaveSlot(ctx, ledger.SlotRecord{
				ModelName:  p.ModelName,
				SlotID:     s.ID,
				Host:       p.Host,
				Port:       p.Port,
				CacheKey:   key,
				ExpiresAt:  time.Now().Add(p.TTL),
				LastUsedAt: time.Now(),
			})
		}
	}
	return nil
}
