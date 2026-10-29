package app

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"llm-proxy/internal/platform/paths"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

func newSystemTestServer(t *testing.T) *AppContext {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LLM_PROXY_HOME", dir)
	p := paths.Paths{ConfigDir: dir, DataDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	dataMgr, err := storage.NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := dataMgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	srv := NewServer(mocks.NewMockManager(), dataMgr)
	t.Cleanup(srv.metrics.Stop)
	return srv
}

// The two global hot-memory defaults are saved independently of each other and of the rest of the memory settings.
func TestApplySystemUpdate_MemoryHotDefaults(t *testing.T) {
	srv := newSystemTestServer(t)
	if got := srv.GetSettings().Memory.HotDefaults(); got != (models.MemoryHotDefaults{AssistantHot: true, AutomationHot: false}) {
		t.Fatalf("shipped defaults = %+v, want assistant on, automations off", got)
	}
	retention := srv.GetSettings().Memory.RetentionDays

	off := models.MemoryHotDefaults{AssistantHot: false, AutomationHot: true}
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Memory: &off}); err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}
	set := srv.GetSettings()
	if got := set.Memory.HotDefaults(); got != off {
		t.Errorf("saved defaults = %+v, want %+v", got, off)
	}
	if set.Memory.RetentionDays != retention {
		t.Errorf("retention changed from %d to %d: only the hot defaults may be written", retention, set.Memory.RetentionDays)
	}

	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{}); err != nil {
		t.Fatalf("ApplySystemUpdate without memory: %v", err)
	}
	if got := srv.GetSettings().Memory.HotDefaults(); got != off {
		t.Errorf("an update that omits memory changed the defaults to %+v", got)
	}
}

// Reminder settings: saved as a unit, validated at the boundary, untouched by updates that omit them.
func TestApplySystemUpdate_RemindersSettings(t *testing.T) {
	srv := newSystemTestServer(t)
	if srv.GetSettings().Reminders != nil {
		t.Fatalf("shipped default should be unset, got %+v", srv.GetSettings().Reminders)
	}

	want := models.RemindersConfig{SkipConnectors: []string{"telegram"}, Timezone: "Europe/London"}
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Reminders: &want}); err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}
	if got := srv.GetSettings().Reminders; got == nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("saved = %+v, want %+v", got, want)
	}

	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{}); err != nil {
		t.Fatal(err)
	}
	if got := srv.GetSettings().Reminders; got == nil || !reflect.DeepEqual(*got, want) {
		t.Errorf("an update that omits reminders changed them to %+v", got)
	}

	bad := models.RemindersConfig{Timezone: "Mars/Base"}
	err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Reminders: &bad})
	if !models.IsRemindersConfigError(err) {
		t.Fatalf("invalid timezone err = %v, want a reminders config error", err)
	}
	if got := srv.GetSettings().Reminders; got == nil || !reflect.DeepEqual(*got, want) {
		t.Errorf("a rejected update must not change the saved settings, got %+v", got)
	}

	reset := models.RemindersConfig{}
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Reminders: &reset}); err != nil {
		t.Fatal(err)
	}
	if got := srv.GetSettings().Reminders; got != nil && !reflect.DeepEqual(*got, reset) {
		t.Errorf("reset left %+v", got)
	}
}

// Agent settings: saved as a unit, absent by default (which reads as on), and untouched by updates that omit them.
func TestApplySystemUpdate_AgentSettings(t *testing.T) {
	srv := newSystemTestServer(t)
	if got := srv.GetSettings().Agent; got != nil {
		t.Fatalf("shipped default is no block, got %+v", got)
	}
	if !srv.GetSettings().Agent.AdaptiveResultCapEnabled() {
		t.Error("no block means the adaptive result cap is on")
	}

	off := false
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Agent: &models.AgentConfig{AdaptiveResultCap: &off}}); err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}
	if srv.GetSettings().Agent.AdaptiveResultCapEnabled() {
		t.Fatal("an explicit off must be saved")
	}
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{}); err != nil {
		t.Fatal(err)
	}
	if srv.GetSettings().Agent.AdaptiveResultCapEnabled() {
		t.Error("an update that omits the agent block changed it")
	}
	on := true
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Agent: &models.AgentConfig{AdaptiveResultCap: &on}}); err != nil {
		t.Fatal(err)
	}
	if !srv.GetSettings().Agent.AdaptiveResultCapEnabled() {
		t.Error("turning it back on must be saved")
	}
}

