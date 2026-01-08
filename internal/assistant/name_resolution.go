package assistant

import (
	"fmt"
	"llm-proxy/internal/nodeherder"
	"strings"
)

func ResolveDevice(ctx *nodeherder.LLMDeviceContext, target, expose string) (*nodeherder.LLMDevice, error) {
	t := normalize(target)

	var matches []nodeherder.LLMDevice

	for _, d := range ctx.Devices {
		name := normalize(d.Name)

		// Device must support the expose
		found := false
		for _, e := range d.Exposes {
			if normalize(e.Name) == expose {
				found = true
				break
			}
		}
		if !found {
			continue
		}

		// Semantic name match
		if !strings.Contains(name, t) && !strings.Contains(t, name) {
			continue
		}

		matches = append(matches, d)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no device matches %q with expose %q", target, expose)
	}

	if len(matches) > 1 {
		var names []string
		for _, d := range matches {
			names = append(names, d.Name)
		}
		return nil, fmt.Errorf("ambiguous device %q; matches: %s", target, strings.Join(names, ", "))
	}

	return &matches[0], nil
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}
