package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// PlayedRangeDTO la MOT khoang [start, end] giay da PHAT THAT (Phase 1 §1, chong tua).
//
// Bieu dien JSON CHINH THUC theo contract la mot CAP SO, khong phai object:
//
//	"played_ranges": [[0, 120], [118, 754]]
//
// Day khong phai chi tiet trinh bay: web khai `type PlayedRange = [number, number]`
// (web/src/lib/played-ranges.ts) va `buildHeartbeatPayload` sinh ra dung mang cap so, roi
// `JSON.stringify` nguyen payload di ca cho `PUT /lessons/:lessonId/progress` lan cho
// `navigator.sendBeacon("/api/progress", ...)`. Doc sai dang nay thi MOI heartbeat deu 400 va
// tien do hoc cua nguoi hoc dung im — trong khi ca hai phia van "build xanh".
//
// Vi vay UnmarshalJSON o day doc dang cap so truoc; dang object {"start":..,"end":..} duoc
// chap nhan them nhu mot ban mo rong khoan dung (mot client khac, hoac du lieu cu), khong phai
// duong chinh. MarshalJSON luon ghi ra dang cap so de chi co MOT bieu dien.
type PlayedRangeDTO struct {
	Start float64
	End   float64
}

// UnmarshalJSON doc [start, end] (chinh thuc) hoac {"start":..,"end":..} (khoan dung).
func (p *PlayedRangeDTO) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)

	// `null` PHAI la loi o day, khong duoc coi la "khong doi gi": encoding/json khong bao loi
	// khi unmarshal `null` vao mot gia tri struct (khong phai con tro/slice/map/interface) —
	// no chi giu nguyen gia tri zero va tra ve nil. Neu khong chan som, mot phan tu `null` trong
	// mang played_ranges se lot qua thanh khoang {0,0} gia, thay vi bi PlayedRangesDTO bo qua
	// nhu cac phan tu hong khac.
	if string(trimmed) == "null" {
		return fmt.Errorf("played_ranges: khoang khong duoc la null")
	}

	if len(trimmed) > 0 && trimmed[0] == '[' {
		var pair []float64
		if err := json.Unmarshal(trimmed, &pair); err != nil {
			return fmt.Errorf("played_ranges: khoang phai la [start, end]: %w", err)
		}
		if len(pair) != 2 {
			return fmt.Errorf("played_ranges: khoang phai co dung 2 phan tu, nhan %d", len(pair))
		}
		p.Start, p.End = pair[0], pair[1]
		return nil
	}

	// `type alias` tao kieu MOI cung cau truc nhung KHONG kem method, nen lenh Unmarshal duoi
	// day khong tu goi lai chinh ham nay (khong de quy vo han).
	type alias PlayedRangeDTO
	var obj alias
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return fmt.Errorf("played_ranges: khoang khong doc duoc: %w", err)
	}
	*p = PlayedRangeDTO(obj)
	return nil
}

// MarshalJSON ghi ra dang cap so — cung dang voi contract va voi web.
func (p PlayedRangeDTO) MarshalJSON() ([]byte, error) {
	return json.Marshal([]float64{p.Start, p.End})
}

// PlayedRangesDTO la danh sach khoang da phat.
//
// Kieu rieng (khong dung thang []PlayedRangeDTO) de co the BO QUA tung khoang hong ma khong
// lam hong ca request — dung cau chu cua contract §1: "bo khoang khong hop le (khong loi ca
// request)". Mot phan tu sai dinh dang (`[1]`, `"abc"`, `null`) bi bo qua thay vi keo ca
// heartbeat ve 400: day la bang chung chong tua, mat mot khoang chi thieu mot doan nho, con mat
// ca request la mat luon ca lan heartbeat do.
type PlayedRangesDTO []PlayedRangeDTO

func (p *PlayedRangesDTO) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	// `null` hoac bo trong => khong co khoang nao, KHONG phai loi (truong la optional).
	if len(trimmed) == 0 || string(trimmed) == "null" {
		*p = nil
		return nil
	}

	// Doc tho tung phan tu: loi cu phap JSON o tang nay van la loi that (body hong), nhung loi
	// o TUNG khoang thi xu ly rieng ben duoi.
	var raw []json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return fmt.Errorf("played_ranges: phai la mang cac khoang [start, end]: %w", err)
	}

	out := make(PlayedRangesDTO, 0, len(raw))
	for _, item := range raw {
		var one PlayedRangeDTO
		if err := json.Unmarshal(item, &one); err != nil {
			continue
		}
		out = append(out, one)
	}
	*p = out
	return nil
}
