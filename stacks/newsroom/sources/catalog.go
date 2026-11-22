package sources

// The built-in list of well-known sources the Briefs topic form offers while the owner types or browses. It is plain data: the form copies the chosen
// entry into the topic ({kind, label, target}), so a topic never depends on this list and a later change here cannot alter what a topic reads.
//
// Every feed address was fetched and parsed as RSS or Atom when the list was written (2026-11-22). Publishers that have no public feed are searches
// (Reuters and the Associated Press stopped offering RSS), which the topic form keeps with its other searches.

// Categories are the groups of the built-in list, in the order the form shows them.
var Categories = []string{"News", "Technology", "Business and finance", "Science and health", "Sport", "Search a publisher"}

// Entry is one well-known source: what the owner sees (Label, Category) and what a topic reads (Kind and Target).
type Entry struct {
	Label    string `json:"label"`
	Category string `json:"category"`
	Kind     Kind   `json:"kind"`
	Target   string `json:"target"`
}

// Spec is the source a topic keeps when the owner picks the entry.
func (e Entry) Spec() Spec { return Spec{Kind: e.Kind, Label: e.Label, Target: e.Target} }

// Catalog returns the built-in list, a fresh copy.
func Catalog() []Entry {
	out := make([]Entry, len(catalog))
	copy(out, catalog)
	return out
}

var catalog = []Entry{
	{"BBC News", "News", KindFeed, "https://feeds.bbci.co.uk/news/rss.xml"},
	{"BBC World", "News", KindFeed, "https://feeds.bbci.co.uk/news/world/rss.xml"},
	{"BBC UK", "News", KindFeed, "https://feeds.bbci.co.uk/news/uk/rss.xml"},
	{"The Guardian World", "News", KindFeed, "https://www.theguardian.com/world/rss"},
	{"The Guardian UK", "News", KindFeed, "https://www.theguardian.com/uk/rss"},
	{"Al Jazeera", "News", KindFeed, "https://www.aljazeera.com/xml/rss/all.xml"},
	{"NPR News", "News", KindFeed, "https://feeds.npr.org/1001/rss.xml"},
	{"Deutsche Welle", "News", KindFeed, "https://rss.dw.com/rdf/rss-en-all"},
	{"France 24", "News", KindFeed, "https://www.france24.com/en/rss"},
	{"Sky News", "News", KindFeed, "https://feeds.skynews.com/feeds/rss/world.xml"},
	{"ABC News Australia", "News", KindFeed, "https://www.abc.net.au/news/feed/51120/rss.xml"},
	{"CBC News", "News", KindFeed, "https://www.cbc.ca/webfeed/rss/rss-topstories"},
	{"The Verge", "Technology", KindFeed, "https://www.theverge.com/rss/index.xml"},
	{"Ars Technica", "Technology", KindFeed, "https://feeds.arstechnica.com/arstechnica/index"},
	{"TechCrunch", "Technology", KindFeed, "https://techcrunch.com/feed/"},
	{"Wired", "Technology", KindFeed, "https://www.wired.com/feed/rss"},
	{"Hacker News front page", "Technology", KindFeed, "https://news.ycombinator.com/rss"},
	{"Engadget", "Technology", KindFeed, "https://www.engadget.com/rss.xml"},
	{"BBC Technology", "Technology", KindFeed, "https://feeds.bbci.co.uk/news/technology/rss.xml"},
	{"MIT Technology Review", "Technology", KindFeed, "https://www.technologyreview.com/feed/"},
	{"The Register", "Technology", KindFeed, "https://www.theregister.com/headlines.atom"},
	{"Krebs on Security", "Technology", KindFeed, "https://krebsonsecurity.com/feed/"},
	{"The Hacker News", "Technology", KindFeed, "https://feeds.feedburner.com/TheHackersNews"},
	{"BBC Business", "Business and finance", KindFeed, "https://feeds.bbci.co.uk/news/business/rss.xml"},
	{"CNBC Top News", "Business and finance", KindFeed, "https://www.cnbc.com/id/100003114/device/rss/rss.html"},
	{"MarketWatch Top Stories", "Business and finance", KindFeed, "https://feeds.content.dowjones.io/public/rss/mw_topstories"},
	{"Financial Times", "Business and finance", KindFeed, "https://www.ft.com/rss/home"},
	{"Guardian Business", "Business and finance", KindFeed, "https://www.theguardian.com/uk/business/rss"},
	{"Investing.com", "Business and finance", KindFeed, "https://www.investing.com/rss/news.rss"},
	{"CoinDesk", "Business and finance", KindFeed, "https://www.coindesk.com/arc/outboundfeeds/rss/"},
	{"Bank of England news", "Business and finance", KindFeed, "https://www.bankofengland.co.uk/rss/news"},
	{"Federal Reserve press releases", "Business and finance", KindFeed, "https://www.federalreserve.gov/feeds/press_all.xml"},
	{"BBC Science", "Science and health", KindFeed, "https://feeds.bbci.co.uk/news/science_and_environment/rss.xml"},
	{"Nature", "Science and health", KindFeed, "https://www.nature.com/nature.rss"},
	{"NASA", "Science and health", KindFeed, "https://www.nasa.gov/news-release/feed/"},
	{"Science Daily", "Science and health", KindFeed, "https://www.sciencedaily.com/rss/all.xml"},
	{"BBC Health", "Science and health", KindFeed, "https://feeds.bbci.co.uk/news/health/rss.xml"},
	{"WHO news", "Science and health", KindFeed, "https://www.who.int/rss-feeds/news-english.xml"},
	{"BBC Sport", "Sport", KindFeed, "https://feeds.bbci.co.uk/sport/rss.xml"},
	{"ESPN", "Sport", KindFeed, "https://www.espn.com/espn/rss/news"},
	{"Reuters", "Search a publisher", KindNews, "site:reuters.com"},
	{"Associated Press", "Search a publisher", KindNews, "site:apnews.com"},
	{"Bloomberg", "Search a publisher", KindNews, "site:bloomberg.com"},
	{"Wall Street Journal", "Search a publisher", KindNews, "site:wsj.com"},
	{"The New York Times", "Search a publisher", KindNews, "site:nytimes.com"},
}
