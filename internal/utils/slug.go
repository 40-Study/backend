package utils

import (
	"regexp"
	"strings"
)

var (
	nonAlphanumRegex = regexp.MustCompile(`[^a-z0-9\s-]`)
	whitespaceRegex  = regexp.MustCompile(`[\s-]+`)
)

// GenerateSlug creates a URL-friendly slug from a string.
func GenerateSlug(s string) string {
	result := strings.ToLower(strings.TrimSpace(s))
	result = nonAlphanumRegex.ReplaceAllString(result, "")
	result = whitespaceRegex.ReplaceAllString(result, "-")
	result = strings.Trim(result, "-")
	return result
}
