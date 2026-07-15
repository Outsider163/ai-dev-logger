package semantic

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	score, err := CosineSimilarity([]float64{1, 0}, []float64{2, 0})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(score-1) > 0.000001 {
		t.Fatalf("expected 1, got %v", score)
	}
}

func TestCosineSimilarityRejectsDifferentDimensions(t *testing.T) {
	if _, err := CosineSimilarity([]float64{1}, []float64{1, 2}); err == nil {
		t.Fatal("expected a dimension error")
	}
}
