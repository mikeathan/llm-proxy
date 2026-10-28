package models

import "time"

// VulnRange is one way a product can be affected by a CVE: an exact version, or a range (any end left empty is open).
// With no version fields it means every version. Matching on these is done by the vulnindex package.
type VulnRange struct {
	CVE       string  `json:"c"`
	Score     float64 `json:"s,omitempty"`
	Exact     string  `json:"v,omitempty"`
	StartIncl string  `json:"si,omitempty"`
	StartExcl string  `json:"se,omitempty"`
	EndIncl   string  `json:"ei,omitempty"`
	EndExcl   string  `json:"ee,omitempty"`
}

// VulnSource describes one data source in the index, so a screen can say how old the data is and what it holds. CVEs and
// Entries count what the index holds from the source; the skipped counts are for the last file imported.
type VulnSource struct {
	ImportedAt time.Time `json:"imported_at"`
	// CVEs is how many CVEs the index holds from this source; Entries how many product ranges they produced.
	CVEs    int `json:"cves"`
	Entries int `json:"entries"`
	// The skipped counts are CVEs the last file's importer left out, by reason: no configuration data yet, a rule that needs two
	// vulnerable parts at once, or a CVE NVD has rejected.
	NoConfig int `json:"no_config,omitempty"`
	NeedsAll int `json:"needs_all,omitempty"`
	Rejected int `json:"rejected,omitempty"`
	// Years lists the NVD yearly files held in the index, so a stopped download can resume with the rest. Empty for KEV.
	Years []int `json:"years,omitempty"`
}

// VulnIndex is the local vulnerability data a scan is compared with. Entries is keyed by "part:vendor:product" (part
// a, o or h; vendor and product lower-cased, as vulnindex.Key builds it). Exploited maps a CVE id to the date CISA
// added it to the Known Exploited Vulnerabilities catalogue. No description or other free text from the source files
// is kept.
type VulnIndex struct {
	NVD       VulnSource             `json:"nvd"`
	KEV       VulnSource             `json:"kev"`
	Entries   map[string][]VulnRange `json:"entries,omitempty"`
	Exploited map[string]string      `json:"exploited,omitempty"`
}
