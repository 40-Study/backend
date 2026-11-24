package service

import (
	"fmt"
	"strings"
)

// QA6 S6-01 (C11): ngôn ngữ máy chấm không chạy được (vd Dart) KHÔNG bao giờ được chạy ngầm như Python.
// SSOT là CodeLanguageID (cùng bảng với cuộc thi và sandbox). "json" (trắc nghiệm) và "text" (tự luận) không đi qua máy chấm
// nên không thuộc diện này.

// assignmentNonJudgeLanguages: loại bài nộp có đường xử lý riêng, không phải ngôn ngữ lập trình.
var assignmentNonJudgeLanguages = map[string]struct{}{"json": {}, "text": {}}

func isNonJudgeSubmissionLanguage(language string) bool {
	_, ok := assignmentNonJudgeLanguages[strings.ToLower(strings.TrimSpace(language))]
	return ok
}

// codeLanguageJudgeable: true khi máy chấm chạy được ngôn ngữ lập trình này.
func codeLanguageJudgeable(language string) bool {
	_, ok := CodeLanguageID(language)
	return ok
}

// errLanguageUnsupported: 422 ERR_LANGUAGE_UNSUPPORTED (chạy thử). Handler trả qua respondContestError.
func errLanguageUnsupported(language string) *ContestError {
	return cerr(422, "ERR_LANGUAGE_UNSUPPORTED", fmt.Sprintf("Ngôn ngữ %s chưa hỗ trợ chấm tự động. Bài này được giảng viên chấm thủ công.", strings.TrimSpace(language)))
}

// assignmentAutoGrade: false khi bài chỉ cho các ngôn ngữ lập trình mà máy chấm không chạy được (chấm thủ công). True khi có ít
// nhất một ngôn ngữ chạy được, hoặc bài trắc nghiệm/tự luận, hoặc danh sách rỗng (giữ nguyên hành vi cũ).
func assignmentAutoGrade(languages []string) bool {
	if len(languages) == 0 {
		return true
	}
	for _, l := range languages {
		if isNonJudgeSubmissionLanguage(l) || codeLanguageJudgeable(l) {
			return true
		}
	}
	return false
}
