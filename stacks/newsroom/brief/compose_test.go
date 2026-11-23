package brief

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// fakeModel answers each call from a script and keeps what it was shown, so a test can check both the prompt and what comes of the answer.
type fakeModel struct {
	answers []string
	errs    []error
	prompts []string
}

func (f *fakeModel) Complete(_ context.Context, system, user string) (string, error) {
	n := len(f.prompts)
	f.prompts = append(f.prompts, system+"\n"+user)
	if n < len(f.errs) && f.errs[n] != nil {
		return "", f.errs[n]
	}
	if n < len(f.answers) {
		return f.answers[n], nil
	}
	return "{}", nil
}

var composeNow = time.Date(2026, 11, 22, 7, 0, 0, 0, time.UTC)

func poolStory(id int64, title, text string, outlets int, topic int64, age time.Duration) PoolItem {
	p := PoolItem{ID: id, Key: TitleKey(title), Title: title, Text: text, Link: fmt.Sprintf("https://example.com/%d", id), Posted: composeNow.Add(-age), FirstSeen: composeNow.Add(-age), Topics: []int64{topic}}
	for i := range outlets {
		p.Reports = append(p.Reports, Report{Source: "feed", Publisher: fmt.Sprintf("Outlet %d", i+1), Link: fmt.Sprintf("https://o%d.example/%d", i, id)})
	}
	return p
}

func oneSection(pool ...PoolItem) ComposeInput {
	return ComposeInput{
		Edition: Edition{ID: 1, Name: "Morning", CollectEvery: 2 * time.Hour, Sections: []Section{{Kind: SectionNews, TopicID: 7}}},
		Topics:  []Topic{{ID: 7, Name: "Technology", Category: "Tech", LastCollected: composeNow.Add(-time.Hour)}},
		Pool:    pool,
		Now:     composeNow,
	}
}

// A line the model wrote about an item it was never shown is removed: every story must cite a shown item (Constitution V.2).
func TestCompose_AFabricatedCitationIsRemoved(t *testing.T) {
	m := &fakeModel{answers: []string{`{"stories":[{"item":1,"headline":"Chip ban widens","summary":"Export rules now cover more chips."},{"item":9,"headline":"Invented story","summary":"Never in the pool."},{"item":0,"headline":"No citation"}]}`}}
	in := oneSection(poolStory(1, "Chip export ban widens", "Rules cover more chips from today.", 2, 7, time.Hour))
	in.Model = m
	got := Compose(context.Background(), in)
	st := got.Sections[0].Stories
	if len(st) != 1 || st[0].PoolID != 1 || st[0].Headline != "Chip ban widens" || st[0].Summary == "" {
		t.Fatalf("only the cited story stays: %+v", st)
	}
	for _, s := range st {
		if strings.Contains(s.Headline+s.Summary, "Invented") {
			t.Errorf("a fabricated line reached the edition: %+v", s)
		}
	}
}

// A summary written from a bare headline is invention nothing can check, so a headline-only story keeps its own headline and gets no summary.
func TestCompose_AHeadlineOnlyStoryHasNoModelSummary(t *testing.T) {
	m := &fakeModel{answers: []string{`{"stories":[{"item":1,"headline":"Reworded","summary":"Made up detail."}]}`}}
	in := oneSection(poolStory(1, "Quake hits coast", "", 1, 7, time.Hour))
	in.Model = m
	s := Compose(context.Background(), in).Sections[0].Stories[0]
	if s.Summary != "" || !s.HeadlineOnly || s.Headline != "Quake hits coast" {
		t.Errorf("headline only: no summary, no rewording: %+v", s)
	}
}

// The model sees a bounded number of candidates, the best by number of outlets, and nothing already shown in the last day unless more outlets now report it.
func TestCompose_CandidatesAreCappedAndTheAlreadyShownRuleHolds(t *testing.T) {
	var pool []PoolItem
	for i := int64(1); i <= 40; i++ {
		pool = append(pool, poolStory(i, fmt.Sprintf("Story number %d", i), "text", 3-int(i%3), 7, time.Hour))
	}
	in := oneSection(pool...)
	in.Shown = []Shown{
		{Key: pool[2].Key, At: composeNow.Add(-5 * time.Hour), Outlets: pool[2].Outlets()},     // shown, nothing new: dropped
		{Key: pool[5].Key, At: composeNow.Add(-5 * time.Hour), Outlets: pool[5].Outlets() - 1}, // shown, more outlets since: an update
		{Key: pool[8].Key, At: composeNow.Add(-30 * time.Hour), Outlets: pool[8].Outlets()},    // shown long ago: fair again
	}
	m := &fakeModel{}
	in.Model = m
	got := Compose(context.Background(), in)
	shown := 0
	for i := 1; i <= 40; i++ {
		if strings.Contains(m.prompts[0], fmt.Sprintf("Story number %d ", i)) || strings.Contains(m.prompts[0], fmt.Sprintf("Story number %d\n", i)) {
			shown++
		}
	}
	if shown != CandidatesPerSection {
		t.Errorf("the model sees %d candidates, want the cap %d", shown, CandidatesPerSection)
	}
	if strings.Contains(m.prompts[0], "Story number 3\n") || strings.Contains(m.prompts[0], "Story number 3 ") {
		t.Error("a story shown 5 hours ago with no new outlet was offered again")
	}
	for _, s := range got.Sections[0].Stories {
		if s.PoolID == pool[2].ID {
			t.Error("an already shown story came back")
		}
		if s.PoolID == pool[5].ID && !s.Update {
			t.Error("a story with more outlets since is an update, and says so")
		}
	}
}

