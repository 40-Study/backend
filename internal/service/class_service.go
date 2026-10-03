package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

type ClassServiceInterface interface {
	// CreateClass (S4): chỉ admin, giảng viên chủ khoá (khi có course_id) hoặc giảng viên (khi không có khoá); người tạo thành chủ lớp.
	CreateClass(ctx context.Context, actorUserID uuid.UUID, isAdmin bool, req dto.CreateClassDTO) (*dto.ClassResponseDTO, error)
	GetAllClasses(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.ClassListResponseDTO, error)
	// GetClassByID (S4): chỉ thành viên lớp, người quản lý lớp và admin; người khác ErrClassNotFound (404).
	GetClassByID(ctx context.Context, id, actorUserID uuid.UUID, isAdmin bool) (*dto.ClassResponseDTO, error)
	UpdateClass(ctx context.Context, id, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateClassDTO) (*dto.ClassResponseDTO, error)
	DeleteClass(ctx context.Context, id, actorUserID uuid.UUID, isAdmin, hardDelete bool) error
	GetClassesByCourseID(ctx context.Context, courseID uuid.UUID) ([]dto.ClassResponseDTO, error)
	// GetOrganizationClasses (B-02): lớp của một tổ chức. Quyền theo tổ chức kiểm ở router (RequireOrgPermission), không kiểm ở đây.
	GetOrganizationClasses(ctx context.Context, orgID uuid.UUID, page, pageSize int, keyword, classStatus string) (*dto.ClassListResponseDTO, error)

	// Gán/gỡ giảng viên (S4): chỉ chủ lớp (người tạo hoặc chủ khoá) và admin. Người không xem được lớp: ErrClassNotFound; xem được nhưng không phải chủ: ErrNotClassOwner.
	AssignTeacherToClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, req dto.AssignTeacherDTO) (*dto.TeacherClassResponseDTO, error)
	AssignTeachersToClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, req dto.AssignTeachersDTO) ([]dto.TeacherClassResponseDTO, error)
	RemoveTeacherFromClass(ctx context.Context, classID, teacherID, actorUserID uuid.UUID, isAdmin bool) error
	GetTeachersByClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.TeacherClassListResponseDTO, error)

	EnrollStudentToClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, req dto.EnrollStudentDTO) (*dto.StudentClassResponseDTO, error)
	RemoveStudentFromClass(ctx context.Context, classID, studentID, actorUserID uuid.UUID, isAdmin bool) error
	GetStudentsByClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.StudentClassListResponseDTO, error)
	GetMyClasses(ctx context.Context, teacherID uuid.UUID) ([]dto.ClassResponseDTO, error)
}

// requireClassTeacherOrAdmin (H-11, audit 260909 vòng 2): UpdateClass/DeleteClass/
// EnrollStudentToClass/RemoveStudentFromClass/GetStudentsByClass trước đây không kiểm tra
// actor có liên quan gì tới lớp không — bất kỳ user đăng nhập nào cũng sửa/xóa lớp, thêm/xóa
// học sinh, hoặc xem danh sách học sinh (rò rỉ email/tên) của LỚP BẤT KỲ.
//
// isAdmin được tính sẵn ở tầng handler (qua middleware.PermissionChecker, permission
// "SYSTEM_SETTINGS_MANAGE") rồi truyền xuống — ClassService không tiêm PermissionChecker
// trực tiếp vì đã có 2 instance ClassService khác nhau được khởi tạo trong app/services.go
// (một cho teacherSvc, một cho Services.Class); tiêm permChecker vào constructor sẽ phải sửa
// cả 2 nơi trong khi chỉ Services.Class thực sự cần dùng.
func (s *ClassService) requireClassTeacherOrAdmin(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool) error {
	// S4: một định nghĩa "quản lý lớp" duy nhất (class_access.go): giảng viên lớp, chủ khoá, người
	// tạo lớp, admin. Trước đây chỉ tra teacher_classes nên chủ khoá không sửa được lớp của khoá mình.
	// B-12: cộng chủ/quản trị tổ chức của lớp (classAccessAsAdmin).
	elevated, err := s.accessAsAdmin(ctx, classID, actorUserID, isAdmin)
	if err != nil {
		return err
	}
	return ensureClassManage(ctx, s.classRepo, s.courseRepo, actorUserID, classID, elevated)
}

