package brief

import (
	"errors"
	"testing"

	"llm-proxy-stacks/newsroom/sources"
)

func TestMySources_AddListRemoveAndSurviveARestart(t *testing.T) {
	s, dir := newStore(t)
	m, err := s.AddMySource("  My blog ", sources.KindFeed, "https://blog.example/feed.xml")
	if err != nil || m.ID == 0 || m.Label != "My blog" {
		t.Fatalf("add: %+v, err %v", m, err)
	}
	again, err := NewStore(dir)
	if err != nil || len(again.MySources()) != 1 || again.MySources()[0] != m {
		t.Fatalf("after reopening: %+v, err %v", again.MySources(), err)
	}
	if err := s.DeleteMySource(m.ID); err != nil || len(s.MySources()) != 0 {
		t.Fatalf("delete: %v, left %v", err, s.MySources())
	}
	if err := s.DeleteMySource(m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
}

func TestMySources_RefusesWhatATopicWouldRefuseAndRepeats(t *testing.T) {
	s, _ := newStore(t)
	builtin := sources.Catalog()[0]
	cases := []struct {
		name, label string
		kind        sources.Kind
		target      string
	}{
		{"no name", " ", sources.KindFeed, "https://blog.example/feed.xml"},
		{"not an address", "Blog", sources.KindFeed, "not a url"},
		{"a kind the list does not hold", "HN", sources.KindHN, "ai"},
		{"already built in", "Again", builtin.Kind, builtin.Target},
		{"already built in, other letter case", "Again", builtin.Kind, upper(builtin.Target)},
	}
	for _, c := range cases {
		if _, err := s.AddMySource(c.label, c.kind, c.target); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := s.AddMySource("Blog", sources.KindFeed, "https://blog.example/feed.xml"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMySource("Blog two", sources.KindFeed, "https://blog.example/FEED.xml"); !errors.Is(err, ErrInvalid) {
		t.Errorf("a repeat of the owner's own source: %v", err)
	}
}

func TestMySources_AreBounded(t *testing.T) {
	s, _ := newStore(t)
	for i := range MaxMySources {
		if _, err := s.AddMySource("Blog", sources.KindFeed, "https://blog.example/"+string(rune('a'+i%26))+string(rune('a'+i/26))+".xml"); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	if _, err := s.AddMySource("One more", sources.KindFeed, "https://blog.example/more.xml"); !errors.Is(err, ErrInvalid) {
		t.Errorf("past the limit: %v", err)
	}
}

func TestMySources_ErasedWithEverythingElse(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.AddMySource("Blog", sources.KindFeed, "https://blog.example/feed.xml"); err != nil {
		t.Fatal(err)
	}
	if err := s.Erase(); err != nil || len(s.MySources()) != 0 {
		t.Fatalf("erase: %v, left %v", err, s.MySources())
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
