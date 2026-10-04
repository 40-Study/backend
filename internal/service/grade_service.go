package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type GradeServiceInterface interface {
	// GradeColumn
	CreateGradeColumn(ctx context.Context, classID, actorUserID uuid.UUID, req dto.CreateGradeColumnDTO) (*dto.GradeColumnResponseDTO, error)
	GetGradeColumns(ctx context.Context, classID, actorUserID uuid.UUID) ([]dto.GradeColumnResponseDTO, error)
	UpdateGradeColumn(ctx context.Context, classID, id, actorUserID uuid.UUID, req dto.UpdateGradeColumnDTO) (*dto.GradeColumnResponseDTO, error)
	DeleteGradeColumn(ctx context.Context, classID, id, actorUserID uuid.UUID) error
	ReorderGradeColumns(ctx context.Context, classID, actorUserID uuid.UUID, req dto.ReorderGradeColumnsDTO) error

	// Grade
	CreateGrade(ctx context.Context, classID, gradedBy uuid.UUID, req dto.CreateGradeDTO) (*dto.GradeResponseDTO, error)
	GetGradesByClass(ctx context.Context, classID, actorUserID uuid.UUID) (*dto.GradeBookDTO, error)
	GetStudentGrades(ctx context.Context, classID, studentID, requesterID uuid.UUID) ([]dto.GradeResponseDTO, error)
	UpdateGrade(ctx context.Context, id, gradedBy uuid.UUID, req dto.UpdateGradeDTO) (*dto.GradeResponseDTO, error)
	DeleteGrade(ctx context.Context, id, actorUserID uuid.UUID) error
	BulkCreateGrades(ctx context.Context, classID, gradedBy uuid.UUID, req dto.BulkCreateGradesDTO) ([]dto.GradeResponseDTO, error)

	// FinalGrade
	CalculateFinalGrades(ctx context.Context, classID, actorUserID uuid.UUID) ([]dto.FinalGradeResponseDTO, error)
	GetFinalGrades(ctx context.Context, classID, actorUserID uuid.UUID) ([]dto.FinalGradeResponseDTO, error)
	UpdateFinalGrade(ctx context.Context, classID, id, actorUserID uuid.UUID, req dto.UpdateFinalGradeDTO) (*dto.FinalGradeResponseDTO, error)
	FinalizeFinalGrades(ctx context.Context, classID, userID uuid.UUID) error

	// My grades
	GetMyGrades(ctx context.Context, studentID uuid.UUID) ([]dto.GradeResponseDTO, error)
	GetMyGradesByClass(ctx context.Context, studentID, classID uuid.UUID) ([]dto.GradeResponseDTO, error)
}

// ErrNotClassTeacher: C-13 (audit 260909) — trước đây CreateGrade/UpdateGrade/DeleteGrade
// chỉ dùng user_id để ghi GradedBy, không kiểm tra người gọi có thực sự dạy lớp đó không.
var ErrNotClassTeacher = errors.New("forbidden: not the teacher of this class")

// ErrGradeScoreOutOfRange: điểm phải nằm trong [0, max_score]. Trước đây điểm 15/10 vẫn được ghi, kéo điểm
// trung bình có trọng số của cả lớp lên theo.
var ErrGradeScoreOutOfRange = errors.New("score must be between 0 and max_score")

func validateScoreRange(score, maxScore decimal.Decimal) error {
	if score.IsNegative() || !maxScore.IsPositive() || score.GreaterThan(maxScore) {
		return ErrGradeScoreOutOfRange
	}
	return nil
}

type GradeService struct {
	repo       repository.GradeRepositoryInterface
	classRepo  repository.ClassRepositoryInterface
	courseRepo repository.CourseRepositoryInterface
	authz      ClassAuthorizer
	redis      *redis.Client
}

// authz có thể nil (test, hoặc môi trường không có PermissionChecker): khi đó không ai được nâng quyền
// ngoài người quản lý lớp theo dữ liệu lớp/khoá.
func NewGradeService(repo repository.GradeRepositoryInterface, classRepo repository.ClassRepositoryInterface, courseRepo repository.CourseRepositoryInterface, authz ClassAuthorizer, redis *redis.Client) *GradeService {
	return &GradeService{repo: repo, classRepo: classRepo, courseRepo: courseRepo, authz: authz, redis: redis}
}

