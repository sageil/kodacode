package permission_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/sageil/kodacode/v1/internal/permission"
)

// ---- Defaults ----

func TestDefaults_ContainsExpectedTools(t *testing.T) {
	d := permission.Defaults()
	tools := []string{"bash", "read", "edit", "glob", "external_directory"}
	for _, tool := range tools {
		if _, ok := d[tool]; !ok {
			t.Errorf("Defaults() missing tool %q", tool)
		}
	}
}

func TestDefaults_BashIsAllow(t *testing.T) {
	d := permission.Defaults()
	got := permission.Resolve(d, "bash", "echo hello")
	if got != permission.ActionAllow {
		t.Errorf("Resolve(defaults, \"bash\", \"echo hello\") = %q, want %q", got, permission.ActionAllow)
	}
}

func TestDefaults_ReadNormalFileIsAllow(t *testing.T) {
	d := permission.Defaults()
	got := permission.Resolve(d, "read", "/project/main.go")
	if got != permission.ActionAllow {
		t.Errorf("Resolve(defaults, \"read\", \"/project/main.go\") = %q, want %q", got, permission.ActionAllow)
	}
}

func TestDefaults_ReadEnvFileIsAsk(t *testing.T) {
	d := permission.Defaults()
	got := permission.Resolve(d, "read", "/project/.env")
	if got != permission.ActionAsk {
		t.Errorf("Resolve(defaults, \"read\", \"/project/.env\") = %q, want %q", got, permission.ActionAsk)
	}
}

func TestDefaults_ReadEnvDotXFileIsAsk(t *testing.T) {
	d := permission.Defaults()
	got := permission.Resolve(d, "read", "/project/.env.local")
	if got != permission.ActionAsk {
		t.Errorf("Resolve(defaults, \"read\", \"/project/.env.local\") = %q, want %q", got, permission.ActionAsk)
	}
}

func TestDefaults_ReadEnvExampleFileIsAllow(t *testing.T) {
	d := permission.Defaults()
	got := permission.Resolve(d, "read", "/project/.env.example")
	if got != permission.ActionAllow {
		t.Errorf("Resolve(defaults, \"read\", \"/project/.env.example\") = %q, want %q", got, permission.ActionAllow)
	}
}

func TestDefaults_EditIsAllow(t *testing.T) {
	d := permission.Defaults()
	got := permission.Resolve(d, "edit", "/project/main.go")
	if got != permission.ActionAllow {
		t.Errorf("Resolve(defaults, \"edit\", \"/project/main.go\") = %q, want %q", got, permission.ActionAllow)
	}
}

func TestDefaults_GlobIsAllow(t *testing.T) {
	d := permission.Defaults()
	got := permission.Resolve(d, "glob", "**/*.go")
	if got != permission.ActionAllow {
		t.Errorf("Resolve(defaults, \"glob\", \"**/*.go\") = %q, want %q", got, permission.ActionAllow)
	}
}

func TestDefaults_ExternalDirectoryIsAsk(t *testing.T) {
	d := permission.Defaults()
	got := permission.Resolve(d, "external_directory", "/tmp")
	if got != permission.ActionAsk {
		t.Errorf("Resolve(defaults, \"external_directory\", \"/tmp\") = %q, want %q", got, permission.ActionAsk)
	}
}

// ---- Resolve ----

func TestResolve_StringShorthandAllow(t *testing.T) {
	cfg := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionAllow},
	}
	got := permission.Resolve(cfg, "bash", "echo hello")
	if got != permission.ActionAllow {
		t.Errorf("Resolve(cfg, \"bash\", \"echo hello\") = %q, want %q", got, permission.ActionAllow)
	}
}

func TestResolve_StringShorthandDeny(t *testing.T) {
	cfg := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionDeny},
	}
	got := permission.Resolve(cfg, "bash", "rm -rf /")
	if got != permission.ActionDeny {
		t.Errorf("Resolve(cfg, \"bash\", \"rm -rf /\") = %q, want %q", got, permission.ActionDeny)
	}
}

func TestResolve_UnknownToolDefaultsToAsk(t *testing.T) {
	cfg := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionAllow},
	}
	got := permission.Resolve(cfg, "unknowntool", "")
	if got != permission.ActionAsk {
		t.Errorf("Resolve(cfg, \"unknowntool\", \"\") = %q, want %q", got, permission.ActionAsk)
	}
}

func TestResolve_NilConfigDefaultsToAsk(t *testing.T) {
	got := permission.Resolve(nil, "bash", "echo hello")
	if got != permission.ActionAsk {
		t.Errorf("Resolve(nil, \"bash\", \"echo hello\") = %q, want %q", got, permission.ActionAsk)
	}
}

