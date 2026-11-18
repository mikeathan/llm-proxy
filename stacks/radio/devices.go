package radio

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"slices"
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
	// MarkMinePath makes a device heard on the radar the owner's (POST), by its short id, and renames one that is already theirs (PUT).
	MarkMinePath = "/admin/api/radio/air/mine/{id}"

	maxLabel = 60
)

// OwnerDevice is one of the owner's devices as shown: no address.
type OwnerDevice struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Visits   int    `json:"visits"`
	FirstDay string `json:"first_seen"`
	LastDay  string `json:"last_seen"`
	// Class is what kind of device it is; ClassBy says who decided: "owner" (the operator's word) or "rules" (the classifier, with Why).
	Class   string `json:"class"`
	ClassBy string `json:"class_by"`
	Why     string `json:"why,omitempty"`
}

type ownerBody struct {
	Address string `json:"address"`
	Label   string `json:"label"`
	Class   string `json:"class,omitempty"`
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
	routes := map[string]func() (stacksdk.Response, error){
		http.MethodGet + " " + DevicesPath:   func() (stacksdk.Response, error) { return s.listOwners(ctx, store) },
		http.MethodPost + " " + DevicesPath:  func() (stacksdk.Response, error) { return s.addOwner(ctx, rec, req.Body) },
		http.MethodGet + " " + PresencePath:  func() (stacksdk.Response, error) { return s.presence(ctx, store) },
		http.MethodDelete + " " + DevicePath: func() (stacksdk.Response, error) { return s.removeOwner(ctx, store, req.Params["id"]) },
		http.MethodPost + " " + ForgetPath:   func() (stacksdk.Response, error) { return s.forget(ctx, rec) },
	}
	if f, ok := routes[req.Method+" "+req.Path]; ok {
		return f()
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
		by := "rules"
		if d.Class != "" && d.ClassVer == 0 {
			by = "owner"
		}
		out = append(out, OwnerDevice{ID: hex.EncodeToString(d.Tag), Label: d.Label, Visits: d.Visits, FirstDay: dayString(d.FirstDay), LastDay: dayString(d.LastDay),
			Class: string(orUnknown(d.Class)), ClassBy: by})
	}
	return jsonResponse(http.StatusOK, map[string]any{"devices": out})
}

func dayString(d int64) string { return dayTime(d).Format(time.DateOnly) }

func (s *Stack) addOwner(ctx context.Context, rec *Recorder, body []byte) (stacksdk.Response, error) {
	var b ownerBody
	if err := strictDecode(body, &b); err != nil {
		return plainError(http.StatusBadRequest, "give the device's address and a name for it")
	}
	mac, err := ParseMAC(b.Address)
	if err != nil {
		return plainError(http.StatusBadRequest, err.Error())
	}
	label, class, bad := ownerFields(b.Label, b.Class)
	if bad != "" {
		return plainError(http.StatusBadRequest, bad)
	}
	if err := rec.AddOwnerDeviceClass(ctx, mac, label, class, s.now()); err != nil {
		return plainError(http.StatusInternalServerError, "the device could not be saved")
	}
	return jsonResponse(http.StatusCreated, map[string]string{"label": label})
}

// ownerFields checks the name and the optional class an owner gives a device. It returns the sentence to answer with when one is wrong.
func ownerFields(rawLabel, rawClass string) (label string, class Class, bad string) {
	label = strings.TrimSpace(rawLabel)
	if label == "" || utf8.RuneCountInString(label) > maxLabel || strings.ContainsFunc(label, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", "", "give the device a name of up to 60 characters"
	}
	class = Class(rawClass)
	if class != "" && !slices.Contains(Classes, class) || class == ClassUnknown {
		return "", "", "the class is infrastructure, personal, neighbour or transient, or left out"
	}
	return label, class, ""
}

// markMine makes a device heard on the radar the owner's, by the short id the radar shows. Only the keyed tag is known for someone else's device, so
// there is no address to seal: the mark holds while the tag matches, which for a device with a random address is until it changes address, and
// it ends when the neighbours are forgotten (the rotation keeps only rows it can tag again from a sealed address).
func (s *Stack) markMine(ctx context.Context, rec *Recorder, store *Store, id string, body []byte) (stacksdk.Response, error) {
	var b struct {
		Label string `json:"label"`
		Class string `json:"class,omitempty"`
	}
	if err := strictDecode(body, &b); err != nil {
		return plainError(http.StatusBadRequest, "give the device a name")
	}
	label, class, bad := ownerFields(b.Label, b.Class)
	if bad != "" {
		return plainError(http.StatusBadRequest, bad)
	}
	tags, err := store.NeighbourTags(ctx, id)
	switch {
	case err != nil:
		return plainError(http.StatusInternalServerError, "the device could not be saved")
	case len(tags) != 1:
		return plainError(http.StatusNotFound, "that device is no longer known: it may have been forgotten or already be yours")
	}
	if err := store.MarkTagOwner(ctx, tags[0], label, string(class)); err != nil {
		return plainError(http.StatusInternalServerError, "the device could not be saved")
	}
	rec.ForgetSighting(tags[0])
	return jsonResponse(http.StatusCreated, map[string]string{"label": label})
}

// renameMine gives one of the owner's devices a new name, by the short id the page shows. A device that is not theirs is not found: marking it
// is a separate answer (markMine).
func (s *Stack) renameMine(ctx context.Context, store *Store, id string, body []byte) (stacksdk.Response, error) {
	var b struct {
		Label string `json:"label"`
	}
	if err := strictDecode(body, &b); err != nil {
		return plainError(http.StatusBadRequest, "give the device a name")
	}
	label, _, bad := ownerFields(b.Label, "")
	if bad != "" {
		return plainError(http.StatusBadRequest, bad)
	}
	tag, status, msg := uniqueTag(ctx, store, id)
	if tag == nil {
		return plainError(status, msg)
	}
	switch ok, err := store.RenameOwner(ctx, tag, label); {
	case err != nil:
		return plainError(http.StatusInternalServerError, "the name could not be saved")
	case !ok:
		return plainError(http.StatusNotFound, "that device is not yours yet: say it is yours first")
	}
	return jsonResponse(http.StatusOK, map[string]string{"label": label})
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

func orUnknown(c string) Class {
	if c == "" {
		return ClassUnknown
	}
	return Class(c)
}
