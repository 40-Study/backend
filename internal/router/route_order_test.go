package router

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// route_order_test.go (I-04, review vòng 4/5) — quét TĨNH toàn bộ internal/router/*.go để bắt
// đúng lớp lỗi đã gây ra M3-01 (quiz_router.go PUT /:id nuốt PUT /reorder) và I-04
// (contest_routes.go GET /:slug nuốt GET /me): Fiber khớp path theo THỨ TỰ ĐĂNG KÝ khi một
// route THAM SỐ (":xxx") và một route TĨNH (literal) cùng chia sẻ tiền tố VÀ cùng độ sâu segment
// — route đăng ký TRƯỚC thắng, nên nếu route tham số đăng ký trước route tĩnh, route tĩnh trở
// thành code chết vĩnh viễn (không handler nào trả về lỗi biên dịch/runtime, chỉ âm thầm sai).
//
// Cách quét: đọc SOURCE TEXT (không cần chạy app thật/không cần import cycle) của từng file
// router, regex bắt (1) khai báo group `newVar := parentVar.Group("prefix")` để dựng lại tiền
// tố ĐẦY ĐỦ của từng biến group (đệ quy qua parentVar, biến gốc không resolve được — ví dụ tham
// số hàm `api fiber.Router` — coi là tiền tố rỗng, ĐÚNG vì mọi route trong 1 file router đều
// tương đối so với đúng 1 gốc `api` được truyền vào), và (2) mọi lời gọi
// `var.Get/Post/Put/Delete/Patch("path", ...)` để ghép prefix + path thành full path, giữ
// nguyên THỨ TỰ xuất hiện trong file (khớp thứ tự đăng ký thật lúc chạy).
//
// Giới hạn đã biết (ghi rõ, không giấu): (1) chỉ quét trong PHẠM VI 1 FILE — đúng như báo cáo
// review vòng 4 mô tả ("toàn bộ 469 route trong internal/router/*.go"), không quét route đăng ký
// trong handler hay cách 2 file router khác nhau cùng mount vào 1 prefix; (2) chỉ nhận diện
// group qua cú pháp `newVar := parentVar.Group("literal")` — group được tạo qua biểu thức phức
// tạp hơn (vd nối chuỗi biến) sẽ không resolve được prefix và bị coi là tiền tố rỗng, có thể lọt
// false negative (không phải false positive) nếu file nào dùng cú pháp lạ — grep thủ công xác
// nhận KHÔNG có file nào trong 43 file router hiện tại dùng cú pháp group phức tạp hơn mẫu trên.
func TestRouterStaticBeforeParam_NoOrderingConflict(t *testing.T) {
	dir := "." // internal/router — test này nằm sẵn trong package đó

	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob router dir failed: %v", err)
	}
	sort.Strings(files)

	groupRe := regexp.MustCompile(`(\w+)\s*:=\s*(\w+)\.Group\("([^"]*)"`)
	routeRe := regexp.MustCompile(`(\w+)\.(Get|Post|Put|Delete|Patch)\(\s*"([^"]*)"`)

	type routeEntry struct {
		method string
		path   string
		line   int
	}

	var conflicts []string
	scannedFiles := 0
	totalRoutes := 0

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s failed: %v", f, err)
		}
		scannedFiles++

		lines := strings.Split(string(data), "\n")
		groupPrefix := map[string]string{}
		var routes []routeEntry
		for i, line := range lines {
			if m := groupRe.FindStringSubmatch(line); m != nil {
				newVar, parentVar, seg := m[1], m[2], m[3]
				groupPrefix[newVar] = groupPrefix[parentVar] + seg
			}
			if m := routeRe.FindStringSubmatch(line); m != nil {
				varName, method, seg := m[1], m[2], m[3]
				full := groupPrefix[varName] + seg
				routes = append(routes, routeEntry{method: method, path: full, line: i + 1})
				totalRoutes++
			}
		}

		for a := 0; a < len(routes); a++ {
			for b := a + 1; b < len(routes); b++ {
				if routes[a].method != routes[b].method {
					continue
				}
				segA := strings.Split(strings.Trim(routes[a].path, "/"), "/")
				segB := strings.Split(strings.Trim(routes[b].path, "/"), "/")
				if len(segA) != len(segB) {
					continue
				}
				// prefixMatch chỉ còn true nếu MỌI segment TRƯỚC vị trí xung đột là literal VÀ
				// GIỐNG HỆT nhau ở CẢ HAI route (nghĩa là 2 route thật sự cùng 1 nhánh cây route
				// tính tới thời điểm đó) — nếu một bên là param còn bên kia là literal ở bất kỳ
				// vị trí NÀO trước đó (theo CẢ HAI chiều, không chỉ chiều "A=param, B=literal"),
				// 2 route đã rẽ sang 2 nhánh cây KHÁC NHAU từ vị trí đó, nên mọi so sánh ở các
				// segment SÂU HƠN không còn ý nghĩa (không phải cùng node cha) — phải DỪNG so
				// sánh ngay, không được coi là "khớp tạm" rồi đi tiếp. Bug đã tự phát hiện khi
				// chạy thử: thiếu nhánh "A=literal, B=param" khiến course_router.go/
				// voucher_router.go bị báo xung đột GIẢ ở segment SÂU HƠN vị trí rẽ nhánh thật.
				prefixMatch := true
				conflictPos := -1
				for k := 0; k < len(segA); k++ {
					aIsParam := strings.HasPrefix(segA[k], ":")
					bIsParam := strings.HasPrefix(segB[k], ":")
					switch {
					case aIsParam && bIsParam:
						// cùng là tham số ở vị trí này -> coi như cùng nhánh, đi tiếp.
					case !aIsParam && !bIsParam && segA[k] == segB[k]:
						// cùng literal, khớp giá trị -> cùng nhánh, đi tiếp.
					case aIsParam && !bIsParam:
						// A (đăng ký TRƯỚC) là tham số, B (đăng ký SAU) là literal, RẼ NHÁNH tại
						// đây -> đúng lớp lỗi quiz_router/contest_routes: A có thể nuốt B.
						conflictPos = k
					default:
						// mọi trường hợp còn lại (A literal/B param theo chiều ngược lại, hoặc
						// 2 literal khác giá trị) -> rẽ sang 2 nhánh cây khác nhau, KHÔNG phải
						// xung đột thứ tự đăng ký -> dừng so sánh cặp route này, không kết luận.
						prefixMatch = false
					}
					if conflictPos >= 0 || !prefixMatch {
						break
					}
				}
				if prefixMatch && conflictPos >= 0 {
					conflicts = append(conflicts, filepath.Base(f)+": ["+routes[a].method+"] "+routes[a].path+
						" (dòng "+itoa(routes[a].line)+") đăng ký TRƯỚC ["+routes[b].method+"] "+routes[b].path+
						" (dòng "+itoa(routes[b].line)+") -> route tĩnh phía sau có thể bị route tham số phía trước nuốt")
				}
			}
		}
	}

	t.Logf("đã quét %d file router, %d route đăng ký", scannedFiles, totalRoutes)
	if len(conflicts) > 0 {
		t.Errorf("phát hiện %d xung đột thứ tự đăng ký route (param trước literal cùng tiền tố/độ sâu):\n%s",
			len(conflicts), strings.Join(conflicts, "\n"))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
