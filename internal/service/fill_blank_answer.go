package service

// fill_blank_answer.go — so khớp câu điền khuyết (fill_blank) cho CẢ quiz thường lẫn cuộc thi (cùng
// đi qua QuizService.checkAnswer). Quy tắc chủ dự án chốt 29/09 (contract "Cuộc thi" ĐÍNH CHÍNH 3):
// bỏ khoảng trắng đầu/cuối, gộp khoảng trắng liên tiếp, không phân biệt hoa thường, chuẩn hoá Unicode
// NFC; dấu tiếng Việt GIỮ NGUYÊN ("ha noi" khác "Hà Nội").

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// normalizeFillBlankAnswer đưa một đáp án điền về dạng so sánh được. NFC chạy CUỐI để chữ gõ bằng dấu
// tổ hợp (NFD, hay gặp khi dán từ macOS) trùng với chữ dựng sẵn — ToLower chỉ đổi chữ cái gốc, không
// đụng dấu tổ hợp, nên chạy NFC sau nó là đủ.
func normalizeFillBlankAnswer(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return norm.NFC.String(strings.ToLower(s))
}

// fillBlankMatches: text của học viên khớp một đáp án is_correct sau chuẩn hoá. Chuỗi rỗng sau chuẩn
// hoá không bao giờ khớp (tránh câu có đáp án chỉ gồm khoảng trắng chấm đúng bài bỏ trống).
func fillBlankMatches(accepted []string, text string) bool {
	got := normalizeFillBlankAnswer(text)
	if got == "" {
		return false
	}
	for _, a := range accepted {
		if normalizeFillBlankAnswer(a) == got {
			return true
		}
	}
	return false
}
