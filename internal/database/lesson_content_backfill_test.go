package database

// Test Postgres THẬT cho phần schema/backfill của nội dung bài học article+quiz (plan 261008 phase 1):
//   - backfill: bỏ NOT EXISTS (chèn trùng), bỏ lọc deleted_at, đổi lọc lesson_id, hoặc bỏ runDataMigrationOnce
//     (quiz bị xoá khỏi bài sống lại) thì test ĐỎ;
//   - widen CHECK: bước nới chk_lesson_contents_type của phase 8 phải đưa DB cũ (3 loại) tới chỗ nhận 'article'/'quiz';
//   - unique một phần uq_lesson_contents_quiz_id do tag AutoMigrate tạo, không cần bước SQL riêng.

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
	"study.com/v1/internal/testutil/pgtest"
)

type lessonFixture struct {
	t  *testing.T
	db *gorm.DB
	// lesson tạo trong cùng một khoá/chương
	courseID, sectionID uuid.UUID
}

func newLessonFixture(t *testing.T, db *gorm.DB) *lessonFixture {
	t.Helper()
	instructor := backfillUser(t, db, "lc")
	c := model.Course{InstructorID: instructor, Title: "QA-lc", Slug: "qa-lc-" + uuid.NewString()}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("tạo khoá: %v", err)
	}
	s := model.Section{CourseID: c.ID, Title: "Chương 1", DisplayOrder: 1}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("tạo chương: %v", err)
	}
	return &lessonFixture{t: t, db: db, courseID: c.ID, sectionID: s.ID}
}

func (f *lessonFixture) lesson(title string) uuid.UUID {
	f.t.Helper()
	l := model.Lesson{SectionID: f.sectionID, Title: title, DisplayOrder: 1}
	if err := f.db.Create(&l).Error; err != nil {
		f.t.Fatalf("tạo bài: %v", err)
	}
	return l.ID
}

func (f *lessonFixture) quiz(lessonID *uuid.UUID, courseID *uuid.UUID, title string) uuid.UUID {
	f.t.Helper()
	q := model.Quiz{LessonID: lessonID, CourseID: courseID, Title: title, TriggerType: "manual"}
	if err := f.db.Create(&q).Error; err != nil {
		f.t.Fatalf("tạo quiz: %v", err)
	}
	return q.ID
}

func (f *lessonFixture) contentsOf(lessonID uuid.UUID) []model.LessonContent {
	f.t.Helper()
	var rows []model.LessonContent
	if err := f.db.Where("lesson_id = ?", lessonID).Order("display_order ASC, created_at ASC").Find(&rows).Error; err != nil {
		f.t.Fatalf("đọc nội dung: %v", err)
	}
	return rows
}

func (f *lessonFixture) countQuizRows() int64 {
	f.t.Helper()
	var n int64
	if err := f.db.Model(&model.LessonContent{}).Where("type = 'quiz'").Count(&n).Error; err != nil {
		f.t.Fatalf("đếm: %v", err)
	}
	return n
}

