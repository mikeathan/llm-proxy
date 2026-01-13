package nodeherder

import "strings"

// LLMDeviceContext represents the device context in a format suitable for LLM consumption.
type LLMDeviceContext struct {
	Version     string      `json:"version"`
	GeneratedAt int64       `json:"generated_at"`
	Devices     []LLMDevice `json:"devices"`
}

const defaultSummaryMaxLen = 4000

func (c *LLMDeviceContext) Summary() string {
	return c.SummaryWithLimit(defaultSummaryMaxLen)
}

func (c *LLMDeviceContext) SummaryWithLimit(maxLen int) string {
	// SummaryWithLimit returns a token-friendly summary capped to avoid LLM context overflow.
	var b strings.Builder

	for _, d := range c.Devices {
		b.WriteString("- ")
		b.WriteString(d.ID)
		b.WriteString(" | ")
		b.WriteString(d.Name)
		b.WriteString(" | exposes: ")

		for i, e := range d.Exposes {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(e.Name)
		}

		b.WriteString("\n")
	}

	out := b.String()
	if maxLen > 0 && len(out) > maxLen {
		suffix := "\n... (truncated)"
		if maxLen <= len(suffix) {
			return out[:maxLen]
		}
		return out[:maxLen-len(suffix)] + suffix
	}

	return out
}

type LLMDevice struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Desc    string      `json:"desc,omitempty"`
	Exposes []LLMExpose `json:"exposes"`
}

type LLMExpose struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Unit         string   `json:"unit,omitempty"`
	States       []string `json:"states,omitempty"`
	On           any      `json:"valueOn,omitempty"`
	Off          any      `json:"valueOff,omitempty"`
	Toggle       any      `json:"valueToggle,omitempty"`
	Aggregations []string `json:"aggregations"`
}
