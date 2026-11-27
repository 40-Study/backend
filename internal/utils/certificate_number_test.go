package utils

import "testing"

func TestNormalizeCertificateNumber(t *testing.T) {
	const clean = "FORTEX-748c42ba-2dfeb3"
	cases := map[string]string{
		"sach":                          clean,
		"khoang trang + tab/xuong dong": "  \t" + clean + " \n",
		"nbsp hai dau":                  "\u00a0" + clean + "\u00a0",
		"bom":                           "\ufeff" + clean,
		"zero-width space cuoi":         clean + "\u200b",
		"zwnj zwj giua ma":              "FORTEX-748c\u200c42ba\u200d-2dfeb3",
		"word joiner dau":               "\u2060" + clean,
		"hon hop":                       " \u200b\u00a0" + clean + "\u200b\u00a0 ",
	}
	for name, in := range cases {
		if got := NormalizeCertificateNumber(in); got != clean {
			t.Errorf("%s (%q): got %q, want %q", name, in, got, clean)
		}
	}
	if got := NormalizeCertificateNumber("\u200b \u00a0\ufeff"); got != "" {
		t.Errorf("chi ky tu vo hinh: got %q, want rong", got)
	}
	if got := NormalizeCertificateNumber("FORTEX-ABC"); got != "FORTEX-ABC" {
		t.Errorf("khong duoc doi kieu chu: got %q", got)
	}
}
