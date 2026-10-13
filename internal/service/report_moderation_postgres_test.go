package service

// ADM-N4 (QA vòng 2 261013): từ một báo cáo, admin phải gỡ được khoá bị báo cáo / ẩn thảo luận và thấy đối tượng đó là gì.
// Postgres thật vì các luật cần pin nằm ở SQL: UPDATE có điều kiện trạng thái, giao dịch gộp (đối tượng + report), lô
// tra cứu target theo từng loại, soft-delete.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type captureNotifier struct{ sent []dto.CreateNotificationDTO }

func (c *captureNotifier) SendNotification(req dto.CreateNotificationDTO) error {
	c.sent = append(c.sent, req)
	return nil
}

type moderationWorld struct {
	t        *testing.T
	f        *s2Fixture
	reports  *ReportService
	mod      *ReportModerationService
	notifier *captureNotifier
	admin    model.User
	reporter model.User
	teacher  model.User
}

func newModerationWorld(t *testing.T) *moderationWorld {
	t.Helper()
	db := isolatedAPISchema(t)
	f := &s2Fixture{t: t, db: db}
	resolver := NewReportTargetResolver(repository.NewReportTargetRepository(db))
	reportRepo := repository.NewReportRepository(db)
	reports := NewReportService(reportRepo)
	reports.SetTargetResolver(resolver)
	n := &captureNotifier{}
	return &moderationWorld{
		t: t, f: f, reports: reports, notifier: n,
		mod:   NewReportModerationService(reportRepo, repository.NewReportModerationRepository(db), resolver, n),
		admin: f.user("admin"), reporter: f.user("reporter"), teacher: f.user("teacher"),
	}
}

func (w *moderationWorld) report(typ string, targetID uuid.UUID) model.Report {
	w.t.Helper()
	r := model.Report{ReporterID: w.reporter.ID, ReportedType: typ, ReportedID: targetID, Reason: "spam", Status: "pending"}
	if err := w.f.db.Create(&r).Error; err != nil {
		w.t.Fatalf("tao report: %v", err)
	}
	return r
}

func (w *moderationWorld) discussion(hidden bool) model.Discussion {
	w.t.Helper()
	title := "Bai viet vi pham " + uuid.NewString()[:6]
	d := model.Discussion{UserID: w.reporter.ID, Title: &title, Content: "noi dung"}
	if err := w.f.db.Create(&d).Error; err != nil {
		w.t.Fatalf("tao thao luan: %v", err)
	}
	if hidden {
		w.f.db.Model(&d).Update("is_hidden", true)
	}
	return d
}

func (w *moderationWorld) courseStatus(id uuid.UUID) string {
	var c model.Course
	w.f.db.First(&c, "id = ?", id)
	return c.Status
}

func (w *moderationWorld) reload(id uuid.UUID) model.Report {
	var r model.Report
	w.f.db.First(&r, "id = ?", id)
	return r
}

func strp(s string) *string { return &s }

func TestReportModeration_GoKhoaBiBaoCao(t *testing.T) {
	w := newModerationWorld(t)
	ctx := context.Background()
	course := w.f.course(w.teacher)
	rep := w.report("course", course.ID)

	got, err := w.reports.GetReportByID(ctx, rep.ID, w.admin.ID, true)
	if err != nil || got.Target == nil || !got.Target.Exists || got.Target.Name == nil || *got.Target.Name != course.Title ||
		*got.Target.Status != "published" || got.Target.Slug == nil || *got.Target.Slug != course.Slug ||
		len(got.Target.AvailableActions) != 1 || got.Target.AvailableActions[0] != "unpublish_course" ||
		got.Target.OwnerID == nil || *got.Target.OwnerID != w.teacher.ID {
		t.Fatalf("target truoc khi go sai: %+v err=%v", got.Target, err)
	}

	res, err := w.mod.Apply(ctx, rep.ID, w.admin.ID, dto.ReportActionRequestDTO{Action: "unpublish_course", Reason: "  Vi pham ban quyen  "})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if st := w.courseStatus(course.ID); st != model.CourseStatusArchived {
		t.Fatalf("khoa phai archived, la %q", st)
	}
	after := w.reload(rep.ID)
	if after.Status != "resolved" || after.ResolvedBy == nil || *after.ResolvedBy != w.admin.ID || after.ResolvedAt == nil ||
		after.AdminNotes == nil || *after.AdminNotes != "Vi pham ban quyen" {
		t.Fatalf("report sau hanh dong: %+v", after)
	}
	if res.Report.Status != "resolved" || *res.Report.Target.Status != "archived" || len(res.Report.Target.AvailableActions) != 0 || res.Action != "unpublish_course" {
		t.Fatalf("response sau hanh dong: %+v target=%+v", res.Report, res.Report.Target)
	}
	if len(w.notifier.sent) != 1 || w.notifier.sent[0].UserIDs[0] != w.teacher.ID || w.notifier.sent[0].ReferenceID == nil || *w.notifier.sent[0].ReferenceID != course.ID {
		t.Fatalf("giao vien phai nhan dung 1 thong bao: %+v", w.notifier.sent)
	}
}