// The sub-agent settings ride in the agent block: saved, read back, and a pool out of range is refused before anything
// is saved.
func TestApplySystemUpdate_SubagentSettings(t *testing.T) {
	srv := newSystemTestServer(t)
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Agent: &models.AgentConfig{SubagentModel: "cloud-m", SubagentConcurrency: 2}}); err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}
	if got := srv.GetSettings().Agent; got.SubagentModelName() != "cloud-m" || got.SubagentLimit() != 2 {
		t.Errorf("saved %+v", got)
	}
	err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Agent: &models.AgentConfig{SubagentConcurrency: 99}, Bind: "0.0.0.0:1"})
	if !models.IsAgentConfigError(err) {
		t.Fatalf("got %v, want an agent config error", err)
	}
	if srv.GetSystem().Server.Bind == "0.0.0.0:1" || srv.GetSettings().Agent.SubagentLimit() != 2 {
		t.Error("a rejected update must not save anything")
	}
}

func TestAppServices_AgentSettingsReadsTheLiveSettings(t *testing.T) {
	srv := newSystemTestServer(t)
	services := AppServices{AppCtx: srv}
	if !services.AgentSettings().AdaptiveResultCapEnabled() {
		t.Error("default on")
	}
	off := false
	_ = srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Agent: &models.AgentConfig{AdaptiveResultCap: &off}})
	if services.AgentSettings().AdaptiveResultCapEnabled() {
		t.Error("a change applies to the next run without a restart")
	}
}

