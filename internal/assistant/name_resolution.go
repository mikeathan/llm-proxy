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
) (*nodeherder.LLMDevice, error) {

	target = strings.ToLower(strings.TrimSpace(target))
	expose = strings.ToLower(strings.TrimSpace(expose))

	var exact []nodeherder.LLMDevice
	var partial []nodeherder.LLMDevice

	for _, d := range ctx.Devices {
		name := strings.ToLower(d.Name)

		// Expose must exist on the device
		if _, ok := d.Exposes[expose]; !ok {
			continue
		}

		if name == target {
			exact = append(exact, d)
			continue
		}

		if strings.Contains(name, target) || strings.Contains(target, name) {
			partial = append(partial, d)
		}
	}

	// Prefer exact match
	if len(exact) == 1 {
		return &exact[0], nil
	}

	// Then partial match
	if len(exact) == 0 && len(partial) == 1 {
		return &partial[0], nil
	}

	if len(exact) > 1 || len(partial) > 1 {
		var names []string
		candidates := exact
		if len(candidates) == 0 {
			candidates = partial
		}

		for _, d := range candidates {
			names = append(names, d.Name)
		}

		return nil, fmt.Errorf(
			"ambiguous device name %q for expose %q; matches: %s",
			target,
			expose,
			strings.Join(names, ", "),
		)
	}

	return nil, fmt.Errorf(
		"no device matches %q with expose %q",
		target,
		expose,
	)
}
