package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrRootRequired       = errors.New("root is required")
	ErrRootNotDirectory   = errors.New("root must be an existing directory")
	ErrPathRequired       = errors.New("path is required")
	ErrAccessRequired     = errors.New("access is required")
	ErrPermissionRequired = errors.New("path requires user approval")
)

type Access string

const (
	AccessRead    Access = "read"
	AccessWrite   Access = "write"
	AccessList    Access = "list"
	AccessExec    Access = "exec"
	AccessWorkdir Access = "workdir"
)

type Grant struct {
	Path      string
	Recursive bool
}

type Options struct {
	Grants []Grant
}

type Decision struct {
	Access       Access
	InputPath    string
	ResolvedPath string
	WithinRoot   bool
	Granted      bool
}

func (d Decision) Allowed() bool {
	return d.WithinRoot || d.Granted
}

func (d Decision) RequiresApproval() bool {
	return !d.Allowed()
}

type Scope struct {
	root   string
	grants []Grant
}

func New(root string, opts ...Options) (*Scope, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrRootRequired
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	resolvedRoot := resolvePathAllowMissing(absoluteRoot)

	info, err := os.Stat(resolvedRoot)
	if err != nil || !info.IsDir() {
		return nil, ErrRootNotDirectory
	}

	scope := &Scope{root: resolvedRoot}
	if len(opts) == 0 {
		return scope, nil
	}
	for _, grant := range opts[0].Grants {
		if _, err := scope.Grant(grant.Path, grant.Recursive); err != nil {
			return nil, err
		}
	}
	return scope, nil
}

func (s *Scope) Root() string {
	return s.root
}

func (s *Scope) Grants() []Grant {
	out := make([]Grant, len(s.grants))
	copy(out, s.grants)
	return out
}

func (s *Scope) Grant(path string, recursive bool) (Grant, error) {
	resolved, err := s.resolve(path)
	if err != nil {
		return Grant{}, err
	}
	grant := Grant{
		Path:      resolved,
		Recursive: recursive,
	}
	for _, existing := range s.grants {
		if existing == grant {
			return grant, nil
		}
	}
	s.grants = append(s.grants, grant)
	return grant, nil
}

func (s *Scope) Check(access Access, path string) (Decision, error) {
	if access == "" {
		return Decision{}, ErrAccessRequired
	}
	switch access {
	case AccessRead, AccessWrite, AccessList, AccessExec, AccessWorkdir:
	default:
		return Decision{}, fmt.Errorf("unsupported access %q", access)
	}

	resolved, err := s.resolve(path)
	if err != nil {
		return Decision{}, err
	}

	decision := Decision{
		Access:       access,
		InputPath:    path,
		ResolvedPath: resolved,
		WithinRoot:   within(s.root, resolved),
	}
	if !decision.WithinRoot {
		decision.Granted = s.isGranted(resolved)
	}
	return decision, nil
}

func (s *Scope) Authorize(access Access, path string) (Decision, error) {
	decision, err := s.Check(access, path)
	if err != nil {
		return Decision{}, err
	}
	if decision.RequiresApproval() {
		return decision, ErrPermissionRequired
	}
	return decision, nil
}

func (s *Scope) resolve(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", ErrPathRequired
	}
	path = expandHomePath(path)
	if filepath.IsAbs(path) {
		return resolvePathAllowMissing(path), nil
	}
	return resolvePathAllowMissing(filepath.Join(s.root, path)), nil
}

func expandHomePath(path string) string {
	trimmed := strings.TrimSpace(path)
	switch {
	case trimmed == "~":
	case strings.HasPrefix(trimmed, "~/"), strings.HasPrefix(trimmed, "~\\"):
	default:
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return path
	}
	if trimmed == "~" {
		return home
	}
	return filepath.Join(home, trimmed[2:])
}

func (s *Scope) isGranted(path string) bool {
	for _, grant := range s.grants {
		if grant.Recursive {
			if within(grant.Path, path) {
				return true
			}
			continue
		}
		if path == grant.Path {
			return true
		}
	}
	return false
}

func resolvePathAllowMissing(path string) string {
	candidate := filepath.Clean(path)
	if candidate == "" {
		return candidate
	}
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
			return joinResolvedSuffix(canonicalizeExistingPath(resolved), suffix)
		}
		if info, err := os.Lstat(candidate); err == nil && info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(candidate)
			if err == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(candidate), target)
				}
				return joinResolvedSuffix(canonicalizeExistingPath(filepath.Clean(target)), suffix)
			}
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return canonicalizeExistingPath(filepath.Clean(path))
		}
		suffix = append(suffix, filepath.Base(candidate))
		candidate = parent
	}
}

func canonicalizeExistingPath(path string) string {
	clean := filepath.Clean(path)
	if clean == "" {
		return clean
	}
	info, err := os.Lstat(clean)
	if err != nil || (info.Mode()&os.ModeSymlink) != 0 {
		return clean
	}

	volume := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, volume)
	current := volume
	if strings.HasPrefix(rest, string(filepath.Separator)) {
		if current == "" {
			current = string(filepath.Separator)
		} else {
			current += string(filepath.Separator)
		}
		rest = strings.TrimPrefix(rest, string(filepath.Separator))
	}
	if rest == "" {
		if current == "" {
			return clean
		}
		return filepath.Clean(current)
	}

	parts := strings.Split(rest, string(filepath.Separator))
	for idx, part := range parts {
		if part == "" || part == "." {
			continue
		}
		matched, ok := directoryEntryName(current, part)
		if !ok {
			return filepath.Clean(filepath.Join(current, filepath.Join(parts[idx:]...)))
		}
		current = filepath.Join(current, matched)
	}
	return filepath.Clean(current)
}

func directoryEntryName(parent, name string) (string, bool) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", false
	}

	for _, entry := range entries {
		if entry.Name() == name {
			return entry.Name(), true
		}
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), name) {
			return entry.Name(), true
		}
	}
	return "", false
}

func joinResolvedSuffix(base string, suffix []string) string {
	resolved := base
	for i := len(suffix) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, suffix[i])
	}
	return filepath.Clean(resolved)
}

func within(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return !escapes(rel)
}

func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
