package seeds

// Guard vĩnh viễn (Living Discipline): mô tả quyền/role trong seed JSON hiện cho admin ở trang
// /admin/permissions, nên không được mang chú thích của người phát triển như "(A-P0-1, QA 260927)",
// "(Phase 4, 28/09/2026)". Lịch sử thay đổi nằm trong git, không nằm trong mô tả.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// QA6-A09: mô tả quyền hiện cho admin bằng tiếng Việt thống nhất với app: "xoá/khoá" (không "xóa/khóa"), "giảng viên/học viên"
// (không "giáo viên/học sinh") và không còn từ tiếng Anh lẫn vào câu.
var qa6BadCopy = regexp.MustCompile(`(?i)\bxóa\b|\bkhóa\b|\bgiáo viên\b|\bhọc sinh\b|\b(report|dashboard|user|traffic|revenue|role|permission|pending|approved|rejected|ban|organization|admin|teacher|gv)\b|\bTự túc\b`)

func TestQA6_PermissionDescriptionCopyIsVietnameseAndConsistent(t *testing.T) {
	if m := qa6BadCopy.FindString("Khóa (Ban) tài khoản user, xem report ở Dashboard"); m == "" {
		t.Fatal("guard không bắt được chuỗi có lỗi đã biết")
	}
	if m := qa6BadCopy.FindString("Khoá hoặc mở khoá tài khoản người dùng, xem báo cáo của giảng viên"); m != "" {
		t.Fatalf("guard bắt nhầm mô tả sạch: %q", m)
	}
	files, err := filepath.Glob(filepath.Join(s2DataDir, "permissions", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("không tìm thấy data/permissions/*.json (err=%v)", err)
	}
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
			if m := qa6BadCopy.FindString(it.Description); m != "" {
				t.Errorf("%s: %s dùng %q trong mô tả %q: viết lại bằng tiếng Việt thống nhất (xoá/khoá, giảng viên/học viên)", filepath.Base(f), it.Name, m, it.Description)
			}
		}
	}
}

// SYSTEM_SETTINGS_MANAGE không được hứa cấu hình mà /admin/settings chưa có (cổng thanh toán, email server, lưu trữ).
func TestQA6_SystemSettingsDescriptionIsHonest(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(s2DataDir, "permissions", "system_admin_permissions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var items []descriptionOnly
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Name != "SYSTEM_SETTINGS_MANAGE" {
			continue
		}
		for _, promised := range []string{"Cổng thanh toán", "Email server", "Storage", "lưu trữ"} {
			if strings.Contains(it.Description, promised) {
				t.Errorf("SYSTEM_SETTINGS_MANAGE hứa %q mà trang cài đặt chưa có: %q", promised, it.Description)
			}
		}
		return
	}
	t.Fatal("không thấy SYSTEM_SETTINGS_MANAGE trong seed")
}