// One update carries the server, GPU, local-provider and registry fields to where each is stored.
func TestApplySystemUpdate_StoresEachSection(t *testing.T) {
	srv := newSystemTestServer(t)
	if err := srv.dataMgr.Registry().Update(func(reg *models.RegistryData) error {
		reg.Catalogue = append(reg.Catalogue, models.ModelRegistryEntry{Name: "alpha"}, models.ModelRegistryEntry{Name: "beta"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	idle, gpuIndex := -1, 2
	err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{
		Bind: "127.0.0.1:9", ModelHost: "10.0.0.5", IdleTimeoutSecs: &idle, Environment: map[string]string{"A": "1"},
		GPUProvider: "nvidia", GPUBinary: "nvidia-smi", GPUIndex: &gpuIndex, GPUSysfsPath: "/sys/gpu",
		GPUSampleIntervalSec: 5, GPUSmoothingAlpha: 0.5,
		DefaultArgs: []string{"--flash-attn"},
		Providers: map[string]models.ProviderItem{
			"local":  {LlamaServerBinary: "/bin/llama", ModelDir: "/models"},
			"openai": {Type: "openai", BaseURL: "https://api.example"},
		},
		PrimaryModel: "alpha", FallbackModel: "beta",
	})
	if err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}

	sys := srv.GetSystem()
	if sys.Server.Bind != "127.0.0.1:9" || sys.Server.ModelHost != "10.0.0.5" || sys.Server.IdleTimeoutSecs != -1 || sys.Server.Environment["A"] != "1" {
		t.Errorf("server section: %+v", sys.Server)
	}
	gpu := sys.Metrics.GPU
	if gpu.Provider != "nvidia" || gpu.Binary != "nvidia-smi" || gpu.Index != 2 || gpu.SysfsPath != "/sys/gpu" ||
		sys.Metrics.GPUSampleIntervalSec != 5 || sys.Metrics.GPUSmoothingAlpha != 0.5 {
		t.Errorf("gpu section: %+v %+v", gpu, sys.Metrics)
	}
	set := srv.GetSettings()
	if set.Local.LlamaServerBinary != "/bin/llama" || set.Local.ModelDir != "/models" || !reflect.DeepEqual(set.Local.DefaultArgs, []string{"--flash-attn"}) {
		t.Errorf("local settings: %+v", set.Local)
	}
	reg := srv.GetRegistry()
	if reg.PrimaryModel != "alpha" || reg.FallbackModel != "beta" {
		t.Errorf("models: primary %q fallback %q", reg.PrimaryModel, reg.FallbackModel)
	}
	if got := reg.Providers["openai"]; got.Type != "openai" || got.BaseURL != "https://api.example" {
		t.Errorf("provider: %+v", got)
	}
	if _, ok := reg.Providers["local"]; ok {
		t.Error("the local provider is infrastructure and must not land in the registry")
	}
}

// A primary or fallback model that is not in the catalogue is refused with the typed error the API maps to 400,
// whether or not providers travel in the same update.
func TestApplySystemUpdate_RefusesAnUnknownModel(t *testing.T) {
	srv := newSystemTestServer(t)
	for _, req := range []models.SystemUpdatePayload{
		{PrimaryModel: "ghost"},
		{FallbackModel: "ghost"},
		{PrimaryModel: "ghost", Providers: map[string]models.ProviderItem{"openai": {Type: "openai"}}},
	} {
		var notFound *models.ModelNotFoundError
		if err := srv.ApplySystemUpdate(context.Background(), req); !errors.As(err, &notFound) {
			t.Errorf("%+v: got %v, want a ModelNotFoundError", req, err)
		}
	}
}

func TestApplySystemUpdate_SavesCommunicationAndSearchWithoutProviders(t *testing.T) {
	srv := newSystemTestServer(t)
	comm := models.CommunicationConfig{Connectors: map[string]models.ConnectorConfig{"bot": {Type: models.ConnectorTypeTelegram}}}
	search := models.DefaultSearchConfig()
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Communication: &comm, Search: &search}); err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}
	reg := srv.GetRegistry()
	if _, ok := reg.Communication.Connectors["bot"]; !ok {
		t.Errorf("communication not saved: %+v", reg.Communication)
	}
	if reg.Search != search {
		t.Errorf("search not saved: %+v", reg.Search)
	}
}

func TestApplySystemUpdate_RejectsAnInvalidSearchBeforeSavingAnything(t *testing.T) {
	srv := newSystemTestServer(t)
	bad := models.SearchConfig{Provider: "no-such-engine"}
	err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Search: &bad, Bind: "0.0.0.0:1"})
	if !models.IsSearchConfigError(err) {
		t.Fatalf("got %v, want a search config error", err)
	}
	if srv.GetSystem().Server.Bind == "0.0.0.0:1" {
		t.Error("a rejected update must not save the fields that came with it")
	}
}

func TestApplySystemUpdate_ExportsServiceCredentials(t *testing.T) {
	srv := newSystemTestServer(t)
	t.Setenv("SERVICE_CLIENT_ID", "")
	t.Setenv("SERVICE_CLIENT_SECRET", "")
	secret := "s-" + "e" + "cret"
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{ServiceClientID: "cid", ServiceClientSecret: secret}); err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}
	if os.Getenv("SERVICE_CLIENT_ID") != "cid" || os.Getenv("SERVICE_CLIENT_SECRET") != secret {
		t.Error("credentials not exported to the process")
	}
}