func TestResolve_PatternLastMatchWins(t *testing.T) {
	// Patterns: "*" → allow, "*.env" → ask. Last match for ".env" is ask.
	cfg := permission.Config{
		"read": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "*", Action: permission.ActionAllow},
				{Glob: "*.env", Action: permission.ActionAsk},
			},
		},
	}
	got := permission.Resolve(cfg, "read", "/project/.env")
	if got != permission.ActionAsk {
		t.Errorf("Resolve(cfg, \"read\", \"/project/.env\") = %q, want %q", got, permission.ActionAsk)
	}
}

func TestResolve_PatternLastMatchWins_NormalFile(t *testing.T) {
	// For a normal .go file, only "*" matches, so the result is allow.
	cfg := permission.Config{
		"read": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "*", Action: permission.ActionAllow},
				{Glob: "*.env", Action: permission.ActionAsk},
			},
		},
	}
	got := permission.Resolve(cfg, "read", "/project/main.go")
	if got != permission.ActionAllow {
		t.Errorf("Resolve(cfg, \"read\", \"/project/main.go\") = %q, want %q", got, permission.ActionAllow)
	}
}

func TestResolve_PatternEnvExampleOverridesEnvStar(t *testing.T) {
	// "*.env.*" → ask comes before "*.env.example" → allow; last match wins.
	cfg := permission.Config{
		"read": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "*", Action: permission.ActionAllow},
				{Glob: "*.env", Action: permission.ActionAsk},
				{Glob: "*.env.*", Action: permission.ActionAsk},
				{Glob: "*.env.example", Action: permission.ActionAllow},
			},
		},
	}
	got := permission.Resolve(cfg, "read", "/project/.env.example")
	if got != permission.ActionAllow {
		t.Errorf("Resolve(cfg, \"read\", \"/project/.env.example\") = %q, want %q", got, permission.ActionAllow)
	}
}

func TestResolve_BashPatternMatchesFullCommand(t *testing.T) {
	cfg := permission.Config{
		"bash": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "*", Action: permission.ActionAllow},
				{Glob: "rm *", Action: permission.ActionAsk},
			},
		},
	}
	got := permission.Resolve(cfg, "bash", "rm /tmp/foo")
	if got != permission.ActionAsk {
		t.Errorf("Resolve(cfg, \"bash\", \"rm /tmp/foo\") = %q, want %q", got, permission.ActionAsk)
	}
}

func TestResolve_BashAllowNonMatchingCommand(t *testing.T) {
	cfg := permission.Config{
		"bash": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "*", Action: permission.ActionAllow},
				{Glob: "rm *", Action: permission.ActionAsk},
			},
		},
	}
	got := permission.Resolve(cfg, "bash", "echo hello")
	if got != permission.ActionAllow {
		t.Errorf("Resolve(cfg, \"bash\", \"echo hello\") = %q, want %q", got, permission.ActionAllow)
	}
}

func TestResolve_PatternsNoMatchDefaultsToAsk(t *testing.T) {
	// No catch-all "*", and "*.env" does not match "main.go".
	cfg := permission.Config{
		"read": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "*.env", Action: permission.ActionAsk},
			},
		},
	}
	got := permission.Resolve(cfg, "read", "main.go")
	if got != permission.ActionAsk {
		t.Errorf("Resolve(cfg, \"read\", \"main.go\") = %q, want %q", got, permission.ActionAsk)
	}
}

func TestResolve_PathPatternMatchesDirectorySubpath(t *testing.T) {
	cfg := permission.Config{
		"read": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "src/*.go", Action: permission.ActionAsk},
			},
		},
	}

	got := permission.Resolve(cfg, "read", "/project/src/main.go")
	if got != permission.ActionAsk {
		t.Errorf("Resolve(cfg, \"read\", \"/project/src/main.go\") = %q, want %q", got, permission.ActionAsk)
	}
}

func TestResolve_PathPatternMatchesAbsolutePath(t *testing.T) {
	cfg := permission.Config{
		"read": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "/tmp/*.log", Action: permission.ActionDeny},
			},
		},
	}

	got := permission.Resolve(cfg, "read", "/tmp/build.log")
	if got != permission.ActionDeny {
		t.Errorf("Resolve(cfg, \"read\", \"/tmp/build.log\") = %q, want %q", got, permission.ActionDeny)
	}
}

// ---- Merge ----

func TestMerge_OverlayOverridesBaseShorthand(t *testing.T) {
	base := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionAllow},
		"read": &permission.Rule{Action: permission.ActionAsk},
	}
	overlay := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionDeny},
	}
	got := permission.Merge(base, overlay)

	// bash: base "allow" + overlay "deny" → both expanded to [* → action],
	// overlay's [* → deny] comes last → deny wins.
	if action := permission.Resolve(got, "bash", "echo hello"); action != permission.ActionDeny {
		t.Errorf("Resolve(merged, bash) = %q, want %q", action, permission.ActionDeny)
	}
	// read: only in base, unchanged.
	if action := permission.Resolve(got, "read", "main.go"); action != permission.ActionAsk {
		t.Errorf("Resolve(merged, read) = %q, want %q", action, permission.ActionAsk)
	}
}

