package detector

// ComputeTV computes the Total Variation distance between two token count distributions.
// TV(P,Q) = 0.5 * sum(|P(x) - Q(x)|) for all x, where P and Q are normalized to probabilities.
func ComputeTV(p, q map[string]int) float64 {
	totalP := countSum(p)
	totalQ := countSum(q)
	if totalP == 0 && totalQ == 0 {
		return 0
	}
	if totalP == 0 || totalQ == 0 {
		// One distribution is empty — treat as maximum distance
		return 1.0
	}

	keys := make(map[string]struct{})
	for k := range p {
		keys[k] = struct{}{}
	}
	for k := range q {
		keys[k] = struct{}{}
	}

	var sum float64
	for k := range keys {
		pProb := float64(p[k]) / float64(totalP)
		qProb := float64(q[k]) / float64(totalQ)
		diff := pProb - qProb
		if diff < 0 {
			diff = -diff
		}
		sum += diff
	}
	return 0.5 * sum
}

func countSum(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

// AverageTV returns the mean of a slice of TV distances.
func AverageTV(tvs []float64) float64 {
	if len(tvs) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range tvs {
		sum += v
	}
	return sum / float64(len(tvs))
}
