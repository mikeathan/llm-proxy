package assistant

import (
	"regexp"
	"strings"
)

// unfulfilledIntentMax bounds how often a run nudges a model that ends a turn by announcing an action it did not
// take. The count re-arms with every successful tool turn (recoveryState.reset), so a model that keeps making
// progress is nudged again, while one that only announces is let finish after this many.
const unfulfilledIntentMax = 2

var (
	// intentLead is the filler a model puts before an announcement ("Now,", "OK,", "Great,").
	// intentAnnounce is the announcement itself, intentVerb the kind of action; together they match a sentence that
	// says the model is about to do something ("Let me fetch…", "I'll search…", "Next, I need to verify…").
	// The verb list is closed on purpose: "Let me know" and "I'll need your decision" are not announcements.
	intentSentence = regexp.MustCompile(`^(?:(?:and|so|now|next|then|ok|okay|good|great|alright|first|finally|also)[,:!.]?\s+)*` +
		`(?:let me|let's|i'll|i will|i'm going to|i am going to|i need to|i should|i want to)\s+` +
		`(?:(?:also|now|just|first|then|quickly|go ahead and)\s+)*` +
		`(?:fetch|search|check|look|get|read|run|verify|confirm|open|try|find|pull|grab|query|call|use|review|examine|` +
		`visit|retrieve|load|scrape|investigate|explore|dig|dive|compare|cross-check|double-check|validate)\b`)

	// sentenceEnd splits prose into sentences at terminal punctuation followed by space.
	sentenceEnd = regexp.MustCompile(`[.!?]+\s+`)
)

// endsWithUnfulfilledIntent reports whether the last sentence of content announces an action the model has not
// taken ("I found several items. Let me fetch the pricing page to confirm."). A turn that ends like that, with no
// tool call, is mid-task narration: the generation stopped (or the model stopped) before the call. It is not a final
// report, and finalizing on it records an unfinished run as complete. Only the last sentence of the last prose line
// counts, so a report that merely mentions an action, a table row, a past-tense account and "Let me know…" are not
// matched. It is a hint that triggers one bounded nudge, never a verdict that discards the text.
func endsWithUnfulfilledIntent(content string) bool {
	sentence := lastSentence(content)
	if sentence == "" {
		return false
	}
	return intentSentence.MatchString(strings.ToLower(sentence))
}

// lastSentence is the last sentence of the last non-empty prose line of content, without list or emphasis markup.
// A table row or a code fence is not prose and yields "".
func lastSentence(content string) string {
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "|") || strings.HasPrefix(line, "```") {
			return ""
		}
		line = strings.NewReplacer("’", "'", "‘", "'").Replace(line)
		line = strings.Trim(strings.TrimLeft(line, "-*>#• \t"), "*_ \t")
		parts := sentenceEnd.Split(line, -1)
		for j := len(parts) - 1; j >= 0; j-- {
			if part := strings.TrimSpace(parts[j]); part != "" {
				return part
			}
		}
		return ""
	}
	return ""
}

// intentGuardApplies reports whether a text-only turn should be checked for an unfulfilled intent: the run has used a
// tool (a first answer from knowledge is not mid-task) and the bounded nudges are not used up.
func (s *runSession) intentGuardApplies() bool {
	return s.recovery.intentNudges < unfulfilledIntentMax && hasToolResult(s.history)
}
