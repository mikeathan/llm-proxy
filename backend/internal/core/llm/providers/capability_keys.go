// capability_keys.go — data-driven capability extraction via key-alias lists
// (§2.10 #2).  One walker parses every provider's JSON shape: OpenRouter's
// top_provider.max_completion_tokens, OpenAI's max_completion_tokens, NVIDIA's
// limits.context, llama.cpp's meta.n_ctx, LM Studio's
// loaded_instances[].config.context_length.  Adding a provider shape = adding
// an alias to a slice, never a parser branch.
package providers

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ContextLengthKeys are the alias keys tried for a published context window.
// Ordered: more specific/preferred keys first (n_ctx before n_ctx_train), and
// "context" covers NVIDIA's limits.context.
var ContextLengthKeys = []string{
	"n_ctx",
	"context_length",
	"context_window",
	"context_size",
	"max_context_length",
	"max_position_embeddings",
	"max_model_len",
	"max_input_tokens",
	"max_sequence_length",
	"n_ctx_train",
	"ctx_size",
	"context",
}

// OutputCapKeys are the alias keys tried for a per-model output cap.
var OutputCapKeys = []string{
	"max_completion_tokens",
	"max_output_tokens",
	"max_tokens",
}

// maxReasonableCapability bounds a coerced capability value to a sane int
// range — providers publish token counts well under this ceiling, so any value
// above it is treated as unknown (protects against malformed payloads).
const maxReasonableCapability = 1 << 30

// extractCapability walks a decoded JSON value (any nesting) and returns the
// first int found under one of the alias keys.  The walk is depth-first and
// alias-ordered: at every map, keys are tried in alias-list order so a
// preferred alias (n_ctx before n_ctx_train) wins over siblings.
func extractCapability(v any, keys []string) int {
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		seen[strings.ToLower(k)] = true
	}
	return extractFirstInt(v, keys, seen)
}

// extractFirstInt performs the depth-first, alias-ordered walk.  At each map
// node we first try the alias keys in order against direct children, then
// recurse into non-matching children.
func extractFirstInt(v any, ordered []string, seen map[string]bool) int {
	switch node := v.(type) {
	case map[string]any:
		for _, k := range ordered {
			if child, ok := node[k]; ok {
				if n := coerceInt(child); n > 0 {
					return n
				}
			}
		}
		for k, child := range node {
			if seen[strings.ToLower(k)] {
				continue
			}
			if n := extractFirstInt(child, ordered, seen); n > 0 {
				return n
			}
		}
	case []any:
		for _, child := range node {
			if n := extractFirstInt(child, ordered, seen); n > 0 {
				return n
			}
		}
	}
	return 0
}

// coerceInt converts a JSON number/string into a bounded positive int, or 0
// when unrepresentable.
func coerceInt(v any) int {
	switch n := v.(type) {
	case float64:
		return boundedCap(int(n))
	case int:
		return boundedCap(n)
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0
		}
		parsed, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		return boundedCap(parsed)
	case json.Number:
		parsed, err := strconv.Atoi(n.String())
		if err != nil {
			return 0
		}
		return boundedCap(parsed)
	default:
		return 0
	}
}

func boundedCap(n int) int {
	if n <= 0 || n > maxReasonableCapability {
		return 0
	}
	return n
}
