package nuclei

// ReviewedTag is the release of the nuclei-templates repository the list below was reviewed at (2026-11-05). A template is fetched
// from exactly this tag and only runs if its file still has the checksum below.
const ReviewedTag = "v10.5.1"

// Reviewed is the list of templates that may run, approved by the owner on 2026-11-05 under Constitution II.3. Every one is an HTTP
// template of severity info or low that sends GET or HEAD requests to paths under the target's own base address, with no raw request,
// payload, fuzzing, credential, callback or redirect to another host (the filter is recorded in the plan
// `stack-modules-and-new-detection` section 6). A change to this list is a security review: a new template, or a new checksum for
// an updated one, is added only by a person who has read it.
var Reviewed = []Template{
	{ID: "ds-store-file", Path: "http/exposures/files/ds-store-file.yaml", SHA256: "eeaa88a701796f263e8e4c32a5d16ae3e18b2cc97d65338632e4ac8b9e49b7a8", Title: "A .DS_Store file is exposed"},
	{ID: "apache-server-status", Path: "http/misconfiguration/apache/apache-server-status.yaml", SHA256: "a0b401b09818e26203a06fdfdffdfd3a5a1af18a7b3ebd72eaaa982adb27f456", Title: "The Apache server status page is open"},
	{ID: "nginx-status", Path: "http/misconfiguration/nginx/nginx-status.yaml", SHA256: "931c4e8049b9d94d2a79d9a476f295db87fe5f5b65d760491a5ca2b4cb80b307", Title: "The nginx status page is open"},
	{ID: "openapi", Path: "http/exposures/apis/openapi.yaml", SHA256: "1a5541ec8f60d5a5fe56cc15a20b95e7609ebec177ec0ac910b99df8d114981b", Title: "An API description (OpenAPI) is exposed"},
	{ID: "grafana-detect", Path: "http/exposed-panels/grafana-detect.yaml", SHA256: "7e9829d7f57dee46f723de23f9f4c95f97ba3855b19e4466080f6d78e606b4d0", Title: "A Grafana dashboard is reachable"},
	{ID: "home-assistant-panel", Path: "http/exposed-panels/home-assistant-panel.yaml", SHA256: "62dbd8aec3c995d7643dd0a463e6c060d8431fc7cc3f1ef45a3139753668b017", Title: "A Home Assistant login page is reachable"},
	{ID: "jellyfin-detect", Path: "http/technologies/jellyfin-detect.yaml", SHA256: "2ce8d5a45bdbd93220e4627e2c5dfd29246815b1923217ca7be1f92404ff8578", Title: "A Jellyfin media server is reachable"},
	{ID: "kubernetes-dashboard", Path: "http/exposed-panels/kubernetes-dashboard.yaml", SHA256: "e794c21a6b83a5273ac22645102b30d4d8049ef996fd5ef9901a8977cdf1700e", Title: "A Kubernetes dashboard is reachable"},
	{ID: "prometheus-exporter", Path: "http/misconfiguration/prometheus/prometheus-exporter.yaml", SHA256: "130836c9ff0f5cc58d92b1c41c5ef564c9a2d88c4b1fef17589750b0c4d67a6e", Title: "A Prometheus metrics page is open"},
	{ID: "default-apache-test-page", Path: "http/technologies/apache/default-apache-test-page.yaml", SHA256: "f4cdf5e2f3b2b65b5683a26c5d979d5d2011854e7dbd50eab4b4c8299b76feec", Title: "The default Apache test page is showing"},
	{ID: "default-nginx-page", Path: "http/technologies/nginx/default-nginx-page.yaml", SHA256: "35a39e681f7dd41ddfb06712ec207adb18d676e0c4d2a3d83c3fbdab103c2c2d", Title: "The default nginx page is showing"},
	{ID: "luci-login-detection", Path: "http/exposed-panels/luci-login-detection.yaml", SHA256: "1b4a00abedd2f48635cee42219ce4b8d0155bfaac9eb43ab9537b400e21a6290", Title: "An OpenWrt (LuCI) login page is reachable"},
	{ID: "asus-router-panel", Path: "http/exposed-panels/asus-router-panel.yaml", SHA256: "638808b4b1c68b842067a7820cb0652b4076dc1697167b393d0b5386eb6e393c", Title: "An ASUS router login page is reachable"},
	{ID: "unifi-os-panel", Path: "http/exposed-panels/unifi-os-panel.yaml", SHA256: "a995965438ef4bda69162a7cddd9f57790de9fc63e71c3a8817cf300b5db3164", Title: "A UniFi OS login page is reachable"},
	{ID: "unifi-securitygateway-panel", Path: "http/exposed-panels/unifi-securitygateway-panel.yaml", SHA256: "41ec6b0facc21fd0500711d9a1b1403c7ecfbd4b9d4b50aa60dd6c2ff72de982", Title: "A UniFi gateway login page is reachable"},
	{ID: "zyxel-router-panel", Path: "http/exposed-panels/zyxel-router-panel.yaml", SHA256: "29b89db73d43c363bb89b6a7cb1d237ceb556a990d83e5a57f4f4168cd4fef1e", Title: "A Zyxel router login page is reachable"},
	{ID: "ubiquiti-edgerouter-panel", Path: "http/exposed-panels/ubiquiti-edgerouter-panel.yaml", SHA256: "e4f15410f5616b5ab9f6ff9a4c0839eff0bfd6f39454dd661944c76aec3cc435", Title: "A Ubiquiti EdgeRouter login page is reachable"},
	{ID: "openvpn-router-management", Path: "http/exposed-panels/openvpn-router-management.yaml", SHA256: "0667d1db7f3db3c09a0b648181579b40d2b5d45a390f2ac947c61897e399d630", Title: "An OpenVPN management page is reachable"},
	{ID: "truenas-panel", Path: "http/exposed-panels/truenas-scale-panel.yaml", SHA256: "bc6a29d94beb01dbc97583f0e0aeebf3357374380fac817b74737239ccf178f1", Title: "A TrueNAS login page is reachable"},
	{ID: "qnap-qts-panel", Path: "http/exposed-panels/qnap/qnap-qts-panel.yaml", SHA256: "c9652f806bc4832c30c0f61abdf5be9466f92d747f8f9b9d489ccf82b10a77fe", Title: "A QNAP login page is reachable"},
	{ID: "synology-dsm-system-info", Path: "http/misconfiguration/synology-dsm-system-info.yaml", SHA256: "460c13c8f72697021ec8a3ff6b1e808f434ec91fe741d116d36c5cb72c2d1b11", Title: "A Synology DSM system information page is reachable"},
	{ID: "brother-printer-panel", Path: "http/exposed-panels/brother-printer-panel.yaml", SHA256: "ed990f3b20dab167bec8316f2450206f12d05eb92e9e046304595706f1ece875", Title: "A Brother printer page is reachable"},
	{ID: "samsung-printer-detect", Path: "http/exposed-panels/samsung-printer-detect.yaml", SHA256: "c455324209f2ddee8674bdf29268769504eaadfc6b0e0efeb9c99bf5b0a2bc51", Title: "A Samsung printer page is reachable"},
	{ID: "fuji-xerox-printer-detect", Path: "http/exposed-panels/fuji-xerox-printer-detect.yaml", SHA256: "54c3addc1d83ffdc16c51f1b163875b8000ce10cbf077c660247927dcaa69c2f", Title: "A Fuji Xerox printer page is reachable"},
}

// Index keys a list of templates by id, for reading results.
func Index(list []Template) map[string]Template {
	out := make(map[string]Template, len(list))
	for _, t := range list {
		out[t.ID] = t
	}
	return out
}
