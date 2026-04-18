package tokens

import (
	_ "embed"
	"strings"
)

//go:embed fallback.txt
var fallbackData string

// Load returns all non-empty lines from the embedded token list.
func Load() []string {
	lines := strings.Split(fallbackData, "\n")
	result := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			result = append(result, l)
		}
	}
	return result
}
