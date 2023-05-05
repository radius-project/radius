package objectformats

import "strings"

// TrimSpaceMulti delete trailing whitespace on every line
// of the given multi-line text.
//
// This is very useful when comparing table formatted strings.
//
// # Function Explanation
// 
//	TrimSpaceMulti takes in a string and returns a string with all the whitespace trimmed from each line. It does this by 
//	splitting the string into lines, trimming the whitespace from each line, and then joining the lines back together. If an
//	 error occurs, it will be returned to the caller.
func TrimSpaceMulti(s string) string {
	lines := strings.Split(s, "\n")
	trimmed := make([]string, len(lines))

	for i, line := range lines {
		trimmed[i] = strings.TrimSpace(line)
	}
	return strings.Join(trimmed, "\n")
}
