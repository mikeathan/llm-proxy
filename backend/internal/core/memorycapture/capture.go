package memorycapture

import "context"

// Result is what Capture did for one message.
type Result struct {
	// Saved holds the text of each fact that was created or updated; an already-saved fact is not reported.
	Saved []string
	// Failed counts facts the store refused. The caller logs it; capture never fails the run it belongs to.
	Failed int
}

// Capture saves the explicit requests in message. Nil collaborators make it inert, so a caller without memory needs no check.
func Capture(ctx context.Context, ex *Extractor, lib Library, workspaceID, message string) Result {
	var res Result
	if ex == nil || lib == nil {
		return res
	}
	for _, c := range ex.Extract(message) {
		outcome, err := lib.Save(ctx, workspaceID, c)
		switch {
		case err != nil:
			res.Failed++
		case outcome != OutcomeDuplicate:
			res.Saved = append(res.Saved, c.Content)
		}
	}
	return res
}
