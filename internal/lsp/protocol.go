// Package lsp implements a JSON-RPC 2.0 client for the Language Server Protocol.
// It communicates with any LSP-compliant server over stdin/stdout using
// Content-Length framed messages.
package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`     // nil for notifications
	Method  string          `json:"method,omitempty"` // set for requests/notifications from server
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type InitializeParams struct {
	ProcessID             int                `json:"processId"`
	RootURI               string             `json:"rootUri"`
	Capabilities          ClientCapabilities `json:"capabilities"`
	ClientInfo            ClientInfo         `json:"clientInfo"`
	WorkspaceFolders      []WorkspaceFolder  `json:"workspaceFolders,omitempty"`
	InitializationOptions any                `json:"initializationOptions,omitempty"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type ClientCapabilities struct {
	TextDocument *TextDocumentClientCapabilities `json:"textDocument,omitempty"`
	Workspace    *WorkspaceClientCapabilities    `json:"workspace,omitempty"`
}

type TextDocumentClientCapabilities struct {
	Definition         *json.RawMessage              `json:"definition,omitempty"`
	References         *json.RawMessage              `json:"references,omitempty"`
	Hover              *json.RawMessage              `json:"hover,omitempty"`
	CodeAction         *json.RawMessage              `json:"codeAction,omitempty"`
	PublishDiagnostics *PublishDiagnosticsCapability `json:"publishDiagnostics,omitempty"`
}

type PublishDiagnosticsCapability struct {
	RelatedInformation bool `json:"relatedInformation,omitempty"`
	VersionSupport     bool `json:"versionSupport,omitempty"`
}

type WorkspaceClientCapabilities struct {
	Symbol           *json.RawMessage `json:"symbol,omitempty"`
	WorkspaceFolders bool             `json:"workspaceFolders,omitempty"`
}

// InitializeResult is returned from the initialize request.
type InitializeResult struct {
	Capabilities json.RawMessage `json:"capabilities"`
}

// Position in a text document (0-indexed line and character).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Only        []string     `json:"only,omitempty"`
}

type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type OptionalVersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version *int   `json:"version,omitempty"`
}

type TextDocumentEdit struct {
	TextDocument OptionalVersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []TextEdit                              `json:"edits"`
}

type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []json.RawMessage     `json:"documentChanges,omitempty"`
}

type Command struct {
	Title     string            `json:"title"`
	Command   string            `json:"command"`
	Arguments []json.RawMessage `json:"arguments,omitempty"`
}

type CodeActionDisabled struct {
	Reason string `json:"reason"`
}

type CodeAction struct {
	Title       string              `json:"title"`
	Kind        string              `json:"kind,omitempty"`
	Edit        *WorkspaceEdit      `json:"edit,omitempty"`
	Command     *Command            `json:"command,omitempty"`
	IsPreferred bool                `json:"isPreferred,omitempty"`
	Disabled    *CodeActionDisabled `json:"disabled,omitempty"`
}

type CreateFileOptions struct {
	Overwrite      bool `json:"overwrite,omitempty"`
	IgnoreIfExists bool `json:"ignoreIfExists,omitempty"`
}

type CreateFile struct {
	Kind    string             `json:"kind"`
	URI     string             `json:"uri"`
	Options *CreateFileOptions `json:"options,omitempty"`
}

type RenameFileOptions struct {
	Overwrite      bool `json:"overwrite,omitempty"`
	IgnoreIfExists bool `json:"ignoreIfExists,omitempty"`
}

type RenameFile struct {
	Kind    string             `json:"kind"`
	OldURI  string             `json:"oldUri"`
	NewURI  string             `json:"newUri"`
	Options *RenameFileOptions `json:"options,omitempty"`
}

type DeleteFileOptions struct {
	Recursive         bool `json:"recursive,omitempty"`
	IgnoreIfNotExists bool `json:"ignoreIfNotExists,omitempty"`
}

