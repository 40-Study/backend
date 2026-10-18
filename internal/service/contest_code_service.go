package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

var ErrJudgeUnavailable = cerr(503, "JUDGE_UNAVAILABLE", "Máy chấm đang không sẵn sàng. Bài nộp chưa được tính, hãy thử lại.")

// CodeLanguageID is shared by contest grading and the free sandbox. Unknown values are rejected.
func CodeLanguageID(language string) (int, bool) {
	ids := map[string]int{"python": 71, "python3": 71, "javascript": 63, "js": 63, "c": 50, "cpp": 54, "c++": 54, "java": 62, "go": 60, "typescript": 74, "ts": 74, "rust": 73, "ruby": 72, "php": 68, "c#": 51, "cs": 51, "swift": 83, "kotlin": 78, "sql": 82}
	id, ok := ids[strings.ToLower(strings.TrimSpace(language))]
	return id, ok
}
func validateCodeProblems(problems []dto.ContestCodeProblemInput) error {
	if len(problems) < 1 || len(problems) > 10 {
		return invalidField("problems", "Cuộc thi code cần từ 1 đến 10 bài.")
	}
	for _, p := range problems {
		if len([]rune(strings.TrimSpace(p.Title))) < 2 || len([]rune(p.Title)) > 255 || strings.TrimSpace(p.Description) == "" || len(p.Description) > 30000 {
			return invalidField("problems", "Nhập tiêu đề và đề bài hợp lệ cho mỗi bài code.")
		}
		if p.Points < 1 || p.Points > 1000 || p.TimeLimit < 1 || p.TimeLimit > 10 || p.MemoryLimit < 32 || p.MemoryLimit > 256 {
			return invalidField("problems", "Điểm 1–1000, CPU 1–10 giây, bộ nhớ 32–256 MB.")
		}
		if p.Difficulty != "EASY" && p.Difficulty != "MEDIUM" && p.Difficulty != "HARD" {
			return invalidField("problems", "Độ khó không hợp lệ.")
		}
		if len(p.Tests) < 1 || len(p.Tests) > 10 {
			return invalidField("problems", "Mỗi bài cần 1–10 test case.")
		}
		visible := false
		for _, t := range p.Tests {
			if !t.Hidden {
				visible = true
			}
			if len(t.Input) > 16000 || len(t.ExpectedOutput) > 16000 || strings.TrimSpace(t.ExpectedOutput) == "" {
				return invalidField("problems", "Test case cần đầu ra, tối đa 16000 ký tự mỗi ô.")
			}
		}
		if !visible {
			return invalidField("problems", "Cần ít nhất một test mẫu công khai.")
		}
	}
	return nil
}
func codeProblemModels(contestID uuid.UUID, inputs []dto.ContestCodeProblemInput) []model.ContestProblem {
	out := make([]model.ContestProblem, 0, len(inputs))
	for i, p := range inputs {
		raw, _ := json.Marshal(p.Tests)
		t, m := p.TimeLimit, p.MemoryLimit
		out = append(out, model.ContestProblem{ID: uuid.New(), ContestID: contestID, Title: strings.TrimSpace(p.Title), Description: p.Description, Type: model.ContestProblemTypeCode, Difficulty: model.ContestProblemDifficulty(p.Difficulty), Points: p.Points, DisplayOrder: i, TimeLimit: &t, MemoryLimit: &m, TestCases: datatypes.JSON(raw)})
	}
	return out
}
func codeProblemDTO(p model.ContestProblem, manage bool) dto.ContestCodeProblemDTO {
	out := dto.ContestCodeProblemDTO{ID: p.ID, Title: p.Title, Description: p.Description, Difficulty: string(p.Difficulty), Points: p.Points, Samples: []dto.ContestCodeTest{}, TimeLimit: 5, MemoryLimit: 128}
	if p.TimeLimit != nil {
		out.TimeLimit = *p.TimeLimit
	}
	if p.MemoryLimit != nil {
		out.MemoryLimit = *p.MemoryLimit
	}
	var tests []dto.ContestCodeTest
	_ = json.Unmarshal(p.TestCases, &tests)
	for _, t := range tests {
		if !t.Hidden {
			out.Samples = append(out.Samples, t)
		}
	}
	if manage {
		out.Tests = tests
	}
	return out
}
func codeSubmissionDTO(sub model.ContestSubmission) dto.ContestCodeResult {
	out := dto.ContestCodeResult{ID: sub.ID.String(), ProblemID: sub.ProblemID, Language: deref(sub.Language), Status: string(sub.Status), Score: sub.Score, MaxScore: sub.MaxScore, CreatedAt: sub.CreatedAt, Tests: []dto.ContestCodeTestResult{}}
	if sub.Output != nil {
		_ = json.Unmarshal([]byte(*sub.Output), &out)
	}
	return out
}
func (s *ContestService) codeAccess(ctx context.Context, id uuid.UUID, a *ContestActor) (*model.Contest, *model.ContestParticipant, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if c == nil || c.Type != model.ContestTypeCoding {
		return nil, nil, ErrContestNotFound
	}
	if a.canManage(c) {
		return c, nil, nil
	}
	if !a.isStudent() {
		return nil, nil, ErrContestRoleNotAllowed
	}
	if c.Status != model.ContestStatusPublished {
		return nil, nil, ErrContestNotFound
	}
	p, err := s.repo.CodeParticipant(ctx, id, a.UserID)
	if err != nil {
		return nil, nil, err
	}
	if p == nil {
		return nil, nil, ErrContestNotJoined
	}
	return c, p, nil
}
func (s *ContestService) CodeSession(ctx context.Context, id uuid.UUID, a *ContestActor) (*dto.ContestCodeSession, error) {
	c, p, err := s.codeAccess(ctx, id, a)
	if err != nil {
		return nil, err
	}
	now := s.now()
	manage := a.canManage(c)
	if !manage && now.Before(c.StartTime) {
		return nil, ErrContestNotActive
	}
	problems, err := s.repo.CodeProblems(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &dto.ContestCodeSession{Problems: []dto.ContestCodeProblemDTO{}, Submissions: []dto.ContestCodeResult{}, ServerTime: now}
	for _, problem := range problems {
		out.Problems = append(out.Problems, codeProblemDTO(problem, manage))
		out.TotalPoints += problem.Points
	}
	if p != nil {
		out.StartedAt = p.StartedAt
		out.FinishedAt = p.FinishedAt
		out.Score = p.TotalScore
		if p.StartedAt != nil {
			dl := contestDeadline(c, *p.StartedAt)
			out.DeadlineAt = &dl
			out.CanSubmit = p.FinishedAt == nil && c.Status == model.ContestStatusPublished && now.Before(dl)
		}
	}
	subs, err := s.repo.CodeSubmissions(ctx, id, a.UserID)
	if err != nil {
		return nil, err
	}
	for _, sub := range subs {
		out.Submissions = append(out.Submissions, codeSubmissionDTO(sub))
	}
	return out, nil
}
func (s *ContestService) StartCode(ctx context.Context, id uuid.UUID, a *ContestActor) (*dto.ContestCodeSession, error) {
	c, p, err := s.codeAccess(ctx, id, a)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrContestRoleNotAllowed
	}
	now := s.now()
	if c.Phase(now) != model.ContestPhaseActive {
		return nil, ErrContestNotActive
	}
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&model.ContestParticipant{}).Where("id = ? AND started_at IS NULL AND finished_at IS NULL", p.ID).Update("started_at", now)
		return res.Error
	})
	if err != nil {
		return nil, err
	}
	return s.CodeSession(ctx, id, a)
}
func (s *ContestService) FinishCode(ctx context.Context, id uuid.UUID, a *ContestActor) (*dto.ContestCodeSession, error) {
	_, p, err := s.codeAccess(ctx, id, a)
	if err != nil {
		return nil, err
	}
	if p == nil || p.StartedAt == nil {
		return nil, ErrContestNotJoined
	}
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		var locked model.ContestParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", p.ID).Error; err != nil {
			return err
		}
		if locked.FinishedAt != nil {
			return nil
		}
		return tx.Model(&locked).Update("finished_at", s.now()).Error
	})
	if err != nil {
		return nil, err
	}
	return s.CodeSession(ctx, id, a)
}
func codeVerdict(status int) string {
	switch status {
	case 3:
		return "ACCEPTED"
	case 4:
		return "WRONG_ANSWER"
	case 5:
		return "TIME_LIMIT"
	case 6:
		return "COMPILATION_ERROR"
	case 7, 8, 9, 10, 11, 12:
		return "RUNTIME_ERROR"
	}
	return "JUDGING"
}
func (s *ContestService) JudgeCode(ctx context.Context, id uuid.UUID, a *ContestActor, req dto.ContestCodeRequest, submit bool) (*dto.ContestCodeResult, error) {
	if s.judge == nil {
		return nil, ErrJudgeUnavailable
	}
	_, ok := CodeLanguageID(req.Language)
	if !ok {
		return nil, invalidField("language", "Ngôn ngữ không được hỗ trợ.")
	}
	if req.RequestID == uuid.Nil || req.ProblemID == uuid.Nil || strings.TrimSpace(req.Code) == "" || len(req.Code) > 60000 || len(req.Stdin) > 16000 {
		return nil, invalidField("code", "Mã nguồn hoặc đầu vào không hợp lệ.")
	}
	c, p, err := s.codeAccess(ctx, id, a)
	if err != nil {
		return nil, err
	}
	if p == nil || p.StartedAt == nil {
		return nil, ErrContestNotJoined
	}
	problems, err := s.repo.CodeProblems(ctx, id)
	if err != nil {
		return nil, err
	}
	var problem *model.ContestProblem
	for i := range problems {
		if problems[i].ID == req.ProblemID {
			problem = &problems[i]
			break
		}
	}
	if problem == nil {
		return nil, ErrContestNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var result *dto.ContestCodeResult
	err = s.repo.Transaction(ctx, func(tx *gorm.DB) error {
		var locked model.ContestParticipant
		// NOWAIT rejects a second click without enqueueing another Judge job.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "NOWAIT"}).First(&locked, "id = ?", p.ID).Error; err != nil {
			if strings.Contains(err.Error(), "55P03") {
				return cerr(409, "JUDGE_BUSY", "Đang chấm một bài khác. Chờ kết quả trước khi gửi tiếp.")
			}
			return err
		}
		if submit {
			var existing model.ContestSubmission
			if err := tx.First(&existing, "id = ?", req.RequestID).Error; err == nil {
				if existing.UserID != a.UserID || existing.ContestID != id || existing.ProblemID != req.ProblemID || deref(existing.Code) != req.Code || deref(existing.Language) != req.Language {
					return ErrContestForbidden
				}
				saved := codeSubmissionDTO(existing)
				result = &saved
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		now := s.now()
		if locked.FinishedAt != nil {
			return ErrContestAlreadySubmitted
		}
		if c.Status != model.ContestStatusPublished || now.Before(c.StartTime) || !now.Before(contestDeadline(c, *locked.StartedAt)) {
			return ErrContestDeadlinePassed
		}
		var tests []dto.ContestCodeTest
		if submit {
			if json.Unmarshal(problem.TestCases, &tests) != nil || len(tests) == 0 {
				return invalidField("tests", "Đề chưa có test case.")
			}
		} else {
			tests = []dto.ContestCodeTest{{Input: req.Stdin}}
		}
		settings := codeProblemDTO(*problem, false)
		result, err = judgeCodeTests(ctx, s.judge, req, problem.ID, tests, problem.Points, settings.TimeLimit, settings.MemoryLimit, submit)
		if err != nil {
			return err
		}

		if !submit {
			return nil
		}
		result.ID = req.RequestID.String()
		raw, _ := json.Marshal(result)
		output := string(raw)
		language := req.Language
		code := req.Code
		sub := model.ContestSubmission{ID: req.RequestID, ContestID: id, ProblemID: problem.ID, UserID: a.UserID, Code: &code, Language: &language, Status: model.ContestSubmissionStatus(result.Status), Score: result.Score, MaxScore: result.MaxScore, Output: &output, ExecutionTime: &result.ExecutionTime, MemoryUsed: &result.MemoryUsed}
		if err := tx.Create(&sub).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE contest_participants SET total_score = (SELECT COALESCE(SUM(best),0) FROM (SELECT MAX(score) AS best FROM contest_submissions WHERE contest_id = ? AND user_id = ? GROUP BY problem_id) scores) WHERE id = ?`, id, a.UserID, p.ID).Error
	})
	return result, err
}
