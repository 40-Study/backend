package seeds

import (
	"errors"
	"fmt"
	"log"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type demoTestCaseSpec struct {
	Input, Output string
	Hidden        bool
}

type demoExerciseSpec struct {
	CourseSlug  string
	LessonTitle string // bài học có content type "exercise" do SeedDemoCourses tạo sẵn (exercise_id rỗng)
	Title       string
	Description string
	Difficulty  string
	Languages   []string
	StarterCode string
	TestCases   []demoTestCaseSpec
}

const demoTodoStarterCode = `const lines = require("fs").readFileSync(0, "utf8").trim().split("\n");
const n = Number(lines[0]);
// TODO: in ra tên các công việc chưa xong (cờ 0), mỗi dòng một tên; không có thì in "EMPTY"
`

// demoExerciseSpecs: bài tập code thật (đề + input/output mẫu) cho bài "Bài tập thực hành: Todo App"
// của khoá React — trang học /learn/<slug>/<lesson> đọc lesson_content.exercise để hiện đề và chấm bài.
var demoExerciseSpecs = []demoExerciseSpec{
	{
		CourseSlug:  "react-nextjs-tu-co-ban-den-nang-cao",
		LessonTitle: "Bài tập thực hành: Todo App",
		Title:       "Todo App: lọc công việc chưa hoàn thành",
		Description: "Danh sách Todo được cho dưới dạng văn bản. Dòng đầu là số công việc N (0 ≤ N ≤ 1000). " +
			"N dòng tiếp theo, mỗi dòng có dạng `<cờ> <tên công việc>`, trong đó cờ = 1 là đã xong, 0 là chưa xong; " +
			"tên có thể chứa dấu cách.\n\nHãy in ra tên các công việc CHƯA xong theo đúng thứ tự xuất hiện, mỗi tên một dòng. " +
			"Nếu mọi công việc đều đã xong (hoặc N = 0) thì in `EMPTY`.\n\nĐây chính là logic của bộ lọc \"Active\" trong Todo App bạn vừa dựng bằng React.",
		Difficulty:  "easy",
		Languages:   []string{"javascript", "python"},
		StarterCode: demoTodoStarterCode,
		TestCases: []demoTestCaseSpec{
			{Input: "3\n0 Học useState\n1 Cài Node.js\n0 Làm bài Todo App", Output: "Học useState\nLàm bài Todo App"},
			{Input: "2\n1 Đọc tài liệu\n1 Xem video", Output: "EMPTY"},
			{Input: "0", Output: "EMPTY", Hidden: true},
			{Input: "4\n0 Viết component TodoItem\n0 Thêm nút xoá\n1 Tạo dự án\n0 Lưu vào localStorage",
				Output: "Viết component TodoItem\nThêm nút xoá\nLưu vào localStorage", Hidden: true},
		},
	},
}

// SeedDemoCourseExercises tạo bài tập code + test case (2 công khai, 2 ẩn) và gắn vào content "exercise"
// còn trống của bài thực hành. Chỉ gắn khi exercise_id đang NULL, không đè bài tập giảng viên đã tự gắn.
func (s *Seeder) SeedDemoCourseExercises(courses map[string]model.Course) error {
	for _, spec := range demoExerciseSpecs {
		course, ok := courses[spec.CourseSlug]
		if !ok {
			return fmt.Errorf("exercise %q: course %s not found", spec.Title, spec.CourseSlug)
		}
		author := course.InstructorID

		var content model.LessonContent
		err := s.db.Joins("JOIN lessons ON lessons.id = lesson_contents.lesson_id").
			Joins("JOIN sections ON sections.id = lessons.section_id").
			Where("sections.course_id = ? AND lessons.title = ? AND lesson_contents.type = ?", course.ID, spec.LessonTitle, "exercise").
			First(&content).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("exercise %q: lesson %q of course %s has no exercise content (run SeedDemoCourses first)", spec.Title, spec.LessonTitle, spec.CourseSlug)
		}
		if err != nil {
			return fmt.Errorf("exercise %q: failed to find lesson content: %w", spec.Title, err)
		}

		exercise := model.CourseExercise{Title: spec.Title, Description: spec.Description, Difficulty: spec.Difficulty,
			Language: pq.StringArray(spec.Languages), StarterCode: spec.StarterCode, TimeLimit: 2, MemoryLimit: 256,
			PassPercentage: 100, CreatedBy: &author}
		if err := s.db.Where("title = ? AND created_by = ?", spec.Title, author).
			Attrs(exercise).FirstOrCreate(&exercise).Error; err != nil {
			return fmt.Errorf("failed to seed exercise %q: %w", spec.Title, err)
		}

		for i, tc := range spec.TestCases {
			rec := model.ExerciseTestCase{ExerciseID: exercise.ID, Input: tc.Input, ExpectedOutput: tc.Output,
				IsHidden: tc.Hidden, DisplayOrder: i + 1}
			if err := s.db.Where("exercise_id = ? AND display_order = ?", exercise.ID, i+1).
				Attrs(rec).FirstOrCreate(&rec).Error; err != nil {
				return fmt.Errorf("failed to seed test case %d of %q: %w", i+1, spec.Title, err)
			}
		}

		if err := s.db.Model(&model.LessonContent{}).
			Where("id = ? AND exercise_id IS NULL", content.ID).
			Update("exercise_id", exercise.ID).Error; err != nil {
			return fmt.Errorf("failed to link exercise %q to lesson content: %w", spec.Title, err)
		}
	}
	log.Printf("Seeded %d demo course exercises\n", len(demoExerciseSpecs))
	return nil
}
