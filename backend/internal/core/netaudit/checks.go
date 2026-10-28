package netaudit

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Checks are findings beyond "this port is open": what a short, fixed list of nmap's safe scripts learned about a
// service. The list below is the whole of what a scan may run. Each script was chosen because nmap classes it safe,
// it talks only to the scanned address, and it tries no passwords. The model never names a script; the nmap engine
// asks for exactly CheckScriptNames().
//
// Text a device wrote is never copied into a finding. Each evaluator reads the script's output to decide yes or no
// and writes its own fixed words as the evidence, so a hostile banner cannot reach a report or a chat message.

// Check names, severities and the script each comes from.
const (
	CheckSMBv1             = "smb_v1"
	CheckSMBSigning        = "smb_signing_not_required"
	CheckFTPAnonymous      = "ftp_anonymous"
	CheckVNCNoAuth         = "vnc_no_auth"
	CheckRedisNoAuth       = "redis_no_auth"
	CheckNFSExports        = "nfs_exports"
	CheckCertExpired       = "tls_cert_expired"
	CheckCertExpiring      = "tls_cert_expiring"
	CheckSSHWeakAlgorithms = "ssh_weak_algorithms"
	CheckRDPNoNLA          = "rdp_no_nla"
	CheckSSHWeakHostKey    = "ssh_weak_hostkey"
	CheckTLSWeakKey        = "tls_weak_key"
	CheckTLSWeakSignature  = "tls_weak_signature"
	CheckMemcachedOpen     = "memcached_exposed"
	CheckMongoNoAuth       = "mongodb_no_auth"
	CheckMQTTAnonymous     = "mqtt_anonymous"
	CheckTelnetOpen        = "telnet_open"
	CheckDockerAPI         = "docker_api_exposed"

	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// checkHit is what an evaluator found: the check, how serious it is, and our own words for it.
type checkHit struct {
	Check    string
	Severity string
	Evidence string
}

// checkTable maps an nmap script id to the evaluator of its output. Adding a check is one row here plus its test; the
// nmap engine asks for every script in this table and no other.
var checkTable = map[string]func(output string, now time.Time) []checkHit{
	"smb-protocols":       evalSMBProtocols,
	"smb2-security-mode":  evalSMBSigning,
	"ftp-anon":            evalAnonymousFTP,
	"vnc-info":            evalVNC,
	"redis-info":          evalRedis,
	"nfs-showmount":       evalNFS,
	"ssl-cert":            evalCertificate,
	"ssh2-enum-algos":     evalSSHAlgorithms,
	"rdp-enum-encryption": evalRDP,
	"ssh-hostkey":         evalSSHHostKey,
	"memcached-info":      evalMemcached,
	"mongodb-info":        evalMongo,
	"mqtt-subscribe":      evalMQTT,
}

// CheckScriptNames lists the scripts a checks scan runs, sorted.
func CheckScriptNames() []string {
	names := make([]string, 0, len(checkTable))
	for name := range checkTable {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// clock is the time checks are judged against; tests replace it.
var clock = time.Now

// checkFindings turns the script results of one open port into findings, in script-name order so the same scan always
// gives the same list. A script the table does not know is ignored.
func checkFindings(device string, p Port) []Finding {
	scripts := slices.Clone(p.Scripts)
	slices.SortFunc(scripts, func(a, b Script) int { return strings.Compare(a.ID, b.ID) })
	var out []Finding
	for _, s := range scripts {
		eval, ok := checkTable[s.ID]
		if !ok {
			continue
		}
		for _, hit := range eval(s.Output, clock()) {
			out = append(out, Finding{
				Device: device, Port: p.Number, Proto: p.Proto, Service: serviceLabel(p),
				Check: hit.Check, Severity: hit.Severity, Evidence: hit.Evidence,
			})
		}
	}
	return out
}

func one(check, severity, evidence string) []checkHit {
	return []checkHit{{Check: check, Severity: severity, Evidence: evidence}}
}

func evalSMBProtocols(out string, _ time.Time) []checkHit {
	if strings.Contains(out, "SMBv1") || strings.Contains(out, "NT LM 0.12") {
		return one(CheckSMBv1, SeverityHigh, "SMBv1, an old and unsafe file-sharing version, is enabled")
	}
	return nil
}

func evalSMBSigning(out string, _ time.Time) []checkHit {
	if strings.Contains(out, "not required") {
		return one(CheckSMBSigning, SeverityMedium, "SMB message signing is not required")
	}
	return nil
}

func evalAnonymousFTP(out string, _ time.Time) []checkHit {
	if strings.Contains(out, "Anonymous FTP login allowed") {
		return one(CheckFTPAnonymous, SeverityHigh, "Anyone can sign in to the FTP server without a password")
	}
	return nil
}

var vncNoneType = regexp.MustCompile(`(?mi)^\s*none\b\s*(\(1\))?\s*$`)

func evalVNC(out string, _ time.Time) []checkHit {
	if vncNoneType.MatchString(out) {
		return one(CheckVNCNoAuth, SeverityCritical, "VNC accepts connections without a password")
	}
	return nil
}

var redisVersionLine = regexp.MustCompile(`(?m)^\s*Version\s+\S`)

func evalRedis(out string, _ time.Time) []checkHit {
	if redisVersionLine.MatchString(out) {
		return one(CheckRedisNoAuth, SeverityCritical, "Redis answers without a password")
	}
	return nil
}

var nfsExportLine = regexp.MustCompile(`(?m)^\s*/\S+`)

func evalNFS(out string, _ time.Time) []checkHit {
	if n := len(nfsExportLine.FindAllString(out, -1)); n > 0 {
		return one(CheckNFSExports, SeverityMedium, fmt.Sprintf("%d NFS export(s) can be listed from the network", n))
	}
	return nil
}

var (
	certNotAfter  = regexp.MustCompile(`Not valid after:\s+(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}:\d{2})`)
	certNotBefore = regexp.MustCompile(`Not valid before:\s+(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}:\d{2})`)
	// certSoon is how far ahead an expiry is worth a warning.
	certSoon = 30 * 24 * time.Hour
	// certShortLived is the longest validity that counts as "renewed by the device itself": a certificate issued for
	// this long is always close to its end, so an approaching expiry says nothing.
	certShortLived = 14 * 24 * time.Hour
)

const certTimeLayout = "2006-01-02T15:04:05"

func parseCertTime(re *regexp.Regexp, out string) (time.Time, bool) {
	m := re.FindStringSubmatch(out)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(certTimeLayout, m[1]+"T"+m[2])
	return t, err == nil
}

func evalCertificate(out string, now time.Time) []checkHit {
	hits := certExpiryHits(out, now)
	if bits, ok := certKeyBits(out); ok && bits < minRSAKeyBits {
		hits = append(hits, checkHit{CheckTLSWeakKey, SeverityMedium, fmt.Sprintf("The certificate uses a %d-bit RSA key; %d bits is the minimum today", bits, minRSAKeyBits)})
	}
	if m := certSignature.FindStringSubmatch(out); m != nil && weakSignature(m[1]) {
		hits = append(hits, checkHit{CheckTLSWeakSignature, SeverityLow, "The certificate is signed with an outdated algorithm (SHA-1 or MD5)"})
	}
	return hits
}

func certExpiryHits(out string, now time.Time) []checkHit {
	expires, ok := parseCertTime(certNotAfter, out)
	if !ok {
		return nil
	}
	switch {
	case expires.Before(now):
		return one(CheckCertExpired, SeverityMedium, "The certificate expired on "+expires.Format("2006-01-02"))
	case expires.Before(now.Add(certSoon)) && !shortLived(out, expires):
		return one(CheckCertExpiring, SeverityLow, "The certificate expires on "+expires.Format("2006-01-02"))
	}
	return nil
}

var (
	certKeyType   = regexp.MustCompile(`Public Key type:\s+(\S+)`)
	certKeyBitsRe = regexp.MustCompile(`Public Key bits:\s+(\d+)`)
	certSignature = regexp.MustCompile(`Signature Algorithm:\s+(\S+)`)
)

// minRSAKeyBits is the shortest RSA key still thought safe; shorter ones can be factored with modest resources.
const minRSAKeyBits = 2048

// certKeyBits is the size of an RSA key. Other key types are sized differently (an elliptic-curve key of 256 bits is
// strong), so they are not judged by this number.
func certKeyBits(out string) (int, bool) {
	t := certKeyType.FindStringSubmatch(out)
	b := certKeyBitsRe.FindStringSubmatch(out)
	if t == nil || b == nil || !strings.EqualFold(t[1], "rsa") {
		return 0, false
	}
	bits, err := strconv.Atoi(b[1])
	return bits, err == nil
}

// weakSignature reports whether a signature algorithm name (such as sha1WithRSAEncryption) uses SHA-1 or MD5.
func weakSignature(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "sha1") || strings.HasPrefix(n, "md5")
}

// shortLived reports whether the certificate was issued for only a few days, which devices that renew their own
// certificates do all the time.
func shortLived(out string, expires time.Time) bool {
	issued, ok := parseCertTime(certNotBefore, out)
	return ok && expires.Sub(issued) <= certShortLived
}

// weakSSHAlgorithms are the clearly broken ones; modern servers offer none of them.
var weakSSHAlgorithms = []string{
	"3des-cbc", "arcfour", "arcfour128", "arcfour256", "blowfish-cbc", "cast128-cbc",
	"diffie-hellman-group1-sha1", "hmac-md5", "hmac-md5-96", "ssh-dss",
}

// maxNamedAlgorithms bounds how many weak algorithms the evidence names.
const maxNamedAlgorithms = 3

func evalSSHAlgorithms(out string, _ time.Time) []checkHit {
	var found []string
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if slices.Contains(weakSSHAlgorithms, name) && !slices.Contains(found, name) {
			found = append(found, name)
		}
	}
	if len(found) == 0 {
		return nil
	}
	slices.Sort(found)
	named := found
	if len(named) > maxNamedAlgorithms {
		named = named[:maxNamedAlgorithms]
	}
	evidence := "Weak SSH algorithms are offered: " + strings.Join(named, ", ")
	if len(found) > len(named) {
		evidence += fmt.Sprintf(" and %d more", len(found)-len(named))
	}
	return one(CheckSSHWeakAlgorithms, SeverityLow, evidence)
}

var rdpNLA = regexp.MustCompile(`CredSSP \(NLA\):\s+(\S+)`)

func evalRDP(out string, _ time.Time) []checkHit {
	m := rdpNLA.FindStringSubmatch(out)
	if m != nil && m[1] != "SUCCESS" {
		return one(CheckRDPNoNLA, SeverityMedium, "Remote Desktop does not require network-level authentication")
	}
	return nil
}

var sshHostKeyLine = regexp.MustCompile(`(?m)^\s*(\d+)\s+\S+\s+\((RSA|DSA|ECDSA|ED25519)\)`)

// evalSSHHostKey flags the host keys that can be broken or forged with modest resources: any DSA key (fixed at 1024
// bits) and an RSA key shorter than minRSAKeyBits. A server that also offers a strong key is still flagged, because a
// client can be made to use the weak one.
func evalSSHHostKey(out string, _ time.Time) []checkHit {
	for _, m := range sshHostKeyLine.FindAllStringSubmatch(out, -1) {
		bits, _ := strconv.Atoi(m[1])
		if m[2] == "DSA" || (m[2] == "RSA" && bits < minRSAKeyBits) {
			return one(CheckSSHWeakHostKey, SeverityLow, "The SSH server offers a weak host key (DSA, or RSA shorter than 2048 bits)")
		}
	}
	return nil
}

var memcachedNoAuth = regexp.MustCompile(`(?m)^\s*Authentication:\s+no\b`)

func evalMemcached(out string, _ time.Time) []checkHit {
	if memcachedNoAuth.MatchString(out) {
		return one(CheckMemcachedOpen, SeverityMedium, "Memcached answers anyone on the network, with no password")
	}
	return nil
}

// evalMongo reads only whether the server answered: mongodb-info asks for the build and status, which a server that
// requires a login refuses. None of what it printed is copied.
func evalMongo(out string, _ time.Time) []checkHit {
	if strings.Contains(out, "MongoDB Build info") || strings.Contains(out, "Server status") {
		return one(CheckMongoNoAuth, SeverityHigh, "MongoDB answers anyone on the network without a login")
	}
	return nil
}

// evalMQTT reads only whether an anonymous client could subscribe. The topics and payloads the script prints are what
// other devices published and are never copied.
func evalMQTT(out string, _ time.Time) []checkHit {
	if strings.Contains(out, "Topics and their most recent payloads") {
		return one(CheckMQTTAnonymous, SeverityMedium, "The MQTT broker lets anyone on the network read what devices publish, with no login")
	}
	return nil
}
