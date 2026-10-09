package seeds

// Guard vĩnh viễn (Living Discipline): mô tả quyền/role trong seed JSON hiện cho admin ở trang
// /admin/permissions, nên không được mang chú thích của người phát triển như "(A-P0-1, QA 260927)",
// "(Phase 4, 28/09/2026)". Lịch sử thay đổi nằm trong git, không nằm trong mô tả.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// tagPattern gom mọi dạng chú thích dev đã gặp: mã QA, số phase, mã lỗi kiểu A-P0-1, ngày dd/mm/yyyy
// và số PR "(#123)".
var tagPattern = regexp.MustCompile(`QA \d{6}|Phase \d|\b[A-Z]-P\d-\d\b|\d{2}/\d{2}/\d{4}|\(#\d+\)`)

// descriptionOnly chỉ lấy khoá "description" của cả quyền lẫn role (roles.json cũng có khoá này).
type descriptionOnly struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Description string `json:"description"`
}

func TestPermissionDescriptionCleanPatternsCatchKnownTags(t *testing.T) {
	for _, bad := range []string{
		"Kiểm duyệt report (A-P0-1, QA 260927)",
		"Xử lý yêu cầu rút tiền (Phase 4, 28/09/2026)",
		"Xem đơn hàng (tính năng đơn hàng+hoàn tiền+doanh thu, 27/09/2026)",
		"Sửa lỗi (#81)",
	} {
		if !tagPattern.MatchString(bad) {
			t.Errorf("guard bỏ sót chú thích dev: %q", bad)
		}
	}
	for _, good := range []string{
		"Kiểm duyệt report vi phạm: xem toàn bộ danh sách, đổi trạng thái, xoá report",
		"Xem và xử lý yêu cầu rút tiền của giảng viên",
	} {
		if tagPattern.MatchString(good) {
			t.Errorf("guard bắt nhầm mô tả sạch: %q", good)
		}
	}
}

func TestPermissionDescriptionCleanNoDevAnnotationsInSeedJSON(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(s2DataDir, "permissions", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("không tìm thấy data/permissions/*.json (err=%v)", err)
	}
	files = append(files, filepath.Join(s2DataDir, "roles.json"))

	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("đọc %s: %v", f, err)
		}
		var items []descriptionOnly
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, it := range items {
			if m := tagPattern.FindString(it.Description); m != "" {
				id := it.Name
				if id == "" {
					id = it.Role
				}
				t.Errorf("%s: %s có chú thích dev %q trong mô tả %q — hãy bỏ nó; lịch sử thay đổi để trong git, không để trong mô tả hiện cho admin",
					filepath.Base(f), id, m, it.Description)
			}
		}
	}
}
