package repository

import (
	"errors"
	"testing"

	"study.com/v1/internal/model"
)

// Bảng ĐẦY ĐỦ (mọi trạng thái x mọi hành động) cho luật chuyển trạng thái duyệt khoá học —
// đặc biệt pin rằng KHÔNG có đường nào tới published ngoài approve từ pending_review.
func TestEvaluateCourseReviewTransition_FullTable(t *testing.T) {
	want := map[CourseReviewAction]map[string]string{
		CourseActionSubmit:  {model.CourseStatusDraft: model.CourseStatusPendingReview, model.CourseStatusRejected: model.CourseStatusPendingReview},
		CourseActionApprove: {model.CourseStatusPendingReview: model.CourseStatusPublished},
		CourseActionReject:  {model.CourseStatusPendingReview: model.CourseStatusRejected},
		// Q5 (QA vòng 2): rút yêu cầu duyệt chỉ từ pending_review, về draft.
		CourseActionWithdraw: {model.CourseStatusPendingReview: model.CourseStatusDraft},
	}
	for action, allowed := range want {
		for _, current := range model.CourseStatuses {
			got, err := EvaluateCourseReviewTransition(current, action)
			if next, ok := allowed[current]; ok {
				if err != nil || got != next {
					t.Errorf("%s tu %s: got (%q,%v), muon %q", action, current, got, err, next)
				}
				continue
			}
			if !errors.Is(err, ErrCourseInvalidReviewStatus) {
				t.Errorf("%s tu %s phai bi chan, got (%q,%v)", action, current, got, err)
			}
		}
	}
}

func TestEvaluateTeacherReview_OnlyPending(t *testing.T) {
	for _, s := range model.TeacherApprovalStatuses {
		err := EvaluateTeacherReview(s)
		if (s == model.TeacherApprovalPending) != (err == nil) {
			t.Errorf("status %s: err=%v", s, err)
		}
	}
}

// Quyết định #5: nộp lại tối đa 3 lần — lần nộp lại thứ 1..3 hợp lệ, thứ 4 bị chặn.
func TestEvaluateTeacherResubmission_Limit(t *testing.T) {
	for used := 0; used < model.MaxTeacherResubmissions; used++ {
		if err := EvaluateTeacherResubmission(model.TeacherApprovalRejected, used); err != nil {
			t.Errorf("lan nop lai thu %d phai hop le: %v", used+1, err)
		}
	}
	if err := EvaluateTeacherResubmission(model.TeacherApprovalRejected, model.MaxTeacherResubmissions); !errors.Is(err, ErrTeacherResubmissionLimit) {
		t.Fatalf("lan nop lai thu %d phai bi chan, err=%v", model.MaxTeacherResubmissions+1, err)
	}
	for _, s := range []string{model.TeacherApprovalPending, model.TeacherApprovalApproved} {
		if err := EvaluateTeacherResubmission(s, 0); !errors.Is(err, ErrTeacherApplicationNotRejected) {
			t.Errorf("nop lai tu %s phai bi chan, err=%v", s, err)
		}
	}
}
