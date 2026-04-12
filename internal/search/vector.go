package search

import (
	"encoding/binary"
	"math"
)

// CosineSimilarity returns the cosine similarity between two float32 vectors.
// Returns 0 if the vectors have different lengths or either has zero magnitude.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB)))
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// VectorToBlob encodes a float32 slice as a little-endian byte slice.
func VectorToBlob(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// BlobToVector decodes a little-endian byte slice into a float32 slice.
// Returns nil if the blob length is not a multiple of 4 (corrupted data).
func BlobToVector(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	if len(b)%4 != 0 {
		return nil
	}
	n := len(b) / 4
	v := make([]float32, n)
	for i := range n {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
