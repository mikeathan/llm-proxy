package brief

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIssueSectionWithoutStoriesMarshalsEmptyList(t *testing.T) {
	b, err := json.Marshal(IssueSection{Kind: SectionNews, TopicID: 1, Name: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"stories":[]`) {
		t.Fatalf("want an empty list, got %s", b)
	}
}
