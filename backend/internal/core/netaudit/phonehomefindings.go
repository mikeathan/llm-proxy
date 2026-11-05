package netaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"llm-proxy/internal/core/netaudit/phonehome"
	"llm-proxy/models"
)

// The phone-home findings: what the router's DNS log says about where a device calls out to (Constitution I.8). They are hints
// to review, never proof: any device on the network can forge a log line, so each is a finding the operator can accept.
const (
	CheckPhoneHomeNewPlace  = "phone_home_new_place"
	CheckPhoneHomeBeaconing = "phone_home_beaconing"
	CheckPhoneHomeAroundDNS = "phone_home_around_dns"
)

// maxPhoneHomeNames is how many names a finding lists; the rest are counted.
const maxPhoneHomeNames = 5

func init() {
	checkTitles[CheckPhoneHomeNewPlace] = "A device asked for a name it had not asked for before"
	checkTitles[CheckPhoneHomeBeaconing] = "A device keeps asking for a new name at a steady beat"
	checkTitles[CheckPhoneHomeAroundDNS] = "A device does not use the router's DNS"
	remediations[CheckPhoneHomeNewPlace] = "Look at the name. If you do not know it, find out which program or device uses it (a vendor's update or cloud service is usual). If it is yours, accept this finding. A name in the router's log can be made up by any device on the network, so treat it as a hint."
	remediations[CheckPhoneHomeBeaconing] = "A program that asks for the same new name every few minutes may be checking in with a server. Find out which one it is on that device. If it is a service you use, accept this finding. A name in the router's log can be made up by any device on the network, so treat it as a hint."
	guides[CheckPhoneHomeNewPlace] = phoneHomeGuide("Find out what uses the name.", "Look the name up with a search engine, or ask the device's maker. A vendor's update or cloud service is the usual answer.")
	guides[CheckPhoneHomeBeaconing] = phoneHomeGuide("Find out which program on the device asks for the name.", "Look at the device's apps and services for one that checks in with a server. A sync, push or update service is the usual answer.")
	guides[CheckPhoneHomeAroundDNS] = FixGuide{
		Steps: []string{
			"Check whether the device is one that uses its own DNS server on purpose (a Chromecast does).",
			"If it is not, open the device's network settings and look for a DNS server it was told to use; set it back to automatic.",
		},
		Risk: "Changing a device's DNS settings can stop it reaching some services until it is set back.",
		Undo: "Put the DNS server back the way it was, or accept this finding to say it is meant to be this way.",
	}
	remediations[CheckPhoneHomeAroundDNS] = "Some devices, such as Chromecasts, use a DNS server of their own on purpose; if this is one, accept this finding. Otherwise look in the device's network settings for a DNS server it was told to use."
}

func phoneHomeGuide(find, how string) FixGuide {
	return FixGuide{
		Steps: []string{find, how, "If it is a service you use, accept this finding. If it is not, remove the program or the device, and change the passwords of any account that was on it."},
		Risk:  "Removing a program or a device can stop something you use. Accepting a finding hides it for ninety days, then it is shown again.",
		Undo:  "Reinstall the program, or take the acceptance back on the Findings tab.",
	}
}

// PhoneHomeFindings turns the watch's signals into findings keyed by the device's address, one per device and kind. A device the
// inventory knows only by hardware address (seen over IPv6 only, or not at all) has no finding, because a finding is keyed by address.
// The evidence is fixed words and the vetted DNS names; the operator's name for the device is never put in it. The Version of a
// finding about names is a digest of the set, so accepting it holds until the set grows.
func PhoneHomeFindings(signals []phonehome.Signal, inv models.DeviceInventory) []Finding {
	ips := map[string]string{}
	for _, d := range inv.Devices {
		if d.IP != "" {
			ips[d.MAC] = d.IP
		}
	}
	var out []Finding
	for _, s := range signals {
		ip, ok := ips[s.MAC]
		if !ok {
			continue
		}
		out = append(out, phoneHomeFinding(s, ip))
	}
	return out
}

func phoneHomeFinding(s phonehome.Signal, ip string) Finding {
	s.Names = slices.Sorted(slices.Values(s.Names))
	f := Finding{Device: ip, Severity: SeverityLow}
	switch s.Kind {
	case phonehome.NewPlace:
		f.Check, f.Evidence, f.Version = CheckPhoneHomeNewPlace, "Asked for a name it did not ask for in its first week: "+listNames(s.Names), namesDigest(s.Names)
	case phonehome.Beaconing:
		f.Check, f.Severity, f.Evidence, f.Version = CheckPhoneHomeBeaconing, SeverityMedium, "Asks for a new name at a steady beat: "+listNames(s.Names), namesDigest(s.Names)
	default:
		f.Check, f.Evidence = CheckPhoneHomeAroundDNS, "Online for a day and has not asked the router's DNS for anything, so it uses a resolver of its own."
	}
	return f
}

// listNames shows the first few names and counts the rest.
func listNames(names []string) string {
	shown := names
	if len(shown) > maxPhoneHomeNames {
		shown = shown[:maxPhoneHomeNames]
	}
	text := strings.Join(shown, ", ")
	if rest := len(names) - len(shown); rest > 0 {
		text += fmt.Sprintf(" and %d more", rest)
	}
	return text
}

func namesDigest(names []string) string {
	sum := sha256.Sum256([]byte(strings.Join(names, "\n")))
	return hex.EncodeToString(sum[:6])
}
