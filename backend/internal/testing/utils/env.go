package utils

import "testing"

func SetRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SERVICE_CLIENT_ID", "client-id")
	t.Setenv("SERVICE_CLIENT_SECRET", "client-secret")
}
