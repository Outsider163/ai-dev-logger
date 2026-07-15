// Package semantic contains small, local vector-search building blocks.
package semantic

import (
	"fmt"
	"math"
)

// CosineSimilarity returns the cosine of the angle between two vectors.
// It is close to 1 when vectors point in similar directions.
func CosineSimilarity(left, right []float64) (float64, error) {
	if len(left) == 0 || len(right) == 0 {
		return 0, fmt.Errorf("vectors must not be empty")
	}
	if len(left) != len(right) {
		return 0, fmt.Errorf("vector dimensions differ: %d and %d", len(left), len(right))
	}

	var dotProduct float64
	var leftLength float64
	var rightLength float64
	for i := range left {
		dotProduct += left[i] * right[i]
		leftLength += left[i] * left[i]
		rightLength += right[i] * right[i]
	}

	denominator := math.Sqrt(leftLength) * math.Sqrt(rightLength)
	if denominator == 0 {
		return 0, fmt.Errorf("zero vector has no cosine similarity")
	}

	return dotProduct / denominator, nil
}
