package nodeherder

import (
	"errors"
	"fmt"
)

var ErrAuthExpired = &DomainError{
	Err:    errors.New("nodeherder auth expired"),
	Status: 401,
	Msg:    "nodeherder authentication expired",
}

var ErrQueryFailed = &DomainError{
	Err:    errors.New("nodeherder query failed"),
	Status: 502,
	Msg:    "nodeherder query failed",
}

type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("nodeherder http %d: %s", e.Status, e.Body)
}

type DomainError struct {
	Err    error
	Status int
	Msg    string
}

func (e *DomainError) Error() string { return e.Msg }
