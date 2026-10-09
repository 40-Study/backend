package service

// Nội dung bài học loại ARTICLE và QUIZ (contract C1, plan 261008 phase 1). Test Postgres THẬT cho ghi/xoá/liên
// kết (isolatedAPISchema), test fake cho đường đọc (khoá bài, preview) và phép tính thuần.
//
// Mutation đã thử khi viết (mỗi dòng làm ít nhất một test ĐỎ): bỏ validateArticleBody ở Create/Update; đổi
// `>` thành `>=` ở trần 200000; bỏ so khớp quiz.LessonID; bỏ kiểm GetContentByQuizID; bỏ chặn đổi loại; bỏ xoá dòng
// nội dung trong QuizRepository.DeleteQuiz; bỏ mapQuizLinkWriteError; đổi 200 từ/phút.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type articleQuizFixture struct {
	t       *testing.T
	db      *gorm.DB
	svc     *LessonContentService
	quizSvc *QuizService
	teacher uuid.UUID
	other   uuid.UUID
	course  uuid.UUID
	lesson  uuid.UUID
}

func newArticleQuizFixture(t *testing.T) *articleQuizFixture {
	t.Helper()
	db := isolatedAPISchema(t)
	lessonRepo, sectionRepo, courseRepo := repository.NewLessonRepository(db), repository.NewSectionRepository(db), repository.NewCourseRepository(db)
	enrollmentRepo := repository.NewEnrollmentRepository(db)
	f := &articleQuizFixture{
		t:       t,
		db:      db,
		svc:     NewLessonContentService(lessonRepo, sectionRepo, courseRepo, enrollmentRepo, nil),
		quizSvc: NewQuizService(repository.NewQuizRepository(db), nil, courseRepo, sectionRepo, lessonRepo, nil, enrollmentRepo),
	}
	f.teacher, f.other = f.user("teacher"), f.user("other")
	f.course, f.lesson = f.courseWithLesson(f.teacher)
	return f
}

func (f *articleQuizFixture) user(kind string) uuid.UUID {
	f.t.Helper()
	s := uuid.NewString()
	u := model.User{Email: "qa-aq-" + kind + "-" + s + "@40study.test", PasswordHash: "x", UserName: "QA-aq-" + kind + "-" + s[:8]}
	if err := f.db.Create(&u).Error; err != nil {
		f.t.Fatalf("tạo user: %v", err)
	}
	return u.ID
}

func (f *articleQuizFixture) courseWithLesson(instructor uuid.UUID) (courseID, lessonID uuid.UUID) {
	f.t.Helper()
	c := model.Course{InstructorID: instructor, Title: "QA-aq", Slug: "qa-aq-" + uuid.NewString()}
	if err := f.db.Create(&c).Error; err != nil {
		f.t.Fatalf("tạo khoá: %v", err)
	}
	s := model.Section{CourseID: c.ID, Title: "Chương", DisplayOrder: 1}
	if err := f.db.Create(&s).Error; err != nil {
		f.t.Fatalf("tạo chương: %v", err)
	}
	l := model.Lesson{SectionID: s.ID, Title: "Bài", DisplayOrder: 1}
	if err := f.db.Create(&l).Error; err != nil {
		f.t.Fatalf("tạo bài: %v", err)
	}
	return c.ID, l.ID
}

func (f *articleQuizFixture) quiz(lessonID *uuid.UUID, creator uuid.UUID) uuid.UUID {
	f.t.Helper()
	q := model.Quiz{LessonID: lessonID, Title: "QA quiz " + uuid.NewString()[:8], TriggerType: "manual", CreatedBy: &creator}
	if err := f.db.Create(&q).Error; err != nil {
		f.t.Fatalf("tạo quiz: %v", err)
	}
	return q.ID
}

func (f *articleQuizFixture) row(contentID uuid.UUID) *model.LessonContent {
	f.t.Helper()
	var c model.LessonContent
	err := f.db.First(&c, "id = ?", contentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		f.t.Fatalf("đọc nội dung: %v", err)
	}
	return &c
}

