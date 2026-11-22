package sources

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// The checks the owner's input goes through before a source is saved. Each kind names what it expects in words the form can show.

var hashtagRe = regexp.MustCompile(`^[\p{L}\p{N}_]{1,100}$`)

// MaxQueryRunes bounds a Hacker News search.
const MaxQueryRunes = 200

// CheckChannel says whether name is a Telegram channel name.
func CheckChannel(name string) error {
	if !channelRe.MatchString(name) {
		return errors.New("a Telegram channel name is 4 to 32 letters, digits or underscores, starting with a letter, such as aipost")
	}
	return nil
}

// TagName reduces what an owner types to a bare tag: "#golang" gives "golang".
func TagName(s string) string { return strings.TrimPrefix(strings.TrimSpace(s), "#") }

// CheckTag says whether tag is a Mastodon tag (no #, no spaces).
func CheckTag(tag string) error {
	if !hashtagRe.MatchString(tag) {
		return errors.New("a Mastodon tag is letters, digits or underscores with no spaces and no #, such as golang")
	}
	return nil
}

// InstanceName reduces what an owner types to a bare host: "https://Mastodon.social/" gives "mastodon.social".
func InstanceName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"https://", "http://"} {
		s = strings.TrimPrefix(s, p)
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return s
}

// CheckInstance says whether host is a plain Mastodon server name.
func CheckInstance(host string) error {
	if !instanceRe.MatchString(host) {
		return errors.New("a Mastodon server is a host name such as mastodon.social")
	}
	return nil
}

// CheckQuery says whether q is a usable Hacker News search.
func CheckQuery(q string) error {
	q = strings.TrimSpace(q)
	if q == "" || len([]rune(q)) > MaxQueryRunes {
		return errors.New("a Hacker News search is 1 to 200 characters of words, such as rust async")
	}
	return nil
}

// CheckNewsQuery says whether q is a usable news search: words, or Google operators such as site:reuters.com and when:1d.
func CheckNewsQuery(q string) error {
	q = strings.TrimSpace(q)
	if q == "" || len([]rune(q)) > MaxQueryRunes {
		return errors.New("a news search is 1 to 200 characters of words, such as inflation or site:reuters.com")
	}
	return nil
}

var localeRe = regexp.MustCompile(`^[a-z]{2}-[A-Z]{2}$`)

// CheckLocale says whether s is a language and country such as en-GB, which picks the edition of Google News a search reads.
func CheckLocale(s string) error {
	if !localeRe.MatchString(s) {
		return errors.New("the region is a language and a country such as en-GB or en-US")
	}
	return nil
}

// CheckAddress says whether addr is an https address with a host, for a feed or a page.
func CheckAddress(addr string) error {
	u, err := url.Parse(addr)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return errors.New("the address must be a full https:// address, such as https://example.com/feed")
	}
	return nil
}