func TestReportModeration_KhongGoDuocKhiKhongPublishedHoacThieuLyDo(t *testing.T) {
	w := newModerationWorld(t)
	ctx := context.Background()

	for _, status := range []string{model.CourseStatusDraft, model.CourseStatusPendingReview, model.CourseStatusArchived} {
		c := w.f.course(w.teacher)
		w.f.db.Model(&c).Update("status", status)
		rep := w.report("course", c.ID)
		_, err := w.mod.Apply(ctx, rep.ID, w.admin.ID, dto.ReportActionRequestDTO{Action: "unpublish_course", Reason: "ly do hop le"})
		if !errors.Is(err, ErrReportTargetUnchanged) {
			t.Errorf("status %s: err = %v, muon ErrReportTargetUnchanged", status, err)
		}
		if st := w.courseStatus(c.ID); st != status {
			t.Errorf("status %s bi doi thanh %s", status, st)
		}
		if r := w.reload(rep.ID); r.Status != "pending" {
			t.Errorf("report doi trang thai du hanh dong that bai: %s", r.Status)
		}
	}

	c := w.f.course(w.teacher)
	rep := w.report("course", c.ID)
	for _, reason := range []string{"", "   ", "ab"} {
		if _, err := w.mod.Apply(ctx, rep.ID, w.admin.ID, dto.ReportActionRequestDTO{Action: "unpublish_course", Reason: reason}); !errors.Is(err, ErrReportActionReasonRequired) {
			t.Errorf("reason %q: err = %v, muon ErrReportActionReasonRequired", reason, err)
		}
	}
	if st := w.courseStatus(c.ID); st != model.CourseStatusPublished || w.reload(rep.ID).Status != "pending" {
		t.Errorf("thieu ly do nhung da co tac dung phu: course=%s", st)
	}
	if len(w.notifier.sent) != 0 {
		t.Errorf("khong duoc thong bao khi that bai: %+v", w.notifier.sent)
	}
}

func TestReportModeration_ActionKhongKhopLoaiVaDoiTuongDaMat(t *testing.T) {
	w := newModerationWorld(t)
	ctx := context.Background()
	c := w.f.course(w.teacher)
	rep := w.report("course", c.ID)
	for _, action := range []string{"hide_discussion", "unhide_discussion", "lock_user", ""} {
		if _, err := w.mod.Apply(ctx, rep.ID, w.admin.ID, dto.ReportActionRequestDTO{Action: action, Reason: "ly do"}); !errors.Is(err, ErrReportActionNotApplicable) {
			t.Errorf("action %q tren report khoa: err = %v", action, err)
		}
	}
	if _, err := w.mod.Apply(ctx, uuid.New(), w.admin.ID, dto.ReportActionRequestDTO{Action: "unpublish_course", Reason: "ly do"}); !errors.Is(err, ErrReportNotFound) {
		t.Errorf("report khong ton tai: %v", err)
	}

	gone := w.f.course(w.teacher)
	repGone := w.report("course", gone.ID)
	w.f.db.Delete(&gone)
	if _, err := w.mod.Apply(ctx, repGone.ID, w.admin.ID, dto.ReportActionRequestDTO{Action: "unpublish_course", Reason: "ly do"}); !errors.Is(err, ErrReportTargetNotFound) {
		t.Errorf("khoa da xoa: %v", err)
	}
	missing := w.report("discussion", uuid.New())
	if _, err := w.mod.Apply(ctx, missing.ID, w.admin.ID, dto.ReportActionRequestDTO{Action: "hide_discussion", Reason: "ly do"}); !errors.Is(err, ErrReportTargetNotFound) {
		t.Errorf("thao luan khong ton tai: %v", err)
	}
}

