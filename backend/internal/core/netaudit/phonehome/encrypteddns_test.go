package phonehome

import (
	"slices"
	"testing"
	"time"
)

func TestIsEncryptedDNSName_MatchesThePublicResolversAndTheirSubdomainsOnly(t *testing.T) {
	for _, name := range []string{
		"dns.google", "cloudflare-dns.com", "mozilla.cloudflare-dns.com", "one.one.one.one", "dns.quad9.net", "dns10.quad9.net",
		"doh.opendns.com", "abc123.dns.nextdns.io", "mask.icloud.com", "mask-h2.icloud.com", "dns.adguard-dns.com", "doh.dns.apple.com",
	} {
		if !IsEncryptedDNSName(name) {
			t.Errorf("%q is a public encrypted-DNS name and was not matched", name)
		}
	}
	for _, name := range []string{
		"use-application-dns.net", // Firefox asks for it to learn whether to switch DoH off: not a sign of a bypass
		"dns.google.example.com", "notdns.google", "fakecloudflare-dns.com", "google.com", "www.icloud.com", "quad9.net", "",
	} {
		if IsEncryptedDNSName(name) {
			t.Errorf("%q was matched as an encrypted-DNS name", name)
		}
	}
}

func TestEncryptedDNS_ReportsRecentNamesWhateverTheDevicesAge(t *testing.T) {
	w, _ := newWatch(t)
	w.Observe(macA, q("dns.google"), day0) // asked in the learning week: still a bypass
	w.Observe(macA, q("mozilla.cloudflare-dns.com"), day0)
	w.Observe(macA, q("fine.example.com"), day0)
	w.Observe(macB, q("dns.quad9.net"), day0.Add(-30*24*time.Hour))
	got := EncryptedDNS(w.Snapshot(), day0.Add(time.Hour))
	if len(got) != 1 || got[0].Kind != EncryptedDNSName || got[0].MAC != macA || !slices.Equal(got[0].Names, []string{"dns.google", "mozilla.cloudflare-dns.com"}) {
		t.Fatalf("signals = %+v, want one for the first device naming the two resolvers it asked for", got)
	}
	if got := EncryptedDNS(w.Snapshot(), day0.Add(9*24*time.Hour)); len(got) != 0 {
		t.Fatalf("signals = %+v nine days later: an old lookup is no longer news", got)
	}
}

func TestJudge_OneNameThatIsNewAndOnAListIsOneSignal(t *testing.T) {
	w, _ := newWatch(t)
	start := day0
	w.Observe(macA, q("warmup.example.com"), start)
	later := start.Add(LearnFor + time.Hour)
	w.Observe(macA, q("dns.google"), later)
	w.Observe(macA, q("bad.example"), later)
	w.Observe(macA, q("fresh.example"), later)
	bad := func(n string) (string, bool) { return n, n == "bad.example" }
	got := Judge(w.Snapshot(), nil, bad, later.Add(time.Minute))
	kinds := map[Kind][]string{}
	for _, s := range got {
		kinds[s.Kind] = s.Names
	}
	if !slices.Equal(kinds[NewPlace], []string{"fresh.example"}) || !slices.Equal(kinds[EncryptedDNSName], []string{"dns.google"}) || !slices.Equal(kinds[KnownBadName], []string{"bad.example"}) {
		t.Fatalf("signals = %+v, want each name in one signal only", got)
	}
}

func TestEncryptedDNS_KeepsNothingNewAndChangesNothing(t *testing.T) {
	w, st := newWatch(t)
	w.Observe(macA, q("dns.google"), day0)
	before := w.Snapshot()
	_ = EncryptedDNS(before, day0)
	if after := w.Snapshot(); len(after.Devices) != len(before.Devices) || len(after.Told) != len(before.Told) {
		t.Fatal("judging a name changed the record")
	}
	_ = st
}