// requireClassGrader: ai được chấm điểm và quản bảng điểm của lớp — xem ensureClassGrade (giảng viên lớp,
// người tạo lớp, instructor khoá, admin hệ thống, chủ/quản trị tổ chức của lớp). Trước đây chỉ tra
// teacher_classes nên chủ khoá, người tạo lớp và chủ tổ chức đều bị chặn (rà soát phân quyền, câu hỏi 7).
func (s *GradeService) requireClassGrader(ctx context.Context, classID, userID uuid.UUID) error {
	return ensureClassGrade(ctx, s.classRepo, s.courseRepo, s.authz, userID, classID)
}

// requireClassGraderWritable: requireClassGrader rồi lớp chưa lưu trữ. Dùng cho thao tác GHI điểm (tạo/sửa/xoá cột,
// điểm, tính và chốt điểm tổng kết); đọc bảng điểm dùng requireClassGrader trần nên lớp lưu trữ vẫn xem được.
func (s *GradeService) requireClassGraderWritable(ctx context.Context, classID, userID uuid.UUID) error {
	if err := s.requireClassGrader(ctx, classID, userID); err != nil {
		return err
	}
	return ensureClassWritable(ctx, s.classRepo, classID)
}

// requireStudentInClass (lane P): điểm chỉ ghi được cho học viên đang học lớp đó. Trước đây giảng viên
// của lớp ghi điểm cho student_id bất kỳ, và điểm hiện ra trong bảng điểm cá nhân (GetMyGrades) của
// người không hề thuộc lớp. Dùng ErrStudentNotInClass như điểm danh lớp (attendance_service.go).
func (s *GradeService) requireStudentInClass(ctx context.Context, classID, studentID uuid.UUID) error {
	in, err := s.classRepo.StudentClassExists(ctx, classID, studentID)
	if err != nil {
		return err
	}
	if !in {
		return ErrStudentNotInClass
	}
	return nil
}

const (
	gradeBookCachePrefix = "gradebook:class:"
	gradeBookCacheTTL    = 10 * time.Minute
)

func (s *GradeService) invalidateGradeCache(ctx context.Context, classID uuid.UUID) {
	if s.redis != nil {
		s.redis.Del(ctx, gradeBookCachePrefix+classID.String())
	}
}

// ============================================================================
// GRADE COLUMN
// ============================================================================

func (s *GradeService) CreateGradeColumn(ctx context.Context, classID, actorUserID uuid.UUID, req dto.CreateGradeColumnDTO) (*dto.GradeColumnResponseDTO, error) {
	if err := s.requireClassGraderWritable(ctx, classID, actorUserID); err != nil {
		return nil, err
	}
	col := &model.GradeColumn{
		ClassID:      classID,
		Name:         req.Name,
		GradeType:    model.GradeType(req.GradeType),
		Weight:       decimal.NewFromFloat(req.Weight),
		MaxScore:     decimal.NewFromFloat(10.0),
		DisplayOrder: req.DisplayOrder,
		IsRequired:   true,
	}
	if req.MaxScore > 0 {
		col.MaxScore = decimal.NewFromFloat(req.MaxScore)
	}
	if req.IsRequired != nil {
		col.IsRequired = *req.IsRequired
	}

	if err := s.repo.CreateGradeColumn(ctx, col); err != nil {
		return nil, err
	}

	s.invalidateGradeCache(ctx, classID)
	return s.mapColumnToDTO(col), nil
}

// GetGradeColumns (H-04 residual, review vòng 1): TRƯỚC ĐÂY không kiểm actorUserID -> mọi
// user đã đăng nhập xem được cấu trúc cột điểm (tên cột, trọng số) của bất kỳ lớp nào. C-13
// vòng 2 gate được CRUD cột điểm nhưng bỏ sót đúng route GET danh sách này.
func (s *GradeService) GetGradeColumns(ctx context.Context, classID, actorUserID uuid.UUID) ([]dto.GradeColumnResponseDTO, error) {
	if err := s.requireClassGrader(ctx, classID, actorUserID); err != nil {
		return nil, err
	}
	cols, err := s.repo.GetGradeColumnsByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}
	result := make([]dto.GradeColumnResponseDTO, len(cols))
	for i, c := range cols {
		result[i] = *s.mapColumnToDTO(&c)
	}
	return result, nil
}

