package app

import (
	"os"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/permissionpolicy"
	"github.com/sageil/kodacode/internal/workspace"
)

func (s *SessionService) matchExternalDirectoryPolicy(state events.SessionState, resolvedPath string) (permissionpolicy.Action, bool) {
	policy := s.permissionPolicyForState(state)
	if policy.Empty() {
		return "", false
	}
	return policy.MatchExternalDirectory(resolvedPath)
}

func (s *SessionService) matchBashPolicy(state events.SessionState, command string) (permissionpolicy.Action, bool) {
	policy := s.permissionPolicyForState(state)
	if policy.Empty() {
		return "", false
	}
	return policy.MatchBash(command)
}

func (s *SessionService) matchNetworkPolicy(state events.SessionState, input NetworkAuthorizationInput) permissionPolicyDecision {
	policy := s.permissionPolicyForState(state)
	if policy.Empty() {
		return permissionPolicyDecision{}
	}
	actions := make([]permissionpolicy.Action, 0, 2)
	decision := permissionPolicyDecision{}
	if url := strings.TrimSpace(input.URL); url != "" {
		if action, ok := policy.MatchWebFetch(url); ok {
			actions = append(actions, action)
			if action == permissionpolicy.ActionDeny && decision.Subject == "" {
				decision.Subject = "permissions.web_fetch"
				decision.Value = url
			}
		}
	}
	if target := strings.TrimSpace(input.Target); target != "" {
		if action, ok := policy.MatchNetworkTarget(target); ok {
			actions = append(actions, action)
			if action == permissionpolicy.ActionDeny {
				decision.Subject = "permissions.network_target"
				decision.Value = target
			}
		}
	}
	decision.Action, decision.Matched = permissionpolicy.Combine(actions...)
	if decision.Matched && decision.Subject == "" {
		decision.Subject = "permissions.network_target"
		decision.Value = strings.TrimSpace(input.Target)
	}
	return decision
}

func (s *SessionService) matchNetworkTargetPolicy(state events.SessionState, target string) (permissionpolicy.Action, bool) {
	policy := s.permissionPolicyForState(state)
	if policy.Empty() {
		return "", false
	}
	return policy.MatchNetworkTarget(target)
}

func (s *SessionService) permissionPolicyForState(state events.SessionState) permissionpolicy.Config {
	if permissionModeGrantsFullAccess(statePermissionMode(state, PermissionModeAuto)) {
		return permissionpolicy.Config{}
	}
	s.policyMu.RLock()
	defer s.policyMu.RUnlock()
	return clonePermissionPolicyConfig(s.policy)
}

func clonePermissionPolicyConfig(policy permissionpolicy.Config) permissionpolicy.Config {
	return permissionpolicy.Config{
		ExternalDirectory: append(permissionpolicy.SubjectRules(nil), policy.ExternalDirectory...),
		Bash:              append(permissionpolicy.SubjectRules(nil), policy.Bash...),
		WebFetch:          append(permissionpolicy.SubjectRules(nil), policy.WebFetch...),
		NetworkTarget:     append(permissionpolicy.SubjectRules(nil), policy.NetworkTarget...),
	}
}

func policyTemporaryGrants(resolvedPath string) []workspace.Grant {
	resolvedPath = strings.TrimSpace(resolvedPath)
	if resolvedPath == "" {
		return nil
	}
	grant := workspace.Grant{Path: resolvedPath}
	if info, err := os.Stat(resolvedPath); err == nil && info.IsDir() {
		grant.Recursive = true
	}
	return []workspace.Grant{grant}
}
