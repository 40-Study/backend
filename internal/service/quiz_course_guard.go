package service

// Review đối kháng PR #79 (QA vòng 2, lane D): quiz gắn khoá học qua course_id/lesson_id là NỘI
// DUNG khoá đó (hiện trong trình phát bài học), nên phải theo đúng 2 luật đã áp cho khoá/chương/
// bài/nội dung bài:
//   - D4: khoá nháp/chờ duyệt/bị từ chối chỉ chủ khoá, admin, người đã ghi danh xem được
//     (canViewCourse) — trước bản vá, GET /quizzes?course_id=<khoá nháp> và GET /quizzes/:id trả
//     200 cho giảng viên khác, lộ tiêu đề/câu hỏi của khoá chưa xuất bản.
//   - Q5: khoá đang chờ duyệt không sửa được (ensureCourseEditable) — trước bản vá tạo/sửa/xoá
//     quiz và câu hỏi của khoá pending_review vẫn 201/200.
//
// Quiz gắn session_id (quiz trong buổi live) KHÔNG thuộc phạm vi: đó là hoạt động lớp học trực
// tiếp, không phải nội dung admin duyệt; quyền của nó vẫn theo checkSessionQuizAccess.
// Quyền SỞ HỮU quiz (giảng viên B sửa quiz của giảng viên A) KHÔNG xử lý ở đây: PR #80 thêm
// created_by + kiểm chủ quiz.

import (
	"context"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

// quizContentCourse trả khoá học mà quiz là NỘI DUNG của nó (course_id trực tiếp, hoặc
// lesson_id -> section -> course). nil khi quiz không gắn khoá/bài hoặc dữ liệu mồ côi.
func (s *QuizService) quizContentCourse(ctx context.Context, lessonID, courseID *uuid.UUID) (*model.Course, error) {
	var cid *uuid.UUID
	switch {
	case courseID != nil:
		cid = courseID
	case lessonID != nil:
		lesson, err := s.lessonRepo.GetByID(ctx, *lessonID)
		if err != nil {
			return nil, err
		}
		if lesson == nil {
			return nil, nil
		}
		section, err := s.sectionRepo.GetByID(ctx, lesson.SectionID)
		if err != nil {
			return nil, err
		}
		if section == nil {
			return nil, nil
		}
		cid = &section.CourseID
	default:
		return nil, nil
	}
	return s.courseRepo.GetByID(ctx, *cid)
}

// ensureQuizCourseVisible — D4 cho quiz: người KHÔNG phải chủ khoá/admin (caller đã lọc
// canView=true) đọc quiz của khoá chưa xuất bản thì nhận ErrCourseHidden, như khoá không tồn tại.
// Khoá công khai: không tốn truy vấn enrollment.
func (s *QuizService) ensureQuizCourseVisible(ctx context.Context, quiz *model.Quiz, userID uuid.UUID) error {
	course, err := s.quizContentCourse(ctx, quiz.LessonID, quiz.CourseID)
	if err != nil {
		return err
	}
	if course == nil || !isPrivateCourseStatus(course.Status) {
		return nil
	}
	enrolled, err := isEnrolledInCourse(ctx, s.enrollmentRepo, userID, course.ID)
	if err != nil {
		return err
	}
	if !canViewCourse(course, userID, false, enrolled) {
		return ErrCourseHidden
	}
	return nil
}

// EnsureQuizCourseEditable — Q5 cho quiz/câu hỏi ĐÃ CÓ: quiz thuộc khoá đang chờ duyệt thì
// ErrCourseLockedForReview (kể cả admin, cùng luật ensureCourseEditable). Quiz không tồn tại trả
// nil để handler phía sau trả 404 như cũ.
func (s *QuizService) EnsureQuizCourseEditable(ctx context.Context, quizID uuid.UUID) error {
	quiz, err := s.repo.GetQuizByID(ctx, quizID)
	if err != nil {
		return err
	}
	if quiz == nil {
		return nil
	}
	return s.EnsureNewQuizCourseEditable(ctx, quiz.LessonID, quiz.CourseID)
}

// EnsureNewQuizCourseEditable — Q5 khi TẠO quiz gắn course_id/lesson_id của khoá đang chờ duyệt.
func (s *QuizService) EnsureNewQuizCourseEditable(ctx context.Context, lessonID, courseID *uuid.UUID) error {
	course, err := s.quizContentCourse(ctx, lessonID, courseID)
	if err != nil {
		return err
	}
	if course == nil {
		return nil
	}
	return ensureCourseEditable(course)
}
