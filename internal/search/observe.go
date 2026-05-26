package search

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	observedResourceFileContent = "file_content"
	observedResourceDirEntries  = "dir_entries"
)

type contentVersionAccumulator struct {
	hash hashWriter
}

type hashWriter interface {
	Write([]byte) (int, error)
	Sum([]byte) []byte
}

func newContentVersionAccumulator() contentVersionAccumulator {
	return contentVersionAccumulator{hash: sha256.New()}
}

func (a *contentVersionAccumulator) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	_, _ = a.hash.Write(p)
}

func (a contentVersionAccumulator) Token() string {
	return hex.EncodeToString(a.hash.Sum(nil))
}
