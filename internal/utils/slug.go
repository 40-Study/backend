package utils

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	nonAlphanumRegex = regexp.MustCompile(`[^a-z0-9\s-]`)
	whitespaceRegex  = regexp.MustCompile(`[\s-]+`)
)

// vietnameseMap maps Vietnamese special characters that NFD normalization
// doesn't handle correctly (đ/Đ are not decomposed by Unicode NFD).
var vietnameseMap = map[rune]string{
	'đ': "d", 'Đ': "d",
}

// removeVietnameseDiacritics strips diacritics using Unicode NFD normalization
// and handles Vietnamese-specific characters (đ/Đ).
func removeVietnameseDiacritics(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	// Replace Vietnamese-specific chars first
	for _, r := range s {
		if mapped, ok := vietnameseMap[r]; ok {
			b.WriteString(mapped)
		} else {
			b.WriteRune(r)
		}
	}

	// NFD decompose: "ậ" → "a" + combining marks, then strip combining marks
	normalized := norm.NFD.String(b.String())
	var result strings.Builder
	result.Grow(len(normalized))
	for _, r := range normalized {
		if !unicode.Is(unicode.Mn, r) { // Mn = Mark, Nonspacing (combining diacritics)
			result.WriteRune(r)
		}
	}
	return result.String()
}

// GenerateSlug creates a URL-friendly slug from a string.
// Handles Vietnamese diacritics: "Lập trình Go" → "lap-trinh-go"
func GenerateSlug(s string) string {
	result := removeVietnameseDiacritics(s)
	result = strings.ToLower(strings.TrimSpace(result))
	result = nonAlphanumRegex.ReplaceAllString(result, "")
	result = whitespaceRegex.ReplaceAllString(result, "-")
	result = strings.Trim(result, "-")
	return result
}

// SlugExistsFunc checks if a slug already exists in the database.
// Used by GenerateUniqueSlug to avoid duplicate slugs.
type SlugExistsFunc func(slug string) (bool, error)

// GenerateUniqueSlug creates a slug and appends -2, -3, etc. if it already exists.
// Uses a callback to check DB existence, handling race conditions via DB unique constraint.
func GenerateUniqueSlug(title string, existsFn SlugExistsFunc) (string, error) {
	base := GenerateSlug(title)
	if base == "" {
		base = "untitled"
	}

	// Try base slug first
	exists, err := existsFn(base)
	if err != nil {
		return "", fmt.Errorf("failed to check slug existence: %w", err)
	}
	if !exists {
		return base, nil
	}

	// Append incrementing suffix: -2, -3, ...
	for i := 2; i <= 100; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		exists, err = existsFn(candidate)
		if err != nil {
			return "", fmt.Errorf("failed to check slug existence: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("unable to generate unique slug for %q after 100 attempts", title)
}
