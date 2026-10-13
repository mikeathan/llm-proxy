package reminders

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// This file turns the text of a /remind command — "<when> <message>" — into a
// reminder time and message without a model: the words that describe the time are
// read from the left and the first word that is not one starts the message. It is
// the one place that does calendar arithmetic for reminders; the assistant tool
// and the UI preview call it too.

const (
	defaultHour     = 9
	morningHour     = 9
	afternoonHour   = 14
	eveningHour     = 18
	tonightHour     = 20
	noonHour        = 12
	hoursPerHalfDay = 12
	hoursPerDay     = 24
	minutesPerHour  = 60
	daysPerWeek     = 7
	cronAnyDay      = "*"
	cronWeekdays    = "1-5"
	cronWeekend     = "0,6"
	clockExample    = `try "tomorrow 9am", "sunday 8pm", "18 oct 20:00" or "in 2h"`
)

var (
	isoDateRe  = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
	slashRe    = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})(?:/(\d{2}|\d{4}))?$`)
	dayRe      = regexp.MustCompile(`^(\d{1,2})(?:st|nd|rd|th)?$`)
	yearRe     = regexp.MustCompile(`^\d{4}$`)
	clockRe    = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?(am|pm)?$`)
	durationRe = regexp.MustCompile(`^(\d+)([a-z]*)$`)
)

var months = map[string]time.Month{
	"jan": time.January, "january": time.January, "feb": time.February, "february": time.February,
	"mar": time.March, "march": time.March, "apr": time.April, "april": time.April, "may": time.May,
	"jun": time.June, "june": time.June, "jul": time.July, "july": time.July, "aug": time.August,
	"august": time.August, "sep": time.September, "sept": time.September, "september": time.September,
	"oct": time.October, "october": time.October, "nov": time.November, "november": time.November,
	"dec": time.December, "december": time.December,
}

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "sun": time.Sunday, "monday": time.Monday, "mon": time.Monday,
	"tuesday": time.Tuesday, "tue": time.Tuesday, "tues": time.Tuesday, "wednesday": time.Wednesday,
	"wed": time.Wednesday, "thursday": time.Thursday, "thu": time.Thursday, "thur": time.Thursday,
	"thurs": time.Thursday, "friday": time.Friday, "fri": time.Friday, "saturday": time.Saturday,
	"sat": time.Saturday,
}

var durationUnits = map[string]time.Duration{
	"m": time.Minute, "min": time.Minute, "mins": time.Minute, "minute": time.Minute, "minutes": time.Minute,
	"h": time.Hour, "hr": time.Hour, "hrs": time.Hour, "hour": time.Hour, "hours": time.Hour,
	"d": hoursPerDay * time.Hour, "day": hoursPerDay * time.Hour, "days": hoursPerDay * time.Hour,
	"w": daysPerWeek * hoursPerDay * time.Hour, "week": daysPerWeek * hoursPerDay * time.Hour,
	"weeks": daysPerWeek * hoursPerDay * time.Hour,
}

// partsOfDay are the words that stand for a default hour.
var partsOfDay = map[string]clock{
	"morning": {morningHour, 0}, "afternoon": {afternoonHour, 0}, "evening": {eveningHour, 0},
	"noon": {noonHour, 0}, "midnight": {0, 0},
}

// ParseError is a /remind command the parser could not read; its text says what
// was wrong and is meant to be shown to the user as it stands.
type ParseError struct{ msg string }

func (e *ParseError) Error() string { return e.msg }

func unreadable(format string, args ...any) error {
	return &ParseError{msg: fmt.Sprintf(format, args...)}
}

// Parsed is a /remind command read into a reminder. At is zero for a recurring
// reminder, which starts at its next cron occurrence.
type Parsed struct {
	At      time.Time
	Cron    string
	Message string
}

type clock struct{ hour, min int }

