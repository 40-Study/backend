package service

import (
	"math"
	"sort"

	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

// MergePlayedRanges — Phase 1 §1 (chống tua).
//
// Trình phát gửi lên các khoảng ĐÃ PHÁT THẬT kể từ lần gửi trước; đoạn bị tua qua không bao
// giờ nằm trong khoảng nào. Hàm này hợp nhất chúng với các khoảng đã lưu trong
// lesson_progress.played_ranges và trả về tổng số giây đã xem.
//
// Vì sao KHÔNG cộng dồn watched_seconds trực tiếp từ client: một trường "đã xem bao nhiêu giây"
// do client tự khai thì kéo thanh tua tới cuối video là có ngay 100%. Ở đây client chỉ khai
// "tôi vừa phát đoạn [a,b]" — và chỉ những khoảng thoả ràng buộc mới được tính.
//
// Ràng buộc (theo contract §1): 0 <= start < end <= durationSeconds. Khoảng vi phạm bị BỎ QUA
// (không làm hỏng cả request) — một client lỗi chỉ mất phần dữ liệu của chính khoảng đó.
// durationSeconds <= 0 (chưa biết thời lượng) thì không kiểm được cận trên, chỉ kiểm start >= 0
// và end > start.
//
// Trả về: danh sách khoảng sau merge (đã sắp xếp, không chồng lấn) và tổng số giây (làm tròn).
func MergePlayedRanges(stored model.PlayedRanges, incoming []model.PlayedRange, durationSeconds int) (model.PlayedRanges, int) {
	cleaned := make(model.PlayedRanges, 0, len(stored)+len(incoming))
	for _, r := range stored {
		if normalized, ok := normalizePlayedRange(r, durationSeconds); ok {
			cleaned = append(cleaned, normalized)
		}
	}
	for _, r := range incoming {
		if normalized, ok := normalizePlayedRange(r, durationSeconds); ok {
			cleaned = append(cleaned, normalized)
		}
	}

	if len(cleaned) == 0 {
		return model.PlayedRanges{}, 0
	}

	sort.Slice(cleaned, func(i, j int) bool {
		if cleaned[i].Start != cleaned[j].Start {
			return cleaned[i].Start < cleaned[j].Start
		}
		return cleaned[i].End < cleaned[j].End
	})

	merged := make(model.PlayedRanges, 0, len(cleaned))
	merged = append(merged, cleaned[0])
	for _, r := range cleaned[1:] {
		last := &merged[len(merged)-1]
		// Chồng lấn HOẶC chạm nhau (End == Start) đều gộp: hai khoảng kề nhau [0,10] và [10,20]
		// là một đoạn phát liền mạch, tách ra chỉ tạo thêm phần tử mà tổng độ dài không đổi.
		if r.Start <= last.End {
			if r.End > last.End {
				last.End = r.End
			}
			continue
		}
		merged = append(merged, r)
	}

	return merged, totalPlayedSeconds(merged)
}

// normalizePlayedRange kiểm tra một khoảng theo contract §1 rồi làm tròn về giây.
//
// Làm tròn SAU khi kiểm: web đã làm tròn trước khi gửi (lib/played-ranges.ts), nhưng một client
// khác gửi số thực (754.4) vẫn phải được xử lý nhất quán — kiểm trên giá trị thô trước để không
// biến một khoảng hợp lệ thành khoảng vượt thời lượng chỉ vì làm tròn lên.
func normalizePlayedRange(r model.PlayedRange, durationSeconds int) (model.PlayedRange, bool) {
	if math.IsNaN(r.Start) || math.IsNaN(r.End) || math.IsInf(r.Start, 0) || math.IsInf(r.End, 0) {
		return model.PlayedRange{}, false
	}
	if r.Start < 0 || r.End <= r.Start {
		return model.PlayedRange{}, false
	}
	if durationSeconds > 0 && r.End > float64(durationSeconds) {
		return model.PlayedRange{}, false
	}

	start := math.Round(r.Start)
	end := math.Round(r.End)
	if durationSeconds > 0 && end > float64(durationSeconds) {
		end = float64(durationSeconds)
	}
	if end <= start {
		return model.PlayedRange{}, false
	}
	return model.PlayedRange{Start: start, End: end}, true
}

func totalPlayedSeconds(ranges model.PlayedRanges) int {
	total := 0.0
	for _, r := range ranges {
		total += r.End - r.Start
	}
	return int(math.Round(total))
}

// WatchedPercent — watched_pct theo contract §1: round(watched_seconds / duration * 100, 1).
//
// durationSeconds <= 0 (chưa biết thời lượng) trả về (zero, false) để caller GIỮ NGUYÊN giá trị
// đang lưu thay vì ghi 0 — ghi 0 sẽ hạ watched_pct của một bài đã xem xong xuống và có thể kéo
// người học ra khỏi trạng thái completed ở tầng hiển thị.
func WatchedPercent(watchedSeconds, durationSeconds int) (decimal.Decimal, bool) {
	if durationSeconds <= 0 {
		return decimal.Zero, false
	}
	pct := decimal.NewFromInt(int64(watchedSeconds)).
		Mul(decimal.NewFromInt(100)).
		Div(decimal.NewFromInt(int64(durationSeconds)))
	return pct.Round(1), true
}
