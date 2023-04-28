package objectformats

import "strings"

// TrimSpaceMulti delete trailing whitespace on every line
// of the given multi-line text.
//
// This is very useful when comparing table formatted strings.
func TrimSpaceMulti(s string) string {
	lines := strings.Split(s, "\n")
	trimmed := make([]string, len(lines))

	for i, line := range lines {
		trimmed[i] = strings.TrimSpace(line)
	}
	return strings.Join(trimmed, "\n")
}
