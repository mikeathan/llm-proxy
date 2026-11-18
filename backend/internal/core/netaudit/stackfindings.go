package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// CheckDNSRouterDiffers is the router's DNS giving different addresses than an outside resolver for names the operator chose, on
// several checks in a row. The Network stack's DNS check offers it (ADR-0036); the wording and the severity are this file's.
const CheckDNSRouterDiffers = "dns_router_differs"

// CheckRadioJoinForeign is a device of the network inventory that asked to join a Wi-Fi network that is not one of the owner's, heard by the
// Radio stack. The stack offers it by the device's tag (ADR-0061); the core maps the tag to the device and words it here.
const CheckRadioJoinForeign = "radio_join_foreign_network"

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
	checkTitles[CheckRadioJoinForeign] = "A device of yours tried to join a Wi-Fi network that is not yours"
	remediations[CheckRadioJoinForeign] = "Radio heard this device ask to join an access point that is not one of the ones you confirmed. Phones and laptops often try networks they have joined before, such as a café's or a neighbour's, which is harmless; a device that keeps trying a network you do not know may have been tricked into saving it. Look at the device's saved Wi-Fi networks and forget the ones you do not use."
	guides[CheckRadioJoinForeign] = FixGuide{
		Steps: []string{
			"Open the device's Wi-Fi settings and look at its list of saved (known) networks.",
			"Forget every network you do not use any more, and any you do not recognise.",
			"If you added an access point of your own (a mesh node, an extender), confirm it on the Radio page under Your networks, so its joins count as yours.",
		},
		Risk: "Forgetting a network means typing its password again the next time you join it.",
		Undo: "Join the forgotten network again, or accept this finding to say the device's joins are understood.",
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

// StackFindingSources pulls the findings of several stacks as one source: a stack that cannot answer adds nothing and its error is joined, so
// one stack never hides another's.
type StackFindingSources []StackFindingSource

// Offered pulls every source.
func (s StackFindingSources) Offered(ctx context.Context) ([]OfferedFinding, error) {
	var out []OfferedFinding
	var errs []error
	for _, src := range s {
		got, err := src.Offered(ctx)
		out = append(out, got...)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return out, errors.Join(errs...)
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

// stackOffer is how the core words one check a stack may offer, and what an offer of it must carry: host names for one, none for another.
type stackOffer struct {
	word      func(OfferedFinding) Finding
	wantNames bool
}

// stackWording turns one checked offer into its finding. A check that no stack may offer is not in the table.
var stackWording = map[string]stackOffer{
	CheckDNSRouterDiffers: {wantNames: true, word: func(o OfferedFinding) Finding {
		return Finding{Device: o.Subject.String(), Check: CheckDNSRouterDiffers, Severity: SeverityMedium,
			Evidence: fmt.Sprintf("For %d checks in a row the router's DNS gave other addresses than an outside resolver for: %s.", o.Count, strings.Join(o.Names, ", "))}
	}},
	CheckRadioJoinForeign: {word: func(o OfferedFinding) Finding {
		return Finding{Device: o.Subject.String(), Check: CheckRadioJoinForeign, Severity: SeverityLow,
			Evidence: fmt.Sprintf("Radio heard this device ask to join an access point that is not one of yours %s, today or yesterday.", timesWord(o.Count))}
	}},
}

func timesWord(n int) string {
	if n == 1 {
		return "once"
	}
	return fmt.Sprintf("%d times", n)
}

// StackFindings words the offers the core can vouch for and drops the rest. The id must be in the core's table, the subject a private
// address, the count within bounds and every name a plain host name.
func StackFindings(offered []OfferedFinding) []Finding {
	var out []Finding
	for _, o := range offered {
		offer, known := stackWording[o.ID]
		if !known || !vouchedFor(o, offer.wantNames) {
			continue
		}
		out = append(out, offer.word(o))
	}
	return out
}

func vouchedFor(o OfferedFinding, wantNames bool) bool {
	if !o.Subject.IsValid() || !(o.Subject.IsPrivate() || o.Subject.IsLinkLocalUnicast()) {
		return false
	}
	if o.Count < 1 || o.Count > maxOfferedCount || len(o.Names) > maxOfferedNames || (len(o.Names) > 0) != wantNames {
		return false
	}
	for _, n := range o.Names {
		if len(n) > maxNameLength || !hostNamePattern.MatchString(n) {
			return false
		}
	}
	return true
}

// WithStackFindings makes the reporter pull the findings the stacks' modules offer. Without it nothing is pulled.
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
