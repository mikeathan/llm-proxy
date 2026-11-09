package stacksdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func validManifest() Manifest {
	return Manifest{
		ID: "demo", Title: "Demo", Purpose: "p",
		Layers:   []Layer{{ID: "low", Title: "Low"}, {ID: "high", Title: "High"}},
		Modules:  []ModuleSpec{{ID: "a", Title: "A", Layer: "low", Sends: SendsNothing}, {ID: "b", Title: "B", Layer: "high", Sends: SendsLAN, Needs: []string{"a"}}},
		Tools:    []ToolSpec{{Name: "demo_echo", Category: "demo", Schema: json.RawMessage(`{"type":"object"}`)}},
		Settings: SettingsSchema{Key: "demo", Version: 1},
		Requires: []string{PortConfig, PortGuard},
	}
}

func TestManifest_ValidatesAndRefusesInconsistency(t *testing.T) {
	if err := validManifest().Validate(); err != nil {
		t.Fatalf("a consistent manifest: %v", err)
	}
	cases := map[string]func(*Manifest){
		"bad id":            func(m *Manifest) { m.ID = "Demo-1" },
		"no settings key":   func(m *Manifest) { m.Settings.Key = "" },
		"duplicate layer":   func(m *Manifest) { m.Layers[1].ID = "low" },
		"unknown layer":     func(m *Manifest) { m.Modules[0].Layer = "nowhere" },
		"duplicate module":  func(m *Manifest) { m.Modules[1].ID = "a" },
		"bad traffic":       func(m *Manifest) { m.Modules[0].Sends = "radio" },
		"need not a module": func(m *Manifest) { m.Modules[1].Needs = []string{"ghost"} },
		"need on higher":    func(m *Manifest) { m.Modules[0].Needs = []string{"b"} },
		"self need":         func(m *Manifest) { m.Modules[1].Needs = []string{"b"} },
		"tool no schema":    func(m *Manifest) { m.Tools[0].Schema = nil },
		"tool duplicate":    func(m *Manifest) { m.Tools = append(m.Tools, m.Tools[0]) },
		"unknown port":      func(m *Manifest) { m.Requires = []string{"root"} },
	}
	for name, mutate := range cases {
		m := validManifest()
		mutate(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// A manifest has no field that declares a privilege or relaxes the sandbox (ADR-0033). Adding one must be a deliberate change to
// this test, which is the review.
func TestManifest_DeclaresNoPrivilege(t *testing.T) {
	var fields []string
	for _, typ := range []reflect.Type{reflect.TypeFor[Manifest](), reflect.TypeFor[ModuleSpec](), reflect.TypeFor[RunRequest]()} {
		for i := range typ.NumField() {
			fields = append(fields, strings.ToLower(typ.Field(i).Name))
		}
	}
	for _, f := range fields {
		for _, banned := range []string{"privilege", "capab", "unix", "unconfined", "env", "sandbox", "root_"} {
			if strings.Contains(f, banned) {
				t.Errorf("field %q looks like a privilege or a sandbox relaxation", f)
			}
		}
	}
}

func TestError_CrossesAsACodeAndMessage(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", Errorf(CodeDenied, "scope"))
	if !errors.Is(err, &Error{Code: CodeDenied}) || errors.Is(err, &Error{Code: CodeTimeout}) {
		t.Error("errors.Is must match on the code alone")
	}
	if CodeOf(err) != CodeDenied || CodeOf(errors.New("x")) != CodeInternal || CodeOf(nil) != "" {
		t.Error("CodeOf")
	}
	var back Error
	b, _ := json.Marshal(Errorf(CodeTimeout, "slow"))
	if err := json.Unmarshal(b, &back); err != nil || back.Code != CodeTimeout || back.Message != "slow" {
		t.Errorf("round trip: %+v %v", back, err)
	}
}

func TestCheckPlainData_RefusesWhatCannotCrossAPort(t *testing.T) {
	type withFunc struct{ F func() }
	type withChan struct{ C chan int }
	type withPtr struct{ P *int }
	type withIface struct{ V any }
	type withIntMap struct{ M map[int]string }
	type nested struct{ In []struct{ F func() } }
	for name, v := range map[string]any{"func": withFunc{}, "chan": withChan{}, "pointer": withPtr{}, "interface": withIface{}, "int-keyed map": withIntMap{}, "nested": nested{}} {
		if err := CheckPlainData(reflect.TypeOf(v)); err == nil {
			t.Errorf("%s field must fail the plain-data check", name)
		}
	}
	type fine struct {
		When time.Time
		Raw  json.RawMessage
		M    map[string][]string
	}
	if err := CheckPlainData(reflect.TypeFor[fine]()); err != nil {
		t.Errorf("plain data refused: %v", err)
	}
}

// Every type that crosses a port is plain data and survives a JSON round trip.
func TestPortTypes_ArePlainDataAndRoundTripThroughJSON(t *testing.T) {
	for _, v := range PortTypes() {
		typ := reflect.TypeOf(v)
		if err := CheckPlainData(typ); err != nil {
			t.Errorf("%s: %v", typ, err)
			continue
		}
		filled := reflect.New(typ).Elem()
		fill(filled)
		b, err := json.Marshal(filled.Interface())
		if err != nil {
			t.Errorf("%s: marshal: %v", typ, err)
			continue
		}
		back := reflect.New(typ)
		if err := json.Unmarshal(b, back.Interface()); err != nil {
			t.Errorf("%s: unmarshal: %v", typ, err)
			continue
		}
		if !reflect.DeepEqual(filled.Interface(), back.Elem().Interface()) {
			t.Errorf("%s did not survive a JSON round trip:\n in  %+v\n out %+v", typ, filled.Interface(), back.Elem().Interface())
		}
	}
}

var (
	rawType  = reflect.TypeFor[json.RawMessage]()
	timeType = reflect.TypeFor[time.Time]()
)

// fill sets every exported field to a non-zero value so a field lost in a round trip shows.
func fill(v reflect.Value) {
	switch {
	case v.Type() == rawType:
		v.Set(reflect.ValueOf(json.RawMessage(`{"k":1}`)))
	case v.Type() == timeType:
		v.Set(reflect.ValueOf(time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)))
	default:
		fillKind(v)
	}
}

func fillKind(v reflect.Value) {
	switch v.Kind() {
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int64:
		v.SetInt(7)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			v.SetBytes([]byte("b"))
			return
		}
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fill(v.Index(0))
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
		elem := reflect.New(v.Type().Elem()).Elem()
		fill(elem)
		v.SetMapIndex(reflect.ValueOf("k").Convert(v.Type().Key()), elem)
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				fill(v.Field(i))
			}
		}
	}
}

