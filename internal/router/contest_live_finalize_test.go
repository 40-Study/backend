package router

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

// finalizeFixture: cuộc thi có giải hạng 1 (voucher + chứng nhận), hạng 2 (chứng nhận) và ngưỡng
// chứng nhận 50%; 3 thí sinh đã nộp (2/2, 2/2, 1/2), cuộc thi đã kết thúc hơn 60s.
// rank2Voucher (tuỳ chọn) gắn thêm voucher cho giải hạng 2.
func (e *ctEnv) finalizeFixture(voucherID uuid.UUID, rank2Voucher ...uuid.UUID) uuid.UUID {
	e.t.Helper()
	rank2 := ""
	if len(rank2Voucher) > 0 {
		rank2 = `,"voucher_id":"` + rank2Voucher[0].String() + `"`
	}
	quizID, correct := e.newQuiz("teacherA")
	r := e.must("tao", e.do("POST", "/api/contests", "teacherA", contestBody(quizID, `"certificate_min_percentage":50`)), 201)
	cid := r.data()["id"].(string)
	e.must("gui duyet", e.do("POST", "/api/contests/"+cid+"/submit-review", "teacherA", ""), 200)
	e.must("duyet", e.do("POST", "/api/admin/contests/"+cid+"/approve", "admin", `{"prizes":[`+
		`{"rank_from":1,"rank_to":1,"grant_certificate":true,"voucher_id":"`+voucherID.String()+`"},`+
		`{"rank_from":2,"rank_to":2,"grant_certificate":true`+rank2+`}]}`), 200)
	id := uuid.MustParse(cid)
	e.setWindow(id, time.Minute, time.Hour)
	for who, right := range map[string]int{"student1": 2, "student2": 2, "student3": 1} {
		e.must("join "+who, e.do("POST", "/api/contests/"+cid+"/join", who, ""), 201)
		st := e.must("start "+who, e.do("POST", "/api/contests/"+cid+"/start", who, ""), 200)
		e.must("submit "+who, e.do("POST", "/api/contests/"+cid+"/submit", who, answersBody(st.data()["attempt_id"].(string), correct, right)), 200)
	}
	e.setWindow(id, 2*time.Hour, -2*time.Minute)
	return id
}

func (e *ctEnv) count(table string, contestID uuid.UUID) int64 {
	var n int64
	e.db.Table(table).Where("contest_id = ?", contestID).Count(&n)
	return n
}

func (e *ctEnv) userVoucherCount() int64 {
	var n int64
	e.db.Table("user_vouchers").Where("source = 'contest_reward'").Count(&n)
	return n
}

// (j) + ĐÍNH CHÍNH: CHỈ admin chốt — GV chủ (dù không có giải voucher), GV khác, học viên đều 403.
// Chốt trước end_time + 60s → 409.
func TestContestLive_FinalizeAdminOnly(t *testing.T) {
	e := newCtEnv(t)
	id, _, _ := e.publishedContest("") // không có giải voucher
	path := "/api/admin/contests/" + id.String() + "/finalize"
	e.setWindow(id, time.Minute, 30*time.Second)
	e.must("admin chot khi chua het gio", e.do("POST", path, "admin", ""), 409, "CONTEST_NOT_ENDED")
	e.setWindow(id, 2*time.Hour, -2*time.Minute)
	for _, who := range []string{"teacherA", "teacherB", "student1", "parent"} {
		e.must(who+" chot", e.do("POST", path, who, ""), 403)
	}
	e.must("route chot cu cua GV khong ton tai", e.do("POST", "/api/contests/"+id.String()+"/finalize", "teacherA", ""), 404)
	// Service cũng tự chặn (phòng khi route bị gắn nhầm quyền).
	_, err := e.svc.Finalize(context.Background(), id, &service.ContestActor{UserID: e.ids["teacherA"], ActiveRole: "TEACHER"})
	if !errors.Is(err, service.ErrContestForbidden) {
		t.Fatalf("service cho GV chu chot: err=%v", err)
	}
	var c model.Contest
	e.db.First(&c, "id = ?", id)
	if c.FinalizedAt != nil {
		t.Fatal("cuoc thi bi chot boi nguoi khong phai admin")
	}
	e.must("admin chot", e.do("POST", path, "admin", ""), 200)
}

