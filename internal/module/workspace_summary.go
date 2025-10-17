package module

// FilterWorkspaceSummaries filters workspace summaries by the given filter (all, active, inactive).
func FilterWorkspaceSummaries(summaries []WorkspaceSummary, filter string) []WorkspaceSummary {
	if filter == "all" {
		return summaries
	}
	filtered := make([]WorkspaceSummary, 0, len(summaries))
	for _, summary := range summaries {
		switch filter {
		case "active":
			if (summary.Configured && summary.Exists) || (summary.Temporary && summary.Exists) {
				filtered = append(filtered, summary)
			}
		case "inactive":
			if summary.Configured && !summary.Exists {
				filtered = append(filtered, summary)
			}
		}
	}
	return filtered
}
