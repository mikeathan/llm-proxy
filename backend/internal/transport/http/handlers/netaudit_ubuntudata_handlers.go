package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"llm-proxy/internal/core/vulndata/ubuntuoval"
)

// ubuntuUpdateWindow is how long one update may take; the file is about 1.3 MB.
const ubuntuUpdateWindow = 2 * time.Minute

// UbuntuData is the part of the Ubuntu security data manager the handlers use. *ubuntuoval.Manager satisfies it.
type UbuntuData interface {
	Status() ubuntuoval.Status
	Update(ctx context.Context) error
	Clear() error
}

// UbuntuDataHandlers serve the "Ubuntu security data" card: what is held, the one-click update from Canonical's fixed address,
// and deleting it. Updating is an operator action behind the admin sign-in like the vulnerability data's download; deleting is a
// step-up route, because it removes what every audit compares the installed packages with.
type UbuntuDataHandlers struct{ data UbuntuData }

func NewUbuntuDataHandlers(data UbuntuData) *UbuntuDataHandlers {
	return &UbuntuDataHandlers{data: data}
}

// Get reports what is held.
func (h *UbuntuDataHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, h.data.Status())
}

// Update fetches this computer's release notices and answers with the status. A refusal or a failure is stated in fixed words;
// the text of the error, which can hold an address, is never sent.
func (h *UbuntuDataHandlers) Update(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), ubuntuUpdateWindow)
	defer cancel()
	switch err := h.data.Update(ctx); {
	case err == nil:
		respondJSON(w, h.data.Status())
	case errors.Is(err, ubuntuoval.ErrBusy):
		writeJSONError(w, http.StatusConflict, ubuntuoval.ErrBusy.Error())
	case errors.Is(err, ubuntuoval.ErrNotUbuntu):
		writeJSONError(w, http.StatusBadRequest, ubuntuoval.ErrNotUbuntu.Error())
	default:
		writeJSONError(w, http.StatusBadGateway, h.data.Status().Reason)
	}
}

// Clear deletes the data held.
func (h *UbuntuDataHandlers) Clear(w http.ResponseWriter, _ *http.Request) {
	if err := h.data.Clear(); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ubuntuoval.ErrBusy) {
			status = http.StatusConflict
		}
		writeJSONError(w, status, err.Error())
		return
	}
	respondJSON(w, h.data.Status())
}
