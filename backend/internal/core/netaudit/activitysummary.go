package netaudit

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"llm-proxy/models"
)

// Bounds on the summary: how many rows go to the model and how long its answer may be.
const (
	SummaryMaxRows  = 60
	SummaryMaxChars = 1200
)

// summaryWords is the one sentence each kind of row becomes. These are the program's own words; the prompt has no others.
var summaryWords = map[string]string{
	ActivityNewDevice:      "a new device appeared",
	ActivityAddressChanged: "an address moved to a different device",
	ActivityAlert:          "an alert was raised",
	ActivityPortsChanged:   "ports opened or closed",
	ActivityAudit:          "an audit finished",
	ActivityRelated:        "a new device appeared within an hour before a decoy service was touched",
	ActivityRouterBlocks:   "the router refused connection attempts from the internet",
}

var thinkBlock = regexp.MustCompile(`(?s)<think>.*?</think>`)

// SummaryPrompt is what the model is shown: one line per row, made only of the kind's fixed words, how long ago it was, and
// numbers. A device's name or address, and the text of an alert, are never in it, so nothing a device or a log said can
// instruct the model. A row of a kind it does not know is left out. It is empty when no row is left, and then no model is
// called.
func SummaryPrompt(rows []ActivityRow, now time.Time) string {
	var lines []string
	for _, r := range rows {
		words, ok := summaryWords[r.Kind]
		if !ok {
			continue
		}
		if len(lines) == SummaryMaxRows {
			break
		}
		lines = append(lines, "- "+ago(now.Sub(r.At))+": "+words+summaryDetail(r))
	}
	return strings.Join(lines, "\n")
}

func summaryDetail(r ActivityRow) string {
	var out string
	switch r.Kind {
	case ActivityAlert:
		if models.NetworkAlertKind(r.AlertKind).Valid() {
			out += " (" + r.AlertKind + ")"
		}
		if r.Count > 1 {
			out += fmt.Sprintf(" x%d", r.Count)
		}
	case ActivityPortsChanged:
		out += fmt.Sprintf(" (%d opened, %d closed)", r.Opened, r.Closed)
	case ActivityAudit:
		out += fmt.Sprintf(" (%d new)", r.Opened)
	case ActivityRouterBlocks:
		out += fmt.Sprintf(" (%d)", r.Count)
	}
	return out
}

func ago(d time.Duration) string {
	hours := max(int(d/time.Hour), 0)
	if hours >= 24 {
		return plural(hours/24, "day", "days") + " ago"
	}
	return plural(hours, "hour", "hours") + " ago"
}

// CleanSummary makes a model's reply fit to show: the thinking is dropped, the text trimmed and cut at SummaryMaxChars on a
// character boundary. A reply with nothing left is empty, so the caller can say the model gave no answer.
func CleanSummary(reply string) string {
	text := strings.TrimSpace(thinkBlock.ReplaceAllString(reply, ""))
	if utf8.RuneCountInString(text) > SummaryMaxChars {
		text = string([]rune(text)[:SummaryMaxChars])
	}
	return strings.TrimSpace(text)
}