// A story shown before that more outlets now report is shown again and labelled an update.
func TestCompose_AStoryWithMoreOutletsSinceIsAnUpdate(t *testing.T) {
	p := poolStory(1, "Port strike spreads", "text", 3, 7, time.Hour)
	in := oneSection(p)
	in.Shown = []Shown{{Key: p.Key, At: composeNow.Add(-6 * time.Hour), Outlets: 1}}
	in.Model = &fakeModel{}
	st := Compose(context.Background(), in).Sections[0].Stories
	if len(st) != 1 || !st[0].Update {
		t.Errorf("an update, labelled: %+v", st)
	}
}

// When the model cannot be used the edition still ships: stories ranked by outlets, under their own headlines, and the section says why.
func TestCompose_AModelThatCannotAnswerStillGivesAnEdition(t *testing.T) {
	cases := map[string]error{
		"limit spent": stacksdk.Errorf(stacksdk.CodeDenied, "the stack's limit of model calls for today is spent"),
		"unavailable": stacksdk.Errorf(stacksdk.CodeUnavailable, "no model could answer"),
		"plain":       errors.New("boom"),
	}
	for name, err := range cases {
		t.Run(name, func(t *testing.T) {
			m := &fakeModel{errs: []error{err, err, err}}
			in := oneSection(poolStory(1, "Small story", "text", 1, 7, time.Hour), poolStory(2, "Big story", "text", 3, 7, 2*time.Hour))
			in.Model = m
			got := Compose(context.Background(), in)
			sec := got.Sections[0]
			if len(sec.Stories) != 2 || sec.Stories[0].PoolID != 2 || sec.Stories[0].Headline != "Big story" || sec.Stories[0].Summary != "" {
				t.Errorf("ranked by outlets, own headline, no summary: %+v", sec.Stories)
			}
			if sec.Note == "" {
				t.Error("the section must say it was not written by the model")
			}
		})
	}
	m := &fakeModel{errs: []error{stacksdk.Errorf(stacksdk.CodeDenied, "spent")}}
	in := oneSection(poolStory(1, "A", "t", 1, 7, time.Hour))
	in.Edition.Sections = append(in.Edition.Sections, Section{Kind: SectionNews, TopicID: 8})
	in.Topics = append(in.Topics, Topic{ID: 8, Name: "World", LastCollected: composeNow})
	in.Pool = append(in.Pool, poolStory(2, "B", "t", 1, 8, time.Hour))
	in.Model = m
	Compose(context.Background(), in)
	if len(m.prompts) != 1 {
		t.Errorf("after the limit is spent no further call is made, got %d", len(m.prompts))
	}
}

// Top stories are chosen by citing numbers from the stories already written, so the model can rank but never add; a bad number is dropped.
func TestCompose_TopStoriesCiteWrittenStories(t *testing.T) {
	var pool []PoolItem
	for i := int64(1); i <= 6; i++ {
		pool = append(pool, poolStory(i, fmt.Sprintf("Headline %d", i), "text", int(i), 7, time.Hour))
	}
	m := &fakeModel{answers: []string{"{}", `{"top":[3,99,3,1]}`}}
	in := oneSection(pool...)
	in.Model = m
	got := Compose(context.Background(), in)
	if len(m.prompts) != 2 {
		t.Fatalf("one call for the section and one for Top stories, got %d", len(m.prompts))
	}
	if got.ModelCalls != 2 {
		t.Errorf("model calls = %d", got.ModelCalls)
	}
	var ids []int64
	for _, s := range got.Top {
		ids = append(ids, s.PoolID)
	}
	if len(ids) != 2 {
		t.Fatalf("the unknown and the repeated number are dropped: %v", ids)
	}
}

