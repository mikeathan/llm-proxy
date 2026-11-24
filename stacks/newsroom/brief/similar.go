package brief

import (
	"slices"
	"strings"
)

// The wording rules that let two reports of one story count as one: an outlet under its regional or web spelling, and a headline reworded a little.

// outletSuffixes are the endings that name a regional edition or the web address, not another outlet.
var outletSuffixes = []string{" uk", " us", " usa", " news uk", ".com", ".co.uk"}

// OutletKey reduces an outlet's name to its identity: lower case, without a leading "the" and without a regional or web ending, so "The Guardian",
// "The Guardian UK" and "Guardian.com" are one outlet.
func OutletKey(name string) string {
	k := strings.Join(strings.Fields(strings.ToLower(name)), " ")
	k = strings.TrimPrefix(k, "the ")
	for changed := true; changed; {
		changed = false
		for _, s := range outletSuffixes {
			if t, ok := strings.CutSuffix(k, s); ok && t != "" {
				k, changed = t, true
			}
		}
	}
	return k
}

// stopWords are the words too common in headlines to say two of them are about one story.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "was": true, "with": true, "from": true, "that": true, "this": true, "after": true, "amid": true,
	"over": true, "into": true, "his": true, "her": true, "its": true, "has": true, "have": true, "will": true, "says": true, "new": true, "what": true,
}

// headlineWords are the words of a title key that say what the story is about.
func headlineWords(key string) []string {
	var out []string
	for _, w := range strings.Fields(key) {
		if len(w) > 2 && !stopWords[w] && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

// minSharedWords and similarShare say when two headlines are one story reworded: at least this many words in common, and at least this share of the words
// of the two together. Different angles on one event share few words and are left to the model (the prompts ask for one story per event).
const (
	minSharedWords = 4
	similarShare   = 0.6
)

// SimilarKeys says whether two title keys are the same story worded a little differently (an added "- analysis", a changed verb).
func SimilarKeys(a, b string) bool {
	wa, wb := headlineWords(a), headlineWords(b)
	shared := 0
	for _, w := range wa {
		if slices.Contains(wb, w) {
			shared++
		}
	}
	union := len(wa) + len(wb) - shared
	return shared >= minSharedWords && union > 0 && float64(shared)/float64(union) >= similarShare
}

// fold adds another report of the same story to a candidate: its reports (once each, up to the bound) and the text a headline-only story lacked.
func (c *candidate) fold(o candidate) {
	c.also = append(c.also, o.item.Key)
	if c.item.Text == "" {
		c.item.Text = o.item.Text
	}
	c.item.Reports = slices.Clone(c.item.Reports)
	for _, r := range o.item.Reports {
		same := func(x Report) bool { return OutletKey(x.Outlet()) == OutletKey(r.Outlet()) && x.Link == r.Link }
		if len(c.item.Reports) < maxReports && !slices.ContainsFunc(c.item.Reports, same) {
			c.item.Reports = append(c.item.Reports, r)
		}
	}
}

// clustered merges candidates that are the same story reworded into the best-ranked one of each. The candidates must already be ranked.
func clustered(ranked []candidate) []candidate {
	var out []candidate
	for _, c := range ranked {
		i := slices.IndexFunc(out, func(k candidate) bool { return SimilarKeys(k.item.Key, c.item.Key) })
		if i < 0 {
			out = append(out, c)
			continue
		}
		out[i].fold(c)
	}
	return out
}
