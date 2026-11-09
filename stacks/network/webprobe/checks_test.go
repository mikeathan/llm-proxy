package webprobe

import (
	"strings"
	"testing"
)

// matching is a made-up answer that each check must accept: the words its device really shows.
var matching = map[string]Response{
	"asus_router_login":           {Status: 200, Body: []byte("<html><title>ASUS Login</title>")},
	"brother_printer_page":        {Status: 200, Body: []byte("<html>Brother MFC-L2700DW series</html>")},
	"fuji_xerox_printer_page":     {Status: 200, Body: []byte("<html>Fuji Xerox Co., Ltd")},
	"samsung_printer_page":        {Status: 200, Body: []byte("<title>SyncThru Web Service</title>")},
	"grafana_login":               {Status: 200, Body: []byte("<title>Grafana</title>")},
	"home_assistant_page":         {Status: 200, Body: []byte("<title>Home Assistant</title>")},
	"kubernetes_dashboard":        {Status: 200, Body: []byte("<title>Kubernetes Dashboard</title>")},
	"openwrt_luci_login":          {Status: 200, Body: []byte("Authorization Required")},
	"openvpn_router_management":   {Status: 200, Body: []byte("Router Management - Server OpenVPN")},
	"qnap_login":                  {Status: 200, Body: []byte("<title>QNAP Turbo NAS</title>")},
	"truenas_login":               {Status: 200, Body: []byte("<title>'TrueNAS'</title><img src=assets/images/TrueNAS.svg>")},
	"ubiquiti_edgerouter_page":    {Status: 200, Body: []byte("EdgeOS")},
	"unifi_os_login":              {Status: 200, Body: []byte("<title>UniFi OS</title>")},
	"unifi_security_gateway_page": {Status: 200, Body: []byte("UniFi Security Gateway")},
	"zyxel_router_page":           {Status: 200, Body: []byte("Welcome to the Web-Based Configurator zyxel")},
	"jellyfin_page":               {Status: 200, Body: []byte(`<meta name="application-name" content="Jellyfin">`)},
	"synology_api_list":           {Status: 200, ContentType: "application/json; charset=utf-8", Body: []byte(`{"data":{"SYNO.API.Auth":{}, "SYNO.Core.Desktop":{}},"success":true}`)},
	"apache_status_page":          {Status: 200, Body: []byte("<title>Apache Server Status for host</title> Server Version: Apache/2.4")},
	"nginx_status_page":           {Status: 200, Body: []byte("Active connections: 3 \nserver accepts handled requests")},
	"metrics_exporter_page":       {Status: 200, Body: []byte(`<h1>Node Exporter</h1><a href="/metrics">Metrics</a>`)},
	"openapi_description":         {Status: 200, ContentType: "application/json", Body: []byte(`{"openapi":"3.0.0","paths":{}}`)},
	"ds_store_file":               {Status: 200, ContentType: "application/octet-stream", Body: []byte("\x00\x00\x00\x01Bud1\x00\x00")},
	"apache_default_page":         {Status: 200, Body: []byte("<title>Apache HTTP Server Test Page powered by CentOS</title>")},
	"nginx_default_page":          {Status: 200, Body: []byte("<title>Welcome to nginx!</title>")},
}

func TestChecks_EachOneHasWordsAPathAndAnAnswerItAccepts(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Checks {
		if seen[c.ID] {
			t.Errorf("%s is listed twice", c.ID)
		}
		seen[c.ID] = true
		if c.Title == "" || len(c.Paths) == 0 || c.Match == nil || (c.Severity != SeverityInfo && c.Severity != SeverityLow) {
			t.Errorf("%s is incomplete: %+v", c.ID, c)
		}
		for _, p := range c.Paths {
			if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, " \r\n") {
				t.Errorf("%s: path %q must be absolute and plain", c.ID, p)
			}
		}
		fixture, ok := matching[c.ID]
		if !ok {
			t.Errorf("%s has no matching fixture", c.ID)
			continue
		}
		if !c.Match(fixture) {
			t.Errorf("%s did not accept the words its device shows", c.ID)
		}
	}
	if len(matching) != len(Checks) {
		t.Errorf("%d fixtures for %d checks", len(matching), len(Checks))
	}
}

func TestChecks_NoneAcceptsAnErrorPageOrAnOrdinaryOne(t *testing.T) {
	for _, c := range Checks {
		fixture := matching[c.ID]
		for name, r := range map[string]Response{
			"the same words with a not-found status": {Status: 404, ContentType: fixture.ContentType, Body: fixture.Body},
			"the same words with a redirect status":  {Status: 302, ContentType: fixture.ContentType, Body: fixture.Body},
			"an ordinary page":                       {Status: 200, ContentType: "text/html", Body: []byte("<html><title>My Router</title><body>Hello</body></html>")},
			"an empty answer":                        {Status: 200},
		} {
			if c.Match(r) {
				t.Errorf("%s accepted %s", c.ID, name)
			}
		}
	}
}

func TestChecks_TheDataFilesNeedTheirTypeOrMagic(t *testing.T) {
	byID := map[string]Check{}
	for _, c := range Checks {
		byID[c.ID] = c
	}
	if byID["ds_store_file"].Match(Response{Status: 200, Body: []byte("<html>DSDB Bud1</html>")}) {
		t.Error("a page that mentions the file's magic words is not the file")
	}
	if byID["openapi_description"].Match(Response{Status: 200, ContentType: "text/html", Body: []byte(`openapi paths`)}) {
		t.Error("a web page that says the words is not the description")
	}
	if byID["synology_api_list"].Match(Response{Status: 200, ContentType: "text/html", Body: []byte(`SYNO.Core. success":true`)}) {
		t.Error("the list must come as JSON")
	}
}
