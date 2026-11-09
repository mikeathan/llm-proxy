package netaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"llm-proxy/internal/core/netaudit/phonehome"
	"llm-proxy/models"
)

// The phone-home findings: what the router's DNS log says about where a device calls out to (Constitution I.8). They are hints
// to review, never proof: any device on the network can forge a log line, so each is a finding the operator can accept.
const (
	CheckPhoneHomeNewPlace  = "phone_home_new_place"
	CheckPhoneHomeBeaconing = "phone_home_beaconing"
	CheckPhoneHomeKnownBad  = "phone_home_known_bad"
	// CheckPhoneHomeEncryptedDNS is a device that asked for the name of a public encrypted-DNS server.
	CheckPhoneHomeEncryptedDNS = "phone_home_encrypted_dns"
)

// maxPhoneHomeNames is how many names a finding lists; the rest are counted.
const maxPhoneHomeNames = 5

func init() {
	checkTitles[CheckPhoneHomeNewPlace] = "A device asked for a name it had not asked for before"
	checkTitles[CheckPhoneHomeBeaconing] = "A device keeps asking for a new name at a steady beat"
	checkTitles[CheckPhoneHomeKnownBad] = "A device asked for a name on a malware list"
	checkTitles[CheckPhoneHomeEncryptedDNS] = "A device may be sending its lookups around the router"
	remediations[CheckPhoneHomeEncryptedDNS] = "The device asked for the name of a public encrypted-DNS server, which a browser or a phone does when it is set to use one. If it does, the router's DNS, the phone-home watch and the malware list cannot see that device's lookups. If you set it up on purpose, accept this finding. Otherwise turn off \"secure DNS\" or \"private DNS\" in the device or browser. A name in the router's log can be made up by any device on the network, so treat it as a hint."
	guides[CheckPhoneHomeEncryptedDNS] = FixGuide{
		Steps: []string{
			"Find the device at the address the finding names. Firefox, Chrome, Edge, Android's Private DNS and Apple's iCloud Private Relay can all send lookups to a public server on their own.",
			"If you want the router to see this device's lookups, turn off secure DNS in the browser's privacy settings, or Private DNS in the phone's network settings.",
			"If you chose it on purpose, accept this finding: it stays quiet until the device asks for a different encrypted-DNS name.",
		},
		Risk: "Turning off encrypted DNS lets the network, and your internet provider, see which names the device looks up.",
		Undo: "Turn secure DNS back on in the device or browser, or take the acceptance back on the Findings tab.",
	}
	remediations[CheckPhoneHomeNewPlace] = "Look at the name. If you do not know it, find out which program or device uses it (a vendor's update or cloud service is usual). If it is yours, accept this finding. A name in the router's log can be made up by any device on the network, so treat it as a hint."
	remediations[CheckPhoneHomeBeaconing] = "A program that asks for the same new name every few minutes may be checking in with a server. Find out which one it is on that device. If it is a service you use, accept this finding. A name in the router's log can be made up by any device on the network, so treat it as a hint."
	guides[CheckPhoneHomeNewPlace] = phoneHomeGuide("Find out what uses the name.", "Look the name up with a search engine, or ask the device's maker. A vendor's update or cloud service is the usual answer.")
	guides[CheckPhoneHomeBeaconing] = phoneHomeGuide("Find out which program on the device asks for the name.", "Look at the device's apps and services for one that checks in with a server. A sync, push or update service is the usual answer.")
	remediations[CheckPhoneHomeKnownBad] = "The name is on abuse.ch's list of hosts that serve malware. Find out which program or device on that address asked for it: a browser tab or an app on a phone or computer is the usual source. Scan that device for malware, remove what you do not know, and change the passwords of accounts used on it. A name in the router's log can be made up by any device on the network, so treat it as a strong hint, not proof."
	guides[CheckPhoneHomeKnownBad] = FixGuide{
		Steps: []string{
			"Find the device at the address the finding names, and what was in use on it when the name was asked for.",
			"Run a malware scan on it, or check its apps and browser add-ons for ones you do not know, and remove them.",
			"Change the passwords of accounts that were used on that device, starting with email.",
		},
		Risk: "Removing a program can stop something you use. Treat a hit as a strong hint: the list is of hosts that serve malware, and a name can also be asked for by a harmless page that links to one.",
		Undo: "Reinstall what you removed, or accept this finding to say it is understood.",
	}
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
		if f, ok := phoneHomeFinding(s, ip); ok {
			out = append(out, f)
		}
	}
	return out
}