// accessAsAdmin: isAdmin nâng lên cho chủ/quản trị tổ chức của lớp, chỉ để kiểm quyền truy cập lớp.
func (s *ClassService) accessAsAdmin(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool) (bool, error) {
	return classAccessAsAdmin(ctx, s.classRepo, s.authz, actorUserID, classID, isAdmin)
}

// ensureVisible / ensureOwner: ensureClassVisible / ensureClassOwner với quyền đã nâng cho chủ tổ chức của lớp.
func (s *ClassService) ensureVisible(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool) error {
	elevated, err := s.accessAsAdmin(ctx, classID, actorUserID, isAdmin)
	if err != nil {
		return err
	}
	return ensureClassVisible(ctx, s.classRepo, s.courseRepo, actorUserID, classID, elevated)
}

func (s *ClassService) ensureOwner(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool) error {
	elevated, err := s.accessAsAdmin(ctx, classID, actorUserID, isAdmin)
	if err != nil {
		return err
	}
	return ensureClassOwner(ctx, s.classRepo, s.courseRepo, actorUserID, classID, elevated)
}

// WithAuthorizer gắn PermissionChecker để chủ/quản trị tổ chức quản lý được lớp của tổ chức mình (B-05/B-12).
// Không gắn thì không ai được nâng quyền (fail-closed).
func (s *ClassService) WithAuthorizer(authz ClassAuthorizer) *ClassService {
	s.authz = authz
	return s
}

// ErrNotOrgMember: tạo lớp trong một tổ chức mà người gọi không phải thành viên active (và không phải admin). Handler tra 403.
var ErrNotOrgMember = errors.New("forbidden: not an active member of this organization")

// ErrNotTeacher (S4): tạo lớp không gắn khoá mà người gọi không phải giảng viên (và không phải admin).
var ErrNotTeacher = errors.New("forbidden: only teachers can create classes")

type ClassService struct {
	classRepo         repository.ClassRepositoryInterface
	courseRepo        repository.CourseRepositoryInterface
	teacherRepo       repository.TeacherRepositoryInterface
	studentRepo       repository.StudentRepositoryInterface
	parentStudentRepo repository.ParentStudentRepositoryInterface
	authz             ClassAuthorizer
}

func NewClassService(
	classRepo repository.ClassRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	teacherRepo repository.TeacherRepositoryInterface,
	studentRepo repository.StudentRepositoryInterface,
	parentStudentRepo repository.ParentStudentRepositoryInterface,
) *ClassService {
	return &ClassService{
		classRepo:         classRepo,
		courseRepo:        courseRepo,
		teacherRepo:       teacherRepo,
		studentRepo:       studentRepo,
		parentStudentRepo: parentStudentRepo,
	}
}

