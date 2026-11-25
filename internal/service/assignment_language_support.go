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
	_, ok := assignmentNonJudgeLanguages[normalizeSubmissionLanguage(language)]
	return ok
}

// codeLanguageJudgeable: true khi máy chấm chạy được ngôn ngữ lập trình này.
func codeLanguageJudgeable(language string) bool {
	_, ok := CodeLanguageID(language)
	return ok
}

// normalizeSubmissionLanguage: cách so khớp ngôn ngữ duy nhất (không phân biệt hoa thường, bỏ khoảng trắng), cùng chuẩn với CodeLanguageID.
func normalizeSubmissionLanguage(language string) string {
	return strings.ToLower(strings.TrimSpace(language))
}

// assignmentAllowsLanguage (QA6 review M3): ngôn ngữ nộp/chạy phải nằm trong danh sách của bài. Khớp theo chuẩn hoá, hoặc cùng
// một ngôn ngữ của máy chấm (cpp = c++, js = javascript). Danh sách rỗng giữ hành vi cũ (không giới hạn).
func assignmentAllowsLanguage(allowed []string, language string) bool {
	if len(allowed) == 0 {
		return true
	}
	want := normalizeSubmissionLanguage(language)
	wantID, wantJudgeable := CodeLanguageID(want)
	for _, a := range allowed {
		if normalizeSubmissionLanguage(a) == want {
			return true
		}
		if id, ok := CodeLanguageID(a); ok && wantJudgeable && id == wantID {
			return true
		}
	}
	return false
}

// errLanguageNotAllowed: 400 ERR_LANGUAGE_NOT_ALLOWED (nộp / chạy thử / chạy tuỳ ý ngôn ngữ ngoài danh sách của bài).
func errLanguageNotAllowed(language string) *ContestError {
	return cerr(400, "ERR_LANGUAGE_NOT_ALLOWED", fmt.Sprintf("Bài này không cho phép ngôn ngữ %s. Hãy chọn một ngôn ngữ trong danh sách của bài.", strings.TrimSpace(language)))
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
