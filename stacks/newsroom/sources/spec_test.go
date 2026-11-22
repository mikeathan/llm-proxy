package sources

import (
	"context"
	"strings"
	"testing"
)

func TestRegistry_OneReaderPerSpecSoTheFloorHolds(t *testing.T) {
	r := &Registry{Fetch: &fakeFetch{}}
	spec := Spec{Kind: KindTelegram, Target: "aipost"}
	if r.Reader(spec) != r.Reader(spec) {
		t.Fatal("asking twice for one spec must give the same reader, or its politeness floor would be lost between calls")
	}
	if r.Reader(spec) == r.Reader(Spec{Kind: KindTelegram, Target: "other_chan"}) {
		t.Error("two channels must not share a reader")
	}
}

func TestBuild_UnknownKindFailsNamingItself(t *testing.T) {
	_, err := Build(Spec{Kind: "rumour", Target: "x"}, &fakeFetch{}).Fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rumour x") {
		t.Fatalf("an unknown kind must fail and name the source once, got %v", err)
	}
}

func TestSpec_CanonicalReducesPastedForms(t *testing.T) {
	got := Spec{Kind: KindTelegram, Target: " https://t.me/s/aipost "}.Canonical()
	if got.Target != "aipost" {
		t.Errorf("target = %q, want aipost", got.Target)
	}
	if err := (Spec{Kind: KindMastodon, Target: "golang"}).Validate(); err == nil {
		t.Error("a Mastodon source without a server must be refused")
	}
	if got := (Spec{Kind: KindFeed, Target: "https://e.example/f"}).Name(); got != "feed https://e.example/f" {
		t.Errorf("name = %q", got)
	}
}
