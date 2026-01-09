package nodeherder

import "strings"

// LLMDeviceContext represents the device context in a format suitable for LLM consumption.
type LLMDeviceContext struct {
	Version     string      `json:"version"`
	GeneratedAt int64       `json:"generated_at"`
	Devices     []LLMDevice `json:"devices"`
}

func (c *LLMDeviceContext) Summary() string {
	var b strings.Builder

	for _, d := range c.Devices {
		b.WriteString("- id: ")
		b.WriteString(d.ID)
		b.WriteString("\n")

		b.WriteString("  name: ")
		b.WriteString(d.Name)
		b.WriteString("\n")

		if len(d.Exposes) > 0 {
			b.WriteString("  exposes:\n")

			for _, e := range d.Exposes {
				b.WriteString("    - ")
				b.WriteString(e.Name)

				if e.Unit != "" {
					b.WriteString("  [unit: ")
					b.WriteString(e.Unit)
					b.WriteString("]")
				}

				if len(e.Aggregations) > 0 {
					b.WriteString("  [aggregations: ")
					b.WriteString(strings.Join(e.Aggregations, ", "))
					b.WriteString("]")
				}

				b.WriteString("\n")
			}
		}

		b.WriteString("\n")
	}

	return b.String()
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
