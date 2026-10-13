package service

// PG-M2 (QA vòng 2 261013): trang giảng viên công khai (GET /teachers/:id/public-profile). Postgres thật vì các
// luật cần pin nằm ở SQL: chỉ khoá published (không draft/archived/đã xoá mềm), thống kê tính từ đúng các khoá
// đó (rating bình quân theo số đánh giá), chỉ người đang giữ vai trò TEACHER còn hoạt động mới có trang, và
// thông tin hồ sơ chuyên môn chỉ hiện khi hồ sơ đã được duyệt.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type publicTeacherWorld struct {
	t   *testing.T
	f   *s2Fixture
	svc *PublicTeacherProfileService
}

func newPublicTeacherWorld(t *testing.T) *publicTeacherWorld {
	t.Helper()
	db := isolatedAPISchema(t)
	return &publicTeacherWorld{t: t, f: &s2Fixture{t: t, db: db}, svc: NewPublicTeacherProfileService(repository.NewPublicTeacherProfileRepository(db))}
}

// teacher tạo user có vai trò TEACHER đang hoạt động, kèm bio.
func (w *publicTeacherWorld) teacher() model.User {
	w.t.Helper()
	u := w.f.user("teacher")
	bio := "Giang vien Flutter 8 nam"
	w.f.db.Model(&u).Update("bio", bio)
	w.grantTeacher(u.ID, model.UserSystemRoleStatusActive)
	return u
}

func (w *publicTeacherWorld) grantTeacher(userID uuid.UUID, status string) {
	w.t.Helper()
	role := model.SystemRole{Name: "TEACHER", Status: "active"}
	if err := w.f.db.Where("name = ?", "TEACHER").FirstOrCreate(&role).Error; err != nil {
		w.t.Fatalf("tao role TEACHER: %v", err)
	}
	usr := model.UserSystemRole{UserID: userID, SystemRoleID: role.ID, Status: status}
	if err := w.f.db.Create(&usr).Error; err != nil {
		w.t.Fatalf("gan role: %v", err)
	}
	if status != model.UserSystemRoleStatusActive { // default:'active' của GORM nuốt giá trị khác khi zero; ghi lại cho chắc
		w.f.db.Model(&usr).Update("status", status)
	}
}

func (w *publicTeacherWorld) course(teacher model.User, status string, students, reviews int, rating string) model.Course {
	w.t.Helper()
	c := w.f.course(teacher)
	w.f.db.Model(&c).Updates(map[string]any{"status": status, "total_students": students, "total_reviews": reviews, "average_rating": decimal.RequireFromString(rating)})
	return c
}