func (f *articleQuizFixture) createArticle(body string) (*dto.LessonContentResponseDTO, error) {
	title := "Bài đọc"
	return f.svc.CreateContent(context.Background(), f.lesson, f.teacher, false,
		dto.CreateLessonContentDTO{Type: "article", Title: &title, ArticleBody: &body})
}

func (f *articleQuizFixture) createQuizContent(lesson, actor uuid.UUID, quizID *uuid.UUID) (*dto.LessonContentResponseDTO, error) {
	return f.svc.CreateContent(context.Background(), lesson, actor, false,
		dto.CreateLessonContentDTO{Type: "quiz", QuizID: quizID})
}

func wantRuleError(t *testing.T, err error, want *LessonContentRuleError) {
	t.Helper()
	var got *LessonContentRuleError
	if !errors.As(err, &got) || got.Code != want.Code || got.Status != want.Status {
		t.Fatalf("lỗi = %v, muốn %s (%d)", err, want.Code, want.Status)
	}
}

func words(n int) string { return strings.TrimSpace(strings.Repeat("từ ", n)) }

func TestLessonContentArticleQuiz_Article_CreateUpdateDelete(t *testing.T) {
	f := newArticleQuizFixture(t)
	ctx := context.Background()

	created, err := f.createArticle("<p>" + words(450) + "</p>")
	if err != nil {
		t.Fatalf("tạo bài đọc: %v", err)
	}
	if created.Type != "article" || created.ArticleBody == nil || created.QuizID != nil {
		t.Fatalf("response sai hình dạng: %+v", created)
	}
	if created.ReadingTimeMinutes == nil || *created.ReadingTimeMinutes != 3 {
		t.Fatalf("450 từ / 200 = 3 phút (làm tròn lên), nhận %v", created.ReadingTimeMinutes)
	}
	if row := f.row(created.ID); row == nil || row.ArticleBody == nil || !strings.HasPrefix(*row.ArticleBody, "<p>từ") {
		t.Fatalf("article_body chưa được lưu: %+v", row)
	}

	short := "<p>Ngắn thôi</p>"
	updated, err := f.svc.UpdateContent(ctx, created.ID, f.teacher, false, dto.UpdateLessonContentDTO{ArticleBody: &short})
	if err != nil {
		t.Fatalf("sửa bài đọc: %v", err)
	}
	if updated.ArticleBody == nil || *updated.ArticleBody != short || updated.ReadingTimeMinutes == nil || *updated.ReadingTimeMinutes != 1 {
		t.Fatalf("sau sửa: body/thời gian đọc sai: %+v", updated)
	}

	// Sửa tiêu đề mà KHÔNG gửi article_body: giữ nguyên nội dung.
	newTitle := "Đổi tiêu đề"
	kept, err := f.svc.UpdateContent(ctx, created.ID, f.teacher, false, dto.UpdateLessonContentDTO{Title: &newTitle})
	if err != nil || kept.ArticleBody == nil || *kept.ArticleBody != short {
		t.Fatalf("không gửi article_body phải giữ nội dung cũ, nhận %+v (lỗi %v)", kept, err)
	}

	if err := f.svc.DeleteContent(ctx, created.ID, f.teacher, false); err != nil {
		t.Fatalf("xoá bài đọc: %v", err)
	}
	if f.row(created.ID) != nil {
		t.Fatal("dòng bài đọc vẫn còn sau khi xoá")
	}
}

