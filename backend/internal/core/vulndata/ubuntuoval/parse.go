package ubuntuoval

import (
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"llm-proxy/models"
)

// Bounds on what one file may make the program hold.
const (
	maxDefinitions = 50_000
	maxNames       = 2_000_000
	maxPerVariable = 5_000
)

// ErrNotUsable means the data was not an OVAL file of security notices with at least one fix to compare with. Nothing of the
// data's own text is in it.
var ErrNotUsable = errors.New("that is not a usable file of Ubuntu security notices")

var (
	usnID       = regexp.MustCompile(`^USN-[0-9]{1,6}-[0-9]{1,3}$`)
	cveID       = regexp.MustCompile(`^CVE-[0-9]{4}-[0-9]{4,10}$`)
	packageName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]{0,99}$`)
	versionText = regexp.MustCompile(`^[A-Za-z0-9.+~:-]{1,100}$`)
)

var severityRank = map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}

type definition struct {
	usn      string
	issued   string
	severity string
	cves     []string
	tests    []string
	patch    bool
}

type testRefs struct{ object, state string }

// reader collects the parts of the file that are joined afterwards: a definition names tests, a test names an object and a
// state, an object names a variable, a variable lists binary package names, a state gives the fixed version.
type reader struct {
	defs       []*definition
	vars       map[string][]string
	objectVar  map[string]string
	stateFixed map[string]string
	tests      map[string]testRefs
	generated  time.Time
	names      int
}

// Parse reads one release's file of Ubuntu security notices (the .usn.oval.xml) and keeps, for each binary package, the
// versions that fixed a flaw. A fix whose version carries "esm" is left out (only Ubuntu Pro installs it), and so is a kernel
// live-patch notice; a name or a version that is not plain text is left out. Nothing of a notice's prose is kept. The reader is
// consumed as a stream, so the file may be large.
func Parse(src io.Reader, release string) (models.UbuntuIndex, error) {
	rd := &reader{vars: map[string][]string{}, objectVar: map[string]string{}, stateFixed: map[string]string{}, tests: map[string]testRefs{}}
	dec := xml.NewDecoder(src)
	if err := rd.read(dec); err != nil {
		return models.UbuntuIndex{}, err
	}
	idx := rd.join(release)
	if len(idx.Notices) == 0 {
		return models.UbuntuIndex{}, ErrNotUsable
	}
	return idx, nil
}

// cursor is where the reader is in the file: the definition, variable, test, object and state it is inside.
type cursor struct {
	def                           *definition
	variable, test, object, state string
}

func (rd *reader) read(dec *xml.Decoder) error {
	var c cursor
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return ErrNotUsable
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(rd.defs) > maxDefinitions {
				return ErrNotUsable
			}
			rd.start(dec, t, &c)
		case xml.EndElement:
			c.end(t.Name.Local)
		}
	}
}

// start handles an opening element by what part of the file it belongs to.
func (rd *reader) start(dec *xml.Decoder, t xml.StartElement, c *cursor) {
	switch t.Name.Local {
	case "definition", "description", "title", "reference", "issued", "cve", "criterion":
		rd.startNotice(dec, t, c)
	case "timestamp":
		if ts, err := time.Parse("2006-01-02T15:04:05", text(dec, t)); err == nil {
			rd.generated = ts.UTC()
		}
	case "constant_variable", "value":
		rd.startVariable(dec, t, c)
	default:
		rd.startCheck(dec, t, c)
	}
}

// startNotice handles the elements of a definition: its title, its USN, its date, its flaws and the tests it needs.
func (rd *reader) startNotice(dec *xml.Decoder, t xml.StartElement, c *cursor) {
	switch t.Name.Local {
	case "definition":
		c.def = nil
		if attr(t, "class") == "patch" {
			c.def = &definition{patch: true}
			rd.defs = append(rd.defs, c.def)
		}
	case "description":
		_ = dec.Skip()
	case "title":
		if title := text(dec, t); c.def != nil && strings.HasPrefix(title, "LSN-") {
			c.def.patch = false
		}
	case "cve":
		rd.addCVE(dec, t, c.def)
	default:
		c.def.note(t)
	}
}

// note records the USN id, the date and the test references a definition names; it does nothing for a definition that is not a
// patch (the receiver may be nil).
func (d *definition) note(t xml.StartElement) {
	if d == nil {
		return
	}
	switch t.Name.Local {
	case "reference":
		if attr(t, "source") == "USN" && usnID.MatchString(attr(t, "ref_id")) {
			d.usn = attr(t, "ref_id")
		}
	case "issued":
		if date := attr(t, "date"); len(date) >= 10 {
			d.issued = date[:10]
		}
	case "criterion":
		d.tests = append(d.tests, attr(t, "test_ref"))
	}
}

// addCVE keeps a flaw id that has the shape of one, and raises the notice's worst severity.
func (rd *reader) addCVE(dec *xml.Decoder, t xml.StartElement, d *definition) {
	c := struct {
		Severity string `xml:"cvss_severity,attr"`
		ID       string `xml:",chardata"`
	}{}
	if err := dec.DecodeElement(&c, &t); err != nil || d == nil || !cveID.MatchString(strings.TrimSpace(c.ID)) {
		return
	}
	d.cves = append(d.cves, strings.TrimSpace(c.ID))
	if sev := strings.ToLower(c.Severity); severityRank[sev] > severityRank[d.severity] {
		d.severity = sev
	}
}

// startVariable handles a variable and the binary package names in it.
func (rd *reader) startVariable(dec *xml.Decoder, t xml.StartElement, c *cursor) {
	switch t.Name.Local {
	case "constant_variable":
		c.variable = attr(t, "id")
	case "value":
		v := text(dec, t)
		if c.variable != "" && len(rd.vars[c.variable]) < maxPerVariable && packageName.MatchString(v) && rd.names < maxNames {
			rd.vars[c.variable] = append(rd.vars[c.variable], v)
			rd.names++
		}
	}
}

// startCheck handles the records that join a definition to a package: its tests, objects and states.
func (rd *reader) startCheck(dec *xml.Decoder, t xml.StartElement, c *cursor) {
	switch t.Name.Local {
	case "dpkginfo_object":
		c.object = attr(t, "id")
	case "name":
		if c.object != "" {
			rd.objectVar[c.object] = attr(t, "var_ref")
		}
	case "dpkginfo_state":
		c.state = attr(t, "id")
	case "evr":
		if v := text(dec, t); c.state != "" && attr(t, "operation") == "less than" && versionText.MatchString(v) {
			rd.stateFixed[c.state] = v
		}
	case "dpkginfo_test":
		c.test = attr(t, "id")
		rd.tests[c.test] = testRefs{}
	case "object", "state":
		rd.refTest(t, c)
	}
}

// refTest records which object or state a test names; the elements of that name elsewhere in the file are not tests.
func (rd *reader) refTest(t xml.StartElement, c *cursor) {
	if c.test == "" {
		return
	}
	r := rd.tests[c.test]
	if t.Name.Local == "object" {
		r.object = attr(t, "object_ref")
	} else {
		r.state = attr(t, "state_ref")
	}
	rd.tests[c.test] = r
}

// end clears the part of the cursor an element that closes was holding.
func (c *cursor) end(name string) {
	switch name {
	case "constant_variable":
		c.variable = ""
	case "dpkginfo_object":
		c.object = ""
	case "dpkginfo_state":
		c.state = ""
	case "dpkginfo_test":
		c.test = ""
	case "definition":
		c.def = nil
	}
}

func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// text reads the text of the element just opened, to its end, and returns it trimmed.
func text(dec *xml.Decoder, start xml.StartElement) string {
	var s string
	if err := dec.DecodeElement(&s, &start); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

// join turns the collected parts into the index: each patch definition's tests give (binary names, fixed version) pairs.
func (rd *reader) join(release string) models.UbuntuIndex {
	idx := models.UbuntuIndex{Release: release, GeneratedAt: rd.generated, Notices: map[string]models.UbuntuNotice{}, Packages: map[string][]models.UbuntuFix{}}
	seen := map[string]bool{}
	for _, d := range rd.defs {
		if !d.patch || d.usn == "" {
			continue
		}
		added := false
		for _, id := range d.tests {
			t, ok := rd.tests[id]
			fixed := rd.stateFixed[t.state]
			if !ok || fixed == "" || strings.Contains(strings.ToLower(fixed), "esm") {
				continue
			}
			for _, name := range rd.vars[rd.objectVar[t.object]] {
				key := name + "|" + d.usn + "|" + fixed
				if seen[key] {
					continue
				}
				seen[key] = true
				idx.Packages[name] = append(idx.Packages[name], models.UbuntuFix{Fixed: fixed, USN: d.usn})
				added = true
			}
		}
		if added {
			cves := slices.Clone(d.cves)
			slices.Sort(cves)
			idx.Notices[d.usn] = models.UbuntuNotice{Issued: d.issued, Severity: d.severity, CVEs: slices.Compact(cves)}
		}
	}
	for _, fixes := range idx.Packages {
		slices.SortFunc(fixes, func(a, b models.UbuntuFix) int { return strings.Compare(a.USN, b.USN) })
	}
	return idx
}