func TestReportModeration_AnHienThaoLuanVaResolveFalse(t *testing.T) {
	w := newModerationWorld(t)
	ctx := context.Background()
	d := w.discussion(false)
	rep := w.report("discussion", d.ID)
	no := false

	res, err := w.mod.Apply(ctx, rep.ID, w.admin.ID, dto.ReportActionRequestDTO{Action: "hide_discussion", Reason: "ngon tu thu han", Resolve: &no})
	if err != nil {
		t.Fatalf("hide: %v", err)
	}
	var hidden model.Discussion
	w.f.db.First(&hidden, "id = ?", d.ID)
	if !hidden.IsHidden {
		t.Fatal("thao luan phai bi an")
	}
	if r := w.reload(rep.ID); r.Status != "pending" || r.ResolvedBy != nil || r.AdminNotes == nil {
		t.Errorf("resolve=false phai giu trang thai pending nhung van luu ghi chu: %+v", r)
	}
	if *res.Report.Target.Status != "hidden" || len(res.Report.Target.AvailableActions) != 1 || res.Report.Target.AvailableActions[0] != "unhide_discussion" {
		t.Errorf("target sau khi an: %+v", res.Report.Target)
	}
	if len(w.notifier.sent) != 0 {
		t.Errorf("an thao luan khong thong bao giao vien: %+v", w.notifier.sent)
	}
	if _, err := w.mod.Apply(ctx, rep.ID, w.admin.ID, dto.ReportActionRequestDTO{Action: "hide_discussion", Reason: "ngon tu thu han"}); !errors.Is(err, ErrReportTargetUnchanged) {
		t.Errorf("an lan hai: %v", err)
	}
	if _, err := w.mod.Apply(ctx, rep.ID, w.admin.ID, dto.ReportActionRequestDTO{Action: "unhide_discussion"}); err != nil {
		t.Fatalf("unhide khong can ly do: %v", err)
	}
	w.f.db.First(&hidden, "id = ?", d.ID)
	if hidden.IsHidden {
		t.Error("thao luan phai hien lai")
	}
}

func TestReportTargets_MoiLoaiDoiTuongVaDoiTuongDaMat(t *testing.T) {
	w := newModerationWorld(t)
	ctx := context.Background()
	course := w.f.course(w.teacher)
	target := w.f.user("target-user")
	w.f.db.Model(&target).Updates(map[string]any{"full_name": "Nguyen Van A", "is_active": false})
	disc := w.discussion(true)
	review := model.Review{UserID: w.reporter.ID, CourseID: course.ID, Rating: 1, Comment: strp("Khoa hoc rat te")}
	if err := w.f.db.Create(&review).Error; err != nil {
		t.Fatalf("tao review: %v", err)
	}
	ratingOnly := model.Review{UserID: w.admin.ID, CourseID: course.ID, Rating: 2}
	w.f.db.Create(&ratingOnly)

	reps := map[string]model.Report{
		"course": w.report("course", course.ID), "user": w.report("user", target.ID), "discussion": w.report("discussion", disc.ID),
		"review": w.report("review", review.ID), "rating-only": w.report("review", ratingOnly.ID), "missing": w.report("user", uuid.New()),
	}
	list, err := w.reports.ListReports(ctx, "", "", 1, 50)
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	byID := map[uuid.UUID]dto.ReportResponseDTO{}
	for _, r := range list.Data {
		byID[r.ID] = r
	}
	tg := func(key string) *dto.ReportTargetDTO {
		r, ok := byID[reps[key].ID]
		if !ok || r.Target == nil {
			t.Fatalf("%s: thieu target trong danh sach", key)
		}
		return r.Target
	}

	if u := tg("user"); !u.Exists || *u.Name != "Nguyen Van A" || *u.Status != "locked" || len(u.AvailableActions) != 0 || *u.OwnerID != target.ID {
		t.Errorf("user: %+v", u)
	}
	d := tg("discussion")
	if !d.Exists || *d.Status != "hidden" || d.Slug != nil && *d.Slug == "" || d.AvailableActions[0] != "unhide_discussion" || d.CourseID != nil {
		t.Errorf("discussion: %+v", d)
	}
	if rv := tg("review"); !rv.Exists || *rv.Name != "Khoa hoc rat te" || rv.Status != nil || *rv.CourseID != course.ID || *rv.CourseSlug != course.Slug || len(rv.AvailableActions) != 0 {
		t.Errorf("review: %+v", rv)
	}
	if rv := tg("rating-only"); !rv.Exists || rv.Name != nil {
		t.Errorf("review chi co sao phai khong co ten: %+v", rv)
	}
	if m := tg("missing"); m.Exists || m.Name != nil || m.Status != nil || m.AvailableActions == nil || len(m.AvailableActions) != 0 {
		t.Errorf("doi tuong da mat phai exists=false va available_actions=[]: %+v", m)
	}
	if c := tg("course"); *c.CourseID != course.ID || *c.CourseSlug != course.Slug {
		t.Errorf("course: %+v", c)
	}

	// Nguoi khong phai kiem duyet (chu report) khong bao gio thay target.
	own, err := w.reports.GetReportByID(ctx, reps["course"].ID, w.reporter.ID, false)
	if err != nil || own.Target != nil {
		t.Errorf("reporter khong duoc thay target: %+v err=%v", own.Target, err)
	}
}
