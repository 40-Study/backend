package service

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// Test cho Phan 1 §1 (tien do chong tua) — internal/service/played_ranges.go va cac helper
// quyet dinh status trong enrollment_service.go.
//
// Day la phan DUY NHAT cua Phase 1 ma mot client doc hai co the lach: neu phep merge sai, nguoi
// hoc chi can keo thanh tua toi cuoi video la xong bai ma khong xem gi. Vi vay cac test duoi day
// khong chi kiem "merge dung" ma con kiem dung nhung duong LACH: tua qua (khong tinh), xem lai
// (khong cong don), khoang vuot thoi luong, va client tu gui status=completed.

// sameRanges so sanh hai danh sach khoang theo gia tri (khong phan biet nil vs rong: ca hai deu
// nghia la "chua phat duoc giay nao").
func sameRanges(a, b model.PlayedRanges) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Start != b[i].Start || a[i].End != b[i].End {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// 1. Merge khoang da phat
// ---------------------------------------------------------------------------

// TestMergePlayedRanges_TuaQuaKhongDuocTinh: nguoi hoc mo bai, keo thanh tua tu 0 den 754 (het
// video) roi moi phat. Neu client chi gui khoang [720,754] thi chi 34 giay duoc tinh — day chinh
// la khac biet giua "vi tri dang phat" va "da phat that".
func TestMergePlayedRanges_TuaQuaKhongDuocTinh(t *testing.T) {
	merged, seconds := MergePlayedRanges(nil, []model.PlayedRange{{Start: 720, End: 754}}, 754)

	if !sameRanges(merged, model.PlayedRanges{{Start: 720, End: 754}}) {
		t.Fatalf("merged = %+v, muon [[720,754]]", merged)
	}
	if seconds != 34 {
		t.Fatalf("watched_seconds = %d, muon 34 (doan 0..720 bi tua qua khong duoc tinh)", seconds)
	}
}

// TestMergePlayedRanges_XemLaiKhongCongDon: phat lai cung mot doan (hoc lai, hoac heartbeat gui
// trung khoang) KHONG duoc cong don thanh 2 lan. Neu test nay do, nguoi hoc chi can phat lai
// doan dau vai lan la watched_seconds vuot thoi luong video.
func TestMergePlayedRanges_XemLaiKhongCongDon(t *testing.T) {
	daLuu := model.PlayedRanges{{Start: 0, End: 120}}
	incoming := []model.PlayedRange{{Start: 0, End: 120}}

	merged, seconds := MergePlayedRanges(daLuu, incoming, 754)

	if !sameRanges(merged, model.PlayedRanges{{Start: 0, End: 120}}) {
		t.Fatalf("merged = %+v, muon [[0,120]] — xem lai khong duoc tao khoang thu hai", merged)
	}
	if seconds != 120 {
		t.Fatalf("watched_seconds = %d, muon 120 (khong phai 240)", seconds)
	}
}

// TestMergePlayedRanges_ChongLanVaKeNhau: hai khoang chong lan ([0,120] + [118,754]) va hai
// khoang chi cham nhau ([0,10] + [10,20]) deu phai gop thanh mot doan lien mach. Contract §1 liet
// ke dung vi du [[0, 120], [118, 754]] nen day la ca chinh thuc, khong phai bien.
func TestMergePlayedRanges_ChongLanVaKeNhau(t *testing.T) {
	t.Run("chong lan", func(t *testing.T) {
		merged, seconds := MergePlayedRanges(nil,
			[]model.PlayedRange{{Start: 0, End: 120}, {Start: 118, End: 754}}, 754)

		if !sameRanges(merged, model.PlayedRanges{{Start: 0, End: 754}}) {
			t.Fatalf("merged = %+v, muon [[0,754]]", merged)
		}
		// 754 chu khong phai 120+636=756: phan chong 118..120 chi duoc tinh MOT lan.
		if seconds != 754 {
			t.Fatalf("watched_seconds = %d, muon 754", seconds)
		}
	})

	t.Run("cham nhau", func(t *testing.T) {
		merged, seconds := MergePlayedRanges(nil,
			[]model.PlayedRange{{Start: 0, End: 10}, {Start: 10, End: 20}}, 754)

		if !sameRanges(merged, model.PlayedRanges{{Start: 0, End: 20}}) {
			t.Fatalf("merged = %+v, muon [[0,20]]", merged)
		}
		if seconds != 20 {
			t.Fatalf("watched_seconds = %d, muon 20", seconds)
		}
	})

	t.Run("roi rac va nguoc thu tu", func(t *testing.T) {
		merged, seconds := MergePlayedRanges(nil, []model.PlayedRange{
			{Start: 300, End: 400},
			{Start: 0, End: 100},
			{Start: 350, End: 500},
		}, 754)

		if !sameRanges(merged, model.PlayedRanges{{Start: 0, End: 100}, {Start: 300, End: 500}}) {
			t.Fatalf("merged = %+v, muon [[0,100],[300,500]]", merged)
		}
		if seconds != 300 {
			t.Fatalf("watched_seconds = %d, muon 300", seconds)
		}
	})
}

// TestMergePlayedRanges_BoKhoangXauKhongLamHongRequest: contract §1 ghi ro "bo khoang khong hop
// le (khong loi ca request)". Mot khoang hong chi duoc phep mat chinh no; cac khoang hop le di
// kem van phai duoc tinh. Neu test nay do theo huong nguoc lai (tra loi/500), mot client loi
// lam mat TOAN BO tien do cua lan heartbeat do.
func TestMergePlayedRanges_BoKhoangXauKhongLamHongRequest(t *testing.T) {
	incoming := []model.PlayedRange{
		{Start: 0, End: 100},     // hop le
		{Start: -5, End: 10},     // start am
		{Start: 200, End: 100},   // end <= start (dao nguoc)
		{Start: 50, End: 50},     // do dai 0
		{Start: 900, End: 1000},  // vuot duration
		{Start: 100, End: 250.4}, // hop le, so thuc
	}

	merged, seconds := MergePlayedRanges(nil, incoming, 754)

	// [0,100] va [100,250] CHAM NHAU (End == Start) nen gop thanh [0,250] — dung hanh vi da
	// khang dinh o TestMergePlayedRanges_ChongLanVaKeNhau/"cham nhau". Diem can kiem o TEST NAY
	// la 4 khoang hong bi loai, khong lam sai tong so giay cua 2 khoang hop le con lai.
	if !sameRanges(merged, model.PlayedRanges{{Start: 0, End: 250}}) {
		t.Fatalf("merged = %+v, muon [[0,250]] — 4 khoang xau phai bi bo qua, 2 khoang hop le con lai (cham nhau) phai gop thanh mot", merged)
	}
	if seconds != 250 {
		t.Fatalf("watched_seconds = %d, muon 250", seconds)
	}
}

// TestMergePlayedRanges_KhoangDaLuuCungBiKiem: du lieu CU trong DB cung phai qua normalize. Mot
// dong ghi tu truoc khi co rang buoc co the chua khoang vuot thoi luong; neu khong kiem lai,
// watched_pct se vuot 100% va khong bao gio dat dung nguong completed.
//
// B-1 (review vòng 2): SỬA kỳ vọng — khoảng vượt duration giờ bị CLAMP về duration (không phải
// bị LOẠI như trước bản vá). Loại hẳn từng là chính lỗ hổng B-1 khai thác được ở chiều ngược lại
// (một request SAU dùng duration_seconds NHỎ hơn có thể xoá sạch khoảng đã lưu hợp lệ trước đó vì
// nó "vượt" cái duration giả mới); clamp giữ lại phần nằm trong duration thay vì mất trắng.
func TestMergePlayedRanges_KhoangDaLuuCungBiKiem(t *testing.T) {
	daLuu := model.PlayedRanges{{Start: 0, End: 9999}} // vuot duration 754, phai bi CLAMP ve 754

	merged, seconds := MergePlayedRanges(daLuu, []model.PlayedRange{{Start: 10, End: 20}}, 754)

	// [0,9999] clamp thanh [0,754], gom voi [10,20] (nam gon trong [0,754]) van la [0,754].
	if !sameRanges(merged, model.PlayedRanges{{Start: 0, End: 754}}) {
		t.Fatalf("merged = %+v, muon [[0,754]] — khoang cu vuot thoi luong phai bi CLAMP, khong duoc mat trang", merged)
	}
	if seconds != 754 {
		t.Fatalf("watched_seconds = %d, muon 754", seconds)
	}
}

// TestMergePlayedRanges_StartVuotDurationThiBiLoai: khac voi khoang chi VUOT o End (duoc clamp,
// xem test tren), mot khoang co Start DA >= duration thi khong the clamp ve gi ca (clamp end se
// <= start) — day la truong hop DUY NHAT con bi loai hoan toan sau sua B-1.
func TestMergePlayedRanges_StartVuotDurationThiBiLoai(t *testing.T) {
	merged, seconds := MergePlayedRanges(nil, []model.PlayedRange{
		{Start: 900, End: 1000}, // start da vuot duration 754, khong clamp duoc
		{Start: 100, End: 200},  // hop le
	}, 754)

	if !sameRanges(merged, model.PlayedRanges{{Start: 100, End: 200}}) {
		t.Fatalf("merged = %+v, muon [[100,200]] — khoang co start vuot duration phai bi loai", merged)
	}
	if seconds != 100 {
		t.Fatalf("watched_seconds = %d, muon 100", seconds)
	}
}

// TestMergePlayedRanges_EndVuotDurationDuocClampKhongMatDuLieu: đúng test team-lead yêu cầu cho
// B-1 (dạng played_ranges) — một khoảng vượt nhẹ qua duration (sai số làm tròn/heartbeat cuối)
// phải được GIỮ LẠI phần hợp lệ, không bị xoá trắng.
func TestMergePlayedRanges_EndVuotDurationDuocClampKhongMatDuLieu(t *testing.T) {
	merged, seconds := MergePlayedRanges(nil, []model.PlayedRange{
		{Start: 1190, End: 1205}, // duration 1200, end vuot nhe 5s
	}, 1200)

	if !sameRanges(merged, model.PlayedRanges{{Start: 1190, End: 1200}}) {
		t.Fatalf("merged = %+v, muon [[1190,1200]] — end vuot duration phai duoc CLAMP, khong bi xoa", merged)
	}
	if seconds != 10 {
		t.Fatalf("watched_seconds = %d, muon 10", seconds)
	}
}

// TestMergePlayedRanges_ChuaBietThoiLuong: beacon co the toi truoc khi trinh phat doc duoc
// metadata (duration_seconds = 0). Khi do khong kiem duoc can tren, nhung van phai kiem
// start >= 0 va end > start — neu khong, mot khoang am se lam watched_seconds am.
func TestMergePlayedRanges_ChuaBietThoiLuong(t *testing.T) {
	merged, seconds := MergePlayedRanges(nil, []model.PlayedRange{
		{Start: 0, End: 60},
		{Start: -30, End: 10},
	}, 0)

	if !sameRanges(merged, model.PlayedRanges{{Start: 0, End: 60}}) {
		t.Fatalf("merged = %+v, muon [[0,60]]", merged)
	}
	if seconds != 60 {
		t.Fatalf("watched_seconds = %d, muon 60", seconds)
	}
}

// TestMergePlayedRanges_RongTraVeRong: khong co khoang nao (moi truong hop cua ban ghi moi) tra
// ve danh sach rong va 0 giay — khong duoc panic tren slice rong.
func TestMergePlayedRanges_RongTraVeRong(t *testing.T) {
	merged, seconds := MergePlayedRanges(nil, nil, 754)

	if len(merged) != 0 {
		t.Fatalf("merged = %+v, muon rong", merged)
	}
	if seconds != 0 {
		t.Fatalf("watched_seconds = %d, muon 0", seconds)
	}
}

// ---------------------------------------------------------------------------
// 2. watched_pct
// ---------------------------------------------------------------------------

// TestWatchedPercent_LamTron1ChuSoThapPhan: contract §1 ghi ro round(watched/duration*100, 1).
func TestWatchedPercent_LamTron1ChuSoThapPhan(t *testing.T) {
	cases := []struct {
		watched, duration int
		want              string
	}{
		{754, 754, "100"},
		{120, 754, "15.9"},
		{679, 754, "90.1"},
		{678, 754, "89.9"},
		{0, 754, "0"},
	}

	for _, c := range cases {
		got, ok := WatchedPercent(c.watched, c.duration)
		if !ok {
			t.Fatalf("WatchedPercent(%d,%d) tra ok=false", c.watched, c.duration)
		}
		if got.String() != c.want {
			t.Fatalf("WatchedPercent(%d,%d) = %s, muon %s", c.watched, c.duration, got.String(), c.want)
		}
	}
}

// TestWatchedPercent_ChuaBietThoiLuongTraOkFalse: duration <= 0 thi caller phai GIU NGUYEN
// watched_pct dang luu thay vi ghi 0 — ghi 0 se ha diem mot bai da hoc xong.
func TestWatchedPercent_ChuaBietThoiLuongTraOkFalse(t *testing.T) {
	for _, duration := range []int{0, -1} {
		if _, ok := WatchedPercent(300, duration); ok {
			t.Fatalf("WatchedPercent(300,%d) tra ok=true, muon false", duration)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. Quyet dinh status (auto-completed, completed bat bien, client khong tu chot)
// ---------------------------------------------------------------------------

// TestResolveLessonStatus_DatNguongThiTuChotCompleted: server tu chot completed khi
// watched_pct >= min_video_pct, du client khong gui status gi.
func TestResolveLessonStatus_DatNguongThiTuChotCompleted(t *testing.T) {
	cases := []struct {
		name      string
		current   string
		requested *string
		pct       decimal.Decimal
		minPct    int
		want      string
	}{
		{"dung bang nguong", "in_progress", nil, decimal.NewFromFloat(90), 90, "completed"},
		{"tren nguong", "in_progress", nil, decimal.NewFromFloat(99.9), 90, "completed"},
		{"duoi nguong", "in_progress", nil, decimal.NewFromFloat(89.9), 90, "in_progress"},
		{"nguong rieng cua khoa", "in_progress", nil, decimal.NewFromFloat(80), 75, "completed"},
		{"client gui in_progress nhung da dat nguong", "in_progress", strPtr("in_progress"), decimal.NewFromFloat(95), 90, "completed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveLessonStatus(c.current, c.requested, c.pct, c.minPct, true); got != c.want {
				t.Fatalf("resolveLessonStatus = %q, muon %q", got, c.want)
			}
		})
	}
}

// TestResolveLessonStatus_ClientGuiCompletedBiBoQua: day la lo hong ma ca §1 sinh ra de bit.
// Client tu gui status="completed" (hoac keo thanh tua roi goi API) KHONG duoc chap nhan neu
// chinh server chua tinh ra watched_pct dat nguong.
func TestResolveLessonStatus_ClientGuiCompletedBiBoQua(t *testing.T) {
	got := resolveLessonStatus("in_progress", strPtr("completed"), decimal.NewFromFloat(12.5), 90, true)

	if got != "in_progress" {
		t.Fatalf("status = %q, muon \"in_progress\" — client khong duoc tu chot completed", got)
	}
}

// TestResolveLessonStatus_CompletedLaBatBien: mot khi da completed thi khong bao gio ha cap, ke
// ca khi client gui not_started (hoac beacon dong tab gui in_progress).
func TestResolveLessonStatus_CompletedLaBatBien(t *testing.T) {
	cases := []struct {
		name      string
		requested *string
	}{
		{"khong gui status", nil},
		{"gui in_progress", strPtr("in_progress")},
		{"gui not_started", strPtr("not_started")},
		{"gui completed nhung pct thap", strPtr("completed")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveLessonStatus("completed", c.requested, decimal.NewFromFloat(10), 90, true)
			if got != "completed" {
				t.Fatalf("status = %q, muon \"completed\" — status chi duoc di len", got)
			}
		})
	}
}

// TestResolveLessonStatus_StatusDiLenKhongDiXuong: giua not_started/in_progress thi cap bac moi
// phai >= cap bac hien tai moi duoc ghi; gui in_progress cho bai vua mo (not_started) la hop le.
func TestResolveLessonStatus_StatusDiLenKhongDiXuong(t *testing.T) {
	if got := resolveLessonStatus("not_started", strPtr("in_progress"), decimal.Zero, 90, true); got != "in_progress" {
		t.Fatalf("status = %q, muon \"in_progress\"", got)
	}
	if got := resolveLessonStatus("in_progress", strPtr("not_started"), decimal.Zero, 90, true); got != "in_progress" {
		t.Fatalf("status = %q, muon \"in_progress\" — khong duoc lui ve not_started", got)
	}
}

// ---------------------------------------------------------------------------
// 4. next_lesson_unlocked
// ---------------------------------------------------------------------------

// TestNextLessonUnlocked_KhoaKhongTuanTu: khoa thuong thi bai ke tiep luon mo; bai cuoi cua khoa
// tra ve false vi khong con bai nao de mo.
func TestNextLessonUnlocked_KhoaKhongTuanTu(t *testing.T) {
	bai1, bai2 := uuid.New(), uuid.New()
	order := []uuid.UUID{bai1, bai2}

	if !nextLessonUnlocked(order, bai1, false, "in_progress") {
		t.Fatal("bai 1 xong ke tiep phai mo khi khoa khong bat tuan tu")
	}
	if nextLessonUnlocked(order, bai2, false, "completed") {
		t.Fatal("bai cuoi cua khoa phai tra false — khong con bai nao de mo")
	}
	if nextLessonUnlocked(order, uuid.New(), false, "completed") {
		t.Fatal("bai khong nam trong khoa phai tra false")
	}
}

// TestNextLessonUnlocked_KhoaTuanTu: bat tuan tu thi bai ke tiep chi mo khi bai hien tai da
// completed — dung dieu kien ma §2 dung lai de tinh `locked` cho curriculum.
func TestNextLessonUnlocked_KhoaTuanTu(t *testing.T) {
	bai1, bai2 := uuid.New(), uuid.New()
	order := []uuid.UUID{bai1, bai2}

	if nextLessonUnlocked(order, bai1, true, "in_progress") {
		t.Fatal("khoa tuan tu: bai 1 moi hoc do dang thi bai 2 phai con khoa")
	}
	if !nextLessonUnlocked(order, bai1, true, "completed") {
		t.Fatal("khoa tuan tu: bai 1 da completed thi bai 2 phai mo")
	}
}

// ---------------------------------------------------------------------------
// 5. Dang JSON cua played_ranges (cap so, khong phai object)
// ---------------------------------------------------------------------------

// TestPlayedRangesDTO_DocDangCapSo: dang CHINH THUC theo contract §1 va theo web
// (`type PlayedRange = [number, number]` trong web/src/lib/played-ranges.ts). Doc sai dang nay
// thi MOI heartbeat tu web deu 400.
func TestPlayedRangesDTO_DocDangCapSo(t *testing.T) {
	var req dto.UpdateLessonProgressDTO
	body := `{"status":"in_progress","position_seconds":754,"duration_seconds":754,
	          "played_ranges":[[0,120],[118,754]]}`

	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("khong doc duoc body cua web: %v", err)
	}
	if len(req.PlayedRanges) != 2 {
		t.Fatalf("doc duoc %d khoang, muon 2", len(req.PlayedRanges))
	}
	if req.PlayedRanges[0].Start != 0 || req.PlayedRanges[0].End != 120 {
		t.Fatalf("khoang 0 = %+v, muon [0,120]", req.PlayedRanges[0])
	}
	if req.PlayedRanges[1].Start != 118 || req.PlayedRanges[1].End != 754 {
		t.Fatalf("khoang 1 = %+v, muon [118,754]", req.PlayedRanges[1])
	}

	// Di het duong: DTO -> model -> merge, dung nhu service lam.
	merged, seconds := MergePlayedRanges(nil, toModelPlayedRanges(req.PlayedRanges), 754)
	if !sameRanges(merged, model.PlayedRanges{{Start: 0, End: 754}}) || seconds != 754 {
		t.Fatalf("merged = %+v (%d giay), muon [[0,754]] (754 giay)", merged, seconds)
	}
}

