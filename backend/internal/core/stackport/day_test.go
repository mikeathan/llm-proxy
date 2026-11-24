package stackport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

func athens(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Athens")
	if err != nil {
		t.Skip("no tz database")
	}
	return loc
}

func dayOf(t *testing.T, open string) Day {
	loc := athens(t)
	return Day{
		Read: func(context.Context) (json.RawMessage, error) {
			return json.RawMessage(`{"open":[` + open + `],"recent":[]}`), nil
		},
		Zone: func() *time.Location { return loc },
		// 2026-11-24 09:00 in Athens (UTC+2).
		Now: func() time.Time { return time.Date(2026, 11, 24, 7, 0, 0, 0, time.UTC) },
	}
}

const reminders = `
 {"id":1,"message":"Call the \n dentist","fire_at":"2026-11-24T14:00:00+02:00","status":"scheduled","connector":"telegram","channel":"telegram","timezone":"Europe/Athens"},
 {"id":2,"message":"Pay the bill","fire_at":"2026-11-24T08:30:00+02:00","status":"scheduled","cron":"30 8 * * *"},
 {"id":3,"message":"Paused one","fire_at":"2026-11-24T10:00:00+02:00","status":"paused"},
 {"id":4,"message":"Tomorrow morning","fire_at":"2026-11-25T07:15:00+02:00","status":"scheduled"},
 {"id":5,"message":"Next week","fire_at":"2026-12-01T07:15:00+02:00","status":"scheduled"},
 {"id":6,"message":"Late tonight","fire_at":"2026-11-24T23:59:00+02:00","status":"scheduled"}`

func TestDay_TodayHoldsTheOwnersWordsAndATimeAndNothingElse(t *testing.T) {
	got, err := dayOf(t, reminders).Items(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	want := []stacksdk.DayItem{{Clock: "08:30", Title: "Pay the bill", Repeats: true}, {Clock: "14:00", Title: "Call the dentist"}, {Clock: "23:59", Title: "Late tonight"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("today: %+v", got)
	}
	all, _ := json.Marshal(got)
	for _, leak := range []string{"telegram", "Europe/Athens", "scheduled", "connector", "\"id\""} {
		if strings.Contains(string(all), leak) {
			t.Errorf("%q crossed the port: %s", leak, all)
		}
	}
}

func TestDay_TomorrowIsTheNextDayInTheOwnersZone(t *testing.T) {
	got, err := dayOf(t, reminders).Items(context.Background(), true)
	if err != nil || len(got) != 1 || got[0].Clock != "07:15" || got[0].Title != "Tomorrow morning" {
		t.Fatalf("tomorrow: %+v, %v", got, err)
	}
}

// A day in the owner's zone is not a day in UTC: 00:30 on the 25th in Athens is still the 24th in UTC.
func TestDay_TheDayIsDrawnInTheOwnersZone(t *testing.T) {
	got, _ := dayOf(t, `{"message":"just after midnight","fire_at":"2026-11-25T00:30:00+02:00","status":"scheduled"}`).Items(context.Background(), true)
	if len(got) != 1 || got[0].Clock != "00:30" {
		t.Fatalf("tomorrow: %+v", got)
	}
}

func TestDay_IsBoundedAndEarliestFirst(t *testing.T) {
	var rows []string
	for i := 0; i < 30; i++ {
		// Later reminders are listed first, so the order of the answer is not the order of the day.
		rows = append(rows, fmt.Sprintf(`{"message":"r","fire_at":"2026-11-24T10:%02d:00+02:00","status":"scheduled"}`, 59-i))
	}
	got, err := dayOf(t, strings.Join(rows, ",")).Items(context.Background(), false)
	if err != nil || len(got) != stacksdk.MaxDayItems {
		t.Fatalf("%d items, %v", len(got), err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Clock > got[i].Clock {
			t.Fatalf("not earliest first: %+v", got)
		}
	}
}

func TestDay_ALongTitleIsOneClippedLine(t *testing.T) {
	long := strings.Repeat("word ", 100)
	got, _ := dayOf(t, `{"message":"`+long+`\n\nsecond line","fire_at":"2026-11-24T10:00:00+02:00","status":"scheduled"}`).Items(context.Background(), false)
	if len(got) != 1 || strings.Contains(got[0].Title, "\n") || len([]rune(got[0].Title)) > dayTitle {
		t.Fatalf("title: %q", got[0].Title)
	}
}

// Reminders off is not an empty day: the stack must be able to tell the two apart.
func TestDay_AnUnreadableStackIsUnavailableNeverAnEmptyDay(t *testing.T) {
	for name, d := range map[string]Day{
		"no reader": {},
		"stack off": {Read: func(context.Context) (json.RawMessage, error) { return nil, errors.New("the stack answered 503") }},
		"bad body":  {Read: func(context.Context) (json.RawMessage, error) { return json.RawMessage("not json"), nil }},
	} {
		items, err := d.Items(context.Background(), false)
		if items != nil || stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
			t.Errorf("%s: %v, %v", name, items, err)
		}
		if strings.Contains(err.Error(), "503") {
			t.Errorf("%s: the stack's own error text crossed: %v", name, err)
		}
	}
}

func TestDay_AQuietDayIsAnEmptyListNotAnError(t *testing.T) {
	got, err := dayOf(t, ``).Items(context.Background(), false)
	if err != nil || len(got) != 0 {
		t.Fatalf("%+v, %v", got, err)
	}
}
