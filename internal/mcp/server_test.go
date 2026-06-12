package mcp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Riad374-code/architectmcp/internal/mcp/session"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNewServerRegistersHostedToolsWithoutPanic(t *testing.T) {
	server := NewServer()
	if server == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestNewHTTPHandlerRequiresAPIKeyValidator(t *testing.T) {
	_, err := NewHTTPHandler(nil)
	if err == nil {
		t.Fatal("expected missing API key validator error")
	}
}

func TestHTTPHandlerAllowsUnauthenticatedHealthCheck(t *testing.T) {
	handler, err := NewHTTPHandler(APIKeyAuthorizerFunc(
		func(context.Context, string) (session.Principal, error) {
			return session.Principal{}, errors.New("should not be called")
		},
	))
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, HealthPath, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestHTTPHandlerRejectsMissingOrInvalidBearerKey(t *testing.T) {
	handler, err := NewHTTPHandler(testAuthorizer(map[string]session.Principal{
		"valid-key": {AccountID: "acct_123", KeyID: "key_123"},
	}))
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}

	tests := []struct {
		name          string
		path          string
		method        string
		authorization string
	}{
		{name: "missing SSE key", path: MCPSSEPath, method: http.MethodGet},
		{name: "wrong scheme", path: MCPSSEPath, method: http.MethodGet, authorization: "Basic abc"},
		{name: "invalid key", path: MCPSSEPath, method: http.MethodGet, authorization: "Bearer invalid-key"},
		{name: "missing message key", path: MCPMessagePath + "?sessionid=missing", method: http.MethodPost},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader("{}"))
			request.Header.Set("Authorization", test.authorization)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
			if response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing Bearer authentication challenge")
			}
		})
	}
}

func TestHTTPHandlerReportsValidatorOutage(t *testing.T) {
	handler, err := NewHTTPHandler(APIKeyAuthorizerFunc(
		func(context.Context, string) (session.Principal, error) {
			return session.Principal{}, errors.New("database unavailable")
		},
	))
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, MCPSSEPath, nil)
	request.Header.Set("Authorization", "Bearer valid-looking-key")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestHTTPHandlerRejectsOversizedMCPBody(t *testing.T) {
	handler, err := NewHTTPHandler(testAuthorizer(map[string]session.Principal{
		"valid-key": {AccountID: "acct_123", KeyID: "key_123"},
	}))
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	response, endpoint := openSSEStream(t, httpServer.URL, "valid-key")
	defer response.Body.Close()

	request, err := http.NewRequest(
		http.MethodPost,
		httpServer.URL+endpoint,
		strings.NewReader(strings.Repeat("x", maxRequestBodySize+1)),
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("Authorization", "Bearer valid-key")
	request.Header.Set("Content-Type", "application/json")

	postResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST oversized message: %v", err)
	}
	defer postResponse.Body.Close()

	if postResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", postResponse.StatusCode, http.StatusBadRequest)
	}
}

func TestHTTPHandlerUsesSSEMessageEndpoint(t *testing.T) {
	handler, err := NewHTTPHandler(testAuthorizer(map[string]session.Principal{
		"valid-key": {AccountID: "acct_123", KeyID: "key_123"},
	}))
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	response, endpoint := openSSEStream(t, httpServer.URL, "valid-key")
	defer response.Body.Close()

	if !strings.HasPrefix(endpoint, MCPMessagePath+"?sessionid=") {
		t.Fatalf("endpoint = %q, want %s session endpoint", endpoint, MCPMessagePath)
	}
}

