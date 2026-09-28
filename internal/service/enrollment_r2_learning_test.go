package service

// Test QA vòng 2, lane A (Học tập): A1 (độ dài thật của video thắng duration khai tay),
// việc phụ instructor_id trong DTO ghi danh, và định dạng giờ RFC3339 (root cause G1).

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// newDurationFixture dựng một bài có MỘT content video trỏ vào video upload nội bộ (URL HLS),
// với duration khai tay `declared` và độ dài thật trên video_uploads là `realDur`.
func newDurationFixture(declared int, realDur float64) (*EnrollmentService, *fakeLessonRepoHealWatched, uuid.UUID, uuid.UUID) {
	enrollmentID := uuid.New()
	enrollment := newEnrollmentWithID(enrollmentID)
	lessonID := uuid.New()
	uploadID := uuid.New()
	hlsURL := "/api/hls/" + uploadID.String() + "/master.m3u8"

	repo := &fakeEnrollmentRepoWatched{
		lessonProgress: &model.LessonProgress{EnrollmentID: enrollmentID, Status: "in_progress"},
		enrollment:     &enrollment,
		courseID:       uuid.New(),
		lessonOrder:    []uuid.UUID{lessonID},
	}
	lessonRepo := &fakeLessonRepoHealWatched{}
	contentID := uuid.New()
	lessonRepo.contents = []model.LessonContent{{ID: contentID, Type: "video", Duration: declared, VideoURL: &hlsURL}}
	videoRepo := &fakeVideoUploadRepoWatched{
		uploads: map[uuid.UUID]*model.VideoUpload{uploadID: {Duration: &realDur}},
	}
	return NewEnrollmentService(repo, &fakeCourseRepoWatched{}, lessonRepo, videoRepo), lessonRepo, lessonID, contentID
}

// TestUpdateLessonProgress_DoDaiThatThangDurationKhaiTayQuaLon (A1, P0): content khai 900s cho video
// thật dài 5s. Trước bản vá, server chỉ đối chiếu video_uploads khi duration khai tay = 0, nên
// mẫu số là 900 và xem trọn video chỉ được 0,6% — bài không bao giờ hoàn thành.
func TestUpdateLessonProgress_DoDaiThatThangDurationKhaiTayQuaLon(t *testing.T) {
	svc, lessonRepo, lessonID, contentID := newDurationFixture(900, 5)

	res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), lessonID,
		dto.UpdateLessonProgressDTO{PlayedRanges: dto.PlayedRangesDTO{{Start: 0, End: 5}}}, false)
	if err != nil {
		t.Fatalf("UpdateLessonProgress lỗi: %v", err)
	}
	if res.Status != "completed" {
		t.Errorf("status = %q (watched_pct=%v), muốn \"completed\": xem trọn video 5s phải đạt 100%%, "+
			"mẫu số phải là độ dài thật trên video_uploads chứ không phải 900s khai tay", res.Status, res.WatchedPct)
	}
	if len(lessonRepo.updatedDurations) != 1 || lessonRepo.updatedDurations[0] != 5 || lessonRepo.updatedIDs[0] != contentID {
		t.Errorf("ghi ngược duration = %v vào %v, muốn [5] vào content %s — cột khai sai phải được sửa",
			lessonRepo.updatedDurations, lessonRepo.updatedIDs, contentID)
	}
}

// TestUpdateLessonProgress_DoDaiThatThangDurationKhaiTayQuaNho (A1, chiều ngược lại — lỗ gian lận):
// content khai 5s cho video thật dài 900s. Xem 5 giây KHÔNG được tính là hoàn thành.
func TestUpdateLessonProgress_DoDaiThatThangDurationKhaiTayQuaNho(t *testing.T) {
	svc, _, lessonID, _ := newDurationFixture(5, 900)

	res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), lessonID,
		dto.UpdateLessonProgressDTO{PlayedRanges: dto.PlayedRangesDTO{{Start: 0, End: 5}}}, false)
	if err != nil {
		t.Fatalf("UpdateLessonProgress lỗi: %v", err)
	}
	if res.Status == "completed" {
		t.Errorf("xem 5s trên video thật 900s bị chốt completed (watched_pct=%v): server phải dùng độ dài thật", res.WatchedPct)
	}
}

