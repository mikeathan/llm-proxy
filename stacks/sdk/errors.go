package stacksdk

import "errors"

// Code classifies an Error. It is what crosses the boundary; a Go error compared by identity does not.
type Code string

// The codes a port call or a tool call can fail with.
const (
	CodeInvalid     Code = "invalid"     // the request was malformed or out of range
	CodeNotFound    Code = "not_found"   // the named thing does not exist
	CodeDenied      Code = "denied"      // a guard or policy refused it
	CodeUnavailable Code = "unavailable" // the capability is off or not ready; retry later
	CodeTimeout     Code = "timeout"     // it ran past its time limit
	CodeInternal    Code = "internal"    // anything else
)

// Error is a failure as a code and a message, both plain text. Inside a stack, Go errors keep their wrapping (Constitution IV.2); at
// the boundary a stack returns an *Error.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// Is makes errors.Is(err, &Error{Code: c}) true for any Error with the same code, whatever its message.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// Errorf builds an Error from a code and a message.
func Errorf(code Code, msg string) *Error { return &Error{Code: code, Message: msg} }

// CodeOf returns the code of the first *Error in err's chain, or CodeInternal for any other non-nil error, or "" for nil.
func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}
