package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// What Marshal writes is what Parse reads back, for every built-in and for a profile that uses every field.
func TestMarshalRoundTrips(t *testing.T) {
	c, _ := newCatalog(t)
	all := c.List()
	all = append(all, Profile{
		ID: "everything", Name: "Every: field", Summary: "line one\nline two: \"quoted\"", Source: User, Instructions: "Body.\n\n---\n\nAfter a rule.",
		AnswerFirst: true, MayAsk: true, MaxSteps: 12, Tools: []string{models.CategorySearch, models.CategoryReminders},
		Memory: models.MemoryModeOn, MemorySave: SaveExplicit, Reminders: true, Delegates: []string{"web-researcher"},
	})
	for _, p := range all {
		got, err := Parse(p.ID, p.Source, Marshal(p))
		if err != nil {
			t.Fatalf("%s: re-reading what Marshal wrote: %v", p.ID, err)
		}
		if !reflect.DeepEqual(got, p) {
			t.Errorf("%s: round trip changed the profile\n got %+v\nwant %+v", p.ID, got, p)
		}
	}
}

func sampleProfile(id string) Profile {
	return Profile{ID: id, Name: "Mine", Summary: "My helper", Instructions: "Be brief.", MemorySave: SaveModel}
}

func TestCatalogSave(t *testing.T) {
	c, dir := newCatalog(t)
	if err := c.Save(sampleProfile("mine")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok := c.Get("mine")
	if !ok || got.Source != User || got.Name != "Mine" || got.Instructions != "Be brief." {
		t.Errorf("saved profile = %+v, %v", got, ok)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "mine.md" {
		t.Errorf("the folder holds only the profile file, got %v", entries)
	}
	edited := sampleProfile("mine")
	edited.Name = "Mine, edited"
	if err := c.Save(edited); err != nil {
		t.Fatalf("Save over itself: %v", err)
	}
	if got, _ := c.Get("mine"); got.Name != "Mine, edited" {
		t.Errorf("an edit replaces the file, got %q", got.Name)
	}
}

func TestCatalogSaveCreatesTheFolder(t *testing.T) {
	c, err := NewCatalog(filepath.Join(t.TempDir(), "profiles"), logging.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Save(sampleProfile("mine")); err != nil {
		t.Fatalf("Save into a missing folder: %v", err)
	}
}

func TestCatalogSaveRefuses(t *testing.T) {
	big := sampleProfile("big")
	big.Instructions = strings.Repeat("x", maxFileBytes)
	noName := sampleProfile("noname")
	noName.Name = ""
	badID := sampleProfile("Bad ID")
	selfHelper := sampleProfile("loop")
	selfHelper.Delegates = []string{"loop"}
	unknownHelper := sampleProfile("lost")
	unknownHelper.Delegates = []string{"nobody"}
	for _, tc := range []struct {
		name string
		p    Profile
		want error
		text string
	}{
		{"too long", big, ErrInvalid, "too large"},
		{"no name", noName, ErrInvalid, "name is required"},
		{"bad id", badID, ErrInvalid, "profile id"},
		{"itself as a helper", selfHelper, ErrInvalid, "itself"},
		{"unknown helper", unknownHelper, ErrInvalid, "nobody"},
		{"a built-in's id", sampleProfile("research"), ErrBuiltin, "Duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, dir := newCatalog(t)
			err := c.Save(tc.p)
			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), tc.text) {
				t.Fatalf("Save = %v, want %v mentioning %q", err, tc.want, tc.text)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("a refused save leaves nothing behind, got %v", entries)
			}
		})
	}
}

func TestCatalogSaveStopsAtTheLimit(t *testing.T) {
	c, _ := newCatalog(t)
	for i := range maxUserProfiles {
		if err := c.Save(sampleProfile(fmt.Sprintf("p%d", i))); err != nil {
			t.Fatalf("profile %d: %v", i, err)
		}
	}
	if err := c.Save(sampleProfile("one-too-many")); !errors.Is(err, ErrInvalid) {
		t.Errorf("the profile past the limit: %v", err)
	}
	if err := c.Save(sampleProfile("p0")); err != nil {
		t.Errorf("editing one of the %d is still fine: %v", maxUserProfiles, err)
	}
}

