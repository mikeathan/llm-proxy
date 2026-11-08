package stackport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

type doc struct {
	On    bool     `json:"on"`
	Every int      `json:"every"`
	Names []string `json:"names"`
	Other string   `json:"other"`
}

type memStore struct {
	v       doc
	saved   int
	failing error
}

func (m *memStore) Get() doc { return m.v }
func (m *memStore) Update(fn func(*doc) error) error {
	next := m.v
	if err := fn(&next); err != nil {
		return err
	}
	if m.failing != nil {
		return m.failing
	}
	m.v = next
	m.saved++
	return nil
}

func TestConfig_LoadShowsEveryFieldAndSaveOfWhatWasLoadedChangesNothing(t *testing.T) {
	s := &memStore{v: doc{On: true, Every: 60, Names: []string{"a"}, Other: "keep"}}
	c := Config[doc]{Store: s}
	got, err := c.Load(context.Background())
	if err != nil || !strings.Contains(string(got), `"every":60`) || !strings.Contains(string(got), `"other":"keep"`) {
		t.Fatalf("load = %s, %v", got, err)
	}
	before := s.v
	if err := c.Save(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(before); string(b) != string(got) {
		t.Fatal("test setup")
	}
	if after, _ := json.Marshal(s.v); string(after) != string(got) {
		t.Errorf("save of the loaded document changed it: %s", after)
	}
}

func TestConfig_SaveMergesAPartialDocumentAndKeepsTheOtherFields(t *testing.T) {
	s := &memStore{v: doc{On: false, Every: 60, Names: []string{"a", "b"}, Other: "keep"}}
	c := Config[doc]{Store: s}
	if err := c.Save(context.Background(), json.RawMessage(`{"on":true,"names":[]}`)); err != nil {
		t.Fatal(err)
	}
	if !s.v.On || len(s.v.Names) != 0 || s.v.Every != 60 || s.v.Other != "keep" {
		t.Errorf("merged = %+v: named fields replace, left-out fields stay", s.v)
	}
	if err := c.Save(context.Background(), json.RawMessage(`{"on":true,"unknown_field":1}`)); err != nil || s.v.Other != "keep" {
		t.Errorf("an unknown key must be ignored, got %v %+v", err, s.v)
	}
}

func TestConfig_ABadDocumentSavesNothingAndSaysWhyInFixedWords(t *testing.T) {
	for name, raw := range map[string]string{
		"not json":      `{"on":`,
		"not an object": `[1,2]`,
		"null":          `null`,
		"wrong type":    `{"every":"secret-value-60"}`,
	} {
		s := &memStore{v: doc{Every: 60}}
		err := Config[doc]{Store: s}.Save(context.Background(), json.RawMessage(raw))
		if stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
			t.Errorf("%s: code = %q (%v)", name, stacksdk.CodeOf(err), err)
		}
		if s.saved != 0 || s.v.Every != 60 {
			t.Errorf("%s: something was saved: %+v", name, s.v)
		}
		if err != nil && strings.Contains(err.Error(), "secret-value") {
			t.Errorf("%s: the message repeats the document: %v", name, err)
		}
	}
}

func TestConfig_AFailingStoreIsAnInternalErrorWithFixedWords(t *testing.T) {
	s := &memStore{failing: errors.New("disk full at /var/lib/secret")}
	err := Config[doc]{Store: s}.Save(context.Background(), json.RawMessage(`{"on":true}`))
	if stacksdk.CodeOf(err) != stacksdk.CodeInternal || strings.Contains(err.Error(), "/var/lib") {
		t.Errorf("err = %v", err)
	}
}

// A save never writes into the slices the stored settings hold: the stored value is read by other goroutines while it is saved.
func TestConfig_ASaveLeavesTheStoredSlicesAlone(t *testing.T) {
	s := &memStore{v: doc{Names: []string{"a", "b"}}}
	held := s.v.Names // what a reader holding the old settings sees
	if err := (Config[doc]{Store: s}).Save(context.Background(), json.RawMessage(`{"names":["x","y"]}`)); err != nil {
		t.Fatal(err)
	}
	if held[0] != "a" || held[1] != "b" {
		t.Errorf("the old slice was rewritten in place: %v", held)
	}
	if s.v.Names[0] != "x" {
		t.Errorf("saved = %v", s.v.Names)
	}
}