// A topic never collected, or collected long ago, is labelled; the problems found when it was read are carried so the edition says where its news may be missing.
func TestCompose_AStaleOrUnreadSectionIsLabelled(t *testing.T) {
	in := oneSection(poolStory(1, "A", "t", 1, 7, time.Hour))
	in.Topics[0].LastCollected = composeNow.Add(-9 * time.Hour)
	in.Topics[0].LastProblems = []string{"Feed X: could not be read"}
	in.Model = &fakeModel{}
	sec := Compose(context.Background(), in).Sections[0]
	if !strings.Contains(sec.Note, "9 hours") || len(sec.Problems) != 1 {
		t.Errorf("stale label and problems: %q %v", sec.Note, sec.Problems)
	}
	in.Topics[0].LastCollected = time.Time{}
	if sec := Compose(context.Background(), in).Sections[0]; !strings.Contains(sec.Note, "not collected") {
		t.Errorf("never collected: %q", sec.Note)
	}
}

// Replaying the same inputs gives the same edition (the plan's acceptance): nothing depends on map order.
func TestCompose_ReplayGivesTheSameEdition(t *testing.T) {
	var pool []PoolItem
	for i := int64(1); i <= 12; i++ {
		pool = append(pool, poolStory(i, fmt.Sprintf("Headline %d", i), "text", 1+int(i%4), 7, time.Duration(i)*time.Minute))
	}
	run := func() Issue {
		in := oneSection(pool...)
		in.Model = &fakeModel{}
		return Compose(context.Background(), in)
	}
	a, b := run(), run()
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Errorf("two runs differ:\n%v\n%v", a, b)
	}
}

// A story that belongs to two topics is in the first section only, and the second section is filled from its other stories.
func TestCompose_AStoryInTwoTopicsAppearsOnce(t *testing.T) {
	both := poolStory(1, "Markets fall on AI fears", "Shares dropped.", 3, 7, time.Hour)
	both.Topics = []int64{7, 8}
	in := oneSection(both, poolStory(2, "New chip announced", "A chip.", 1, 8, 2*time.Hour))
	in.Edition.Sections = []Section{{Kind: SectionNews, TopicID: 7}, {Kind: SectionNews, TopicID: 8}}
	in.Topics = append(in.Topics, Topic{ID: 8, Name: "Economy", LastCollected: composeNow.Add(-time.Hour)})
	got := Compose(context.Background(), in)
	if len(got.Sections) != 2 || len(got.Sections[0].Stories) != 1 || got.Sections[0].Stories[0].PoolID != 1 {
		t.Fatalf("the shared story belongs to the first section: %+v", got.Sections)
	}
	if st := got.Sections[1].Stories; len(st) != 1 || st[0].PoolID != 2 {
		t.Errorf("the second section keeps only its own story: %+v", st)
	}
}

// Topics that share a category (however it is spelled) are one section: a story found by any of them is in it, once, and the section is as stale as its
// oldest topic.
func TestCompose_TopicsOfOneCategoryAreOneSection(t *testing.T) {
	both := poolStory(1, "Chip maker shares jump", "Shares rose.", 2, 7, time.Hour)
	both.Topics = []int64{7, 8}
	in := oneSection(both, poolStory(2, "New phone announced", "A phone.", 1, 8, 2*time.Hour), poolStory(3, "Election result", "A vote.", 1, 9, time.Hour))
	in.Edition.Sections = []Section{{Kind: SectionNews, TopicID: 7}, {Kind: SectionNews, TopicID: 9}, {Kind: SectionNews, TopicID: 8}}
	in.Topics = []Topic{
		{ID: 7, Name: "AI", Category: "Technology", LastCollected: composeNow.Add(-time.Hour), LastProblems: []string{"Feed X: could not be read"}},
		{ID: 8, Name: "Phones", Category: " technology ", LastCollected: composeNow.Add(-5 * time.Hour)},
		{ID: 9, Name: "UK", Category: "News", LastCollected: composeNow.Add(-time.Hour)},
	}
	got := Compose(context.Background(), in)
	if len(got.Sections) != 2 || got.Sections[0].Name != "Technology" || got.Sections[1].Name != "News" {
		t.Fatalf("one section a category, in first-seen order: %+v", got.Sections)
	}
	tech := got.Sections[0]
	if len(tech.Stories) != 2 {
		t.Errorf("both topics' stories, the shared one once: %+v", tech.Stories)
	}
	if !tech.AsOf.Equal(composeNow.Add(-5*time.Hour)) || len(tech.Problems) != 1 || !strings.HasPrefix(tech.Problems[0], "AI: ") {
		t.Errorf("oldest read and a problem named by its topic: %v %v", tech.AsOf, tech.Problems)
	}
}

// A topic with no category is its own section, named as the topic.
func TestTopic_NoCategoryMeansItsOwnNameAndNormalizeFillsIt(t *testing.T) {
	if g := (Topic{Name: " Markets "}).Group(); g != "Markets" {
		t.Errorf("group: %q", g)
	}
	n, err := (Topic{Name: "Markets", Queries: []string{"markets"}}).Normalize()
	if err != nil || n.Category != "Markets" {
		t.Errorf("normalize: %+v %v", n, err)
	}
}
