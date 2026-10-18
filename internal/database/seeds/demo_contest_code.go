package seeds

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"time"
)

// Idempotent local teaching fixture; never resets submissions or existing owners.
func (s *Seeder) SeedDemoCodingContest(users map[string]model.User) error {
	teacher := users["teacher1@demo.com"]
	admin := users["admin@demo.com"]
	if teacher.ID == uuid.Nil {
		return fmt.Errorf("missing demo teacher")
	}
	now := time.Now()
	duration := 120
	c := model.Contest{Title: "Thử thách code · Tổng hai số", Slug: "demo-code-judge0", Type: model.ContestTypeCoding, Status: model.ContestStatusPublished, StartTime: now.Add(-time.Hour), EndTime: now.AddDate(0, 0, 7), DurationMinutes: &duration, IsPublic: true, CreatedBy: teacher.ID, ReviewedBy: &admin.ID, ReviewedAt: &now}
	if err := s.db.Where("slug = ?", c.Slug).Assign(map[string]interface{}{"start_time": now.Add(-time.Hour), "end_time": now.AddDate(0, 0, 7)}).Attrs(c).FirstOrCreate(&c).Error; err != nil {
		return err
	}
	tests := []dto.ContestCodeTest{{Input: "2 3", ExpectedOutput: "5", Hidden: false}, {Input: "-5 2", ExpectedOutput: "-3", Hidden: true}, {Input: "1000000000 1000000000", ExpectedOutput: "2000000000", Hidden: true}}
	raw, _ := json.Marshal(tests)
	cpu, ram := 2, 128
	p := model.ContestProblem{ContestID: c.ID, Title: "Tổng hai số nguyên", Description: "Đọc hai số nguyên a và b trên một dòng, cách nhau bởi khoảng trắng. In ra tổng a + b.\n\nRàng buộc: -10^9 ≤ a, b ≤ 10^9. Chương trình đọc từ stdin và in kết quả ra stdout.", Type: model.ContestProblemTypeCode, Difficulty: model.ContestProblemDifficultyEasy, Points: 100, TimeLimit: &cpu, MemoryLimit: &ram, TestCases: datatypes.JSON(raw)}
	return s.db.Where("contest_id = ? AND display_order = 0", c.ID).Attrs(p).FirstOrCreate(&p).Error
}
