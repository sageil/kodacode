package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/provider"
)

// runLogin handles the "kodacode login [provider]" subcommand.
func runLogin() error {
	providerID := "anthropic" // default
	if len(os.Args) > 2 {
		providerID = os.Args[2]
	}

	switch providerID {
	case "openai":
		return loginOpenAI()
	default:
		return fmt.Errorf("OAuth login is not supported for provider %q\ncurrently supported: openai\nfor other providers, set api_key in your config file", providerID)
	}
}

func loginOpenAI() error {
	pkce, err := provider.GeneratePKCE()
	if err != nil {
		return fmt.Errorf("generate PKCE: %w", err)
	}

	authURL := provider.OpenAIOAuthAuthorizeURL(pkce)

	fmt.Println("Opening browser for ChatGPT Pro/Plus OAuth login...")
	fmt.Printf("\nIf the browser doesn't open, visit:\n%s\n\n", authURL)
	openBrowser(authURL)

	fmt.Println("Waiting for browser callback on http://localhost:1455 ...")
	code, err := provider.OpenAIOAuthListenForCode(pkce.Verifier)
	if err != nil {
		return fmt.Errorf("listen for callback: %w", err)
	}

	entry, err := provider.OpenAIOAuthExchange(code, pkce.Verifier)
	if err != nil {
		return err
	}

	store := provider.NewAuthStore()
	if err := store.Set("openai", *entry); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	found := false
	for _, p := range cfg.Providers {
		if p.ID == "openai" {
			found = true
			break
		}
	}
	if !found {
		cfg.Providers = append(cfg.Providers, config.ProviderConfig{ID: "openai"})
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}

	fmt.Println("\nLogin successful! OpenAI OAuth credentials saved.")
	fmt.Println("kodacode will use your ChatGPT Pro/Plus subscription.")
	if entry.AccountID != "" {
		fmt.Printf("Account ID: %s\n", entry.AccountID)
	}
	return nil
}

// runLogout handles the "kodacode logout [provider]" subcommand.
func runLogout() error {
	providerID := "anthropic"
	if len(os.Args) > 2 {
		providerID = os.Args[2]
	}

	store := provider.NewAuthStore()
	if err := store.Remove(providerID); err != nil {
		return err
	}
	fmt.Printf("Logged out from %s. Will fall back to API key auth.\n", providerID)
	return nil
}

// openBrowser opens a URL in the user's default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
