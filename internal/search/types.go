package search

import (
	"errors"
	"fmt"
	"strings"
)

type Mode string

const (
	ModeLexical Mode = "lexical"
	ModeHybrid  Mode = "hybrid"
)

type Source string

const (
	SourceLexical  Source = "lexical"
	SourceSemantic Source = "semantic"
	SourceMerged   Source = "merged"
)

var (
	ErrQueryRequired            = errors.New("query is required")
	ErrRootPathRequired         = errors.New("root_path is required")
	ErrModeInvalid              = errors.New("search mode must be lexical or hybrid")
	ErrRegexUnsupportedInHybrid = errors.New("regex search is supported only in lexical mode")
)

type Request struct {
	Query         string
	RootPath      string
	WorkspaceRoot string
	Glob          string
	Regex         bool
	CaseSensitive bool
	MaxResults    int
	Mode          Mode
}

func (r Request) Validate() error {
	if strings.TrimSpace(r.Query) == "" {
		return ErrQueryRequired
	}
	if strings.TrimSpace(r.RootPath) == "" {
		return ErrRootPathRequired
	}
	switch r.Mode {
	case "", ModeLexical, ModeHybrid:
		if r.Regex && r.Mode == ModeHybrid {
			return ErrRegexUnsupportedInHybrid
		}
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrModeInvalid, r.Mode)
	}
}

type Result struct {
	Path    string
	Line    int
	Snippet string
	Source  Source
	score   float64
}

type ObservedResource struct {
	Kind    string
	Path    string
	Version string
	State   string
}

type Observation struct {
	Complete  bool
	Resources []ObservedResource
}

type Response struct {
	Mode        Mode
	Notice      string
	Results     []Result
	Fallback    bool
	Observation *Observation
}
