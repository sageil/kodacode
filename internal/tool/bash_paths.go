package tool

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/sageil/kodacode/internal/workspace"
)

type bashPathAnalysis struct {
	Requests         []PathRequest
	OpaquePathReason string
}

type bashPathAnalyzer struct {
	workingDir       string
	requests         []PathRequest
	opaquePathReason string
}

func analyzeBashPathRequests(workingDir, command string) (bashPathAnalysis, error) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return bashPathAnalysis{}, err
	}
	analyzer := &bashPathAnalyzer{workingDir: strings.TrimSpace(workingDir)}
	analyzer.visitStmtList(file.Stmts)
	return bashPathAnalysis{
		Requests:         dedupePathRequests(analyzer.requests),
		OpaquePathReason: analyzer.opaquePathReason,
	}, nil
}

func (a *bashPathAnalyzer) fork() *bashPathAnalyzer {
	return &bashPathAnalyzer{
		workingDir: a.workingDir,
	}
}

func (a *bashPathAnalyzer) merge(other *bashPathAnalyzer) {
	if other == nil {
		return
	}
	a.requests = append(a.requests, other.requests...)
	if strings.TrimSpace(a.opaquePathReason) == "" && strings.TrimSpace(other.opaquePathReason) != "" {
		a.opaquePathReason = other.opaquePathReason
	}
}

func (a *bashPathAnalyzer) addRequest(access workspace.Access, path, reason string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	a.requests = append(a.requests, PathRequest{
		Access: access,
		Path:   path,
		Reason: reason,
	})
}

func (a *bashPathAnalyzer) setOpaquePathReason(reason string) {
	if strings.TrimSpace(a.opaquePathReason) != "" || strings.TrimSpace(reason) == "" {
		return
	}
	a.opaquePathReason = reason
}
