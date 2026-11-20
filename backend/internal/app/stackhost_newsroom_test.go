package app

import (
	"testing"

	"llm-proxy-stacks/newsroom"
	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/stack"
	"llm-proxy/models"
)

// The registry (core) and the manifest (stack) both describe the Topics module; the registry must not drift from the manifest, including the words
// the card shows.
func TestNewsroomStack_TheRegistryModuleMatchesTheManifestSpec(t *testing.T) {
	spec := newsroomStack().Manifest().Modules[0]
	for _, m := range allModules(t, newTestStacks(t).reg) {
		if m.ID != spec.ID {
			continue
		}
		if m.Title != spec.Title || m.Purpose != spec.Purpose || m.Layer != spec.Layer || string(m.Sends) != string(spec.Sends) || m.StepUp != spec.StepUp || m.Stack != stack.Newsroom {
			t.Errorf("registry %+v differs from manifest %+v", m, spec)
		}
		if m.About.Watches != spec.About.Watches || m.About.Never != spec.About.Never || m.About.ShowsUp != spec.About.ShowsUp || m.About.Settings != spec.About.Settings {
			t.Errorf("the card's words differ: registry %+v, manifest %+v", m.About, spec.About)
		}
		return
	}
	t.Fatal("the Topics module is not in the registry")
}

func TestNewsroomStack_TheSwitchIsOnUntilTurnedOffAndNeedsNoPassphrase(t *testing.T) {
	d := newTestStacks(t)
	m, ok := d.reg.Module(stack.Newsroom, newsroom.ModuleID)
	if !ok {
		t.Fatal("the Topics module is not in the registry")
	}
	if m.StepUp || !m.Switch.Enabled() {
		t.Fatalf("a fresh install has the module on without a passphrase, got step-up %v, enabled %v", m.StepUp, m.Switch.Enabled())
	}
	if err := m.Switch.Set(false); err != nil || !d.newsroom.Get().Disabled || m.Switch.Enabled() {
		t.Errorf("switching off stores the off flag, got %+v, %v", d.newsroom.Get(), err)
	}
}

func TestNewsroomStack_IsNotReadyWhileInternetAccessIsOff(t *testing.T) {
	d := newTestStacks(t)
	d.guard = models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true}
	m, _ := d.reg.Module(stack.Newsroom, newsroom.ModuleID)
	if ok, why := m.Ready(); ok || why != blockedNoInternet {
		t.Errorf("ready = %v, %q", ok, why)
	}
}

func TestNewsroomStack_ItsHostTakesExactlyThePortsItsManifestRequires(t *testing.T) {
	m := newsroomStack().Manifest()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []string{stacksdk.PortConfig, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortFetch, stacksdk.PortModel, stacksdk.PortNotice}
	if len(m.Requires) != len(want) {
		t.Errorf("requires %v, want %v", m.Requires, want)
	}
	if _, err := claimedFiles(networkStack().Manifest(), agentsStack().Manifest(), radioStack().Manifest(), remindersStack().Manifest(), m); err != nil {
		t.Error(err)
	}
}
