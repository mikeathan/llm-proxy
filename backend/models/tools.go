package models

// Tool Names
const (
	// Terminal
	ToolTerminalExecute = "execute_terminal_command"

	// Search
	ToolInternetSearch = "internet_search"

	// Communication
	ToolNotifyUser = "notify_user"

	// FileSystem
	ToolFileRead      = "read_file"
	ToolFileWrite     = "write_file"
	ToolDirectoryList = "list_directory"
	
	// Network
	ToolNetworkFetch = "fetch_url"
	ToolNetworkScan  = "scan_local_network"
	ToolNetworkInfo  = "get_network_info"

	// Security/Admin
	ToolApplyGuardrails = "security_guardrails"

	// System
	ToolSubmitTask = "submit_task"
)

// Tool Categories
const (
	CategoryTerminal      = "terminal"
	CategorySearch        = "search"
	CategoryCommunication = "communication"
	CategoryFileSystem    = "filesystem"
	CategoryNetwork       = "network"
	CategoryGlobal        = "security"
	CategorySystem        = "system"
)
