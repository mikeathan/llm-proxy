package brief

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

func TestTopicNormalize(t *testing.T) {
	ok, err := Topic{Name: " World ", Queries: []string{" Ukraine ", "Ukraine", "site:reuters.com when:1d"}, Region: "en-GB", Ignore: []string{" Horoscope ", "horoscope", ""},
		Sources: []sources.Spec{{Kind: sources.KindFeed, Target: "https://feeds.bbci.co.uk/news/world/rss.xml"}}}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if ok.Name != "World" || !reflect.DeepEqual(ok.Queries, []string{"Ukraine", "site:reuters.com when:1d"}) || !reflect.DeepEqual(ok.Ignore, []string{"horoscope"}) {
		t.Errorf("a topic is trimmed and de-duplicated: %+v", ok)
	}
	for name, bad := range map[string]Topic{
		"no name":          {Queries: []string{"x"}},
		"nothing to read":  {Name: "x"},
		"bad region":       {Name: "x", Queries: []string{"x"}, Region: "uk"},
		"telegram":         {Name: "x", Sources: []sources.Spec{{Kind: sources.KindTelegram, Target: "aipost"}}},
		"page":             {Name: "x", Sources: []sources.Spec{{Kind: sources.KindPage, Target: "https://e.example/"}}},
		"bad feed address": {Name: "x", Sources: []sources.Spec{{Kind: sources.KindFeed, Target: "http://e.example/f"}}},
		"too many queries": {Name: "x", Queries: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}},
		"overlong ignore":  {Name: "x", Queries: []string{"x"}, Ignore: []string{string(make([]rune, MaxWordRunes+1))}},
	} {
		if _, err := bad.Normalize(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s must be refused as invalid, got %v", name, err)
		}
	}
}

func TestTopicReadersNameTheTopicAndStartWithTheSearches(t *testing.T) {
	topic := Topic{Name: "World", Queries: []string{"Ukraine"}, Region: "en-GB", Sources: []sources.Spec{{Kind: sources.KindFeed, Target: "https://e.example/f"}}}
	got := topic.Readers()
	if len(got) != 2 || got[0].Kind != sources.KindNews || got[0].Instance != "en-GB" || got[0].Name() != "World: Ukraine" || got[1].Kind != sources.KindFeed {
		t.Errorf("readers = %+v", got)
	}
}

func exists(ids ...int64) func(int64) bool {
	return func(id int64) bool {
		for _, i := range ids {
			if i == id {
				return true
			}
		}
		return false
	}
}

func TestEditionNormalize(t *testing.T) {
	got, err := Edition{Name: "Morning", ReadyBy: "07:00", Sections: []Section{{Kind: SectionNews, TopicID: 1}}}.Normalize(exists(1))
	if err != nil {
		t.Fatal(err)
	}
	if got.CollectEvery != DefaultEvery || got.CollectFor != DefaultCollectFor || !reflect.DeepEqual(got.Days, []int{0, 1, 2, 3, 4, 5, 6}) {
		t.Errorf("defaults: every day, collect every 2h for 12h: %+v", got)
	}
	for name, bad := range map[string]Edition{
		"no name":           {ReadyBy: "07:00", Sections: []Section{{Kind: SectionNews, TopicID: 1}}},
		"bad time":          {Name: "x", ReadyBy: "7am", Sections: []Section{{Kind: SectionNews, TopicID: 1}}},
		"hour 24":           {Name: "x", ReadyBy: "24:00", Sections: []Section{{Kind: SectionNews, TopicID: 1}}},
		"no sections":       {Name: "x", ReadyBy: "07:00"},
		"unknown topic":     {Name: "x", ReadyBy: "07:00", Sections: []Section{{Kind: SectionNews, TopicID: 9}}},
		"unknown kind":      {Name: "x", ReadyBy: "07:00", Sections: []Section{{Kind: "weather", TopicID: 1}}},
		"bad day":           {Name: "x", ReadyBy: "07:00", Days: []int{7}, Sections: []Section{{Kind: SectionNews, TopicID: 1}}},
		"collect too often": {Name: "x", ReadyBy: "07:00", CollectEvery: time.Minute, Sections: []Section{{Kind: SectionNews, TopicID: 1}}},
		"collect too long":  {Name: "x", ReadyBy: "07:00", CollectFor: 48 * time.Hour, Sections: []Section{{Kind: SectionNews, TopicID: 1}}},
	} {
		if _, err := bad.Normalize(exists(1)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s must be refused as invalid, got %v", name, err)
		}
	}
}

func TestEditionCronMeansReadyByOnTheChosenDays(t *testing.T) {
	e := Edition{ReadyBy: "07:05", Days: []int{1, 2, 3, 4, 5}}
	if got := e.Cron(); got != "5 7 * * 1,2,3,4,5" {
		t.Errorf("cron = %q", got)
	}
	days, _ := cleanDays([]int{5, 1, 1, 0})
	if !reflect.DeepEqual(days, []int{0, 1, 5}) {
		t.Errorf("days are sorted and de-duplicated: %v", days)
	}
}
