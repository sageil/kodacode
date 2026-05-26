package tool

import (
	"fmt"
	"io"
	"os"
	"sync"
)

type readVersionCacheEntry struct {
	state   observedVersionState
	version string
}

var readObservedVersionCache sync.Map

// ReadObservedVersion returns the same full-file content token that the read
// tool records in Result.ObservedResources for successful text-file reads.
func ReadObservedVersion(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	before, beforeOK, err := readObservedVersionStateForFile(file)
	if err != nil {
		return "", err
	}
	if beforeOK {
		if cached, ok := cachedReadObservedVersion(path, before); ok {
			return cached, nil
		}
	}

	version, err := readObservedVersionFromFile(path, file)
	if err != nil {
		return "", err
	}

	if !beforeOK {
		return version, nil
	}
	stableState, stableOK, err := stableObservedVersionState(file, before, beforeOK)
	if err != nil {
		return "", err
	}
	if stableOK {
		storeCachedReadObservedVersion(path, stableState, version)
	}
	return version, nil
}

func readObservedVersionFromFile(path string, file *os.File) (string, error) {
	version := newReadVersionAccumulator()
	buf := make([]byte, 32*1024)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			version.Write(buf[:n])
		}
		if readErr == nil {
			continue
		}
		if readErr == io.EOF {
			return version.Token(), nil
		}
		return "", fmt.Errorf("read %s: %w", path, readErr)
	}
}

func cachedReadObservedVersion(path string, state observedVersionState) (string, bool) {
	entry, ok := readObservedVersionCache.Load(path)
	if !ok {
		return "", false
	}
	cached, ok := entry.(readVersionCacheEntry)
	if !ok || cached.state != state || cached.version == "" {
		if !ok {
			readObservedVersionCache.Delete(path)
		}
		return "", false
	}
	return cached.version, true
}

func storeCachedReadObservedVersion(path string, state observedVersionState, version string) {
	if path == "" || version == "" {
		return
	}
	readObservedVersionCache.Store(path, readVersionCacheEntry{
		state:   state,
		version: version,
	})
}
