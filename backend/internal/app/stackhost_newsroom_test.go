package app

import (
	"testing"

	"llm-proxy-stacks/newsroom"
	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/stack"
	"llm-proxy/internal/core/stackport"
	"llm-proxy/models"
)

// The registry (core) and the manifest (stack) both describe each module (Watches and Briefs); the registry must not drift from the manifest, including
// the words the card shows.
func TestNewsroomStack_TheRegistryModulesMatchTheManifestSpecs(t *testing.T) {
	registry := allModules(t, newTestStacks(t).reg)
	for _, spec := range newsroomStack().Manifest().Modules {
		found := false
		for _, m := range registry {
			if m.ID != spec.ID || m.Stack != stack.Newsroom {
				continue
			}
			found = true
			if m.Title != spec.Title || m.Purpose != spec.Purpose || m.Layer != spec.Layer || string(m.Sends) != string(spec.Sends) || m.StepUp != spec.StepUp {
				t.Errorf("registry %+v differs from manifest %+v", m, spec)
			}
			if m.About.Watches != spec.About.Watches || m.About.Never != spec.About.Never || m.About.ShowsUp != spec.About.ShowsUp || m.About.Settings != spec.About.Settings {
				t.Errorf("the card's words differ: registry %+v, manifest %+v", m.About, spec.About)
			}
		}
		if !found {
			t.Errorf("the %s module is not in the registry", spec.ID)
		}
	}
}

func TestNewsroomStack_EachSwitchMovesOnlyItsOwnFlag(t *testing.T) {
	d := newTestStacks(t)
	watches, _ := d.reg.Module(stack.Newsroom, newsroom.ModuleID)
	briefs, ok := d.reg.Module(stack.Newsroom, newsroom.BriefModuleID)
	if !ok || briefs.StepUp || !briefs.Switch.Enabled() {
		t.Fatalf("a fresh install has the Briefs module on without a passphrase: %+v, %v", briefs, ok)
	}
	if err := briefs.Switch.Set(false); err != nil || !d.newsroom.Get().BriefsDisabled || d.newsroom.Get().Disabled || !watches.Switch.Enabled() {
		t.Errorf("switching Briefs off must not switch Watches off, got %+v, %v", d.newsroom.Get(), err)
	}
	if err := watches.Switch.Set(false); err != nil || briefs.Switch.Enabled() == true || !d.newsroom.Get().Disabled {
		t.Errorf("both flags are kept side by side, got %+v, %v", d.newsroom.Get(), err)
	}
	if err := briefs.Switch.Set(true); err != nil || d.newsroom.Get().BriefsDisabled || !d.newsroom.Get().Disabled {
		t.Errorf("switching Briefs back on leaves Watches off, got %+v, %v", d.newsroom.Get(), err)
	}
}

func TestNewsroomStack_TheSwitchIsOnUntilTurnedOffAndNeedsNoPassphrase(t *testing.T) {
	d := newTestStacks(t)
	m, ok := d.reg.Module(stack.Newsroom, newsroom.ModuleID)
	if !ok {
		t.Fatal("the Watches module is not in the registry")
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
	want := []string{stacksdk.PortConfig, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortFetch, stacksdk.PortModel, stacksdk.PortNotice, stacksdk.PortSchedule, stacksdk.PortHome, stacksdk.PortDay}
	if len(m.Requires) != len(want) {
		t.Errorf("requires %v, want %v", m.Requires, want)
	}
	if _, err := claimedFiles(networkStack().Manifest(), agentsStack().Manifest(), radioStack().Manifest(), remindersStack().Manifest(), m); err != nil {
		t.Error(err)
	}
}

// The screen says what an empty daily limit means from the stack's own constants; the limit itself is the core's, so the two must agree.
func TestNewsroomStack_TheDailyLimitBoundsMatchTheCoresModelPort(t *testing.T) {
	if newsroom.DefaultModelCalls != stackport.ModelDailyLimit || newsroom.MaxModelCalls != stackport.ModelDailyLimitMax {
		t.Errorf("the stack says %d and %d, the core holds %d and %d", newsroom.DefaultModelCalls, newsroom.MaxModelCalls, stackport.ModelDailyLimit, stackport.ModelDailyLimitMax)
	}
}
