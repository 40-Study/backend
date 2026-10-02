package service

// Lane L2 (Postgres thật, schema tạm): chủ/quản trị tổ chức được chấm điểm lớp của tổ chức, có lưu người
// chấm (graded_by + graded_by_name), và cài đặt riêng tư bảng xếp hạng lưu rồi tải lại vẫn giữ.
//
// MỘT fixture cho cả nhóm (mỗi fixture là một lần Migrate đầy đủ, vài chục giây trên CI; package service đã
// sát giới hạn 10 phút của `go test`), các ca con dùng lớp riêng nên không ảnh hưởng nhau.
//
// Test đi theo từng vai:
//   - giảng viên được gán vào lớp, instructor chủ khoá (nhóm vốn đã hoặc nay được chấm),
//   - chủ tổ chức A (ORG_MEMBERS_MANAGE trong org role đang active) của lớp thuộc tổ chức A: được chấm,
//   - thành viên tổ chức A không có quyền quản trị, chủ tổ chức B, chủ tổ chức A đã bị gỡ role,
//     chủ tổ chức A với lớp KHÔNG thuộc tổ chức A, người lạ: không xem được lớp -> 404,
//   - học viên trong lớp: xem được nhưng không chấm -> 403,
//   - admin hệ thống: được chấm.
// Bỏ nhánh orgManagesClass trong ensureClassGrade thì "chủ tổ chức A" ĐỎ; đổi 404 thành 403 (hoặc ngược lại)
// thì các vai không-xem-được / học viên ĐỎ; bỏ Omit(clause.Associations) ở UpdateGrade thì graded_by sau
// khi chủ tổ chức sửa điểm vẫn là giảng viên cũ và test ĐỎ.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/data"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type gradeOrgEnv struct {
	*s6OrgEnv
	svc                                                 *GradeService
	instructor, coTeacher, student, stranger            model.User
	ownerA, memberA, ownerB, revokedOwnerA, systemAdmin model.User
}

func mustCreate(t *testing.T, db *gorm.DB, v any) {
	t.Helper()
	if err := db.Create(v).Error; err != nil {
		t.Fatal(err)
	}
}

func newGradeOrgEnv(t *testing.T) *gradeOrgEnv {
	t.Helper()
	e := &gradeOrgEnv{s6OrgEnv: newS6OrgEnv(t)}
	for name, u := range map[string]*model.User{
		"instructor": &e.instructor, "co-teacher": &e.coTeacher, "student": &e.student, "stranger": &e.stranger,
		"owner-a": &e.ownerA, "member-a": &e.memberA, "owner-b": &e.ownerB, "revoked-owner-a": &e.revokedOwnerA,
		"system-admin": &e.systemAdmin,
	} {
		*u = e.user(name)
	}

	// Instructor chủ khoá là thành viên active của tổ chức A (vai không mang quyền nào). Việc là thành viên KHÔNG
	// kéo lớp của họ vào phạm vi tổ chức: chỉ lớp có classes.organization_id = A mới thuộc quyền chủ tổ chức A.
	e.grant(e.instructor, e.orgA, e.orgRole(e.orgA, "GIANG_VIEN"))
	e.grant(e.ownerA, e.orgA, e.orgRole(e.orgA, "ORG_OWNER", "ORG_MEMBERS_MANAGE", "ORG_ROLES_MANAGE"))
	e.grant(e.memberA, e.orgA, e.orgRole(e.orgA, "THANH_VIEN"))
	e.grant(e.ownerB, e.orgB, e.orgRole(e.orgB, "ORG_OWNER", "ORG_MEMBERS_MANAGE"))
	e.grant(e.revokedOwnerA, e.orgA, e.orgRole(e.orgA, "ORG_OWNER", "ORG_MEMBERS_MANAGE"))
	if err := e.db.Model(&model.UserOrganizationRole{}).Where("user_id = ?", e.revokedOwnerA.ID).
		Update("status", model.UserOrgRoleStatusInactive).Error; err != nil {
		t.Fatal(err)
	}

	sys := model.SystemRole{Name: "L2_SYSTEM_ADMIN", Status: "active"}
	mustCreate(t, e.db, &sys)
	mustCreate(t, e.db, &model.SystemRolePermission{SystemRoleID: sys.ID, PermissionID: e.permIDs["SYSTEM_SETTINGS_MANAGE"]})
	mustCreate(t, e.db, &model.UserSystemRole{UserID: e.systemAdmin.ID, SystemRoleID: sys.ID, Status: model.UserSystemRoleStatusActive})

	e.svc = e.newGradeService(e.checker)
	return e
}