// ParseCommand reads "<when> <message>". when is a day (today, tomorrow, tonight,
// a weekday, "18th of october", "18 oct", "oct 18", "18/10", "2026-10-18"), an
// optional time ("8pm", "20:30", "noon", "evening"), or "in 2h" / "in 3 days", or
// "every day|weekday|<weekday> [time]". A day without a time is 09:00. Times are in loc.
func ParseCommand(text string, now time.Time, loc *time.Location) (Parsed, error) {
	s := newWordScanner(text)
	if s.done() {
		return Parsed{}, unreadable("when should I remind you? %s", clockExample)
	}
	now = now.In(loc)
	var p Parsed
	var err error
	if s.accept("every") {
		p.Cron, err = parseRecurring(s)
	} else {
		p.At, err = parseOneOff(s, now, loc)
	}
	if err != nil {
		return Parsed{}, err
	}
	p.Message = s.rest()
	if p.Message == "" {
		return Parsed{}, unreadable("what should the reminder say? Put the message after the time")
	}
	return p, nil
}

func parseOneOff(s *wordScanner, now time.Time, loc *time.Location) (time.Time, error) {
	if s.accept("in") {
		return parseRelative(s, now)
	}
	day, dayClock, dayFound, err := parseDay(s, now, loc)
	if err != nil {
		return time.Time{}, err
	}
	if !dayFound {
		return parseTimeOnly(s, now, loc)
	}
	c, found, err := parseClock(s)
	if err != nil {
		return time.Time{}, err
	}
	if !found {
		c = dayClock
	}
	return time.Date(day.Year(), day.Month(), day.Day(), c.hour, c.min, 0, 0, loc), nil
}

func parseRelative(s *wordScanner, now time.Time) (time.Time, error) {
	start := s.pos - 1
	w, ok := s.next()
	m := durationRe.FindStringSubmatch(w)
	if !ok || m == nil {
		return time.Time{}, unreadable(`couldn't read "%s" as a duration; %s`, s.since(start), clockExample)
	}
	amount, _ := strconv.Atoi(m[1])
	unit := m[2]
	if unit == "" {
		unit, _ = s.next()
	}
	d, known := durationUnits[strings.ToLower(unit)]
	if !known || amount <= 0 {
		return time.Time{}, unreadable(`"%s" isn't a duration; %s`, s.since(start), clockExample)
	}
	return addCalendar(now, amount, d), nil
}

// addCalendar adds amount units of d; whole days and weeks keep the wall-clock time
// across a clock change instead of drifting an hour.
func addCalendar(now time.Time, amount int, d time.Duration) time.Time {
	if d >= hoursPerDay*time.Hour {
		return now.AddDate(0, 0, amount*int(d/(hoursPerDay*time.Hour)))
	}
	return now.Add(time.Duration(amount) * d)
}

// parseDay reads the day. The clock is the hour the day word implies ("tonight");
// found is false, with nothing consumed, when the text does not start with a day.
func parseDay(s *wordScanner, now time.Time, loc *time.Location) (day time.Time, c clock, found bool, err error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if day, c, ok := parseDayWord(s, today); ok {
		return day, c, true, nil
	}
	date, found, err := parseDate(s, today, loc)
	if err != nil || !found {
		return time.Time{}, clock{}, false, err
	}
	return date, clock{defaultHour, 0}, true, nil
}

// parseTimeOnly reads a time with no day ("8pm call mum"): today if it is still
// ahead, otherwise the same time tomorrow.
func parseTimeOnly(s *wordScanner, now time.Time, loc *time.Location) (time.Time, error) {
	c, found, err := parseClock(s)
	if err != nil {
		return time.Time{}, err
	}
	if !found {
		return time.Time{}, unreadable(`couldn't read "%s" as a day or time; %s`, s.upcoming(), clockExample)
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), c.hour, c.min, 0, 0, loc)
	if !at.After(now) {
		at = at.AddDate(0, 0, 1)
	}
	return at, nil
}

// parseDayWord reads today, tomorrow, tonight or [next|this] <weekday>; ok is
// false, with nothing consumed, for anything else.
func parseDayWord(s *wordScanner, today time.Time) (time.Time, clock, bool) {
	save := s.pos
	w, _ := s.peek()
	switch w {
	case "today":
		s.advance()
		return today, clock{defaultHour, 0}, true
	case "tomorrow":
		s.advance()
		return today.AddDate(0, 0, 1), clock{defaultHour, 0}, true
	case "tonight":
		s.advance()
		return today, clock{tonightHour, 0}, true
	case "next", "this":
		s.advance()
		w, _ = s.peek()
	}
	if wd, isDay := weekdays[w]; isDay {
		s.advance()
		return nextWeekday(today, wd), clock{defaultHour, 0}, true
	}
	s.pos = save
	return time.Time{}, clock{}, false
}

