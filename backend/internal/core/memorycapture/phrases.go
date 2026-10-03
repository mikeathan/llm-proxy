package memorycapture

import "llm-proxy/internal/platform/memory"

// Limits on what an explicit request may capture.
const (
	maxCapturesPerMessage = 5
	minBodyWords          = 2
	maxBodyChars          = 500
	// A short body that leans on a pronoun ("remember that one") means nothing once stored on its own.
	unresolvedMaxWords = 5

	// A message longer than this, with more lines, or shaped like a document (heading, table, list) is pasted material,
	// not the user speaking: it is never scanned.
	maxMessageChars    = 1000
	maxMessageLines    = 8
	minListItemsToSkip = 3
)

// family is a group of phrases that mean the same thing to memory. Phrases are literal, lowercase and use a straight
// apostrophe; "don't" is also matched as "do not" and "dont". Matching is anchored at the start of a sentence, longest
// phrase first, so "remember that" wins over "remember". Only phrases that say "keep this for later" belong here:
// "always"/"never"/"whenever" are ordinary imperatives that often apply to one task only, so they are left to the
// review. To support a new wording add it here; TestPhraseTable checks that every phrase captures a fact on its own.
// Another language is more rows, not more code.
type family struct {
	name     string
	mode     memory.Mode
	triggers []string
}

var families = []family{
	{
		name: "store",
		mode: memory.ModeOnDemand,
		triggers: []string{
			"remember", "remember that", "remember to",
			"don't forget", "don't forget that", "don't forget to", "never forget", "never forget that",
			"keep in mind", "keep in mind that", "bear in mind", "bear in mind that",
			"make a note", "make a note of", "make a note that", "note that", "take note", "take note that",
			"for future reference", "for the record",
			"save this", "store this", "write this down", "log this",
			"memorize", "memorise", "add this to memory", "add to memory", "put this in memory",
		},
	},
	{
		name: "standing",
		mode: memory.ModeAlways,
		triggers: []string{
			"from now on", "going forward", "in the future", "in future", "from here on", "from here on out", "henceforth",
		},
	},
}

// fillers are conversational openers skipped before a phrase is looked for.
var fillers = []string{
	"please", "kindly", "hey", "hi", "hello", "ok", "okay", "also", "and", "so", "btw", "by the way",
	"one more thing", "oh", "yes", "thanks", "thank you", "right", "well",
}

// contractions lists the other spellings generated for a phrase.
var contractions = map[string][]string{
	"don't": {"do not", "dont"},
}

// sensitiveTerms make a whole sentence uncapturable: a stored credential would be injected into later prompts.
var sensitiveTerms = []string{
	"password", "passwd", "passphrase", "secret", "token", "api key", "api-key", "apikey",
	"private key", "ssh key", "credential", "bearer",
}

// unresolvedStarts are words that cannot start a stored fact: a bare reference or a question.
var unresolvedStarts = map[string]bool{
	"it": true, "this": true, "that": true, "these": true, "those": true, "them": true, "they": true,
	"what": true, "when": true, "how": true, "why": true, "who": true, "which": true, "where": true, "if": true,
}
