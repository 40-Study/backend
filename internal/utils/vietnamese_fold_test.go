package utils

import (
	"testing"

	"study.com/v1/internal/testutil/pgtest"
)

func TestFoldVietnamese(t *testing.T) {
	cases := map[string]string{
		"Đại số tuyến tính": "dai so tuyen tinh",
		"ĐẠI SỐ":            "dai so",
		"dai so":            "dai so",
		"Toán rời rạc":      "toan roi rac",
		"Người Việt":        "nguoi viet",
		"Ưu tiên Ơn":        "uu tien on",
		"Go & C++ 101":      "go & c++ 101",
		"100%":              "100%",
		"":                  "",
		// Dấu rời (NFD) từ bàn phím/IME khác phải gộp về dạng dựng sẵn rồi mới bỏ dấu.
		"Đại số": "dai so",
	}
	for in, want := range cases {
		if got := FoldVietnamese(in); got != want {
			t.Errorf("FoldVietnamese(%q) = %q, muốn %q", in, got, want)
		}
	}
}

func TestVietnameseFoldTables_SameLength(t *testing.T) {
	if f, to := []rune(vietnameseFoldFrom), []rune(vietnameseFoldTo); len(f) != len(to) || len(f) == 0 {
		t.Fatalf("translate() cần hai chuỗi cùng độ dài rune: from=%d to=%d", len(f), len(to))
	}
}

// Phép bỏ dấu phía Go (từ khoá) và phía SQL (cột) phải ra CÙNG kết quả với từng ký tự trong bảng: lệch một ký
// tự là từ khoá có chữ đó không bao giờ khớp cột. Postgres thật.
func TestVietnameseFold_GoAndSQLAgree(t *testing.T) {
	db := pgtest.Open(t)
	for _, r := range vietnameseFoldFrom {
		in := "x" + string(r) + "Y"
		var sqlFolded string
		if err := db.Raw("SELECT " + vietnameseFoldSQL("?::text"), in).Scan(&sqlFolded).Error; err != nil {
			t.Fatalf("SQL fold %q: %v", in, err)
		}
		if goFolded := FoldVietnamese(in); sqlFolded != goFolded {
			t.Errorf("ký tự %q: SQL %q, Go %q", string(r), sqlFolded, goFolded)
		}
	}
}
