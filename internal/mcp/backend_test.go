package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	plancontract "github.com/Riad374-code/architectmcp/internal/contracts/plan"
)

func TestBackendClientValidatesLicenseAndCreatesSession(t *testing.T) {
	var gotRequest backendLicenseRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != backendLicensePath {
			t.Fatalf("path = %q, want %q", request.URL.Path, backendLicensePath)
		}
		if request.Header.Get("Authorization") != "Bearer backend-service-token" {
			t.Fatalf(
				"authorization = %q, want MCP backend service bearer",
				request.Header.Get("Authorization"),
			)
		}
		if request.Header.Get(APIKeyHeader) != "sk_mcp_live_secret" {
			t.Fatalf("X-API-Key = %q, want customer API key", request.Header.Get(APIKeyHeader))
		}
		if err := json.NewDecoder(request.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(backendLicenseResponse{
			Valid: true,
			Plan:  "pro",
			Entitlements: backendEntitlements{
				PlanCallsRemaining: 42,
				Seats:              1,
				Features:           []string{"dashboard", "watch_mode"},
			},
			Token:    "session-jwt",
			TokenTTL: 604800,
		})
	}))
	defer server.Close()
	client := newTestBackendClient(t, server.URL)

	principal, err := client.AuthorizeAPIKey(context.Background(), "sk_mcp_live_secret")
	if err != nil {
		t.Fatalf("AuthorizeAPIKey: %v", err)
	}

	if gotRequest.APIKey != "sk_mcp_live_secret" {
		t.Errorf("api_key = %q, want customer API key", gotRequest.APIKey)
	}
	if gotRequest.MachineID != "sha256:test-machine" {
		t.Errorf("machine_id = %q, want sha256:test-machine", gotRequest.MachineID)
	}
	if gotRequest.Version != "mcp/"+serverVersion {
		t.Errorf("version = %q, want mcp/%s", gotRequest.Version, serverVersion)
	}
	if principal.BackendToken != "session-jwt" {
		t.Errorf("backend token = %q, want session-jwt", principal.BackendToken)
	}
	if principal.CredentialHash != credentialFingerprint("sk_mcp_live_secret") {
		t.Error("credential fingerprint was not attached to the session")
	}
	if principal.PlanCallsRemaining != 42 {
		t.Errorf("remaining = %d, want 42", principal.PlanCallsRemaining)
	}
}

func TestBackendClientMapsDeniedLicense(t *testing.T) {
	tests := []struct {
		reason string
		want   error
	}{
		{reason: "key_revoked", want: ErrInvalidAPIKey},
		{reason: "subscription_inactive", want: ErrInvalidAPIKey},
		{reason: "limit_reached", want: ErrInsufficientBalance},
		{reason: "rate_limited", want: ErrRateLimited},
	}
	for _, test := range tests {
		t.Run(test.reason, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(backendLicenseResponse{
					Valid:  false,
					Reason: test.reason,
				})
			}))
			defer server.Close()
			client := newTestBackendClient(t, server.URL)

			_, err := client.AuthorizeAPIKey(context.Background(), "sk_mcp_live_secret")
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestBackendClientTreatsLicenseUnauthorizedAsServiceFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	client := newTestBackendClient(t, server.URL)

	_, err := client.AuthorizeAPIKey(context.Background(), "sk_mcp_live_secret")
	if err == nil {
		t.Fatal("expected service authentication error")
	}
	if errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("error = %v, must not blame the customer API key", err)
	}
}

func TestBackendClientExecutesPlanWithSessionToken(t *testing.T) {
	var gotAuth string
	var gotRequest backendPlanRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != backendPlanPath {
			t.Fatalf("path = %q, want %q", request.URL.Path, backendPlanPath)
		}
		gotAuth = request.Header.Get("Authorization")
		if err := json.NewDecoder(request.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(backendPlanResponse{
			Output: plancontract.Output{
				Status:  plancontract.StatusNeedsInput,
				Message: "answer questions",
			},
		})
	}))
	defer server.Close()
	client := newTestBackendClient(t, server.URL)

	output, err := client.ExecutePlan(context.Background(), "session-jwt", plancontract.Input{
		RawIdea: "A collaborative planning workspace",
	})
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	if gotAuth != "Bearer session-jwt" {
		t.Errorf("authorization = %q, want backend session JWT", gotAuth)
	}
	if gotRequest.Input.RawIdea != "A collaborative planning workspace" {
		t.Errorf("input = %#v", gotRequest.Input)
	}
	if output.Status != plancontract.StatusNeedsInput {
		t.Errorf("status = %q, want %q", output.Status, plancontract.StatusNeedsInput)
	}
}

func TestBackendClientRejectsInsecureRemoteURL(t *testing.T) {
	_, err := NewBackendClient(BackendClientConfig{
		BaseURL:      "http://api.architectmcp.com",
		MachineID:    "machine",
		ServiceToken: "backend-service-token",
	})
	if err == nil {
		t.Fatal("expected non-loopback HTTP URL to be rejected")
	}
}

func TestBackendClientRequiresServiceTokenForRemoteBackend(t *testing.T) {
	_, err := NewBackendClient(BackendClientConfig{
		BaseURL:   "https://api.architectmcp.com",
		MachineID: "machine",
	})
	if err == nil {
		t.Fatal("expected remote backend without service token to be rejected")
	}
}

func TestBackendClientAllowsLoopbackWithoutServiceToken(t *testing.T) {
	_, err := NewBackendClient(BackendClientConfig{
		BaseURL:   "http://localhost:3002",
		MachineID: "machine",
	})
	if err != nil {
		t.Fatalf("NewBackendClient: %v", err)
	}
}

func TestBackendClientRejectsServiceTokenWhitespace(t *testing.T) {
	_, err := NewBackendClient(BackendClientConfig{
		BaseURL:      "https://api.architectmcp.com",
		MachineID:    "machine",
		ServiceToken: "invalid token",
	})
	if err == nil {
		t.Fatal("expected service token with whitespace to be rejected")
	}
}

func newTestBackendClient(t *testing.T, baseURL string) *BackendClient {
	t.Helper()
	client, err := NewBackendClient(BackendClientConfig{
		BaseURL:      baseURL,
		MachineID:    "sha256:test-machine",
		ServiceToken: "backend-service-token",
		HTTPClient:   http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("NewBackendClient: %v", err)
	}
	return client
}