func (s *GradeService) UpdateGradeColumn(ctx context.Context, classID, id, actorUserID uuid.UUID, req dto.UpdateGradeColumnDTO) (*dto.GradeColumnResponseDTO, error) {
	if err := s.requireClassGraderWritable(ctx, classID, actorUserID); err != nil {
		return nil, err
	}
	col, err := s.repo.GetGradeColumnByID(ctx, id)
	if err != nil || col == nil {
		return nil, errors.New("grade column not found")
	}
	if col.ClassID != classID {
		return nil, errors.New("column does not belong to this class")
	}

	if req.Name != nil {
		col.Name = *req.Name
	}
	if req.GradeType != nil {
		col.GradeType = model.GradeType(*req.GradeType)
	}
	if req.Weight != nil {
		col.Weight = decimal.NewFromFloat(*req.Weight)
	}
	if req.MaxScore != nil {
		col.MaxScore = decimal.NewFromFloat(*req.MaxScore)
	}
	if req.DisplayOrder != nil {
		col.DisplayOrder = *req.DisplayOrder
	}
	if req.IsRequired != nil {
		col.IsRequired = *req.IsRequired
	}

	if err := s.repo.UpdateGradeColumn(ctx, col); err != nil {
		return nil, err
	}

	s.invalidateGradeCache(ctx, classID)
	return s.mapColumnToDTO(col), nil
}

func (s *GradeService) DeleteGradeColumn(ctx context.Context, classID, id, actorUserID uuid.UUID) error {
	if err := s.requireClassGraderWritable(ctx, classID, actorUserID); err != nil {
		return err
	}
	col, err := s.repo.GetGradeColumnByID(ctx, id)
	if err != nil || col == nil {
		return errors.New("grade column not found")
	}
	if col.ClassID != classID {
		return errors.New("column does not belong to this class")
	}

	if err := s.repo.DeleteGradeColumn(ctx, id); err != nil {
		return err
	}
	s.invalidateGradeCache(ctx, classID)
	return nil
}

func (s *GradeService) ReorderGradeColumns(ctx context.Context, classID, actorUserID uuid.UUID, req dto.ReorderGradeColumnsDTO) error {
	if err := s.requireClassGraderWritable(ctx, classID, actorUserID); err != nil {
		return err
	}
	ids := make([]uuid.UUID, len(req.ColumnIDs))
	for i, idStr := range req.ColumnIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return errors.New("invalid column_id: " + idStr)
		}
		ids[i] = id
	}

	if err := s.repo.ReorderGradeColumns(ctx, classID, ids); err != nil {
		return err
	}
	s.invalidateGradeCache(ctx, classID)
	return nil
}

// ============================================================================
// GRADE
// ============================================================================

func (s *GradeService) CreateGrade(ctx context.Context, classID, gradedBy uuid.UUID, req dto.CreateGradeDTO) (*dto.GradeResponseDTO, error) {
	if err := s.requireClassGraderWritable(ctx, classID, gradedBy); err != nil {
		return nil, err
	}

	grade, err := s.buildGrade(ctx, classID, gradedBy, req)
	if err != nil {
		return nil, err
	}

	if err := s.repo.CreateGrade(ctx, grade); err != nil {
		return nil, err
	}

	s.invalidateGradeCache(ctx, classID)

	created, _ := s.repo.GetGradeByID(ctx, grade.ID)
	if created != nil {
		return s.mapGradeToDTO(created), nil
	}
	return s.mapGradeToDTO(grade), nil
}

