package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"llm-proxy/internal/platform/logging"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
)

// maxLockdownBody bounds the switch's request body.
const maxLockdownBody = 1 << 10

// LockdownSwitch is the lock down switch: *lockdown.Manager.
type LockdownSwitch interface {
	State() models.Lockdown
	Engage(keepSession string, revokeTokens bool) (models.Lockdown, error)
	Release() (models.Lockdown, error)
}

// LockdownHandlers serve the lock down switch. Switching it on and lifting it both need the passphrase (step-up).
type LockdownHandlers struct{ sw LockdownSwitch }

func NewLockdownHandlers(sw LockdownSwitch) *LockdownHandlers { return &LockdownHandlers{sw: sw} }

// Get handles GET /admin/api/lockdown.
func (h *LockdownHandlers) Get(w http.ResponseWriter, _ *http.Request) { respondJSON(w, h.sw.State()) }

// Engage handles POST /admin/api/lockdown {"revoke_tokens": bool}: it switches lock down on and signs out every session but the
// one that asked. What it could not end is logged and the switch stays on: a half-done lock down is still a lock down.
func (h *LockdownHandlers) Engage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RevokeTokens bool `json:"revoke_tokens"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxLockdownBody))
	if err != nil || (len(body) > 0 && json.Unmarshal(body, &req) != nil) {
		writeJSONError(w, http.StatusBadRequest, "the body must be {\"revoke_tokens\": true or false}")
		return
	}
	keep := ""
	if c, err := r.Cookie(api.SessionCookie); err == nil {
		keep = c.Value
	}
	st, err := h.sw.Engage(keep, req.RevokeTokens)
	if err != nil {
		logging.Warn("lock down", "error", err.Error())
		if !st.On {
			writeJSONError(w, http.StatusInternalServerError, "failed to switch lock down on")
			return
		}
	}
	respondJSON(w, st)
}

// Release handles DELETE /admin/api/lockdown.
func (h *LockdownHandlers) Release(w http.ResponseWriter, _ *http.Request) {
	st, err := h.sw.Release()
	if err != nil {
		logging.Warn("lock down", "error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, "failed to lift lock down")
		return
	}
	respondJSON(w, st)
}
