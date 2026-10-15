package service

import (
	"fmt"
	"strings"

	"study.com/v1/internal/dto"
)

// RunnableLanguage: một ngôn ngữ chạy được trên code editor (sandbox, bài tập khoá học). SSOT của contract sec. 3.
type RunnableLanguage struct {
	Key      string
	Label    string
	Judge0ID int
}

// runnableLanguages: thứ tự là thứ tự hiển thị (E2). HTML KHÔNG chạy được (không có Judge0 id). Id 50/54/62/63/71
// có trong Judge0 CE v1.13.1 db/languages/active.rb; 82 (SQLite) là id sandbox đang dùng, kiểm lại live ở Phase 7.
var runnableLanguages = []RunnableLanguage{
	{Key: "python", Label: "Python 3", Judge0ID: 71},
	{Key: "javascript", Label: "JavaScript (Node.js)", Judge0ID: 63},
	{Key: "cpp", Label: "C++ (GCC)", Judge0ID: 54},
	{Key: "c", Label: "C (GCC)", Judge0ID: 50},
	{Key: "java", Label: "Java", Judge0ID: 62},
	{Key: "sql", Label: "SQL (SQLite)", Judge0ID: 82},
}

// runnableLanguageAliases: khoá cũ/viết tắt mà client đang gửi -> khoá chuẩn.
var runnableLanguageAliases = map[string]string{
	"python3": "python",
	"js":      "javascript",
	"c++":     "cpp",
}

// LookupLanguage chuẩn hoá khoá (trim, chữ thường, alias) và trả ngôn ngữ chạy được; khoá lạ là
// ErrUnsupportedLanguage, KHÔNG bao giờ mặc định về Python.
func LookupLanguage(key string) (RunnableLanguage, error) {
	k := strings.ToLower(strings.TrimSpace(key))
	if canonical, ok := runnableLanguageAliases[k]; ok {
		k = canonical
	}
	for _, l := range runnableLanguages {
		if l.Key == k {
			return l, nil
		}
	}
	return RunnableLanguage{}, fmt.Errorf("%w: %q", ErrUnsupportedLanguage, key)
}

// RunnableLanguages trả danh sách cho GET /submissions/languages (E2), theo thứ tự hiển thị.
func RunnableLanguages() []dto.RunnableLanguageDTO {
	out := make([]dto.RunnableLanguageDTO, 0, len(runnableLanguages))
	for _, l := range runnableLanguages {
		out = append(out, dto.RunnableLanguageDTO{Key: l.Key, Label: l.Label})
	}
	return out
}

// legacyLiveAssignmentLanguageIDs: khoá mà endpoint bài tập LIVE (submit/run/run-custom) vẫn nhận, kể cả ngoài bảng
// chạy được (typescript, go, rust...). Chỉ dùng cho bài live; BE-A chuyển getLanguageID sang map này.
var legacyLiveAssignmentLanguageIDs = map[string]int{
	"c":          50,
	"c++":        54,
	"cpp":        54,
	"java":       62,
	"python":     71,
	"python3":    71,
	"javascript": 63,
	"js":         63,
	"typescript": 74,
	"ts":         74,
	"go":         60,
	"rust":       73,
	"ruby":       72,
	"php":        68,
	"c#":         51,
	"cs":         51,
	"swift":      83,
	"kotlin":     78,
}

// LegacyLiveAssignmentLanguageID: Judge0 id của một khoá bài live; ok=false khi khoá lạ (người gọi tự quyết, không
// có mặc định ẩn).
func LegacyLiveAssignmentLanguageID(key string) (int, bool) {
	id, ok := legacyLiveAssignmentLanguageIDs[key]
	return id, ok
}
