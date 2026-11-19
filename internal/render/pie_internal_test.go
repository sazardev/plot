package render

import (
	"math"
	"testing"
)

func TestPiePercentagesSumToOneHundred(t *testing.T) {
	t.Parallel()

	tests := [][]float64{
		{1, 1, 1},
		{1, 2, 3},
		{30, 20, 50},
		{1, 1, 1, 1, 1, 1, 1},
	}
	for _, values := range tests {
		percents := piePercentages(values)
		sum := 0.0
		for _, percent := range percents {
			sum += percent
		}
		if math.Abs(sum-100) > 0.001 {
			t.Fatalf("percentages for %v sum to %v", values, sum)
		}
	}
}
