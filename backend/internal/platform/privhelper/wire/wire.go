// Package wire is the privileged helper's wire protocol: one request and one response per connection, each a 4-byte big-endian length
// followed by a JSON document (design network-privileged-helper-design.md section 6). Every field is plain data (Constitution IV.6).
// A frame is refused when its length is over the cap, its JSON has a field the type does not know, anything follows the document, or the
// version is not ours; the length is checked before a byte of the body is read, so a hostile peer cannot make the reader allocate.
package wire

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	// Version is the only protocol version this code speaks.
	Version = 1
	// MaxRequestBytes bounds a request frame. A request is an action name and a few typed parameters.
	MaxRequestBytes = 4096
	// MaxResponseBytes is the largest response frame any action may produce; an action's own cap is lower or equal.
	MaxResponseBytes = 1 << 20

	headerBytes = 4
	idBytes     = 16
)

// Typed failures; match them with errors.Is.
var (
	// ErrTooLarge means a frame's declared length is over the cap or zero.
	ErrTooLarge = errors.New("frame length is over the cap")
	// ErrMalformed means a frame is not exactly one JSON document of the expected type.
	ErrMalformed = errors.New("frame is not a valid document")
	// ErrVersion means the peer speaks another protocol version.
	ErrVersion = errors.New("unsupported protocol version")
)

// Status is how a request ended.
type Status string

// The statuses.
const (
	// StatusOK: the action ran and Result holds what it produced.
	StatusOK Status = "ok"
	// StatusRefused: a rule said no; Code says which.
	StatusRefused Status = "refused"
	// StatusError: the action was allowed and failed.
	StatusError Status = "error"
)

// Code names why a request was refused or failed. It is the whole explanation a caller gets from the privileged side.
type Code string

// The codes. A refusal never says more than its code and one fixed sentence.
const (
	CodeScope    Code = "scope"    // a target or parameter is outside what the action may touch
	CodeCaps     Code = "caps"     // a time, size or rate cap was reached or exceeded
	CodeDisabled Code = "disabled" // the action's switch or unit is off
	CodeCaller   Code = "caller"   // the peer is not the service's main process
	CodeBusy     Code = "busy"     // the helper is at its limit
	CodeUnknown  Code = "unknown"  // the action is not in the catalogue
	CodeInvalid  Code = "invalid"  // the request or its parameters are malformed
	CodeFailed   Code = "failed"   // the action ran and failed
)

// Request asks the helper to run one catalogue action. Params is decoded strictly by the catalogue, not here.
type Request struct {
	V      int             `json:"v"`
	ID     string          `json:"id"`
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

// Result is what an action produced: raw bounded bytes. The helper never parses them (design section 7).
type Result struct {
	Kind      string `json:"kind"`
	Data      []byte `json:"data"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Response answers one Request, carrying its ID.
type Response struct {
	V       int     `json:"v"`
	ID      string  `json:"id"`
	Status  Status  `json:"status"`
	Result  *Result `json:"result,omitempty"`
	Code    Code    `json:"code,omitempty"`
	Message string  `json:"message,omitempty"`
}

// NewID returns a random request ID.
func NewID() (string, error) {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("request id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// DecodeStrict decodes data into dst, refusing a field dst does not have and anything after the document.
func DecodeStrict(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if dec.More() {
		return fmt.Errorf("%w: data after the document", ErrMalformed)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: data after the document", ErrMalformed)
	}
	return nil
}

// WriteFrame writes v as one frame of at most max bytes.
func WriteFrame(w io.Writer, v any, max int) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}
	if len(body) == 0 || len(body) > max {
		return ErrTooLarge
	}
	frame := make([]byte, headerBytes, headerBytes+len(body))
	binary.BigEndian.PutUint32(frame, uint32(len(body)))
	if _, err := w.Write(append(frame, body...)); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	return nil
}

// ReadFrame reads one frame of at most max bytes into dst. The declared length is checked first, and exactly that many bytes are read.
func ReadFrame(r io.Reader, max int, dst any) error {
	var header [headerBytes]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return fmt.Errorf("read frame header: %w", err)
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 || uint64(n) > uint64(max) {
		return ErrTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return fmt.Errorf("read frame body: %w", err)
	}
	return DecodeStrict(body, dst)
}

// ReadRequest reads and checks one request.
func ReadRequest(r io.Reader) (Request, error) {
	var req Request
	if err := ReadFrame(r, MaxRequestBytes, &req); err != nil {
		return Request{}, err
	}
	if req.V != Version {
		return Request{}, fmt.Errorf("%w: %d", ErrVersion, req.V)
	}
	if req.ID == "" || req.Action == "" {
		return Request{}, fmt.Errorf("%w: id and action are required", ErrMalformed)
	}
	return req, nil
}

// ReadResponse reads and checks one response to the request with the given ID.
func ReadResponse(r io.Reader, id string) (Response, error) {
	var resp Response
	if err := ReadFrame(r, MaxResponseBytes, &resp); err != nil {
		return Response{}, err
	}
	switch {
	case resp.V != Version:
		return Response{}, fmt.Errorf("%w: %d", ErrVersion, resp.V)
	case resp.ID != id:
		return Response{}, fmt.Errorf("%w: answer to another request", ErrMalformed)
	case resp.Status != StatusOK && resp.Status != StatusRefused && resp.Status != StatusError:
		return Response{}, fmt.Errorf("%w: unknown status", ErrMalformed)
	}
	return resp, nil
}
