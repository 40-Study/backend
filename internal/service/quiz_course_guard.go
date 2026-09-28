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
// Re-review vòng 2: một quiz có thể gắn CẢ lesson_id lẫn course_id. Trước đây guard chỉ xét
// course_id, nên gửi {lesson_id: bài của khoá đang chờ duyệt, course_id: khoá nháp khác} lách
// được Q5 (quiz mới hiện trong bài của khoá pending). Giờ guard xét MỌI khoá mà quiz gắn vào, và
// khi TẠO quiz thì hai khoá phải trùng nhau (khoá suy ra từ lesson là chuẩn), người tạo phải là
// chủ khoá hoặc admin (vá IDOR teacher2 tạo quiz trong khoá của teacher1, có từ trước #79).
//
// Quiz gắn session_id (quiz trong buổi live) KHÔNG thuộc phạm vi: đó là hoạt động lớp học trực
// tiếp, không phải nội dung admin duyệt; quyền của nó vẫn theo checkSessionQuizAccess.
// Quyền SỬA/XOÁ quiz đã có của người khác (giảng viên B sửa quiz của A) KHÔNG xử lý ở đây: PR #80
// thêm created_by + ErrQuizNotOwner.

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

var (
	// ErrQuizCourseMismatch — body tạo quiz có lesson_id thuộc khoá A nhưng course_id là khoá B.
	ErrQuizCourseMismatch = errors.New("lesson_id and course_id belong to different courses")
	// ErrQuizCourseNotOwner — tạo quiz trong khoá (đã xuất bản) mà mình không phải chủ khoá.
	ErrQuizCourseNotOwner = errors.New("only the course owner or an admin can add quizzes to this course")
)

// lessonCourse — khoá chứa bài học (lesson -> section -> course). nil khi lessonID nil hoặc dữ
// liệu mồ côi.
func (s *QuizService) lessonCourse(ctx context.Context, lessonID *uuid.UUID) (*model.Course, error) {
	if lessonID == nil {
		return nil, nil
	}
	lesson, err := s.lessonRepo.GetByID(ctx, *lessonID)
	if err != nil || lesson == nil {
		return nil, err
	}
	section, err := s.sectionRepo.GetByID(ctx, lesson.SectionID)
	if err != nil || section == nil {
		return nil, err
	}
	return s.courseRepo.GetByID(ctx, section.CourseID)
}

// quizContentCourses — MỌI khoá mà quiz là nội dung của nó: khoá của lesson_id và khoá course_id
// (bỏ trùng). Rỗng khi quiz không gắn khoá/bài hoặc dữ liệu mồ côi.
func (s *QuizService) quizContentCourses(ctx context.Context, lessonID, courseID *uuid.UUID) ([]*model.Course, error) {
	fromLesson, err := s.lessonCourse(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	var direct *model.Course
	if courseID != nil && (fromLesson == nil || fromLesson.ID != *courseID) {
		if direct, err = s.courseRepo.GetByID(ctx, *courseID); err != nil {
			return nil, err
		}
	}
	courses := make([]*model.Course, 0, 2)
	for _, c := range []*model.Course{fromLesson, direct} {
		if c != nil {
			courses = append(courses, c)
		}
	}
	return courses, nil
}

// ensureQuizCourseVisible — D4 cho quiz: người KHÔNG phải chủ khoá/admin (caller đã lọc
// canView=true) đọc quiz của khoá chưa xuất bản thì nhận ErrCourseHidden, như khoá không tồn tại.
// Chỉ cần MỘT khoá gắn với quiz bị ẩn là quiz bị ẩn. Khoá công khai: không tốn truy vấn enrollment.
func (s *QuizService) ensureQuizCourseVisible(ctx context.Context, quiz *model.Quiz, userID uuid.UUID) error {
	courses, err := s.quizContentCourses(ctx, quiz.LessonID, quiz.CourseID)
	if err != nil {
		return err
	}
	for _, course := range courses {
		if !isPrivateCourseStatus(course.Status) {
			continue
		}
		enrolled, err := isEnrolledInCourse(ctx, s.enrollmentRepo, userID, course.ID)
		if err != nil {
			return err
		}
		if !canViewCourse(course, userID, false, enrolled) {
			return ErrCourseHidden
		}
	}
	return nil
}

// EnsureQuizCourseEditable — Q5 cho quiz/câu hỏi ĐÃ CÓ: quiz gắn với BẤT KỲ khoá nào đang chờ
// duyệt thì ErrCourseLockedForReview (kể cả admin, cùng luật ensureCourseEditable). Quiz không
// tồn tại trả nil để handler phía sau trả 404 như cũ.
func (s *QuizService) EnsureQuizCourseEditable(ctx context.Context, quizID uuid.UUID) error {
	quiz, err := s.repo.GetQuizByID(ctx, quizID)
	if err != nil {
		return err
	}
	if quiz == nil {
		return nil
	}
	courses, err := s.quizContentCourses(ctx, quiz.LessonID, quiz.CourseID)
	if err != nil {
		return err
	}
	for _, course := range courses {
		if err := ensureCourseEditable(course); err != nil {
			return err
		}
	}
	return nil
}

// EnsureNewQuizCourseEditable — kiểm trước khi TẠO quiz gắn course_id/lesson_id, theo thứ tự:
//  1. có cả hai mà khác khoá -> ErrQuizCourseMismatch (400);
//  2. không phải chủ khoá/admin -> ErrCourseHidden (404) nếu khoá chưa xuất bản (không lộ sự tồn
//     tại, như D4), ErrQuizCourseNotOwner (403) nếu đã xuất bản;
//  3. khoá đang chờ duyệt -> ErrCourseLockedForReview (409).
//
// lesson_id/course_id không tồn tại: trả nil để CreateQuiz tự báo lỗi như cũ.
func (s *QuizService) EnsureNewQuizCourseEditable(ctx context.Context, userID uuid.UUID, isAdmin bool, lessonID, courseID *uuid.UUID) error {
	fromLesson, err := s.lessonCourse(ctx, lessonID)
	if err != nil {
		return err
	}
	if fromLesson != nil && courseID != nil && fromLesson.ID != *courseID {
		return ErrQuizCourseMismatch
	}
	courses, err := s.quizContentCourses(ctx, lessonID, courseID)
	if err != nil {
		return err
	}
	for _, course := range courses {
		if !isAdmin && course.InstructorID != userID {
			if isPrivateCourseStatus(course.Status) {
				return ErrCourseHidden
			}
			return ErrQuizCourseNotOwner
		}
		if err := ensureCourseEditable(course); err != nil {
			return err
		}
	}
	return nil
}
