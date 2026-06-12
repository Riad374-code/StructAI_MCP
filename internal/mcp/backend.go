package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	checkcontract "github.com/Riad374-code/architectmcp/internal/contracts/check"
	plancontract "github.com/Riad374-code/architectmcp/internal/contracts/plan"
	"github.com/Riad374-code/architectmcp/internal/mcp/session"
)

const (
	backendLicensePath        = "/v1/license/validate"
	backendPlanPath           = "/v1/mcp/tools/architect-plan"
	backendCheckPath          = "/v1/mcp/tools/architect-check"
	backendRequestTimeout     = 30 * time.Second
	maxBackendResponseBytes   = 32 << 20
	backendVersionPrefix      = "mcp/"
	backendReasonLimitReached = "limit_reached"
)

// BackendClientConfig configures the product backend integration.
type BackendClientConfig struct {
	BaseURL    string
	MachineID  string
	HTTPClient *http.Client
}

// BackendClient validates API keys and proxies tool calls to backend-owned
// business logic. It never logs raw API keys or backend session tokens.
type BackendClient struct {
	baseURL    string
	machineID  string
	httpClient *http.Client
}

type backendLicenseRequest struct {
	APIKey    string `json:"api_key"`
	MachineID string `json:"machine_id"`
	Version   string `json:"version"`
}

type backendLicenseResponse struct {
	Valid        bool                `json:"valid"`
	Reason       string              `json:"reason,omitempty"`
	Plan         string              `json:"plan,omitempty"`
	Entitlements backendEntitlements `json:"entitlements,omitempty"`
	Token        string              `json:"token,omitempty"`
	TokenTTL     int                 `json:"token_ttl_seconds,omitempty"`
}

type backendEntitlements struct {
	PlanCallsRemaining int      `json:"plan_calls_remaining"`
	Seats              int      `json:"seats"`
	Features           []string `json:"features"`
}

type backendPlanRequest struct {
	ToolName            string             `json:"tool_name"`
	ToolContractVersion string             `json:"tool_contract_version"`
	InputSchemaVersion  string             `json:"input_schema_version"`
	OutputSchemaVersion string             `json:"output_schema_version"`
	MCPVersion          string             `json:"mcp_version"`
	Input               plancontract.Input `json:"input"`
}

type backendPlanResponse struct {
	Output plancontract.Output `json:"output"`
}

type backendCheckRequest struct {
	ToolName            string              `json:"tool_name"`
	ToolContractVersion string              `json:"tool_contract_version"`
	InputSchemaVersion  string              `json:"input_schema_version"`
	OutputSchemaVersion string              `json:"output_schema_version"`
	MCPVersion          string              `json:"mcp_version"`
	Input               checkcontract.Input `json:"input"`
}

type backendCheckResponse struct {
	Output checkcontract.Output `json:"output"`
}

// NewBackendClient validates and creates a backend integration client.
func NewBackendClient(config BackendClientConfig) (*BackendClient, error) {
	baseURL, err := validateBackendURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	machineID := strings.TrimSpace(config.MachineID)
	if machineID == "" {
		return nil, fmt.Errorf("backend machine ID is required")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: backendRequestTimeout}
	}
	return &BackendClient{
		baseURL:    baseURL,
		machineID:  machineID,
		httpClient: httpClient,
	}, nil
}

// AuthorizeAPIKey exchanges a customer API key for a scoped backend session.
func (c *BackendClient) AuthorizeAPIKey(
	ctx context.Context,
	key string,
) (session.Principal, error) {
	var response backendLicenseResponse
	err := c.postJSON(ctx, backendLicensePath, "", key, backendLicenseRequest{
		APIKey:    key,
		MachineID: c.machineID,
		Version:   backendVersionPrefix + serverVersion,
	}, &response)
	if err != nil {
		return session.Principal{}, err
	}
	if !response.Valid {
		return session.Principal{}, authorizationReasonError(response.Reason)
	}
	if strings.TrimSpace(response.Token) == "" {
		return session.Principal{}, fmt.Errorf("backend license response missing session token")
	}
	fingerprint := credentialFingerprint(key)
	return session.Principal{
		AccountID:          "credential-" + fingerprint[:16],
		KeyID:              fingerprint[:16],
		Plan:               response.Plan,
		Features:           append([]string(nil), response.Entitlements.Features...),
		PlanCallsRemaining: response.Entitlements.PlanCallsRemaining,
		CredentialHash:     fingerprint,
		BackendToken:       response.Token,
	}, nil
}

