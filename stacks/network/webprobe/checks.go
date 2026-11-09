// Package webprobe asks a device's web server for a few fixed pages and says what they show: an administration page that anyone
// on the network can open, a status or metrics page that tells more than it needs to, a file that should not be served, a server
// that still shows its default page. Every request is a plain GET of a path written in this file; the answer is read only to
// decide whether it matches, and a hit carries this file's own words, never the device's text.
package webprobe

import (
	"bytes"
	"strings"
)

// Severities of a check, spelled as the audit spells them.
const (
	SeverityInfo = "info"
	SeverityLow  = "low"
)

// Response is what a device answered to one request: the status, the content type and the start of the body.
type Response struct {
	Status      int
	ContentType string
	Body        []byte
}

// Check is one fixed question about a web page: the paths it asks for, how serious a match is, and the test of an answer. A check
// matches when any of its paths gives an answer the test accepts. Title is the words a finding uses.
type Check struct {
	ID       string
	Title    string
	Severity string
	Paths    []string
	Match    func(Response) bool
}

const statusOK = 200

// has reports whether the body holds every one of the words, with their case as written.
func has(r Response, words ...string) bool {
	for _, w := range words {
		if !bytes.Contains(r.Body, []byte(w)) {
			return false
		}
	}
	return true
}

// hasAny reports whether the body holds at least one of the words, with their case as written.
func hasAny(r Response, words ...string) bool {
	for _, w := range words {
		if bytes.Contains(r.Body, []byte(w)) {
			return true
		}
	}
	return false
}

// okAnd is an answer with status 200 that the test accepts.
func okAnd(test func(Response) bool) func(Response) bool {
	return func(r Response) bool { return r.Status == statusOK && test(r) }
}

// page is the check for a page that shows a given text.
func page(id, title, severity string, paths []string, test func(Response) bool) Check {
	return Check{ID: id, Title: title, Severity: severity, Paths: paths, Match: okAnd(test)}
}

