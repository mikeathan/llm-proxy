// Package profile is the catalog of prompt profiles: a profile bundles the behaviour block of the system prompt
// with the policy the agent loop needs to match it (answer-first, tools, memory, reminders). Built-ins are embedded
// files (prompts/profiles); a user's own are files in the data root's profiles directory, outside every workspace.
package profile

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"llm-proxy/models"
)

// Source says where a profile came from.
type Source string

const (
	Builtin Source = "builtin"
	User    Source = "user"
)

const (
	frontMatterFence = "---"
	maxStepsLimit    = 200 // far above any model's own cap; a typo guard, not a policy
)

// idPattern is a file-name stem that is safe to join to a directory.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// alwaysOnCategories are never hidden by a profile: the run's own error channel, the guardrail tool and the
// journal (which is already scoped to runs that keep one).
var alwaysOnCategories = []string{models.CategorySystem, models.CategoryGlobal, models.CategoryAutomation}

// Profile is one selectable behaviour. Boolean fields left out of a file are false.
type Profile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Source  Source `json:"source"`
	// Instructions is the behaviour block placed after the code-owned core rules.
	Instructions string `json:"-"`
	// AnswerFirst makes a plain text first turn a complete answer: no prior tool result is needed, the no-tool output
	// cap does not apply and the "call a tool" nudges are not sent.
	AnswerFirst bool `json:"answer_first"`
	// MayAsk lets a turn end with one clarifying question. Unattended profiles leave it false.
	MayAsk bool `json:"may_ask"`
	// MaxSteps overrides the model's step budget when greater than zero.
	MaxSteps int `json:"max_steps,omitempty"`
	// Tools lists the tool categories the profile exposes; empty exposes all of them.
	Tools []string `json:"tools,omitempty"`
	// Memory is the profile's default for hot memory; an explicit workspace or automation setting still wins.
	Memory models.MemoryMode `json:"memory,omitempty"`
	// MemorySave is who decides what gets saved to memory (SaveMode). Reading memory is the Memory field's business.
	MemorySave SaveMode `json:"memory_save"`
	// Reminders exposes the reminder tools.
	Reminders bool `json:"reminders"`
}

// frontMatter is the file header. Unknown keys are an error, so a typo in a user's file is not silently ignored.
type frontMatter struct {
	Name        string            `yaml:"name"`
	Summary     string            `yaml:"summary"`
	AnswerFirst bool              `yaml:"answer_first"`
	MayAsk      bool              `yaml:"may_ask"`
	MaxSteps    int               `yaml:"max_steps"`
	Tools       []string          `yaml:"tools"`
	Memory      models.MemoryMode `yaml:"memory"`
	MemorySave  SaveMode          `yaml:"memory_save"`
	Reminders   bool              `yaml:"reminders"`
}

// Unprofiled is what an agent runs with when no profile was chosen (a direct constructor, a test): the loop as it
// behaved before profiles existed — tool-first, with the save guidance and the reminders on and every tool exposed.
// Runs started by a chat or an automation always carry a real profile.
func Unprofiled() Profile { return Profile{Reminders: true, MemorySave: SaveModel} }

// ValidID reports whether id can name a profile (and so a file).
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Parse reads a profile file: YAML front matter between two "---" lines, then the instructions.
func Parse(id string, source Source, data []byte) (Profile, error) {
	if !ValidID(id) {
		return Profile{}, fmt.Errorf("profile id %q: use lowercase letters, digits and dashes (at most 40)", id)
	}
	header, body, err := splitFrontMatter(string(data))
	if err != nil {
		return Profile{}, fmt.Errorf("profile %s: %w", id, err)
	}
	var fm frontMatter
	dec := yaml.NewDecoder(bytes.NewReader([]byte(header)))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil {
		return Profile{}, fmt.Errorf("profile %s: front matter: %w", id, err)
	}
	p := Profile{
		ID: id, Name: strings.TrimSpace(fm.Name), Summary: strings.TrimSpace(fm.Summary), Source: source,
		Instructions: strings.TrimSpace(body), AnswerFirst: fm.AnswerFirst, MayAsk: fm.MayAsk, MaxSteps: fm.MaxSteps,
		Tools: fm.Tools, Memory: fm.Memory, MemorySave: fm.MemorySave.Effective(), Reminders: fm.Reminders,
	}
	if err := p.validate(); err != nil {
		return Profile{}, fmt.Errorf("profile %s: %w", id, err)
	}
	return p, nil
}

// splitFrontMatter separates the header lines from the body.
func splitFrontMatter(text string) (header, body string, err error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	rest, found := strings.CutPrefix(text, frontMatterFence+"\n")
	if !found {
		return "", "", errors.New("the file must start with a --- front matter block")
	}
	header, body, found = strings.Cut(rest, "\n"+frontMatterFence)
	if !found {
		return "", "", errors.New("the front matter block is not closed with ---")
	}
	return header, strings.TrimPrefix(body, "\n"), nil
}

func (p Profile) validate() error {
	switch {
	case p.Name == "":
		return errors.New("name is required")
	case p.Instructions == "":
		return errors.New("instructions are required: write them after the front matter")
	case p.MaxSteps < 0 || p.MaxSteps > maxStepsLimit:
		return fmt.Errorf("max_steps must be between 0 and %d", maxStepsLimit)
	case p.Memory != models.MemoryModeInherit && !p.Memory.Valid():
		return fmt.Errorf("memory %q: use on or off, or leave it out", p.Memory)
	case !p.MemorySave.Valid():
		return fmt.Errorf("memory_save %q: use %s, or leave it out", p.MemorySave, saveModeChoices)
	}
	return p.validateTools()
}

// validateTools checks the tool categories the profile lists and that its reminders flag can be honoured by them.
func (p Profile) validateTools() error {
	for _, category := range p.Tools {
		if !models.IsToolCategory(category) {
			return fmt.Errorf("tools: %q is not a tool category (%s)", category, strings.Join(models.ToolCategories(), ", "))
		}
	}
	if p.Reminders && len(p.Tools) > 0 && !slices.Contains(p.Tools, models.CategoryReminders) {
		return errors.New("reminders: true needs the reminders category in tools")
	}
	return nil
}

// ExcludedTools names the built-in tools this profile hides: every category it does not list (none when it lists
// none), the always-on categories excepted, the reminder tools whenever Reminders is off, and the memory write tools
// unless the profile lets the model save.
func (p Profile) ExcludedTools() []string {
	var hidden []string
	if len(p.Tools) > 0 {
		for _, category := range models.ToolCategories() {
			if !slices.Contains(p.Tools, category) && !slices.Contains(alwaysOnCategories, category) {
				hidden = append(hidden, models.ToolsInCategory(category)...)
			}
		}
	}
	if !p.Reminders {
		hidden = append(hidden, models.ToolsInCategory(models.CategoryReminders)...)
	}
	if !p.MemorySave.OffersWriteTools() {
		hidden = append(hidden, models.ToolMemoryUpdate, models.ToolMemoryForget)
	}
	slices.Sort(hidden)
	return slices.Compact(hidden)
}

// GuidesMemorySaves reports whether the profile has the model decide what to save, so the run is told when to.
func (p Profile) GuidesMemorySaves() bool { return p.MemorySave.OffersWriteTools() }

// CapturesMemory reports whether code may save what the user explicitly asks to remember.
func (p Profile) CapturesMemory() bool { return p.MemorySave.Effective() != SaveOff }
