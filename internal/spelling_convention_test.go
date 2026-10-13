package internal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Quy ước chính tả của dự án: dấu thanh kiểu CŨ (khoá, xoá, huỷ, tuỳ, hoá, thoả, thuỷ, hoà), không dùng kiểu mới
// (khóa, xóa, hủy, tùy, hóa, thỏa, thủy, hòa). Test này chỉ quét chuỗi literal (message trả cho người dùng, tiêu đề/nội
// dung thông báo, dữ liệu seed...), KHÔNG quét comment nên không buộc phải sửa comment.
var newToneSpelling = regexp.MustCompile(`(?i)khóa|xóa|hủy|tùy|hóa|thỏa|thủy|hòa`)

func TestUserFacingStringLiteralsUseOldToneSpelling(t *testing.T) {
	roots := []string{".", filepath.Join("..", "cmd")}
	fset := token.NewFileSet()
	checked := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, perr := parser.ParseFile(fset, path, nil, 0) // mode 0: bỏ comment
			if perr != nil {
				return perr
			}
			checked++
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				val, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					val = lit.Value
				}
				if m := newToneSpelling.FindString(val); m != "" {
					t.Errorf("%s: chuỗi literal dùng dấu thanh kiểu mới %q, đổi sang kiểu cũ (khoá, xoá, huỷ, tuỳ, hoá, thoả, thuỷ, hoà)", fset.Position(lit.Pos()), m)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if checked == 0 {
		t.Fatal("không quét được file .go nào: đường dẫn sai, test không chứng minh gì")
	}
}
