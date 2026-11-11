package footprint

import (
	"time"

	"llm-proxy/models"
)

// Learns is what each outside service is told by a look, in words the switch shows. It is fixed text the program owns, so the
// operator reads what leaves the computer before turning a service on.
var learns = map[string]string{
	ServiceCerts:    "The public certificate log (crt.sh) learns each domain you watch.",
	ServiceExpiry:   "The domain's registry server, and IANA's public list of registries, learn each domain you watch.",
	ServiceMail:     "The DNS-over-HTTPS resolver (Cloudflare) learns each domain you watch.",
	ServiceBreaches: "The breach service (Have I Been Pwned) learns only the first six characters of a one-way hash of each address, with your own key. It needs a key whose plan includes the private search.",
}

// ServiceView is one outside service as the Footprint page shows it.
type ServiceView struct {
	ID       string `json:"id"`
	On       bool   `json:"on"`
	Learns   string `json:"learns"`
	NeedsKey bool   `json:"needs_key,omitempty"`
}

// ResultView is the latest answer of one check on one watch, with the typed facts and the fixed reason sentence.
type ResultView struct {
	Check      string    `json:"check"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	ReasonText string    `json:"reason_text,omitempty"`
	At         time.Time `json:"at"`
	LookedAt   time.Time `json:"looked_at,omitzero"`
	NewCerts   int       `json:"new_certs,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitzero"`
	SPF        string    `json:"spf,omitempty"`
	DMARC      string    `json:"dmarc,omitempty"`
	Breaches   int       `json:"breaches,omitempty"`
	// BreachNames are the plain short names that passed the check; Breaches can be larger.
	BreachNames []string `json:"breach_names,omitempty"`
	// Stale is true when the last good look is older than the watch's schedule allows: the findings of that check are then not renewed.
	Stale bool `json:"stale,omitempty"`
}

// WatchView is one watched domain or address. Name is the domain, or the masked address; the full address is never sent to the page.
type WatchView struct {
	Label     string       `json:"label"`
	Name      string       `json:"name"`
	Kind      string       `json:"kind"`
	Frequency string       `json:"frequency"`
	Results   []ResultView `json:"results"`
}

// View is the Footprint page.
type View struct {
	Watches    []WatchView   `json:"watches"`
	Services   []ServiceView `json:"services"`
	KeySaved   bool          `json:"key_saved"`
	MaxDomains int           `json:"max_domains"`
	MaxEmails  int           `json:"max_emails"`
}

// ViewOf builds the page from settings and stored results. It asks nothing outside.
func ViewOf(cfg models.FootprintConfig, results []models.FootprintResult, keySaved bool, now time.Time) View {
	by := map[string][]models.FootprintResult{}
	for _, r := range results {
		by[r.Watch] = append(by[r.Watch], r)
	}
	v := View{Watches: []WatchView{}, KeySaved: keySaved, MaxDomains: MaxDomains, MaxEmails: MaxEmails, Services: []ServiceView{
		{ID: ServiceCerts, On: cfg.CertificatesOn, Learns: learns[ServiceCerts]},
		{ID: ServiceExpiry, On: cfg.ExpiryOn, Learns: learns[ServiceExpiry]},
		{ID: ServiceMail, On: cfg.MailRecordsOn, Learns: learns[ServiceMail]},
		{ID: ServiceBreaches, On: cfg.BreachesOn, Learns: learns[ServiceBreaches], NeedsKey: true},
	}}
	for _, d := range cfg.Domains {
		label := models.DomainFindingPrefix + d.Name
		v.Watches = append(v.Watches, WatchView{Label: label, Name: d.Name, Kind: "domain", Frequency: d.Frequency, Results: resultViews(by[label], d.Frequency, now)})
	}
	for _, e := range cfg.Emails {
		label := models.EmailFindingPrefix + e.ID
		v.Watches = append(v.Watches, WatchView{Label: label, Name: Mask(e.Address), Kind: "email", Frequency: e.Frequency, Results: resultViews(by[label], e.Frequency, now)})
	}
	return v
}

func resultViews(rs []models.FootprintResult, frequency string, now time.Time) []ResultView {
	out := make([]ResultView, 0, len(rs))
	for _, r := range rs {
		rv := ResultView{Check: r.Check, Status: r.Status, Reason: r.Reason, At: r.At, LookedAt: r.LookedAt, NewCerts: r.NewCerts, ExpiresAt: r.ExpiresAt, SPF: r.SPF, DMARC: r.DMARC, Breaches: len(r.Breaches),
			Stale: !r.LookedAt.IsZero() && now.Sub(r.LookedAt) > freshFor(frequency)}
		if r.Status != models.FootprintLooked {
			rv.ReasonText = ReasonText(r.Reason)
		}
		for _, b := range r.Breaches {
			if b.Name != "" {
				rv.BreachNames = append(rv.BreachNames, b.Name)
			}
		}
		out = append(out, rv)
	}
	return out
}