func TestLessonContentArticleQuiz_Article_BlankAndOversizeBody(t *testing.T) {
	f := newArticleQuizFixture(t)

	for name, body := range map[string]string{
		"rỗng":           "",
		"khoảng trắng":   "   \n\t ",
		"Tiptap rỗng":    "<p></p>",
		"chỉ thẻ + nbsp": "<p>&nbsp;</p><p> </p>",
		"chỉ script":     "<script>alert(1)</script>",
		"chỉ style":      "<style>p{color:red}</style>",
	} {
		_, err := f.createArticle(body)
		if err == nil {
			t.Errorf("%s: muốn ARTICLE_BODY_REQUIRED, nhưng tạo được", name)
			continue
		}
		wantRuleError(t, err, ErrArticleBodyRequired)
	}
	if _, err := f.svc.CreateContent(context.Background(), f.lesson, f.teacher, false, dto.CreateLessonContentDTO{Type: "article"}); err == nil {
		t.Error("thiếu hẳn article_body phải bị từ chối")
	} else {
		wantRuleError(t, err, ErrArticleBodyRequired)
	}

	if _, err := f.createArticle(`<p><img src="https://cdn.example/a.png"></p>`); err != nil {
		t.Errorf("bài đọc chỉ có ảnh là hợp lệ: %v", err)
	}

	// Trần tính theo KÝ TỰ, không theo byte: 200000 chữ có dấu (3 byte/chữ) vẫn hợp lệ, 200001 thì không.
	if _, err := f.createArticle(strings.Repeat("ạ", maxArticleBodyChars)); err != nil {
		t.Errorf("đúng %d ký tự phải hợp lệ: %v", maxArticleBodyChars, err)
	}
	_, err := f.createArticle(strings.Repeat("ạ", maxArticleBodyChars+1))
	if err == nil {
		t.Fatal("vượt trần phải bị từ chối")
	}
	wantRuleError(t, err, ErrArticleBodyTooLong)

	// Sửa: rỗng / quá dài bị từ chối và nội dung cũ còn nguyên.
	ok, err := f.createArticle("<p>Gốc</p>")
	if err != nil {
		t.Fatalf("tạo: %v", err)
	}
	blank, long := "  ", strings.Repeat("a", maxArticleBodyChars+1)
	_, err = f.svc.UpdateContent(context.Background(), ok.ID, f.teacher, false, dto.UpdateLessonContentDTO{ArticleBody: &blank})
	wantRuleError(t, err, ErrArticleBodyRequired)
	_, err = f.svc.UpdateContent(context.Background(), ok.ID, f.teacher, false, dto.UpdateLessonContentDTO{ArticleBody: &long})
	wantRuleError(t, err, ErrArticleBodyTooLong)
	if row := f.row(ok.ID); row == nil || row.ArticleBody == nil || *row.ArticleBody != "<p>Gốc</p>" {
		t.Fatalf("sửa thất bại không được làm đổi nội dung: %+v", row)
	}
}

