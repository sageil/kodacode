package tool

import (
	"encoding/json"
	"errors"

	"github.com/sageil/kodacode/internal/workspace"
)

var (
	ErrReadPathsRequired = errors.New("path or paths is required")
	ErrReadPathRequired  = ErrReadPathsRequired
	ErrReadPathConflict  = errors.New("use either path or paths, not both")
	ErrReadEmptyPath     = errors.New("paths must not contain empty values")
	ErrReadOffsetInvalid = errors.New("offset must be at least 0")
	ErrReadLimitInvalid  = errors.New("limit must be at least 1")
)

const (
	ReadToolName     = "read"
	DefaultReadLimit = 1000
)

type ReadTool struct{}

type readInput struct {
	Paths     []string
	Offset    int
	HasOffset bool
	Limit     int
	HasLimit  bool
}

type readResult struct {
	path       string
	resolved   string
	body       string
	version    string
	state      string
	startLine  int
	endLine    int
	totalLines int
	complete   bool
}

type readFailure struct {
	path  string
	error string
}

func NewReadTool() ReadTool {
	return ReadTool{}
}

func (ReadTool) Definition() Definition {
	description := "Read one or more workspace files. Send either `path` or `paths`, not both. Omit `offset` and `limit` for normal reads; use them only for known ranges, large files, or continuing from a footer."
	return Definition{
		Name:                 ReadToolName,
		Description:          description,
		ProviderDescription:  description,
		InputSchema:          json.RawMessage(`{"type":"object","properties":{"path":{"type":["string","null"],"description":"Single workspace-relative file path. Do not combine with paths."},"paths":{"type":["array","string","null"],"items":{"type":"string"},"description":"One or more workspace-relative file paths. Do not combine with path."},"offset":{"type":["integer","string","null"],"description":"0-based starting line offset. Omit for normal reads; use for known ranges or continuation."},"limit":{"type":["integer","string","null"],"default":1000,"description":"Maximum lines per file. Omit for the default 1000-line read; use for known ranges or large files."},"line_start":{"type":["integer","string","null"],"description":"1-based inclusive alias for offset."},"line_end":{"type":["integer","string","null"],"description":"1-based inclusive end line used with line_start."}},"additionalProperties":false}`),
		ProviderInputSchema:  json.RawMessage(`{"type":"object","properties":{"path":{"type":["string","null"],"description":"Single workspace-relative file path. Do not combine with paths."},"paths":{"type":["array","string","null"],"items":{"type":"string"},"description":"One or more workspace-relative file paths. Do not combine with path."},"offset":{"type":["integer","string","null"],"description":"0-based starting line offset. Omit for normal reads; use for known ranges or continuation."},"limit":{"type":["integer","string","null"],"default":1000,"description":"Maximum lines per file. Omit for the default 1000-line read; use for known ranges or large files."}},"additionalProperties":false}`),
		ArgumentExamples:     []string{`{"paths":["file.txt"]}`, `{"paths":["file.txt","README.md"]}`},
		RequiresWorkspace:    true,
		ProviderRichGuidance: true,
		ParallelSafe:         true,
	}
}

func (ReadTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseReadInput(args)
	if err != nil {
		return nil, err
	}
	requests := make([]PathRequest, 0, len(input.Paths))
	for _, path := range input.Paths {
		requests = append(requests, PathRequest{
			Access: workspace.AccessRead,
			Path:   path,
			Reason: "read file contents",
		})
	}
	return requests, nil
}

func (ReadTool) NormalizedInputKey(args json.RawMessage) (string, error) {
	input, err := parseReadInput(args)
	if err != nil {
		return "", err
	}
	key := struct {
		Paths  []string `json:"paths"`
		Offset int      `json:"offset,omitempty"`
		Limit  int      `json:"limit"`
	}{
		Paths: append([]string(nil), input.Paths...),
		Limit: effectiveReadLimit(input),
	}
	if input.HasOffset {
		key.Offset = input.Offset
	}

	keyBytes, err := json.Marshal(key)
	if err != nil {
		return "", err
	}
	return string(keyBytes), nil
}
