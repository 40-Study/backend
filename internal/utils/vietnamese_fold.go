package utils

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
)

// Tìm kiếm không phân biệt dấu tiếng Việt (QA2 F-07: gõ "dai so" phải ra "Đại số").
//
// Vì sao không dùng extension `unaccent`: nó phải được CREATE EXTENSION trên từng DB (quyền, và nằm trong MỘT
// schema nên các test chạy trong schema tạm không thấy được), hàm unaccent() chỉ STABLE nên không đặt được vào
// chỉ mục biểu thức nếu không bọc thêm, và nó bỏ dấu của MỌI ngôn ngữ trong khi sản phẩm chỉ cần tiếng Việt.
// `translate()` + `lower()` có sẵn trong mọi Postgres, IMMUTABLE, không phụ thuộc schema hay quyền.
//
// Ánh xạ nằm ở MỘT bảng (vietnameseBase). Phía Go (FoldVietnamese, dùng cho từ khoá) và phía SQL
// (vietnameseFoldSQL, dùng cho cột) cùng sinh từ bảng này nên không thể lệch nhau; test pin điều đó trên Postgres.
// Chỉ nhận dạng dựng sẵn (NFC) cho cột; từ khoá được chuẩn hoá về NFC trước nên bàn phím gõ rời dấu vẫn khớp.

// vietnameseBase: chữ cái cơ sở -> mọi dạng có dấu viết thường (đ -> d tính như một dạng có dấu).
var vietnameseBase = []struct {
	base     rune
	variants string
}{
	{'a', "àáảãạăằắẳẵặâầấẩẫậ"},
	{'e', "èéẻẽẹêềếểễệ"},
	{'i', "ìíỉĩị"},
	{'o', "òóỏõọôồốổỗộơờớởỡợ"},
	{'u', "ùúủũụưừứửữự"},
	{'y', "ỳýỷỹỵ"},
	{'d', "đ"},
}

// vietnameseFoldFrom/To: hai chuỗi cùng độ dài rune cho translate(): chữ có dấu (cả hoa lẫn thường) -> chữ cơ sở thường.
var vietnameseFoldFrom, vietnameseFoldTo = buildVietnameseFold()

var vietnameseFoldMap = func() map[rune]rune {
	m := make(map[rune]rune)
	from, to := []rune(vietnameseFoldFrom), []rune(vietnameseFoldTo)
	for i := range from {
		m[from[i]] = to[i]
	}
	return m
}()

func buildVietnameseFold() (string, string) {
	var from, to []rune
	for _, g := range vietnameseBase {
		for _, r := range g.variants {
			from = append(from, r, unicode.ToUpper(r))
			to = append(to, g.base, g.base)
		}
	}
	return string(from), string(to)
}

// FoldVietnamese bỏ dấu tiếng Việt và hạ chữ thường: "Đại SỐ" -> "dai so". Cùng kết quả với biểu thức SQL
// lower(translate(cột, ...)) mà ApplyAccentInsensitiveKeywordSearch dùng cho cột.
func FoldVietnamese(s string) string {
	s = norm.NFC.String(s)
	return strings.ToLower(strings.Map(func(r rune) rune {
		if folded, ok := vietnameseFoldMap[r]; ok {
			return folded
		}
		return r
	}, s))
}

// vietnameseFoldSQL: biểu thức SQL cùng phép ánh xạ trên một cột. Chuỗi hằng lấy từ bảng ở trên, không có dữ liệu
// người dùng, nên nối thẳng vào câu lệnh là an toàn. translate() trước (nó tự xử lý chữ hoa có dấu mà không phụ
// thuộc locale của DB), lower() sau cho phần ASCII còn lại.
func vietnameseFoldSQL(column string) string {
	return "lower(translate(" + column + ", '" + vietnameseFoldFrom + "', '" + vietnameseFoldTo + "'))"
}

// ApplyAccentInsensitiveKeywordSearch giống ApplyKeywordSearch (LIKE "chứa chuỗi", keyword khớp literal) nhưng bỏ
// qua dấu tiếng Việt và hoa/thường ở cả hai phía. columns là hằng số của nơi gọi, không bao giờ là input người dùng.
// Không dùng được chỉ mục B-tree (pattern bắt đầu bằng %), giống ILIKE trước đây; muốn chỉ mục thì thêm
// pg_trgm GIN trên chính biểu thức này.
func ApplyAccentInsensitiveKeywordSearch(query *gorm.DB, keyword string, columns ...string) *gorm.DB {
	if keyword == "" || len(columns) == 0 {
		return query
	}

	pattern := ContainsLikePattern(FoldVietnamese(keyword))
	conditions := make([]string, len(columns))
	args := make([]interface{}, len(columns))
	for i, col := range columns {
		conditions[i] = vietnameseFoldSQL(col) + " LIKE ?"
		args[i] = pattern
	}
	return query.Where(strings.Join(conditions, " OR "), args...)
}
