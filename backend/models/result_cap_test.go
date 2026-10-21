package models

import (
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestResultCapContext(t *testing.T) {
	if n, ok := ResultCapFrom(context.Background()); ok || n != 0 {
		t.Errorf("a context with no cap reports none: %d %v", n, ok)
	}
	if n, ok := ResultCapFrom(nil); ok || n != 0 { //nolint:staticcheck // a nil context must not panic
		t.Errorf("nil context: %d %v", n, ok)
	}
	ctx := WithResultCap(context.Background(), 8000)
	if n, ok := ResultCapFrom(ctx); !ok || n != 8000 {
		t.Errorf("cap: %d %v", n, ok)
	}
	if n, ok := ResultCapFrom(WithResultCap(context.Background(), 0)); ok || n != 0 {
		t.Errorf("a zero cap means none is set: %d %v", n, ok)
	}
}

func TestAgentConfig_AdaptiveResultCapDefaultsToOnAndHonoursAnExplicitOff(t *testing.T) {
	var none *AgentConfig
	if !none.AdaptiveResultCapEnabled() {
		t.Error("an absent block means on")
	}
	if !(&AgentConfig{}).AdaptiveResultCapEnabled() {
		t.Error("an absent field means on, not false")
	}
	off := false
	if (&AgentConfig{AdaptiveResultCap: &off}).AdaptiveResultCapEnabled() {
		t.Error("an explicit false is off")
	}
	on := true
	if !(&AgentConfig{AdaptiveResultCap: &on}).AdaptiveResultCapEnabled() {
		t.Error("an explicit true is on")
	}
	if got := none.Effective(); got.AdaptiveResultCap == nil || !*got.AdaptiveResultCap {
		t.Errorf("the effective view carries the resolved value: %+v", got)
	}
	if got := (&AgentConfig{AdaptiveResultCap: &off}).Effective(); *got.AdaptiveResultCap {
		t.Error("the effective view keeps an explicit off")
	}
}

func TestAgentConfig_YAMLRoundTrip(t *testing.T) {
	var cfg AppConfig
	if err := yaml.Unmarshal([]byte("agent:\n  adaptive_result_cap: false\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Agent == nil || cfg.Agent.AdaptiveResultCapEnabled() {
		t.Errorf("explicit false must load as off: %+v", cfg.Agent)
	}
	out, _ := yaml.Marshal(cfg)
	if !strings.Contains(string(out), "adaptive_result_cap: false") {
		t.Errorf("explicit false must be written back: %s", out)
	}
	var empty AppConfig
	if err := yaml.Unmarshal([]byte("memory:\n  assistant_hot: true\n"), &empty); err != nil || empty.Agent != nil {
		t.Errorf("a file with no agent block leaves it nil (and so on): %v %v", err, empty.Agent)
	}
	if strings.Contains(func() string { b, _ := yaml.Marshal(empty); return string(b) }(), "agent") {
		t.Error("an unset block is not written")
	}
}
