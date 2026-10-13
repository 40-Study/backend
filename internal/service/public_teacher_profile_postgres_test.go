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
	if s == nil {
		t.Fatal("stats nil")
	}
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
	if p.Stats == nil || !p.Stats.AverageRating.IsZero() || p.Stats.CourseCount != 1 {
		t.Fatalf("stats: %+v", p.Stats)
	}
	empty := w.teacher()
	p, err = w.svc.Get(context.Background(), empty.ID)
	if err != nil || p.Courses == nil || len(p.Courses) != 0 || p.Stats == nil || p.Stats.CourseCount != 0 {
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

// Review QA2 L4 (quyết định chủ dự án 13/10): trang giảng viên công khai TÔN TRỌNG cài đặt "Hiển thị hồ sơ"
// (user_preferences.profile_visibility). Người xem trang này luôn là khách ẩn danh, nên chỉ `public` được xem đầy đủ;
// hidden (và private/friends, vốn chỉ cho khách thấy phần tối thiểu, và giá trị lạ - fail-closed) chỉ còn tên + các khoá
// đã xuất bản, kèm profile_hidden=true: không bio, avatar, hồ sơ chuyên môn, thống kê, ngày tham gia.
func TestPublicTeacherProfile_TonTrongCaiDatHienThiHoSo(t *testing.T) {
	w := newPublicTeacherWorld(t)
	ctx := context.Background()
	spec := "Mobile"
	mk := func(visibility string) (model.User, model.Course) {
		u := w.teacher()
		avatar := "https://cdn.test/a.png"
		w.f.db.Model(&u).Updates(map[string]any{"full_name": "Giang Vien Ten", "avatar_url": avatar})
		w.f.db.Create(&model.TeacherProfile{UserID: u.ID, Specialization: &spec, ApprovalStatus: model.TeacherApprovalApproved})
		if visibility != "" {
			w.f.db.Create(&model.UserPreference{UserID: u.ID, ProfileVisibility: visibility})
		}
		return u, w.course(u, model.CourseStatusPublished, 5, 2, "4.00")
	}

	for _, vis := range []string{"", "public"} {
		u, _ := mk(vis)
		p, err := w.svc.Get(ctx, u.ID)
		if err != nil || p.ProfileHidden || p.Bio == nil || p.AvatarURL == nil || p.Stats == nil || p.JoinedAt == nil || p.Specialization == nil {
			t.Errorf("visibility %q phai day du: %+v err=%v", vis, p, err)
		}
	}
	for _, vis := range []string{"hidden", "private", "friends", "gia-tri-la"} {
		u, c := mk(vis)
		p, err := w.svc.Get(ctx, u.ID)
		if err != nil {
			t.Fatalf("%s: %v", vis, err)
		}
		if !p.ProfileHidden || p.FullName == nil || *p.FullName != "Giang Vien Ten" || p.UserName == "" || len(p.Courses) != 1 || p.Courses[0].ID != c.ID {
			t.Errorf("%s: phai con ten + khoa da xuat ban va profile_hidden=true: %+v", vis, p)
		}
		if p.Bio != nil || p.AvatarURL != nil || p.Specialization != nil || p.Education != nil || p.ExperienceYears != nil || p.Department != nil ||
			p.Stats != nil || p.JoinedAt != nil || p.IsVerified {
			t.Errorf("%s: lo du lieu cua ho so bi an: %+v", vis, p)
		}
		raw, _ := json.Marshal(p)
		if strings.Contains(string(raw), "a.png") || strings.Contains(string(raw), "Giang vien Flutter") {
			t.Errorf("%s: JSON lo avatar/bio: %s", vis, raw)
		}
	}
}