// ---- routes ----

func routeManifest(routes ...RouteSpec) Manifest {
	m := validManifest()
	m.Routes = routes
	return m
}

func TestValidate_RoutesAreCheckedAsData(t *testing.T) {
	ok := []RouteSpec{
		{Method: "GET", Path: "/admin/api/example/things"},
		{Method: "PUT", Path: "/admin/api/example/things/{id}", Access: AccessStepUp},
		{Method: "POST", Path: "/admin/api/example/things", Access: AccessAdmin},
	}
	if err := routeManifest(ok...).Validate(); err != nil {
		t.Fatalf("valid routes refused: %v", err)
	}
	bad := map[string][]RouteSpec{
		"no method":        {{Path: "/a"}},
		"unknown method":   {{Method: "TRACE", Path: "/a"}},
		"no leading slash": {{Method: "GET", Path: "a"}},
		"dot dot":          {{Method: "GET", Path: "/a/../b"}},
		"double slash":     {{Method: "GET", Path: "/a//b"}},
		"query in path":    {{Method: "GET", Path: "/a?x=1"}},
		"unknown access":   {{Method: "GET", Path: "/a", Access: "public"}},
		"duplicate":        {{Method: "GET", Path: "/a"}, {Method: "GET", Path: "/a", Access: AccessStepUp}},
		"action, admin":    {{Method: "POST", Path: "/a", Action: "scan_udp"}},
		"action, a read":   {{Method: "GET", Path: "/a", Access: AccessStepUp, Action: "scan_udp"}},
		"action, wildcard": {{Method: "POST", Path: "/a/{id}", Access: AccessStepUp, Action: "scan_udp"}},
	}
	for name, routes := range bad {
		if err := routeManifest(routes...).Validate(); err == nil {
			t.Errorf("%s: a manifest with these routes must be refused", name)
		}
	}
}

