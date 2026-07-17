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
	ToolFileRead       = "read_file"
	ToolFileWrite      = "write_file"
	ToolFileAppend     = "append_file"
	ToolFileEditBlock  = "edit_file_block"
	ToolDirectoryList  = "list_directory"
	
	// Network
	ToolNetworkFetch = "fetch_url"
	ToolNetworkScan  = "scan_local_network"
	ToolNetworkInfo  = "get_network_info"

	// Security/Admin
	ToolApplyGuardrails = "security_guardrails"

	// Memory
	ToolMemorySearch = "memory_search"
	ToolMemoryUpdate = "memory_update"

	// System
	ToolSystemError = "system_error"
)

// Tool Categories
const (
	CategoryTerminal      = "terminal"
	CategorySearch        = "search"
	CategoryCommunication = "communication"
	CategoryFileSystem    = "filesystem"
	CategoryNetwork       = "network"
	CategoryGlobal        = "security"
	CategoryMemory        = "memory"
	CategorySystem        = "system"
)

// Tool error message format strings (use with ToolFileWrite / ToolFileAppend via fmt.Sprintf).
const (
	ToolMissingForAppendMsg = "file does not exist: use %s first, then %s to add more content"
	ToolMissingForEditMsg   = "file does not exist: use %s to create it, or verify the path"
)