// ExecutePlan delegates the full planning operation to backend business logic.
// The backend owns validation, persistence, idempotency, and billing.
func (c *BackendClient) ExecutePlan(
	ctx context.Context,
	backendToken string,
	input plancontract.Input,
) (plancontract.Output, error) {
	if strings.TrimSpace(backendToken) == "" {
		return plancontract.Output{}, fmt.Errorf("backend session token is required")
	}
	var response backendPlanResponse
	err := c.postJSON(ctx, backendPlanPath, backendToken, "", backendPlanRequest{
		ToolName:            plancontract.ToolName,
		ToolContractVersion: plancontract.ToolContractVersion,
		InputSchemaVersion:  plancontract.InputSchemaVersion,
		OutputSchemaVersion: plancontract.OutputSchemaVersion,
		MCPVersion:          backendVersionPrefix + serverVersion,
		Input:               input,
	}, &response)
	if err != nil {
		return plancontract.Output{}, err
	}
	if response.Output.Status == "" {
		return plancontract.Output{}, fmt.Errorf("backend plan response missing status")
	}
	return response.Output, nil
}

// ExecuteCheck delegates deterministic checking to backend business logic.
func (c *BackendClient) ExecuteCheck(
	ctx context.Context,
	backendToken string,
	input checkcontract.Input,
) (checkcontract.Output, error) {
	if strings.TrimSpace(backendToken) == "" {
		return checkcontract.Output{}, fmt.Errorf("backend session token is required")
	}
	var response backendCheckResponse
	err := c.postJSON(ctx, backendCheckPath, backendToken, "", backendCheckRequest{
		ToolName:            checkcontract.ToolName,
		ToolContractVersion: checkcontract.ToolContractVersion,
		InputSchemaVersion:  checkcontract.InputSchemaVersion,
		OutputSchemaVersion: checkcontract.OutputSchemaVersion,
		MCPVersion:          backendVersionPrefix + serverVersion,
		Input:               input,
	}, &response)
	if err != nil {
		return checkcontract.Output{}, err
	}
	if response.Output.Status == "" {
		return checkcontract.Output{}, fmt.Errorf("backend check response missing status")
	}
	return response.Output, nil
}

func authorizationReasonError(reason string) error {
	switch strings.TrimSpace(reason) {
	case backendReasonLimitReached:
		return ErrInsufficientBalance
	case "rate_limited":
		return ErrRateLimited
	case "key_revoked", "subscription_inactive", "key_not_found", "":
		return ErrInvalidAPIKey
	default:
		return ErrInvalidAPIKey
	}
}

func validateBackendURL(raw string) (string, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		return "", fmt.Errorf("backend base URL is required")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse backend base URL: %w", err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("backend base URL host is required")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", fmt.Errorf("backend base URL must use HTTPS except for loopback development")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *BackendClient) postJSON(
	ctx context.Context,
	path string,
	bearerToken string,
	apiKey string,
	body any,
	out any,
) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode backend request: %w", err)
	}
	endpoint, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return fmt.Errorf("build backend endpoint: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create backend request: %w", err)
	}
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	if apiKey != "" {
		request.Header.Set(APIKeyHeader, apiKey)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "ArchitectMCP/"+serverVersion)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call backend: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxBackendResponseBytes))
		return backendStatusError(response.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxBackendResponseBytes))
		return nil
	}
	limited := io.LimitReader(response.Body, maxBackendResponseBytes)
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("backend returned empty response")
		}
		return fmt.Errorf("decode backend response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func backendStatusError(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrInvalidAPIKey
	case http.StatusPaymentRequired:
		return ErrInsufficientBalance
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("backend returned status %d", status)
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing backend response: %w", err)
	}
	return fmt.Errorf("backend returned multiple JSON values")
}
