package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type StartupTrustState struct {
	WorkspaceRoot     string
	WorkspaceRequired bool
	Servers           []StartupTrustServer
}

func (s StartupTrustState) Pending() bool {
	return s.WorkspaceRequired || len(s.Servers) > 0
}

type StartupTrustServer struct {
	Name        string
	Type        string
	Fingerprint string
	Command     string
	Args        []string
	EnvKeys     []string
}

type ResolveStartupTrustInput struct {
	WorkspaceRoot   string
	TrustWorkspace  bool
	ServerDecisions map[string]bool
}

type WorkspaceTrustState struct {
	WorkspaceRoot string
	Trusted       bool
	UpdatedAt     time.Time
	Servers       []WorkspaceMCPTrustState
}

type WorkspaceMCPTrustState struct {
	Fingerprint string
	Kind        string
	Label       string
	Trusted     bool
	UpdatedAt   time.Time
}

type RevokeTrustScope string

const (
	RevokeTrustScopeWorkspace    RevokeTrustScope = "workspace"
	RevokeTrustScopeServer       RevokeTrustScope = "server"
	RevokeTrustScopeWorkspaceAll RevokeTrustScope = "workspace_all"
	RevokeTrustScopeAll          RevokeTrustScope = "all"
)

type RevokeTrustInput struct {
	SessionID     string
	WorkspaceRoot string
	Scope         RevokeTrustScope
	Fingerprint   string
}