func TestRoute_AnEmptyAccessIsAdminNeverLess(t *testing.T) {
	if got := (RouteSpec{}).Level(); got != AccessAdmin {
		t.Errorf("empty access = %q, want %q", got, AccessAdmin)
	}
	if got := (RouteSpec{Access: AccessStepUp}).Level(); got != AccessStepUp {
		t.Errorf("step_up = %q", got)
	}
}

func TestPathParams_ListsTheWildcardsOfAPattern(t *testing.T) {
	got := RouteSpec{Method: "GET", Path: "/a/{mac}/b/{id}"}.Params()
	if len(got) != 2 || got[0] != "mac" || got[1] != "id" {
		t.Errorf("params = %v", got)
	}
	if len(RouteSpec{Method: "GET", Path: "/a/b"}.Params()) != 0 {
		t.Error("a path without wildcards has no params")
	}
}

func TestValidate_ARouteWithAnActionNeedsThePrivilegedPort(t *testing.T) {
	m := routeManifest(RouteSpec{Method: "POST", Path: "/admin/api/example/scan", Access: AccessStepUp, Action: "scan_udp"})
	if err := m.Validate(); err == nil {
		t.Fatal("a stack that names an action without requiring the privileged port must be refused")
	}
	m.Requires = append(m.Requires, PortPrivileged)
	if err := m.Validate(); err != nil {
		t.Fatalf("step-up write, no wildcard, port required: %v", err)
	}
}

func TestValidate_StorageNamesAreCheckedAsData(t *testing.T) {
	m := validManifest()
	m.Storage = []string{"network-dns-checks.json", "baseline.json"}
	if err := m.Validate(); err != nil {
		t.Fatalf("plain file names refused: %v", err)
	}
	bad := map[string][]string{
		"empty":            {""},
		"a path":           {"sub/file.json"},
		"a windows path":   {`sub\file.json`},
		"dot":              {"."},
		"dot dot":          {".."},
		"up and out":       {"../settings.yml"},
		"listed twice":     {"a.json", "a.json"},
		"hidden traversal": {"a/../b"},
	}
	for name, files := range bad {
		m := validManifest()
		m.Storage = files
		if err := m.Validate(); err == nil {
			t.Errorf("%s: a manifest claiming %q must be refused", name, files)
		}
	}
}

func TestValidate_TheFetchAndGatewayPortsCanBeRequired(t *testing.T) {
	m := validManifest()
	m.Requires = []string{PortConfig, PortFetch, PortGateway}
	if err := m.Validate(); err != nil {
		t.Fatalf("the fetch port refused: %v", err)
	}
}

// A fetch request has no field that chooses a method, a header or a client: the core sends one guarded GET, so a stack cannot widen it.
func TestFetchRequest_CannotWidenTheGuardedGet(t *testing.T) {
	var fields []string
	for _, typ := range []reflect.Type{reflect.TypeFor[FetchRequest](), reflect.TypeFor[FetchResponse]()} {
		for i := range typ.NumField() {
			fields = append(fields, strings.ToLower(typ.Field(i).Name))
		}
	}
	want := []string{"url", "accept", "maxbytes", "body", "truncated"}
	if !reflect.DeepEqual(fields, want) {
		t.Errorf("fetch fields = %v, want %v: adding one is a deliberate change that needs this test and the guard reviewed", fields, want)
	}
}

// ---- findings (ADR-0036) ----