type DeleteFile struct {
	Kind    string             `json:"kind"`
	URI     string             `json:"uri"`
	Options *DeleteFileOptions `json:"options,omitempty"`
}

// HoverResult is the response to textDocument/hover.
type HoverResult struct {
	Contents json.RawMessage `json:"contents"` // MarkupContent, string, or []MarkedString
}

// ExtractHoverText attempts to extract readable text from the hover contents,
// which can be a MarkupContent, a string, or an array of MarkedString objects.
func (h HoverResult) ExtractHoverText() string {
	// Try MarkupContent first: {"kind": "...", "value": "..."}
	var mc struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if json.Unmarshal(h.Contents, &mc) == nil && mc.Value != "" {
		return mc.Value
	}
	// Try plain string.
	var s string
	if json.Unmarshal(h.Contents, &s) == nil && s != "" {
		return s
	}
	// Try array of MarkedString.
	var arr []json.RawMessage
	if json.Unmarshal(h.Contents, &arr) == nil {
		var parts []string
		for _, raw := range arr {
			var ms struct {
				Language string `json:"language"`
				Value    string `json:"value"`
			}
			if json.Unmarshal(raw, &ms) == nil && ms.Value != "" {
				parts = append(parts, ms.Value)
				continue
			}
			var plain string
			if json.Unmarshal(raw, &plain) == nil && plain != "" {
				parts = append(parts, plain)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(h.Contents)
}

type ReferenceParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      ReferenceContext       `json:"context"`
}

type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// Diagnostic represents a compiler error/warning from the server.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1=Error, 2=Warning, 3=Info, 4=Hint
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// WorkspaceSymbolParams is the request for workspace/symbol.
type WorkspaceSymbolParams struct {
	Query string `json:"query"`
}

// SymbolInformation is a workspace symbol result.
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"` // LSP SymbolKind enum
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// SymbolKindName returns a human-readable name for an LSP SymbolKind.
func SymbolKindName(kind int) string {
	switch kind {
	case 1:
		return "File"
	case 2:
		return "Module"
	case 3:
		return "Namespace"
	case 4:
		return "Package"
	case 5:
		return "Class"
	case 6:
		return "Method"
	case 7:
		return "Property"
	case 8:
		return "Field"
	case 9:
		return "Constructor"
	case 10:
		return "Enum"
	case 11:
		return "Interface"
	case 12:
		return "Function"
	case 13:
		return "Variable"
	case 14:
		return "Constant"
	case 15:
		return "String"
	case 16:
		return "Number"
	case 17:
		return "Boolean"
	case 18:
		return "Array"
	case 19:
		return "Object"
	case 22:
		return "Struct"
	case 23:
		return "Event"
	case 24:
		return "Operator"
	case 25:
		return "TypeParameter"
	default:
		return "Symbol"
	}
}

// FileURI converts an absolute file path to a file:// URI.
func FileURI(path string) string {
	abs, _ := filepath.Abs(path)
	return "file://" + abs
}

// URIToPath converts a file:// URI to a local file path.
func URIToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return strings.TrimPrefix(uri, "file://")
	}
	return filepath.FromSlash(u.Path)
}

// LanguageID returns the LSP language identifier for a file extension.
func LanguageID(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h", ".hpp":
		return "cpp"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".lua":
		return "lua"
	case ".zig":
		return "zig"
	case ".cs":
		return "csharp"
	case ".swift":
		return "swift"
	case ".kt", ".kts":
		return "kotlin"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	case ".sh", ".bash":
		return "shellscript"
	default:
		return strings.TrimPrefix(strings.ToLower(ext), ".")
	}
}

// SeverityString returns a human-readable label for a diagnostic severity.
func SeverityString(s int) string {
	switch s {
	case 1:
		return "Error"
	case 2:
		return "Warning"
	case 3:
		return "Info"
	case 4:
		return "Hint"
	default:
		return fmt.Sprintf("Severity(%d)", s)
	}
}
