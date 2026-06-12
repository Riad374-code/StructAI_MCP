package mcp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Riad374-code/architectmcp/internal/mcp/session"
)

const (
	MCPSSEPath         = "/mcp/sse"
	MCPMessagePath     = "/mcp/message"
	HealthPath         = "/healthz"
	APIKeyHeader       = "X-API-Key"
	maxRequestBodySize = 32 << 20
	readHeaderTimeout  = 10 * time.Second
	requestTimeout     = 30 * time.Second
	idleTimeout        = 60 * time.Second
	shutdownTimeout    = 10 * time.Second
)

// NewHTTPHandler builds the hosted-only HTTP surface. Health checks are
// public; every MCP request requires a validated API key.
func NewHTTPHandler(authorizer APIKeyAuthorizer, options ...ServerOption) (http.Handler, error) {
	if authorizer == nil {
		return nil, fmt.Errorf("API key authorizer is required")
	}

	mcpServer := NewServer(options...)
	mcpHandler := newSSEHTTPHandler(authorizer, mcpServer)

	mux := http.NewServeMux()
	mux.Handle(MCPSSEPath, mcpHandler)
	mux.Handle(MCPMessagePath, mcpHandler)
	mux.HandleFunc(HealthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return securityHeaders(mux), nil
}

type sseHTTPHandler struct {
	authorizer APIKeyAuthorizer
	server     *sdk.Server

	mu       sync.Mutex
	sessions map[string]sseSession
}

type sseSession struct {
	transport *sdk.SSEServerTransport
	principal session.Principal
}

func newSSEHTTPHandler(authorizer APIKeyAuthorizer, server *sdk.Server) http.Handler {
	return &sseHTTPHandler{
		authorizer: authorizer,
		server:     server,
		sessions:   make(map[string]sseSession),
	}
}

func (h *sseHTTPHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case MCPSSEPath:
		h.serveSSE(w, request)
	case MCPMessagePath:
		h.serveMessage(w, request)
	default:
		http.NotFound(w, request)
	}
}

func (h *sseHTTPHandler) serveSSE(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, ok := authenticateRequest(w, request, h.authorizer)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sessionID := rand.Text()
	endpoint := MCPMessagePath + "?sessionid=" + url.QueryEscape(sessionID)
	transport := &sdk.SSEServerTransport{Endpoint: endpoint, Response: w}

	h.mu.Lock()
	h.sessions[sessionID] = sseSession{transport: transport, principal: principal}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.sessions, sessionID)
		h.mu.Unlock()
	}()

	ctx := session.WithPrincipal(request.Context(), principal)
	sessionTransport := &sseSessionTransport{
		delegate:  transport,
		sessionID: sessionID,
	}
	serverSession, err := h.server.Connect(ctx, sessionTransport, nil)
	if err != nil {
		http.Error(w, "connection failed", http.StatusInternalServerError)
		return
	}
	defer serverSession.Close()

	done := make(chan struct{})
	go func() {
		_ = serverSession.Wait()
		close(done)
	}()

	select {
	case <-request.Context().Done():
	case <-done:
	}
}

func (h *sseHTTPHandler) serveMessage(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key, ok := requestAPIKey(request.Header)
	if !ok {
		writeAuthError(w, http.StatusUnauthorized)
		return
	}
	if !hasJSONContentType(request.Header.Get("Content-Type")) {
		http.Error(w, "Content-Type must be 'application/json'", http.StatusUnsupportedMediaType)
		return
	}
	sessionID := request.URL.Query().Get("sessionid")
	if sessionID == "" {
		http.Error(w, "sessionid must be provided", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	currentSession, ok := h.sessions[sessionID]
	h.mu.Unlock()
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if !credentialMatches(key, currentSession.principal.CredentialHash) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	request.Body = http.MaxBytesReader(w, request.Body, maxRequestBodySize)
	currentSession.transport.ServeHTTP(w, request)
}

func credentialMatches(key, expectedHash string) bool {
	actualHash := credentialFingerprint(key)
	if len(actualHash) != len(expectedHash) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) == 1
}

type sseSessionTransport struct {
	delegate  *sdk.SSEServerTransport
	sessionID string
}

func (t *sseSessionTransport) Connect(ctx context.Context) (sdk.Connection, error) {
	connection, err := t.delegate.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &sseSessionConnection{
		Connection: connection,
		sessionID:  t.sessionID,
	}, nil
}

type sseSessionConnection struct {
	sdk.Connection
	sessionID string
}

func (c *sseSessionConnection) SessionID() string {
	return c.sessionID
}

func authenticateRequest(
	w http.ResponseWriter,
	request *http.Request,
	authorizer APIKeyAuthorizer,
) (session.Principal, bool) {
	key, ok := requestAPIKey(request.Header)
	if !ok {
		writeAuthError(w, http.StatusUnauthorized)
		return session.Principal{}, false
	}
	principal, err := authorizer.AuthorizeAPIKey(request.Context(), key)
	if err != nil {
		if errors.Is(err, ErrInvalidAPIKey) {
			writeAuthError(w, http.StatusUnauthorized)
			return session.Principal{}, false
		}
		if errors.Is(err, ErrInsufficientBalance) {
			http.Error(w, "payment required", http.StatusPaymentRequired)
			return session.Principal{}, false
		}
		if errors.Is(err, ErrRateLimited) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return session.Principal{}, false
		}
		http.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
		return session.Principal{}, false
	}
	return principalWithCredentialHash(principal, key), true
}

func principalWithCredentialHash(principal session.Principal, key string) session.Principal {
	if principal.CredentialHash != "" {
		return principal
	}
	return session.Principal{
		AccountID:          principal.AccountID,
		KeyID:              principal.KeyID,
		Plan:               principal.Plan,
		Features:           append([]string(nil), principal.Features...),
		PlanCallsRemaining: principal.PlanCallsRemaining,
		CredentialHash:     credentialFingerprint(key),
		BackendToken:       principal.BackendToken,
	}
}

func hasJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}

func bearerKey(authorization string) (string, bool) {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func requestAPIKey(header http.Header) (string, bool) {
	directKey := strings.TrimSpace(header.Get(APIKeyHeader))
	bearer, hasBearer := bearerKey(header.Get("Authorization"))
	if directKey == "" {
		return bearer, hasBearer
	}
	if hasBearer && !credentialMatches(bearer, credentialFingerprint(directKey)) {
		return "", false
	}
	return directKey, true
}

func writeAuthError(w http.ResponseWriter, status int) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, "unauthorized", status)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, request)
	})
}

// Run serves the hosted MCP API until the context is cancelled.
func Run(ctx context.Context, addr string, authorizer APIKeyAuthorizer, options ...ServerOption) error {
	handler, err := NewHTTPHandler(authorizer, options...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("HTTP listen address is required")
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       requestTimeout,
		IdleTimeout:       idleTimeout,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		return nil
	}
}
