package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

type LessonCodeDetail struct {
	Exercise       *dto.ExerciseDetailDTO `json:"exercise"`
	LastSubmission *dto.ContestCodeResult `json:"last_submission,omitempty"`
}

func (s *ExerciseService) SetLessonJudge(judge Judge0Client, access LessonContentServiceInterface) {
	s.judge, s.lessonAccess = judge, access
}

func (s *ExerciseService) lessonExerciseAccess(ctx context.Context, lessonID, exerciseID, userID uuid.UUID, admin bool) error {
	if s.lessonAccess == nil {
		return ErrJudgeUnavailable
	}
	contents, err := s.lessonAccess.GetContentsByLessonID(ctx, lessonID, userID, admin)
	if err != nil {
		return cerr(403, "LESSON_LOCKED", "Bạn chưa được phép làm bài tập này.")
	}
	for _, c := range contents {
		if c.Type == "exercise" && c.ExerciseID != nil && *c.ExerciseID == exerciseID {
			return nil
		}
	}
	return cerr(404, "EXERCISE_NOT_FOUND", "Không tìm thấy bài tập trong bài học.")
}

func lessonSubmissionResult(sub *model.ExerciseSubmission) *dto.ContestCodeResult {
	out := &dto.ContestCodeResult{ID: sub.ID.String(), ProblemID: sub.ExerciseID, Language: sub.Language, Status: strings.ToUpper(sub.Verdict), Passed: sub.TestCasesPassed, Total: sub.TotalTestCases, MaxScore: 100, CreatedAt: sub.CreatedAt, Tests: []dto.ContestCodeTestResult{}}
	if out.Total > 0 {
		out.Score = 100 * out.Passed / out.Total
	}
	if sub.ExecutionTimeMs != nil {
		out.ExecutionTime = *sub.ExecutionTimeMs
	}
	if sub.MemoryUsedKB != nil {
		out.MemoryUsed = *sub.MemoryUsedKB
	}
	return out
}

func (s *ExerciseService) LessonCode(ctx context.Context, lessonID, exerciseID, userID uuid.UUID, admin bool) (*LessonCodeDetail, error) {
	if err := s.lessonExerciseAccess(ctx, lessonID, exerciseID, userID, admin); err != nil {
		return nil, err
	}
	exercise, err := s.GetExerciseByID(ctx, exerciseID, false)
	if err != nil {
		return nil, cerr(404, "EXERCISE_NOT_FOUND", "Không tìm thấy bài tập.")
	}
	out := &LessonCodeDetail{Exercise: exercise}
	last, err := s.repo.GetLatestSubmission(ctx, exerciseID, userID)
	if err == nil && last != nil {
		out.LastSubmission = lessonSubmissionResult(last)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return out, nil
}

func (s *ExerciseService) JudgeLessonCode(ctx context.Context, lessonID, exerciseID, userID uuid.UUID, admin bool, req dto.ContestCodeRequest, submit bool) (*dto.ContestCodeResult, error) {
	if err := s.lessonExerciseAccess(ctx, lessonID, exerciseID, userID, admin); err != nil {
		return nil, err
	}
	if s.judge == nil {
		return nil, ErrJudgeUnavailable
	}
	if req.RequestID == uuid.Nil || strings.TrimSpace(req.Code) == "" || len(req.Code) > 60000 || len(req.Stdin) > 16000 {
		return nil, invalidField("code", "Mã nguồn hoặc đầu vào không hợp lệ.")
	}
	languageID, ok := CodeLanguageID(req.Language)
	if !ok {
		return nil, invalidField("language", "Ngôn ngữ không được hỗ trợ.")
	}
	exercise, err := s.repo.GetExerciseWithTestCases(ctx, exerciseID)
	if err != nil {
		return nil, cerr(404, "EXERCISE_NOT_FOUND", "Không tìm thấy bài tập.")
	}
	allowed := len(exercise.Language) == 0
	for _, l := range exercise.Language {
		if id, valid := CodeLanguageID(l); valid && id == languageID {
			allowed = true
		}
	}
	if !allowed {
		return nil, invalidField("language", "Đề bài không hỗ trợ ngôn ngữ này.")
	}
	if len(exercise.TestCases) == 0 || len(exercise.TestCases) > 20 {
		return nil, invalidField("tests", "Bài tập cần từ 1 đến 20 test case để chấm.")
	}
	tests := []dto.ContestCodeTest{{Input: req.Stdin}}
	if submit {
		tests = nil
		for _, t := range exercise.TestCases {
			tests = append(tests, dto.ContestCodeTest{Input: t.Input, ExpectedOutput: t.ExpectedOutput, Hidden: t.IsHidden})
		}
	}
	store, ok := s.repo.(interface {
		JudgeTransaction(context.Context, uuid.UUID, uuid.UUID, func(*gorm.DB) error) error
	})
	if !ok {
		return nil, ErrJudgeUnavailable
	}
	// No fixed deadline here: judgeCodeTests sizes it by test count (capped at 90 s).
	// A fixed 60 s turned 4 slow tests into JUDGE_UNAVAILABLE (QA 261021 C1e).
	var result *dto.ContestCodeResult
	err = store.JudgeTransaction(ctx, exerciseID, userID, func(tx *gorm.DB) error {
		if submit {
			var previous model.ExerciseSubmission
			if err := tx.First(&previous, "id = ?", req.RequestID).Error; err == nil {
				if previous.ExerciseID != exerciseID || previous.UserID != userID || previous.Language != req.Language || previous.Code != req.Code {
					return cerr(409, "REQUEST_CONFLICT", "Mã lần nộp đã được sử dụng.")
				}
				result = lessonSubmissionResult(&previous)
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		var err error
		result, err = judgeCodeTests(ctx, s.judge, req, exerciseID, tests, 100, exercise.TimeLimit, exercise.MemoryLimit, submit)
		if err != nil {
			return err
		}
		if !submit {
			return nil
		}
		result.ID = req.RequestID.String()
		sub := model.ExerciseSubmission{ID: req.RequestID, ExerciseID: exerciseID, UserID: userID, Code: req.Code, Language: req.Language, Verdict: strings.ToLower(result.Status), TestCasesPassed: result.Passed, TotalTestCases: result.Total, ExecutionTimeMs: &result.ExecutionTime, MemoryUsedKB: &result.MemoryUsed, CreatedAt: result.CreatedAt}
		return tx.Create(&sub).Error
	})
	if err != nil && err.Error() == "exercise judge busy" {
		return nil, cerr(409, "JUDGE_BUSY", "Đang chấm bài. Chờ kết quả trước khi gửi tiếp.")
	}
	return result, err
}