func (e *gradeOrgEnv) newGradeService(authz ClassAuthorizer) *GradeService {
	return NewGradeService(repository.NewGradeRepository(e.db), repository.NewClassRepository(e.db),
		repository.NewCourseRepository(e.db), authz, nil)
}

// classInOrgA: lớp tạo trong tổ chức A (organization_id = A) của khoá do instructor làm chủ; coTeacher được gán,
// student đang học.
func (e *gradeOrgEnv) classInOrgA(t *testing.T) model.Class {
	t.Helper()
	course := e.course(e.instructor)
	class := model.Class{Name: "L2 grade " + uuid.NewString()[:6], CourseID: &course.ID, Status: "active", OrganizationID: &e.orgA.ID}
	mustCreate(t, e.db, &class)
	mustCreate(t, e.db, &model.TeacherClass{TeacherID: e.coTeacher.ID, ClassID: class.ID, Role: "primary"})
	mustCreate(t, e.db, &model.StudentClass{StudentID: e.student.ID, ClassID: class.ID, Status: "active"})
	return class
}

// personalClassOfOrgMember: lớp CÁ NHÂN (organization_id NULL) của instructor, người là thành viên active của tổ chức A.
func (e *gradeOrgEnv) personalClassOfOrgMember(t *testing.T) model.Class {
	t.Helper()
	course := e.course(e.instructor)
	class := model.Class{Name: "L2 personal " + uuid.NewString()[:6], CourseID: &course.ID, Status: "active"}
	mustCreate(t, e.db, &class)
	mustCreate(t, e.db, &model.StudentClass{StudentID: e.student.ID, ClassID: class.ID, Status: "active"})
	return class
}

// classInOrgB: lớp tạo trong tổ chức B.
func (e *gradeOrgEnv) classInOrgB(t *testing.T) model.Class {
	t.Helper()
	course := e.course(e.stranger)
	class := model.Class{Name: "L2 orgB " + uuid.NewString()[:6], CourseID: &course.ID, Status: "active", OrganizationID: &e.orgB.ID}
	mustCreate(t, e.db, &class)
	mustCreate(t, e.db, &model.StudentClass{StudentID: e.student.ID, ClassID: class.ID, Status: "active"})
	return class
}

// classOutsideOrgA: lớp cá nhân của người lạ, không dính tổ chức nào.
func (e *gradeOrgEnv) classOutsideOrgA(t *testing.T) model.Class {
	t.Helper()
	course := e.course(e.stranger)
	class := model.Class{Name: "L2 foreign " + uuid.NewString()[:6], CourseID: &course.ID, Status: "active"}
	mustCreate(t, e.db, &class)
	mustCreate(t, e.db, &model.StudentClass{StudentID: e.student.ID, ClassID: class.ID, Status: "active"})
	return class
}

func (e *gradeOrgEnv) gradeDTO() dto.CreateGradeDTO {
	return dto.CreateGradeDTO{StudentID: e.student.ID.String(), GradeType: "assignment", Title: "Bài 1", Score: 8, MaxScore: 10}
}

func (e *gradeOrgEnv) gradeRows(classID uuid.UUID) int64 {
	var n int64
	e.db.Model(&model.Grade{}).Where("class_id = ?", classID).Count(&n)
	return n
}