func TestCatalogSaveEditsAFileThatReplacesABuiltin(t *testing.T) {
	c, dir := newCatalog(t)
	writeUser(t, dir, "research", "---\nname: My research\n---\nMine.")
	edited := sampleProfile("research")
	edited.Name = "My research 2"
	if err := c.Save(edited); err != nil {
		t.Fatalf("a user file that already replaces a built-in can be edited: %v", err)
	}
}

func TestCatalogDelete(t *testing.T) {
	c, dir := newCatalog(t)
	if err := c.Save(sampleProfile("mine")); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete("mine"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := c.Get("mine"); ok {
		t.Error("the deleted profile is gone")
	}
	if err := c.Delete("mine"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
	if err := c.Delete("general"); !errors.Is(err, ErrBuiltin) {
		t.Errorf("deleting a built-in: %v", err)
	}
	writeUser(t, dir, "research", "---\nname: My research\n---\nMine.")
	if err := c.Delete("research"); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Get("research"); got.Source != Builtin {
		t.Errorf("deleting the file that replaced a built-in brings the built-in back, got %+v", got)
	}
	if err := c.Delete("../x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a path-like id: %v", err)
	}
}

func TestCatalogDelegatingTo(t *testing.T) {
	c, _ := newCatalog(t)
	boss := sampleProfile("boss")
	boss.Delegates = []string{"web-researcher"}
	if err := c.Save(boss); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.DelegatingTo("web-researcher"), ","); got != "boss,research" {
		t.Errorf("profiles that name web-researcher = %q", got)
	}
	if got := c.DelegatingTo("boss"); len(got) != 0 {
		t.Errorf("nothing names boss, got %v", got)
	}
}

// Every way the parser can refuse a profile names the setting it is about, so the form can show it there.
func TestExplainNamesTheSetting(t *testing.T) {
	c, _ := newCatalog(t)
	long := sampleProfile("p")
	long.Instructions = strings.Repeat("x", maxFileBytes)
	for _, tc := range []struct {
		field string
		mod   func(*Profile)
	}{
		{"name", func(p *Profile) { p.Name = "" }},
		{"instructions", func(p *Profile) { p.Instructions = "" }},
		{"max_steps", func(p *Profile) { p.MaxSteps = 999 }},
		{"memory", func(p *Profile) { p.Memory = "sometimes" }},
		{"memory_save", func(p *Profile) { p.MemorySave = "maybe" }},
		{"tools", func(p *Profile) { p.Tools = []string{"nonsense"} }},
		{"reminders", func(p *Profile) { p.Reminders = true; p.Tools = []string{models.CategorySearch} }},
		{"delegates", func(p *Profile) { p.Delegates = []string{"ghost"} }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			p := sampleProfile("p")
			tc.mod(&p)
			err := c.Save(p)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Save = %v, want ErrInvalid", err)
			}
			if msg := func() string { _, m := Explain(err); return m }(); strings.Contains(msg, "profile p:") || strings.HasPrefix(msg, "invalid profile") {
				t.Errorf("the message keeps internal prefixes: %q", msg)
			}
			if got, _ := Explain(err); got != tc.field {
				t.Errorf("Explain field = %q, want %q (error %q)", got, tc.field, err)
			}
		})
	}
	if got, _ := Explain(c.Save(long)); got != "" {
		t.Errorf("a too-large profile is about the whole profile, got %q", got)
	}
}

// The schema offers exactly the settings Parse knows, and the tool kinds a profile may list.
func TestSchemaMatchesTheFileFormat(t *testing.T) {
	var keys []string
	for _, f := range Schema() {
		keys = append(keys, f.Key)
		if f.Label == "" || f.Help == "" {
			t.Errorf("%s has no label or help", f.Key)
		}
	}
	fileKeys := []string{"name", "summary", "instructions", "answer_first", "may_ask", "max_steps", "tools", "memory", "memory_save", "reminders", "delegates"}
	if !reflect.DeepEqual(keys, fileKeys) {
		t.Errorf("schema keys = %v, want %v", keys, fileKeys)
	}
	for _, f := range Schema() {
		if f.Key != "tools" {
			continue
		}
		for _, o := range f.Options {
			p := sampleProfile("p")
			p.Tools = []string{o.Value}
			if err := p.validate(); err != nil {
				t.Errorf("the editor offers %q but the parser refuses it: %v", o.Value, err)
			}
		}
	}
}
