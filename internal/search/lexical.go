package search

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	snippetMaxChars     = 160
	observedResourceMax = 1024
)

var errMatchLimit = errors.New("match limit reached")

func Lexical(req Request) ([]Result, error) {
	response, err := LexicalObserved(req)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

func LexicalObserved(req Request) (Response, error) {
	return lexicalObservedWithSkipDirs(req, defaultSkipDirMatcher())
}

func (s *Service) lexicalObserved(req Request) (Response, error) {
	return lexicalObservedWithSkipDirs(req, s.skipDirs)
}

func lexicalObservedWithSkipDirs(req Request, skipDirs skipDirMatcher) (Response, error) {
	info, err := os.Stat(req.RootPath)
	if err != nil {
		return Response{}, err
	}
	if info.IsDir() {
		return lexicalDirectoryObserved(req, skipDirs)
	}
	outcome, err := lexicalFileObserved(req, req.RootPath, filepath.Base(req.RootPath), req.MaxResults)
	if err != nil {
		return Response{}, err
	}
	observation := &Observation{
		Complete:  outcome.Complete,
		Resources: append([]ObservedResource(nil), outcome.Resources...),
	}
	return Response{
		Mode:        ModeLexical,
		Results:     append([]Result(nil), outcome.Results...),
		Observation: observation,
	}, nil
}

type lexicalOutcome struct {
	Results   []Result
	Resources []ObservedResource
	Complete  bool
}

func lexicalDirectoryObserved(req Request, skipDirs skipDirMatcher) (Response, error) {
	results := make([]Result, 0, req.MaxResults)
	resources := make([]ObservedResource, 0, 32)
	complete := true
	captureResources := true

	var walk func(string) error
	walk = func(dir string) error {
		entries, observedDir, err := readDirStable(dir)
		if err != nil {
			return err
		}
		if observedDir != nil && captureResources {
			var ok bool
			resources, ok = appendObservedResources(resources, *observedDir)
			if !ok {
				complete = false
				captureResources = false
				resources = nil
			}
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() && skipDirs.shouldSkip(name) {
				continue
			}
			current := filepath.Join(dir, name)
			relPath := relPathFromRoot(req.RootPath, current)
			if entry.IsDir() {
				if err := walk(current); err != nil {
					return err
				}
				continue
			}
			fileOutcome, err := lexicalFileObserved(req, current, relPath, req.MaxResults-len(results))
			if err != nil {
				return err
			}
			results = append(results, fileOutcome.Results...)
			if captureResources && len(fileOutcome.Resources) > 0 {
				var ok bool
				resources, ok = appendObservedResources(resources, fileOutcome.Resources...)
				if !ok {
					complete = false
					captureResources = false
					resources = nil
				}
			}
			if !fileOutcome.Complete {
				complete = false
				return errMatchLimit
			}
			if len(results) >= req.MaxResults {
				complete = false
				return errMatchLimit
			}
		}
		return nil
	}

	err := walk(req.RootPath)
	if err != nil && !errors.Is(err, errMatchLimit) {
		return Response{}, err
	}
	if errors.Is(err, errMatchLimit) {
		complete = false
	}
	return Response{
		Mode:    ModeLexical,
		Results: results,
		Observation: &Observation{
			Complete:  complete,
			Resources: resources,
		},
	}, nil
}

func lexicalFileObserved(req Request, absolutePath, relativePath string, limit int) (lexicalOutcome, error) {
	if !matchesGlob(req.Glob, relativePath) {
		return lexicalOutcome{Complete: true}, nil
	}
	file, err := os.Open(absolutePath)
	if err != nil {
		return lexicalOutcome{}, err
	}
	defer file.Close() //nolint:errcheck

	stateBefore, stateBeforeOK, err := observedFileStateToken(file)
	if err != nil {
		return lexicalOutcome{}, err
	}

	binary, probe, err := inspectBinaryProbe(file)
	if err != nil {
		return lexicalOutcome{}, err
	}
	if binary {
		acc := newContentVersionAccumulator()
		acc.Write(probe)
		buf := make([]byte, 32*1024)
		for {
			n, readErr := file.Read(buf)
			if n > 0 {
				acc.Write(buf[:n])
			}
			if readErr == nil {
				continue
			}
			if readErr == io.EOF {
				stateToken, err := stableObservedFileStateToken(file, stateBefore, stateBeforeOK)
				if err != nil {
					return lexicalOutcome{}, err
				}
				return lexicalOutcome{
					Complete: true,
					Resources: []ObservedResource{{
						Kind:    observedResourceFileContent,
						Path:    absolutePath,
						Version: acc.Token(),
						State:   stateToken,
					}},
				}, nil
			}
			return lexicalOutcome{}, readErr
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return lexicalOutcome{}, err
	}

	matcher, err := newLineMatcher(req)
	if err != nil {
		return lexicalOutcome{}, err
	}
	results := make([]Result, 0, max(limit, 0))
	lineNumber := 0
	acc := newContentVersionAccumulator()
	reader := bufio.NewReader(file)
	for {
		rawLine, readErr := reader.ReadBytes('\n')
		if len(rawLine) > 0 {
			lineNumber++
			acc.Write(rawLine)
			line := trimSearchLineEnding(rawLine)
			if !matcher.Match(line) {
				if readErr == io.EOF {
					break
				}
				if readErr == nil {
					continue
				}
				return lexicalOutcome{}, readErr
			}
			results = append(results, Result{
				Path:    displayPath(req.WorkspaceRoot, absolutePath),
				Line:    lineNumber,
				Snippet: summarizeLine(line),
				Source:  SourceLexical,
			})
			if len(results) >= limit {
				return lexicalOutcome{
					Results: results,
					Resources: []ObservedResource{{
						Kind:    observedResourceFileContent,
						Path:    absolutePath,
						Version: acc.Token(),
					}},
					Complete: false,
				}, nil
			}
		}
		if readErr == nil {
			continue
		}
		if readErr == io.EOF {
			break
		}
		return lexicalOutcome{}, readErr
	}
	return lexicalOutcome{
		Results: results,
		Resources: []ObservedResource{{
			Kind:    observedResourceFileContent,
			Path:    absolutePath,
			Version: acc.Token(),
			State:   stableObservedFileStateTokenOrEmpty(file, stateBefore, stateBeforeOK),
		}},
		Complete: true,
	}, nil
}

func appendObservedResources(resources []ObservedResource, additions ...ObservedResource) ([]ObservedResource, bool) {
	if len(additions) == 0 {
		return resources, true
	}
	if len(resources)+len(additions) > observedResourceMax {
		return nil, false
	}
	return append(resources, additions...), true
}

func trimSearchLineEnding(rawLine []byte) string {
	line := rawLine
	if n := len(line); n > 0 && line[n-1] == '\n' {
		line = line[:n-1]
	}
	if n := len(line); n > 0 && line[n-1] == '\r' {
		line = line[:n-1]
	}
	return string(line)
}

func observedFileStateToken(file *os.File) (string, bool, error) {
	info, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	if info.IsDir() {
		return "", false, nil
	}
	state, ok, err := observedStateForInfo(info)
	if err != nil || !ok {
		return "", false, err
	}
	token, ok := observedStateToken(state)
	if !ok {
		return "", false, nil
	}
	return token, true, nil
}

func stableObservedFileStateToken(file *os.File, before string, beforeOK bool) (string, error) {
	if !beforeOK || strings.TrimSpace(before) == "" {
		return "", nil
	}
	after, afterOK, err := observedFileStateToken(file)
	if err != nil {
		return "", err
	}
	if !afterOK || after != before {
		return "", nil
	}
	return before, nil
}

func stableObservedFileStateTokenOrEmpty(file *os.File, before string, beforeOK bool) string {
	token, err := stableObservedFileStateToken(file, before, beforeOK)
	if err != nil {
		return ""
	}
	return token
}

func readDirStable(path string) ([]os.DirEntry, *ObservedResource, error) {
	before, beforeOK, err := dirEntriesVersion(path)
	if err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, nil, err
	}
	if !beforeOK {
		return entries, nil, nil
	}
	after, afterOK, err := dirEntriesVersion(path)
	if err != nil || !afterOK || before != after {
		return entries, nil, nil
	}
	return entries, &ObservedResource{
		Kind:    observedResourceDirEntries,
		Path:    path,
		Version: before,
	}, nil
}

func dirEntriesVersion(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() {
		return "", false, nil
	}
	state, ok, err := observedStateForInfo(info)
	if err != nil || !ok {
		return "", false, err
	}
	token, ok := observedStateToken(state)
	if !ok {
		return "", false, nil
	}
	return token, true, nil
}

func inspectBinaryProbe(file *os.File) (bool, []byte, error) {
	var probe [8000]byte
	n, err := file.Read(probe[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return false, nil, err
	}
	buf := append([]byte(nil), probe[:n]...)
	return bytes.IndexByte(buf, 0) >= 0, buf, nil
}

type lineMatcher struct {
	literal       string
	caseSensitive bool
	regex         *regexp.Regexp
}

func newLineMatcher(req Request) (lineMatcher, error) {
	if req.Regex {
		pattern := req.Query
		if !req.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return lineMatcher{}, err
		}
		return lineMatcher{regex: compiled}, nil
	}

	pattern := req.Query
	if !req.CaseSensitive {
		pattern = strings.ToLower(pattern)
	}
	return lineMatcher{literal: pattern, caseSensitive: req.CaseSensitive}, nil
}

func (m lineMatcher) Match(line string) bool {
	if m.regex != nil {
		return m.regex.MatchString(line)
	}
	return containsPattern(line, m.literal, m.caseSensitive)
}

func FormatResponse(resp Response) string {
	lines := make([]string, 0, len(resp.Results)+1)
	if strings.TrimSpace(resp.Notice) != "" {
		lines = append(lines, "notice: "+strings.TrimSpace(resp.Notice))
	}
	if len(resp.Results) == 0 {
		lines = append(lines, "no matches found")
		return strings.Join(lines, "\n")
	}
	for _, result := range resp.Results {
		if resp.Mode == ModeLexical && !resp.Fallback {
			lines = append(lines, formatResultLine("", result))
			continue
		}
		lines = append(lines, formatResultLine("["+string(result.Source)+"] ", result))
	}
	return strings.Join(lines, "\n")
}

func formatResultLine(prefix string, result Result) string {
	return fmt.Sprintf("%s%s:%d:%s", prefix, result.Path, result.Line, result.Snippet)
}

func matchesGlob(glob, relativePath string) bool {
	if glob == "" {
		return true
	}
	slashPath := filepath.ToSlash(relativePath)
	if strings.Contains(glob, "/") {
		ok, err := path.Match(glob, slashPath)
		return err == nil && ok
	}
	ok, err := path.Match(glob, path.Base(slashPath))
	return err == nil && ok
}

func containsPattern(line, pattern string, caseSensitive bool) bool {
	if caseSensitive {
		return strings.Contains(line, pattern)
	}
	return strings.Contains(strings.ToLower(line), pattern)
}

func relPathFromRoot(rootPath, targetPath string) string {
	rel, err := filepath.Rel(rootPath, targetPath)
	if err != nil {
		return filepath.Base(targetPath)
	}
	return rel
}

func displayPath(workspaceRoot, absolutePath string) string {
	if workspaceRoot == "" {
		return absolutePath
	}
	rel, err := filepath.Rel(workspaceRoot, absolutePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return absolutePath
	}
	return filepath.ToSlash(rel)
}

func summarizeLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "(blank line)"
	}
	runes := []rune(trimmed)
	if len(runes) <= snippetMaxChars {
		return trimmed
	}
	return string(runes[:snippetMaxChars]) + "..."
}
