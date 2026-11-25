package brief

import (
	"context"
	"errors"
	"slices"
	"time"

	"llm-proxy-stacks/sdk"
)

// Summary is the short message of an issue for a connector (ADR-0083): its name, its top stories as a headline and a link, and the home facts by id and
// count. It carries no sentence of the stack's own, no reminder and no section text: the core words it, and re-words each home fact from its own table.
func Summary(is Issue) stacksdk.BriefNoticeRequest {
	req := stacksdk.BriefNoticeRequest{Name: is.Name}
	stories := is.Top
	if len(stories) == 0 {
		for _, sec := range is.Sections {
			stories = append(stories, sec.Stories...)
		}
	}
	for _, s := range stories[:min(len(stories), stacksdk.MaxBriefNoticeStories)] {
		req.Stories = append(req.Stories, stacksdk.NoticeItem{Title: s.Headline, Link: s.Link})
	}
	for _, h := range is.Home[:min(len(is.Home), stacksdk.MaxHomeLines)] {
		req.Home = append(req.Home, stacksdk.BriefNoticeHome{ID: h.ID, Count: h.Count})
	}
	return req
}

// Unsent are the scheduled issues made since the cutoff whose summary has not gone out; a manual one (no ready time) is never among them.
func (s *Store) Unsent(since time.Time) []Issue {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Issue
	for _, is := range s.doc.Issues {
		if !is.ReadyFor.IsZero() && is.SentAt.IsZero() && is.SendState != SendNothing && !is.MadeAt.Before(since) {
			out = append(out, is)
		}
	}
	return out
}

// The fixed words for why a summary has not gone out; the Briefs page words each itself.
const (
	SendRefused = "refused"
	SendFailed  = "failed"
	SendGaveUp  = "gave_up"
	// SendNothing marks an issue with nothing to say: no send was made, so SentAt stays empty and the page does not claim one.
	SendNothing = "nothing"
)

// setSendState records why a summary has not gone out, and writes only when it changes: Deliver runs on every pass, and a send that keeps failing must not
// rewrite the file each time.
func (s *Store) setSendState(id int64, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.doc.Issues, func(is Issue) bool { return is.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	if s.doc.Issues[i].SendState == state {
		return nil
	}
	s.doc.Issues[i].SendState = state
	return s.saveLocked()
}

// expireSends marks the issues made before the cutoff whose send kept failing as given up, so the page stops saying "trying again". A refusal is left as it is:
// it is the owner's connector setting, and the fix it names stays useful however late the brief is read.
func (s *Store) expireSends(before time.Time) {
	var ids []int64
	s.mu.Lock()
	for _, is := range s.doc.Issues {
		if is.SendState == SendFailed && is.MadeAt.Before(before) {
			ids = append(ids, is.ID)
		}
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.setSendState(id, SendGaveUp)
	}
}

// sendStateOf is the state an error leaves: a refusal is the owner's connector setting, anything else is a failure to try again.
func sendStateOf(err error) string {
	if stacksdk.CodeOf(err) == stacksdk.CodeInvalid {
		return SendRefused
	}
	return SendFailed
}

// MarkSent notes that the summary of the issue went out, so it is not sent again. Marking it again keeps the first time.
func (s *Store) MarkSent(id int64, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.doc.Issues, func(is Issue) bool { return is.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	if !s.doc.Issues[i].SentAt.IsZero() {
		return nil
	}
	s.doc.Issues[i].SentAt = now.UTC()
	s.doc.Issues[i].SendState = ""
	return s.saveLocked()
}

// Sent is what one pass of Deliver did for one issue, for the caller to log.
type Sent struct {
	IssueID int64
	// Err is why it could not go this time, or nil when it did; an issue that failed is tried again on the next pass until PublishCatchUp is over.
	Err error
}

// Deliver sends the summary of every scheduled issue that is not yet sent, for an edition that names a connector. A manual issue ("write a new one now")
// is never sent: the owner is looking at it. An issue is marked sent only after the connector took it, so a refused or failed send is tried again on the
// next pass for PublishCatchUp, then given up. An issue with nothing to say (no story, no home fact) is marked "nothing" (not as sent) and sends nothing.
func (p *Publisher) Deliver(ctx context.Context) []Sent {
	if p.Send == nil {
		return nil
	}
	now := p.now()
	editions := p.Store.Editions()
	var out []Sent
	p.Store.expireSends(now.Add(-PublishCatchUp))
	for _, is := range p.Store.Unsent(now.Add(-PublishCatchUp)) {
		if ctx.Err() != nil {
			break
		}
		i := slices.IndexFunc(editions, func(e Edition) bool { return e.ID == is.EditionID })
		if i < 0 || editions[i].SendTo == "" {
			continue
		}
		req := Summary(is)
		if len(req.Stories) == 0 && len(req.Home) == 0 {
			_ = p.Store.setSendState(is.ID, SendNothing)
			continue
		}
		err := p.Send(ctx, editions[i].SendTo, req)
		if err == nil {
			err = p.Store.MarkSent(is.ID, now)
		} else if serr := p.Store.setSendState(is.ID, sendStateOf(err)); serr != nil {
			err = errors.Join(err, serr)
		}
		out = append(out, Sent{IssueID: is.ID, Err: err})
	}
	return out
}