func (s *ClassService) CreateClass(ctx context.Context, actorUserID uuid.UUID, isAdmin bool, req dto.CreateClassDTO) (*dto.ClassResponseDTO, error) {
	// Validate CourseID exists if provided
	if req.CourseID != nil {
		exists, err := s.courseRepo.Exists(ctx, *req.CourseID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("course not found")
		}
	}

	// S4: trước đây ai đăng nhập cũng tạo được lớp vào khoá của người khác. Lớp vào khoá phải do
	// chủ khoá (hoặc admin) tạo; lớp không gắn khoá phải do giảng viên (hoặc admin) tạo.
	if !isAdmin {
		if req.CourseID != nil {
			owner, err := courseInstructorOf(ctx, s.courseRepo, actorUserID, req.CourseID)
			if err != nil {
				return nil, err
			}
			if !owner {
				return nil, ErrNotCourseInstructor
			}
		} else {
			isTeacher, err := s.teacherRepo.Exists(ctx, actorUserID)
			if err != nil {
				return nil, err
			}
			if !isTeacher {
				return nil, ErrNotTeacher
			}
		}
	}

	// Lớp trong tổ chức: người tạo phải là thành viên active của tổ chức đó (admin hệ thống thì chỉ cần tổ chức
	// tồn tại). Lớp cá nhân (không gửi organization_id) để NULL. Không suy luận tổ chức từ nơi nào khác.
	if req.OrganizationID != nil {
		if isAdmin {
			exists, err := s.classRepo.OrganizationExists(ctx, *req.OrganizationID)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, errors.New("organization not found")
			}
		} else {
			member, err := s.classRepo.ActiveOrgMemberExists(ctx, actorUserID, *req.OrganizationID)
			if err != nil {
				return nil, err
			}
			if !member {
				return nil, ErrNotOrgMember
			}
		}
	}

	// Class luôn bắt đầu với status "draft".
	// Để chuyển sang "active" (lớp thật), cần gọi API Update với status = "active"
	// khi lớp đã sẵn sàng hoạt động (có đủ giáo viên, lịch học, etc.)
	class := &model.Class{
		Name:        req.Name,
		Description: req.Description,
		CourseID:    req.CourseID,
		Status:      "draft",
		MaxStudents: req.MaxStudents,
		CreatedBy:   &actorUserID,

		OrganizationID: req.OrganizationID,
	}

	if req.StartDate != nil {
		t, err := time.Parse("2006-01-02", *req.StartDate)
		if err != nil {
			return nil, errors.New("invalid start_date format, expected YYYY-MM-DD")
		}
		class.StartDate = &t
	}
	if req.EndDate != nil {
		t, err := time.Parse("2006-01-02", *req.EndDate)
		if err != nil {
			return nil, errors.New("invalid end_date format, expected YYYY-MM-DD")
		}
		class.EndDate = &t
	}

	if err := s.classRepo.Create(ctx, class); err != nil {
		return nil, err
	}

	return s.toClassResponseDTO(ctx, class), nil
}