func TestLessonContentArticleQuiz_Quiz_LinkHappyPathAndRejections(t *testing.T) {
	f := newArticleQuizFixture(t)
	ctx := context.Background()
	lesson := f.lesson

	q := f.quiz(&lesson, f.teacher)
	created, err := f.createQuizContent(lesson, f.teacher, &q)
	if err != nil {
		t.Fatalf("gắn quiz: %v", err)
	}
	if created.Type != "quiz" || created.QuizID == nil || *created.QuizID != q || created.ArticleBody != nil || created.ReadingTimeMinutes != nil {
		t.Fatalf("response quiz sai hình dạng: %+v", created)
	}
	if row := f.row(created.ID); row == nil || row.QuizID == nil || *row.QuizID != q {
		t.Fatalf("quiz_id chưa được lưu: %+v", row)
	}

	// 409 QUIZ_ALREADY_LINKED: quiz đã là nội dung của một dòng khác.
	_, err = f.createQuizContent(lesson, f.teacher, &q)
	wantRuleError(t, err, ErrQuizAlreadyLinked)

	// 400 QUIZ_ID_REQUIRED
	_, err = f.createQuizContent(lesson, f.teacher, nil)
	wantRuleError(t, err, ErrQuizIDRequired)

	// 404 QUIZ_NOT_FOUND: không tồn tại, hoặc đã xoá mềm.
	missing := uuid.New()
	_, err = f.createQuizContent(lesson, f.teacher, &missing)
	wantRuleError(t, err, ErrContentQuizNotFound)
	gone := f.quiz(&lesson, f.teacher)
	if err := f.db.Delete(&model.Quiz{}, "id = ?", gone).Error; err != nil {
		t.Fatalf("xoá mềm quiz: %v", err)
	}
	_, err = f.createQuizContent(lesson, f.teacher, &gone)
	wantRuleError(t, err, ErrContentQuizNotFound)

	// 409 QUIZ_LESSON_MISMATCH: quiz của bài khác (cùng khoá), và quiz không gắn bài nào.
	_, otherLesson := f.courseWithLesson(f.teacher)
	elsewhere := f.quiz(&otherLesson, f.teacher)
	_, err = f.createQuizContent(lesson, f.teacher, &elsewhere)
	wantRuleError(t, err, ErrQuizLessonMismatch)
	free := f.quiz(nil, f.teacher)
	_, err = f.createQuizContent(lesson, f.teacher, &free)
	wantRuleError(t, err, ErrQuizLessonMismatch)

	// Sửa: chuyển sang quiz khác của cùng bài được; chuyển sang quiz đã gắn dòng khác thì 409; gửi lại đúng quiz_id hiện có thì giữ nguyên.
	second := f.quiz(&lesson, f.teacher)
	moved, err := f.svc.UpdateContent(ctx, created.ID, f.teacher, false, dto.UpdateLessonContentDTO{QuizID: &second})
	if err != nil || moved.QuizID == nil || *moved.QuizID != second {
		t.Fatalf("đổi quiz_id sang quiz hợp lệ: %+v (lỗi %v)", moved, err)
	}
	third := f.quiz(&lesson, f.teacher)
	if _, err := f.createQuizContent(lesson, f.teacher, &third); err != nil {
		t.Fatalf("gắn quiz thứ ba: %v", err)
	}
	_, err = f.svc.UpdateContent(ctx, created.ID, f.teacher, false, dto.UpdateLessonContentDTO{QuizID: &third})
	wantRuleError(t, err, ErrQuizAlreadyLinked)
	if _, err := f.svc.UpdateContent(ctx, created.ID, f.teacher, false, dto.UpdateLessonContentDTO{QuizID: &second}); err != nil {
		t.Fatalf("gửi lại quiz_id hiện có không được lỗi: %v", err)
	}
}

func TestLessonContentArticleQuiz_TypeIsImmutableForArticleAndQuiz(t *testing.T) {
	f := newArticleQuizFixture(t)
	ctx := context.Background()
	lesson := f.lesson
	article, err := f.createArticle("<p>Bài</p>")
	if err != nil {
		t.Fatalf("tạo bài đọc: %v", err)
	}
	q := f.quiz(&lesson, f.teacher)
	quizContent, err := f.createQuizContent(lesson, f.teacher, &q)
	if err != nil {
		t.Fatalf("gắn quiz: %v", err)
	}
	videoURL := "https://cdn.example/v.mp4"
	video, err := f.svc.CreateContent(ctx, lesson, f.teacher, false, dto.CreateLessonContentDTO{Type: "video", VideoURL: &videoURL})
	if err != nil {
		t.Fatalf("tạo video: %v", err)
	}

	set := func(typ string) dto.UpdateLessonContentDTO { return dto.UpdateLessonContentDTO{Type: &typ} }
	for name, c := range map[string]struct {
		id  uuid.UUID
		typ string
	}{
		"article -> video": {article.ID, "video"},
		"article -> quiz":  {article.ID, "quiz"},
		"quiz -> exercise": {quizContent.ID, "exercise"},
		"quiz -> article":  {quizContent.ID, "article"},
		"video -> article": {video.ID, "article"},
		"video -> quiz":    {video.ID, "quiz"},
	} {
		_, err := f.svc.UpdateContent(ctx, c.id, f.teacher, false, set(c.typ))
		if err == nil {
			t.Errorf("%s: muốn CONTENT_TYPE_IMMUTABLE, nhưng đổi được", name)
			continue
		}
		wantRuleError(t, err, ErrContentTypeImmutable)
	}
	if row := f.row(video.ID); row == nil || row.Type != "video" {
		t.Fatalf("loại video bị đổi dù bị từ chối: %+v", row)
	}

	// Gửi lại ĐÚNG loại hiện có là hợp lệ; các loại cũ vẫn đổi qua lại như trước (không phá hành vi cũ).
	if _, err := f.svc.UpdateContent(ctx, article.ID, f.teacher, false, set("article")); err != nil {
		t.Errorf("article -> article phải hợp lệ: %v", err)
	}
	if _, err := f.svc.UpdateContent(ctx, video.ID, f.teacher, false, set("exercise")); err != nil {
		t.Errorf("video -> exercise vẫn phải hợp lệ như trước: %v", err)
	}
}

