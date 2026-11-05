package netaudit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Service checks look at an open port whose service is a problem if it answers without a login: a camera stream, an AI model
// server, a notebook server. Each makes one or two fixed requests to the device's own address and port through the guarded
// client, sends no credential, and reads the answer as typed facts (a status line, a JSON field). What is on the other end is
// untrusted: only the table's own wording reaches a finding. A reply that cannot be read is "unknown", never "protected" and
// never a finding, and an earlier finding is kept through it (see ServiceChecks.Carry).
const (
	CheckRTSPNoAuth        = "rtsp_no_auth"
	CheckAIAPIExposed      = "ai_api_exposed"
	CheckAdminServiceOpen  = "admin_service_exposed"
	maxServiceProbes       = 64
	serviceProbeTimeout    = 3 * time.Second
	maxServiceReply        = 64 << 10
	maxRTSPReply           = 2048
	rtspStatusPrefix       = "RTSP/1."
	statusOK, statusDenied = 200, 401
)

// verdict is what one probe learned.
type verdict int

const (
	verdictUnknown verdict = iota
	verdictOpen
	verdictProtected
)

// serviceProbe is one service: the ports that usually carry it, how to ask, and the words of the finding.
type serviceProbe struct {
	name     string
	check    string
	severity string
	ports    []int
	words    string
	ask      func(ctx context.Context, d ServiceDeps, addr netip.Addr, port int) verdict
}

// ServiceDeps are the guarded ways to reach a device: the dialer (NetworkTools.DialContext) and the client (HTTPClient).
type ServiceDeps struct {
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
	HTTP interface {
		Do(*http.Request) (*http.Response, error)
	}
}

var serviceProbes = []serviceProbe{
	{name: "RTSP camera stream", check: CheckRTSPNoAuth, severity: SeverityHigh, ports: []int{554, 8554}, ask: askRTSP,
		words: "A video stream on this device can be watched by anyone on the network, without a password"},
	httpProbe("Ollama", CheckAIAPIExposed, SeverityHigh, []int{11434}, "/api/tags", jsonHasKey("models"),
		"Ollama answers anyone on the network without a login: its models can be used, and pulled or deleted"),
	httpProbe("an OpenAI-compatible model server", CheckAIAPIExposed, SeverityHigh, []int{8000, 8080, 8081, 1234}, "/v1/models", jsonHasKey("data"),
		"A model server (llama.cpp, vLLM or another OpenAI-compatible one) answers anyone on the network without a login; anyone can use its compute and read what it is asked"),
	httpProbe("Jupyter", CheckAdminServiceOpen, SeverityCritical, []int{8888}, "/api/status", jsonHasKey("started"),
		"A Jupyter notebook server answers without a token; anyone on the network can run programs on this machine"),
	httpProbe("Consul", CheckAdminServiceOpen, SeverityHigh, []int{8500}, "/v1/status/leader", bodyIsQuotedString,
		"Consul answers without an access-control token; anyone on the network can read and change its services and settings"),
	httpProbe("Prometheus", CheckAdminServiceOpen, SeverityLow, []int{9090}, "/api/v1/status/buildinfo", jsonHasKey("status"),
		"Prometheus answers anyone on the network; its metrics and targets show how the network is built"),
	httpProbe("node_exporter", CheckAdminServiceOpen, SeverityLow, []int{9100}, "/metrics", bodyHasPrefix("# HELP"),
		"A metrics exporter answers anyone on the network; its numbers show what the machine is and does"),
	httpProbe("a Docker registry", CheckAdminServiceOpen, SeverityMedium, []int{5000}, "/v2/", bodyIsBraces,
		"A container registry answers without a login; anyone on the network can pull its images and may be able to push"),
}

// ServiceChecks runs the service checks over the open ports of a scan. It holds no state between runs: one instance serves every
// audit, and two audits may report at the same time.
type ServiceChecks struct {
	deps   ServiceDeps
	probes []serviceProbe
}

// ServiceResult is what one run of the service checks found: the findings, and the ports it could not judge, which Carry uses.
type ServiceResult struct {
	Findings []Finding
	unknown  map[string]bool
}

func NewServiceChecks(deps ServiceDeps) *ServiceChecks {
	return &ServiceChecks{deps: deps, probes: serviceProbes}
}

func serviceKey(device string, port int, check string) string {
	return device + "|" + strconv.Itoa(port) + "|" + check
}

// Findings probes every open port of a scanned host that a check covers, at most maxServiceProbes of them, and returns the
// findings of the services that answered without a login, with the ports that gave no answer it could judge.
func (s *ServiceChecks) Findings(ctx context.Context, hosts []Host) ServiceResult {
	unknown := map[string]bool{}
	var out []Finding
	probes := 0
	for _, h := range hosts {
		for _, p := range h.Ports {
			if p.State != "open" {
				continue
			}
			for _, sp := range s.probes {
				if !slices.Contains(sp.ports, p.Number) || probes >= maxServiceProbes || ctx.Err() != nil {
					continue
				}
				probes++
				switch sp.ask(ctx, s.deps, h.Addr, p.Number) {
				case verdictOpen:
					out = append(out, Finding{Device: h.Addr.String(), Port: p.Number, Proto: "tcp", Service: sp.name, Check: sp.check, Severity: sp.severity, Evidence: sp.words})
				case verdictUnknown:
					unknown[serviceKey(h.Addr.String(), p.Number, sp.check)] = true
				}
			}
		}
	}
	return ServiceResult{Findings: out, unknown: unknown}
}

