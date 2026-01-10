package devices

import (
	"fmt"
	"llm-proxy/internal/assistant/tools"
	"llm-proxy/internal/nodeherder"
	"sort"
	"strings"
)

type Candidate struct {
	ID   string
	Name string
}

type AmbiguousDeviceError struct {
	Target     string
	Expose     string
	Candidates []Candidate
}

// commonTokenWeights down ranks generic words that appear in many device names
// so location- or function-specific tokens dominate matching.
// Values are multipliers (0..1). Lower = less important.
var commonTokenWeights = map[string]float64{
	"sensor":      0.15,
	"device":      0.15,
	"room":        0.4,
	"meter":       0.4,
	"switch":      0.4,
	"plug":        0.4,
	"outlet":      0.4,
	"light":       0.5,
	"lamp":        0.5,
	"temp":        0.3,
	"temperature": 0.3,
	"humidity":    0.3,
	"motion":      0.3,
	"climate":     0.4,
	"thermo":      0.3,
	"heater":      0.5,
	"cooler":      0.5,
	"fan":         0.5,
	"air":         0.6,
	"power":       0.6,
	"energy":      0.6,
}

func (e *AmbiguousDeviceError) Error() string {
	names := make([]string, 0, len(e.Candidates))
	for _, c := range e.Candidates {
		names = append(names, c.Name)
	}
	return fmt.Sprintf("ambiguous device %q; matches: %s", e.Target, strings.Join(names, ", "))
}

func ResolveDevice(ctx *nodeherder.LLMDeviceContext, target, expose string) (*nodeherder.LLMDevice, error) {
	// ResolveDevice deterministically matches a device using weighted token overlap
	// and returns an ambiguity error when multiple candidates are too close.
	t := normalize(target)
	exposeKey := tools.NormalizeExpose(expose)
	if exposeKey == "" {
		return nil, fmt.Errorf("invalid expose %q", expose)
	}

	type scored struct {
		device nodeherder.LLMDevice
		score  float64
	}
	var matches []scored
	for _, d := range ctx.Devices {
		name := normalize(d.Name)

		if !deviceHasExpose(d, exposeKey) {
			continue
		}

		score := tokenScore(name, t)
		if score <= 0 {
			continue
		}

		matches = append(matches, scored{device: d, score: score})
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no device matches %q with expose %q", target, exposeKey)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].device.Name < matches[j].device.Name
		}
		return matches[i].score > matches[j].score
	})

	if len(matches) > 1 {
		top := matches[0]
		next := matches[1]
		if top.score > next.score+0.15 {
			return &top.device, nil
		}
		candidates := make([]Candidate, 0, len(matches))
		for _, d := range matches {
			candidates = append(candidates, Candidate{
				ID:   d.device.ID,
				Name: d.device.Name,
			})
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Name == candidates[j].Name {
				return candidates[i].ID < candidates[j].ID
			}
			return candidates[i].Name < candidates[j].Name
		})
		return nil, &AmbiguousDeviceError{
			Target:     target,
			Expose:     exposeKey,
			Candidates: candidates,
		}
	}

	return &matches[0].device, nil
}

func deviceHasExpose(d nodeherder.LLMDevice, exposeKey string) bool {
	for _, e := range d.Exposes {
		if tools.NormalizeExpose(e.Name) == exposeKey {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func tokenScore(name, target string) float64 {
	// Exact matches dominate; otherwise use weighted overlap to down rank common tokens.
	if name == target {
		// 1e6 is a deliberately huge score to guarantee that
		// exact string matches always outrank any fuzzy match.
		// All other scores are in the range 0..1.
		return 1e6
	}

	nameTokens := strings.Fields(name)
	targetTokens := strings.Fields(target)
	if len(targetTokens) == 0 {
		return 0
	}

	weight := func(token string) float64 {
		if w, ok := commonTokenWeights[token]; ok {
			return w
		}
		return 1.0
	}

	totalTargetWeight := 0.0
	matchedWeight := 0.0
	nameSet := make(map[string]struct{}, len(nameTokens))
	for _, token := range nameTokens {
		nameSet[token] = struct{}{}
	}

	for _, token := range targetTokens {
		w := weight(token)
		totalTargetWeight += w
		if _, ok := nameSet[token]; ok {
			matchedWeight += w
		}
	}

	if totalTargetWeight == 0 {
		return 0
	}

	score := matchedWeight / totalTargetWeight
	if score < 0.3 {
		return 0
	}
	return score
}
