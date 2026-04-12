package openai

import (
	"math"
	"testing"
)

func TestFloat64sToFloat32s(t *testing.T) {
	input := []float64{0.1, 0.2, -0.3, 1.0, 0.0}
	got := float64sToFloat32s(input)

	if len(got) != len(input) {
		t.Fatalf("len = %d, want %d", len(got), len(input))
	}
	for i, v := range got {
		if diff := math.Abs(float64(v) - input[i]); diff > 1e-6 {
			t.Errorf("[%d] = %f, want %f (diff %e)", i, v, input[i], diff)
		}
	}
}

func TestFloat64sToFloat32s_Empty(t *testing.T) {
	got := float64sToFloat32s(nil)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
