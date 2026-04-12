// Package permission defines the permission rule types and YAML unmarshalling
// for kodacode's OpenCode-compatible permission system.
//
// A Config maps tool names to Rule values. Each Rule is either a plain string
// action ("allow", "ask", "deny") or an ordered list of Pattern entries where
// the last matching pattern wins.
//
// YAML shorthand (string rule):
//
//	permission:
//	  bash: allow
//
// YAML object rule (last matching glob wins):
//
//	permission:
//	  read:
//	    "*": allow
//	    "*.env": ask
package permission

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Action is one of "allow", "ask", or "deny".
type Action string

const (
	// ActionAllow permits the tool call without prompting.
	ActionAllow Action = "allow"
	// ActionAsk prompts the user before executing the tool call.
	ActionAsk Action = "ask"
	// ActionDeny blocks the tool call.
	ActionDeny Action = "deny"
)

// Pattern is a single glob-to-action mapping within a Rule.
type Pattern struct {
	// Glob is matched against the tool's key argument.
	Glob string
	// Action is the action to take when the glob matches.
	Action Action
}

// Rule is a permission rule for a single tool.
// Exactly one of Action (string shorthand) or Patterns (object syntax) is set.
// When Patterns is non-empty, rules are evaluated in order and the last
// matching pattern wins.
type Rule struct {
	// Action is set when the rule is a plain string shorthand (e.g. "allow").
	Action Action
	// Patterns is set when the rule is an object mapping globs to actions.
	// Evaluated in declaration order; last match wins.
	Patterns []Pattern
}

// UnmarshalYAML supports both string shorthand and object map syntax.
//
// String shorthand:
//
//	bash: allow
//
// Object map:
//
//	read:
//	  "*": allow
//	  "*.env": ask
func (r *Rule) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		r.Action = Action(value.Value)
		return nil
	case yaml.MappingNode:
		if len(value.Content)%2 != 0 {
			return fmt.Errorf("permission rule: odd number of mapping nodes")
		}
		for i := 0; i < len(value.Content); i += 2 {
			glob := value.Content[i].Value
			action := Action(value.Content[i+1].Value)
			r.Patterns = append(r.Patterns, Pattern{Glob: glob, Action: action})
		}
		return nil
	default:
		return fmt.Errorf("permission rule: unexpected YAML node kind %v", value.Kind)
	}
}

// Config is the full permission configuration — a map from tool name to Rule.
// A nil Config is valid and means "use defaults".
type Config map[string]*Rule