func TestL2_ClassLane_Postgres(t *testing.T) {
	e := newGradeOrgEnv(t)
	ctx := context.Background()

	t.Run("quyền chấm điểm theo vai", func(t *testing.T) {
		class, foreign := e.classInOrgA(t), e.classOutsideOrgA(t)
		personal, inB := e.personalClassOfOrgMember(t), e.classInOrgB(t)
		cases := []struct {
			name    string
			actor   model.User
			classID uuid.UUID
			want    error // nil = được chấm
		}{
			{"giảng viên được gán vào lớp", e.coTeacher, class.ID, nil},
			{"instructor chủ khoá", e.instructor, class.ID, nil},
			{"chủ tổ chức A, lớp thuộc A", e.ownerA, class.ID, nil},
			{"admin hệ thống", e.systemAdmin, class.ID, nil},
			{"thành viên tổ chức A không có quyền quản trị", e.memberA, class.ID, ErrClassNotFound},
			{"chủ tổ chức B", e.ownerB, class.ID, ErrClassNotFound},
			{"chủ tổ chức A đã bị gỡ role", e.revokedOwnerA, class.ID, ErrClassNotFound},
			{"chủ tổ chức A, lớp KHÔNG thuộc A", e.ownerA, foreign.ID, ErrClassNotFound},
			// Chủ tổ chức thêm giảng viên vào tổ chức rồi cố chấm lớp CÁ NHÂN của người đó: bị chặn.
			{"chủ tổ chức A, lớp cá nhân của giảng viên thành viên A", e.ownerA, personal.ID, ErrClassNotFound},
			{"chủ tổ chức A, lớp thuộc tổ chức B", e.ownerA, inB.ID, ErrClassNotFound},
			{"chủ tổ chức B, lớp thuộc tổ chức B", e.ownerB, inB.ID, nil},
			{"người lạ", e.stranger, class.ID, ErrClassNotFound},
			{"học viên trong lớp", e.student, class.ID, ErrNotClassTeacher},
			{"lớp không tồn tại", e.ownerA, uuid.New(), ErrClassNotFound},
		}
		for _, c := range cases {
			t.Run("chấm điểm: "+c.name, func(t *testing.T) {
				got, err := e.svc.CreateGrade(ctx, c.classID, c.actor.ID, e.gradeDTO())
				if c.want == nil {
					if err != nil {
						t.Fatalf("bị chặn nhầm: %v", err)
					}
					if got.GradedBy != c.actor.ID {
						t.Errorf("graded_by=%v, muốn %v", got.GradedBy, c.actor.ID)
					}
					if got.GradedByName != c.actor.UserName {
						t.Errorf("graded_by_name=%q, muốn %q", got.GradedByName, c.actor.UserName)
					}
					return
				}
				if !errors.Is(err, c.want) {
					t.Fatalf("err=%v, muốn %v", err, c.want)
				}
			})
			t.Run("xem bảng điểm: "+c.name, func(t *testing.T) {
				if _, err := e.svc.GetGradesByClass(ctx, c.classID, c.actor.ID); !errors.Is(err, c.want) {
					t.Fatalf("err=%v, muốn %v", err, c.want)
				}
			})
		}
		// Chỉ các lần chấm được phép để lại dòng điểm: 4 vai được chấm, đúng 4 dòng; lớp ngoài tổ chức không có dòng nào.
		// Lớp thuộc B: chỉ chủ tổ chức B chấm được (1 dòng); lớp cá nhân: không ai trong số này.
		if n := e.gradeRows(inB.ID); n != 1 {
			t.Errorf("lớp của tổ chức B có %d dòng điểm, muốn 1", n)
		}
		if n := e.gradeRows(personal.ID); n != 0 {
			t.Errorf("lớp cá nhân có %d dòng điểm, muốn 0", n)
		}
		if n := e.gradeRows(class.ID); n != 4 {
			t.Errorf("có %d dòng điểm, muốn 4 (chỉ người được phép chấm)", n)
		}
		if n := e.gradeRows(foreign.ID); n != 0 {
			t.Errorf("lớp ngoài tổ chức A có %d dòng điểm, muốn 0", n)
		}
	})

	// Chủ tổ chức sửa điểm do giảng viên chấm: graded_by đổi sang chủ tổ chức (GORM từng ghi đè lại bằng id người
	// chấm cũ vì Grader đã Preload), tên trong response và ở danh sách là người chấm mới. Học viên đọc thấy cùng tên.
	t.Run("chủ tổ chức sửa điểm ghi người chấm", func(t *testing.T) {
		class := e.classInOrgA(t)
		g, err := e.svc.CreateGrade(ctx, class.ID, e.coTeacher.ID, e.gradeDTO())
		if err != nil {
			t.Fatal(err)
		}
		if g.GradedBy != e.coTeacher.ID || g.GradedByName != e.coTeacher.UserName {
			t.Fatalf("giảng viên chấm: graded_by=%v tên=%q", g.GradedBy, g.GradedByName)
		}

		score := 9.5
		up, err := e.svc.UpdateGrade(ctx, g.ID, e.ownerA.ID, dto.UpdateGradeDTO{Score: &score})
		if err != nil {
			t.Fatalf("chủ tổ chức sửa điểm: %v", err)
		}
		if up.GradedBy != e.ownerA.ID || up.GradedByName != e.ownerA.UserName {
			t.Errorf("response sau sửa: graded_by=%v tên=%q, muốn chủ tổ chức", up.GradedBy, up.GradedByName)
		}
		var stored model.Grade
		if err := e.db.First(&stored, "id = ?", g.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.GradedBy != e.ownerA.ID {
			t.Errorf("DB graded_by=%v, muốn chủ tổ chức %v", stored.GradedBy, e.ownerA.ID)
		}

		book, err := e.svc.GetGradesByClass(ctx, class.ID, e.ownerA.ID)
		if err != nil || len(book.Students) != 1 || len(book.Students[0].Grades) != 1 {
			t.Fatalf("bảng điểm: %+v err=%v", book, err)
		}
		if name := book.Students[0].Grades[0].GradedByName; name != e.ownerA.UserName {
			t.Errorf("bảng điểm lớp: graded_by_name=%q, muốn %q", name, e.ownerA.UserName)
		}
		mine, err := e.svc.GetMyGradesByClass(ctx, e.student.ID, class.ID)
		if err != nil || len(mine) != 1 || mine[0].GradedByName != e.ownerA.UserName {
			t.Errorf("điểm của học viên ở lớp: %+v err=%v, muốn graded_by_name=%q", mine, err, e.ownerA.UserName)
		}

		// Học viên không sửa/xoá được điểm của chính mình (403), người lạ nhận 404.
		if _, err := e.svc.UpdateGrade(ctx, g.ID, e.student.ID, dto.UpdateGradeDTO{Score: &score}); !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("học viên sửa điểm: err=%v, muốn ErrNotClassTeacher", err)
		}
		if err := e.svc.DeleteGrade(ctx, g.ID, e.stranger.ID); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("người lạ xoá điểm: err=%v, muốn ErrClassNotFound", err)
		}
		if err := e.svc.DeleteGrade(ctx, g.ID, e.ownerA.ID); err != nil {
			t.Errorf("chủ tổ chức xoá điểm: %v", err)
		}
	})

	// Chấm hàng loạt: cả lô theo cùng luật quyền (chủ tổ chức được, người ngoài/học viên bị chặn trước khi ghi).
	t.Run("chấm hàng loạt", func(t *testing.T) {
		class := e.classInOrgA(t)
		req := dto.BulkCreateGradesDTO{Grades: []dto.CreateGradeDTO{e.gradeDTO(), e.gradeDTO()}}
		if out, err := e.svc.BulkCreateGrades(ctx, class.ID, e.ownerA.ID, req); err != nil || len(out) != 2 || out[0].GradedBy != e.ownerA.ID {
			t.Fatalf("chủ tổ chức chấm lô: %v err=%v", out, err)
		}
		if _, err := e.svc.BulkCreateGrades(ctx, class.ID, e.ownerB.ID, req); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("chủ tổ chức B chấm lô: err=%v, muốn ErrClassNotFound", err)
		}
		if _, err := e.svc.BulkCreateGrades(ctx, class.ID, e.student.ID, req); !errors.Is(err, ErrNotClassTeacher) {
			t.Errorf("học viên chấm lô: err=%v, muốn ErrNotClassTeacher", err)
		}
		if n := e.gradeRows(class.ID); n != 2 {
			t.Errorf("có %d dòng điểm, muốn 2 (lô bị từ chối không ghi gì)", n)
		}
	})

	// Không có Authorizer (nil): chỉ người quản lý lớp theo dữ liệu lớp/khoá chấm được; chủ tổ chức không tự có quyền.
	t.Run("không có authorizer thì fail-closed", func(t *testing.T) {
		class := e.classInOrgA(t)
		svc := e.newGradeService(nil)
		if _, err := svc.CreateGrade(ctx, class.ID, e.ownerA.ID, e.gradeDTO()); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("chủ tổ chức khi không có authorizer: err=%v, muốn ErrClassNotFound", err)
		}
		if _, err := svc.CreateGrade(ctx, class.ID, e.coTeacher.ID, e.gradeDTO()); err != nil {
			t.Errorf("giảng viên lớp bị chặn nhầm: %v", err)
		}
	})

	// Tạo lớp trong tổ chức: chỉ thành viên active của tổ chức đó mới gắn được organization_id; lớp cá nhân để NULL;
	// không có backfill/suy luận.
	t.Run("tạo lớp trong tổ chức", func(t *testing.T) {
		svc := NewClassService(repository.NewClassRepository(e.db), repository.NewCourseRepository(e.db),
			s4TeacherRepo{teachers: map[uuid.UUID]bool{e.instructor.ID: true, e.stranger.ID: true}}, nil, nil)
		stored := func(id uuid.UUID) *uuid.UUID {
			var c model.Class
			if err := e.db.First(&c, "id = ?", id).Error; err != nil {
				t.Fatal(err)
			}
			return c.OrganizationID
		}
		in, err := svc.CreateClass(ctx, e.instructor.ID, false, dto.CreateClassDTO{Name: "Lớp tổ chức A", OrganizationID: &e.orgA.ID})
		if err != nil {
			t.Fatalf("thành viên A tạo lớp trong A: %v", err)
		}
		if got := stored(in.ID); got == nil || *got != e.orgA.ID || in.OrganizationID == nil || *in.OrganizationID != e.orgA.ID {
			t.Errorf("organization_id lưu=%v, response=%v, muốn %v", got, in.OrganizationID, e.orgA.ID)
		}
		personal, err := svc.CreateClass(ctx, e.instructor.ID, false, dto.CreateClassDTO{Name: "Lớp cá nhân"})
		if err != nil || stored(personal.ID) != nil || personal.OrganizationID != nil {
			t.Errorf("lớp cá nhân: err=%v, organization_id phải NULL", err)
		}
		// Thành viên của A không gắn lớp vào B; người lạ không gắn vào A; role đã bị gỡ thì không còn là thành viên.
		if _, err := svc.CreateClass(ctx, e.instructor.ID, false, dto.CreateClassDTO{Name: "x", OrganizationID: &e.orgB.ID}); !errors.Is(err, ErrNotOrgMember) {
			t.Errorf("tạo lớp vào tổ chức không phải thành viên: err=%v, muốn ErrNotOrgMember", err)
		}
		if _, err := svc.CreateClass(ctx, e.stranger.ID, false, dto.CreateClassDTO{Name: "x", OrganizationID: &e.orgA.ID}); !errors.Is(err, ErrNotOrgMember) {
			t.Errorf("người lạ tạo lớp vào A: err=%v, muốn ErrNotOrgMember", err)
		}
		// Admin hệ thống: tổ chức phải tồn tại.
		if _, err := svc.CreateClass(ctx, e.systemAdmin.ID, true, dto.CreateClassDTO{Name: "x", OrganizationID: &e.orgB.ID}); err != nil {
			t.Errorf("admin tạo lớp vào tổ chức có thật: %v", err)
		}
		missing := uuid.New()
		if _, err := svc.CreateClass(ctx, e.systemAdmin.ID, true, dto.CreateClassDTO{Name: "x", OrganizationID: &missing}); err == nil {
			t.Error("admin tạo lớp vào tổ chức không tồn tại phải lỗi")
		}
		// Chủ tổ chức A thêm instructor vào A rồi KHÔNG thấy lớp cá nhân của họ: vẫn 404 (xem matrix ở trên).
		if _, err := NewClassService(repository.NewClassRepository(e.db), repository.NewCourseRepository(e.db), nil, nil, nil).
			GetClassByID(ctx, personal.ID, e.ownerA.ID, false); !errors.Is(err, ErrClassNotFound) {
			t.Errorf("chủ tổ chức xem lớp cá nhân: err=%v, muốn ErrClassNotFound", err)
		}
	})

	// Cài đặt riêng tư bảng xếp hạng: backend là SSOT của tên field `leaderboard_display`. Web từng gửi
	// `leaderboard_visibility`, backend bỏ qua khoá lạ nên lựa chọn không bao giờ được lưu. Lưu xong, tải lại
	// (service mới, đọc từ DB) vẫn giữ giá trị.
	t.Run("riêng tư bảng xếp hạng: lưu rồi tải lại vẫn giữ", func(t *testing.T) {
		u := e.user("privacy")
		newSvc := func() *UserPreferenceService {
			return NewUserPreferenceService(repository.NewUserPreferenceRepository(e.db))
		}

		if got, err := newSvc().GetPrivacySettings(u.ID); err != nil || got.LeaderboardDisplay != "name" {
			t.Fatalf("mặc định: %+v err=%v, muốn leaderboard_display=name", got, err)
		}
		for _, want := range []string{"anonymous", "username", "name"} {
			var req dto.UpdatePrivacySettingsDTO
			if err := json.Unmarshal([]byte(`{"leaderboard_display":"`+want+`"}`), &req); err != nil {
				t.Fatal(err)
			}
			if _, err := newSvc().UpdatePrivacySettings(u.ID, req); err != nil {
				t.Fatalf("lưu %s: %v", want, err)
			}
			if got, err := newSvc().GetPrivacySettings(u.ID); err != nil || got.LeaderboardDisplay != want {
				t.Errorf("sau khi lưu %q và tải lại: %+v err=%v", want, got, err)
			}
		}
	})
}