func (s *ClassService) GetAllClasses(ctx context.Context, page, pageSize int, keyword string, status string) (*dto.ClassListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	classes, total, err := s.classRepo.GetAll(ctx, page, pageSize, keyword, status)
	if err != nil {
		return nil, err
	}

	classDTOs := make([]dto.ClassResponseDTO, len(classes))
	for i, c := range classes {
		classDTOs[i] = *s.toClassResponseDTO(ctx, &c)
	}

	return &dto.ClassListResponseDTO{
		Classes:  classDTOs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ClassService) GetOrganizationClasses(ctx context.Context, orgID uuid.UUID, page, pageSize int, keyword, classStatus string) (*dto.ClassListResponseDTO, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	classes, total, err := s.classRepo.GetByOrganization(ctx, orgID, page, pageSize, keyword, classStatus)
	if err != nil {
		return nil, err
	}
	classDTOs := make([]dto.ClassResponseDTO, len(classes))
	for i, c := range classes {
		classDTOs[i] = *s.toClassResponseDTO(ctx, &c)
	}
	return &dto.ClassListResponseDTO{Classes: classDTOs, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *ClassService) GetClassByID(ctx context.Context, id, actorUserID uuid.UUID, isAdmin bool) (*dto.ClassResponseDTO, error) {
	if err := s.ensureVisible(ctx, id, actorUserID, isAdmin); err != nil {
		return nil, err
	}
	class, err := s.classRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}
	return s.toClassResponseDTO(ctx, class), nil
}

func (s *ClassService) UpdateClass(ctx context.Context, id, actorUserID uuid.UUID, isAdmin bool, req dto.UpdateClassDTO) (*dto.ClassResponseDTO, error) {
	if err := s.requireClassTeacherOrAdmin(ctx, id, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	class, err := s.classRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	// Validate CourseID exists if being updated
	if req.CourseID != nil {
		exists, err := s.courseRepo.Exists(ctx, *req.CourseID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("course not found")
		}
		// S4: đổi khoá chuyển quyền sở hữu lớp (chủ khoá là chủ lớp), nên đòi CẢ HAI: người gọi là chủ lớp
		// hiện tại (người tạo, chủ khoá cũ, admin) VÀ là chủ khoá đích. Chỉ kiểm khoá đích thì giảng viên
		// được gán vào lớp vẫn kéo lớp khỏi khoá của chủ cũ sang khoá của chính mình. Đặt lại đúng khoá
		// hiện tại thì không đổi gì, không đòi thêm (form sửa lớp luôn gửi course_id).
		if !isAdmin && (class.CourseID == nil || *class.CourseID != *req.CourseID) {
			if err := ensureClassOwner(ctx, s.classRepo, s.courseRepo, actorUserID, id, false); err != nil {
				return nil, err
			}
			owner, err := courseInstructorOf(ctx, s.courseRepo, actorUserID, req.CourseID)
			if err != nil {
				return nil, err
			}
			if !owner {
				return nil, ErrNotCourseInstructor
			}
		}
		class.CourseID = req.CourseID
	}

	if req.Name != nil {
		class.Name = *req.Name
	}
	if req.Description != nil {
		class.Description = req.Description
	}
	if req.Status != nil {
		class.Status = *req.Status
	}
	if req.MaxStudents != nil {
		class.MaxStudents = req.MaxStudents
	}
	if req.StartDate != nil {
		t, err := time.Parse("2006-01-02", *req.StartDate)
		if err != nil {
			return nil, errors.New("invalid start_date format, expected YYYY-MM-DD")
		}
		class.StartDate = &t
	}
	if req.EndDate != nil {
		t, err := time.Parse("2006-01-02", *req.EndDate)
		if err != nil {
			return nil, errors.New("invalid end_date format, expected YYYY-MM-DD")
		}
		class.EndDate = &t
	}

	if err := s.classRepo.Update(ctx, class); err != nil {
		return nil, err
	}

	return s.toClassResponseDTO(ctx, class), nil
}

func (s *ClassService) DeleteClass(ctx context.Context, id, actorUserID uuid.UUID, isAdmin, hardDelete bool) error {
	if err := s.requireClassTeacherOrAdmin(ctx, id, actorUserID, isAdmin); err != nil {
		return err
	}

	class, err := s.classRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if class == nil {
		return errors.New("class not found")
	}
	return s.classRepo.Delete(ctx, id, hardDelete)
}

// Teacher-Class

// AssignTeacher assigns a single teacher to a class.
// Role can be:
// - "primary": Giáo viên chính, chịu trách nhiệm chính cho lớp học
// - "assistant": Trợ giảng, hỗ trợ giáo viên chính
func (s *ClassService) AssignTeacherToClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, req dto.AssignTeacherDTO) (*dto.TeacherClassResponseDTO, error) {
	// S4: chỉ chủ lớp/admin. Đây là cửa duy nhất để thành CanManage trên assignment của lớp.
	if err := s.ensureOwner(ctx, classID, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	// Check teacher exists
	exists, err := s.teacherRepo.Exists(ctx, req.TeacherID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("teacher not found")
	}

	// Check if teacher is already assigned to this class
	alreadyAssigned, err := s.classRepo.TeacherClassExists(ctx, classID, req.TeacherID)
	if err != nil {
		return nil, err
	}
	if alreadyAssigned {
		return nil, errors.New("teacher is already assigned to this class")
	}

	role := req.Role
	if role == "" {
		role = "primary"
	}

	tc := &model.TeacherClass{
		ID:        uuid.New(),
		TeacherID: req.TeacherID,
		ClassID:   classID,
		Role:      role,
	}

	if err := s.classRepo.AssignTeacher(ctx, tc); err != nil {
		return nil, err
	}

	return &dto.TeacherClassResponseDTO{
		ID:         tc.ID,
		TeacherID:  tc.TeacherID,
		ClassID:    tc.ClassID,
		Role:       tc.Role,
		AssignedAt: utils.FormatTimestamp(tc.AssignedAt),
	}, nil
}

// AssignTeachers assigns multiple teachers to a class at once
func (s *ClassService) AssignTeachersToClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, req dto.AssignTeachersDTO) ([]dto.TeacherClassResponseDTO, error) {
	if err := s.ensureOwner(ctx, classID, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	// Validate all teachers and check for duplicates
	tcs := make([]*model.TeacherClass, 0, len(req.Teachers))
	for _, t := range req.Teachers {
		// Check teacher exists
		exists, err := s.teacherRepo.Exists(ctx, t.TeacherID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, errors.New("teacher not found: " + t.TeacherID.String())
		}

		// Check if teacher is already assigned
		alreadyAssigned, err := s.classRepo.TeacherClassExists(ctx, classID, t.TeacherID)
		if err != nil {
			return nil, err
		}
		if alreadyAssigned {
			return nil, errors.New("teacher is already assigned to this class: " + t.TeacherID.String())
		}

		role := t.Role
		if role == "" {
			role = "primary"
		}

		tcs = append(tcs, &model.TeacherClass{
			ID:        uuid.New(),
			TeacherID: t.TeacherID,
			ClassID:   classID,
			Role:      role,
		})
	}

	if err := s.classRepo.AssignTeachers(ctx, tcs); err != nil {
		return nil, err
	}

	result := make([]dto.TeacherClassResponseDTO, len(tcs))
	for i, tc := range tcs {
		result[i] = dto.TeacherClassResponseDTO{
			ID:         tc.ID,
			TeacherID:  tc.TeacherID,
			ClassID:    tc.ClassID,
			Role:       tc.Role,
			AssignedAt: utils.FormatTimestamp(tc.AssignedAt),
		}
	}

	return result, nil
}

func (s *ClassService) RemoveTeacherFromClass(ctx context.Context, classID, teacherID, actorUserID uuid.UUID, isAdmin bool) error {
	if err := s.ensureOwner(ctx, classID, actorUserID, isAdmin); err != nil {
		return err
	}

	// Check teacher-class assignment exists
	exists, err := s.classRepo.TeacherClassExists(ctx, classID, teacherID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("teacher is not assigned to this class")
	}

	return s.classRepo.RemoveTeacher(ctx, classID, teacherID)
}

func (s *ClassService) GetTeachersByClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.TeacherClassListResponseDTO, error) {
	// S5 (review S4, M-7): danh sách giảng viên của lớp chỉ thành viên lớp, người quản lý lớp và admin; người khác 404.
	if err := s.ensureVisible(ctx, classID, actorUserID, isAdmin); err != nil {
		return nil, err
	}
	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	teachers, total, err := s.classRepo.GetTeachers(ctx, classID, page, pageSize)
	if err != nil {
		return nil, err
	}

	result := make([]dto.TeacherClassResponseDTO, len(teachers))
	for i, tc := range teachers {
		result[i] = dto.TeacherClassResponseDTO{
			ID:         tc.ID,
			TeacherID:  tc.TeacherID,
			ClassID:    tc.ClassID,
			Role:       tc.Role,
			AssignedAt: utils.FormatTimestamp(tc.AssignedAt),
			Teacher: &dto.TeacherResponseDTO{
				ID:       tc.Teacher.ID,
				UserName: tc.Teacher.UserName,
				FullName: tc.Teacher.FullName,
			},
		}
	}

	return &dto.TeacherClassListResponseDTO{
		Teachers: result,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// Student-Class

func (s *ClassService) EnrollStudentToClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, req dto.EnrollStudentDTO) (*dto.StudentClassResponseDTO, error) {
	if err := s.requireClassTeacherOrAdmin(ctx, classID, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	// Check student exists
	exists, err := s.studentRepo.Exists(ctx, req.StudentID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("student not found")
	}

	// Check if student is already enrolled
	alreadyEnrolled, err := s.classRepo.StudentClassExists(ctx, classID, req.StudentID)
	if err != nil {
		return nil, err
	}
	if alreadyEnrolled {
		return nil, errors.New("student is already enrolled in this class")
	}

	sc := &model.StudentClass{
		ID:        uuid.New(),
		StudentID: req.StudentID,
		ClassID:   classID,
		Status:    "active",
	}

	// Use EnrollStudentWithLock to prevent race condition when checking MaxStudents
	if err := s.classRepo.EnrollStudentWithLock(ctx, sc, class.MaxStudents); err != nil {
		return nil, err
	}

	return &dto.StudentClassResponseDTO{
		ID:         sc.ID,
		StudentID:  sc.StudentID,
		ClassID:    sc.ClassID,
		EnrolledAt: utils.FormatTimestamp(sc.EnrolledAt),
		Status:     sc.Status,
	}, nil
}

func (s *ClassService) RemoveStudentFromClass(ctx context.Context, classID, studentID, actorUserID uuid.UUID, isAdmin bool) error {
	if err := s.requireClassTeacherOrAdmin(ctx, classID, actorUserID, isAdmin); err != nil {
		return err
	}

	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return err
	}
	if class == nil {
		return errors.New("class not found")
	}

	// Check student-class enrollment exists
	exists, err := s.classRepo.StudentClassExists(ctx, classID, studentID)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("student is not enrolled in this class")
	}

	return s.classRepo.RemoveStudent(ctx, classID, studentID)
}

func (s *ClassService) GetStudentsByClass(ctx context.Context, classID, actorUserID uuid.UUID, isAdmin bool, page, pageSize int) (*dto.StudentClassListResponseDTO, error) {
	// H-11 + S4: danh sách học sinh chỉ cho thành viên lớp (học viên, giảng viên), người quản lý lớp và admin;
	// người khác nhận 404 (không dò được lớp nào tồn tại, không lấy được tên học sinh lớp khác).
	if err := s.ensureVisible(ctx, classID, actorUserID, isAdmin); err != nil {
		return nil, err
	}

	// Check class exists
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil {
		return nil, errors.New("class not found")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	students, total, err := s.classRepo.GetStudents(ctx, classID, page, pageSize)
	if err != nil {
		return nil, err
	}

	result := make([]dto.StudentClassResponseDTO, len(students))
	for i, sc := range students {
		result[i] = dto.StudentClassResponseDTO{
			ID:         sc.ID,
			StudentID:  sc.StudentID,
			ClassID:    sc.ClassID,
			EnrolledAt: utils.FormatTimestamp(sc.EnrolledAt),
			Status:     sc.Status,
			UserName:   sc.Student.UserName,
			FullName:   sc.Student.FullName,
			AvatarURL:  sc.Student.AvatarURL,
		}
	}

	return &dto.StudentClassListResponseDTO{
		Students: result,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *ClassService) GetMyClasses(ctx context.Context, teacherID uuid.UUID) ([]dto.ClassResponseDTO, error) {
	classes, err := s.classRepo.GetClassesByTeacher(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	result := make([]dto.ClassResponseDTO, len(classes))
	for i, c := range classes {
		result[i] = *s.toClassResponseDTO(ctx, &c)
	}
	return result, nil
}

func (s *ClassService) GetClassesByCourseID(ctx context.Context, courseID uuid.UUID) ([]dto.ClassResponseDTO, error) {
	classes, err := s.classRepo.GetByCourseID(ctx, courseID)
	if err != nil {
		return nil, err
	}
	result := make([]dto.ClassResponseDTO, len(classes))
	for i, c := range classes {
		result[i] = *s.toClassResponseDTO(ctx, &c)
	}
	return result, nil
}

// GetTeacherStudents (P1 QA 260927 teacher): chuyển sang TeacherService (dựa trên enrollment
// khoá học, không chỉ lớp) — xem teacher_service.go. Method này đã bị xoá khỏi đây vì không còn
// caller nào khác (grep xác nhận trước khi xoá) và bảng "Quản lý học viên" luôn trống với giáo
// viên chưa tạo lớp, dù khoá của họ đã có hàng nghìn học viên mua qua enrollment.

func (s *ClassService) toClassResponseDTO(ctx context.Context, class *model.Class) *dto.ClassResponseDTO {
	teacherCount, _ := s.classRepo.GetTeacherCount(ctx, class.ID)
	studentCount, _ := s.classRepo.GetStudentCount(ctx, class.ID)

	return &dto.ClassResponseDTO{
		ID:             class.ID,
		Name:           class.Name,
		Description:    class.Description,
		CourseID:       class.CourseID,
		OrganizationID: class.OrganizationID,
		Status:         class.Status,
		MaxStudents:    class.MaxStudents,
		StartDate:      class.StartDate,
		EndDate:        class.EndDate,
		TeacherCount:   teacherCount,
		StudentCount:   studentCount,
		CreatedAt:      utils.FormatTimestamp(class.CreatedAt),
		UpdatedAt:      utils.FormatTimestamp(class.UpdatedAt),
	}
}