// Carry returns the earlier findings of a port this run could not judge (no answer, an unreadable one): a camera or a notebook
// server that did not answer tonight has not been fixed, and the diff must not read it as resolved.
func (r ServiceResult) Carry(earlier []Finding) []Finding {
	var out []Finding
	for _, f := range earlier {
		if r.unknown[serviceKey(f.Device, f.Port, probeCheckOf(f.Check))] {
			out = append(out, f)
		}
	}
	return out
}

func httpProbe(name, check, severity string, ports []int, path string, open func(body []byte) bool, words string) serviceProbe {
	return serviceProbe{name: name, check: check, severity: severity, ports: ports, words: words,
		ask: func(ctx context.Context, d ServiceDeps, addr netip.Addr, port int) verdict {
			return askHTTP(ctx, d, addr, port, path, open)
		}}
}

// askHTTP makes one GET and reads the status: 200 with a body of the expected shape is open, 401 or 403 is protected, anything
// else is unknown. A redirect is not followed.
func askHTTP(ctx context.Context, d ServiceDeps, addr netip.Addr, port int, path string, open func([]byte) bool) verdict {
	ctx, cancel := context.WithTimeout(ctx, serviceProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(addr.String(), strconv.Itoa(port))+path, nil)
	if err != nil {
		return verdictUnknown
	}
	client := d.HTTP
	if hc, ok := client.(*http.Client); ok {
		c := *hc
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &c
	}
	resp, err := client.Do(req)
	if err != nil {
		return verdictUnknown
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return verdictProtected
	case http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxServiceReply))
		if open(body) {
			return verdictOpen
		}
	}
	return verdictUnknown
}

func jsonHasKey(key string) func([]byte) bool {
	return func(body []byte) bool {
		var obj map[string]json.RawMessage
		if json.Unmarshal(body, &obj) != nil {
			return false
		}
		_, ok := obj[key]
		return ok
	}
}

func bodyHasPrefix(prefix string) func([]byte) bool {
	return func(body []byte) bool { return strings.HasPrefix(strings.TrimSpace(string(body)), prefix) }
}

func bodyIsBraces(body []byte) bool { return strings.TrimSpace(string(body)) == "{}" }

func bodyIsQuotedString(body []byte) bool {
	var s string
	return json.Unmarshal(body, &s) == nil
}

// askRTSP sends OPTIONS and then DESCRIBE for the root path, with no credentials, on one connection. 200 to DESCRIBE means the
// stream's description is given to anyone; 401 or 403 means a login is required; any other answer (a 404 for a path that needs
// a name, a reply that is not RTSP) is unknown.
func askRTSP(ctx context.Context, d ServiceDeps, addr netip.Addr, port int) verdict {
	ctx, cancel := context.WithTimeout(ctx, serviceProbeTimeout)
	defer cancel()
	target := net.JoinHostPort(addr.String(), strconv.Itoa(port))
	conn, err := d.Dial(ctx, "tcp", target)
	if err != nil {
		return verdictUnknown
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	url := "rtsp://" + target + "/"
	r := bufio.NewReader(io.LimitReader(conn, maxRTSPReply*4))
	for cseq, method := range []string{"OPTIONS", "DESCRIBE"} {
		extra := ""
		if method == "DESCRIBE" {
			extra = "Accept: application/sdp\r\n"
		}
		if _, err := fmt.Fprintf(conn, "%s %s RTSP/1.0\r\nCSeq: %d\r\n%s\r\n", method, url, cseq+1, extra); err != nil {
			return verdictUnknown
		}
		code, ok := readRTSPStatus(r)
		if !ok {
			return verdictUnknown
		}
		if method == "OPTIONS" {
			if code == statusDenied || code == http.StatusForbidden {
				return verdictProtected
			}
			continue
		}
		switch code {
		case statusOK:
			return verdictOpen
		case statusDenied, http.StatusForbidden:
			return verdictProtected
		}
	}
	return verdictUnknown
}

// readRTSPStatus reads one reply's status line and discards its headers and body up to the blank line. A line that does not
// start like an RTSP status is not RTSP.
func readRTSPStatus(r *bufio.Reader) (int, bool) {
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, rtspStatusPrefix) {
		return 0, false
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, false
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}
	for range 64 { // headers end at a blank line; a reply with endless headers is cut off
		h, err := r.ReadString('\n')
		if err != nil || strings.TrimSpace(h) == "" {
			break
		}
	}
	return code, true
}
