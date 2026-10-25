package profile

import (
	"slices"
	"strings"

	"llm-proxy/models"
)

// Field kinds the editor knows how to draw.
const (
	KindText     = "text"
	KindLongText = "longtext"
	KindToggle   = "toggle"
	KindNumber   = "number"
	KindChoice   = "choice"
	KindChoices  = "choices"
	KindProfiles = "profiles"
)

// Option is one value a choice field accepts.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field describes one editable setting of a profile: what the editor shows and what the parser accepts, in one table.
type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	Help     string   `json:"help"`
	Required bool     `json:"required,omitempty"`
	Max      int      `json:"max,omitempty"`
	Options  []Option `json:"options,omitempty"`
}

// toolCategoryLabels name the categories a profile may list; the always-on ones, general and delegation are not
// offered (delegation is chosen under delegates).
var toolCategoryLabels = map[string]string{
	models.CategoryTerminal:      "Terminal commands",
	models.CategorySearch:        "Web search",
	models.CategoryCommunication: "Send messages",
	models.CategoryFileSystem:    "Read and write files",
	models.CategoryNetwork:       "Fetch pages, network scan",
	models.CategoryMemory:        "Memory",
	models.CategoryReminders:     "Reminders",
}

// Schema lists the fields of a profile in the order the editor draws them.
func Schema() []Field {
	return []Field{
		{Key: "name", Label: "Name", Kind: KindText, Required: true, Help: "What the picker shows."},
		{Key: "summary", Label: "Summary", Kind: KindText, Help: "One line under the name, so you can tell profiles apart."},
		{Key: "instructions", Label: "Instructions", Kind: KindLongText, Required: true,
			Help: "How this agent behaves. They go after the built-in rules, which they cannot remove."},
		{Key: "answer_first", Label: "Answer first", Kind: KindToggle,
			Help: "A plain text reply is a complete answer; it is not pushed to use a tool first."},
		{Key: "may_ask", Label: "May ask a question", Kind: KindToggle,
			Help: "A turn may end with one clarifying question. Leave off for unattended runs."},
		{Key: "max_steps", Label: "Step limit", Kind: KindNumber, Max: maxStepsLimit,
			Help: "Most steps one run may take. 0 uses the model's own limit."},
		{Key: "tools", Label: "Tools", Kind: KindChoices, Options: toolOptions(),
			Help: "Only these kinds of tool are offered. None ticked offers all of them."},
		{Key: "memory", Label: "Memory", Kind: KindChoice, Options: []Option{
			{Value: "", Label: "Use the global setting"}, {Value: string(models.MemoryModeOn), Label: "On"}, {Value: string(models.MemoryModeOff), Label: "Off"}},
			Help: "Whether saved facts are put in front of the model. A workspace or automation setting still wins."},
		{Key: "memory_save", Label: "Saving to memory", Kind: KindChoice, Options: []Option{
			{Value: string(SaveModel), Label: "The model decides"}, {Value: string(SaveExplicit), Label: "Only what I ask to remember"}, {Value: string(SaveOff), Label: "Nothing"}},
			Help: "Who decides what is saved. Reading memory is not affected."},
		{Key: "reminders", Label: "Reminders", Kind: KindToggle,
			Help: "The reminder tools are offered. Needs the Reminders tool kind when tools are limited."},
		{Key: "local_only", Label: "Local model only", Kind: KindToggle,
			Help: "Runs stop unless the model is on this machine or network, so nothing the run sees is sent to a cloud provider."},
		{Key: "delegates", Label: "Helpers", Kind: KindProfiles,
			Help: "Profiles this one may hand tasks to. A helper runs on the Helper model with only web search and page fetch, whatever its own tools say."},
	}
}

func toolOptions() []Option {
	var out []Option
	for _, category := range models.ToolCategories() {
		if label, ok := toolCategoryLabels[category]; ok {
			out = append(out, Option{Value: category, Label: label})
		}
	}
	return out
}

// Explain reads a refusal from Save: the setting it is about ("" when it is about the profile as a whole) and the
// message for the person, without the internal prefixes, so a form can show it beside the right control.
func Explain(err error) (field, message string) {
	message = strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")
	if rest, ok := strings.CutPrefix(message, "profile "); ok {
		_, message, _ = strings.Cut(rest, ": ") // "profile <id>: " names the file, not the setting
	}
	keys := make([]string, 0, 12)
	for _, f := range Schema() {
		keys = append(keys, f.Key)
	}
	slices.SortFunc(keys, func(a, b string) int { return len(b) - len(a) }) // memory_save before memory
	for _, key := range keys {
		if strings.HasPrefix(message, key) {
			return key, message
		}
	}
	return "", message
}
