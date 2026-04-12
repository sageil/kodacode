package permission_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"

	"github.com/sageil/kodacode/v1/internal/permission"
)

func TestRuleUnmarshalYAML_StringShorthand(t *testing.T) {
	input := `bash: allow`
	var cfg permission.Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal(%q) error: %v", input, err)
	}
	rule, ok := cfg["bash"]
	if !ok {
		t.Fatalf("Config missing key %q", "bash")
	}
	if rule.Action != permission.ActionAllow {
		t.Errorf("Config[\"bash\"].Action = %q, want %q", rule.Action, permission.ActionAllow)
	}
	if len(rule.Patterns) != 0 {
		t.Errorf("Config[\"bash\"].Patterns = %v, want empty", rule.Patterns)
	}
}

func TestRuleUnmarshalYAML_ObjectSyntax(t *testing.T) {
	input := `
read:
  "*": allow
  "*.env": ask
`
	var cfg permission.Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal error: %v", err)
	}
	rule, ok := cfg["read"]
	if !ok {
		t.Fatalf("Config missing key %q", "read")
	}
	if rule.Action != "" {
		t.Errorf("Config[\"read\"].Action = %q, want empty", rule.Action)
	}
	want := []permission.Pattern{
		{Glob: "*", Action: permission.ActionAllow},
		{Glob: "*.env", Action: permission.ActionAsk},
	}
	if diff := cmp.Diff(want, rule.Patterns); diff != "" {
		t.Errorf("Config[\"read\"].Patterns mismatch (-want +got):\n%s", diff)
	}
}

func TestRuleUnmarshalYAML_MultipleTools(t *testing.T) {
	input := `
bash: allow
edit: deny
glob: ask
`
	var cfg permission.Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal error: %v", err)
	}
	cases := []struct {
		tool string
		want permission.Action
	}{
		{"bash", permission.ActionAllow},
		{"edit", permission.ActionDeny},
		{"glob", permission.ActionAsk},
	}
	for _, tc := range cases {
		rule, ok := cfg[tc.tool]
		if !ok {
			t.Errorf("Config missing key %q", tc.tool)
			continue
		}
		if rule.Action != tc.want {
			t.Errorf("Config[%q].Action = %q, want %q", tc.tool, rule.Action, tc.want)
		}
	}
}

func TestRuleUnmarshalYAML_MixedToolsAndObjectRules(t *testing.T) {
	input := `
bash: allow
read:
  "*": allow
  "*.env": ask
  "*.env.example": allow
`
	var cfg permission.Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal error: %v", err)
	}

	bashRule, ok := cfg["bash"]
	if !ok {
		t.Fatalf("Config missing key %q", "bash")
	}
	if bashRule.Action != permission.ActionAllow {
		t.Errorf("Config[\"bash\"].Action = %q, want %q", bashRule.Action, permission.ActionAllow)
	}

	readRule, ok := cfg["read"]
	if !ok {
		t.Fatalf("Config missing key %q", "read")
	}
	wantPatterns := []permission.Pattern{
		{Glob: "*", Action: permission.ActionAllow},
		{Glob: "*.env", Action: permission.ActionAsk},
		{Glob: "*.env.example", Action: permission.ActionAllow},
	}
	if diff := cmp.Diff(wantPatterns, readRule.Patterns); diff != "" {
		t.Errorf("Config[\"read\"].Patterns mismatch (-want +got):\n%s", diff)
	}
}

func TestRuleUnmarshalYAML_EmptyConfig(t *testing.T) {
	input := `{}`
	var cfg permission.Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal(%q) error: %v", input, err)
	}
	if len(cfg) != 0 {
		t.Errorf("Config = %v, want empty map", cfg)
	}
}
