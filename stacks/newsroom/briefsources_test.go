package newsroom

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

func TestBriefSourceRoutes_ListTheBuiltInSourcesThenAddAndRemoveOneOfTheOwners(t *testing.T) {
	s := startBriefs(t, "", &rssFetch{}, time.Hour)
	var list []sourceEntry
	resp, err := call(t, s, "GET", RouteBriefSources, "")
	if err != nil || json.Unmarshal(resp.Body, &list) != nil || len(list) != len(sources.Catalog()) || !list[0].Builtin {
		t.Fatalf("list: %s %v", resp.Body, err)
	}
	resp, err = call(t, s, "POST", RouteBriefSources, `{"label":"My blog","kind":"feed","target":"https://blog.example/feed.xml"}`)
	var added sourceEntry
	if err != nil || resp.Status != http.StatusCreated || json.Unmarshal(resp.Body, &added) != nil || added.ID == 0 || added.Builtin || added.Category != "My sources" {
		t.Fatalf("add: %s %v", resp.Body, err)
	}
	resp, _ = call(t, s, "GET", RouteBriefSources, "")
	_ = json.Unmarshal(resp.Body, &list)
	if last := list[len(list)-1]; last.ID != added.ID || last.Label != "My blog" {
		t.Fatalf("the new source comes last: %+v", last)
	}
	if resp, err = call(t, s, "DELETE", fmt.Sprintf("%ssources/%d", RouteBriefs, added.ID), ""); err != nil || resp.Status != http.StatusNoContent {
		t.Fatalf("delete: %+v %v", resp, err)
	}
	if _, err = call(t, s, "DELETE", fmt.Sprintf("%ssources/%d", RouteBriefs, added.ID), ""); err == nil {
		t.Error("removing it twice is an error")
	}
}

func TestBriefRoutes_DeleteAllForgetsTopicsEditionsAndOwnSources(t *testing.T) {
	s := startBriefs(t, "", &rssFetch{}, time.Hour)
	if _, err := call(t, s, "POST", RouteBriefSources, `{"label":"My blog","kind":"feed","target":"https://blog.example/feed.xml"}`); err != nil {
		t.Fatal(err)
	}
	if resp, err := call(t, s, "DELETE", RouteBriefAll, ""); err != nil || resp.Status != http.StatusNoContent {
		t.Fatalf("delete all: %+v %v", resp, err)
	}
	var list []sourceEntry
	resp, _ := call(t, s, "GET", RouteBriefSources, "")
	if err := json.Unmarshal(resp.Body, &list); err != nil || len(list) != len(sources.Catalog()) {
		t.Fatalf("only the built-in sources remain: %s %v", resp.Body, err)
	}
	for _, route := range []string{RouteBriefTopics, RouteBriefEditions} {
		if resp, _ := call(t, s, "GET", route, ""); strings.TrimSpace(string(resp.Body)) != "[]" {
			t.Errorf("%s should be empty: %s", route, resp.Body)
		}
	}
}

func TestBriefSourceRoutes_RefuseABadSourceInPlainWords(t *testing.T) {
	s := startBriefs(t, "", &rssFetch{}, time.Hour)
	for _, body := range []string{`not json`, `{"label":"x","kind":"feed","target":"nope"}`, `{"label":"","kind":"feed","target":"https://blog.example/feed.xml"}`} {
		if _, err := call(t, s, "POST", RouteBriefSources, body); err == nil || strings.TrimSpace(err.Error()) == "" {
			t.Errorf("%s: expected a refusal, got %v", body, err)
		}
	}
}
