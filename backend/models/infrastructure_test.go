package models

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSchedulerConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SchedulerConfig
		wantErr error
	}{
		{
			name: "shipped defaults are valid",
			cfg:  DefaultSchedulerConfig(),
		},
		{
			name: "explicit values are valid",
			cfg:  SchedulerConfig{LocalConcurrency: 2, CloudConcurrency: 8, PreemptAutomations: new(false)},
		},
		{
			name:    "zero local concurrency is rejected",
			cfg:     SchedulerConfig{LocalConcurrency: 0, CloudConcurrency: 1},
			wantErr: ErrSchedulerConcurrencyBelowMinimum,
		},
		{
			name:    "negative cloud concurrency is rejected",
			cfg:     SchedulerConfig{LocalConcurrency: 1, CloudConcurrency: -3},
			wantErr: ErrSchedulerConcurrencyBelowMinimum,
		},
		{
			name: "inbound defaults allow a bounded wait",
			cfg:  SchedulerConfig{LocalConcurrency: 1, CloudConcurrency: 1, InboundWaitSeconds: new(60), InboundMaxQueued: new(32)},
		},
		{
			name: "inbound wait of zero refuses immediately and is allowed",
			cfg:  SchedulerConfig{LocalConcurrency: 1, CloudConcurrency: 1, InboundWaitSeconds: new(0)},
		},
		{
			name: "inbound wait of -1 means unlimited and is allowed",
			cfg:  SchedulerConfig{LocalConcurrency: 1, CloudConcurrency: 1, InboundWaitSeconds: new(-1)},
		},
		{
			name:    "inbound wait below -1 is rejected",
			cfg:     SchedulerConfig{LocalConcurrency: 1, CloudConcurrency: 1, InboundWaitSeconds: new(-2)},
			wantErr: ErrSchedulerInboundInvalid,
		},
		{
			name:    "an empty inbound queue is rejected",
			cfg:     SchedulerConfig{LocalConcurrency: 1, CloudConcurrency: 1, InboundMaxQueued: new(0)},
			wantErr: ErrSchedulerInboundInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsSchedulerConfigError(t *testing.T) {
	if !IsSchedulerConfigError(ErrSchedulerConcurrencyBelowMinimum) {
		t.Fatal("IsSchedulerConfigError(sentinel) = false, want true")
	}
	if !IsSchedulerConfigError(fmt.Errorf("save rejected: %w", ErrSchedulerConcurrencyBelowMinimum)) {
		t.Fatal("IsSchedulerConfigError(wrapped sentinel) = false, want true")
	}
	if !IsSchedulerConfigError(ErrSchedulerInboundInvalid) {
		t.Fatal("IsSchedulerConfigError(inbound sentinel) = false, want true")
	}
	if IsSchedulerConfigError(errors.New("unrelated")) {
		t.Fatal("IsSchedulerConfigError(unrelated) = true, want false")
	}
}

// A settings.yml without the retention key (or without a memory block at all)
// must still reap session entries on the default window — "keep forever" would
// silently let unattended runs grow the store without bound.
func TestMemoryConfig_SessionRetention(t *testing.T) {
	def := time.Duration(DefaultMemoryConfig().RetentionDays) * 24 * time.Hour
	cases := []struct {
		name string
		cfg  *MemoryConfig
		want time.Duration
	}{
		{"nil config uses default", nil, def},
		{"unset days uses default", &MemoryConfig{}, def},
		{"negative days uses default", &MemoryConfig{RetentionDays: -5}, def},
		{"explicit days honoured", &MemoryConfig{RetentionDays: 7}, 7 * 24 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.SessionRetention(); got != tc.want {
				t.Errorf("SessionRetention() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMemoryConfig_HotDefaults(t *testing.T) {
	cases := []struct {
		name           string
		cfg            *MemoryConfig
		assistant, job bool
	}{
		{"nil config", nil, true, false},
		{"unset fields", &MemoryConfig{}, true, false},
		{"shipped defaults", new(DefaultMemoryConfig()), true, false},
		{"assistant switched off", &MemoryConfig{AssistantHot: new(false)}, false, false},
		{"automations switched on", &MemoryConfig{AutomationHot: new(true)}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.cfg.AssistantHotDefault(); got != c.assistant {
				t.Errorf("AssistantHotDefault() = %v, want %v", got, c.assistant)
			}
			if got := c.cfg.AutomationHotDefault(); got != c.job {
				t.Errorf("AutomationHotDefault() = %v, want %v", got, c.job)
			}
			if got := c.cfg.HotDefaults(); got != (MemoryHotDefaults{AssistantHot: c.assistant, AutomationHot: c.job}) {
				t.Errorf("HotDefaults() = %+v", got)
			}
		})
	}
}