// TestPlayedRangesDTO_BoQuaPhanTuHong: mot phan tu sai dinh dang bi bo qua, cac phan tu con lai
// van duoc doc — thay vi keo ca heartbeat ve 400.
func TestPlayedRangesDTO_BoQuaPhanTuHong(t *testing.T) {
	var req dto.UpdateLessonProgressDTO
	body := `{"played_ranges":[[0,120],[1],"abc",null,[118,754],[1,2,3]]}`

	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("mot phan tu hong lam hong ca request: %v", err)
	}
	if len(req.PlayedRanges) != 2 {
		t.Fatalf("doc duoc %d khoang, muon 2 (4 phan tu hong phai bi bo qua)", len(req.PlayedRanges))
	}
	if req.PlayedRanges[1].Start != 118 || req.PlayedRanges[1].End != 754 {
		t.Fatalf("khoang thu hai = %+v, muon [118,754]", req.PlayedRanges[1])
	}
}

// TestPlayedRangesDTO_VangMatHoacNullKhongPhaiLoi: truong la optional (client cu khong gui), nen
// thieu truong / null / mang rong deu phai doc duoc thanh "khong co khoang nao".
func TestPlayedRangesDTO_VangMatHoacNullKhongPhaiLoi(t *testing.T) {
	for _, body := range []string{`{}`, `{"played_ranges":null}`, `{"played_ranges":[]}`} {
		var req dto.UpdateLessonProgressDTO
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatalf("body %s lam hong request: %v", body, err)
		}
		if len(req.PlayedRanges) != 0 {
			t.Fatalf("body %s cho ra %d khoang, muon 0", body, len(req.PlayedRanges))
		}
	}
}

