package handlers

import (
	"errors"
	"net/http"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

// DeviceInventory is what the operator can do with the devices seen on the network: look, name, approve, forget, and
// confirm the router's hardware address. *netaudit.Inventory satisfies it.
type DeviceInventory interface {
	View() (models.DeviceInventory, error)
	Name(mac, name string) error
	Approve(mac string) error
	Forget(mac string) error
	ConfirmGateway() error
}

// PresenceControl is the presence check's settings and its status. The settings and the status come from two objects.
type PresenceControl interface {
	View() netaudit.PresenceView
	Set(enabled bool, minutes int) error
	Status() netaudit.PresenceStatus
}

// NetauditDevicesHandlers serve the device list and the presence check's settings.
type NetauditDevicesHandlers struct {
	devices  DeviceInventory
	presence PresenceControl
	now      func() time.Time
}

func NewNetauditDevicesHandlers(devices DeviceInventory, presence PresenceControl) *NetauditDevicesHandlers {
	return &NetauditDevicesHandlers{devices: devices, presence: presence, now: time.Now}
}

// List returns every device seen on the network, those that need a look first, and the router's hardware address.
func (h *NetauditDevicesHandlers) List(w http.ResponseWriter, _ *http.Request) {
	inv, err := h.devices.View()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, netaudit.ViewOf(inv, h.now()))
}

// deviceRequest changes one device. A name approves it too; Approved approves it without a name.
type deviceRequest struct {
	Name     *string `json:"name"`
	Approved *bool   `json:"approved"`
}

// Update names a device (which approves it) or approves it. Neither field is a mistake.
func (h *NetauditDevicesHandlers) Update(w http.ResponseWriter, r *http.Request) {
	var req deviceRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	mac := r.PathValue("mac")
	var err error
	switch {
	case req.Name != nil:
		err = h.devices.Name(mac, *req.Name)
	case req.Approved != nil && *req.Approved:
		err = h.devices.Approve(mac)
	default:
		writeJSONError(w, http.StatusBadRequest, "say what to change: a name, or approved")
		return
	}
	h.respondChange(w, err)
}

// Forget removes a device from the list; if it is on the network it comes back as a new device.
func (h *NetauditDevicesHandlers) Forget(w http.ResponseWriter, r *http.Request) {
	h.respondChange(w, h.devices.Forget(r.PathValue("mac")))
}

// ConfirmGateway accepts the router's current hardware address as the right one. It silences the alarm for a spoofed
// router, so it is a step-up route.
func (h *NetauditDevicesHandlers) ConfirmGateway(w http.ResponseWriter, _ *http.Request) {
	h.respondChange(w, h.devices.ConfirmGateway())
}

func (h *NetauditDevicesHandlers) respondChange(w http.ResponseWriter, err error) {
	if err != nil {
		writeJSONError(w, deviceStatus(err), err.Error())
		return
	}
	h.List(w, nil)
}

func deviceStatus(err error) int {
	switch {
	case errors.Is(err, netaudit.ErrBadMAC), errors.Is(err, netaudit.ErrBadDeviceName):
		return http.StatusBadRequest
	case errors.Is(err, netaudit.ErrNoSuchDevice):
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// presenceResponse is the settings with what the check last did, so a check that cannot look says so.
type presenceResponse struct {
	netaudit.PresenceView
	netaudit.PresenceStatus
}

// GetPresence returns the presence check's settings and status.
func (h *NetauditDevicesHandlers) GetPresence(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, presenceResponse{h.presence.View(), h.presence.Status()})
}

// presenceRequest turns the check on or off and chooses how often it runs (0 is the default).
type presenceRequest struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"`
}

// PutPresence saves the presence check's settings; an interval that is not one of the choices is a 400.
func (h *NetauditDevicesHandlers) PutPresence(w http.ResponseWriter, r *http.Request) {
	var req presenceRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := h.presence.Set(req.Enabled, req.IntervalMinutes); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, netaudit.ErrBadPresenceInterval) {
			status = http.StatusBadRequest
		}
		writeJSONError(w, status, err.Error())
		return
	}
	h.GetPresence(w, nil)
}