// nextWeekday is the next occurrence of wd strictly after today.
func nextWeekday(today time.Time, wd time.Weekday) time.Time {
	ahead := (int(wd) - int(today.Weekday()) + daysPerWeek) % daysPerWeek
	if ahead == 0 {
		ahead = daysPerWeek
	}
	return today.AddDate(0, 0, ahead)
}

// parseDate reads a calendar date; found is false (nothing consumed) when the
// next words are not one.
func parseDate(s *wordScanner, today time.Time, loc *time.Location) (time.Time, bool, error) {
	w, ok := s.peek()
	if !ok {
		return time.Time{}, false, nil
	}
	if m := isoDateRe.FindStringSubmatch(w); m != nil {
		s.advance()
		return buildDate(today, atoi(m[1]), time.Month(atoi(m[2])), atoi(m[3]), w, loc)
	}
	if m := slashRe.FindStringSubmatch(w); m != nil {
		s.advance()
		return buildDate(today, expandYear(m[3]), time.Month(atoi(m[2])), atoi(m[1]), w, loc)
	}
	if m := dayRe.FindStringSubmatch(w); m != nil {
		return parseDayThenMonth(s, today, atoi(m[1]), loc)
	}
	if month, isMonth := months[w]; isMonth {
		return parseMonthThenDay(s, today, month, loc)
	}
	return time.Time{}, false, nil
}

// parseDayThenMonth reads "18[th] [of] oct [year]"; it consumes nothing when no month follows.
func parseDayThenMonth(s *wordScanner, today time.Time, day int, loc *time.Location) (time.Time, bool, error) {
	save := s.pos
	s.advance()
	s.accept("of")
	w, ok := s.peek()
	month, isMonth := months[w]
	if !ok || !isMonth {
		s.pos = save
		return time.Time{}, false, nil
	}
	s.advance()
	label := fmt.Sprintf("%d %s", day, w)
	return buildDate(today, s.optionalYear(), month, day, label, loc)
}

// parseMonthThenDay reads "oct 18[th] [year]"; it consumes nothing when no day follows.
func parseMonthThenDay(s *wordScanner, today time.Time, month time.Month, loc *time.Location) (time.Time, bool, error) {
	save := s.pos
	name, _ := s.next()
	w, ok := s.peek()
	m := dayRe.FindStringSubmatch(w)
	if !ok || m == nil {
		s.pos = save
		return time.Time{}, false, nil
	}
	s.advance()
	return buildDate(today, s.optionalYear(), month, atoi(m[1]), fmt.Sprintf("%s %s", name, w), loc)
}

// buildDate makes the date, refusing one the calendar doesn't have. A zero year
// means the next such date from today.
func buildDate(today time.Time, year int, month time.Month, day int, label string, loc *time.Location) (time.Time, bool, error) {
	explicit := year != 0
	if !explicit {
		year = today.Year()
	}
	d := time.Date(year, month, day, 0, 0, 0, 0, loc)
	if d.Month() != month || d.Day() != day {
		return time.Time{}, false, unreadable(`"%s" isn't a date on the calendar`, label)
	}
	if !explicit && d.Before(today) {
		d = d.AddDate(1, 0, 0)
	}
	return d, true, nil
}

// parseClock reads an optional "[at] 8pm | 20:30 | noon | evening". A bare number
// is not a time, so "5 things" stays in the message.
func parseClock(s *wordScanner) (clock, bool, error) {
	save := s.pos
	atWord := s.accept("at")
	w, ok := s.peek()
	if !ok {
		s.pos = save
		return clock{}, false, nil
	}
	if part, isPart := partsOfDay[w]; isPart {
		s.advance()
		return part, true, nil
	}
	m := clockRe.FindStringSubmatch(w)
	if m == nil || (m[2] == "" && m[3] == "") {
		if atWord {
			s.pos = save
		}
		return clock{}, false, nil
	}
	s.advance()
	c, err := buildClock(m, w)
	return c, err == nil, err
}

