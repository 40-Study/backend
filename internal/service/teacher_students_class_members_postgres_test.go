package service

// Q3-08 (QA3): "Học viên của tôi" của giáo viên phải gồm cả học viên đang học trong lớp của giáo viên mà chưa ghi danh khoá
// nào (trước đây chỉ đọc enrollments), và nhận page_size tới 200. QA4-T17: mỗi LỚP đang học của học viên có mặt (một dòng
// mỗi học viên + lớp), kể cả lớp ngoài khoá học viên đã ghi danh; web gom theo học viên (group-students.ts).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

func TestGetMyStudents_IncludesActiveClassMembersWithoutEnrollment(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	teacher, other := f.user("teacher"), f.user("other-teacher")
	enrolled, classOnly, multiClass, dropped, foreign := f.user("enrolled"), f.user("class-only"), f.user("multi-class"), f.user("dropped"), f.user("foreign")
	course := f.course(teacher)

	mkClass := func(name string, courseID *uuid.UUID, createdBy *uuid.UUID) model.Class {
		c := model.Class{Name: name, CourseID: courseID, Status: "active", CreatedBy: createdBy}
		if err := f.db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
		return c
	}
	member := func(c model.Class, u model.User, status string) {
		if err := f.db.Create(&model.StudentClass{StudentID: u.ID, ClassID: c.ID, Status: status}).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := f.db.Create(&model.Enrollment{UserID: enrolled.ID, CourseID: course.ID}).Error; err != nil {
		t.Fatal(err)
	}
	// Lớp theo khoá của giáo viên, giảng viên được gán qua teacher_classes (không phải người tạo).
	classX := mkClass("Lớp X", &course.ID, nil)
	if err := f.db.Create(&model.TeacherClass{TeacherID: teacher.ID, ClassID: classX.ID, Role: "primary"}).Error; err != nil {
		t.Fatal(err)
	}
	// Lớp không gắn khoá do chính giáo viên tạo.
	classY := mkClass("Lớp Y", nil, &teacher.ID)
	// Lớp của giáo viên khác.
	classZ := mkClass("Lớp Z", nil, &other.ID)

	member(classX, enrolled, "active") // đã có ở dòng ghi danh: không lặp lại lớp X
	member(classY, enrolled, "active") // QA4-T17: lớp ngoài khoá đã ghi danh vẫn phải hiện
	member(classX, classOnly, "active")
	member(classX, multiClass, "active")
	member(classY, multiClass, "active") // ở hai lớp: hai dòng lớp, mỗi lớp một lần
	member(classX, dropped, "dropped")   // đã rời lớp: không tính
	member(classZ, foreign, "active")    // lớp người khác: không tính

	repo := repository.NewEnrollmentRepository(f.db)
	rows, total, err := repo.GetByInstructor(ctx, teacher.ID, 1, 200)
	if err != nil {
		t.Fatalf("GetByInstructor: %v", err)
	}
	// Khoá (học viên, lớp, có khoá hay không) -> số lần xuất hiện.
	type key struct {
		user, class string
		course      bool
	}
	got := map[key]int{}
	className := map[uuid.UUID]string{classX.ID: "X", classY.ID: "Y", classZ.ID: "Z"}
	for _, r := range rows {
		k := key{user: r.UserName, course: r.CourseID != nil}
		if r.ClassID != nil {
			k.class = className[*r.ClassID]
		}
		got[k]++
		if r.ClassOnly == (r.CourseID != nil) {
			t.Errorf("class_only=%v nhưng course_id=%v: %+v", r.ClassOnly, r.CourseID, r)
		}
	}
	want := map[key]int{
		{enrolled.UserName, "X", true}:    1, // dòng ghi danh mang lớp X của khoá
		{enrolled.UserName, "Y", false}:   1, // QA4-T17
		{classOnly.UserName, "X", false}:  1,
		{multiClass.UserName, "X", false}: 1,
		{multiClass.UserName, "Y", false}: 1,
	}
	if total != int64(len(want)) || len(rows) != len(want) {
		t.Fatalf("total=%d rows=%d (%v), muốn %d dòng %v", total, len(rows), got, len(want), want)
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%+v xuất hiện %d lần, muốn %d (tất cả: %v)", k, got[k], n, got)
		}
	}
	for k := range got {
		if k.user == dropped.UserName || k.user == foreign.UserName {
			t.Errorf("%s không được xuất hiện (đã rời lớp / lớp giáo viên khác)", k.user)
		}
	}

	// Phân trang qua cả hai nguồn: không mất, không lặp dòng.
	seen := map[string]bool{}
	for page := 1; page <= len(want); page++ {
		pageRows, pageTotal, err := repo.GetByInstructor(ctx, teacher.ID, page, 1)
		if err != nil || pageTotal != int64(len(want)) {
			t.Fatalf("page %d: total=%d err=%v", page, pageTotal, err)
		}
		if len(pageRows) != 1 {
			t.Fatalf("page %d: rows=%v thiếu", page, pageRows)
		}
		r := pageRows[0]
		k := r.UserName + "|" + r.ClassID.String()
		if seen[k] {
			t.Fatalf("page %d: dòng %s lặp", page, k)
		}
		seen[k] = true
	}
}

func TestGetMyStudents_PageSizeUpTo200AndClassOnlyHasNoCourseOrProgress(t *testing.T) {
	f := newS2Fixture(t)
	ctx := context.Background()
	teacher, student := f.user("teacher"), f.user("student")
	c := model.Class{Name: "Lớp", Status: "active", CreatedBy: &teacher.ID}
	if err := f.db.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&model.StudentClass{StudentID: student.ID, ClassID: c.ID, Status: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewTeacherService(nil, repository.NewEnrollmentRepository(f.db), repository.NewParentStudentRepository(f.db))

	for _, size := range []int{150, 200} {
		got, err := svc.GetMyStudents(ctx, teacher.ID, 1, size)
		if err != nil {
			t.Fatal(err)
		}
		if got.PageSize != size {
			t.Errorf("page_size %d bị đổi thành %d", size, got.PageSize)
		}
	}
	if got, _ := svc.GetMyStudents(ctx, teacher.ID, 1, 201); got.PageSize != 20 {
		t.Errorf("page_size 201 vượt trần phải về 20, nhận %d", got.PageSize)
	}

	got, err := svc.GetMyStudents(ctx, teacher.ID, 1, 200)
	if err != nil || len(got.Students) != 1 {
		t.Fatalf("students=%v err=%v", got, err)
	}
	s := got.Students[0]
	if s.ID != student.ID || s.CourseID != nil || s.CourseName != nil || s.Progress != nil {
		t.Errorf("học viên chỉ-có-lớp không được có khoá/tiến độ: %+v", s)
	}
	if s.ClassID == nil || *s.ClassID != c.ID || s.ClassName == nil || *s.ClassName != "Lớp" {
		t.Errorf("học viên chỉ-có-lớp phải mang tên lớp: %+v", s)
	}
}
