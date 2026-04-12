package openai

import (
	"fmt"
	"net/http"
	"runtime"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/sageil/kodacode/v1/internal/provider"
)

const copilotBaseURL = "https://api.githubcopilot.com"

// NewWithCopilotAuth creates an OpenAI-compatible provider that authenticates
// using a GitHub OAuth token (gho_). The token is used directly as a Bearer
// token. If a session token exchange is available, CopilotAuth handles it
// transparently.
func NewWithCopilotAuth(auth *provider.CopilotAuth, models []provider.Model) *Client {
	middleware := func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		token := auth.Token()
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Copilot-Integration-Id", "vscode-chat")
		req.Header.Set("Editor-Version", "vscode/1.100.0")
		req.Header.Set("User-Agent", fmt.Sprintf("kodacode/1.0 (%s %s)", runtime.GOOS, runtime.GOARCH))
		return next(req)
	}

	client := openaisdk.NewClient(
		option.WithAPIKey("copilot-placeholder"),
		option.WithBaseURL(copilotBaseURL),
		option.WithMiddleware(middleware),
	)

	return &Client{
		id:        "github-copilot",
		name:      "GitHub Copilot",
		models:    models,
		sdkClient: client,
	}
}
