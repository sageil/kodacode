package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
)

type readVersionAccumulator struct {
	hash hash.Hash
}

func newReadVersionAccumulator() readVersionAccumulator {
	return readVersionAccumulator{hash: sha256.New()}
}

func (a *readVersionAccumulator) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	_, _ = a.hash.Write(p)
}

func (a readVersionAccumulator) Token() string {
	return hex.EncodeToString(a.hash.Sum(nil))
}

func isLikelyBinaryFile(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()

	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if n == 0 {
		if err == io.EOF {
			return false, nil
		}
		return false, err
	}
	buf = buf[:n]

	nonPrintable := 0
	for _, b := range buf {
		if b == 0 {
			return true, nil
		}
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(len(buf)) > 0.3, nil
}
