package tool

import (
	"context"
	"errors"
	"strings"
)

type CodeIntelLocation struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	EndLine   int    `json:"end_line,omitempty"`
	EndChar   int    `json:"end_char,omitempty"`
}

type CodeIntelDiagnostic struct {
	Line      int    `json:"line"`
	Character int    `json:"character"`
	Severity  string `json:"severity,omitempty"`
	Message   string `json:"message,omitempty"`
	Source    string `json:"source,omitempty"`
}

type CodeIntelFileDiagnostics struct {
	Path        string                `json:"path"`
	Error       string                `json:"error,omitempty"`
	Diagnostics []CodeIntelDiagnostic `json:"diagnostics,omitempty"`
}

type CodeIntelSymbol struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

type CodeIntelNoticeKind string

const (
	CodeIntelNoticeKindUnavailable CodeIntelNoticeKind = "unavailable"
	CodeIntelNoticeKindUnsupported CodeIntelNoticeKind = "unsupported"
)

type CodeIntelNotice struct {
	Kind    CodeIntelNoticeKind `json:"kind,omitempty"`
	Message string              `json:"message,omitempty"`
}

type CodeIntelNoticeError struct {
	Notice CodeIntelNotice
}

func (e *CodeIntelNoticeError) Error() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.Notice.Message)
}

func NewCodeIntelNoticeError(kind CodeIntelNoticeKind, message string) error {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return nil
	}
	return &CodeIntelNoticeError{
		Notice: CodeIntelNotice{
			Kind:    kind,
			Message: trimmed,
		},
	}
}

func AsCodeIntelNotice(err error) (*CodeIntelNotice, bool) {
	var noticeErr *CodeIntelNoticeError
	if !errors.As(err, &noticeErr) || noticeErr == nil {
		return nil, false
	}
	notice := noticeErr.Notice
	return &notice, true
}

type CodeIntelSymbolsResult struct {
	Symbols []CodeIntelSymbol `json:"symbols,omitempty"`
	Notice  *CodeIntelNotice  `json:"notice,omitempty"`
}

type CodeIntelTraceMode string

const (
	CodeIntelTraceModeCallers CodeIntelTraceMode = "callers"
	CodeIntelTraceModeCallees CodeIntelTraceMode = "callees"
	CodeIntelTraceModeGraph   CodeIntelTraceMode = "graph"
)

type CodeIntelTraceRequest struct {
	Path      string
	Line      int
	Character int
	Mode      CodeIntelTraceMode
	Depth     int
	MaxNodes  int
}

type CodeIntelTraceNode struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Path      string `json:"path,omitempty"`
	Line      int    `json:"line,omitempty"`
	Character int    `json:"character,omitempty"`
}

type CodeIntelTraceEdge struct {
	FromID string `json:"from_id,omitempty"`
	ToID   string `json:"to_id,omitempty"`
}

type CodeIntelTraceResult struct {
	Supported bool                 `json:"supported"`
	Found     bool                 `json:"found"`
	Notice    *CodeIntelNotice     `json:"notice,omitempty"`
	RootID    string               `json:"root_id,omitempty"`
	Nodes     []CodeIntelTraceNode `json:"nodes,omitempty"`
	Edges     []CodeIntelTraceEdge `json:"edges,omitempty"`
	Truncated bool                 `json:"truncated,omitempty"`
}

type CodeIntelRefsMode string

const (
	CodeIntelRefsModeAll     CodeIntelRefsMode = "all"
	CodeIntelRefsModeReaders CodeIntelRefsMode = "readers"
	CodeIntelRefsModeWriters CodeIntelRefsMode = "writers"
)

type CodeIntelReferenceKind string

const (
	CodeIntelReferenceKindRead      CodeIntelReferenceKind = "read"
	CodeIntelReferenceKindWrite     CodeIntelReferenceKind = "write"
	CodeIntelReferenceKindReference CodeIntelReferenceKind = "reference"
)

type CodeIntelRefsRequest struct {
	Path               string
	Line               int
	Character          int
	Mode               CodeIntelRefsMode
	MaxResults         int
	IncludeDeclaration bool
}

type CodeIntelReference struct {
	Kind      CodeIntelReferenceKind `json:"kind,omitempty"`
	Path      string                 `json:"path,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Character int                    `json:"character,omitempty"`
	Snippet   string                 `json:"snippet,omitempty"`
}

type CodeIntelRefsResult struct {
	Supported                bool                 `json:"supported"`
	Found                    bool                 `json:"found"`
	Notice                   *CodeIntelNotice     `json:"notice,omitempty"`
	Target                   CodeIntelTraceNode   `json:"target,omitempty"`
	References               []CodeIntelReference `json:"references,omitempty"`
	Truncated                bool                 `json:"truncated,omitempty"`
	ClassificationSupported  bool                 `json:"classification_supported,omitempty"`
	ClassificationIncomplete bool                 `json:"classification_incomplete,omitempty"`
}

type CodeIntelRenameRequest struct {
	Path      string
	Line      int
	Character int
	NewName   string
}

type CodeIntelCodeActionRequest struct {
	Path           string
	StartLine      int
	StartCharacter int
	EndLine        int
	EndCharacter   int
	Title          string
	Kind           string
	OnlyPreferred  bool
}

type CodeIntelMutationSummary struct {
	Paths     []string `json:"paths,omitempty"`
	TextEdits int      `json:"text_edits,omitempty"`
	Created   int      `json:"created,omitempty"`
	Renamed   int      `json:"renamed,omitempty"`
	Deleted   int      `json:"deleted,omitempty"`
}

type CodeIntelCodeActionResult struct {
	Title   string                   `json:"title,omitempty"`
	Summary CodeIntelMutationSummary `json:"summary,omitempty"`
}

type CodeIntel interface {
	Definition(ctx context.Context, filePath string, line, character int) ([]CodeIntelLocation, error)
	Diagnostics(ctx context.Context, filePaths []string) ([]CodeIntelFileDiagnostics, error)
	Symbols(ctx context.Context, query string) (CodeIntelSymbolsResult, error)
	Trace(ctx context.Context, request CodeIntelTraceRequest) (CodeIntelTraceResult, error)
	Refs(ctx context.Context, request CodeIntelRefsRequest) (CodeIntelRefsResult, error)
	RenameSymbol(ctx context.Context, request CodeIntelRenameRequest) (CodeIntelMutationSummary, error)
	ApplyCodeAction(ctx context.Context, request CodeIntelCodeActionRequest) (CodeIntelCodeActionResult, error)
}