type trustRecord struct {
	Trusted   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type mcpTrustRecord struct {
	Trusted   bool
	Kind      string
	Label     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type workspaceMCPTrustRecord struct {
	Fingerprint string
	Trusted     bool
	Kind        string
	Label       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type startupTrustStore struct {
	db *sql.DB
}

func newStartupTrustStore(path string) (*startupTrustStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("trust db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateStartupTrustStore(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &startupTrustStore{db: db}, nil
}

func migrateStartupTrustStore(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS kodacode_workspace_trust (
    workspace_root TEXT PRIMARY KEY,
    trusted        INTEGER NOT NULL,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS kodacode_workspace_mcp_trust (
    workspace_root TEXT    NOT NULL,
    fingerprint    TEXT    NOT NULL,
    kind           TEXT    NOT NULL,
    label          TEXT    NOT NULL,
    trusted        INTEGER NOT NULL,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    PRIMARY KEY (workspace_root, fingerprint)
);
`)
	return err
}

func (s *startupTrustStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *startupTrustStore) WorkspaceTrust(ctx context.Context, workspaceRoot string) (trustRecord, bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceRoot) == "" {
		return trustRecord{}, false, nil
	}
	var trusted int
	var createdAt int64
	var updatedAt int64
	err := s.db.QueryRowContext(ctx, `
SELECT trusted, created_at, updated_at
FROM kodacode_workspace_trust
WHERE workspace_root = ?`,
		workspaceRoot,
	).Scan(&trusted, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return trustRecord{}, false, nil
	}
	if err != nil {
		return trustRecord{}, false, err
	}
	return trustRecord{
		Trusted:   trusted != 0,
		CreatedAt: time.Unix(0, createdAt).UTC(),
		UpdatedAt: time.Unix(0, updatedAt).UTC(),
	}, true, nil
}

func (s *startupTrustStore) SetWorkspaceTrust(ctx context.Context, workspaceRoot string, trusted bool) error {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceRoot) == "" {
		return nil
	}
	now := time.Now().UTC().UnixNano()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO kodacode_workspace_trust (workspace_root, trusted, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(workspace_root) DO UPDATE SET
    trusted = excluded.trusted,
    updated_at = excluded.updated_at`,
		workspaceRoot,
		boolToInt(trusted),
		now,
		now,
	)
	return err
}

func (s *startupTrustStore) DeleteWorkspaceTrust(ctx context.Context, workspaceRoot string) error {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceRoot) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM kodacode_workspace_trust
WHERE workspace_root = ?`,
		workspaceRoot,
	)
	return err
}

func (s *startupTrustStore) MCPTrust(ctx context.Context, workspaceRoot, fingerprint string) (mcpTrustRecord, bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(fingerprint) == "" {
		return mcpTrustRecord{}, false, nil
	}
	var trusted int
	var kind string
	var label string
	var createdAt int64
	var updatedAt int64
	err := s.db.QueryRowContext(ctx, `
SELECT trusted, kind, label, created_at, updated_at
FROM kodacode_workspace_mcp_trust
WHERE workspace_root = ? AND fingerprint = ?`,
		workspaceRoot,
		fingerprint,
	).Scan(&trusted, &kind, &label, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return mcpTrustRecord{}, false, nil
	}
	if err != nil {
		return mcpTrustRecord{}, false, err
	}
	return mcpTrustRecord{
		Trusted:   trusted != 0,
		Kind:      kind,
		Label:     label,
		CreatedAt: time.Unix(0, createdAt).UTC(),
		UpdatedAt: time.Unix(0, updatedAt).UTC(),
	}, true, nil
}

func (s *startupTrustStore) SetMCPTrust(ctx context.Context, workspaceRoot, fingerprint, kind, label string, trusted bool) error {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(fingerprint) == "" {
		return nil
	}
	now := time.Now().UTC().UnixNano()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO kodacode_workspace_mcp_trust (workspace_root, fingerprint, kind, label, trusted, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_root, fingerprint) DO UPDATE SET
    kind = excluded.kind,
    label = excluded.label,
    trusted = excluded.trusted,
    updated_at = excluded.updated_at`,
		workspaceRoot,
		fingerprint,
		strings.TrimSpace(kind),
		strings.TrimSpace(label),
		boolToInt(trusted),
		now,
		now,
	)
	return err
}

func (s *startupTrustStore) ListWorkspaceMCPTrust(ctx context.Context, workspaceRoot string) ([]workspaceMCPTrustRecord, error) {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceRoot) == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT fingerprint, trusted, kind, label, created_at, updated_at
FROM kodacode_workspace_mcp_trust
WHERE workspace_root = ?
ORDER BY label, fingerprint`,
		workspaceRoot,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var records []workspaceMCPTrustRecord
	for rows.Next() {
		var record workspaceMCPTrustRecord
		var trusted int
		var createdAt int64
		var updatedAt int64
		if err := rows.Scan(
			&record.Fingerprint,
			&trusted,
			&record.Kind,
			&record.Label,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		record.Trusted = trusted != 0
		record.CreatedAt = time.Unix(0, createdAt).UTC()
		record.UpdatedAt = time.Unix(0, updatedAt).UTC()
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *startupTrustStore) DeleteMCPTrust(ctx context.Context, workspaceRoot, fingerprint string) error {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(fingerprint) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM kodacode_workspace_mcp_trust
WHERE workspace_root = ? AND fingerprint = ?`,
		workspaceRoot,
		fingerprint,
	)
	return err
}

func (s *startupTrustStore) DeleteWorkspaceMCPTrust(ctx context.Context, workspaceRoot string) error {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceRoot) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM kodacode_workspace_mcp_trust
WHERE workspace_root = ?`,
		workspaceRoot,
	)
	return err
}

func (s *startupTrustStore) DeleteAllTrust(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kodacode_workspace_mcp_trust`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kodacode_workspace_trust`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type mcpServerFingerprintInput struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func mcpServerFingerprint(server MCPServerConfig) (string, error) {
	env := make(map[string]string, len(server.Env))
	envKeys := make([]string, 0, len(server.Env))
	for key := range server.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		env[key] = server.Env[key]
	}
	payload, err := json.Marshal(mcpServerFingerprintInput{
		Type:    strings.TrimSpace(server.Type),
		Command: strings.TrimSpace(server.Command),
		Args:    append([]string(nil), server.Args...),
		Env:     env,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func mcpServerEnvKeys(server MCPServerConfig) []string {
	if len(server.Env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(server.Env))
	for key := range server.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
