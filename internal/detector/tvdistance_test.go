package detector_test

import (
	"math"
	"testing"

	"github.com/ironarmor/llmdetect/internal/detector"
)

func TestComputeTV_Identical(t *testing.T) {
	p := map[string]int{"a": 10, "b": 20}
	q := map[string]int{"a": 10, "b": 20}
	got := detector.ComputeTV(p, q)
	if math.Abs(got) > 1e-9 {
		t.Errorf("identical distributions: TV = %f, want 0", got)
	}
}

func TestComputeTV_Disjoint(t *testing.T) {
	p := map[string]int{"a": 10}
	q := map[string]int{"b": 10}
	got := detector.ComputeTV(p, q)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("disjoint distributions: TV = %f, want 1.0", got)
	}
}

func TestComputeTV_Partial(t *testing.T) {
	// P: a=1/2, b=1/2   Q: a=1/4, b=3/4
	// TV = 0.5 * (|1/2-1/4| + |1/2-3/4|) = 0.5 * (1/4 + 1/4) = 0.25
	p := map[string]int{"a": 2, "b": 2}
	q := map[string]int{"a": 1, "b": 3}
	got := detector.ComputeTV(p, q)
	if math.Abs(got-0.25) > 1e-9 {
		t.Errorf("partial: TV = %f, want 0.25", got)
	}
}

func TestAverageTV(t *testing.T) {
	tvs := []float64{0.1, 0.3, 0.5}
	got := detector.AverageTV(tvs)
	if math.Abs(got-0.3) > 1e-9 {
		t.Errorf("AverageTV = %f, want 0.3", got)
	}
}

func TestAverageTV_Empty(t *testing.T) {
	got := detector.AverageTV(nil)
	if got != 0 {
		t.Errorf("AverageTV(nil) = %f, want 0", got)
	}
}

func TestComputeTV_EmptyChannel(t *testing.T) {
	// Official has data, channel returned nothing (all queries failed)
	p := map[string]int{"world": 10}
	q := map[string]int{}
	got := detector.ComputeTV(p, q)
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("empty channel: TV = %f, want 1.0", got)
	}
}

func TestComputeTV_BothEmpty(t *testing.T) {
	got := detector.ComputeTV(map[string]int{}, map[string]int{})
	if got != 0 {
		t.Errorf("both empty: TV = %f, want 0", got)
	}
}