func buildClock(m []string, label string) (clock, error) {
	hour, min := atoi(m[1]), atoi(m[2])
	if min >= minutesPerHour {
		return clock{}, unreadable(`"%s" isn't a valid time`, label)
	}
	switch m[3] {
	case "":
		if hour >= hoursPerDay {
			return clock{}, unreadable(`"%s" isn't a valid time`, label)
		}
	case "am", "pm":
		if hour < 1 || hour > hoursPerHalfDay {
			return clock{}, unreadable(`"%s" isn't a valid time`, label)
		}
		hour %= hoursPerHalfDay
		if m[3] == "pm" {
			hour += hoursPerHalfDay
		}
	}
	return clock{hour, min}, nil
}

// parseRecurring reads "day|weekday|<weekday> [time]" after "every" into a cron spec.
func parseRecurring(s *wordScanner) (string, error) {
	w, ok := s.next()
	dow, known := recurringDays(strings.ToLower(w))
	if !ok || !known {
		return "", unreadable(`couldn't read "%s" after "every"; try "every day 9am", "every weekday 8:30am" or "every monday"`, w)
	}
	c, found, err := parseClock(s)
	if err != nil {
		return "", err
	}
	if !found {
		c = clock{defaultHour, 0}
	}
	return fmt.Sprintf("%d %d * * %s", c.min, c.hour, dow), nil
}

func recurringDays(w string) (string, bool) {
	switch w {
	case "day", "daily":
		return cronAnyDay, true
	case "weekday", "weekdays":
		return cronWeekdays, true
	case "weekend", "weekends":
		return cronWeekend, true
	}
	if wd, ok := weekdays[strings.TrimSuffix(w, "s")]; ok {
		return strconv.Itoa(int(wd)), true
	}
	wd, ok := weekdays[w]
	return strconv.Itoa(int(wd)), ok
}

func atoi(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}

// expandYear turns "26" into 2026; "" stays 0 (no year given).
func expandYear(v string) int {
	n := atoi(v)
	if n > 0 && n < 100 {
		return 2000 + n
	}
	return n
}

// wordScanner walks the words of a command, remembering where each ended so the rest
// of the text can be returned exactly as typed.
type wordScanner struct {
	src    string
	words  []scanWord
	pos    int
	closed bool // a word ended in ":" — the time part is over
}

type scanWord struct {
	text string // lower-cased, trailing ":" removed
	raw  string
	end  int // offset in src just after the word
	stop bool
}

func newWordScanner(src string) *wordScanner {
	s := &wordScanner{src: src}
	i := 0
	for i < len(src) {
		for i < len(src) && isSpace(src[i]) {
			i++
		}
		start := i
		for i < len(src) && !isSpace(src[i]) {
			i++
		}
		if start == i {
			break
		}
		raw := src[start:i]
		stop := len(raw) > 1 && strings.HasSuffix(raw, ":")
		s.words = append(s.words, scanWord{text: strings.ToLower(strings.TrimSuffix(raw, ":")), raw: raw, end: i, stop: stop})
	}
	return s
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

func (s *wordScanner) done() bool { return s.pos >= len(s.words) || s.closed }

func (s *wordScanner) peek() (string, bool) {
	if s.done() {
		return "", false
	}
	return s.words[s.pos].text, true
}

func (s *wordScanner) advance() {
	if s.done() {
		return
	}
	s.closed = s.words[s.pos].stop
	s.pos++
}

func (s *wordScanner) next() (string, bool) {
	w, ok := s.peek()
	s.advance()
	return w, ok
}

// accept consumes the next word when it is w.
func (s *wordScanner) accept(w string) bool {
	if got, ok := s.peek(); ok && got == w {
		s.advance()
		return true
	}
	return false
}

// optionalYear consumes a four-digit year if one follows.
func (s *wordScanner) optionalYear() int {
	if w, ok := s.peek(); ok && yearRe.MatchString(w) {
		s.advance()
		return atoi(w)
	}
	return 0
}

// since is the raw text of the words from word index start up to the current one.
func (s *wordScanner) since(start int) string {
	var parts []string
	for _, w := range s.words[start:min(s.pos, len(s.words))] {
		parts = append(parts, w.raw)
	}
	return strings.Join(parts, " ")
}

// upcoming is the next word, for error messages.
func (s *wordScanner) upcoming() string {
	if s.pos < len(s.words) {
		return s.words[s.pos].raw
	}
	return ""
}

// rest is the text after the last consumed word, as typed.
func (s *wordScanner) rest() string {
	if s.pos == 0 {
		return strings.TrimSpace(s.src)
	}
	return strings.TrimSpace(s.src[s.words[s.pos-1].end:])
}
