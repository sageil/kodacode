package tool

import (
	"os"
	"strings"
)

func CurrentObservedResourceState(kind ObservedResourceKind, path string) (string, bool, error) {
	switch kind {
	case ObservedResourceFileContent:
		return fileContentObservedState(path)
	case ObservedResourceDirEntries:
		return dirEntriesObservedVersion(path)
	default:
		return "", false, nil
	}
}

func CurrentObservedResourceVersion(kind ObservedResourceKind, path string) (string, bool, error) {
	switch kind {
	case ObservedResourceFileContent:
		version, err := ReadObservedVersion(path)
		if err != nil {
			return "", false, err
		}
		return version, true, nil
	case ObservedResourceDirEntries:
		return dirEntriesObservedVersion(path)
	default:
		return "", false, nil
	}
}

func ObserveDirEntriesResource(path string) (ObservedResource, bool) {
	version, ok, err := dirEntriesObservedVersion(path)
	if err != nil || !ok {
		return ObservedResource{}, false
	}
	return ObservedResource{
		Kind:    ObservedResourceDirEntries,
		Path:    path,
		Version: version,
	}, true
}

func ReadDirWithObservedResource(path string) ([]os.DirEntry, *ObservedResource, error) {
	before, beforeOK, err := dirEntriesObservedVersion(path)
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
	after, afterOK, err := dirEntriesObservedVersion(path)
	if err != nil || !afterOK || before != after {
		return entries, nil, nil
	}
	observed := &ObservedResource{
		Kind:    ObservedResourceDirEntries,
		Path:    path,
		Version: before,
	}
	return entries, observed, nil
}

func fileContentObservedState(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	if info.IsDir() {
		return "", false, nil
	}
	state, ok, err := observedVersionStateForInfo(info)
	if err != nil || !ok {
		return "", false, err
	}
	token, ok := observedVersionStateToken(state)
	if !ok || strings.TrimSpace(token) == "" {
		return "", false, nil
	}
	return token, true, nil
}

func dirEntriesObservedVersion(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() {
		return "", false, nil
	}
	state, ok, err := observedVersionStateForInfo(info)
	if err != nil || !ok {
		return "", false, err
	}
	token, ok := observedVersionStateToken(state)
	if !ok || strings.TrimSpace(token) == "" {
		return "", false, nil
	}
	return token, true, nil
}

func readObservedVersionStateForFile(file *os.File) (observedVersionState, bool, error) {
	info, err := file.Stat()
	if err != nil {
		return observedVersionState{}, false, err
	}
	return observedVersionStateForInfo(info)
}

func stableObservedVersionState(file *os.File, before observedVersionState, beforeOK bool) (observedVersionState, bool, error) {
	if !beforeOK {
		return observedVersionState{}, false, nil
	}
	after, afterOK, err := readObservedVersionStateForFile(file)
	if err != nil {
		return observedVersionState{}, false, err
	}
	if !afterOK || after != before {
		return observedVersionState{}, false, nil
	}
	return before, true, nil
}