// TestUpdateLessonProgress_DurationKhaiDungKhongGhiLai: khai đúng thì không ghi DB thừa mỗi nhịp.
func TestUpdateLessonProgress_DurationKhaiDungKhongGhiLai(t *testing.T) {
	svc, lessonRepo, lessonID, _ := newDurationFixture(120, 120)

	if _, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), lessonID,
		dto.UpdateLessonProgressDTO{PlayedRanges: dto.PlayedRangesDTO{{Start: 0, End: 10}}}, false); err != nil {
		t.Fatalf("UpdateLessonProgress lỗi: %v", err)
	}
	if len(lessonRepo.updatedDurations) != 0 {
		t.Errorf("ghi lại duration %v dù cột đã đúng, muốn 0 lần ghi", lessonRepo.updatedDurations)
	}
}

// TestGetMyEnrollments_TraInstructorID (việc phụ lane A, mở khoá E1): trước đây DTO ghi danh không
// có giảng viên nên hộp "Tin nhắn mới" lọc mất mọi khoá.
func TestGetMyEnrollments_TraInstructorID(t *testing.T) {
	instructorID := uuid.New()
	fullName := "Nguyễn Văn A"
	e := newEnrollmentWithID(uuid.New())
	e.Course.InstructorID = instructorID
	e.Course.Instructor.ID = instructorID
	e.Course.Instructor.UserName = "teacher1"
	e.Course.Instructor.FullName = &fullName

	repo := &fakeEnrollmentRepoWatched{enrollments: []model.Enrollment{e}, total: 1}
	svc := NewEnrollmentService(repo, nil, &fakeLessonRepoWatched{}, nil)

	res, err := svc.GetMyEnrollments(context.Background(), uuid.New(), 1, 20)
	if err != nil {
		t.Fatalf("GetMyEnrollments lỗi: %v", err)
	}
	got := res.Enrollments[0]
	if got.InstructorID != instructorID {
		t.Errorf("instructor_id = %s, muốn %s", got.InstructorID, instructorID)
	}
	if got.Instructor == nil || got.Instructor.ID != instructorID || got.Instructor.Name != fullName {
		t.Errorf("instructor = %+v, muốn id=%s name=%q", got.Instructor, instructorID, fullName)
	}
}

// TestToEnrollmentResponseDTO_GioLaRFC3339That (G1 phần của lane A): Format tay
// "2006-01-02T15:04:05Z" gắn chữ Z vào giờ ĐỊA PHƯƠNG, web đọc thành UTC và hiện lệch +7h.
func TestToEnrollmentResponseDTO_GioLaRFC3339That(t *testing.T) {
	hcm := time.FixedZone("ICT", 7*3600)
	enrolled := time.Date(2026, 9, 28, 10, 0, 0, 0, hcm) // = 03:00 UTC
	e := newEnrollmentWithID(uuid.New())
	e.EnrolledAt = enrolled
	e.CompletedAt = &enrolled
	e.LastAccessedAt = &enrolled

	d := (&EnrollmentService{}).toEnrollmentResponseDTO(&e)
	for name, v := range map[string]string{"enrolled_at": d.EnrolledAt, "completed_at": *d.CompletedAt, "last_accessed_at": *d.LastAccessedAt} {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t.Fatalf("%s = %q không phải RFC3339: %v", name, v, err)
		}
		if !parsed.Equal(enrolled) {
			t.Errorf("%s = %q, đọc lại thành %s — lệch %s so với mốc thật %s",
				name, v, parsed.UTC(), parsed.Sub(enrolled), enrolled.UTC())
		}
		if !strings.HasSuffix(v, "Z") {
			t.Errorf("%s = %q, muốn giờ UTC có hậu tố Z", name, v)
		}
	}
}
