package tool

import (
	"os"

	"github.com/sageil/kodacode/internal/filemutation"
)

func WithFileMutationLock(path string, fn func() error) error {
	return filemutation.WithLock(path, fn)
}

func WriteFileAtomically(path string, content []byte, mode os.FileMode) error {
	return filemutation.WriteAtomically(path, content, mode)
}
