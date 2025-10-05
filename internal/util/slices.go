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
