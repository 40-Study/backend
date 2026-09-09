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
// Giới hạn đã biết (ghi rõ, không giấu):
//  1. Chỉ quét trong PHẠM VI 1 FILE — đúng như báo cáo review vòng 4 mô tả ("toàn bộ 469 route
//     trong internal/router/*.go"), không quét route đăng ký trong handler hay cách 2 file router
//     khác nhau cùng mount vào 1 prefix (I-B, review vòng 5 — CHƯA khắc phục, ghi nhận rủi ro
//     còn tồn đọng, không phải phạm vi I-A).
//  2. Chỉ nhận diện group/middleware-chain qua 2 cú pháp: `newVar := parentVar.Group("literal")`
//     VÀ `newVar := parentVar.Use(...)` (có hoặc không kèm path literal làm đối số đầu) — group/
//     chain tạo qua biểu thức phức tạp hơn nữa (vd nối chuỗi biến, `Group(prefix+suffix)`) vẫn
//     không resolve được prefix và bị coi là tiền tố rỗng, có thể lọt false negative.
//
// SỬA (I-A, review vòng 5→6): TRƯỚC ĐÂY comment ở đây khẳng định "grep thủ công xác nhận KHÔNG
// có file nào dùng cú pháp group phức tạp hơn `newVar := parentVar.Group("...")`" — KHẲNG ĐỊNH
// NÀY SAI: `gamification_router.go:22` dùng `auth := achievements.Use(middleware.AuthMiddleware(
// cfg, redis))` — Fiber's `Use()` trả về CHÍNH router đó (cùng tiền tố `/achievements`, KHÔNG
// thêm segment nào), nhưng regex CŨ chỉ khớp `.Group("...")` nên biến `auth` bị coi là tiền tố
// RỖNG — mọi route đăng ký qua `auth` (`/me`, `/:id/unlock`) MẤT tiền tố `/achievements`, không
// còn so sánh được với route khác cùng prefix -> bỏ lọt xung đột thật (đã tự chứng minh bằng
// MUT-18: chèn `achievements.Get("/:zzaid", ...)` TRƯỚC `auth.Get("/me", ...)` — Fiber thật sẽ
// nuốt mất `/me`, nhưng test cũ vẫn PASS). Đã thêm regex thứ 2 nhận `newVar := parentVar.Use(`
// (có/không kèm path literal) — `auth` giờ kế thừa ĐÚNG tiền tố từ `achievements`.
func TestRouterStaticBeforeParam_NoOrderingConflict(t *testing.T) {
	dir := "." // internal/router — test này nằm sẵn trong package đó

	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob router dir failed: %v", err)
	}
	sort.Strings(files)

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
		routes := extractRoutesFromSource(lines)
		totalRoutes += len(routes)

		for _, c := range findOrderingConflicts(routes) {
			conflicts = append(conflicts, filepath.Base(f)+": "+c)
		}
	}

	t.Logf("đã quét %d file router, %d route đăng ký", scannedFiles, totalRoutes)
	if len(conflicts) > 0 {
		t.Errorf("phát hiện %d xung đột thứ tự đăng ký route (param trước literal cùng tiền tố/độ sâu):\n%s",
			len(conflicts), strings.Join(conflicts, "\n"))
	}
}

type routeEntry struct {
	method string
	path   string
	line   int
}

// groupOrUseWithPathRe khớp `newVar := parentVar.Group("prefix")` VÀ `newVar := parentVar.Use(
// "prefix", ...)` — cả 2 dạng đều có literal path làm đối số ĐẦU TIÊN, cộng thẳng vào prefix của
// parentVar.
var groupOrUseWithPathRe = regexp.MustCompile(`(\w+)\s*:=\s*(\w+)\.(?:Group|Use)\(\s*"([^"]*)"`)

// useNoPathRe (I-A, review vòng 5→6) khớp `newVar := parentVar.Use(<handler, không phải path>)`
// — dạng gamification_router.go dùng thật: `auth := achievements.Use(middleware.AuthMiddleware(
// cfg, redis))`. Không có path literal nào được thêm — `newVar` kế thừa NGUYÊN VẸN prefix của
// `parentVar`. Chỉ khớp khi groupOrUseWithPathRe ở TRÊN không khớp trước (đối số đầu không phải
// chuỗi) — xem thứ tự if/else if trong extractRoutesFromSource.
var useNoPathRe = regexp.MustCompile(`(\w+)\s*:=\s*(\w+)\.Use\(`)

var routeRe = regexp.MustCompile(`(\w+)\.(Get|Post|Put|Delete|Patch)\(\s*"([^"]*)"`)

