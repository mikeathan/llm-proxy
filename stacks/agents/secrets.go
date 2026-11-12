package agents

import (
	"context"
	"fmt"
	"time"

	"llm-proxy-stacks/sdk"
)

// scanSecrets asks the core to look through the files agents wrote and records each place not reported before, by place and kind only. One
// alert per workspace names how many places were new. The core walks the folders and keeps the bounds; no file's bytes cross the port.
func (w *watch) scanSecrets(ctx context.Context) {
	if w.settings(ctx).SecretsPassOff || w.scan == nil {
		return
	}
	hits, err := w.scan.Scan(ctx)
	if err != nil {
		w.report(fmt.Errorf("failed to look through the workspaces for the secrets pass: %w", err))
		return
	}
	now := w.now()
	w.lastRun.Store(&now)
	for _, group := range byWorkspace(hits) {
		if ctx.Err() != nil {
			return
		}
		w.recordSecrets(ctx, group[0].Workspace, group)
	}
}

// byWorkspace groups hits by workspace, in the order each workspace first appears.
func byWorkspace(hits []stacksdk.SecretHit) [][]stacksdk.SecretHit {
	var order []string
	groups := map[string][]stacksdk.SecretHit{}
	for _, h := range hits {
		if _, seen := groups[h.Workspace]; !seen {
			order = append(order, h.Workspace)
		}
		groups[h.Workspace] = append(groups[h.Workspace], h)
	}
	out := make([][]stacksdk.SecretHit, 0, len(order))
	for _, id := range order {
		out = append(out, groups[id])
	}
	return out
}

// forgetOldest removes the oldest entries until at most keep are left, so a long-lived record keeps reporting new places (an old place found
// again is then reported again, which is the lesser fault).
func forgetOldest(seen map[string]time.Time, keep int) {
	for len(seen) > keep {
		oldest, at := "", time.Time{}
		for k, t := range seen {
			if oldest == "" || t.Before(at) {
				oldest, at = k, t
			}
		}
		delete(seen, oldest)
	}
}

// recordSecrets keeps the places not reported before and alerts once for them.
func (w *watch) recordSecrets(ctx context.Context, workspace string, found []stacksdk.SecretHit) {
	w.update(ctx, func(r *Record, now time.Time) *stacksdk.Alert {
		if r.SecretsSeen == nil {
			r.SecretsSeen = map[string]time.Time{}
		}
		fresh := 0
		for _, f := range found {
			key := workspace + "/" + f.Where + "|" + f.Kind
			if _, seen := r.SecretsSeen[key]; seen {
				continue
			}
			forgetOldest(r.SecretsSeen, maxSeenSecrets-1)
			r.SecretsSeen[key] = now
			addEvent(r, Event{At: now, Kind: KindSecretFound, Workspace: workspace, Where: f.Where, Detail: f.Kind})
			fresh++
		}
		if fresh == 0 {
			return nil
		}
		return &stacksdk.Alert{ID: AlertSecrets, Workspace: workspace, Count: fresh}
	})
}
