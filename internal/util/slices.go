package util

// IndexOf returns the index of needle in values, or -1 if not found.
func IndexOf(values []string, needle string) int {
	for i, v := range values {
		if v == needle {
			return i
		}
	}
	return -1
}

// MergeStrings merges base and extra slices, removing duplicates while preserving order.
func MergeStrings(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	merged := make([]string, 0, len(base)+len(extra))
	for _, v := range base {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		merged = append(merged, v)
	}
	for _, v := range extra {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		merged = append(merged, v)
	}
	return merged
}

// CyclicNext returns the next item in the slice after current, wrapping to the beginning if needed.
// If current is not found, returns the first item. Returns empty string if slice is empty.
func CyclicNext(values []string, current string) string {
	if len(values) == 0 {
		return ""
	}
	idx := IndexOf(values, current)
	if idx == -1 {
		return values[0]
	}
	return values[(idx+1)%len(values)]
}

// CyclicPrev returns the previous item in the slice before current, wrapping to the end if needed.
// If current is not found, returns the last item. Returns empty string if slice is empty.
func CyclicPrev(values []string, current string) string {
	if len(values) == 0 {
		return ""
	}
	idx := IndexOf(values, current)
	if idx == -1 {
		return values[len(values)-1]
	}
	nextIdx := idx - 1
	if nextIdx < 0 {
		nextIdx = len(values) - 1
	}
	return values[nextIdx]
}

// CyclicIndex returns the item at the position current + delta with wraparound.
// If current is not found and delta > 0, starts from the first item.
// If current is not found and delta <= 0, starts from the last item.
// Returns empty string if slice is empty.
func CyclicIndex(values []string, current string, delta int) string {
	if len(values) == 0 {
		return ""
	}

	idx := IndexOf(values, current)
	if idx == -1 {
		if delta > 0 {
			idx = 0
		} else {
			idx = len(values) - 1
		}
	} else {
		idx = idx + delta
		// Handle wraparound for both positive and negative deltas
		idx = (idx%len(values) + len(values)) % len(values)
	}

	return values[idx]
}
