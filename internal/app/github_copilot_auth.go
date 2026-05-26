package app

type GitHubCopilotAuthChallenge struct {
	BaseURL         string
	DeviceCode      string
	UserCode        string
	VerificationURL string
	ExpiresIn       int
	Interval        int
}