// A system update that changes a program the server runs, or what it is started with, is command execution as the service
// user; the admin API asks for the passphrase again for it (plan admin-api-authentication). An ordinary save sends the
// same values it read, which must not count, or every save would ask.
func TestSystemUpdateChangesWhatRuns(t *testing.T) {
	srv := newSystemTestServer(t)
	ctx := context.Background()
	if err := srv.ApplySystemUpdate(ctx, models.SystemUpdatePayload{
		Environment: map[string]string{"A": "1"}, DefaultArgs: []string{"-ngl", "99"}, GPUBinary: "nvidia-smi",
		Providers: map[string]models.ProviderItem{"local": {LlamaServerBinary: "/usr/bin/llama-server", DefaultArgs: []string{"-ngl", "99"}}},
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		req  models.SystemUpdatePayload
		want bool
	}{
		{"an empty update", models.SystemUpdatePayload{}, false},
		{"the same values the settings page read back", models.SystemUpdatePayload{
			Environment: map[string]string{"A": "1"}, DefaultArgs: []string{"-ngl", "99"}, GPUBinary: "nvidia-smi",
			Providers: map[string]models.ProviderItem{"local": {LlamaServerBinary: "/usr/bin/llama-server", DefaultArgs: []string{"-ngl", "99"}}},
		}, false},
		{"settings that do not run anything", models.SystemUpdatePayload{ModelHost: "10.0.0.5", PrimaryModel: "x", Agent: &models.AgentConfig{}}, false},
		{"a new environment variable", models.SystemUpdatePayload{Environment: map[string]string{"A": "1", "LD_PRELOAD": "/tmp/x.so"}}, true},
		{"a changed environment variable", models.SystemUpdatePayload{Environment: map[string]string{"A": "2"}}, true},
		{"the environment cleared", models.SystemUpdatePayload{Environment: map[string]string{}}, true},
		{"other model arguments", models.SystemUpdatePayload{DefaultArgs: []string{"--host", "0.0.0.0"}}, true},
		{"another GPU program", models.SystemUpdatePayload{GPUBinary: "/tmp/evil"}, true},
		{"another server program", models.SystemUpdatePayload{Providers: map[string]models.ProviderItem{"local": {LlamaServerBinary: "/tmp/evil"}}}, true},
		{"other arguments through the local provider", models.SystemUpdatePayload{Providers: map[string]models.ProviderItem{"local": {LlamaServerBinary: "/usr/bin/llama-server", DefaultArgs: []string{"--x"}}}}, true},
	}
	for _, c := range cases {
		if got := srv.SystemUpdateChangesWhatRuns(c.req); got != c.want {
			t.Errorf("%s: = %v, want %v", c.name, got, c.want)
		}
	}
}

// The settings page reads a masked webhook secret and sends it back on every save of the communication block: the save
// must keep the stored secret, not store the mask, and a typed replacement must still replace it.
func TestApplySystemUpdate_MaskedWebhookSecretKeepsTheStoredOne(t *testing.T) {
	srv := newSystemTestServer(t)
	conn := func(token string) *models.CommunicationConfig {
		return &models.CommunicationConfig{Connectors: map[string]models.ConnectorConfig{"tg": {
			Type: models.ConnectorTypeTelegram, Enabled: true, Settings: map[string]string{"workspace_id": "w", models.ConnectorSettingWebhookToken: token},
		}}}
	}
	stored := func() string {
		return srv.GetRegistry().Communication.Connectors["tg"].Settings[models.ConnectorSettingWebhookToken]
	}
	save := func(c *models.CommunicationConfig) {
		t.Helper()
		if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{Communication: c}); err != nil {
			t.Fatalf("ApplySystemUpdate: %v", err)
		}
	}

	save(conn("real-secret"))
	save(conn(models.MaskedSecret))
	if got := stored(); got != "real-secret" {
		t.Fatalf("a save that sent the mask changed the secret to %q", got)
	}
	save(conn("replacement"))
	if got := stored(); got != "replacement" {
		t.Errorf("a typed secret must replace the stored one, got %q", got)
	}
}
