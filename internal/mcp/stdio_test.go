package mcp

import (
	"strings"
	"testing"
)

func TestFilteredEnv_OnlyAllowlistedVars(t *testing.T) {
	// Set a known safe and a known unsafe env var.
	t.Setenv("HOME", "/test/home")
	t.Setenv("SECRET_API_KEY", "leaked")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-secret")
	t.Setenv("OPENAI_API_KEY", "sk-openai-secret")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA-secret")

	result := filteredEnv(nil)

	env := make(map[string]string)
	for _, entry := range result {
		k, v, _ := strings.Cut(entry, "=")
		env[k] = v
	}

	// Safe vars should be present.
	if env["HOME"] != "/test/home" {
		t.Errorf("HOME not inherited: got %q", env["HOME"])
	}

	// Unsafe vars must NOT be present.
	for _, key := range []string{"SECRET_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "AWS_ACCESS_KEY_ID"} {
		if _, ok := env[key]; ok {
			t.Errorf("unsafe var %q leaked to subprocess env", key)
		}
	}

	// Only allowlisted keys should be present (from parent).
	for _, entry := range result {
		k, _, _ := strings.Cut(entry, "=")
		if !safeEnvKeys[k] {
			t.Errorf("non-allowlisted key %q found in filtered env", k)
		}
	}
}

func TestFilteredEnv_UserConfigOverrides(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")

	extra := map[string]string{
		"MY_TOKEN":  "value123",
		"NODE_PATH": "/custom/node",
	}
	result := filteredEnv(extra)

	env := make(map[string]string)
	for _, entry := range result {
		k, v, _ := strings.Cut(entry, "=")
		env[k] = v
	}

	// User-configured vars should be present.
	if env["MY_TOKEN"] != "value123" {
		t.Errorf("user env MY_TOKEN not set: got %q", env["MY_TOKEN"])
	}
	if env["NODE_PATH"] != "/custom/node" {
		t.Errorf("user env NODE_PATH not set: got %q", env["NODE_PATH"])
	}
	// Safe parent var should also be present.
	if env["PATH"] != "/usr/bin" {
		t.Errorf("PATH not inherited: got %q", env["PATH"])
	}
}

func TestFilteredEnv_UserConfigCanOverrideSafeKeys(t *testing.T) {
	t.Setenv("PATH", "/original/path")

	extra := map[string]string{
		"PATH": "/custom/path",
	}
	result := filteredEnv(extra)

	// Both entries may appear; the last one wins for exec.
	// Verify at least the custom one is present.
	found := false
	for _, entry := range result {
		if entry == "PATH=/custom/path" {
			found = true
		}
	}
	if !found {
		t.Error("user override PATH=/custom/path not found in env")
	}
}

func TestSafeEnvKeys_ComprehensiveCheck(t *testing.T) {
	// Verify well-known secret env var names are NOT in the safe list.
	dangerous := []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"GITHUB_TOKEN",
		"DATABASE_URL",
		"SECRET_KEY",
		"GOOGLE_APPLICATION_CREDENTIALS",
	}
	for _, key := range dangerous {
		if safeEnvKeys[key] {
			t.Errorf("dangerous key %q is in safeEnvKeys allowlist", key)
		}
	}

	// Verify expected safe keys are present.
	expected := []string{"PATH", "HOME", "LANG", "TERM", "USER", "SHELL", "TMPDIR"}
	for _, key := range expected {
		if !safeEnvKeys[key] {
			t.Errorf("expected safe key %q missing from safeEnvKeys", key)
		}
	}

}