func TestLessonContentArticleQuiz_NonOwnerAndLockedCourse(t *testing.T) {
	f := newArticleQuizFixture(t)
	ctx := context.Background()
	lesson := f.lesson
	body := "<p>x</p>"

	existing, err := f.createArticle("<p>Của giảng viên chính</p>")
	if err != nil {
		t.Fatalf("tạo: %v", err)
	}
	q := f.quiz(&lesson, f.teacher)

	if _, err := f.svc.CreateContent(ctx, lesson, f.other, false, dto.CreateLessonContentDTO{Type: "article", ArticleBody: &body}); !errors.Is(err, ErrNotLessonCourseOwner) {
		t.Errorf("giảng viên khác tạo bài đọc: lỗi = %v, muốn ErrNotLessonCourseOwner (403)", err)
	}
	if _, err := f.createQuizContent(lesson, f.other, &q); !errors.Is(err, ErrNotLessonCourseOwner) {
		t.Errorf("giảng viên khác gắn quiz: lỗi = %v, muốn ErrNotLessonCourseOwner (403)", err)
	}
	if _, err := f.svc.UpdateContent(ctx, existing.ID, f.other, false, dto.UpdateLessonContentDTO{ArticleBody: &body}); !errors.Is(err, ErrNotLessonCourseOwner) {
		t.Errorf("giảng viên khác sửa bài đọc: lỗi = %v, muốn ErrNotLessonCourseOwner (403)", err)
	}
	if err := f.svc.DeleteContent(ctx, existing.ID, f.other, false); !errors.Is(err, ErrNotLessonCourseOwner) {
		t.Errorf("giảng viên khác xoá bài đọc: lỗi = %v, muốn ErrNotLessonCourseOwner (403)", err)
	}
	if row := f.row(existing.ID); row == nil || *row.ArticleBody != "<p>Của giảng viên chính</p>" {
		t.Fatalf("bài đọc bị đụng dù bị từ chối: %+v", row)
	}
	// Admin (isAdmin=true) không phải chủ khoá vẫn được.
	if _, err := f.svc.CreateContent(ctx, lesson, f.other, true, dto.CreateLessonContentDTO{Type: "article", ArticleBody: &body}); err != nil {
		t.Errorf("admin phải tạo được bài đọc: %v", err)
	}

	// Khoá đang chờ duyệt bị khoá sửa — cùng luật với video (ensureCourseEditable).
	if err := f.db.Model(&model.Course{}).Where("id = ?", f.course).Update("status", model.CourseStatusPendingReview).Error; err != nil {
		t.Fatalf("đặt trạng thái khoá: %v", err)
	}
	if _, err := f.createArticle("<p>Khi chờ duyệt</p>"); !errors.Is(err, ErrCourseLockedForReview) {
		t.Errorf("tạo bài đọc ở khoá chờ duyệt: lỗi = %v, muốn ErrCourseLockedForReview", err)
	}
	if _, err := f.createQuizContent(lesson, f.teacher, &q); !errors.Is(err, ErrCourseLockedForReview) {
		t.Errorf("gắn quiz ở khoá chờ duyệt: lỗi = %v, muốn ErrCourseLockedForReview", err)
	}
}