// buildGrade kiểm học viên thuộc lớp + khoảng điểm rồi dựng bản ghi điểm CHƯA ghi. Tách khỏi CreateGrade để
// BulkCreateGrades validate được cả lô trước khi ghi dòng nào (quyền lớp do người gọi kiểm trước).
func (s *GradeService) buildGrade(ctx context.Context, classID, gradedBy uuid.UUID, req dto.CreateGradeDTO) (*model.Grade, error) {
	studentID, err := uuid.Parse(req.StudentID)
	if err != nil {
		return nil, errors.New("invalid student_id")
	}
	if err := s.requireStudentInClass(ctx, classID, studentID); err != nil {
		return nil, err
	}
	// Service có thể được gọi không qua handler (không qua validate): thiếu score là lỗi, không phải điểm 0.
	if req.Score == nil {
		return nil, errors.New("score is required")
	}
	if err := validateScoreRange(decimal.NewFromFloat(*req.Score), decimal.NewFromFloat(req.MaxScore)); err != nil {
		return nil, err
	}

	grade := &model.Grade{
		StudentID: studentID,
		ClassID:   classID,
		GradeType: model.GradeType(req.GradeType),
		Title:     req.Title,
		Score:     decimal.NewFromFloat(*req.Score),
		MaxScore:  decimal.NewFromFloat(req.MaxScore),
		Weight:    decimal.NewFromFloat(1.0),
		GradedBy:  gradedBy,
		GradedAt:  time.Now(),
	}

	if req.Weight > 0 {
		grade.Weight = decimal.NewFromFloat(req.Weight)
	}
	if req.Feedback != "" {
		grade.Feedback = &req.Feedback
	}
	if req.AssignmentID != "" {
		id, _ := uuid.Parse(req.AssignmentID)
		grade.AssignmentID = &id
	}
	if req.QuizID != "" {
		id, _ := uuid.Parse(req.QuizID)
		grade.QuizID = &id
	}
	if req.SessionID != "" {
		id, _ := uuid.Parse(req.SessionID)
		grade.SessionID = &id
	}
	return grade, nil
}

func (s *GradeService) GetGradesByClass(ctx context.Context, classID, actorUserID uuid.UUID) (*dto.GradeBookDTO, error) {
	// C-13: gradebook chứa điểm của TOÀN BỘ học sinh trong lớp — chỉ giáo viên của lớp mới
	// được xem, tránh học sinh dò classID để xem điểm bạn cùng lớp.
	if err := s.requireClassGrader(ctx, classID, actorUserID); err != nil {
		return nil, err
	}
	// Check cache
	if s.redis != nil {
		cacheKey := gradeBookCachePrefix + classID.String()
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var result dto.GradeBookDTO
			if json.Unmarshal([]byte(cached), &result) == nil {
				return &result, nil
			}
		}
	}

	columns, err := s.repo.GetGradeColumnsByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	grades, err := s.repo.GetGradesByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	// Group grades by student
	studentGrades := make(map[uuid.UUID][]model.Grade)
	studentNames := make(map[uuid.UUID]string)
	for _, g := range grades {
		studentGrades[g.StudentID] = append(studentGrades[g.StudentID], g)
		name := g.Student.UserName
		if g.Student.FullName != nil {
			name = *g.Student.FullName
		}
		studentNames[g.StudentID] = name
	}

	colDTOs := make([]dto.GradeColumnResponseDTO, len(columns))
	for i, c := range columns {
		colDTOs[i] = *s.mapColumnToDTO(&c)
	}

	var studentRows []dto.StudentGradeRowDTO
	for studentID, gs := range studentGrades {
		gradeDTOs := make([]dto.GradeResponseDTO, len(gs))
		for i, g := range gs {
			gradeDTOs[i] = *s.mapGradeToDTO(&g)
		}
		studentRows = append(studentRows, dto.StudentGradeRowDTO{
			StudentID:   studentID,
			StudentName: studentNames[studentID],
			Grades:      gradeDTOs,
		})
	}

	result := &dto.GradeBookDTO{
		ClassID:  classID,
		Columns:  colDTOs,
		Students: studentRows,
	}

	// Cache
	if s.redis != nil {
		if data, err := json.Marshal(result); err == nil {
			s.redis.Set(ctx, gradeBookCachePrefix+classID.String(), data, gradeBookCacheTTL)
		}
	}

	return result, nil
}

func (s *GradeService) GetStudentGrades(ctx context.Context, classID, studentID, requesterID uuid.UUID) ([]dto.GradeResponseDTO, error) {
	// C-13: chỉ chính học sinh đó hoặc giáo viên của lớp mới được xem điểm.
	if requesterID != studentID {
		if err := s.requireClassGrader(ctx, classID, requesterID); err != nil {
			return nil, err
		}
	}

	grades, err := s.repo.GetGradesByStudentAndClass(ctx, studentID, classID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.GradeResponseDTO, len(grades))
	for i, g := range grades {
		result[i] = *s.mapGradeToDTO(&g)
	}
	return result, nil
}