// (i) Chốt 2 lần tuần tự: đúng 1 bộ award/voucher, lần 2 already_finalized và không thông báo lại.
func TestContestLive_FinalizeIdempotentSequential(t *testing.T) {
	e := newCtEnv(t)
	id := e.finalizeFixture(e.newVoucher(true))
	path := "/api/admin/contests/" + id.String() + "/finalize"
	r := e.must("chot", e.do("POST", path, "admin", ""), 200)
	d := r.data()
	if d["already_finalized"] != false || d["ranked_count"] != 3.0 || d["award_count"] != 3.0 ||
		d["certificate_count"] != 3.0 || d["voucher_count"] != 1.0 || d["notified_count"] != 3.0 {
		t.Fatalf("ket qua chot sai: %s", r.raw)
	}
	r = e.must("chot lan 2", e.do("POST", path, "admin", ""), 200)
	d = r.data()
	if d["already_finalized"] != true || d["award_count"] != 3.0 || d["voucher_count"] != 1.0 || d["notified_count"] != 0.0 {
		t.Fatalf("chot lan 2 sai: %s", r.raw)
	}
	// Contract §5 bước 7: thông báo gửi SAU commit — lúc được gọi, kết nối ngoài tx đã thấy kết quả chốt.
	if e.issuer.uncommittedCalls != 0 {
		t.Fatal("thong bao duoc gui TRUOC khi ket qua chot commit")
	}
	if e.count("contest_awards", id) != 3 || e.userVoucherCount() != 1 || e.issuer.notifyCalls() != 1 {
		t.Fatalf("phat trung: awards=%d vouchers=%d notify=%d", e.count("contest_awards", id), e.userVoucherCount(), e.issuer.notifyCalls())
	}
	// student3 (1/2 = 50%) ngoài top 2 → chứng nhận theo ngưỡng, rank NULL.
	var aw model.ContestAward
	e.db.First(&aw, "contest_id = ? AND user_id = ?", id, e.ids["student3"])
	if aw.Rank != nil || aw.CertificateNumber == nil || aw.UserVoucherID != nil {
		t.Fatalf("award nguong cua student3 sai: %+v", aw)
	}
	lb := e.must("BXH da chot", e.do("GET", "/api/contests/"+id.String()+"/leaderboard", "", ""), 200)
	if lb.data()["finalized"] != true || len(lb.items()) != 3 {
		t.Fatalf("BXH sau chot sai: %s", lb.raw)
	}
	e.must("chung nhan cua minh", e.do("GET", "/api/contests/"+id.String()+"/certificate", "student1", ""), 200)
	e.must("khong du thi khong co chung nhan", e.do("GET", "/api/contests/"+id.String()+"/certificate", "student4", ""), 404, "CONTEST_CERTIFICATE_NOT_FOUND")
	e.must("huy sau khi chot", e.do("POST", "/api/admin/contests/"+id.String()+"/cancel", "admin", `{"reason":"x"}`), 409, "CONTEST_INVALID_STATUS")
}

// (i) 2 lần chốt ĐỒNG THỜI: row lock tuần tự hoá — đúng 1 lần phát, lần kia already_finalized.
func TestContestLive_FinalizeConcurrent(t *testing.T) {
	e := newCtEnv(t)
	id := e.finalizeFixture(e.newVoucher(true))
	var wg sync.WaitGroup
	res := make([]ctResp, 2)
	for i := range res {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res[i] = e.do("POST", "/api/admin/contests/"+id.String()+"/finalize", "admin", "")
		}(i)
	}
	wg.Wait()
	first := 0
	for _, r := range res {
		if r.status != 200 {
			t.Fatalf("chot dong thoi: %d %s", r.status, r.raw)
		}
		if r.data()["already_finalized"] == false {
			first++
		}
	}
	if first != 1 || e.count("contest_awards", id) != 3 || e.userVoucherCount() != 1 || e.issuer.notifyCalls() != 1 {
		t.Fatalf("chot dong thoi phat trung: first=%d awards=%d vouchers=%d notify=%d",
			first, e.count("contest_awards", id), e.userVoucherCount(), e.issuer.notifyCalls())
	}
}

// (i) Voucher giải hạng 2 bị tắt trước khi chốt → 409 và rollback SẠCH: voucher hạng 1 (đã phát
// trong CÙNG tx trước khi gặp lỗi) cũng biến mất; không hạng, không award, chưa chốt. Bật lại
// voucher thì chốt được.
func TestContestLive_FinalizeVoucherDisabledRollsBack(t *testing.T) {
	e := newCtEnv(t)
	good, voucherID := e.newVoucher(true), e.newVoucher(true)
	id := e.finalizeFixture(good, voucherID)
	e.db.Exec("UPDATE vouchers SET is_active = false WHERE id = ?", voucherID)
	e.must("chot voi voucher tat", e.do("POST", "/api/admin/contests/"+id.String()+"/finalize", "admin", ""), 409, "CONTEST_VOUCHER_UNAVAILABLE")
	var ranked int64
	e.db.Model(&model.ContestParticipant{}).Where("contest_id = ? AND rank IS NOT NULL", id).Count(&ranked)
	var c model.Contest
	e.db.First(&c, "id = ?", id)
	if ranked != 0 || e.count("contest_awards", id) != 0 || e.userVoucherCount() != 0 || c.FinalizedAt != nil || e.issuer.notifyCalls() != 0 {
		t.Fatalf("rollback khong sach: ranked=%d awards=%d vouchers=%d finalized=%v", ranked, e.count("contest_awards", id), e.userVoucherCount(), c.FinalizedAt)
	}
	e.db.Exec("UPDATE vouchers SET is_active = true WHERE id = ?", voucherID)
	e.must("chot lai", e.do("POST", "/api/admin/contests/"+id.String()+"/finalize", "admin", ""), 200)
}
