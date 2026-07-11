package handlers

import (
	"net/http"

	"llm-proxy/models"
)

// SecretsHandlers serves provider API key and tool secret endpoints.
// The frontend receives only masked values (sk-...1234) — never the real credential.
type SecretsHandlers struct {
	admin AdminService
}

func NewSecretsHandlers(admin AdminService) *SecretsHandlers {
	return &SecretsHandlers{admin: admin}
}

// AdminProviderKeysHandler returns the masked set of API keys for a provider.
func (h *SecretsHandlers) AdminProviderKeysHandler(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}

	keys := h.admin.Secrets().MaskedProviderKeys(provider)
	respondJSON(w, keys)
}

// AdminProviderKeysPutHandler replaces the full key set for a provider.
// Any key that carries a masked value is automatically hydrated from the store
// (the store handles this internally in SetProviderKeys), so round-tripping
// through the masked UI never discards a real key.
// After saving, models whose CredentialID no longer matches any remaining key
// are removed to prevent dead config.
func (h *SecretsHandlers) AdminProviderKeysPutHandler(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}

	var keys []models.APIKeyItem
	if !decodeJSONBody(w, r, &keys) {
		return
	}
	if keys == nil {
		keys = []models.APIKeyItem{}
	}

	if err := h.admin.Secrets().SetProviderKeys(provider, keys); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save keys: "+err.Error())
		return
	}

	remainingKeyNames := make(map[string]bool, len(keys))
	for _, k := range keys {
		remainingKeyNames[k.Name] = true
	}
	if err := h.admin.UpdateRegistry(func(reg *models.RegistryData) {
		out := reg.Catalogue[:0]
		for _, m := range reg.Catalogue {
			if m.ProviderID != provider {
				out = append(out, m)
				continue
			}
			if m.CredentialID != "" && remainingKeyNames[m.CredentialID] {
				out = append(out, m)
				continue
			}
			if m.CredentialID == "" && len(keys) > 0 {
				out = append(out, m)
				continue
			}
		}
		reg.Catalogue = out
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to cleanup orphaned models: "+err.Error())
		return
	}

	respondJSON(w, h.admin.Secrets().MaskedProviderKeys(provider))
}

func cascadeRemoveModelsForKey(reg *models.RegistryData, provider, keyName, keyID string, remainingKeyCount int) {
	out := reg.Catalogue[:0]
	for _, m := range reg.Catalogue {
		if m.ProviderID != provider {
			out = append(out, m)
			continue
		}
		if m.CredentialID != "" && m.CredentialID != keyName && m.CredentialID != keyID {
			out = append(out, m)
			continue
		}
		if m.CredentialID == "" && remainingKeyCount > 0 {
			out = append(out, m)
			continue
		}
	}
	reg.Catalogue = out
}

// AdminProviderKeyDeleteHandler removes a single key by ID from a provider,
// or all keys for the provider when key_id is empty.
func (h *SecretsHandlers) AdminProviderKeyDeleteHandler(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}

	keyID := r.URL.Query().Get("key_id")
	if keyID == "" {
		if err := h.admin.DeleteProviderWithCleanup(provider); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		respondJSON(w, h.admin.Secrets().MaskedProviderKeys(provider))
		return
	}

	keys := h.admin.Secrets().GetProviderKeys(provider)
	var targetName, targetID string
	for _, k := range keys {
		if k.ID == keyID {
			targetName = k.Name
			targetID = k.ID
			break
		}
	}

	if err := h.admin.Secrets().DeleteProviderKey(provider, keyID); err != nil {
		if err == models.ErrSecretNotFound {
			writeJSONError(w, http.StatusNotFound, "key not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete key: "+err.Error())
		return
	}

	remainingKeys := h.admin.Secrets().GetProviderKeys(provider)
	if err := h.admin.UpdateRegistry(func(reg *models.RegistryData) {
		cascadeRemoveModelsForKey(reg, provider, targetName, targetID, len(remainingKeys))
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to cleanup orphaned models: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *SecretsHandlers) AdminToolSecretHandler(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	provider := r.URL.Query().Get("provider")
	if category == "" || provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing category or provider")
		return
	}
	respondJSON(w, map[string]string{"secret": h.admin.Secrets().MaskedSecret(category, provider)})
}

func (h *SecretsHandlers) AdminToolSecretPutHandler(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	provider := r.URL.Query().Get("provider")
	if category == "" || provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing category or provider")
		return
	}
	var req struct {
		Secret string `json:"secret"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.admin.Secrets().SetSecret(category, provider, req.Secret); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save secret: "+err.Error())
		return
	}

	respondJSON(w, map[string]string{"secret": h.admin.Secrets().MaskedSecret(category, provider)})
}