func TestLessonContentQuizBackfill_EveryOrphanLessonQuizGetsOneRow_RunsOnce(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	f := newLessonFixture(t, db)

	emptyLesson := f.lesson("trống")
	videoLesson := f.lesson("có video")
	twoQuizLesson := f.lesson("hai quiz")
	linkedLesson := f.lesson("đã gắn")

	orphanOnEmpty := f.quiz(&emptyLesson, nil, "Quiz trên bài trống")
	// Nhánh bài tập -> quiz: quiz gắn bài kể cả bài ĐÃ có video — đây là ca mà điều kiện "bài chưa có nội dung" bỏ sót.
	video := model.LessonContent{LessonID: videoLesson, Type: "video", DisplayOrder: 0, IsMandatory: true}
	if err := db.Create(&video).Error; err != nil {
		t.Fatalf("tạo video: %v", err)
	}
	orphanOnVideo := f.quiz(&videoLesson, nil, "Quiz trên bài có video")
	first := f.quiz(&twoQuizLesson, nil, "Quiz 1")
	second := f.quiz(&twoQuizLesson, nil, "Quiz 2")

	alreadyLinked := f.quiz(&linkedLesson, nil, "Quiz đã có dòng")
	linkedRow := model.LessonContent{LessonID: linkedLesson, Type: "quiz", QuizID: &alreadyLinked, IsMandatory: true}
	if err := db.Create(&linkedRow).Error; err != nil {
		t.Fatalf("tạo dòng quiz sẵn có: %v", err)
	}

	deletedQuiz := f.quiz(&emptyLesson, nil, "Quiz đã xoá")
	if err := db.Delete(&model.Quiz{}, "id = ?", deletedQuiz).Error; err != nil {
		t.Fatalf("xoá mềm quiz: %v", err)
	}
	f.quiz(nil, &f.courseID, "Quiz chỉ gắn khoá, không gắn bài")

	before := f.countQuizRows() // 1: dòng đã có
	if err := runLessonContentQuizBackfill(db); err != nil {
		t.Fatalf("backfill lần 1: %v", err)
	}
	if got := f.countQuizRows() - before; got != 4 {
		t.Fatalf("lần 1 chèn %d dòng, muốn 4 (bài trống, bài có video, hai quiz của bài thứ ba)", got)
	}

	onEmpty := f.contentsOf(emptyLesson)
	if len(onEmpty) != 1 || onEmpty[0].QuizID == nil || *onEmpty[0].QuizID != orphanOnEmpty || onEmpty[0].Type != "quiz" {
		t.Fatalf("bài trống: muốn đúng 1 dòng quiz trỏ %s (quiz xoá mềm KHÔNG có dòng), nhận %+v", orphanOnEmpty, onEmpty)
	}
	if onEmpty[0].DisplayOrder != 0 || onEmpty[0].IsMandatory {
		t.Errorf("bài trống: display_order=%d is_mandatory=%v, muốn 0/false", onEmpty[0].DisplayOrder, onEmpty[0].IsMandatory)
	}
	if onEmpty[0].Title == nil || *onEmpty[0].Title != "Quiz trên bài trống" {
		t.Errorf("tiêu đề dòng phải lấy từ quiz, nhận %v", onEmpty[0].Title)
	}

	onVideo := f.contentsOf(videoLesson)
	if len(onVideo) != 2 || onVideo[1].QuizID == nil || *onVideo[1].QuizID != orphanOnVideo || onVideo[1].DisplayOrder != 1 {
		t.Fatalf("bài có video: muốn video rồi quiz (display_order 1), nhận %+v", onVideo)
	}

	onTwo := f.contentsOf(twoQuizLesson)
	if len(onTwo) != 2 || onTwo[0].DisplayOrder == onTwo[1].DisplayOrder {
		t.Fatalf("hai quiz cùng bài phải có display_order khác nhau, nhận %+v", onTwo)
	}
	seen := map[uuid.UUID]bool{}
	for _, r := range onTwo {
		seen[*r.QuizID] = true
	}
	if !seen[first] || !seen[second] {
		t.Errorf("thiếu dòng cho một trong hai quiz: %v", seen)
	}

	if rows := f.contentsOf(linkedLesson); len(rows) != 1 || rows[0].ID != linkedRow.ID {
		t.Errorf("quiz đã có dòng không được chèn thêm dòng, nhận %+v", rows)
	}

	// Lần hai: chèn 0. Xoá một dòng (giảng viên bỏ quiz khỏi bài) rồi chạy lại: KHÔNG sống lại, vì bước chỉ chạy một lần.
	total := f.countQuizRows()
	if err := runLessonContentQuizBackfill(db); err != nil {
		t.Fatalf("backfill lần 2: %v", err)
	}
	if f.countQuizRows() != total {
		t.Fatalf("lần 2 phải chèn 0 dòng, tổng %d -> %d", total, f.countQuizRows())
	}
	if err := db.Delete(&model.LessonContent{}, "id = ?", onEmpty[0].ID).Error; err != nil {
		t.Fatalf("xoá dòng: %v", err)
	}
	if err := runLessonContentQuizBackfill(db); err != nil {
		t.Fatalf("backfill lần 3: %v", err)
	}
	if rows := f.contentsOf(emptyLesson); len(rows) != 0 {
		t.Fatalf("dòng giảng viên đã xoá không được sống lại sau khi khởi động lại, nhận %+v", rows)
	}

	// Chính câu SQL vẫn idempotent (an toàn nếu bản ghi đánh dấu bị mất): quiz vừa mất dòng được chèn lại ĐÚNG 1 dòng, lần sau 0.
	for run, want := range []int64{1, 0} {
		n, err := backfillLessonContentQuizzes(db)
		if err != nil || n != want {
			t.Fatalf("SQL trần lần %d: chèn %d dòng (lỗi %v), muốn %d", run+1, n, err, want)
		}
	}
}

