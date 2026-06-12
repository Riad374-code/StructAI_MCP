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

func main() {
	addr := envOrDefault("ARCHITECTMCP_ADDR", ":8080")
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
	machineID, err := machineIDFromEnv()
	if err != nil {
		return nil, err
	}
	return mcp.NewBackendClient(mcp.BackendClientConfig{
		BaseURL:   envOrDefault("ARCHITECTMCP_BACKEND_URL", "http://localhost:3002"),
		MachineID: machineID,
	})
}

func machineIDFromEnv() (string, error) {
	if machineID := strings.TrimSpace(os.Getenv("ARCHITECTMCP_MACHINE_ID")); machineID != "" {
		return machineID, nil
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("resolve machine ID: %w", err)
	}
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return "", fmt.Errorf("ARCHITECTMCP_MACHINE_ID is required when hostname is unavailable")
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
