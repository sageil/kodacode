package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/workspace"
)

var ErrPermissionPolicyDenied = errors.New("action denied by runtime permission policy")

type PermissionPolicyDeniedError struct {
	Subject string
	Value   string
}

func (e PermissionPolicyDeniedError) Error() string {
	subject := strings.TrimSpace(e.Subject)
	value := strings.TrimSpace(e.Value)
	switch {
	case subject == "" && value == "":
		return ErrPermissionPolicyDenied.Error()
	case subject == "":
		return fmt.Sprintf("%s: %s", ErrPermissionPolicyDenied, value)
	case value == "":
		return fmt.Sprintf("%s: %s", ErrPermissionPolicyDenied, subject)
	default:
		return fmt.Sprintf("%s: %s denied %s", ErrPermissionPolicyDenied, subject, value)
	}
}

func (e PermissionPolicyDeniedError) Unwrap() error {
	return ErrPermissionPolicyDenied
}

func appendUniqueWorkspaceGrants(existing []workspace.Grant, extra ...workspace.Grant) []workspace.Grant {
	if len(extra) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing))
	out := append([]workspace.Grant(nil), existing...)
	for _, grant := range existing {
		key := grant.Path + "\x00" + strconv.FormatBool(grant.Recursive)
		seen[key] = struct{}{}
	}
	for _, grant := range extra {
		if strings.TrimSpace(grant.Path) == "" {
			continue
		}
		key := grant.Path + "\x00" + strconv.FormatBool(grant.Recursive)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, grant)
	}
	return out
}