// phoneHomeFinding is the finding for one signal. A device that asks the router for nothing is not one: the router's log cannot tell a
// device that goes around it from a phone with private DNS, a browser with secure DNS or a device the router never logs, so it is a note on
// the device's row (SilentDevices), not a problem to accept.
func phoneHomeFinding(s phonehome.Signal, ip string) (Finding, bool) {
	s.Names = slices.Sorted(slices.Values(s.Names))
	f := Finding{Device: ip, Severity: SeverityLow}
	switch s.Kind {
	case phonehome.NewPlace:
		f.Check, f.Evidence, f.Version = CheckPhoneHomeNewPlace, "Asked for a name it did not ask for in its first week: "+listNames(s.Names), namesDigest(s.Names)
	case phonehome.Beaconing:
		f.Check, f.Severity, f.Evidence, f.Version = CheckPhoneHomeBeaconing, SeverityMedium, "Asks for a new name at a steady beat: "+listNames(s.Names), namesDigest(s.Names)
	case phonehome.EncryptedDNSName:
		f.Check, f.Evidence, f.Version = CheckPhoneHomeEncryptedDNS, "Asked for the name of a public encrypted-DNS server, so its other lookups may no longer go through the router: "+listNames(s.Names), namesDigest(s.Names)
	case phonehome.KnownBadName:
		f.Check, f.Severity, f.Evidence, f.Version = CheckPhoneHomeKnownBad, SeverityHigh, "Asked for a name on abuse.ch's list of hosts that serve malware: "+listNames(s.Names), namesDigest(s.Names)
	default:
		return Finding{}, false
	}
	return f, true
}

// SilentDevices are the hardware addresses of the devices that are online and have asked the router's DNS for nothing for a day, for the
// note on a device's row. Nothing is read from a device and nothing is raised: it is a fact the screen shows, with the reasons it can be
// harmless. Without a watch (or while it is off) it is empty.
func SilentDevices(src PhoneHomeSource, inv models.DeviceInventory, now time.Time) map[string]bool {
	out := map[string]bool{}
	if src == nil {
		return out
	}
	for _, s := range src.Signals(PhoneHomeDevices(inv), now) {
		if s.Kind == phonehome.AroundDNS {
			out[s.MAC] = true
		}
	}
	return out
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

// PhoneHomeConfig reads and changes whether the phone-home watch is on. It is off until the operator turns it on; it opens no
// socket of its own (the router log's socket carries the lines), so switching it needs no passphrase.
type PhoneHomeConfig struct{ store PolicyStore }

func NewPhoneHomeConfig(store PolicyStore) *PhoneHomeConfig { return &PhoneHomeConfig{store: store} }

// Enabled reports whether the watch is on.
func (c *PhoneHomeConfig) Enabled() bool { return c.store.Get().PhoneHomeEnabled }

// Set switches the watch on or off.
func (c *PhoneHomeConfig) Set(enabled bool) error {
	if err := c.store.Update(func(cfg *models.NetauditConfig) error {
		cfg.PhoneHomeEnabled = enabled
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the phone-home setting: %w", err)
	}
	return nil
}

// PhoneHomeSource is what the nightly report reads: the signals of the watch for the devices of the inventory. It is empty while the
// watch is off.
type PhoneHomeSource interface {
	Signals(devices []phonehome.InventoryDevice, now time.Time) []phonehome.Signal
}

// PhoneHomeDevices are the devices the "goes around your DNS" signal may judge: not the router itself (it never asks itself) and not a
// device with a private hardware address, which changes and would be reported over and over.
func PhoneHomeDevices(inv models.DeviceInventory) []phonehome.InventoryDevice {
	var out []phonehome.InventoryDevice
	for _, d := range inv.Devices {
		if d.Randomized || (inv.GatewayMAC != "" && d.MAC == inv.GatewayMAC) {
			continue
		}
		out = append(out, phonehome.InventoryDevice{MAC: d.MAC, FirstSeen: d.FirstSeen, LastSeen: d.LastSeen})
	}
	return out
}
