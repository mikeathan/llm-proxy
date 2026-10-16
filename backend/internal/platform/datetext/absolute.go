// Package datetext rewrites relative day references in free text into absolute dates, deterministically and without a
// model, so a fact saved today still says the right thing when it is read next month.
package datetext

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	daysPerWeek = 7
	dateLayout  = "2006-01-02"
)

// phrase matches one relative reference: "this|next|last <weekday>", "in N days|weeks", or today/tomorrow/yesterday, as
// whole words so "within 3 days" and "todayish" are not touched.
var phrase = regexp.MustCompile(`(?i)\b(?:(this|next|last)\s+(sunday|monday|tuesday|wednesday|thursday|friday|saturday)|in\s+(\d{1,3})\s+(days?|weeks?)|(today|tomorrow|yesterday))\b`)

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
	"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday,
}

// Absolutize rewrites relative day references in text into absolute dates as seen from now: today, tomorrow, yesterday,
// this/next/last <weekday>, and in N days/weeks. Each becomes "YYYY-MM-DD (Weekday)". English only; a bare weekday
// ("on Friday") or a vague span ("next week") is ambiguous and left as written.
//
// "this <weekday>" is the first such day on or after today, "next <weekday>" the first strictly after today, and
// "last <weekday>" the most recent strictly before today.
func Absolutize(text string, now time.Time) string {
	return phrase.ReplaceAllStringFunc(text, func(match string) string {
		m := phrase.FindStringSubmatch(match)
		switch {
		case m[2] != "":
			return format(weekdayDate(now, strings.ToLower(m[1]), weekdays[strings.ToLower(m[2])]))
		case m[3] != "":
			n, _ := strconv.Atoi(m[3])
			if strings.HasPrefix(strings.ToLower(m[4]), "week") {
				n *= daysPerWeek
			}
			return format(addDays(now, n))
		}
		switch strings.ToLower(m[5]) {
		case "tomorrow":
			return format(addDays(now, 1))
		case "yesterday":
			return format(addDays(now, -1))
		}
		return format(now)
	})
}

// weekdayDate is the date of the this/next/last occurrence of wd as seen from now.
func weekdayDate(now time.Time, which string, wd time.Weekday) time.Time {
	ahead := (int(wd) - int(now.Weekday()) + daysPerWeek) % daysPerWeek
	switch which {
	case "next":
		if ahead == 0 {
			ahead = daysPerWeek
		}
		return addDays(now, ahead)
	case "last":
		back := (int(now.Weekday()) - int(wd) + daysPerWeek) % daysPerWeek
		if back == 0 {
			back = daysPerWeek
		}
		return addDays(now, -back)
	}
	return addDays(now, ahead)
}

// addDays moves by calendar days in now's own zone, so a daylight-saving change cannot shift the date.
func addDays(now time.Time, days int) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day()+days, 0, 0, 0, 0, now.Location())
}

func format(t time.Time) string {
	return t.Format(dateLayout) + " (" + t.Weekday().String() + ")"
}
