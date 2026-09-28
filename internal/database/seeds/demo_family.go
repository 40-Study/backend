package seeds

import (
	"fmt"
	"log"
	"time"

	"study.com/v1/internal/model"
)

// SeedDemoFamilyLinks (parent P0-2, QA 260927): hồ sơ parent1@demo.com ghi rõ "Phụ huynh của Lê
// Văn C." (Lê Văn C = student1@demo.com — bio set ở auth_service.go, độc lập hoàn toàn với bảng
// parent_student_relations) nhưng KHÔNG có row liên kết ACTIVE nào cho cặp này trong DB — không
// tài khoản demo nào test được luồng lõi "phụ huynh xem tiến độ con" (qa-260927-parent.md, root
// cause parent_student_repository.go#GetChildrenByParentID lọc status='active', không có row nào
// khớp). Seed này tạo 1 liên kết ACTIVE parent1 <-> student1.
func (s *Seeder) SeedDemoFamilyLinks(users map[string]model.User) error {
	log.Println("Seeding demo parent-student links...")

	parent, ok := users["parent1@demo.com"]
	if !ok {
		return fmt.Errorf("parent1@demo.com not found for family link seed")
	}
	student, ok := users["student1@demo.com"]
	if !ok {
		return fmt.Errorf("student1@demo.com not found for family link seed")
	}

	now := time.Now()
	relation := model.ParentStudentRelation{
		ParentUserID:  parent.ID,
		StudentUserID: student.ID,
		Relationship:  model.RelationshipParent,
		Status:        model.ParentStudentStatusActive,
		ConfirmedAt:   &now,
		ConfirmedBy:   ptr("system"),
	}

	if err := s.db.Where("parent_user_id = ? AND student_user_id = ?", parent.ID, student.ID).
		Attrs(relation).
		FirstOrCreate(&relation).Error; err != nil {
		return fmt.Errorf("failed to seed parent-student relation: %w", err)
	}

	// `Attrs()` trong FirstOrCreate chỉ áp dụng khi TẠO MỚI — nếu row đã tồn tại từ một lần chạy
	// QA/test trước (vd bị revoke khi thử luồng mời) thì phải ép lại về active tường minh.
	if relation.Status != model.ParentStudentStatusActive {
		if err := s.db.Model(&model.ParentStudentRelation{}).
			Where("parent_user_id = ? AND student_user_id = ?", parent.ID, student.ID).
			Updates(map[string]interface{}{
				"status":       model.ParentStudentStatusActive,
				"confirmed_at": now,
				"confirmed_by": "system",
			}).Error; err != nil {
			return fmt.Errorf("failed to activate parent-student relation: %w", err)
		}
	}

	log.Println("Seeded active parent-student link: parent1 <-> student1")
	return nil
}