func TestHTTPHandlerRejectsMessageKeyMismatch(t *testing.T) {
	handler, err := NewHTTPHandler(testAuthorizer(map[string]session.Principal{
		"first-key":  {AccountID: "acct_123", KeyID: "key_123"},
		"second-key": {AccountID: "acct_999", KeyID: "key_999"},
	}))
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	response, endpoint := openSSEStream(t, httpServer.URL, "first-key")
	defer response.Body.Close()

	request, err := http.NewRequest(
		http.MethodPost,
		httpServer.URL+endpoint,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`),
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("Authorization", "Bearer second-key")
	request.Header.Set("Content-Type", "application/json")

	postResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST message: %v", err)
	}
	defer postResponse.Body.Close()

	if postResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", postResponse.StatusCode, http.StatusForbidden)
	}
}

func TestBearerKeyParsing(t *testing.T) {
	tests := []struct {
		name   string
		header string
		wantOK bool
		want   string
	}{
		{name: "valid", header: "Bearer abc", wantOK: true, want: "abc"},
		{name: "case-insensitive scheme", header: "bearer abc", wantOK: true, want: "abc"},
		{name: "missing", header: "", wantOK: false},
		{name: "wrong scheme", header: "Basic abc", wantOK: false},
		{name: "too many parts", header: "Bearer abc def", wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := bearerKey(test.header)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if got != test.want {
				t.Fatalf("key = %q, want %q", got, test.want)
			}
		})
	}
}

func TestHTTPHandlerServesMCPForValidBearerKey(t *testing.T) {
	handler, err := NewHTTPHandler(testAuthorizer(map[string]session.Principal{
		"valid-key": {AccountID: "acct_123", KeyID: "key_123"},
	}))
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	clientSession := connectSSEClient(t, httpServer.URL, "valid-key")
	defer clientSession.Close()

	if err := clientSession.Ping(context.Background(), nil); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestHTTPHandlerExposesHostedTools(t *testing.T) {
	handler, err := NewHTTPHandler(testAuthorizer(map[string]session.Principal{
		"valid-key": {AccountID: "acct_123", KeyID: "key_123"},
	}))
	if err != nil {
		t.Fatalf("NewHTTPHandler: %v", err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	clientSession := connectSSEClient(t, httpServer.URL, "valid-key")
	defer clientSession.Close()

	result, err := clientSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	toolNames := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}
	if !toolNames["architect_plan"] {
		t.Fatalf("architect_plan missing from tools/list: %#v", result.Tools)
	}
	if !toolNames["architect_check"] {
		t.Fatalf("architect_check missing from tools/list: %#v", result.Tools)
	}
}

func TestStaticAPIKeyValidator(t *testing.T) {
	validator, err := NewStaticAPIKeyValidator([]string{" first-key ", "second-key"})
	if err != nil {
		t.Fatalf("NewStaticAPIKeyValidator: %v", err)
	}
	if err := validator.ValidateAPIKey(context.Background(), "first-key"); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	if err := validator.ValidateAPIKey(context.Background(), "wrong-key"); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("invalid key error = %v, want ErrInvalidAPIKey", err)
	}
}

func TestStaticAPIKeyValidatorRejectsEmptyConfiguration(t *testing.T) {
	if _, err := NewStaticAPIKeyValidator(nil); err == nil {
		t.Fatal("expected empty key configuration error")
	}
	if _, err := NewStaticAPIKeyValidator([]string{" "}); err == nil {
		t.Fatal("expected blank key configuration error")
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	err := Run(context.Background(), ":0", nil)
	if err == nil {
		t.Fatal("expected missing validator error")
	}

	validator := APIKeyValidatorFunc(func(context.Context, string) error { return nil })
	err = Run(context.Background(), " ", validator)
	if err == nil {
		t.Fatal("expected missing listen address error")
	}
}

func testAuthorizer(keys map[string]session.Principal) APIKeyAuthorizer {
	return APIKeyAuthorizerFunc(func(_ context.Context, key string) (session.Principal, error) {
		principal, ok := keys[key]
		if !ok {
			return session.Principal{}, ErrInvalidAPIKey
		}
		return principalWithCredentialHash(principal, key), nil
	})
}

func connectSSEClient(t *testing.T, baseURL string, key string) *sdk.ClientSession {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(context.Background(), &sdk.SSEClientTransport{
		Endpoint:   baseURL + MCPSSEPath,
		HTTPClient: bearerHTTPClient(key),
	}, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	return clientSession
}

func bearerHTTPClient(key string) *http.Client {
	return &http.Client{
		Transport: bearerRoundTripper{
			key:  key,
			next: http.DefaultTransport,
		},
	}
}

type bearerRoundTripper struct {
	key  string
	next http.RoundTripper
}

func (t bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+t.key)
	return t.next.RoundTrip(clone)
}

func openSSEStream(t *testing.T, baseURL string, key string) (*http.Response, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+MCPSSEPath, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET SSE: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("SSE status = %d, body=%s", response.StatusCode, string(body))
	}
	reader := bufio.NewReader(response.Body)
	var endpoint string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			response.Body.Close()
			t.Fatalf("read endpoint event: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			endpoint = strings.TrimPrefix(line, "data: ")
		}
		if line == "" && endpoint != "" {
			return response, endpoint
		}
	}
}
