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