// Xoá quiz (xoá mềm) phải xoá dòng nội dung của nó CÙNG LÚC — không để lại mục "quiz" trỏ vào quiz đã xoá.
func TestLessonContentArticleQuiz_QuizDeleteRemovesItsContentRow(t *testing.T) {
	f := newArticleQuizFixture(t)
	ctx := context.Background()
	lesson := f.lesson
	article, err := f.createArticle("<p>Ở lại</p>")
	if err != nil {
		t.Fatalf("tạo bài đọc: %v", err)
	}
	doomed, kept := f.quiz(&lesson, f.teacher), f.quiz(&lesson, f.teacher)
	doomedRow, err := f.createQuizContent(lesson, f.teacher, &doomed)
	if err != nil {
		t.Fatalf("gắn quiz 1: %v", err)
	}
	keptRow, err := f.createQuizContent(lesson, f.teacher, &kept)
	if err != nil {
		t.Fatalf("gắn quiz 2: %v", err)
	}

	if err := f.quizSvc.DeleteQuiz(ctx, doomed, f.teacher, false); err != nil {
		t.Fatalf("xoá quiz: %v", err)
	}
	if f.row(doomedRow.ID) != nil {
		t.Fatal("dòng nội dung của quiz đã xoá vẫn còn")
	}
	if f.row(keptRow.ID) == nil || f.row(article.ID) == nil {
		t.Fatal("xoá một quiz không được xoá nội dung khác của bài")
	}
	// Quiz đã giải phóng: quiz khác gắn lại được, và quiz đã xoá thì không gắn được nữa.
	if _, err := f.createQuizContent(lesson, f.teacher, &doomed); err == nil {
		t.Fatal("quiz đã xoá không được gắn lại")
	} else {
		wantRuleError(t, err, ErrContentQuizNotFound)
	}
}

// Race: hai request cùng gắn một quiz, cả hai qua được bước kiểm trước; bên thua gặp unique của DB và phải nhận 409.
func TestLessonContentArticleQuiz_UniqueViolationMapsToAlreadyLinked(t *testing.T) {
	quizRow := &model.LessonContent{Type: model.LessonContentTypeQuiz}
	for _, err := range []error{
		gorm.ErrDuplicatedKey,
		errors.New(`ERROR: duplicate key value violates unique constraint "uq_lesson_contents_quiz_id" (SQLSTATE 23505)`),
	} {
		wantRuleError(t, mapQuizLinkWriteError(quizRow, err), ErrQuizAlreadyLinked)
	}
	other := errors.New("connection reset")
	if got := mapQuizLinkWriteError(quizRow, other); got != other {
		t.Errorf("lỗi khác phải đi qua nguyên vẹn, nhận %v", got)
	}
	// Vi phạm unique ở loại KHÁC không được nhận nhầm là quiz đã gắn.
	if got := mapQuizLinkWriteError(&model.LessonContent{Type: "video"}, gorm.ErrDuplicatedKey); errors.Is(got, ErrQuizAlreadyLinked) || got != gorm.ErrDuplicatedKey {
		t.Errorf("video không được bị map sang QUIZ_ALREADY_LINKED, nhận %v", got)
	}
	if mapQuizLinkWriteError(quizRow, nil) != nil {
		t.Error("nil phải giữ nil")
	}
}

func TestLessonContentArticleQuiz_ReadingTimeFromWordCount(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"rỗng tối thiểu 1 phút", "<p>a</p>", 1},
		{"đúng 200 từ", "<p>" + words(200) + "</p>", 1},
		{"201 từ làm tròn lên", "<p>" + words(201) + "</p>", 2},
		{"400 từ", "<p>" + words(400) + "</p>", 2},
		{"thẻ không tính là từ", "<p><strong>" + words(200) + "</strong></p><ul><li></li></ul>", 1},
		{"script/style không tính", "<script>" + words(5000) + "</script><style>" + words(5000) + "</style><p>ngắn</p>", 1},
		{"thực thể giải mã", "<p>" + strings.Repeat("&nbsp;", 3000) + words(10) + "</p>", 1},
		{"1000 từ", words(1000), 5},
	}
	for _, c := range cases {
		if got := articleReadingTimeMinutes(c.body); got != c.want {
			t.Errorf("%s: %d phút, muốn %d", c.name, got, c.want)
		}
	}
}

