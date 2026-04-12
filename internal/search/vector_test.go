package search

import (
	"math"
	"testing"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{1, 2, 3}
	if got := CosineSimilarity(a, a); math.Abs(float64(got)-1.0) > 1e-6 {
		t.Errorf("identical vectors: got %f, want 1.0", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	if got := CosineSimilarity(a, b); math.Abs(float64(got)) > 1e-6 {
		t.Errorf("orthogonal vectors: got %f, want 0.0", got)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{-1, -2, -3}
	if got := CosineSimilarity(a, b); math.Abs(float64(got)+1.0) > 1e-6 {
		t.Errorf("opposite vectors: got %f, want -1.0", got)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	if got := CosineSimilarity(a, b); got != 0 {
		t.Errorf("zero vector: got %f, want 0.0", got)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2}
	if got := CosineSimilarity(a, b); got != 0 {
		t.Errorf("different lengths: got %f, want 0.0", got)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	if got := CosineSimilarity(nil, nil); got != 0 {
		t.Errorf("empty vectors: got %f, want 0.0", got)
	}
}

func TestBlobRoundtrip(t *testing.T) {
	original := []float32{0.1, -0.5, 3.14, 0, -1e10}
	blob := VectorToBlob(original)

	if len(blob) != len(original)*4 {
		t.Fatalf("blob size = %d, want %d", len(blob), len(original)*4)
	}

	decoded := BlobToVector(blob)
	if len(decoded) != len(original) {
		t.Fatalf("decoded len = %d, want %d", len(decoded), len(original))
	}
	for i, v := range decoded {
		if v != original[i] {
			t.Errorf("[%d] = %f, want %f", i, v, original[i])
		}
	}
}

func TestBlobEmpty(t *testing.T) {
	blob := VectorToBlob(nil)
	if len(blob) != 0 {
		t.Errorf("empty vector blob len = %d, want 0", len(blob))
	}
	decoded := BlobToVector(nil)
	if decoded != nil {
		t.Errorf("empty blob decoded = %v, want nil", decoded)
	}
}

func TestBlobToVector_CorruptedLength(t *testing.T) {
	// 5 bytes is not a multiple of 4.
	corrupted := []byte{0x00, 0x01, 0x02, 0x03, 0x04}
	decoded := BlobToVector(corrupted)
	if decoded != nil {
		t.Errorf("corrupted blob decoded = %v, want nil", decoded)
	}
}