// extractRoutesFromSource dựng lại toàn bộ route (method + full path + số dòng) từ nội dung
// SOURCE TEXT của 1 file router — tách riêng khỏi TestRouterStaticBeforeParam_NoOrderingConflict
// để test riêng detector bằng fixture (không cần đọc file thật) — xem
// TestRouteConflictDetector_DetectsUseChainConflict.
func extractRoutesFromSource(lines []string) []routeEntry {
	groupPrefix := map[string]string{}
	var routes []routeEntry
	for i, line := range lines {
		// Bỏ qua dòng comment (`//...`) — quét TEXT thô nên comment chứa VÍ DỤ code (vd đoạn
		// giải thích C-02 ở contest_routes.go tự trích dẫn `contests.Get("/:slug", ...)` trong
		// lời giải thích) trước đây bị regex hiểu nhầm thành route thật, tạo false positive.
		// Không xử lý block comment `/* ... */` nhiều dòng — hiện không có file router nào dùng.
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		if m := groupOrUseWithPathRe.FindStringSubmatch(line); m != nil {
			newVar, parentVar, seg := m[1], m[2], m[3]
			groupPrefix[newVar] = groupPrefix[parentVar] + seg
		} else if m := useNoPathRe.FindStringSubmatch(line); m != nil {
			newVar, parentVar := m[1], m[2]
			groupPrefix[newVar] = groupPrefix[parentVar]
		}
		if m := routeRe.FindStringSubmatch(line); m != nil {
			varName, method, seg := m[1], m[2], m[3]
			full := groupPrefix[varName] + seg
			routes = append(routes, routeEntry{method: method, path: full, line: i + 1})
		}
	}
	return routes
}

// findOrderingConflicts so sánh mọi cặp route CÙNG method, CÙNG độ sâu segment trong danh sách
// routes (đã theo ĐÚNG thứ tự đăng ký thật) — trả về mô tả xung đột (không kèm tên file, caller
// tự ghép).
func findOrderingConflicts(routes []routeEntry) []string {
	var conflicts []string
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
				conflicts = append(conflicts, "["+routes[a].method+"] "+routes[a].path+
					" (dòng "+itoa(routes[a].line)+") đăng ký TRƯỚC ["+routes[b].method+"] "+routes[b].path+
					" (dòng "+itoa(routes[b].line)+") -> route tĩnh phía sau có thể bị route tham số phía trước nuốt")
			}
		}
	}
	return conflicts
}

// TestRouteConflictDetector_DetectsUseChainConflict (I-A, review vòng 5→6) — mutation test dùng
// FIXTURE (không đụng gamification_router.go thật, tránh phá route thật chỉ để test detector):
// mô phỏng ĐÚNG cú pháp `newVar := parentVar.Use(<handler>)` gamification_router.go dùng, cố tình
// chèn route tham số "/:zzaid" TRƯỚC route tĩnh "/me" (giống hệt MUT-18 review vòng 5 — Fiber
// thật sẽ nuốt mất "/me"). Nếu detector coi "auth" là tiền tố RỖNG (bug I-A cũ), 2 route full
// path sẽ là "/:zzaid" và "/me" — ĐỘ SÂU BẰNG NHAU (1 segment) NHƯNG không cùng nhánh cây thật
// (thiếu tiền tố "/achievements" chung) — vẫn vô tình bị so sánh (false-negative kiểu khác) hoặc
// bỏ sót tuỳ regex — điểm mấu chốt cần assert: PHẢI bắt được xung đột SAU khi có tiền tố đúng.
func TestRouteConflictDetector_DetectsUseChainConflict(t *testing.T) {
	fixture := []string{
		`achievements := api.Group("/achievements")`,
		`achievements.Get("/", h.List)`,
		`auth := achievements.Use(middleware.AuthMiddleware(cfg, redis))`,
		`auth.Get("/:zzaid", h.GetOne)`,
		`auth.Get("/me", h.GetMy)`,
	}
	routes := extractRoutesFromSource(fixture)
	if len(routes) != 3 {
		t.Fatalf("kỳ vọng dựng lại đúng 3 route từ fixture, nhận %d: %+v", len(routes), routes)
	}
	if routes[1].path != "/achievements/:zzaid" || routes[2].path != "/achievements/me" {
		t.Fatalf("kỳ vọng 'auth' kế thừa tiền tố '/achievements' qua Use() — nhận path[1]=%q path[2]=%q",
			routes[1].path, routes[2].path)
	}
	conflicts := findOrderingConflicts(routes)
	if len(conflicts) == 0 {
		t.Fatalf("detector không bắt được xung đột param-trước-literal qua chuỗi Use() (auth := achievements.Use(...)) — false negative I-A tái phát")
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