// A finding a stack offers has typed fields only: no field can hold free text, so nothing a device or a resolver sent can ride in it.
func TestStackFinding_HasNoFreeTextField(t *testing.T) {
	typ := reflect.TypeFor[StackFinding]()
	allowed := map[string]bool{"ID": true, "Names": true}
	for i := range typ.NumField() {
		f := typ.Field(i)
		hasString := f.Type.Kind() == reflect.String || (f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String)
		if hasString && !allowed[f.Name] {
			t.Errorf("StackFinding.%s is a %s; only ID and Names may be text, and the core checks both", f.Name, f.Type)
		}
	}
	if err := CheckPlainData(typ); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_FindingIDsAreCheckedAsData(t *testing.T) {
	withFinds := func(a, b []string) Manifest {
		m := validManifest()
		m.Modules[0].Finds, m.Modules[1].Finds = a, b
		return m
	}
	if err := withFinds([]string{"dns_router_differs"}, nil).Validate(); err != nil {
		t.Fatalf("a module that finds one clean id: %v", err)
	}
	for name, m := range map[string]Manifest{
		"a bad id":       withFinds([]string{"Bad Id"}, nil),
		"an empty id":    withFinds([]string{""}, nil),
		"twice in one":   withFinds([]string{"x_y", "x_y"}, nil),
		"in two modules": withFinds([]string{"x_y"}, []string{"x_y"}),
	} {
		if err := m.Validate(); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
}

// A module that sends datagrams to devices during the nightly report is declared in Looks, and the stack must require the scoped dial
// (ADR-0038 rule 2): a dial that first applies the operator's declared scan scope, not the plain guarded dial.
func TestValidate_LooksAreCheckedAsDataAndNeedTheScopedDial(t *testing.T) {
	withLooks := func(looks []string, requires ...string) Manifest {
		m := validManifest()
		m.Looks, m.Requires = looks, requires
		return m
	}
	if err := withLooks([]string{"udp_probes"}, PortConfig, PortScopedGuard).Validate(); err != nil {
		t.Fatalf("a sensor with the scoped dial: %v", err)
	}
	for name, m := range map[string]Manifest{
		"without the scoped dial":    withLooks([]string{"udp_probes"}, PortConfig, PortGuard),
		"a bad id":                   withLooks([]string{"Udp Probes"}, PortConfig, PortScopedGuard),
		"an empty id":                withLooks([]string{""}, PortConfig, PortScopedGuard),
		"the same id twice":          withLooks([]string{"udp_probes", "udp_probes"}, PortConfig, PortScopedGuard),
		"an unknown port name check": withLooks(nil, "scopedguards"),
	} {
		if err := m.Validate(); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
	}
	if err := withLooks(nil, PortConfig, PortScopedGuard).Validate(); err != nil {
		t.Errorf("requiring the scoped dial without a sensor is allowed: %v", err)
	}
}

// A sensor's request and answer carry addresses, ports and one module id, and nothing a device said: the only text is the module id, which the
// core checks against the manifest. Adding a text field to either type must be a deliberate change to this test.
func TestLookTypes_CarryNoTextFromTheNetwork(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[LookRequest](), reflect.TypeFor[LookResult](), reflect.TypeFor[Pair]()} {
		if err := CheckPlainData(typ); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		for i := range typ.NumField() {
			f := typ.Field(i)
			isText := f.Type.Kind() == reflect.String || (f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String)
			if isText && !(typ == reflect.TypeFor[LookRequest]() && f.Name == "Module") {
				t.Errorf("%s.%s is text; only LookRequest.Module may be", typ, f.Name)
			}
		}
	}
}

// The web sensor's request and answers carry addresses, ports, numbers, dates and one check id, and nothing a device said: the only text is
// the module id of a request and the id of a hit, both of which the core checks against tables it owns. Adding a text field must be a deliberate
// change to this test (ADR-0038 rule 3).
func TestWebTypes_CarryNoTextFromTheNetwork(t *testing.T) {
	allowed := map[string]bool{"WebRequest.Module": true, "WebHit.ID": true}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[WebTarget](), reflect.TypeFor[WebRequest](), reflect.TypeFor[WebHit](), reflect.TypeFor[WebResult](),
		reflect.TypeFor[CertFact](), reflect.TypeFor[CertResult](),
	} {
		if err := CheckPlainData(typ); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		for i := range typ.NumField() {
			f := typ.Field(i)
			isText := f.Type.Kind() == reflect.String || (f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.String)
			if isText && !allowed[typ.Name()+"."+f.Name] {
				t.Errorf("%s.%s is text; only WebRequest.Module and WebHit.ID may be", typ, f.Name)
			}
		}
	}
	if f, ok := reflect.TypeFor[CertFact]().FieldByName("OldVersions"); !ok || f.Type != reflect.TypeFor[[]uint16]() {
		t.Error("protocol versions travel as numbers, so the core maps them to its own words")
	}
}

// A request is bounded the same way on both sides: the SDK names the bounds and the stack and the host refuse more.
func TestWebBounds_AreNamed(t *testing.T) {
	if MaxWebTargets < 1 || MaxLookSeconds < 1 {
		t.Fatalf("bounds: %d targets, %d seconds", MaxWebTargets, MaxLookSeconds)
	}
}
