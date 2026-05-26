package permissionpolicy

import (
	"fmt"
	"strings"
)

type Action string

const (
	ActionAllow Action = "allow"
	ActionAsk   Action = "ask"
	ActionDeny  Action = "deny"
)

type Rule struct {
	Pattern string
	Action  Action
}

type SubjectRules []Rule

type Config struct {
	ExternalDirectory SubjectRules
	Bash              SubjectRules
	WebFetch          SubjectRules
	NetworkTarget     SubjectRules
}

func (a Action) Validate() error {
	switch strings.TrimSpace(string(a)) {
	case string(ActionAllow), string(ActionAsk), string(ActionDeny):
		return nil
	default:
		return fmt.Errorf("invalid action %q", a)
	}
}

func (r Rule) Validate() error {
	if strings.TrimSpace(r.Pattern) == "" {
		return fmt.Errorf("pattern is required")
	}
	if err := r.Action.Validate(); err != nil {
		return err
	}
	return nil
}

func (s SubjectRules) Validate(subject string) error {
	for idx, rule := range s {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("%s rule %d: %w", subject, idx, err)
		}
	}
	return nil
}

func (c Config) Validate() error {
	if err := c.ExternalDirectory.Validate("external_directory"); err != nil {
		return err
	}
	if err := c.Bash.Validate("bash"); err != nil {
		return err
	}
	if err := c.WebFetch.Validate("web_fetch"); err != nil {
		return err
	}
	if err := c.NetworkTarget.Validate("network_target"); err != nil {
		return err
	}
	return nil
}

func (c Config) Empty() bool {
	return len(c.ExternalDirectory) == 0 &&
		len(c.Bash) == 0 &&
		len(c.WebFetch) == 0 &&
		len(c.NetworkTarget) == 0
}

func Combine(actions ...Action) (Action, bool) {
	var (
		combined Action
		matched  bool
	)
	for _, action := range actions {
		switch action {
		case ActionDeny:
			return ActionDeny, true
		case ActionAsk:
			combined = ActionAsk
			matched = true
		case ActionAllow:
			if !matched {
				combined = ActionAllow
				matched = true
			}
		}
	}
	return combined, matched
}