func TestPublicTeacherProfile_ChiKhoaPublishedVaThongKeDung(t *testing.T) {
	w := newPublicTeacherWorld(t)
	ctx := context.Background()
	teacher := w.teacher()
	a := w.course(teacher, model.CourseStatusPublished, 10, 4, "5.00")
	b := w.course(teacher, model.CourseStatusPublished, 30, 6, "4.00")
	w.course(teacher, model.CourseStatusDraft, 999, 99, "1.00")
	w.course(teacher, model.CourseStatusArchived, 999, 99, "1.00")
	gone := w.course(teacher, model.CourseStatusPublished, 999, 99, "1.00")
	w.f.db.Delete(&gone)
	other := w.f.user("other-teacher")
	w.course(other, model.CourseStatusPublished, 500, 50, "3.00") // khoá của giảng viên khác không được tính

	p, err := w.svc.Get(ctx, teacher.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.ID != teacher.ID || p.Bio == nil || *p.Bio != "Giang vien Flutter 8 nam" {
		t.Fatalf("thong tin co ban sai: %+v", p)
	}
	got := map[uuid.UUID]bool{}
	for _, c := range p.Courses {
		got[c.ID] = true
	}
	if len(p.Courses) != 2 || !got[a.ID] || !got[b.ID] {
		t.Fatalf("muon dung 2 khoa published cua giang vien, duoc %d: %v", len(p.Courses), got)
	}
	// 10+30 hoc vien, 4+6 danh gia, rating trung binh co trong so theo so danh gia: (5*4 + 4*6) / 10 = 4.4.
	s := p.Stats
	if s.CourseCount != 2 || s.StudentCount != 40 || s.ReviewCount != 10 || !s.AverageRating.Equal(decimal.RequireFromString("4.4")) {
		t.Fatalf("stats sai: %+v", s)
	}
}

func TestPublicTeacherProfile_KhongCoDanhGiaThiRatingBang0(t *testing.T) {
	w := newPublicTeacherWorld(t)
	teacher := w.teacher()
	w.course(teacher, model.CourseStatusPublished, 3, 0, "0.00")
	p, err := w.svc.Get(context.Background(), teacher.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !p.Stats.AverageRating.IsZero() || p.Stats.CourseCount != 1 {
		t.Fatalf("stats: %+v", p.Stats)
	}
	empty := w.teacher()
	p, err = w.svc.Get(context.Background(), empty.ID)
	if err != nil || p.Courses == nil || len(p.Courses) != 0 || p.Stats.CourseCount != 0 {
		t.Fatalf("giang vien chua co khoa: courses=%#v stats=%+v err=%v", p.Courses, p.Stats, err)
	}
}

func TestPublicTeacherProfile_ChiNguoiGiuVaiTroTeacherConHoatDongMoCoTrang(t *testing.T) {
	w := newPublicTeacherWorld(t)
	ctx := context.Background()

	student := w.f.user("student")
	locked := w.teacher()
	w.f.db.Model(&model.User{}).Where("id = ?", locked.ID).Update("is_active", false)
	revoked := w.f.user("revoked")
	w.grantTeacher(revoked.ID, model.UserSystemRoleStatusInactive)
	deleted := w.teacher()
	w.f.db.Delete(&deleted)

	for name, id := range map[string]uuid.UUID{"khong ton tai": uuid.New(), "khong co role teacher": student.ID, "bi khoa": locked.ID, "role bi thu hoi": revoked.ID, "da xoa mem": deleted.ID} {
		if _, err := w.svc.Get(ctx, id); err != ErrTeacherPublicProfileNotFound {
			t.Errorf("%s: err = %v, muon ErrTeacherPublicProfileNotFound", name, err)
		}
	}
}

func TestPublicTeacherProfile_HoSoChuyenMonChiHienKhiDaDuyet(t *testing.T) {
	w := newPublicTeacherWorld(t)
	ctx := context.Background()
	spec, edu, dept, cert := "Mobile", "Thac si CNTT", "Khoa CNTT", "Google Certified"
	years := 8
	mk := func(approval string) uuid.UUID {
		u := w.teacher()
		p := model.TeacherProfile{UserID: u.ID, Specialization: &spec, Education: &edu, ExperienceYears: &years, Department: &dept, CertificateInfo: &cert, ApprovalStatus: approval}
		if err := w.f.db.Create(&p).Error; err != nil {
			t.Fatalf("tao ho so: %v", err)
		}
		return u.ID
	}

	approved, err := w.svc.Get(ctx, mk(model.TeacherApprovalApproved))
	if err != nil || approved.Specialization == nil || *approved.Specialization != spec || approved.ExperienceYears == nil || *approved.ExperienceYears != 8 ||
		approved.Education == nil || approved.Department == nil {
		t.Fatalf("ho so da duyet phai hien: %+v err=%v", approved, err)
	}
	for _, status := range []string{model.TeacherApprovalPending, model.TeacherApprovalRejected} {
		p, err := w.svc.Get(ctx, mk(status))
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if p.Specialization != nil || p.Education != nil || p.ExperienceYears != nil || p.Department != nil {
			t.Errorf("ho so %s khong duoc lo thong tin chuyen mon: %+v", status, p)
		}
	}
}

// Trang công khai không bao giờ mang PII, dữ liệu ngân hàng hay trạng thái duyệt.
func TestPublicTeacherProfile_JSONKhongCoTruongNhayCam(t *testing.T) {
	w := newPublicTeacherWorld(t)
	u := w.teacher()
	bank, acct, reason := "VCB", "0123456789", "bang cap gia"
	phone := "0900000000"
	w.f.db.Model(&u).Update("phone", phone)
	w.f.db.Create(&model.TeacherProfile{UserID: u.ID, BankName: &bank, BankAccountNumber: &acct, RejectionReason: &reason, ApprovalStatus: model.TeacherApprovalApproved})
	w.course(u, model.CourseStatusPublished, 1, 1, "5.00")

	p, err := w.svc.Get(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	raw, _ := json.Marshal(p)
	body := strings.ToLower(string(raw))
	for _, banned := range []string{"email", "phone", "0900000000", "date_of_birth", "bank", "0123456789", "approval", "rejection", "bang cap gia", "certificate", "40study.test"} {
		if strings.Contains(body, banned) {
			t.Errorf("JSON cong khai lo %q: %s", banned, raw)
		}
	}
}
