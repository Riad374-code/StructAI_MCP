package main

import (
	"strings"
	"testing"
)

func TestConfigureBackendClientRequiresBackendURL(t *testing.T) {
	t.Setenv(backendURLEnv, "")
	t.Setenv(backendBearerTokenEnv, "")
	t.Setenv(machineIDEnv, "sha256:test-machine")

	_, err := configureBackendClient()
	if err == nil || !strings.Contains(err.Error(), "ARCHITECTMCP_BACKEND_URL") {
		t.Fatalf("error = %v, want missing backend URL error", err)
	}
}

func TestConfigureBackendClientRequiresBearerForRemoteBackend(t *testing.T) {
	t.Setenv(backendURLEnv, "https://api.architectmcp.com")
	t.Setenv(backendBearerTokenEnv, "")
	t.Setenv(machineIDEnv, "sha256:test-machine")

	_, err := configureBackendClient()
	if err == nil || !strings.Contains(err.Error(), "service bearer token") {
		t.Fatalf("error = %v, want missing service bearer token error", err)
	}
}