// Checks is the whole list, in the order findings name them. Changing it is a security review (Constitution II.3 item 5 applies in
// spirit: a path may not log in, guess, or send anything but a plain GET).
var Checks = []Check{
	page("asus_router_login", "The login page of an Asus router", SeverityInfo, []string{"/Main_Login.asp"},
		func(r Response) bool {
			return hasAny(r, "<title>ASUS Login</title>", "Sign in with your ASUS router account")
		}),
	page("brother_printer_page", "The status page of a Brother printer", SeverityInfo, []string{"/general/status.html"},
		func(r Response) bool {
			low := Response{Body: bytes.ToLower(r.Body)}
			return hasAny(low, "brother industries", "brother printer", "brother mfc", "brother hl", "brother dcp", "solutions.brother.com")
		}),
	page("fuji_xerox_printer_page", "The status page of a Fuji Xerox printer", SeverityInfo, []string{"/hdstat.htm"},
		func(r Response) bool { return has(r, "Fuji Xerox Co., Ltd") }),
	page("samsung_printer_page", "The web page of a Samsung printer", SeverityInfo, []string{"/sws/index.html"},
		func(r Response) bool { return has(r, "SyncThru Web Service") }),
	page("grafana_login", "The login page of Grafana", SeverityInfo, []string{"/login", "/graph/login"},
		func(r Response) bool { return has(r, "<title>Grafana</title>") }),
	page("home_assistant_page", "The page of Home Assistant", SeverityInfo, []string{"/"},
		func(r Response) bool { return has(r, "<title>Home Assistant</title>") }),
	page("kubernetes_dashboard", "A Kubernetes dashboard", SeverityInfo, []string{"/"},
		func(r Response) bool { return has(r, "Kubernetes Dashboard</title>") }),
	page("openwrt_luci_login", "The login page of an OpenWrt router", SeverityInfo, []string{"/cgi-bin/luci"},
		func(r Response) bool { return has(r, "Authorization Required") }),
	page("openvpn_router_management", "The OpenVPN page of a router", SeverityInfo, []string{"/"},
		func(r Response) bool { return has(r, "Router Management - Server OpenVPN") }),
	page("qnap_login", "The login page of a QNAP storage box", SeverityInfo, []string{"/cgi-bin/", "/cgi-bin/html/login.html"},
		func(r Response) bool { return has(r, "QNAP Turbo NAS</title>") }),
	page("truenas_login", "The login page of TrueNAS", SeverityInfo, []string{"/ui/sessions/signin"},
		func(r Response) bool { return has(r, "'TrueNAS'", "assets/images/TrueNAS") }),
	page("ubiquiti_edgerouter_page", "The page of a Ubiquiti EdgeRouter", SeverityInfo, []string{"/"},
		func(r Response) bool { return has(r, "EdgeOS") }),
	page("unifi_os_login", "The login page of UniFi OS", SeverityInfo, []string{"/login"},
		func(r Response) bool { return hasAny(r, "<title>UniFi OS</title>", "UNIFI_OS_MANIFEST") }),
	page("unifi_security_gateway_page", "The page of a UniFi Security Gateway", SeverityInfo, []string{"/"},
		func(r Response) bool { return has(r, "UniFi Security Gateway") }),
	page("zyxel_router_page", "The configuration page of a Zyxel router", SeverityInfo, []string{"/", "/login"},
		func(r Response) bool { return has(r, "Welcome to the Web-Based Configurator", "zyxel") }),
	page("jellyfin_page", "The page of a Jellyfin media server", SeverityInfo,
		[]string{"/home.html", "/web/home.html", "/index.html", "/web/index.html", "/web/manifest.json"},
		func(r Response) bool {
			return hasAny(r, `name="application-name" content="Jellyfin"`, `class="page homePage libraryPage allLibraryPage backdropPage pageWithAbsoluteTabs withTabs"`, "The Free Software Media System")
		}),
	page("synology_api_list", "Synology DSM lists its programming interface to anyone", SeverityLow,
		[]string{"/webapi/entry.cgi?api=SYNO.API.Info&version=1&method=query&query=all"},
		func(r Response) bool {
			return strings.Contains(r.ContentType, "application/json") && has(r, `success":true`) && hasAny(r, "SYNO.Core.", "SYNO.FileStation.", "SYNO.DSM.")
		}),
	page("apache_status_page", "An Apache status page that anyone can open", SeverityLow, []string{"/server-status", "/server-info"},
		func(r Response) bool {
			return hasAny(r, "Apache Server Status", "Apache Server Information") && has(r, "Server Version")
		}),
	page("nginx_status_page", "An nginx status page that anyone can open", SeverityLow, []string{"/nginx_status", "/nginx-status"},
		func(r Response) bool { return has(r, "Active connections:") }),
	page("metrics_exporter_page", "A metrics page that anyone can open", SeverityLow, []string{"/"},
		func(r Response) bool { return has(r, "Exporter", `<a href="/metrics">Metrics</a>`) }),
	page("openapi_description", "A description of the device's programming interface that anyone can read", SeverityLow, []string{"/openapi.json"},
		func(r Response) bool {
			return (strings.Contains(r.ContentType, "application/json") || strings.Contains(r.ContentType, "application/openapi+json")) && has(r, "openapi", "paths")
		}),
	page("ds_store_file", "A Mac folder-listing file that the web server hands out", SeverityLow, []string{"/.DS_Store"},
		func(r Response) bool { return bytes.HasPrefix(r.Body, []byte("\x00\x00\x00\x01Bud1")) }),
	page("apache_default_page", "A web server that still shows Apache's test page", SeverityLow, []string{"/"},
		func(r Response) bool { return has(r, "<title>Apache HTTP Server Test Page powered by CentOS</title>") }),
	page("nginx_default_page", "A web server that still shows nginx's welcome page", SeverityLow, []string{"/"},
		func(r Response) bool { return has(r, "<title>Welcome to nginx!</title>") }),
}
