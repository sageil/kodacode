package permissionpolicy

import "testing"

func TestSubjectRulesMatchUsesLastMatchingRule(t *testing.T) {
	rules := SubjectRules{
		{Pattern: "*", Action: ActionAsk},
		{Pattern: "/tmp/*", Action: ActionAllow},
		{Pattern: "/tmp/private/*", Action: ActionDeny},
	}

	action, ok := rules.Match("/tmp/private/notes.txt")
	if !ok {
		t.Fatal("Match() ok = false, want true")
	}
	if action != ActionDeny {
		t.Fatalf("action = %q, want %q", action, ActionDeny)
	}
}

func TestConfigMatchExternalDirectoryNormalizesCleanPath(t *testing.T) {
	config := Config{
		ExternalDirectory: SubjectRules{
			{Pattern: "/tmp/workspace/*", Action: ActionAllow},
		},
	}

	action, ok := config.MatchExternalDirectory("/tmp/workspace/../workspace/notes.txt")
	if !ok {
		t.Fatal("MatchExternalDirectory() ok = false, want true")
	}
	if action != ActionAllow {
		t.Fatalf("action = %q, want %q", action, ActionAllow)
	}
}

func TestConfigMatchNetworkTargetNormalizesCase(t *testing.T) {
	config := Config{
		NetworkTarget: SubjectRules{
			{Pattern: "api.github.com", Action: ActionAllow},
			{Pattern: "external network", Action: ActionAsk},
		},
	}

	action, ok := config.MatchNetworkTarget("API.GitHub.COM")
	if !ok {
		t.Fatal("MatchNetworkTarget() ok = false, want true")
	}
	if action != ActionAllow {
		t.Fatalf("action = %q, want %q", action, ActionAllow)
	}
}

func TestConfigValidateRejectsInvalidAction(t *testing.T) {
	config := Config{
		ExternalDirectory: SubjectRules{
			{Pattern: "*", Action: Action("maybe")},
		},
	}

	if err := config.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid action")
	}
}

func TestCombineUsesMostRestrictiveMatchedAction(t *testing.T) {
	action, ok := Combine(ActionAllow, ActionAsk, ActionDeny)
	if !ok {
		t.Fatal("Combine() ok = false, want true")
	}
	if action != ActionDeny {
		t.Fatalf("action = %q, want %q", action, ActionDeny)
	}

	action, ok = Combine(ActionAllow, ActionAsk)
	if !ok {
		t.Fatal("Combine() ok = false, want true")
	}
	if action != ActionAsk {
		t.Fatalf("action = %q, want %q", action, ActionAsk)
	}
}
