package service

// Test cho F2 + F3 (review 260917, ca hai deu tai hien tren server that):
//   - F2: moi lan ghi tien do phai di qua WithLessonProgressLock; nhanh INSERT thua race (request
//     song song vua tao ban ghi) phai chay lai qua nhanh UPDATE va HOP NHAT len ban ghi do, khong
//     tra loi "duplicated key" nhu truoc.
//   - F3: ghi tien do vao bai dang khoa tuan tu phai tra ErrLessonLocked TRUOC moi thao tac ghi.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// raceOnceRepo: lan INSERT dau tien "thua race" — dung luc do mot request khac da tao ban ghi
// (otherRow). Lan chay lai, WithLessonProgressLock phai thay ban ghi do.
type raceOnceRepo struct {
	*fakeEnrollmentRepoWatched
	otherRow    *model.LessonProgress
	insertCalls int
}

func (r *raceOnceRepo) InsertLessonProgressIfAbsent(ctx context.Context, p *model.LessonProgress) (bool, error) {
	r.insertCalls++
	r.lessonProgress = r.otherRow // request kia da commit ban ghi cua no
	return false, nil
}

func (r *raceOnceRepo) WithLessonProgressLock(ctx context.Context, userID, lessonID uuid.UUID, fn func(repo repository.EnrollmentRepositoryInterface, locked *model.LessonProgress) error) error {
	r.lockCalls++
	return fn(r, r.lessonProgress)
}

func TestUpdateLessonProgress_ThuaRaceInsert_ChayLaiQuaNhanhUpdate(t *testing.T) {
	enrollmentID := uuid.New()
	enrollment := newEnrollmentWithID(enrollmentID)
	other := &model.LessonProgress{
		EnrollmentID: enrollmentID,
		Status:       "in_progress",
		PlayedRanges: model.PlayedRanges{{Start: 0, End: 20}},
	}
	repo := &raceOnceRepo{
		fakeEnrollmentRepoWatched: &fakeEnrollmentRepoWatched{
			lessonProgress: nil, // chua co ban ghi luc request nay bat dau
			enrollment:     &enrollment,
			courseID:       uuid.New(),
		},
		otherRow: other,
	}
	svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{}, &fakeLessonRepoWatched{}, nil)

	duration := 1000
	_, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), uuid.New(),
		dto.UpdateLessonProgressDTO{
			DurationSeconds: &duration,
			PlayedRanges:    dto.PlayedRangesDTO{{Start: 50, End: 70}},
		}, false)
	if err != nil {
		t.Fatalf("thua race INSERT khong duoc la loi voi client (truoc ban va: 400 duplicated key): %v", err)
	}
	if repo.insertCalls != 1 {
		t.Fatalf("INSERT duoc goi %d lan, mong doi 1 (lan chay lai phai di nhanh UPDATE)", repo.insertCalls)
	}
	if repo.lockCalls != 2 {
		t.Fatalf("WithLessonProgressLock duoc goi %d lan, mong doi 2 (lan dau + MOT lan chay lai)", repo.lockCalls)
	}
	if repo.updateCalls != 1 {
		t.Fatalf("UpdateLessonProgressFields duoc goi %d lan, mong doi 1", repo.updateCalls)
	}
	got, ok := repo.updateUpdates["played_ranges"].(model.PlayedRanges)
	if !ok {
		t.Fatalf("map UPDATE thieu played_ranges: %+v", repo.updateUpdates)
	}
	want := model.PlayedRanges{{Start: 0, End: 20}, {Start: 50, End: 70}}
	if !sameRanges(got, want) {
		t.Fatalf("played_ranges = %+v, mong doi %+v (phai hop nhat LEN ban ghi cua request kia, khong ghi de)", got, want)
	}
}

// lockedSequentialRepo: khoa tuan tu, bai truoc (prevID) chua hoan thanh.
type lockedSequentialRepo struct {
	*fakeEnrollmentRepoWatched
	order []repository.LessonOrderInfo
}

func (r *lockedSequentialRepo) GetLessonOrderInfoByCourseID(ctx context.Context, courseID uuid.UUID) ([]repository.LessonOrderInfo, error) {
	return r.order, nil
}

func TestUpdateLessonProgress_BaiKhoaTuanTu_TraErrLessonLocked(t *testing.T) {
	prevID, lockedID := uuid.New(), uuid.New()
	enrollment := newEnrollmentWithID(uuid.New())
	base := &fakeEnrollmentRepoWatched{enrollment: &enrollment, courseID: uuid.New()}
	repo := &lockedSequentialRepo{
		fakeEnrollmentRepoWatched: base,
		order:                     []repository.LessonOrderInfo{{ID: prevID}, {ID: lockedID}},
	}
	course := &model.Course{Sequential: true, InstructorID: uuid.New()}
	svc := NewEnrollmentService(repo, &fakeCourseRepoWatched{course: course}, &fakeLessonRepoWatched{}, nil)

	duration := 600
	res, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), lockedID,
		dto.UpdateLessonProgressDTO{
			DurationSeconds: &duration,
			PlayedRanges:    dto.PlayedRangesDTO{{Start: 0, End: 600}},
		}, false)
	if err != ErrLessonLocked {
		t.Fatalf("err = %v, mong doi ErrLessonLocked", err)
	}
	if res != nil {
		t.Fatalf("res = %+v, mong doi nil", res)
	}
	if base.lockCalls != 0 || base.updateCalls != 0 || base.upserted != nil {
		t.Fatalf("bai khoa khong duoc cham toi duong ghi: lock=%d update=%d insert=%v", base.lockCalls, base.updateCalls, base.upserted != nil)
	}

	// Chu khoa hoc va admin duoc bypass (giong phia DOC noi dung bai).
	if _, err := svc.UpdateLessonProgress(context.Background(), uuid.New(), lockedID,
		dto.UpdateLessonProgressDTO{DurationSeconds: &duration, PlayedRanges: dto.PlayedRangesDTO{{Start: 0, End: 10}}}, true); err != nil {
		t.Fatalf("admin khong duoc bi khoa: %v", err)
	}
	if _, err := svc.UpdateLessonProgress(context.Background(), course.InstructorID, lockedID,
		dto.UpdateLessonProgressDTO{DurationSeconds: &duration, PlayedRanges: dto.PlayedRangesDTO{{Start: 0, End: 10}}}, false); err != nil {
		t.Fatalf("chu khoa hoc khong duoc bi khoa: %v", err)
	}
}
