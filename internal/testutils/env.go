package testutils

import "testing"

func SetRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DEVICE_CONTEXT_BASE_URL", "http://mock-device-context")
	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")
}
