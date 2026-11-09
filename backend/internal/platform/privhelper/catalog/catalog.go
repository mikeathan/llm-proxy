// Package catalog is the closed list of what the privileged helper may do (ADR-0034, Constitution I.11). The list is Go data compiled into
// both the service and the helper; a stack, a manifest, a model or an operator cannot add a row. Each row has typed parameters that are
// decoded strictly, a scope check made from fixed rules in code, and a builder that makes the whole argument list itself: no byte of
// a caller's text becomes an argument except through a typed, checked value.
package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"llm-proxy/internal/platform/privhelper/wire"
	"llm-proxy/internal/platform/scanscope"
)

// SwitchKey is the setting that holds the actions the operator has turned on: a list of action IDs under sandboxing in settings.yml. It is
// changed only through PUT /admin/api/host, which needs the passphrase again (the second gate of Constitution I.11).
const SwitchKey = "sandboxing.helper_actions"

// Typed refusals; match them with errors.Is. Each maps to a wire code.
var (
	// ErrUnknown means the action is not in the catalogue.
	ErrUnknown = errors.New("no such action")
	// ErrInvalid means the parameters are malformed: an unknown field, a wrong type, a missing value.
	ErrInvalid = errors.New("invalid parameters")
	// ErrScope means a parameter is outside what the action may touch.
	ErrScope = errors.New("parameter is outside the action's scope")
)

// Class is the privilege class: which unit runs an action.
type Class string

// The classes (design section 5).
const (
	ClassNet Class = "net" // CAP_NET_RAW and packet sockets
	ClassLog Class = "log" // the adm group, no network (no action yet: deferred by D1)
)

// ResultKind is the shape of what an action returns.
type ResultKind string

// The result kinds. The helper returns bytes it never parses (design section 7).
const ResultBytes ResultKind = "bytes"

// Caps bound one run. The helper enforces them; the service cannot raise them.
type Caps struct {
	Timeout        time.Duration
	MaxOutputBytes int
	MaxPerHour     int
}

// Jail is the confinement the helper puts on the program an action starts (isolation profile, as plain data).
type Jail struct {
	// Network keeps the program's TCP network access; a scan needs it and a log read does not.
	Network bool
}

// Prepared is an action ready to run: the program, its complete argument list, and the canonical form of the checked parameters, which
// is what a grant is bound to.
type Prepared struct {
	Program   string
	Args      []string
	Canonical []byte
	// Target is the one address the action touches, for the helper's own scope check; "" when the action touches no host.
	Target string
}

// Action is one row of the catalogue.
type Action struct {
	ID     string
	Class  Class
	Switch string // the settings list the action's ID must be in (SwitchKey)
	Caps   Caps
	Jail   Jail
	Result ResultKind

	prepare func(raw json.RawMessage) (Prepared, error)
}

// Prepare decodes raw as this action's parameters strictly, checks them against the action's scope and builds the program and arguments.
func (a Action) Prepare(raw json.RawMessage) (Prepared, error) {
	return a.prepare(raw)
}

// Code is the wire code for a refusal from Prepare.
func Code(err error) wire.Code {
	switch {
	case errors.Is(err, ErrUnknown):
		return wire.CodeUnknown
	case errors.Is(err, ErrScope):
		return wire.CodeScope
	default:
		return wire.CodeInvalid
	}
}

// row builds an Action whose parameters are a P; Host is the one address the parameters name, which the helper checks against its own scope. check validates P against the action's scope and puts it in canonical form (so two
// spellings of one request have one canonical bytes); build turns the checked P into arguments and takes nothing else.
func row[P interface{ Host() string }](a Action, program string, check func(*P) error, build func(P) []string) Action {
	a.prepare = func(raw json.RawMessage) (Prepared, error) {
		var p P
		if len(raw) == 0 {
			return Prepared{}, fmt.Errorf("%w: parameters are required", ErrInvalid)
		}
		if err := wire.DecodeStrict(raw, &p); err != nil {
			return Prepared{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if err := check(&p); err != nil {
			return Prepared{}, err
		}
		canon, err := json.Marshal(p)
		if err != nil {
			return Prepared{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		return Prepared{Program: program, Args: build(p), Canonical: canon, Target: p.Host()}, nil
	}
	return a
}

// privateBlocks are the only addresses a target may be in: the fixed invariant of Constitution II.3 item 3. The operator's declared
// ranges are a second check, made by the service's port before it asks the helper; the helper keeps no copy of them.
var privateBlocks = scanscope.PolicyFromCIDRs([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})

// checkTarget requires one private IPv4 address, written as an address and nothing else, and puts it in canonical form.
func checkTarget(target *string) error {
	a, err := netip.ParseAddr(*target)
	if err != nil {
		return fmt.Errorf("%w: target is not an address", ErrInvalid)
	}
	if err := privateBlocks.Check(a); err != nil {
		return fmt.Errorf("%w: %v", ErrScope, err)
	}
	*target = a.Unmap().String()
	return nil
}

// All returns every action, in a fixed order.
func All() []Action {
	return []Action{scanUDP(), osDetect(), captureWindow(), captureHost()}
}

// Lookup returns the action with the given ID.
func Lookup(id string) (Action, error) {
	all := All()
	i := slices.IndexFunc(all, func(a Action) bool { return a.ID == id })
	if i < 0 {
		return Action{}, ErrUnknown
	}
	return all[i], nil
}
