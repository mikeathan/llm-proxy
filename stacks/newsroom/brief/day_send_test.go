package brief

import (
	"context"
	"errors"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

type sendRig struct {
	sent []stacksdk.BriefNoticeRequest
	to   []string
	err  error
}

func (s *sendRig) send(_ context.Context, channel string, req stacksdk.BriefNoticeRequest) error {
	if s.err != nil {
		return s.err
	}
	s.sent, s.to = append(s.sent, req), append(s.to, channel)
	return nil
}

// "Your day" is stored on the issue only when the edition asks; a morning edition reads today and an evening one tomorrow; and the model is never shown a title.
func TestPublisher_DayBlockIsStoredAndNeverShownToTheModel(t *testing.T) {
	r, p, m := publishRig(t, func(e *Edition) { e.Day = true })
	var asked []bool
	p.Day = func(_ context.Context, tomorrow bool) ([]stacksdk.DayItem, error) {
		asked = append(asked, tomorrow)
		return []stacksdk.DayItem{{Clock: "14:00", Title: "SECRET-REMINDER-TITLE"}}, nil
	}
	r.at(6, 45)
	is, err := p.MakeNow(context.Background(), r.store.Editions()[0].ID)
	if err != nil || is.Day == nil || len(is.Day.Items) != 1 || is.Day.Items[0].Title != "SECRET-REMINDER-TITLE" || is.Day.Tomorrow || is.Day.Unavailable {
		t.Fatalf("day block: %+v, %v", is.Day, err)
	}
	if len(m.prompts) == 0 {
		t.Fatal("the model was never asked, so this proves nothing")
	}
	for _, prompt := range m.prompts {
		if strings.Contains(prompt, "SECRET-REMINDER-TITLE") || strings.Contains(prompt, "14:00") {
			t.Errorf("the model saw a reminder: %q", prompt)
		}
	}
	if len(asked) != 1 || asked[0] {
		t.Errorf("a 07:00 edition asks about today: %v", asked)
	}
	r2, p2, _ := publishRig(t, nil)
	p2.Day = func(context.Context, bool) ([]stacksdk.DayItem, error) {
		return []stacksdk.DayItem{{Clock: "1", Title: "x"}}, nil
	}
	r2.at(6, 45)
	if is, _ := p2.MakeNow(context.Background(), r2.store.Editions()[0].ID); is.Day != nil {
		t.Errorf("an edition that did not ask has no block: %+v", is.Day)
	}
}

func TestEdition_AnEveningBriefIsAboutTomorrow(t *testing.T) {
	for ready, want := range map[string]bool{"07:00": false, "11:59": false, "12:00": true, "20:00": true} {
		if got := (Edition{ReadyBy: ready}).ShowsTomorrow(); got != want {
			t.Errorf("%s: %v", ready, got)
		}
	}
}

// Reminders being off is said as such: it is not an empty day.
func TestPublisher_DayThatCannotBeReadIsMarkedUnavailable(t *testing.T) {
	r, p, _ := publishRig(t, func(e *Edition) { e.Day = true })
	p.Day = func(context.Context, bool) ([]stacksdk.DayItem, error) {
		return nil, stacksdk.Errorf(stacksdk.CodeUnavailable, "Reminders is off or could not be read")
	}
	r.at(6, 45)
	is, err := p.MakeNow(context.Background(), r.store.Editions()[0].ID)
	if err != nil || is.Day == nil || !is.Day.Unavailable || len(is.Day.Items) != 0 {
		t.Fatalf("%+v, %v", is.Day, err)
	}
	// A quiet day is a block with no items and is not unavailable.
	p.Day = func(context.Context, bool) ([]stacksdk.DayItem, error) { return nil, nil }
	if is, _ := p.MakeNow(context.Background(), r.store.Editions()[0].ID); is.Day == nil || is.Day.Unavailable || is.Day.Items == nil {
		t.Errorf("quiet day: %+v", is.Day)
	}
}

func TestEdition_ADayOnlyBriefNeedsNoNewsSection(t *testing.T) {
	e := Edition{Name: "Day", ReadyBy: "07:00", Day: true}
	if _, err := e.Normalize(func(int64) bool { return false }); err != nil {
		t.Errorf("a day-only brief: %v", err)
	}
	if _, err := (Edition{Name: "x", ReadyBy: "07:00", SendTo: "Tele gram", Day: true}).Normalize(func(int64) bool { return false }); !errors.Is(err, ErrInvalid) {
		t.Errorf("send to is a connector type: %v", err)
	}
}

func sendRigFor(t *testing.T, sendTo string) (*rig, *Publisher, *sendRig) {
	r, p, _ := publishRig(t, func(e *Edition) { e.Home = true; e.Day = true; e.SendTo = sendTo })
	s := &sendRig{}
	p.Send = s.send
	p.Home = func(context.Context) []stacksdk.HomeLine {
		return []stacksdk.HomeLine{{ID: "network_online", Count: 16, Words: "STACK-STORED-SENTENCE"}}
	}
	p.Day = func(context.Context, bool) ([]stacksdk.DayItem, error) {
		return []stacksdk.DayItem{{Clock: "14:00", Title: "SECRET-REMINDER-TITLE"}}, nil
	}
	return r, p, s
}

// The summary goes out once for each scheduled issue, only to the connector the edition names, and carries the home facts as id and count only, no reminder.
func TestPublisher_DeliverSendsOnceAndNeverCarriesWordsOrReminders(t *testing.T) {
	r, p, s := sendRigFor(t, "telegram")
	r.at(6, 45)
	tick(t, p)
	if got := p.Deliver(context.Background()); len(got) != 1 || got[0].Err != nil {
		t.Fatalf("deliver: %+v", got)
	}
	if len(s.sent) != 1 || s.to[0] != "telegram" || s.sent[0].Name == "" || len(s.sent[0].Stories) == 0 {
		t.Fatalf("sent: %+v to %v", s.sent, s.to)
	}
	if got := s.sent[0].Home; len(got) != 1 || got[0] != (stacksdk.BriefNoticeHome{ID: "network_online", Count: 16}) {
		t.Errorf("home facts are id and count: %+v", got)
	}
	// The request type has no room for a sentence or a reminder; a copy of everything in it shows neither.
	if all := strings.Join(flatten(s.sent[0]), "|"); strings.Contains(all, "STACK-STORED-SENTENCE") || strings.Contains(all, "SECRET-REMINDER-TITLE") {
		t.Errorf("a stored sentence or a reminder reached the summary: %s", all)
	}
	if again := p.Deliver(context.Background()); len(again) != 0 || len(s.sent) != 1 {
		t.Errorf("an issue is told once: %+v, %d sent", again, len(s.sent))
	}
	is := r.store.Issues(r.store.Editions()[0].ID)[0]
	if is.SentAt.IsZero() {
		t.Error("the issue records when it went out")
	}
}

func flatten(r stacksdk.BriefNoticeRequest) []string {
	out := []string{r.Name}
	for _, s := range r.Stories {
		out = append(out, s.Title, s.Link)
	}
	for _, h := range r.Home {
		out = append(out, h.ID)
	}
	return out
}

func TestPublisher_DeliverTriesAgainAfterAFailureAndNotAtAllWithoutAChannelOrForAManualIssue(t *testing.T) {
	r, p, s := sendRigFor(t, "telegram")
	r.at(6, 45)
	tick(t, p)
	s.err = stacksdk.Errorf(stacksdk.CodeUnavailable, "down")
	if got := p.Deliver(context.Background()); len(got) != 1 || got[0].Err == nil || len(s.sent) != 0 {
		t.Fatalf("a failed send is reported: %+v", got)
	}
	if is := r.store.Issues(r.store.Editions()[0].ID)[0]; !is.SentAt.IsZero() {
		t.Error("it is marked sent only after the connector took it")
	}
	s.err = nil
	r.at(6, 50)
	if got := p.Deliver(context.Background()); len(got) != 1 || got[0].Err != nil || len(s.sent) != 1 {
		t.Fatalf("the next pass sends it: %+v", got)
	}

	r2, p2, s2 := sendRigFor(t, "")
	r2.at(6, 45)
	tick(t, p2)
	if got := p2.Deliver(context.Background()); len(got) != 0 || len(s2.sent) != 0 {
		t.Errorf("an edition with no connector sends nothing: %+v", got)
	}

	r3, p3, s3 := sendRigFor(t, "telegram")
	r3.at(6, 45)
	if _, err := p3.MakeNow(context.Background(), r3.store.Editions()[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := p3.Deliver(context.Background()); len(got) != 0 || len(s3.sent) != 0 {
		t.Errorf("an issue the owner asked for by hand is not pushed: %+v", got)
	}
}

func TestPublisher_DeliverGivesUpAfterTheCatchUpAndSkipsAnIssueWithNothingToSay(t *testing.T) {
	r, p, s := sendRigFor(t, "telegram")
	s.err = errors.New("down")
	r.at(6, 45)
	tick(t, p)
	p.Deliver(context.Background())
	s.err = nil
	r.at(11, 0) // more than PublishCatchUp after it was made
	if got := p.Deliver(context.Background()); len(got) != 0 || len(s.sent) != 0 {
		t.Errorf("an old issue is not pushed: %+v", got)
	}

	// An issue with nothing to say (no story, no home fact) sends nothing and is not retried.
	r2, p2, s2 := sendRigFor(t, "telegram")
	r2.at(6, 45)
	editionID := r2.store.Editions()[0].ID
	saved, err := r2.store.SaveIssue(Issue{EditionID: editionID, Name: "Morning", MadeAt: r2.now, ReadyFor: r2.now, Sections: []IssueSection{}}, r2.now)
	if err != nil {
		t.Fatal(err)
	}
	if got := p2.Deliver(context.Background()); len(got) != 0 || len(s2.sent) != 0 {
		t.Errorf("an empty issue sends nothing: %+v", got)
	}
	if is, _ := r2.store.Issue(saved.ID); is.SentAt.IsZero() {
		t.Error("an empty issue is marked so it is not looked at again")
	}
}