func (s *GradeService) UpdateGrade(ctx context.Context, id, gradedBy uuid.UUID, req dto.UpdateGradeDTO) (*dto.GradeResponseDTO, error) {
	grade, err := s.repo.GetGradeByID(ctx, id)
	if err != nil || grade == nil {
		return nil, errors.New("grade not found")
	}
	if err := s.requireClassGraderWritable(ctx, grade.ClassID, gradedBy); err != nil {
		return nil, err
	}

	if req.Score != nil {
		grade.Score = decimal.NewFromFloat(*req.Score)
	}
	if req.MaxScore != nil {
		grade.MaxScore = decimal.NewFromFloat(*req.MaxScore)
	}
	if req.Weight != nil {
		grade.Weight = decimal.NewFromFloat(*req.Weight)
	}
	if req.Feedback != nil {
		grade.Feedback = req.Feedback
	}
	if req.IsFinal != nil {
		grade.IsFinal = *req.IsFinal
	}
	// Kiểm trên giá trị SAU khi áp thay đổi: chỉ sửa max_score xuống thấp hơn điểm đang có cũng phải bị chặn.
	if err := validateScoreRange(grade.Score, grade.MaxScore); err != nil {
		return nil, err
	}
	grade.GradedBy = gradedBy
	grade.GradedAt = time.Now()

	if err := s.repo.UpdateGrade(ctx, grade); err != nil {
		return nil, err
	}

	s.invalidateGradeCache(ctx, grade.ClassID)
	// Nạp lại để Grader là người vừa chấm (grade.Grader đang giữ người chấm cũ).
	if updated, _ := s.repo.GetGradeByID(ctx, grade.ID); updated != nil {
		return s.mapGradeToDTO(updated), nil
	}
	return s.mapGradeToDTO(grade), nil
}

func (s *GradeService) DeleteGrade(ctx context.Context, id, actorUserID uuid.UUID) error {
	grade, err := s.repo.GetGradeByID(ctx, id)
	if err != nil || grade == nil {
		return errors.New("grade not found")
	}
	if err := s.requireClassGraderWritable(ctx, grade.ClassID, actorUserID); err != nil {
		return err
	}

	if err := s.repo.DeleteGrade(ctx, id); err != nil {
		return err
	}

	s.invalidateGradeCache(ctx, grade.ClassID)
	return nil
}

func (s *GradeService) BulkCreateGrades(ctx context.Context, classID, gradedBy uuid.UUID, req dto.BulkCreateGradesDTO) ([]dto.GradeResponseDTO, error) {
	// Kiểm quyền và học viên của CẢ lô trước khi ghi dòng nào (như điểm danh lớp, M-4 review S4),
	// để một học viên ngoài lớp không làm lô ghi dở dang.
	if err := s.requireClassGraderWritable(ctx, classID, gradedBy); err != nil {
		return nil, err
	}
	// Dựng và validate TOÀN BỘ lô (học viên thuộc lớp, khoảng điểm) trước khi ghi: một dòng sai ở giữa lô
	// không được để lại các dòng trước đó đã ghi. Ghi bằng MỘT câu INSERT nhiều dòng nên Postgres coi là
	// nguyên tử: hoặc đủ cả lô, hoặc không dòng nào.
	grades := make([]model.Grade, 0, len(req.Grades))
	for i, gReq := range req.Grades {
		grade, err := s.buildGrade(ctx, classID, gradedBy, gReq)
		if err != nil {
			return nil, fmt.Errorf("dòng %d: %w", i+1, err)
		}
		grades = append(grades, *grade)
	}
	if err := s.repo.BulkCreateGrades(ctx, grades); err != nil {
		return nil, err
	}
	s.invalidateGradeCache(ctx, classID)

	results := make([]dto.GradeResponseDTO, 0, len(grades))
	for i := range grades {
		// Nạp lại để có thông tin người chấm/học viên như CreateGrade.
		if created, _ := s.repo.GetGradeByID(ctx, grades[i].ID); created != nil {
			results = append(results, *s.mapGradeToDTO(created))
			continue
		}
		results = append(results, *s.mapGradeToDTO(&grades[i]))
	}
	return results, nil
}

// ============================================================================
// FINAL GRADE
// ============================================================================

