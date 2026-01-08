package nodeherder

// LLMDeviceContext represents the device context in a format suitable for LLM consumption.
type LLMDeviceContext struct {
	Version     string      `json:"version"`
	GeneratedAt int64       `json:"generated_at"`
	Devices     []LLMDevice `json:"devices"`
}

func (c *LLMDeviceContext) Summary() string {
	var out string

	for _, d := range c.Devices {
		out += "- id: " + d.ID + "\n"
		out += "  name: " + d.Name + "\n"

		if len(d.Exposes) > 0 {
			out += "  exposes:\n"
			for key, e := range d.Exposes {
				out += "    - " + key
				if e.Unit != "" {
					out += " (" + e.Unit + ")"
				}
				out += "\n"
			}
		}

		out += "\n"
	}

	return out
}

type LLMDevice struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Desc    string               `json:"desc,omitempty"`
	Exposes map[string]LLMExpose `json:"exposes"`
}

type LLMExpose struct {
	Type         string   `json:"type"`
	Unit         string   `json:"unit,omitempty"`
	States       []string `json:"states,omitempty"`
	On           any      `json:"on,omitempty"`
	Off          any      `json:"off,omitempty"`
	Toggle       any      `json:"toggle,omitempty"`
	Aggregations []string `json:"aggregations"`
}
