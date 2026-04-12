package tool_test

import (
	"crypto/sha256"
	"encoding/hex"
)

func versionToken(content string) string {
	data := []byte(content)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}
