package netaudit

import "testing"

func TestConfirmedText_LeavesOutAGuessAndKeepsWhatWasConfirmed(t *testing.T) {
	for in, want := range map[string]string{
		"afs3-fileserver (guess)":           "",
		"Cloudflare public DNS (guess)":     "",
		"OpenSSH 9.6p1 Ubuntu 3ubuntu13":    "OpenSSH 9.6p1 Ubuntu 3ubuntu13",
		"ssh":                               "ssh",
		"":                                  "",
		"a guess (guess) of something else": "a guess (guess) of something else",
	} {
		if got := ConfirmedText(in); got != want {
			t.Errorf("ConfirmedText(%q) = %q, want %q", in, got, want)
		}
	}
	// The marker the stored findings carry is the one removed: if serviceLabel changes its mark this fails.
	if got := ConfirmedText(serviceLabel(Port{Service: "domain", Guessed: true})); got != "" {
		t.Errorf("a guessed port's label must be left out, got %q", got)
	}
	if got := ConfirmedText(joinVersion(Port{Product: "Cloudflare public DNS", ProductGuessed: true})); got != "" {
		t.Errorf("a guessed product must be left out, got %q", got)
	}
}
