package sources

import (
	"context"
	"fmt"

	"llm-proxy-stacks/sdk"
)

// get makes one guarded GET through the SDK port and wraps a failure with the source's name; the port's code and fixed sentence stay reachable
// with errors.As. A body cut at the byte bound is refused: half a feed or half a JSON document would be read as a smaller, valid-looking one.
func get(ctx context.Context, f stacksdk.Fetch, name, target, accept string) ([]byte, error) {
	resp, err := f.Get(ctx, stacksdk.FetchRequest{URL: target, Accept: accept, MaxBytes: maxBody})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if resp.Truncated {
		return nil, fmt.Errorf("%s: the answer was longer than %d bytes: %w", name, maxBody, ErrShape)
	}
	return resp.Body, nil
}
