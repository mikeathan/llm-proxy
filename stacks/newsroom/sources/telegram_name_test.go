package sources

import "testing"

func TestChannelName(t *testing.T) {
	for in, want := range map[string]string{
		"aipost": "aipost", "@aipost": "aipost", " t.me/aipost ": "aipost",
		"https://t.me/s/aipost": "aipost", "https://t.me/aipost/123?x=1": "aipost",
	} {
		if got := ChannelName(in); got != want {
			t.Errorf("ChannelName(%q) = %q, want %q", in, got, want)
		}
	}
}
