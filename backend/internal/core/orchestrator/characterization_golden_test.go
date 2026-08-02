// characterization_golden_test.go — Phase A of the cloud-provider token-budget
// plan.  Captures today's ApplyMetadataDefaults + ResolveICUWeight output into
// a committed snapshot file.  The snapshot's diff is the behaviour-change
// audit: every local entry must remain byte-identical through Phase B+C, and a
// single changed local line fails review.
package orchestrator

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llm-proxy/models"
)

var updateGolden = flag.Bool("update", false, "rewrite the characterization golden snapshot file")

// goldenCase is one row of the characterization matrix.  cfg mirrors the
// runtime config manager.go builds from the registry catalogue: identity fields
// plus metadata; agent-tuning fields start zeroed so ApplyMetadataDefaults
// derives them from context.
type goldenCase struct {
	id  string
	cfg models.ModelConfig
}

// registryGGUF builds a ModelConfig identical to manager.go's registry mapping
// for a local GGUF served through the openai-compatible path.
func registryGGUF(name, filename string, metadata *models.ModelMetadata) models.ModelConfig {
	return models.ModelConfig{
		Name:     name,
		Provider: "openai",
		Filename: filename,
		Metadata: metadata,
		ProviderConfig: &models.ProviderConfig{
			APIKeyName: "test-key",
		},
	}
}

// registryCloud builds a ModelConfig for a metadata-less cloud catalogue entry.
func registryCloud(name, provider string) models.ModelConfig {
	return models.ModelConfig{
		Name:     name,
		Provider: provider,
		Filename: name,
		ProviderConfig: &models.ProviderConfig{
			APIKeyName: "dev",
		},
	}
}

func characterizationMatrix() []goldenCase {
	return []goldenCase{
		// --- every model in backend/data/registry.json (local GGUF, openai slug) ---
		{
			id:  "registry:gpt-oss-20b-Q4_K_M.gguf",
			cfg: registryGGUF("gpt-oss-20b-Q4_K_M.gguf", "gpt-oss-20b-Q4_K_M.gguf", &models.ModelMetadata{ContextLength: 131072, Nctx: 8192, Parameters: 20914757184}),
		},
		{
			id:  "registry:qwen3.5-4b-instruct-q4_k_m.gguf",
			cfg: registryGGUF("qwen3.5-4b-instruct-q4_k_m.gguf", "qwen3.5-4b-instruct-q4_k_m.gguf", &models.ModelMetadata{ContextLength: 262144, Nctx: 8192, Parameters: 4205751296}),
		},
		{
			id:  "registry:gemma-4-4b-it-Q4_K_M.gguf",
			cfg: registryGGUF("gemma-4-4b-it-Q4_K_M.gguf", "gemma-4-4b-it-Q4_K_M.gguf", &models.ModelMetadata{ContextLength: 131072, Nctx: 8192, Parameters: 7518069290}),
		},
		{
			id:  "registry:allura-forge_Llama-3.3-8B-Instruct-Q5_K_M.gguf",
			cfg: registryGGUF("allura-forge_Llama-3.3-8B-Instruct-Q5_K_M.gguf", "allura-forge_Llama-3.3-8B-Instruct-Q5_K_M.gguf", &models.ModelMetadata{ContextLength: 16384, Nctx: 16384, Parameters: 8030261248}),
		},
		{
			id:  "registry:Qwen3.5-9B-UD-Q4_K_XL.gguf",
			cfg: registryGGUF("Qwen3.5-9B-UD-Q4_K_XL.gguf", "Qwen3.5-9B-UD-Q4_K_XL.gguf", &models.ModelMetadata{ContextLength: 262144, Nctx: 8192, Parameters: 9197093888}),
		},
		{
			id:  "registry:LFM2-24B-A2B-Q4_K_M.gguf",
			cfg: registryGGUF("LFM2-24B-A2B-Q4_K_M.gguf", "LFM2-24B-A2B-Q4_K_M.gguf", &models.ModelMetadata{ContextLength: 128000, Nctx: 8192, Parameters: 23843661440}),
		},
		// --- every model in backend/data/registry.json (nvidia, no metadata) ---
		{
			id:  "registry:laguna-xs-2.1",
			cfg: registryCloud("laguna-xs-2.1", "nvidia"),
		},
		{
			id:  "registry:deepseek-v4-flash",
			cfg: registryCloud("deepseek-v4-flash", "nvidia"),
		},
		{
			id:  "registry:gemma-4-31b-it",
			cfg: registryCloud("gemma-4-31b-it", "nvidia"),
		},
		// --- provider: local with / without Metadata.Nctx ---
		{
			id: "local:nctx",
			cfg: models.ModelConfig{
				Name:     "local-qwen",
				Provider: "local",
				Metadata: &models.ModelMetadata{ContextLength: 262144, Nctx: 8192, Parameters: 4205751296},
			},
		},
		{
			// Without Nctx the row carries ContextLength (priority 2), so it
			// resolves to a numeric local default — never a typed error.
			id: "local:context-length-only",
			cfg: models.ModelConfig{
				Name:     "local-qwen",
				Provider: "local",
				Metadata: &models.ModelMetadata{ContextLength: 8192, Parameters: 4205751296},
			},
		},
		// --- provider: openai + .gguf name + Nctx present ---
		{
			id: "openai:gguf-name-nctx",
			cfg: models.ModelConfig{
				Name:     "qwen3.5-9b.gguf",
				Provider: "openai",
				Metadata: &models.ModelMetadata{ContextLength: 262144, Nctx: 8192},
			},
		},
		// --- provider: openai + local base URL + non-.gguf name (M8 case) ---
		{
			id: "openai:local-url-non-gguf",
			cfg: models.ModelConfig{
				Name:     "llama-alias",
				Provider: "openai",
				ProviderConfig: &models.ProviderConfig{
					BaseURL: "http://127.0.0.1:8080",
				},
			},
		},
		// --- provider: openai + remote base URL + non-.gguf name (genuine cloud) ---
		{
			id: "openai:remote-url-non-gguf",
			cfg: models.ModelConfig{
				Name:     "gpt-4o",
				Provider: "openai",
				ProviderConfig: &models.ProviderConfig{
					BaseURL: "https://api.openai.com/v1",
				},
			},
		},
		// --- each cloud provider: metadata absent / present / inflated (>128K) ---
		{
			id: "nvidia:no-metadata",
			cfg: models.ModelConfig{Name: "nvidia-model", Provider: "nvidia"},
		},
		{
			id: "nvidia:metadata-present",
			cfg: models.ModelConfig{Name: "nvidia-model", Provider: "nvidia", Metadata: &models.ModelMetadata{ContextLength: 32768}},
		},
		{
			id: "nvidia:metadata-inflated",
			cfg: models.ModelConfig{Name: "nvidia-model", Provider: "nvidia", Metadata: &models.ModelMetadata{ContextLength: 1_000_000}},
		},
		{
			id: "openrouter:no-metadata",
			cfg: models.ModelConfig{Name: "claude", Provider: "openrouter"},
		},
		{
			id: "openrouter:metadata-present",
			cfg: models.ModelConfig{Name: "deepseek/deepseek-v4-flash", Provider: "openrouter", Metadata: &models.ModelMetadata{ContextLength: 1_048_576}},
		},
		{
			id: "openrouter:metadata-inflated",
			cfg: models.ModelConfig{Name: "claude", Provider: "openrouter", Metadata: &models.ModelMetadata{ContextLength: 2_000_000}},
		},
		{
			id: "gemini:no-metadata",
			cfg: models.ModelConfig{Name: "gemini-2", Provider: "gemini"},
		},
		{
			id: "gemini:metadata-present",
			cfg: models.ModelConfig{Name: "gemini-2", Provider: "gemini", Metadata: &models.ModelMetadata{ContextLength: 1_048_576}},
		},
		{
			id: "gemini:metadata-inflated",
			cfg: models.ModelConfig{Name: "gemini-2", Provider: "gemini", Metadata: &models.ModelMetadata{ContextLength: 2_000_000}},
		},
	}
}

