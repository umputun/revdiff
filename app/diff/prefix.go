package diff

import "strings"

// normalizePrefixes trims whitespace and trailing slashes from each prefix,
// skipping empty values (e.g., from env var trailing commas).
func normalizePrefixes(prefixes []string) []string {
	normalized := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		p = strings.TrimRight(p, "/")
		if p == "" {
			continue
		}
		normalized = append(normalized, p)
	}
	return normalized
}

// matchesPrefix returns true if the file path matches any prefix.
// A prefix matches if the file equals the prefix exactly, or starts with prefix + "/".
func matchesPrefix(file string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if file == prefix || strings.HasPrefix(file, prefix+"/") {
			return true
		}
	}
	return false
}
