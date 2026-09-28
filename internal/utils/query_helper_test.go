package utils

import "testing"

// QA vòng 2 (G5): pattern "chứa chuỗi" phải escape `\`, `%`, `_` để Postgres ILIKE khớp literal.
// Hành vi ILIKE thật được pin ở repository/admin_keyword_escape_pg_test.go.
func TestContainsLikePattern_EscapeKyTuDacBiet(t *testing.T) {
	cases := map[string]string{
		"abc":   "%abc%",
		"%":     `%\%%`,
		"a_b":   `%a\_b%`,
		`c\d`:   `%c\\d%`,
		`\%_`:   `%\\\%\_%`,
		"Toán ": "%Toán %",
	}
	for in, want := range cases {
		if got := ContainsLikePattern(in); got != want {
			t.Fatalf("ContainsLikePattern(%q) = %q, muốn %q", in, got, want)
		}
	}
}
