package models

// ToolEffect classifies what a tool call can change. The agent loop uses it to
// decide whether the rest of a tool-call batch may still run after one call
// fails, and whether a turn's text beside its calls can be the run's report.
type ToolEffect int

const (
	// EffectMutating means the call may change workspace, host or external
	// state (files, shell, notifications, MCP servers). This is the default, so
	// an unlisted or unknown tool is never assumed safe to run past.
	EffectMutating ToolEffect = iota
	// EffectReadOnly means the call only reads: a later call in the same batch
	// cannot depend on what it changed, because it changes nothing.
	EffectReadOnly
	// EffectHousekeeping means the call records run bookkeeping (saving a
	// memory, writing the automation journal) rather than doing the task, so
	// text written beside it may be the run's report.
	EffectHousekeeping
)

// ToolReach is how far a tool's network egress goes, and so which run network scopes forbid it.
// ReachNone is the zero value and means "not governed by the scope table" — it does not mean the
// tool has no network (a terminal command or an MCP server is governed by other layers).
type ToolReach string

const (
	ReachNone     ToolReach = ""         // not governed by the scope table
	ReachAny      ToolReach = "any"      // works on the LAN and the internet
	ReachLAN      ToolReach = "lan"      // local network only
	ReachInternet ToolReach = "internet" // internet only
)

// ToolGate names the guardrail switch that statically enables a tool category; the schema hides the
// tool while its gate is closed.
type ToolGate string

const (
	GateNone          ToolGate = ""
	GateNetwork       ToolGate = "network"
	GateSearch        ToolGate = "search"
	GateCommunication ToolGate = "communication"
)

// ToolSpec is everything the platform knows about a built-in tool, declared once.
type ToolSpec struct {
	Name     string
	Category string
	Effect   ToolEffect
	Reach    ToolReach
	Gate     ToolGate
}

// toolCatalog is the one place a built-in tool is declared. The category table, the side-effect
// class, the network-gated set and the scope-forbidden sets are all derived from it, so adding a
// tool is one row (plus its manifest). Order within a category is the order tools are listed in.
var toolCatalog = []ToolSpec{
	{Name: ToolTerminalExecute, Category: CategoryTerminal},
	{Name: ToolInternetSearch, Category: CategorySearch, Effect: EffectReadOnly, Reach: ReachInternet, Gate: GateSearch},
	{Name: ToolNotifyUser, Category: CategoryCommunication, Reach: ReachInternet, Gate: GateCommunication},
	{Name: ToolFileRead, Category: CategoryFileSystem, Effect: EffectReadOnly},
	{Name: ToolFileWrite, Category: CategoryFileSystem},
	{Name: ToolFileAppend, Category: CategoryFileSystem},
	{Name: ToolFileEditBlock, Category: CategoryFileSystem},
	{Name: ToolDirectoryList, Category: CategoryFileSystem, Effect: EffectReadOnly},
	{Name: ToolNetworkFetch, Category: CategoryNetwork, Effect: EffectReadOnly, Reach: ReachAny, Gate: GateNetwork},
	{Name: ToolNetworkInfo, Category: CategoryNetwork, Effect: EffectReadOnly, Reach: ReachLAN, Gate: GateNetwork},
	// network_scan sends connection probes to other hosts, so it is not read-only: a failed call must not let the rest
	// of a batch run on as if nothing happened.
	{Name: ToolNetworkScan, Category: CategoryNetaudit, Reach: ReachLAN, Gate: GateNetwork},
	{Name: ToolApplyGuardrails, Category: CategoryGlobal},
	{Name: ToolMemorySearch, Category: CategoryMemory, Effect: EffectReadOnly},
	{Name: ToolMemoryUpdate, Category: CategoryMemory, Effect: EffectHousekeeping},
	{Name: ToolMemoryForget, Category: CategoryMemory},
	{Name: ToolScheduleReminder, Category: CategoryReminders},
	{Name: ToolListReminders, Category: CategoryReminders, Effect: EffectReadOnly},
	{Name: ToolCancelReminder, Category: CategoryReminders},
	{Name: ToolAutomationJournal, Category: CategoryAutomation, Effect: EffectHousekeeping},
	{Name: ToolSystemError, Category: CategorySystem},
	{Name: ToolDelegate, Category: CategoryDelegation},
}

// catalogIndex is the catalog turned around for lookup.
type catalogIndex struct {
	specByTool      map[string]ToolSpec
	toolsByCategory map[string][]string
	categoryByTool  map[string]string
}

var index = func() catalogIndex {
	idx := catalogIndex{
		specByTool:      make(map[string]ToolSpec, len(toolCatalog)),
		toolsByCategory: map[string][]string{},
		categoryByTool:  make(map[string]string, len(toolCatalog)),
	}
	for _, spec := range toolCatalog {
		idx.specByTool[spec.Name] = spec
		idx.toolsByCategory[spec.Category] = append(idx.toolsByCategory[spec.Category], spec.Name)
		idx.categoryByTool[spec.Name] = spec.Category
	}
	return idx
}()

// SpecOf returns the catalog row of a built-in tool; ok is false for any other tool (an MCP tool).
func SpecOf(toolName string) (ToolSpec, bool) {
	spec, ok := index.specByTool[toolName]
	return spec, ok
}

// EffectOf returns the side-effect class of a tool. A tool outside the catalog (every MCP tool) is
// EffectMutating.
func EffectOf(toolName string) ToolEffect {
	return index.specByTool[toolName].Effect
}

// GateOf returns the guardrail switch gating a tool, or GateNone.
func GateOf(toolName string) ToolGate {
	return index.specByTool[toolName].Gate
}

// ToolsWithGate lists the tools a guardrail switch gates, in catalog order.
func ToolsWithGate(gate ToolGate) []string {
	var out []string
	for _, spec := range toolCatalog {
		if spec.Gate == gate {
			out = append(out, spec.Name)
		}
	}
	return out
}

// HostNetworkGatedTools lists the tools that need agent egress: every tool governed by the scope
// table. The host network switch hides and hard-denies them. A fresh slice.
func HostNetworkGatedTools() []string {
	var out []string
	for _, spec := range toolCatalog {
		if spec.Reach != ReachNone {
			out = append(out, spec.Name)
		}
	}
	return out
}

// ScopeForbiddenTools lists the egress tools a resolved run scope must never expose: none forbids
// every governed tool, lan forbids the internet-only ones, internet_only forbids the LAN-only
// ones, and internet (or an unknown scope) forbids none. A fresh slice.
func ScopeForbiddenTools(scope NetworkScope) []string {
	var forbid func(ToolReach) bool
	switch scope {
	case NetworkScopeNone:
		forbid = func(r ToolReach) bool { return r != ReachNone }
	case NetworkScopeLan:
		forbid = func(r ToolReach) bool { return r == ReachInternet }
	case NetworkScopeInternetOnly:
		forbid = func(r ToolReach) bool { return r == ReachLAN }
	default:
		return nil
	}
	var out []string
	for _, spec := range toolCatalog {
		if forbid(spec.Reach) {
			out = append(out, spec.Name)
		}
	}
	return out
}
