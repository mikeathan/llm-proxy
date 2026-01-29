package testutils

import "testing"

func SetRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("MCP_SERVER_SSE_URL", "http://mock-mcp-server/events")
	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")
}