// TestPlayedRangesDTO_MarshalRaCapSo: chieu ra cung phai la cap so, de mot response/echo khong
// am tham doi dinh dang ma web dang doc.
func TestPlayedRangesDTO_MarshalRaCapSo(t *testing.T) {
	out, err := json.Marshal(dto.PlayedRangesDTO{
		{Start: 0, End: 120},
		{Start: 118, End: 754},
	})
	if err != nil {
		t.Fatalf("marshal loi: %v", err)
	}
	if string(out) != "[[0,120],[118,754]]" {
		t.Fatalf("marshal ra %s, muon [[0,120],[118,754]]", out)
	}
}

// TestLessonProgressStateDTO_CompletedAtLuonCoMat: contract §1 ghi `completed_at: "...|null"` va
// web khai `completed_at: string | null`. Bo sot truong (omitempty) khac voi null o phia client
// nen truong nay PHAI xuat hien, ke ca khi con null.
func TestLessonProgressStateDTO_CompletedAtLuonCoMat(t *testing.T) {
	out, err := json.Marshal(dto.LessonProgressStateDTO{})
	if err != nil {
		t.Fatalf("marshal loi: %v", err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal loi: %v", err)
	}

	for _, key := range []string{
		"lesson_id", "status", "watched_seconds", "watched_pct",
		"last_position_seconds", "completed_at", "next_lesson_unlocked",
	} {
		if _, ok := parsed[key]; !ok {
			t.Fatalf("thieu truong %q trong data cua response (contract §1) — JSON: %s", key, out)
		}
	}
	if string(parsed["completed_at"]) != "null" {
		t.Fatalf("completed_at = %s, muon null khi chua hoc xong", parsed["completed_at"])
	}
}
