package team

import (
	"strings"
	"unicode/utf8"
)

// previewForLog returns a single-line, size-capped preview suitable for stderr.
func previewForLog(s string, max int) string {
	if max <= 0 {
		max = 200
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimSpace(s)
	if s == "" {
		return "<empty>"
	}
	if len(s) <= max {
		return strings.ReplaceAll(s, "\n", "\\n")
	}
	n := max
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	if n == 0 {
		n = max
	}
	return strings.ReplaceAll(s[:n], "\n", "\\n") + "..."
}
