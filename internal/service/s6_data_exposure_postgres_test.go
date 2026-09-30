package service

// Lane S6 (Postgres thật), theo từng vai — ba chỗ lộ dữ liệu / sai điều kiện ở route công khai:
//  1. GET /teachers?keyword= và GET /teacher-profiles?keyword= khớp theo EMAIL giảng viên: gõ đúng
//     "ten@gmail.com" (hoặc dò tiền tố) là dựng lại được email, dù S3 đã bỏ email khỏi DTO.
//  2. POST /courses/:id/reviews chỉ cần đăng nhập: ai cũng chấm sao khoá chưa học; đơn hoàn tiền vẫn giữ quyền.
//  3. GET /users/:id/public-profile bỏ qua cài đặt riêng tư: khách cũng xem đủ hồ sơ.
// Bỏ điều kiện tương ứng ở repository/service thì test ĐỎ.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/apperr"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// ── 1. Tìm kiếm giảng viên không theo email ─────────────────────────────────

func TestS6_Teacher_TimKiemCongKhaiKhongTheoEmail(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	tag := uuid.NewString()[:8]
	email := "kin-mat-" + tag + "@40study.test"
	fullName := "Nguyen Van Giang " + tag
	teacher := model.User{Email: email, PasswordHash: "x", UserName: "gv_" + tag, FullName: &fullName, IsActive: true}
	if err := f.db.Create(&teacher).Error; err != nil {
		t.Fatal(err)
	}
	role := model.SystemRole{Name: "TEACHER", Status: "active"}
	if err := f.db.Where("name = ?", role.Name).FirstOrCreate(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.UserSystemRole{UserID: teacher.ID, SystemRoleID: role.ID, Status: model.UserSystemRoleStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	spec := "Toan hoc " + tag
	if err := f.db.Create(&model.TeacherProfile{UserID: teacher.ID, Specialization: &spec, ApprovalStatus: model.TeacherApprovalApproved}).Error; err != nil {
		t.Fatal(err)
	}

	teachers := repository.NewTeacherRepository(f.db)
	profiles := repository.NewTeacherProfileRepository(f.db)
	countTeachers := func(kw string) int64 {
		_, total, err := teachers.GetAllTeachers(ctx, 1, 20, kw, "")
		if err != nil {
			t.Fatal(err)
		}
		return total
	}
	countProfiles := func(kw string) int64 {
		_, total, err := profiles.GetAll(ctx, 1, 20, kw, "")
		if err != nil {
			t.Fatal(err)
		}
		return total
	}

	// Email đầy đủ, tiền tố email, và phần miền email đều KHÔNG được khớp.
	for _, kw := range []string{email, "kin-mat-" + tag, "kin-mat", "@40study.test"} {
		if n := countTeachers(kw); n != 0 {
			t.Errorf("/teachers?keyword=%q khớp %d giảng viên theo email, muốn 0", kw, n)
		}
		if n := countProfiles(kw); n != 0 {
			t.Errorf("/teacher-profiles?keyword=%q khớp %d hồ sơ theo email, muốn 0", kw, n)
		}
	}
	// Tìm theo tên / tên đăng nhập / chuyên môn vẫn chạy.
	if n := countTeachers("Van Giang " + tag); n != 1 {
		t.Errorf("/teachers theo họ tên: %d, muốn 1", n)
	}
	if n := countTeachers("gv_" + tag); n != 1 {
		t.Errorf("/teachers theo user_name: %d, muốn 1", n)
	}
	if n := countProfiles("gv_" + tag); n != 1 {
		t.Errorf("/teacher-profiles theo user_name: %d, muốn 1", n)
	}
	if n := countProfiles("Toan hoc " + tag); n != 1 {
		t.Errorf("/teacher-profiles theo chuyên môn: %d, muốn 1", n)
	}
}

// ── 2. Đánh giá khoá học ────────────────────────────────────────────────────

func requireAppErrStatus(t *testing.T, name string, err error, status int) {
	t.Helper()
	var known *apperr.KnownError
	if !errors.As(err, &known) || known.Status != status {
		t.Errorf("%s: err=%v, muốn lỗi nghiệp vụ HTTP %d", name, err, status)
	}
}

func TestS6_Review_ChiHocVienGhiDanhMoiDuocDanhGia(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	owner, stranger, learner, finisher, refunded := f.user("owner"), f.user("stranger"), f.user("learner"), f.user("finisher"), f.user("refunded")
	course := f.course(owner)
	now := time.Now()
	for _, e := range []model.Enrollment{
		{UserID: learner.ID, CourseID: course.ID, EnrolledAt: now},
		{UserID: finisher.ID, CourseID: course.ID, EnrolledAt: now, CompletedAt: &now},
		{UserID: refunded.ID, CourseID: course.ID, EnrolledAt: now},
	} {
		if err := f.db.Create(&e).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Hoàn tiền = soft-delete enrollment (admin_order_service.revokeEnrollmentsForRefund).
	if err := f.db.Where("user_id = ? AND course_id = ?", refunded.ID, course.ID).Delete(&model.Enrollment{}).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewReviewService(repository.NewReviewRepository(f.db), repository.NewCourseRepository(f.db), repository.NewEnrollmentRepository(f.db), nil)
	req := dto.CreateReviewDTO{Rating: 5, Comment: "Tuyet voi"}

	for name, u := range map[string]model.User{"người chưa ghi danh": stranger, "chủ khoá (không ghi danh)": owner, "đơn đã hoàn tiền": refunded} {
		got, err := svc.CreateReview(ctx, u.ID, course.ID, req)
		if got != nil {
			t.Errorf("%s: vẫn tạo được đánh giá", name)
		}
		requireAppErrStatus(t, name, err, http.StatusForbidden)
	}
	var n int64
	f.db.Model(&model.Review{}).Where("course_id = ?", course.ID).Count(&n)
	if n != 0 {
		t.Fatalf("có %d đánh giá của người không đủ điều kiện", n)
	}

	for name, u := range map[string]model.User{"học viên đang học": learner, "học viên đã hoàn thành": finisher} {
		if got, err := svc.CreateReview(ctx, u.ID, course.ID, req); err != nil || got == nil {
			t.Errorf("%s bị chặn nhầm: err=%v", name, err)
		}
		// Mỗi người một đánh giá cho mỗi khoá.
		_, err := svc.CreateReview(ctx, u.ID, course.ID, req)
		requireAppErrStatus(t, name+" đánh giá lần 2", err, http.StatusConflict)
	}
	f.db.Model(&model.Review{}).Where("course_id = ?", course.ID).Count(&n)
	if n != 2 {
		t.Errorf("số đánh giá = %d, muốn 2 (mỗi học viên đủ điều kiện đúng 1)", n)
	}
}

// Hai request đồng thời cùng qua bước "đã đánh giá chưa": chỉ mục duy nhất (user_id, course_id) giữ đúng 1 dòng
// và request thua nhận 409, không phải 500.
func TestS6_Review_DongThoiChiMotDanhGia(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	owner, learner := f.user("owner"), f.user("learner")
	course := f.course(owner)
	if err := f.db.Create(&model.Enrollment{UserID: learner.ID, CourseID: course.ID, EnrolledAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewReviewService(repository.NewReviewRepository(f.db), repository.NewCourseRepository(f.db), repository.NewEnrollmentRepository(f.db), nil)

	const workers = 6
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := svc.CreateReview(ctx, learner.ID, course.ID, dto.CreateReviewDTO{Rating: 4})
			errs <- err
		}()
	}
	ok := 0
	for i := 0; i < workers; i++ {
		err := <-errs
		if err == nil {
			ok++
			continue
		}
		requireAppErrStatus(t, "request thua", err, http.StatusConflict)
	}
	if ok != 1 {
		t.Errorf("%d request tạo được đánh giá, muốn đúng 1", ok)
	}
}

// staleReviewRepo giả lập cuộc đua: lần đọc "đã đánh giá chưa" trả nil (đọc trước khi request kia commit) trong
// khi dòng đã tồn tại, nên chỉ chỉ mục duy nhất mới chặn được. Test dồn 6 goroutine không đáng tin để chạm đường này.
type staleReviewRepo struct{ repository.ReviewRepositoryInterface }

func (staleReviewRepo) GetReviewByUserAndCourse(context.Context, uuid.UUID, uuid.UUID) (*model.Review, error) {
	return nil, nil
}

func TestS6_Review_DuplicateKeyTraThanh409(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	owner, learner := f.user("owner"), f.user("learner")
	course := f.course(owner)
	if err := f.db.Create(&model.Enrollment{UserID: learner.ID, CourseID: course.ID, EnrolledAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	real := repository.NewReviewRepository(f.db)
	svc := NewReviewService(staleReviewRepo{real}, repository.NewCourseRepository(f.db), repository.NewEnrollmentRepository(f.db), nil)
	if _, err := svc.CreateReview(ctx, learner.ID, course.ID, dto.CreateReviewDTO{Rating: 4}); err != nil {
		t.Fatalf("đánh giá đầu: %v", err)
	}
	_, err := svc.CreateReview(ctx, learner.ID, course.ID, dto.CreateReviewDTO{Rating: 5})
	requireAppErrStatus(t, "đánh giá thứ hai (đọc cũ)", err, http.StatusConflict)
}
// ── 3. Hồ sơ công khai theo cài đặt riêng tư ────────────────────────────────

func TestS6_PublicProfile_TheoCaiDatRiengTu(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	viewer, admin := f.user("viewer"), f.user("admin")
	svc := NewUserStatsService(repository.NewUserStatsRepository(f.db), repository.NewUserPreferenceRepository(f.db))

	bio := "Tiểu sử riêng tư"
	full := "Le Thi Rieng Tu"
	mk := func(visibility string) model.User {
		u := f.user("owner-" + visibility)
		if err := f.db.Model(&u).Updates(map[string]interface{}{"bio": bio, "full_name": full}).Error; err != nil {
			t.Fatal(err)
		}
		if visibility != "" {
			if err := f.db.Create(&model.UserPreference{UserID: u.ID, ProfileVisibility: visibility}).Error; err != nil {
				t.Fatal(err)
			}
		}
		return u
	}
	ownerOf := map[string]model.User{}
	for _, v := range []string{"", "public", "friends", "private", "hidden", "gia-tri-la"} {
		ownerOf[v] = mk(v)
	}

	type who struct {
		name   string
		id     *uuid.UUID
		admin  bool
		isSelf bool
	}
	viewers := func(owner model.User) []who {
		return []who{
			{"khách", nil, false, false},
			{"người dùng khác", &viewer.ID, false, false},
			{"chính chủ", &owner.ID, false, true},
			{"admin", &admin.ID, true, false},
		}
	}

	for _, vis := range []string{"", "public"} { // chưa có dòng cài đặt = công khai
		owner := ownerOf[vis]
		for _, w := range viewers(owner) {
			got, err := svc.GetPublicProfile(ctx, owner.ID, w.id, w.admin)
			if err != nil || got == nil || got.IsPrivate || got.Bio == nil || *got.Bio != bio {
				t.Errorf("visibility=%q %s: err=%v got=%+v, muốn hồ sơ đầy đủ", vis, w.name, err, got)
			}
		}
	}

	// private, friends (chưa có tính năng bạn bè) và giá trị lạ: người khác chỉ thấy tên + avatar.
	for _, vis := range []string{"private", "friends", "gia-tri-la"} {
		owner := ownerOf[vis]
		for _, w := range viewers(owner) {
			got, err := svc.GetPublicProfile(ctx, owner.ID, w.id, w.admin)
			if err != nil || got == nil {
				t.Errorf("visibility=%q %s: err=%v", vis, w.name, err)
				continue
			}
			if w.isSelf || w.admin {
				if got.IsPrivate || got.Bio == nil {
					t.Errorf("visibility=%q %s phải thấy đầy đủ: %+v", vis, w.name, got)
				}
				continue
			}
			if !got.IsPrivate || got.UserName != owner.UserName || got.FullName == nil || *got.FullName != full {
				t.Errorf("visibility=%q %s: phải thấy tên: %+v", vis, w.name, got)
			}
			if got.Bio != nil || got.Stats.TotalPoints != 0 || got.Stats.LessonsCompleted != 0 || !got.JoinedAt.IsZero() ||
				len(got.FeaturedAchievements) != 0 || len(got.Activity) != 0 || len(got.CompletedCourses) != 0 {
				t.Errorf("visibility=%q %s: lộ dữ liệu riêng tư: %+v", vis, w.name, got)
			}
		}
	}

	// hidden: người khác 404, chính chủ và admin vẫn xem được.
	hidden := ownerOf["hidden"]
	for _, w := range viewers(hidden) {
		got, err := svc.GetPublicProfile(ctx, hidden.ID, w.id, w.admin)
		if w.isSelf || w.admin {
			if err != nil || got == nil || got.IsPrivate {
				t.Errorf("hidden %s: err=%v got=%+v, muốn thấy đầy đủ", w.name, err, got)
			}
			continue
		}
		if !errors.Is(err, ErrPublicProfileNotFound) || got != nil {
			t.Errorf("hidden %s: err=%v got=%+v, muốn ErrPublicProfileNotFound", w.name, err, got)
		}
	}

	// Người dùng không tồn tại: vẫn 404.
	if _, err := svc.GetPublicProfile(ctx, uuid.New(), nil, false); !errors.Is(err, ErrPublicProfileNotFound) {
		t.Errorf("người dùng không tồn tại: err=%v", err)
	}
}