func (s *GradeService) CalculateFinalGrades(ctx context.Context, classID, actorUserID uuid.UUID) ([]dto.FinalGradeResponseDTO, error) {
	if err := s.requireClassGraderWritable(ctx, classID, actorUserID); err != nil {
		return nil, err
	}
	grades, err := s.repo.GetGradesByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	// Group by student
	studentGrades := make(map[uuid.UUID][]model.Grade)
	for _, g := range grades {
		studentGrades[g.StudentID] = append(studentGrades[g.StudentID], g)
	}

	type studentAvg struct {
		studentID uuid.UUID
		avg       decimal.Decimal
	}
	var avgs []studentAvg

	for studentID, gs := range studentGrades {
		totalWeight := decimal.Zero
		weightedSum := decimal.Zero
		for _, g := range gs {
			score := g.Score.Div(g.MaxScore).Mul(decimal.NewFromInt(10)) // Normalize to 10
			weightedSum = weightedSum.Add(score.Mul(g.Weight))
			totalWeight = totalWeight.Add(g.Weight)
		}
		avg := decimal.Zero
		if !totalWeight.IsZero() {
			avg = weightedSum.Div(totalWeight)
		}
		avgs = append(avgs, studentAvg{studentID: studentID, avg: avg})
	}

	// Sort by average descending for ranking
	sort.Slice(avgs, func(i, j int) bool {
		return avgs[i].avg.GreaterThan(avgs[j].avg)
	})

	var results []dto.FinalGradeResponseDTO
	for rank, sa := range avgs {
		letterGrade := s.calculateLetterGrade(sa.avg)
		gpa := s.calculateGPA(sa.avg)
		status := "in_progress"
		if sa.avg.GreaterThanOrEqual(decimal.NewFromFloat(4.0)) {
			status = "passed"
		} else {
			status = "failed"
		}
		r := rank + 1

		fg := &model.FinalGrade{
			StudentID:       sa.studentID,
			ClassID:         classID,
			WeightedAverage: sa.avg,
			LetterGrade:     &letterGrade,
			GPA:             &gpa,
			Rank:            &r,
			Status:          status,
			CalculatedAt:    time.Now(),
		}

		if err := s.repo.UpsertFinalGrade(ctx, fg); err != nil {
			return nil, err
		}

		loaded, _ := s.repo.GetFinalGradeByStudentAndClass(ctx, sa.studentID, classID)
		if loaded != nil {
			results = append(results, *s.mapFinalGradeToDTO(loaded))
		}
	}

	s.invalidateGradeCache(ctx, classID)
	return results, nil
}

// GetFinalGrades (H-04 residual, review vòng 1): TRƯỚC ĐÂY không kiểm actorUserID -> rò rỉ
// điểm tổng kết toàn lớp cho bất kỳ user nào đã đăng nhập. Cùng lỗ hổng như GetGradeColumns.
func (s *GradeService) GetFinalGrades(ctx context.Context, classID, actorUserID uuid.UUID) ([]dto.FinalGradeResponseDTO, error) {
	if err := s.requireClassGrader(ctx, classID, actorUserID); err != nil {
		return nil, err
	}
	fgs, err := s.repo.GetFinalGradesByClassID(ctx, classID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.FinalGradeResponseDTO, len(fgs))
	for i, fg := range fgs {
		result[i] = *s.mapFinalGradeToDTO(&fg)
	}
	return result, nil
}

func (s *GradeService) UpdateFinalGrade(ctx context.Context, classID, id, actorUserID uuid.UUID, req dto.UpdateFinalGradeDTO) (*dto.FinalGradeResponseDTO, error) {
	if err := s.requireClassGraderWritable(ctx, classID, actorUserID); err != nil {
		return nil, err
	}
	fg, err := s.repo.GetFinalGradeByID(ctx, id)
	if err != nil || fg == nil {
		return nil, errors.New("final grade not found")
	}
	if fg.ClassID != classID {
		return nil, errors.New("final grade does not belong to this class")
	}

	if req.LetterGrade != nil {
		fg.LetterGrade = req.LetterGrade
	}
	if req.GPA != nil {
		gpa := decimal.NewFromFloat(*req.GPA)
		fg.GPA = &gpa
	}
	if req.Notes != nil {
		fg.Notes = req.Notes
	}

	if err := s.repo.UpdateFinalGrade(ctx, fg); err != nil {
		return nil, err
	}

	s.invalidateGradeCache(ctx, classID)
	return s.mapFinalGradeToDTO(fg), nil
}

func (s *GradeService) FinalizeFinalGrades(ctx context.Context, classID, userID uuid.UUID) error {
	if err := s.requireClassGraderWritable(ctx, classID, userID); err != nil {
		return err
	}
	if err := s.repo.FinalizeFinalGrades(ctx, classID, userID); err != nil {
		return err
	}
	s.invalidateGradeCache(ctx, classID)
	return nil
}

