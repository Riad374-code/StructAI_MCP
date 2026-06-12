package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Riad374-code/architectmcp/internal/mcp/session"
)

// ErrInvalidAPIKey is returned by validators when a key is unknown, revoked,
// or otherwise unauthorized. Callers must not expose more specific reasons.
var ErrInvalidAPIKey = errors.New("invalid API key")

// ErrInsufficientBalance is returned when the key is valid but cannot spend a
// billable tool call.
var ErrInsufficientBalance = errors.New("insufficient balance")

// ErrRateLimited is returned when the backend rejects the request for abuse or
// quota-rate reasons.
var ErrRateLimited = errors.New("rate limited")

// APIKeyAuthorizer authenticates one Bearer key and returns the backend-derived
// account principal. The product backend should implement this interface with
// its API-key, subscription, and balance store.
type APIKeyAuthorizer interface {
	AuthorizeAPIKey(ctx context.Context, key string) (session.Principal, error)
}

// APIKeyAuthorizerFunc adapts a function into an APIKeyAuthorizer.
type APIKeyAuthorizerFunc func(context.Context, string) (session.Principal, error)

// AuthorizeAPIKey authorizes one API key.
func (f APIKeyAuthorizerFunc) AuthorizeAPIKey(
	ctx context.Context,
	key string,
) (session.Principal, error) {
	return f(ctx, key)
}

// APIKeyValidator authenticates one Bearer key without returning a principal.
// It remains for bootstrap/static deployments; SaaS deployments should use
// APIKeyAuthorizer.
type APIKeyValidator interface {
	ValidateAPIKey(ctx context.Context, key string) error
}

// APIKeyValidatorFunc adapts a function into an APIKeyValidator.
type APIKeyValidatorFunc func(context.Context, string) error

func (f APIKeyValidatorFunc) ValidateAPIKey(ctx context.Context, key string) error {
	return f(ctx, key)
}

// AuthorizeAPIKey adapts legacy validators into authorizers for tests and
// bootstrap deployments.
func (f APIKeyValidatorFunc) AuthorizeAPIKey(
	ctx context.Context,
	key string,
) (session.Principal, error) {
	if err := f.ValidateAPIKey(ctx, key); err != nil {
		return session.Principal{}, err
	}
	return staticPrincipalForKey(key), nil
}

// StaticAPIKeyValidator is a deployment bootstrap for environments that load
// keys from a secret store into process configuration. It stores only SHA-256
// digests and compares every configured digest in constant time.
type StaticAPIKeyValidator struct {
	keyHashes [][sha256.Size]byte
}

func NewStaticAPIKeyValidator(keys []string) (*StaticAPIKeyValidator, error) {
	hashes := make([][sha256.Size]byte, 0, len(keys))
	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		hashes = append(hashes, sha256.Sum256([]byte(key)))
	}
	if len(hashes) == 0 {
		return nil, fmt.Errorf("at least one non-empty API key is required")
	}
	return &StaticAPIKeyValidator{keyHashes: hashes}, nil
}

func (v *StaticAPIKeyValidator) ValidateAPIKey(_ context.Context, key string) error {
	candidate := sha256.Sum256([]byte(key))
	isValid := 0
	for _, expected := range v.keyHashes {
		isValid |= subtle.ConstantTimeCompare(candidate[:], expected[:])
	}
	if isValid != 1 {
		return ErrInvalidAPIKey
	}
	return nil
}

// AuthorizeAPIKey authenticates a statically configured key and returns a
// non-secret bootstrap principal. Production SaaS deployments should use the
// backend authorizer instead.
func (v *StaticAPIKeyValidator) AuthorizeAPIKey(
	ctx context.Context,
	key string,
) (session.Principal, error) {
	if err := v.ValidateAPIKey(ctx, key); err != nil {
		return session.Principal{}, err
	}
	return staticPrincipalForKey(key), nil
}

func staticPrincipalForKey(key string) session.Principal {
	fingerprint := credentialFingerprint(key)
	return session.Principal{
		AccountID:      "static",
		KeyID:          "static-" + fingerprint[:8],
		Plan:           "bootstrap",
		Features:       []string{"architect_plan", "architect_check"},
		CredentialHash: fingerprint,
	}
}

func credentialFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
