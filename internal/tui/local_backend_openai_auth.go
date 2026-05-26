package tui

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
)

const (
	openAIAuthRedirectURI = "http://localhost:1455/auth/callback"
	openAIAuthOriginator  = "codex_cli"
)

type openAIAuthFlow struct {
	id               string
	state            string
	codeVerifier     string
	redirectURI      string
	authorizationURL string
	listener         net.Listener
	server           *http.Server
	callbackCh       chan openAIAuthCallback
}

type openAIAuthCallback struct {
	code string
	err  error
}

func (b *LocalBackend) BeginOpenAIAuth(_ context.Context) (app.OpenAIAuthChallenge, error) {
	if b == nil || b.runtime == nil {
		return app.OpenAIAuthChallenge{}, nil
	}
	b.closeOpenAIAuthFlow("")

	flow, err := newOpenAIAuthFlow()
	if err != nil {
		return app.OpenAIAuthChallenge{}, err
	}

	b.openAIAuthMu.Lock()
	b.openAIAuthFlow = flow
	b.openAIAuthMu.Unlock()

	return app.OpenAIAuthChallenge{
		FlowID:           flow.id,
		AuthorizationURL: flow.authorizationURL,
		RedirectURI:      flow.redirectURI,
	}, nil
}

func (b *LocalBackend) CompleteOpenAIAuth(ctx context.Context, challenge app.OpenAIAuthChallenge) (app.DialogState, error) {
	if b == nil || b.runtime == nil {
		return app.DialogState{}, nil
	}
	flow, err := b.openAIAuthFlowForID(challenge.FlowID)
	if err != nil {
		return app.DialogState{}, err
	}
	defer b.closeOpenAIAuthFlow(flow.id)

	callback, err := flow.wait(ctx)
	if err != nil {
		return app.DialogState{}, err
	}
	entry, err := b.exchangeOpenAIOAuthCode(ctx, provider.OpenAIOAuthCodeExchangeRequest{
		Code:         callback.code,
		RedirectURI:  flow.redirectURI,
		CodeVerifier: flow.codeVerifier,
	})
	if err != nil {
		return app.DialogState{}, err
	}

	if err := b.applyReconfigurableMutation(func(configStore *app.ConfigStore, auth *provider.AuthStore) error {
		if err := configStore.UpsertProvider(app.ProviderConnectionInput{
			ProviderID: "openai",
		}); err != nil {
			return err
		}
		return auth.Set("openai", *entry)
	}); err != nil {
		return app.DialogState{}, err
	}

	return b.runtime.DialogState()
}

func (b *LocalBackend) openAIAuthFlowForID(flowID string) (*openAIAuthFlow, error) {
	b.openAIAuthMu.Lock()
	defer b.openAIAuthMu.Unlock()
	if b.openAIAuthFlow == nil {
		return nil, errors.New("openai auth flow is not active")
	}
	if strings.TrimSpace(flowID) != "" && b.openAIAuthFlow.id != strings.TrimSpace(flowID) {
		return nil, errors.New("openai auth flow does not match the active request")
	}
	return b.openAIAuthFlow, nil
}

func (b *LocalBackend) closeOpenAIAuthFlow(flowID string) {
	b.openAIAuthMu.Lock()
	flow := b.openAIAuthFlow
	if flow == nil {
		b.openAIAuthMu.Unlock()
		return
	}
	if strings.TrimSpace(flowID) != "" && flow.id != strings.TrimSpace(flowID) {
		b.openAIAuthMu.Unlock()
		return
	}
	b.openAIAuthFlow = nil
	b.openAIAuthMu.Unlock()
	flow.close()
}

func newOpenAIAuthFlow() (*openAIAuthFlow, error) {
	state, err := randomOAuthString(24)
	if err != nil {
		return nil, err
	}
	codeVerifier, err := randomOAuthString(48)
	if err != nil {
		return nil, err
	}
	flowID, err := randomOAuthString(18)
	if err != nil {
		return nil, err
	}
	authorizationURL, err := provider.BuildOpenAIOAuthAuthorizeURL(
		openAIAuthRedirectURI,
		state,
		provider.OpenAIOAuthCodeChallenge(codeVerifier),
		openAIAuthOriginator,
	)
	if err != nil {
		return nil, err
	}

	flow := &openAIAuthFlow{
		id:               flowID,
		state:            state,
		codeVerifier:     codeVerifier,
		redirectURI:      openAIAuthRedirectURI,
		authorizationURL: authorizationURL,
		callbackCh:       make(chan openAIAuthCallback, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", flow.handleCallback)
	flow.server = &http.Server{Handler: mux}

	listener, err := net.Listen("tcp", "localhost:1455")
	if err != nil {
		return nil, fmt.Errorf("openai oauth: listen on localhost:1455: %w", err)
	}
	flow.listener = listener

	go func() {
		if err := flow.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			flow.sendCallback(openAIAuthCallback{err: fmt.Errorf("openai oauth: callback server: %w", err)})
		}
	}()

	return flow, nil
}

func (f *openAIAuthFlow) wait(ctx context.Context) (openAIAuthCallback, error) {
	select {
	case <-ctx.Done():
		return openAIAuthCallback{}, ctx.Err()
	case callback := <-f.callbackCh:
		if callback.err != nil {
			return openAIAuthCallback{}, callback.err
		}
		return callback, nil
	}
}

func (f *openAIAuthFlow) handleCallback(w http.ResponseWriter, r *http.Request) {
	if errText := strings.TrimSpace(r.URL.Query().Get("error")); errText != "" {
		description := strings.TrimSpace(r.URL.Query().Get("error_description"))
		if description != "" {
			errText += ": " + description
		}
		f.writeCallbackResponse(w, http.StatusBadRequest, "OpenAI sign-in failed. Return to KodaCode for details.")
		f.sendCallback(openAIAuthCallback{err: errors.New(errText)})
		return
	}

	if strings.TrimSpace(r.URL.Query().Get("state")) != f.state {
		f.writeCallbackResponse(w, http.StatusBadRequest, "OpenAI sign-in state mismatch. Return to KodaCode and try again.")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		f.writeCallbackResponse(w, http.StatusBadRequest, "OpenAI did not return an authorization code. Return to KodaCode and try again.")
		return
	}

	f.writeCallbackResponse(w, http.StatusOK, "OpenAI sign-in complete. You can return to KodaCode.")
	f.sendCallback(openAIAuthCallback{code: code})
}

func (f *openAIAuthFlow) writeCallbackResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("<!doctype html><html><body><p>" + message + "</p></body></html>"))
}

func (f *openAIAuthFlow) sendCallback(callback openAIAuthCallback) {
	select {
	case f.callbackCh <- callback:
	default:
	}
}

func (f *openAIAuthFlow) close() {
	if f == nil || f.server == nil {
		return
	}
	_ = f.server.Close()
}

func randomOAuthString(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("oauth random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