func (s *GradeService) GetMyGrades(ctx context.Context, studentID uuid.UUID) ([]dto.GradeResponseDTO, error) {
	grades, err := s.repo.GetGradesByStudentID(ctx, studentID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.GradeResponseDTO, len(grades))
	for i, g := range grades {
		result[i] = *s.mapGradeToDTO(&g)
	}
	return result, nil
}

func (s *GradeService) GetMyGradesByClass(ctx context.Context, studentID, classID uuid.UUID) ([]dto.GradeResponseDTO, error) {
	return s.GetStudentGrades(ctx, classID, studentID, studentID)
}

// ============================================================================
// HELPERS
// ============================================================================

func (s *GradeService) calculateLetterGrade(avg decimal.Decimal) string {
	f, _ := avg.Float64()
	switch {
	case f >= 9.0:
		return "A+"
	case f >= 8.5:
		return "A"
	case f >= 8.0:
		return "B+"
	case f >= 7.0:
		return "B"
	case f >= 6.5:
		return "C+"
	case f >= 5.5:
		return "C"
	case f >= 5.0:
		return "D+"
	case f >= 4.0:
		return "D"
	default:
		return "F"
	}
}

func (s *GradeService) calculateGPA(avg decimal.Decimal) decimal.Decimal {
	f, _ := avg.Float64()
	gpa := f / 10.0 * 4.0
	gpa = math.Round(gpa*100) / 100
	return decimal.NewFromFloat(gpa)
}

// ============================================================================
// MAPPERS
// ============================================================================

// graderDisplayName: tên hiển thị của người chấm (họ tên, không có thì username). Rỗng khi Grader chưa
// được Preload — không bịa tên.
func graderDisplayName(u *model.User) string {
	if u == nil || u.ID == uuid.Nil {
		return ""
	}
	if u.FullName != nil && *u.FullName != "" {
		return *u.FullName
	}
	return u.UserName
}

func (s *GradeService) mapColumnToDTO(col *model.GradeColumn) *dto.GradeColumnResponseDTO {
	return &dto.GradeColumnResponseDTO{
		ID:           col.ID,
		ClassID:      col.ClassID,
		Name:         col.Name,
		GradeType:    string(col.GradeType),
		Weight:       col.Weight,
		MaxScore:     col.MaxScore,
		DisplayOrder: col.DisplayOrder,
		IsRequired:   col.IsRequired,
	}
}

func (s *GradeService) mapGradeToDTO(g *model.Grade) *dto.GradeResponseDTO {
	name := ""
	if g.Student.FullName != nil {
		name = *g.Student.FullName
	} else {
		name = g.Student.UserName
	}

	return &dto.GradeResponseDTO{
		GradedByName: graderDisplayName(&g.Grader),
		ID:           g.ID,
		StudentID:    g.StudentID,
		StudentName:  name,
		ClassID:      g.ClassID,
		ClassName:    g.Class.Name,
		AssignmentID: g.AssignmentID,
		QuizID:       g.QuizID,
		SessionID:    g.SessionID,
		GradeType:    string(g.GradeType),
		Title:        g.Title,
		Score:        g.Score,
		MaxScore:     g.MaxScore,
		Weight:       g.Weight,
		GradedBy:     g.GradedBy,
		GradedAt:     g.GradedAt,
		Feedback:     g.Feedback,
		IsFinal:      g.IsFinal,
	}
}

func (s *GradeService) mapFinalGradeToDTO(fg *model.FinalGrade) *dto.FinalGradeResponseDTO {
	name := ""
	if fg.Student.FullName != nil {
		name = *fg.Student.FullName
	} else {
		name = fg.Student.UserName
	}

	return &dto.FinalGradeResponseDTO{
		ID:              fg.ID,
		StudentID:       fg.StudentID,
		StudentName:     name,
		ClassID:         fg.ClassID,
		WeightedAverage: fg.WeightedAverage,
		LetterGrade:     fg.LetterGrade,
		GPA:             fg.GPA,
		Rank:            fg.Rank,
		Status:          fg.Status,
		Notes:           fg.Notes,
		CalculatedAt:    fg.CalculatedAt,
		FinalizedAt:     fg.FinalizedAt,
	}
}
