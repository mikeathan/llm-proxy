package api

import (
	"net/http"

	"llm-proxy/models"
)

// AdminProviderKeysHandler returns the masked set of API keys for a provider.
// The frontend receives only masked values (sk-...1234) — never the real credential.
func (h *AdminHandlers) AdminProviderKeysHandler(w http.ResponseWriter, r *http.Request) {
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
func (h *AdminHandlers) AdminProviderKeysPutHandler(w http.ResponseWriter, r *http.Request) {
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

	// Respond with the masked representation of the saved keys.
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
func (h *AdminHandlers) AdminProviderKeyDeleteHandler(w http.ResponseWriter, r *http.Request) {
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

	// Capture key identity before deleting so we can cascade
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

	// Cascade: remove models that reference this key
	remainingKeys := h.admin.Secrets().GetProviderKeys(provider)
	if err := h.admin.UpdateRegistry(func(reg *models.RegistryData) {
		cascadeRemoveModelsForKey(reg, provider, targetName, targetID, len(remainingKeys))
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to cleanup orphaned models: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminToolSecretHandler returns the masked value of a tool secret.
func (h *AdminHandlers) AdminToolSecretHandler(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	provider := r.URL.Query().Get("provider")
	if category == "" || provider == "" {
		writeJSONError(w, http.StatusBadRequest, "missing category or provider")
		return
	}
	respondJSON(w, map[string]string{"secret": h.admin.Secrets().MaskedSecret(category, provider)})
}

// AdminToolSecretPutHandler sets a tool secret.
func (h *AdminHandlers) AdminToolSecretPutHandler(w http.ResponseWriter, r *http.Request) {
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

	// Respond with the masked representation.
	respondJSON(w, map[string]string{"secret": h.admin.Secrets().MaskedSecret(category, provider)})
}
