package tool

import (
	"context"
	"encoding/json"
	"strings"

	searchsvc "github.com/sageil/kodacode/internal/search"
	"github.com/sageil/kodacode/internal/workspace"
)

const SearchToolName = "search"

type SearchTool struct{}

func NewSearchTool() SearchTool {
	return SearchTool{}
}

func (SearchTool) Definition() Definition {
	description := "Search file contents under a workspace path. Use `path:\".\"` for workspace-wide search, never `/`. Use lexical for exact or regex matches and hybrid for semantic ranking. Optional `glob`, `regex`, `case_sensitive`, and `max_matches` narrow results."
	return Definition{
		Name:                SearchToolName,
		Description:         description,
		ProviderDescription: description,
		InputSchema:         json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query. In lexical mode this is matched literally unless regex is true. In hybrid mode it is also used for semantic ranking."},"path":{"type":"string","description":"File or directory to search. Relative paths resolve from the workspace directory. Use \".\" for workspace-wide search; do not use the filesystem root like \"/\"."},"mode":{"type":["string","null"],"description":"Use null or omit this field to let the runtime choose hybrid when semantic search is configured and regex is false; otherwise it uses lexical. Use \"lexical\" to force exact or regex matching, or \"hybrid\" to request lexical plus semantic ranking.","enum":["lexical","hybrid",null]},"glob":{"type":["string","null"],"description":"Use null or omit this field to search all files, or provide a basename pattern like \"*.go\" or a relative path pattern like \"internal/*.go\"."},"regex":{"type":["boolean","string","null"],"description":"Use true to treat query as a line-based regular expression. Regex search is supported only in lexical mode. Omit this field to accept the default false behavior."},"case_sensitive":{"type":["boolean","string","null"],"description":"Use null or omit this field to accept the default case-insensitive matching. Regex search uses this flag to choose case-sensitive or case-insensitive compilation."},"max_matches":{"type":["integer","string","null"],"description":"Use null or omit this field to accept the default limit of 200 results. Values above 200 are clamped to 200."},"limit":{"type":["integer","string","null"],"description":"Alias for max_matches. Prefer max_matches for new calls."},"max_results":{"type":["integer","string","null"],"description":"Alias for max_matches. Prefer max_matches for new calls."}},"required":["query","path"],"additionalProperties":false}`),
		ProviderInputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Text or regex to search for."},"path":{"type":"string","description":"Workspace-relative file or directory. Use \".\" for workspace-wide search; never use \"/\"."},"mode":{"type":["string","null"],"description":"Use lexical for exact or regex matches; hybrid for semantic ranking.","enum":["lexical","hybrid",null]},"glob":{"type":["string","null"],"description":"Optional file pattern such as \"*.go\" or \"internal/*.go\"."},"regex":{"type":["boolean","string","null"],"description":"True treats query as a line regex. Use lexical mode with regex."},"case_sensitive":{"type":["boolean","string","null"],"description":"Optional case-sensitive matching flag."},"max_matches":{"type":["integer","string","null"],"description":"Maximum results. Defaults to 200 and clamps at 200."}},"required":["query","path"],"additionalProperties":false}`),
		ArgumentExamples:    []string{`{"query":"cache","path":"src","mode":"lexical","glob":"*.go","regex":false,"case_sensitive":false,"max_matches":20}`},
		RequiresWorkspace:   true,
		ParallelSafe:        true,
	}
}

func NormalizeSearchArguments(args json.RawMessage, service *searchsvc.Service) (json.RawMessage, error) {
	input, err := parseSearchInput(args)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Mode) != "" {
		return args, nil
	}

	var raw map[string]json.RawMessage
	if err := DecodeArgs(SearchToolName, args, &raw); err != nil {
		return nil, err
	}
	mode := searchsvc.ModeLexical
	if service != nil {
		mode = service.ResolveMode(searchsvc.Request{
			Regex: input.Regex,
		})
	}
	raw["mode"] = json.RawMessage(`"` + string(mode) + `"`)
	normalized, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func (SearchTool) PathRequests(args json.RawMessage) ([]PathRequest, error) {
	input, err := parseSearchInput(args)
	if err != nil {
		return nil, err
	}
	return []PathRequest{{
		Access: workspace.AccessRead,
		Path:   input.Path,
		Reason: "search file contents",
	}}, nil
}