// Bước nới CHK của phase 8: DB cũ chỉ cho 3 loại phải nhận 'article'/'quiz' sau bước, và vẫn từ chối loại lạ.
func TestLessonContentTypeCheck_WidenedOnOldDB(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	f := newLessonFixture(t, db)
	lesson := f.lesson("bài")

	// Tên constraint GORM sinh cho tag `check:` không tên — hand-off phase 8 dựa vào tên này.
	var names []string
	if err := db.Raw(`SELECT conname FROM pg_constraint WHERE conrelid = 'lesson_contents'::regclass AND contype = 'c'`).Scan(&names).Error; err != nil {
		t.Fatalf("đọc constraint: %v", err)
	}
	found := false
	for _, n := range names {
		found = found || n == "chk_lesson_contents_type"
	}
	if !found {
		t.Fatalf("GORM không tạo chk_lesson_contents_type; có %v — sửa tên trong hand-off phase 8", names)
	}

	// Đưa về hình dạng DB cũ.
	if err := db.Exec(`ALTER TABLE lesson_contents DROP CONSTRAINT chk_lesson_contents_type;
		ALTER TABLE lesson_contents ADD CONSTRAINT chk_lesson_contents_type CHECK (type IN ('video','livestream','exercise'))`).Error; err != nil {
		t.Fatalf("giả lập DB cũ: %v", err)
	}
	insert := func(typ string) error {
		return db.Exec(`INSERT INTO lesson_contents (id, lesson_id, type) VALUES (gen_random_uuid(), ?, ?)`, lesson, typ).Error
	}
	if err := insert("article"); err == nil {
		t.Fatal("DB cũ phải từ chối 'article' trước khi nới (nếu không, test này không chứng minh gì)")
	}

	sql := buildCheckConstraintSQL("lesson_contents", "chk_lesson_contents_type", "type", model.LessonContentTypes)
	for run := 1; run <= 2; run++ { // lần hai: không đổi gì (idempotent)
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("nới CHK lần %d: %v", run, err)
		}
	}
	for _, typ := range model.LessonContentTypes {
		if err := insert(typ); err != nil {
			t.Errorf("sau khi nới, type %q phải ghi được: %v", typ, err)
		}
	}
	if err := insert("bogus"); err == nil {
		t.Error("loại lạ vẫn phải bị CHK từ chối")
	}
}

// uq_lesson_contents_quiz_id đến từ tag AutoMigrate: mỗi quiz tối đa một dòng nội dung, NULL thì không giới hạn;
// xoá cứng quiz thì FK ON DELETE SET NULL gỡ liên kết thay vì chặn.
func TestLessonContentQuizID_UniqueWhenSet_NullableMany_FKSetsNull(t *testing.T) {
	db := pgtest.IsolatedSchema(t, Migrate)
	f := newLessonFixture(t, db)
	lesson := f.lesson("bài")
	quiz := f.quiz(&lesson, nil, "Quiz")

	mk := func(quizID *uuid.UUID) error {
		return db.Create(&model.LessonContent{LessonID: lesson, Type: "quiz", QuizID: quizID, IsMandatory: true}).Error
	}
	if err := mk(&quiz); err != nil {
		t.Fatalf("dòng đầu: %v", err)
	}
	if err := mk(&quiz); err == nil {
		t.Fatal("dòng thứ hai cùng quiz_id phải vi phạm unique uq_lesson_contents_quiz_id")
	}
	if err := mk(nil); err != nil {
		t.Fatalf("quiz_id NULL (video/bài đọc) không được bị unique chặn: %v", err)
	}
	if err := mk(nil); err != nil {
		t.Fatalf("nhiều dòng quiz_id NULL phải ghi được: %v", err)
	}

	if err := db.Exec(`DELETE FROM quizzes WHERE id = ?`, quiz).Error; err != nil {
		t.Fatalf("xoá cứng quiz: %v", err)
	}
	var left int64
	db.Model(&model.LessonContent{}).Where("quiz_id = ?", quiz).Count(&left)
	if left != 0 {
		t.Fatalf("FK ON DELETE SET NULL phải gỡ quiz_id, còn %d dòng", left)
	}
}
