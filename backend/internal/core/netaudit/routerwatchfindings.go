package netaudit

import (
	"fmt"
	"sort"

	"llm-proxy/models"
)

// The router log findings: what the router's log says about failed sign-ins to its web page and Wi-Fi disconnects (Constitution I.8).
// They are hints to review, never proof: any device on the network can forge a log line.
const (
	CheckRouterSignInFailures  = "router_sign_in_failures"
	CheckWifiDisconnectBurst   = "wifi_disconnect_burst"
	CheckWifiDeviceDisconnects = "wifi_device_disconnects"
)

func init() {
	checkTitles[CheckRouterSignInFailures] = "Someone failed to sign in to the router's web page again and again"
	checkTitles[CheckWifiDisconnectBurst] = "Many devices were disconnected from the Wi-Fi at once"
	checkTitles[CheckWifiDeviceDisconnects] = "A device keeps being disconnected from the Wi-Fi"
	remediations[CheckRouterSignInFailures] = "If you were not typing a wrong password, someone on the network is guessing the router's. Change the router's password to a long one, turn off its remote access, and find which device is trying (the router's own log shows the address). A line in the router's log can be made up by any device on the network, so treat this as a hint."
	remediations[CheckWifiDisconnectBurst] = "A burst of disconnects can be a device stuck in a loop, a mesh node restarting, or someone nearby sending forged disconnect frames. If it keeps happening, turn on the router's protected management frames (802.11w) setting and check the Wi-Fi channels. A line in the router's log can be made up by any device on the network, so treat this as a hint."
	remediations[CheckWifiDeviceDisconnects] = "A device that is disconnected over and over may have a weak signal, a power-saving fault or a driver problem. Move it closer to a router or node, update its software, or forget and re-join the Wi-Fi. If it is meant to roam, accept this finding."
	guides[CheckRouterSignInFailures] = FixGuide{
		Steps: []string{
			"Open the router's own admin page and change its password to a long one only you know.",
			"Turn off the router's remote access (from the internet) and its web access over Wi-Fi if you do not need them.",
			"Look at the router's system log for the address that failed to sign in, and find that device.",
		},
		Risk: "A new password has to be given to anything that signs in to the router, such as a backup tool.",
		Undo: "Set the old password again, or accept this finding to say it is understood.",
	}
	guides[CheckWifiDisconnectBurst] = FixGuide{
		Steps: []string{
			"Check whether a mesh node or the router restarted at that time (the router's log shows it).",
			"If not, open the router's wireless settings and turn on protected management frames (802.11w) where it is offered.",
			"Watch the Wi-Fi page of the router for a device you do not know.",
		},
		Risk: "Protected management frames are not understood by some old devices, which may then not connect.",
		Undo: "Turn the setting back to its old value, or accept this finding.",
	}
	guides[CheckWifiDeviceDisconnects] = FixGuide{
		Steps: []string{
			"Move the device closer to a router or node, or away from things that block the signal.",
			"Update the device's software and forget the Wi-Fi network on it, then join again.",
			"If the device moves between the router and a node on purpose, accept this finding.",
		},
		Risk: "Forgetting the network means typing the Wi-Fi password again.",
		Undo: "Join the network again, or take the acceptance back on the Findings tab.",
	}
}

// RouterLogFindings turns a snapshot of the router's log into findings. The bursts are keyed by the router's address, and a device with
// many disconnects by its own address; a device the inventory knows only by hardware address has no finding, because a finding is
// keyed by address. The evidence is fixed words and a number, never the operator's name for the device. Nothing is returned while the
// router's address is unknown.
func RouterLogFindings(s RouterSnapshot, router string, inv models.DeviceInventory) []Finding {
	var out []Finding
	if router != "" && s.SignInBursts > 0 {
		out = append(out, Finding{Check: CheckRouterSignInFailures, Device: router, Severity: SeverityMedium,
			Evidence: fmt.Sprintf("The router's log showed %d burst(s) of %d or more failed sign-ins within %d minutes in the last day.", s.SignInBursts, routerSignInFailures, int(routerSignInWindow.Minutes()))})
	}
	if router != "" && s.DeauthBursts > 0 {
		out = append(out, Finding{Check: CheckWifiDisconnectBurst, Device: router, Severity: SeverityMedium,
			Evidence: fmt.Sprintf("The router's log showed %d burst(s) of %d or more Wi-Fi disconnects within a minute in the last day.", s.DeauthBursts, routerDeauthBurst)})
	}
	ips := map[string]string{}
	for _, d := range inv.Devices {
		if d.IP != "" {
			ips[d.MAC] = d.IP
		}
	}
	macs := make([]string, 0, len(s.Devices))
	for mac := range s.Devices {
		macs = append(macs, mac)
	}
	sort.Strings(macs)
	for _, mac := range macs {
		if ip, ok := ips[mac]; ok && s.Devices[mac] >= routerDeviceDeauths {
			out = append(out, Finding{Check: CheckWifiDeviceDisconnects, Device: ip, Severity: SeverityLow,
				Evidence: fmt.Sprintf("The router's log showed %d Wi-Fi disconnects for it in the last day (a limit of %d).", s.Devices[mac], routerDeviceDeauths)})
		}
	}
	return out
}