func (SearchTool) NormalizedInputKey(args json.RawMessage) (string, error) {
	input, err := parseSearchInput(args)
	if err != nil {
		return "", err
	}
	key, err := json.Marshal(struct {
		Query         string `json:"query"`
		Path          string `json:"path"`
		Mode          string `json:"mode"`
		Glob          string `json:"glob,omitempty"`
		Regex         bool   `json:"regex"`
		CaseSensitive bool   `json:"case_sensitive"`
		MaxMatches    int    `json:"max_matches"`
	}{
		Query:         input.Query,
		Path:          input.Path,
		Mode:          reuseSearchMode(input.Mode),
		Glob:          input.Glob,
		Regex:         input.Regex,
		CaseSensitive: input.CaseSensitive,
		MaxMatches:    input.MaxMatches,
	})
	if err != nil {
		return "", err
	}
	return string(key), nil
}

func (SearchTool) Execute(ctx context.Context, ectx ExecutionContext, args json.RawMessage) (Result, error) {
	input, err := parseSearchInput(args)
	if err != nil {
		return Result{}, err
	}

	decision, err := ectx.ResolvePath(workspace.AccessRead, input.Path)
	if err != nil {
		return Result{}, err
	}

	request := searchsvc.Request{
		Query:         input.Query,
		RootPath:      decision.ResolvedPath,
		WorkspaceRoot: searchWorkspaceRoot(ectx),
		Glob:          input.Glob,
		Regex:         input.Regex,
		CaseSensitive: input.CaseSensitive,
		MaxResults:    input.MaxMatches,
		Mode:          searchsvc.Mode(input.Mode),
	}
	if err := request.Validate(); err != nil {
		return Result{}, err
	}
	if ectx.Search != nil {
		response, err := ectx.Search.Search(ctx, request)
		if err != nil {
			return Result{}, err
		}
		response.Notice = joinSearchNotice(input.Notice, response.Notice)
		structured, err := MarshalStructuredResult(structuredSearchResponse(response))
		if err != nil {
			return Result{}, err
		}
		return Result{
			Output:            searchsvc.FormatResponse(response),
			StructuredResult:  structured,
			ObservedResources: observedSearchResources(response),
		}, nil
	}

	response, err := searchsvc.LexicalObserved(request)
	if err != nil {
		return Result{}, err
	}
	response.Notice = input.Notice
	if searchsvc.Mode(input.Mode) == searchsvc.ModeHybrid {
		response.Mode = searchsvc.ModeHybrid
		response.Fallback = true
		response.Notice = joinSearchNotice(input.Notice, "semantic search is not configured; using lexical results")
		response.Observation = nil
	}
	structured, err := MarshalStructuredResult(structuredSearchResponse(response))
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output:            searchsvc.FormatResponse(response),
		StructuredResult:  structured,
		ObservedResources: observedSearchResources(response),
	}, nil
}

func searchWorkspaceRoot(ectx ExecutionContext) string {
	if ectx.Workspace == nil {
		return ""
	}
	return ectx.Workspace.Root()
}

func observedSearchResources(response searchsvc.Response) []ObservedResource {
	if response.Mode != searchsvc.ModeLexical || response.Fallback || response.Observation == nil || !response.Observation.Complete {
		return nil
	}
	if len(response.Observation.Resources) == 0 {
		return nil
	}
	out := make([]ObservedResource, 0, len(response.Observation.Resources))
	for _, resource := range response.Observation.Resources {
		switch resource.Kind {
		case "file_content":
			out = append(out, ObservedResource{
				Kind:    ObservedResourceFileContent,
				Path:    resource.Path,
				Version: resource.Version,
				State:   resource.State,
			})
		case "dir_entries":
			out = append(out, ObservedResource{
				Kind:    ObservedResourceDirEntries,
				Path:    resource.Path,
				Version: resource.Version,
			})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func reuseSearchMode(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "auto"
	}
	return mode
}

type searchStructuredResult struct {
	Mode     searchsvc.Mode          `json:"mode"`
	Notice   string                  `json:"notice,omitempty"`
	Fallback bool                    `json:"fallback,omitempty"`
	Results  []searchStructuredMatch `json:"results"`
}

type searchStructuredMatch struct {
	Path    string           `json:"path"`
	Line    int              `json:"line,omitempty"`
	Snippet string           `json:"snippet,omitempty"`
	Source  searchsvc.Source `json:"source,omitempty"`
}

func structuredSearchResponse(response searchsvc.Response) searchStructuredResult {
	out := searchStructuredResult{
		Mode:     response.Mode,
		Notice:   response.Notice,
		Fallback: response.Fallback,
		Results:  make([]searchStructuredMatch, 0, len(response.Results)),
	}
	for _, result := range response.Results {
		out.Results = append(out.Results, searchStructuredMatch{
			Path:    result.Path,
			Line:    result.Line,
			Snippet: result.Snippet,
			Source:  result.Source,
		})
	}
	return out
}
