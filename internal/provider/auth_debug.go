package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type providerAuthDebugState struct {
	ProviderID         string
	FailurePhase       string
	AccessHash         string
	AccessExpiresAt    int64
	AccessExpiresInMs  int64
	TokenUpdateAt      int64
	TokenUpdateAgeMs   int64
	TokenUpdateSource  string
	TokenUpdateForced  bool
	TokenUpdateChanged bool
}

type openAIRequestAuthDebugger interface {
	AuthDebugState() providerAuthDebugState
}

func authDebugStateFor(authorizer openAIRequestAuthorizer) (providerAuthDebugState, bool) {
	debugger, ok := authorizer.(openAIRequestAuthDebugger)
	if !ok {
		return providerAuthDebugState{}, false
	}
	state := debugger.AuthDebugState()
	if strings.TrimSpace(state.ProviderID) == "" && strings.TrimSpace(state.AccessHash) == "" {
		return providerAuthDebugState{}, false
	}
	return state, true
}

func annotateAuthProviderError(authorizer openAIRequestAuthorizer, phase string, err error) error {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return err
	}
	if !LooksLikeAuthProviderResponse(providerErr.StatusCode, providerErr.Error()) {
		return err
	}
	state, ok := authDebugStateFor(authorizer)
	if !ok {
		return err
	}
	state.FailurePhase = strings.TrimSpace(phase)
	copyErr := *providerErr
	copyErr.AuthDebug = &state
	return &copyErr
}

func authTokenHash(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:6])
}

func authExpiryRemainingMillis(expiresAt int64, now time.Time) int64 {
	if expiresAt <= 0 {
		return 0
	}
	return expiresAt - now.UnixMilli()
}
