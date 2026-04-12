package provider

// CopilotAuth holds a GitHub OAuth token (gho_) for Copilot API access.
type CopilotAuth struct {
	ghoToken string
}

// NewCopilotAuth creates a CopilotAuth from a GitHub OAuth token.
func NewCopilotAuth(ghoToken string) *CopilotAuth {
	return &CopilotAuth{ghoToken: ghoToken}
}

// Token returns the GitHub OAuth token for use as a Bearer token.
func (ca *CopilotAuth) Token() string {
	return ca.ghoToken
}

// ReadCopilotTokenFromNeovim reads the gho_ token from the Neovim/Vim Copilot
// plugin config at ~/.config/github-copilot/hosts.json.
func ReadCopilotTokenFromNeovim() (string, error) {
	return readCopilotTokenFromFile("~/.config/github-copilot/hosts.json", "neovim")
}

// ReadCopilotTokenFromOpencode reads the gho_ token from opencode's auth store
// at ~/.local/share/opencode/auth.json.
func ReadCopilotTokenFromOpencode() (string, error) {
	return readCopilotTokenFromFile("~/.local/share/opencode/auth.json", "opencode")
}
