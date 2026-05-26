package tool

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var readBinaryExtensions = map[string]struct{}{
	".zip": {}, ".gz": {}, ".tar": {}, ".rar": {}, ".7z": {},
	".exe": {}, ".dll": {}, ".so": {}, ".dylib": {},
	".wasm": {}, ".pyc": {}, ".pyo": {},
	".o": {}, ".a": {}, ".lib": {},
	".class": {}, ".jar": {},
	".ico": {}, ".ttf": {}, ".otf": {}, ".woff": {}, ".woff2": {}, ".eot": {},
	".sqlite": {}, ".db": {},
	".dat": {}, ".bin": {},
}

var readMediaExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".bmp":  "image/bmp",
	".webp": "image/webp",
	".pdf":  "application/pdf",
}

const (
	readTransientRetryAttempts = 3
	readTransientRetryDelay    = 25 * time.Millisecond
)

type readCapturedLine struct {
	number int
	text   string
}

type readPathRenderer func(resolvedPath, displayPath string, startLine, limit int) (readResult, error)

func renderReadPath(resolvedPath, displayPath string, startLine, limit int) (readResult, error) {
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return readResult{}, err
	}
	if info.IsDir() {
		return readResult{}, fmt.Errorf("%s is a directory, not a file", displayPath)
	}

	if placeholder, ok, err := renderReadPlaceholder(resolvedPath, displayPath, info); err != nil {
		return readResult{}, err
	} else if ok {
		return placeholder, nil
	}

	return renderReadTextFile(resolvedPath, displayPath, startLine, limit)
}

func renderReadPathWithTransientRetry(resolvedPath, displayPath string, startLine, limit int) (readResult, error) {
	return renderReadPathWithTransientRetryFunc(renderReadPath, time.Sleep, resolvedPath, displayPath, startLine, limit)
}

func renderReadPathWithTransientRetryFunc(renderer readPathRenderer, sleep func(time.Duration), resolvedPath, displayPath string, startLine, limit int) (readResult, error) {
	for attempt := 1; attempt <= readTransientRetryAttempts; attempt++ {
		result, err := renderer(resolvedPath, displayPath, startLine, limit)
		if !readTransientEOF(err) {
			return result, err
		}
		if attempt < readTransientRetryAttempts && sleep != nil {
			sleep(readTransientRetryDelay)
		}
	}
	return readResult{}, readTransientEOFMessage(readTransientRetryAttempts)
}

func readTransientEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func readTransientEOFMessage(attempts int) error {
	if attempts <= 1 {
		return errors.New("file changed while reading: hit EOF. The file may have been truncated or replaced by another process; retry the read after the write finishes")
	}
	return fmt.Errorf("file changed while reading: hit EOF after %d attempts. The file may have been truncated or replaced by another process; retry the read after the write finishes", attempts)
}

func renderReadPlaceholder(resolvedPath, displayPath string, info os.FileInfo) (readResult, bool, error) {
	ext := strings.ToLower(filepath.Ext(resolvedPath))
	if mime, ok := readMediaExtensions[ext]; ok {
		return readResult{
			path: displayPath,
			body: fmt.Sprintf("(%s file, %d bytes. Binary content omitted to protect context budget.)", mime, info.Size()),
		}, true, nil
	}
	if _, ok := readBinaryExtensions[ext]; ok {
		return readResult{
			path: displayPath,
			body: fmt.Sprintf("(binary file, %d bytes. Contents omitted to protect context budget.)", info.Size()),
		}, true, nil
	}

	binary, err := isLikelyBinaryFile(resolvedPath)
	if err != nil {
		return readResult{}, false, err
	}
	if !binary {
		return readResult{}, false, nil
	}
	return readResult{
		path: displayPath,
		body: fmt.Sprintf("(binary file, %d bytes. Contents omitted to protect context budget.)", info.Size()),
	}, true, nil
}

func renderReadTextFile(resolvedPath, displayPath string, startLine, limit int) (readResult, error) {
	file, err := os.Open(resolvedPath)
	if err != nil {
		return readResult{}, err
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReader(file)
	version := newReadVersionAccumulator()
	stateBefore, stateBeforeOK, err := readObservedVersionStateForFile(file)
	if err != nil {
		return readResult{}, err
	}
	windowEndLine := effectiveReadWindowEndLine(startLine, limit)
	capacityHint := 64
	if limit > 0 {
		capacityHint = min(limit, 64)
	}
	capturedLines := make([]readCapturedLine, 0, capacityHint)
	totalLines := 0

	for {
		rawLine, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return readResult{}, fmt.Errorf("read %s: %w", displayPath, readErr)
		}
		if rawLine == "" && readErr == io.EOF {
			break
		}

		version.Write([]byte(rawLine))
		totalLines++
		if totalLines >= startLine && (windowEndLine == 0 || totalLines <= windowEndLine) {
			capturedLines = append(capturedLines, readCapturedLine{
				number: totalLines,
				text:   trimReadLine(rawLine),
			})
		}
		if readErr == io.EOF {
			break
		}
	}

	token := version.Token()
	stateToken := ""
	stableState, stableStateOK, err := stableObservedVersionState(file, stateBefore, stateBeforeOK)
	if err != nil {
		return readResult{}, err
	}
	if stableStateOK {
		storeCachedReadObservedVersion(resolvedPath, stableState, token)
		if tokenValue, ok := observedVersionStateToken(stableState); ok {
			stateToken = tokenValue
		}
	}
	if totalLines == 0 {
		return readResult{
			path:       displayPath,
			body:       "(empty file)",
			version:    token,
			state:      stateToken,
			totalLines: 0,
			complete:   true,
		}, nil
	}
	if startLine > totalLines {
		return readResult{
			path:       displayPath,
			body:       fmt.Sprintf("(offset %d exceeds file length of %d lines)", startLine-1, totalLines),
			version:    token,
			state:      stateToken,
			totalLines: totalLines,
		}, nil
	}
	requestedWindowEndLine := min(totalLines, windowEndLine)
	width := len(strconv.Itoa(requestedWindowEndLine))
	bodyLines, firstShownLine, lastShownLine := renderCapturedReadLines(capturedLines, width)
	if lastShownLine == 0 {
		return readResult{
			path:    displayPath,
			body:    fmt.Sprintf("(no lines shown from offset %d)", startLine-1),
			version: token,
		}, nil
	}

	footer := readWindowFooter(startLine, lastShownLine, totalLines, lastShownLine < totalLines)
	bodyLines = append(bodyLines, footer)
	return readResult{
		path:       displayPath,
		body:       strings.Join(bodyLines, "\n"),
		version:    token,
		state:      stateToken,
		startLine:  firstShownLine,
		endLine:    lastShownLine,
		totalLines: totalLines,
		complete:   firstShownLine == 1 && lastShownLine == totalLines,
	}, nil
}

func effectiveReadWindowEndLine(startLine, limit int) int {
	return startLine + limit - 1
}

func renderCapturedReadLines(capturedLines []readCapturedLine, width int) ([]string, int, int) {
	bodyLines := make([]string, 0, len(capturedLines))
	firstShownLine := 0
	lastShownLine := 0
	for _, capturedLine := range capturedLines {
		entry := fmt.Sprintf("%*d: %s", width, capturedLine.number, capturedLine.text)
		if firstShownLine == 0 {
			firstShownLine = capturedLine.number
		}
		bodyLines = append(bodyLines, entry)
		lastShownLine = capturedLine.number
	}
	return bodyLines, firstShownLine, lastShownLine
}
