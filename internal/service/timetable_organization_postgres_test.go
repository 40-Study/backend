package service

// Thời khoá biểu phân biệt lớp cá nhân (dạy tự do) với lớp tổ chức (Postgres thật, schema tạm): mỗi mục mang
// organization_id/organization_name của lớp, và GET /me/timetable lọc được theo tổ chức hoặc chỉ lớp cá nhân.
// Bỏ annotateTimetableOrganizations thì ca "gắn tổ chức" ĐỎ; bỏ FilterClassIDsByOrganization thì các ca lọc ĐỎ.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

type timetableOrgEnv struct {
	*s4ClassEnv
	org      model.Organization
	orgClass model.Class
	svc      *ScheduleService
}

// newTimetableOrgEnv: giảng viên coTeacher dạy một lớp cá nhân (e.class) và một lớp của tổ chức, mỗi lớp một lịch lặp.
func newTimetableOrgEnv(t *testing.T) *timetableOrgEnv {
	t.Helper()
	e := &timetableOrgEnv{s4ClassEnv: newS4ClassEnv(t)}
	db := e.f.db
	e.org = model.Organization{Name: "QA-org-" + uuid.NewString()[:8], Status: "active"}
	if err := db.Create(&e.org).Error; err != nil {
		t.Fatal(err)
	}
	e.orgClass = model.Class{Name: "QA-org-class", CourseID: &e.course.ID, Status: "active", OrganizationID: &e.org.ID}
	if err := db.Create(&e.orgClass).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.TeacherClass{TeacherID: e.coTeacher.ID, ClassID: e.orgClass.ID, Role: "primary"}).Error; err != nil {
		t.Fatal(err)
	}
	e.svc = newS5ScheduleSvc(e.s4ClassEnv)
	ctx := context.Background()
	for i, classID := range []uuid.UUID{e.class.ID, e.orgClass.ID} {
		if _, err := e.svc.CreateSchedule(ctx, classID, e.coTeacher.ID, false, dto.CreateClassScheduleDTO{
			DayOfWeek: 1 + i, StartTime: "19:00", EndTime: "21:00", EffectiveFrom: "2031-01-01", EffectiveUntil: "2031-12-31"}); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func classIDsOf(entries []dto.TimetableEntryDTO) map[uuid.UUID]dto.TimetableEntryDTO {
	out := map[uuid.UUID]dto.TimetableEntryDTO{}
	for _, en := range entries {
		out[en.ClassID] = en
	}
	return out
}

func TestTimetableOrg_EntriesCarryOrganization_Postgres(t *testing.T) {
	e := newTimetableOrgEnv(t)
	got, err := e.svc.GetMyTimetable(context.Background(), e.coTeacher.ID, "TEACHER", dto.TimetableOrgFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byClass := classIDsOf(got.Entries)
	personal, org := byClass[e.class.ID], byClass[e.orgClass.ID]
	if personal.ClassID == uuid.Nil || org.ClassID == uuid.Nil {
		t.Fatalf("muốn mục của cả hai lớp, có %+v", got.Entries)
	}
	if personal.OrganizationID != nil || personal.OrganizationName != nil {
		t.Errorf("lớp cá nhân không được mang tổ chức: %+v", personal)
	}
	if org.OrganizationID == nil || *org.OrganizationID != e.org.ID || org.OrganizationName == nil || *org.OrganizationName != e.org.Name {
		t.Errorf("lớp tổ chức phải mang id+tên tổ chức %s: %+v", e.org.Name, org)
	}
}

func TestTimetableOrg_Filters_Postgres(t *testing.T) {
	e := newTimetableOrgEnv(t)
	ctx := context.Background()
	other := uuid.New()
	for _, tc := range []struct {
		name   string
		filter dto.TimetableOrgFilter
		want   []uuid.UUID
	}{
		{"không lọc: cả hai", dto.TimetableOrgFilter{}, []uuid.UUID{e.class.ID, e.orgClass.ID}},
		{"chỉ lớp cá nhân", dto.TimetableOrgFilter{PersonalOnly: true}, []uuid.UUID{e.class.ID}},
		{"chỉ tổ chức", dto.TimetableOrgFilter{OrganizationID: &e.org.ID}, []uuid.UUID{e.orgClass.ID}},
		{"tổ chức không dạy", dto.TimetableOrgFilter{OrganizationID: &other}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, _ := time.Parse("2006-01-02", "2031-03-01")
			to, _ := time.Parse("2006-01-02", "2031-03-31")
			plain, err := e.svc.GetMyTimetable(ctx, e.coTeacher.ID, "TEACHER", tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			ranged, err := e.svc.GetMyTimetableWithSessions(ctx, e.coTeacher.ID, "TEACHER", from, to, tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			for label, entries := range map[string][]dto.TimetableEntryDTO{"lịch lặp": plain.Entries, "kèm buổi": ranged.Entries} {
				got := classIDsOf(entries)
				if len(got) != len(tc.want) {
					t.Fatalf("%s: lớp=%v, muốn %v", label, got, tc.want)
				}
				for _, id := range tc.want {
					if _, ok := got[id]; !ok {
						t.Errorf("%s: thiếu lớp %s", label, id)
					}
				}
			}
		})
	}
	// Học viên cũng lọc được: e.student chỉ học lớp cá nhân.
	stu, err := e.svc.GetMyTimetable(ctx, e.student.ID, "STUDENT", dto.TimetableOrgFilter{OrganizationID: &e.org.ID})
	if err != nil || len(stu.Entries) != 0 {
		t.Errorf("học viên lọc theo tổ chức không học: %+v err=%v", stu, err)
	}
}

func TestTimetableOrg_SoftDeletedOrgKeepsIDDropsName_Postgres(t *testing.T) {
	e := newTimetableOrgEnv(t)
	if err := e.f.db.Delete(&e.org).Error; err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.GetMyTimetable(context.Background(), e.coTeacher.ID, "TEACHER", dto.TimetableOrgFilter{})
	if err != nil {
		t.Fatal(err)
	}
	org := classIDsOf(got.Entries)[e.orgClass.ID]
	if org.OrganizationID == nil || *org.OrganizationID != e.org.ID || org.OrganizationName != nil {
		t.Errorf("tổ chức xoá mềm: muốn giữ id, bỏ tên; có %+v", org)
	}
}
