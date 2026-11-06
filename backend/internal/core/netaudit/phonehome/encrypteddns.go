package phonehome

import (
	"slices"
	"strings"
	"time"

	"llm-proxy/models"
)

// encryptedDNSNames are the names of public encrypted-DNS (DNS over HTTPS or TLS) servers, and of the relay that hides a device's
// lookups. A device that asks the router's DNS for one of them is, as a rule, about to send its other lookups there instead, where the
// router, the phone-home watch and the malware list cannot see them. A name matches itself and anything under it (a personal
// `<id>.dns.nextdns.io`), nothing else. The list is fixed in code; use-application-dns.net is left out on purpose: Firefox asks for it to
// find out whether to switch encrypted DNS off, so asking is not a sign of a bypass.
var encryptedDNSNames = []string{
	"dns.google",
	"cloudflare-dns.com",
	"one.one.one.one",
	"dns.quad9.net", "dns9.quad9.net", "dns10.quad9.net", "dns11.quad9.net",
	"doh.opendns.com",
	"dns.nextdns.io",
	"dns.adguard-dns.com", "unfiltered.adguard-dns.com",
	"doh.dns.apple.com",
	"mask.icloud.com", "mask-h2.icloud.com",
}

// IsEncryptedDNSName says whether a (vetted, lower-case) DNS name is one of the fixed encrypted-DNS names or lies under one.
func IsEncryptedDNSName(name string) bool {
	return slices.ContainsFunc(encryptedDNSNames, func(base string) bool {
		return name == base || strings.HasSuffix(name, "."+base)
	})
}

// EncryptedDNS finds, for each device, the encrypted-DNS names it asked for within the last NewFor. It judges every name whatever the
// device's age: a bypass is a bypass in the learning week too. It reads the names and keeps and changes nothing.
func EncryptedDNS(state models.PhoneHome, now time.Time) []Signal {
	return listed(state, func(name string) (string, bool) { return name, IsEncryptedDNSName(name) }, now, EncryptedDNSName)
}
