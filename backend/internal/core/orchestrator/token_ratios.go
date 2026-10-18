package orchestrator

import "sync"

const (
	// DefaultCharsPerToken is what a model with no observation yet is assumed to cost. It sits below the measured
	// range for dense tool output (JSON search results 2.8, raw HTML 2.7 to 3.4 on Qwen3.6) and under prose (4.1), so
	// a first request errs towards pruning early; one observation corrects it.
	DefaultCharsPerToken = 3.0

	minCharsPerToken = 1.5
	maxCharsPerToken = 6.0

	// ratioBlend is the weight of the newest observation: a run's content mix changes as tool results pile up, so the
	// estimate follows the latest requests but one odd request does not swing it.
	ratioBlend = 0.5

	// maxTrackedModels bounds the store: a model name is a registry entry, so this is never reached in practice, and a
	// name past it is simply not learned.
	maxTrackedModels = 256
)

// TokenRatios learns, per model, how many characters of a request cost one prompt token. The server's own count of a
// request (its reported prompt tokens) is the only exact answer, and the ratio varies with the content (dense JSON and
// HTML cost more tokens per character than prose), so the agent measures it instead of assuming 4.0. A nil store is
// valid: it reports the default and learns nothing.
type TokenRatios struct {
	mu     sync.Mutex
	ratios map[string]float64
}

func NewTokenRatios() *TokenRatios {
	return &TokenRatios{ratios: make(map[string]float64)}
}

// CharsPerToken is the model's learned ratio, or the default before any observation.
func (r *TokenRatios) CharsPerToken(model string) float64 {
	if r == nil {
		return DefaultCharsPerToken
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.ratios[model]; ok {
		return v
	}
	return DefaultCharsPerToken
}

// Observe records that a request of chars characters cost promptTokens tokens. Observations without both counts are
// ignored and the result is kept inside a plausible range.
func (r *TokenRatios) Observe(model string, chars, promptTokens int) {
	if r == nil || chars <= 0 || promptTokens <= 0 {
		return
	}
	observed := min(max(float64(chars)/float64(promptTokens), minCharsPerToken), maxCharsPerToken)
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, tracked := r.ratios[model]
	switch {
	case tracked:
		r.ratios[model] = prev*(1-ratioBlend) + observed*ratioBlend
	case len(r.ratios) < maxTrackedModels:
		r.ratios[model] = observed
	}
}
