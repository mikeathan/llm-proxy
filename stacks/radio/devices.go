package radio

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"llm-proxy-stacks/sdk"
)

// The owner's own devices (plan §4): the operator names an address as theirs, and the stack keeps it sealed with the label so it can be matched
// again and told apart from everyone else's. The address is never shown again (a list gives an id, the label and the days), and a neighbour's address is
// never accepted here or stored anywhere.
const (
	// DevicesPath lists (GET) and adds (POST) the owner's devices.
	DevicesPath = "/admin/api/radio/devices/mine"
	// DevicePath removes one (DELETE).
	DevicePath = "/admin/api/radio/devices/mine/{id}"
	// ForgetPath makes the stack forget everything about other people's devices and rotates the tags (POST).
	ForgetPath = "/admin/api/radio/forget"

	maxLabel = 60
)

// OwnerDevice is one of the owner's devices as shown: no address.
type OwnerDevice struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Visits   int    `json:"visits"`
	FirstDay string `json:"first_seen"`
	LastDay  string `json:"last_seen"`
}

type ownerBody struct {
	Address string `json:"address"`
	Label   string `json:"label"`
}

// ParseMAC reads the usual forms (colons or dashes, any case). A group address is not a device and is refused.
func ParseMAC(s string) (MAC, error) {
	var m MAC
	s = strings.ReplaceAll(strings.TrimSpace(s), "-", ":")
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return m, errors.New("radio: an address is six pairs of digits")
	}
	for i, p := range parts {
		b, err := hex.DecodeString(p)
		if err != nil || len(b) != 1 || len(p) != 2 {
			return m, errors.New("radio: an address is six pairs of digits")
		}
		m[i] = b[0]
	}
	if m[0]&1 != 0 || m == (MAC{}) {
		return m, errors.New("radio: that is a group address, not a device")
	}
	return m, nil
}

func (s *Stack) handleDevices(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	h, started := s.snapshot()
	if !started {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the radio stack is not running")
	}
	rec, store, err := s.recorder(h)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the radio database could not be opened")
	}
	switch {
	case req.Path == DevicesPath && req.Method == http.MethodGet:
		return s.listOwners(ctx, store)
	case req.Path == DevicesPath && req.Method == http.MethodPost:
		return s.addOwner(ctx, rec, req.Body)
	case req.Path == DevicePath && req.Method == http.MethodDelete:
		return s.removeOwner(ctx, store, req.Params["id"])
	case req.Path == ForgetPath && req.Method == http.MethodPost:
		return s.forget(ctx, rec)
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}

func (s *Stack) listOwners(ctx context.Context, store *Store) (stacksdk.Response, error) {
	rows, err := store.OwnerDevices(ctx)
	if err != nil {
		return plainError(http.StatusInternalServerError, "the devices could not be read")
	}
	out := make([]OwnerDevice, 0, len(rows))
	for _, d := range rows {
		out = append(out, OwnerDevice{ID: hex.EncodeToString(d.Tag), Label: d.Label, Visits: d.Visits, FirstDay: dayString(d.FirstDay), LastDay: dayString(d.LastDay)})
	}
	return jsonResponse(http.StatusOK, map[string]any{"devices": out})
}

func dayString(d int64) string { return dayTime(d).Format(time.DateOnly) }

func (s *Stack) addOwner(ctx context.Context, rec *Recorder, body []byte) (stacksdk.Response, error) {
	var b ownerBody
	if err := strictDecode(body, &b); err != nil {
		return plainError(http.StatusBadRequest, "give the device's address and a name for it")
	}
	label := strings.TrimSpace(b.Label)
	mac, err := ParseMAC(b.Address)
	switch {
	case err != nil:
		return plainError(http.StatusBadRequest, err.Error())
	case label == "" || utf8.RuneCountInString(label) > maxLabel || strings.ContainsFunc(label, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return plainError(http.StatusBadRequest, "give the device a name of up to 60 characters")
	}
	if err := rec.AddOwnerDevice(ctx, mac, label, s.now()); err != nil {
		return plainError(http.StatusInternalServerError, "the device could not be saved")
	}
	return jsonResponse(http.StatusCreated, map[string]string{"label": label})
}

func (s *Stack) removeOwner(ctx context.Context, store *Store, id string) (stacksdk.Response, error) {
	tag, err := hex.DecodeString(id)
	if err != nil || len(tag) == 0 {
		return plainError(http.StatusNotFound, "no such device")
	}
	ok, err := store.DeleteOwner(ctx, tag)
	switch {
	case err != nil:
		return plainError(http.StatusInternalServerError, "the device could not be removed")
	case !ok:
		return plainError(http.StatusNotFound, "no such device")
	}
	return jsonResponse(http.StatusOK, map[string]bool{"removed": true})
}

func (s *Stack) forget(ctx context.Context, rec *Recorder) (stacksdk.Response, error) {
	s.mu.Lock()
	listening := s.run != nil && s.run.snapshot().Listening
	s.mu.Unlock()
	if listening {
		return plainError(http.StatusConflict, "stop listening first: a run is still recording")
	}
	if err := rec.ForgetNeighbours(ctx); err != nil {
		return plainError(http.StatusInternalServerError, "the neighbours could not be forgotten")
	}
	return jsonResponse(http.StatusOK, map[string]bool{"forgotten": true})
}
