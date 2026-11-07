package stack

import (
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

type flag struct{ on bool }

func (f *flag) Enabled() bool          { return f.on }
func (f *flag) Set(enabled bool) error { f.on = enabled; return nil }

// testManifest is a one-module stack written inline: the core's tests do not import a stack (Constitution IV.6).
func testManifest() stacksdk.Manifest {
	return stacksdk.Manifest{
		ID: "example", Title: "Example", Purpose: "p",
		Layers:   []stacksdk.Layer{{ID: "base", Title: "Base"}},
		Modules:  []stacksdk.ModuleSpec{{ID: "echo", Title: "Echo", Layer: "base", Sends: stacksdk.SendsNothing, About: stacksdk.About{Never: "send anything"}}},
		Settings: stacksdk.SettingsSchema{Key: "example", Version: 1},
	}
}

func wireAll(f *flag) Wire {
	return func(string) (Wiring, bool) { return Wiring{Switch: f}, true }
}

// A manifest is read into the same Stack and Modules the registry already takes, and the registry still refuses an inconsistent
// description.
func TestFromManifest_BuildsAWorkingRegistry(t *testing.T) {
	m := testManifest()
	st := StackFromManifest(m)
	mods, err := ModulesFromManifest(m, wireAll(&flag{}))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := New([]Stack{st}, mods)
	if err != nil {
		t.Fatalf("the reference stack must make a valid registry: %v", err)
	}
	view, err := reg.View(StackID("example"), nil)
	if err != nil || len(view.Layers) != 1 || len(view.Layers[0].Modules) != 1 {
		t.Fatalf("view = %+v, %v", view, err)
	}
	mv := view.Layers[0].Modules[0]
	if mv.ID != "echo" || mv.Sends != SendsNothing || mv.About.Never == "" {
		t.Errorf("module view = %+v", mv)
	}
	if _, err := reg.Switch(StackID("example"), "echo", true); err != nil {
		t.Errorf("switching through the wired Switch: %v", err)
	}
}

func TestModulesFromManifest_RefusesAnInconsistentDescription(t *testing.T) {
	m := testManifest()
	if _, err := ModulesFromManifest(m, func(string) (Wiring, bool) { return Wiring{}, false }); err == nil || !strings.Contains(err.Error(), "echo") {
		t.Errorf("a module with nothing wired to it must be refused, naming it: %v", err)
	}
	bad := m
	bad.Modules = append([]stacksdk.ModuleSpec{}, m.Modules...)
	bad.Modules[0].Layer = "nowhere"
	if _, err := ModulesFromManifest(bad, wireAll(&flag{})); err == nil {
		t.Error("an invalid manifest must be refused before it is read")
	}
	if _, err := New([]Stack{StackFromManifest(m), StackFromManifest(m)}, nil); err == nil {
		t.Error("the same stack twice must still be refused")
	}
}

// The SDK and the registry spell each kind of traffic the same way, so a manifest's value is read without translation.
func TestTrafficValuesMatchTheSDK(t *testing.T) {
	for sdk, own := range map[stacksdk.Traffic]Traffic{
		stacksdk.SendsNothing: SendsNothing, stacksdk.SendsLAN: SendsLAN, stacksdk.SendsInternet: SendsInternet,
		stacksdk.SendsMessage: SendsMessage, stacksdk.Listens: Listens,
	} {
		if string(sdk) != string(own) || !own.Valid() {
			t.Errorf("%q vs %q", sdk, own)
		}
	}
}
