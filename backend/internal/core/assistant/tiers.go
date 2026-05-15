package assistant

type ProviderTuningDefaults struct {
	MaxSteps       int
	ContextBudget  int
	MaxTokens      int
	ToolCallFormat string
	Prefill        bool
}

func ProviderTiers() map[string]ProviderTuningDefaults {
	return map[string]ProviderTuningDefaults{
		"local":      {MaxSteps: 25, ContextBudget: 8000, MaxTokens: 2048, ToolCallFormat: "", Prefill: false},
		"gemini":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false},
		"vertex":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false},
		"openai":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false},
		"openrouter": {MaxSteps: 30, ContextBudget: 30000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false},
		"mulerouter": {MaxSteps: 30, ContextBudget: 30000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false},
		"nvidia":     {MaxSteps: 30, ContextBudget: 20000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false},
	}
}
