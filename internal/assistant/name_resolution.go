package assistant

import (
	"fmt"
	"llm-proxy/internal/nodeherder"
	"strings"
)

func ResolveDevice(
	ctx *nodeherder.LLMDeviceContext,
	target string,
	expose string,
) (nodeherder.LLMDevice, error) {

	target = strings.ToLower(target)
	expose = strings.ToLower(expose)

	var matches []nodeherder.LLMDevice

	for _, d := range ctx.Devices {
		name := strings.ToLower(d.Name)

		if !strings.Contains(name, target) {
			continue
		}

		if _, ok := d.Exposes[expose]; !ok {
			continue
		}

		matches = append(matches, d)
	}

	if len(matches) == 0 {
		return nodeherder.LLMDevice{}, fmt.Errorf(
			"no device matches name=%q with metric=%q", target, expose,
		)
	}

	if len(matches) > 1 {
		var names []string
		for _, d := range matches {
			names = append(names, d.Name)
		}
		return nodeherder.LLMDevice{}, fmt.Errorf(
			"ambiguous device reference %q: %v", target, names,
		)
	}

	return matches[0], nil
}