// goldenLine renders one case as a stable, byte-identical line.  Fields recorded
// per §3.1: MaxTokens, ContextBudget, ToolCallFormat, ResolveICUWeight.  The
// ICU weight is resolved explicitly (ApplyMetadataDefaults does not set it).
func goldenLine(c goldenCase) string {
	cfg := c.cfg
	ApplyMetadataDefaults(&cfg)
	weight := ResolveICUWeight(cfg)
	return fmt.Sprintf("%s | max_tokens=%d context_budget=%d tool_call_format=%q icu_weight=%.6f",
		c.id, cfg.MaxTokens, cfg.ContextBudget, cfg.ToolCallFormat, weight)
}

func goldenPath() string {
	return filepath.Join("testdata", "characterization_golden.txt")
}

func TestCharacterizationGolden(t *testing.T) {
	cases := characterizationMatrix()

	if *updateGolden {
		var sb strings.Builder
		for _, c := range cases {
			sb.WriteString(goldenLine(c))
			sb.WriteByte('\n')
		}
		dir := filepath.Dir(goldenPath())
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath(), []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	golden, err := os.ReadFile(goldenPath())
	if err != nil {
		t.Fatalf("golden snapshot not found — run `go test -run TestCharacterizationGolden -update` to generate: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(golden), "\n"), "\n")
	if len(lines) != len(cases) {
		t.Fatalf("golden has %d lines, matrix has %d cases", len(lines), len(cases))
	}

	for i, c := range cases {
		want := lines[i]
		got := goldenLine(c)
		if got != want {
			t.Errorf("case %q differs from golden:\n  got:  %s\n  want: %s", c.id, got, want)
		}
	}
}
