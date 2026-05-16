package assistant

type ProviderTuningDefaults struct {
	MaxSteps        int
	ContextBudget   int
	MaxTokens       int
	ToolCallFormat  string
	Prefill         bool
	ReasoningBudget int
}

func ProviderTiers() map[string]ProviderTuningDefaults {
	return map[string]ProviderTuningDefaults{
		"local":      {MaxSteps: 25, ContextBudget: 8000, MaxTokens: 2048, ToolCallFormat: "", Prefill: false, ReasoningBudget: 0},
		"gemini":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 8192},
		"vertex":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 8192},
		"openai":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 8192},
		"openrouter": {MaxSteps: 30, ContextBudget: 30000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 4096},
		"mulerouter": {MaxSteps: 30, ContextBudget: 30000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 4096},
		"nvidia":     {MaxSteps: 30, ContextBudget: 20000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 2048},
	}
}