// Đường đọc của học viên: dòng article/quiz theo ĐÚNG luật khoá bài như video — bài khoá thì không nạp nội dung.
func TestLessonContentArticleQuiz_StudentReadPath_LockedLessonHidesArticleAndQuiz(t *testing.T) {
	lessonID, courseID := uuid.New(), uuid.New()
	lesson := &model.Lesson{IsPreview: false}
	lesson.ID = lessonID
	body, quiz := "<p>Bí mật của bài khoá</p>", uuid.New()
	rows := []model.LessonContent{
		{Type: "article", ArticleBody: &body},
		{Type: "quiz", QuizID: &quiz},
	}

	build := func(enrolled bool) (*LessonContentService, *fakeLessonRepoForLock) {
		lessonRepo := &fakeLessonRepoForLock{lesson: lesson, contents: rows}
		enrollmentRepo := &fakeEnrollmentRepoForLock{
			courseID: courseID,
			order:    []repository.LessonOrderInfo{{ID: lessonID, IsPreview: false}},
		}
		if enrolled {
			enrollmentRepo.enrollment = &model.Enrollment{}
		}
		return NewLessonContentService(lessonRepo, nil, &fakeCourseRepoForLock{course: &model.Course{Sequential: true}}, enrollmentRepo, nil), lessonRepo
	}

	svc, repo := build(false) // chưa ghi danh -> bài khoá
	if _, err := svc.GetContentsByLessonID(context.Background(), lessonID, uuid.New(), false); err != ErrLessonLocked {
		t.Fatalf("bài khoá: lỗi = %v, muốn ErrLessonLocked", err)
	}
	if repo.gotContentsCall {
		t.Fatal("bài khoá không được nạp nội dung (article_body sẽ lộ)")
	}

	svc, _ = build(true) // đã ghi danh, bài mở
	got, err := svc.GetContentsByLessonID(context.Background(), lessonID, uuid.New(), false)
	if err != nil || len(got) != 2 {
		t.Fatalf("học viên đã ghi danh phải đọc được: %v (%d mục)", err, len(got))
	}
	if got[0].ArticleBody == nil || *got[0].ArticleBody != body || got[0].ReadingTimeMinutes == nil || *got[0].ReadingTimeMinutes != 1 {
		t.Errorf("bài đọc thiếu body/thời gian đọc: %+v", got[0])
	}
	if got[1].QuizID == nil || *got[1].QuizID != quiz || got[1].ArticleBody != nil {
		t.Errorf("dòng quiz sai hình dạng: %+v", got[1])
	}
}

// Route công khai chỉ lộ article_body của bài is_preview trên khoá published (luật có sẵn, nay phủ cho article).
func TestLessonContentArticleQuiz_PreviewExposesArticleOnlyForPreviewLessons(t *testing.T) {
	body := "<p>Nội dung xem thử</p>"
	build := func(isPreview bool) (*LessonContentService, uuid.UUID) {
		courseID, sectionID, lessonID := uuid.New(), uuid.New(), uuid.New()
		course := &model.Course{Status: model.CourseStatusPublished}
		course.ID = courseID
		section := &model.Section{CourseID: courseID}
		section.ID = sectionID
		lesson := &model.Lesson{ID: lessonID, SectionID: sectionID, IsPreview: isPreview}
		svc := NewLessonContentService(
			&previewLessonRepoStub{lesson: lesson, contents: []model.LessonContent{{Type: "article", ArticleBody: &body}}},
			&previewSectionRepoStub{section: section}, &previewCourseRepoStub{course: course}, nil, nil)
		return svc, lessonID
	}

	svc, lessonID := build(true)
	got, err := svc.GetPreviewContentsByLessonID(context.Background(), "khoa", lessonID)
	if err != nil || len(got) != 1 || got[0].ArticleBody == nil || *got[0].ArticleBody != body {
		t.Fatalf("bài preview phải trả bài đọc: %+v (lỗi %v)", got, err)
	}

	svc, lessonID = build(false)
	got, err = svc.GetPreviewContentsByLessonID(context.Background(), "khoa", lessonID)
	if err != ErrLessonNotPreview || len(got) != 0 {
		t.Fatalf("bài không preview không được lộ bài đọc: %+v (lỗi %v)", got, err)
	}
}
