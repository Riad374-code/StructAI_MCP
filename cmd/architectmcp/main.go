// Command architectmcp runs the hosted ArchitectMCP Streamable HTTP server.
//
// This binary is delivery infrastructure only. Backend services own product
// business logic, persistence, and billing.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Riad374-code/architectmcp/internal/mcp"
)

const (
	addrEnv               = "ARCHITECTMCP_ADDR"
	backendBearerTokenEnv = "ARCHITECTMCP_BACKEND_BEARER_TOKEN"
	backendURLEnv         = "ARCHITECTMCP_BACKEND_URL"
	machineIDEnv          = "ARCHITECTMCP_MACHINE_ID"
	defaultAddr           = ":8080"
)

func main() {
	addr := envOrDefault(addrEnv, defaultAddr)
	backendClient, err := configureBackendClient()
	if err != nil {
		log.Fatalf("architectmcp configuration: %v", err)
	}
	if err := mcp.Run(
		context.Background(),
		addr,
		backendClient,
		mcp.WithPlanExecutor(backendClient),
		mcp.WithCheckExecutor(backendClient),
	); err != nil {
		log.Fatalf("architectmcp: %v", err)
	}
}

func configureBackendClient() (*mcp.BackendClient, error) {
	backendURL, err := requiredEnv(backendURLEnv)
	if err != nil {
		return nil, err
	}
	machineID, err := machineIDFromEnv()
	if err != nil {
		return nil, err
	}
	return mcp.NewBackendClient(mcp.BackendClientConfig{
		BaseURL:      backendURL,
		MachineID:    machineID,
		ServiceToken: strings.TrimSpace(os.Getenv(backendBearerTokenEnv)),
	})
}

func machineIDFromEnv() (string, error) {
	if machineID := strings.TrimSpace(os.Getenv(machineIDEnv)); machineID != "" {
		return machineID, nil
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("resolve machine ID: %w", err)
	}
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return "", fmt.Errorf("%s is required when hostname is unavailable", machineIDEnv)
	}
	sum := sha256.Sum256([]byte(hostname))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}
