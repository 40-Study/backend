package config

// Kiểm .env.example (ở gốc repo backend) đủ dùng trên máy mới:
//   - mọi biến docker-compose.yaml bắt buộc bằng ${VAR:?...} phải có mặt và KHÁC RỖNG trong .env.example,
//     nếu không `cp .env.example .env && docker compose up` dừng ngay với "set VAR in .env";
//   - PORT mặc định trong code (khi không đặt PORT) phải trùng cổng dev ghi trong .env.example, để
//     `go run ./cmd/api` không có PORT vẫn nghe đúng cổng mà web (NEXT_PUBLIC_API_URL) gọi tới.

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// backendRoot trả thư mục gốc repo backend (internal/config -> ../..).
func backendRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("không xác định được đường dẫn file test")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// readEnvExample đọc .env.example thành map KEY -> giá trị (bỏ dòng trống và chú thích).
func readEnvExample(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join(backendRoot(t), ".env.example"))
	if err != nil {
		t.Fatalf("mở .env.example: %v", err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("đọc .env.example: %v", err)
	}
	return out
}

// composeRequiredVarRE bắt ${VAR:?...} và ${VAR?...}: compose báo lỗi khi VAR thiếu (dạng ":?" cả khi rỗng).
var composeRequiredVarRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*):?\?[^}]*\}`)

func TestEnvExample_HasEveryComposeRequiredVar(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(backendRoot(t), "docker-compose.yaml"))
	if err != nil {
		t.Fatalf("đọc docker-compose.yaml: %v", err)
	}
	required := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue // dịch vụ đã comment không được compose đọc
		}
		for _, m := range composeRequiredVarRE.FindAllStringSubmatch(line, -1) {
			required[m[1]] = true
		}
	}
	if len(required) == 0 {
		t.Fatal("không tìm thấy biến ${VAR:?...} nào trong docker-compose.yaml: regex hoặc file đã đổi")
	}

	env := readEnvExample(t)
	var missing []string
	for name := range required {
		if strings.TrimSpace(env[name]) == "" {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf(".env.example thiếu hoặc để rỗng %d biến docker-compose bắt buộc: %v", len(missing), missing)
	}
}

func TestDefaultPort_MatchesEnvExampleDevPort(t *testing.T) {
	if got := readEnvExample(t)["PORT"]; got != DefaultPort {
		t.Fatalf("PORT trong .env.example = %q, PORT mặc định trong code = %q: phải trùng nhau", got, DefaultPort)
	}
}
