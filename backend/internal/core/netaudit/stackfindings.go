package netaudit

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// CheckDNSRouterDiffers is the router's DNS giving different addresses than an outside resolver for names the operator chose, on
// several checks in a row. The Network stack's DNS check offers it (ADR-0036); the wording and the severity are this file's.
const CheckDNSRouterDiffers = "dns_router_differs"

func init() {
	checkTitles[CheckDNSRouterDiffers] = "Your router's DNS answers differently from an outside resolver"
	remediations[CheckDNSRouterDiffers] = "Several checks in a row got other addresses from the router than from an outside resolver. A content network can do that on its own, but a router that was taken over sends people to the wrong place. Look at the router's DNS settings, update its firmware and change its admin password."
	guides[CheckDNSRouterDiffers] = FixGuide{
		Steps: []string{
			"Open the router's settings and read which DNS servers it uses; they should be your provider's or one you chose.",
			"Look the same names up with another resolver (for example with dig and a public resolver) and compare the addresses with the ones in this finding.",
			"If the router's answers are wrong, change its admin password, update its firmware and set its DNS servers again.",
		},
		Risk: "Changing the router's DNS servers briefly interrupts name lookups for every device on the network.",
		Undo: "Put the previous DNS servers back in the router, or accept this finding to say the difference is understood.",
	}
}

// OfferedFinding is one finding a stack offered, in the core's own type so the core imports no stack. It is typed fields only: nothing a
// device or a resolver sent is in it, and the core checks each field before it words a finding.
type OfferedFinding struct {
	ID      string
	Subject netip.Addr
	Count   int
	Names   []string
	Since   time.Time
}

// StackFindingSource is what the run reporter pulls findings from: the host of the stacks, which keeps to the ids each module declared.
type StackFindingSource interface {
	Offered(ctx context.Context) ([]OfferedFinding, error)
}

const (
	// maxOfferedCount bounds a count a stack states; a bigger one is not believed.
	maxOfferedCount = 10000
	// maxOfferedNames bounds the names in one finding.
	maxOfferedNames = 5
	maxNameLength   = 253
)

// hostNamePattern is a lower-case host name: letters, digits, hyphens and dots, starting and ending with a letter or digit.
var hostNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// stackWording turns one checked offer into its finding. A check that no stack may offer is not in the table.
var stackWording = map[string]func(OfferedFinding) Finding{
	CheckDNSRouterDiffers: func(o OfferedFinding) Finding {
		return Finding{Device: o.Subject.String(), Check: CheckDNSRouterDiffers, Severity: SeverityMedium,
			Evidence: fmt.Sprintf("For %d checks in a row the router's DNS gave other addresses than an outside resolver for: %s.", o.Count, strings.Join(o.Names, ", "))}
	},
}

// StackFindings words the offers the core can vouch for and drops the rest. The id must be in the core's table, the subject a private
// address, the count within bounds and every name a plain host name.
func StackFindings(offered []OfferedFinding) []Finding {
	var out []Finding
	for _, o := range offered {
		word, known := stackWording[o.ID]
		if !known || !vouchedFor(o) {
			continue
		}
		out = append(out, word(o))
	}
	return out
}

func vouchedFor(o OfferedFinding) bool {
	if !o.Subject.IsValid() || !(o.Subject.IsPrivate() || o.Subject.IsLinkLocalUnicast()) {
		return false
	}
	if o.Count < 1 || o.Count > maxOfferedCount || len(o.Names) == 0 || len(o.Names) > maxOfferedNames {
		return false
	}
	for _, n := range o.Names {
		if len(n) > maxNameLength || !hostNamePattern.MatchString(n) {
			return false
		}
	}
	return true
}

// WithStackFindings makes the reporter pull the findings the Network stack's modules offer. Without it nothing is pulled.
func (r *RunReporter) WithStackFindings(src StackFindingSource) *RunReporter {
	r.offered = src
	return r
}

// addStackFindings appends what the stacks offer for the devices this scan looked at, as the router log's findings are. A stack that
// cannot answer adds nothing and is stated, and never hides the scan.
func (r *RunReporter) addStackFindings(ctx context.Context, result *Result) string {
	if r.offered == nil {
		return ""
	}
	offered, err := r.offered.Offered(ctx)
	result.Findings = append(result.Findings, restrictTo(StackFindings(offered), scannedDevices(result.Hosts))...)
	if err != nil {
		return "\n\n> Note: what the stacks offer could not be read, so their findings were not compared."
	}
	return ""
}