func TestMerge_NewOverlayToolIsAdded(t *testing.T) {
	base := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionAllow},
	}
	overlay := permission.Config{
		"edit": &permission.Rule{Action: permission.ActionDeny},
	}
	got := permission.Merge(base, overlay)
	if _, ok := got["edit"]; !ok {
		t.Errorf("Merge() missing overlay-only tool %q", "edit")
	}
}

func TestMerge_NilOverlayReturnsBaseCopy(t *testing.T) {
	base := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionAllow},
	}
	got := permission.Merge(base, nil)
	if diff := cmp.Diff(base, got); diff != "" {
		t.Errorf("Merge(base, nil) mismatch (-want +got):\n%s", diff)
	}
}

func TestMerge_NilBaseReturnsOverlayCopy(t *testing.T) {
	overlay := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionDeny},
	}
	got := permission.Merge(nil, overlay)
	if diff := cmp.Diff(overlay, got); diff != "" {
		t.Errorf("Merge(nil, overlay) mismatch (-want +got):\n%s", diff)
	}
}

func TestMerge_PatternLevelMerge_OverlayAddsPatterns(t *testing.T) {
	base := permission.Config{
		"read": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "*", Action: permission.ActionAllow},
				{Glob: "*.env", Action: permission.ActionAsk},
				{Glob: "*.env.*", Action: permission.ActionAsk},
				{Glob: "*.env.example", Action: permission.ActionAllow},
			},
		},
	}
	overlay := permission.Config{
		"read": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "*.env", Action: permission.ActionDeny},
			},
		},
	}
	got := permission.Merge(base, overlay)

	// .env → overlay's deny wins (last match).
	if action := permission.Resolve(got, "read", "/project/.env"); action != permission.ActionDeny {
		t.Errorf(".env: got %q, want %q", action, permission.ActionDeny)
	}
	// .env.example → base's allow still present (overlay didn't mention it).
	if action := permission.Resolve(got, "read", "/project/.env.example"); action != permission.ActionAllow {
		t.Errorf(".env.example: got %q, want %q", action, permission.ActionAllow)
	}
	// normal file → base's catch-all allow still present.
	if action := permission.Resolve(got, "read", "/project/main.go"); action != permission.ActionAllow {
		t.Errorf("main.go: got %q, want %q", action, permission.ActionAllow)
	}
}

func TestMerge_ShorthandAndPatterns(t *testing.T) {
	// Base is shorthand "allow", overlay adds specific pattern.
	base := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionAllow},
	}
	overlay := permission.Config{
		"bash": &permission.Rule{
			Patterns: []permission.Pattern{
				{Glob: "rm *", Action: permission.ActionDeny},
			},
		},
	}
	got := permission.Merge(base, overlay)

	// Normal commands: base's [* → allow] still applies.
	if action := permission.Resolve(got, "bash", "echo hello"); action != permission.ActionAllow {
		t.Errorf("echo: got %q, want %q", action, permission.ActionAllow)
	}
	// rm: overlay's deny wins.
	if action := permission.Resolve(got, "bash", "rm -rf /tmp"); action != permission.ActionDeny {
		t.Errorf("rm: got %q, want %q", action, permission.ActionDeny)
	}
}

func TestMerge_DefaultsPreservedWithProjectOverride(t *testing.T) {
	// Simulates: defaults → project config that sets read: allow.
	// The project shorthand should not wipe out the default .env protections.
	defaults := permission.Defaults()
	project := permission.Config{
		"read": &permission.Rule{Action: permission.ActionAllow},
	}
	got := permission.Merge(defaults, project)

	// .env.example → default's allow preserved (project's [* → allow] also matches,
	// but the default *.env.example → allow came before; project's * → allow is last).
	// Either way, result is allow.
	if action := permission.Resolve(got, "read", "/project/.env.example"); action != permission.ActionAllow {
		t.Errorf(".env.example: got %q, want %q", action, permission.ActionAllow)
	}
	// .env → default's [*.env → ask] came before project's [* → allow];
	// project's catch-all is last match → allow.
	if action := permission.Resolve(got, "read", "/project/.env"); action != permission.ActionAllow {
		t.Errorf(".env with project allow override: got %q, want %q", action, permission.ActionAllow)
	}
}

func TestMerge_DoesNotMutateSources(t *testing.T) {
	base := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionAllow},
	}
	overlay := permission.Config{
		"bash": &permission.Rule{Action: permission.ActionDeny},
	}
	_ = permission.Merge(base, overlay)
	if base["bash"].Action != permission.ActionAllow {
		t.Errorf("Merge mutated base: base[\"bash\"].Action = %q, want %q", base["bash"].Action, permission.ActionAllow)
	}
}
