// MCP server types
export interface McpServer {
  name: string
  url: string
  enabled: boolean
}

export interface NewMcpServerForm {
  name: string
  url: string
}
