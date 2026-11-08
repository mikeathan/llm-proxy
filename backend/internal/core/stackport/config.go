package stackport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"llm-proxy-stacks/sdk"
)

// Fixed sentences a stack is told: never the decoder's own words, which can quote the document.
const (
	msgNotJSON  = "the settings document is not a JSON object"
	msgNoSave   = "the settings could not be saved"
	msgNoLoad   = "the settings could not be read"
	msgNotStore = "the settings document does not fit the settings"
)

// Store is a typed settings document the core keeps (for Network, models.NetauditConfig behind the storage view). Update applies fn to a
// copy and saves it; an error from fn saves nothing.
type Store[T any] interface {
	Get() T
	Update(fn func(*T) error) error
}

// Config is the SDK's Config port over a typed settings document. The document crosses as JSON using the type's own json tags, which
// carry no omitempty, so Load shows every field.
//
// Save merges: a field the document names replaces the stored one, a field it leaves out is kept. A stack that sends only its own
// fields therefore cannot clobber the others, and a document that does not fit the type saves nothing.
type Config[T any] struct {
	Store Store[T]
}

// Load returns the whole document as JSON.
func (c Config[T]) Load(context.Context) (json.RawMessage, error) {
	doc, err := json.Marshal(c.Store.Get())
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, msgNoLoad)
	}
	return doc, nil
}

// Save merges doc over the stored document and saves the result.
func (c Config[T]) Save(_ context.Context, doc json.RawMessage) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(doc, &probe); err != nil || probe == nil {
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgNotJSON)
	}
	err := c.Store.Update(func(cur *T) error {
		merged, err := merge(*cur, probe)
		if err != nil {
			return fmt.Errorf("%s: %w", msgNotStore, errInvalid)
		}
		*cur = merged
		return nil
	})
	switch {
	case err == nil:
		return nil
	case isInvalid(err):
		return stacksdk.Errorf(stacksdk.CodeInvalid, msgNotStore)
	}
	return stacksdk.Errorf(stacksdk.CodeInternal, msgNoSave)
}

// errInvalid marks a document that does not fit the type, so Save can tell it from a store that failed.
var errInvalid = errors.New("invalid settings document")

func isInvalid(err error) bool { return errors.Is(err, errInvalid) }

// merge overlays the named fields of a document on the current settings and decodes the result into a fresh value. It never decodes into
// the current value: a shallow copy of the settings shares its slices with the stored ones, and decoding into them would rewrite memory
// that other goroutines are reading.
func merge[T any](cur T, over map[string]json.RawMessage) (T, error) {
	var zero T
	raw, err := json.Marshal(cur)
	if err != nil {
		return zero, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return zero, err
	}
	for k, v := range over {
		fields[k] = v
	}
	joined, err := json.Marshal(fields)
	if err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal(joined, &out); err != nil {
		return zero, err
	}
	return out, nil
}