// Quyền dùng để xác định "chủ/quản trị tổ chức" phải thuộc phạm vi tổ chức, nếu không bộ lọc S6 ở
// PermissionChecker sẽ loại nó khỏi org role và chủ tổ chức mất quyền chấm một cách im lặng.
func TestL2_OrgClassManagePermission_ThuocPhamViToChuc(t *testing.T) {
	if !data.IsOrgPermission(orgClassManagePermission) {
		t.Fatalf("%s không nằm trong data/permissions/org_owner_permissions.json", orgClassManagePermission)
	}
}

// Khoá `leaderboard_visibility` (tên web dùng trước đây) không phải field của backend: nó không được ghi vào
// `leaderboard_display`. Test này ghim hợp đồng để ai đó "thêm alias" phải chủ động đổi nó. Không cần DB.
func TestL2_PrivacySettings_TenFieldLaHopDong(t *testing.T) {
	var req dto.UpdatePrivacySettingsDTO
	if err := json.Unmarshal([]byte(`{"leaderboard_visibility":"anonymous"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.LeaderboardDisplay != nil {
		t.Fatalf("leaderboard_visibility không được map vào LeaderboardDisplay, thấy %q", *req.LeaderboardDisplay)
	}

	raw, err := json.Marshal(dto.PrivacySettingsResponseDTO{ProfileVisibility: "public", ActivityStatus: "everyone", LeaderboardDisplay: "name"})
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]string
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"profile_visibility", "activity_status", "leaderboard_display"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("response thiếu khoá %q: %s", k, raw)
		}
	}
	if _, ok := keys["leaderboard_visibility"]; ok {
		t.Errorf("response không được có khoá leaderboard_visibility: %s", raw)
	}
}
