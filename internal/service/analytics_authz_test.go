package service

// Test cho P1 QA 260927 teacher: /analytics/livestream/:sessionId, /analytics/participants/:sessionId
// và /analytics/assignment/:assignmentId trước bản vá này chỉ có AuthMiddleware (bất kỳ ai đã
// đăng nhập) và KHÔNG kiểm actor có liên quan gì tới buổi live/bài tập — biết (hoặc đoán) đúng
// id là xem được số liệu của lớp/giáo viên khác. Test dưới đây kiểm ensureSessionAnalyticsAccess
// (dùng chung cho GetLivestreamAnalytics/GetParticipantAnalytics).

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type fakeLivestreamRepoForAnalytics struct {
	repository.LivestreamRepositoryInterface
	session *model.LivestreamSession
}

func (r *fakeLivestreamRepoForAnalytics) GetByID(_ context.Context, _ uuid.UUID) (*model.LivestreamSession, error) {
	return r.session, nil
}

type fakeClassRepoForAnalytics struct {
	repository.ClassRepositoryInterface
	class          *model.Class
	teacherOfClass map[uuid.UUID]bool // classID -> actor is teacher
}

func (r *fakeClassRepoForAnalytics) GetByID(_ context.Context, _ uuid.UUID) (*model.Class, error) {
	return r.class, nil
}

func (r *fakeClassRepoForAnalytics) TeacherClassExists(_ context.Context, classID, teacherID uuid.UUID) (bool, error) {
	return r.teacherOfClass[classID], nil
}

func (r *fakeClassRepoForAnalytics) StudentClassExists(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return false, nil
}

type fakeCourseRepoForAnalytics struct {
	repository.CourseRepositoryInterface
	course *model.Course
}

func (r *fakeCourseRepoForAnalytics) GetByID(_ context.Context, _ uuid.UUID) (*model.Course, error) {
	return r.course, nil
}

func TestGetLivestreamAnalytics_KhongPhaiHostHayGiaoVienLop_TuChoi(t *testing.T) {
	classID := uuid.New()
	sessionID := uuid.New()
	hostID := uuid.New()
	strangerID := uuid.New() // giáo viên KHÁC — không phải host, không dạy lớp này

	svc := &AnalyticsService{
		livestreamRepo: &fakeLivestreamRepoForAnalytics{session: &model.LivestreamSession{
			BaseModel: model.BaseModel{ID: sessionID},
			HostID:    hostID,
			ClassID:   classID,
		}},
		classRepo: &fakeClassRepoForAnalytics{
			class:          &model.Class{BaseModel: model.BaseModel{ID: classID}},
			teacherOfClass: map[uuid.UUID]bool{}, // stranger KHÔNG dạy lớp này
		},
		courseRepo: &fakeCourseRepoForAnalytics{},
	}

	_, err := svc.GetLivestreamAnalytics(context.Background(), sessionID, strangerID, false)
	if err == nil {
		t.Fatal("muon loi 403 (ErrNotAnalyticsOwner), duoc nil — giao vien khac xem duoc so lieu cua lop nguoi khac")
	}
	if !errors.Is(err, ErrNotAnalyticsOwner) {
		t.Fatalf("muon ErrNotAnalyticsOwner, duoc: %v", err)
	}
}

func TestGetLivestreamAnalytics_LaHost_ChoPhep(t *testing.T) {
	classID := uuid.New()
	sessionID := uuid.New()
	hostID := uuid.New()

	svc := &AnalyticsService{
		analyticsRepo: &fakeAnalyticsRepoNoOp{},
		livestreamRepo: &fakeLivestreamRepoForAnalytics{session: &model.LivestreamSession{
			BaseModel: model.BaseModel{ID: sessionID},
			HostID:    hostID,
			ClassID:   classID,
		}},
		classRepo:  &fakeClassRepoForAnalytics{},
		courseRepo: &fakeCourseRepoForAnalytics{},
	}

	_, err := svc.GetLivestreamAnalytics(context.Background(), sessionID, hostID, false)
	if err != nil {
		t.Fatalf("host cua phien phai xem duoc analytics, loi: %v", err)
	}
}

type fakeAnalyticsRepoNoOp struct {
	repository.AnalyticsRepositoryInterface
}

func (r *fakeAnalyticsRepoNoOp) GetBySession(_ context.Context, sessionID uuid.UUID) (*model.LivestreamAnalytics, error) {
	return nil, nil
}
