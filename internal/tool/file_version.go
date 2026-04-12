package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"os"
	"sync"
)

func fileVersion(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:8])
}

type versionAccumulator struct {
	hash hash.Hash
}

func (a *versionAccumulator) ensure() {
	if a.hash == nil {
		a.hash = sha256.New()
	}
}

func (a *versionAccumulator) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	a.ensure()
	_, _ = a.hash.Write(p)
}

func (a *versionAccumulator) Token() string {
	a.ensure()
	return hex.EncodeToString(a.hash.Sum(nil)[:8])
}

func versionMismatchMessage(path, expected, current string) string {
	return "file version mismatch for " + path + `. Expected version "` + expected + `", current version is "` + current + `". Reread the file and retry with the current version.`
}

func versionMissingMessage(path, expected string) string {
	return "file version mismatch for " + path + `. Expected version "` + expected + `", but the file no longer exists. Reread the file and retry.`
}

type cachedFileSnapshot struct {
	identity   fileSnapshotIdentity
	version    string
	totalLines int
}

var fileSnapshotCache sync.Map

func cachedSnapshot(path string, info os.FileInfo) (cachedFileSnapshot, bool) {
	entry, ok := fileSnapshotCache.Load(path)
	if !ok {
		return cachedFileSnapshot{}, false
	}
	snap, ok := entry.(cachedFileSnapshot)
	if !ok {
		return cachedFileSnapshot{}, false
	}
	if snap.identity != snapshotIdentity(info) {
		return cachedFileSnapshot{}, false
	}
	return snap, true
}

func storeSnapshot(path string, info os.FileInfo, version string, totalLines int) {
	fileSnapshotCache.Store(path, cachedFileSnapshot{
		identity:   snapshotIdentity(info),
		version:    version,
		totalLines: totalLines,
	})
}

func invalidateSnapshot(path string) {
	fileSnapshotCache.Delete(path)
}
