package repository

import (
	"context"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// ParentHasChildTaughtBy (QA hồi quy 03/10, A-06): true nếu phụ huynh có MỘT liên kết con ĐÃ XÁC NHẬN
// (status = active, và con chưa bị tắt quyền liên hệ giảng viên — can_contact_teachers) mà con đó đang được
// giảng viên teacherID dạy, tức là một trong hai:
//   - con đang ghi danh (chưa xoá mềm) một khoá có instructor_id = teacherID, hoặc
//   - con đang là thành viên active của một lớp (student_classes, cùng định nghĩa "active" với
//     StudentClassActiveCondition) mà teacherID được gán dạy (teacher_classes). Lớp đã xoá mềm hoặc đã lưu trữ
//     (archived) không tính.
//
// Một câu EXISTS duy nhất để canCreateDirectConversation không phải lặp theo từng con.
func (r *ParentStudentRepository) ParentHasChildTaughtBy(ctx context.Context, parentID, teacherID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT count(*) FROM parent_student_relations psr
		WHERE psr.parent_user_id = ? AND psr.status = ? AND psr.can_contact_teachers = TRUE
		  AND (
		    EXISTS (SELECT 1 FROM enrollments e JOIN courses co ON co.id = e.course_id
		            WHERE e.user_id = psr.student_user_id AND co.instructor_id = ?
		              AND e.deleted_at IS NULL AND co.deleted_at IS NULL)
		    OR EXISTS (SELECT 1 FROM student_classes sc
		            JOIN teacher_classes tc ON tc.class_id = sc.class_id
		            JOIN classes cl ON cl.id = sc.class_id
		            WHERE sc.student_id = psr.student_user_id AND tc.teacher_id = ?
		              AND (sc.status = 'active' OR sc.status = '' OR sc.status IS NULL)
		              AND cl.deleted_at IS NULL AND cl.status <> 'archived')
		  )`,
		parentID, model.ParentStudentStatusActive, teacherID, teacherID).Scan(&n).Error
	return n > 0, err
}
