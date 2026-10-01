package seeds

import "study.com/v1/internal/model"

// demo_class_data.go — dữ liệu khai báo cho SeedDemoClasses (lớp học, lịch, livestream, bài tập,
// bài nộp, điểm). Tách khỏi logic seed để sửa nội dung demo không phải đọc code GORM.

// classSpec mô tả một lớp demo và mọi thứ gắn với nó.
type classSpec struct {
	Name          string
	Description   string
	CourseSlug    string
	TeacherEmail  string
	StudentEmails []string
	Days          []int           // 0=Chủ nhật ... 6=Thứ bảy (khớp CHECK day_of_week của class_schedules)
	StartTime     model.TimeOfDay // "HH:MM", giờ địa phương
	EndTime       model.TimeOfDay
	Room          string
	Location      string // ghi vào session_attendances.location: online | offline
	MaxStudents   int
	StartDaysAgo  int // ngày khai giảng, tính lùi từ hôm nay ở lần seed đầu tiên
	DurationWeeks int
	RoomKey       string   // tiền tố room_name (unique) của phiên livestream
	Topics        []string // chủ đề buổi học, xoay vòng theo số buổi
	Language      string   // ngôn ngữ lập trình của bài tập
	Lives         []liveSpec
	Assignments   []assignmentSpec
	ExtraGrades   []extraGradeSpec
}

// liveSpec: phiên livestream của lớp. OffsetDays âm = đã diễn ra (ended), dương = sắp tới (scheduled).
type liveSpec struct {
	Title       string
	Description string
	OffsetDays  int
	Hour, Min   int
}

// assignmentSpec: bài tập về nhà/dự án gắn vào một phiên livestream (LiveIndex) và lớp.
type assignmentSpec struct {
	Title         string
	Description   string
	Type          string // homework | project (CHECK type của assignments)
	Difficulty    model.AssignmentDifficulty
	LiveIndex     int
	DueOffsetDays int // hạn nộp: âm = đã qua, dương = sắp tới
	Published     bool
	StarterCode   string
	Solution      string
	TestCases     [][2]string // {input, expected}; test cuối cùng là test ẩn
	Results       []submissionSpec
}

// submissionSpec: bài nộp của một học sinh. Score > 0 nghĩa là giáo viên đã chấm (sinh bản ghi grades).
type submissionSpec struct {
	StudentEmail  string
	Verdict       model.SubmissionVerdict
	Passed        int
	DaysBeforeDue int // nộp trước hạn bao nhiêu ngày (âm = nộp muộn)
	Score         float64
	Feedback      string
}

// extraGradeSpec: cột điểm không gắn bài tập (giữa kỳ, chuyên cần...).
type extraGradeSpec struct {
	StudentEmail string
	Type         model.GradeType
	Title        string
	Score        float64
	Weight       float64
	DaysAgo      int
	Feedback     string
}

// demoClassSpecs trả về danh sách lớp demo theo thứ tự seed.
func demoClassSpecs() []classSpec {
	return []classSpec{reactClassSpec(), flutterClassSpec(), pythonClassSpec()}
}

func reactClassSpec() classSpec {
	return classSpec{
		Name:          "Lớp ReactJS K12 - Tối 2-4-6",
		Description:   "Lớp học trực tiếp kèm khoá React + Next.js: học buổi tối thứ 2, 4, 6, chữa bài qua livestream mỗi tuần.",
		CourseSlug:    "react-nextjs-tu-co-ban-den-nang-cao",
		TeacherEmail:  "teacher1@demo.com",
		StudentEmails: []string{"student1@demo.com", "student2@demo.com"},
		Days:          []int{1, 3, 5},
		StartTime:     "19:00",
		EndTime:       "21:00",
		Room:          "Phòng 301 - 40Study Cầu Giấy",
		Location:      "offline",
		MaxStudents:   25,
		StartDaysAgo:  28,
		DurationWeeks: 12,
		RoomKey:       "react-k12",
		Language:      "javascript",
		Topics: []string{
			"JSX và cách React render giao diện",
			"Components & Props: chia nhỏ giao diện",
			"State với useState, xử lý sự kiện",
			"useEffect và vòng đời component",
			"Làm việc với danh sách, key và form",
			"Custom hook: tách logic dùng lại",
			"Gọi API với fetch, xử lý loading/error",
			"Context API cho state dùng chung",
			"Next.js App Router: routing theo file",
			"Server Components và data fetching",
		},
		Lives: []liveSpec{
			{Title: "Live chữa bài: Props và danh sách", Description: "Chữa bài tập tuần 2, giải đáp lỗi key trong list.", OffsetDays: -12, Hour: 20, Min: 0},
			{Title: "Live chữa bài: useEffect và gọi API", Description: "Review code bài tập custom hook, hướng dẫn xử lý race condition khi fetch.", OffsetDays: -4, Hour: 20, Min: 0},
			{Title: "Live hỏi đáp: chuẩn bị đồ án Next.js", Description: "Giới thiệu đề đồ án cuối khoá, chia nhóm và hỏi đáp.", OffsetDays: 3, Hour: 20, Min: 30},
		},
		Assignments: []assignmentSpec{
			{
				Title:       "Tính tổng giỏ hàng",
				Description: "Viết hàm cartTotal(items) nhận mảng {price, qty} và trả về tổng tiền. Bỏ qua sản phẩm có qty <= 0.",
				Type:        "homework", Difficulty: model.DifficultyEasy, LiveIndex: 0, DueOffsetDays: -9, Published: true,
				StarterCode: "function cartTotal(items) {\n  // TODO\n}\n",
				Solution:    "function cartTotal(items) {\n  return items.filter((i) => i.qty > 0).reduce((sum, i) => sum + i.price * i.qty, 0);\n}\n",
				TestCases:   [][2]string{{`[{"price":100,"qty":2}]`, "200"}, {`[]`, "0"}, {`[{"price":50,"qty":-1},{"price":30,"qty":3}]`, "90"}},
				Results: []submissionSpec{
					{StudentEmail: "student1@demo.com", Verdict: model.VerdictAccepted, Passed: 3, DaysBeforeDue: 2, Score: 10, Feedback: "Dùng filter + reduce rất gọn, đặt tên biến rõ ràng. Tốt!"},
					{StudentEmail: "student2@demo.com", Verdict: model.VerdictWrongAnswer, Passed: 2, DaysBeforeDue: 0, Score: 6.5, Feedback: "Chưa loại sản phẩm có số lượng âm nên sai test ẩn. Đọc kỹ đề và thêm điều kiện lọc nhé."},
				},
			},
			{
				Title:       "Custom hook useDebounce",
				Description: "Viết hook useDebounce(value, delay) trả về giá trị chỉ cập nhật sau delay ms kể từ lần thay đổi cuối. Dùng cho ô tìm kiếm khoá học.",
				Type:        "homework", Difficulty: model.DifficultyMedium, LiveIndex: 1, DueOffsetDays: -1, Published: true,
				StarterCode: "import { useEffect, useState } from 'react';\n\nexport function useDebounce(value, delay) {\n  // TODO\n}\n",
				Solution:    "import { useEffect, useState } from 'react';\n\nexport function useDebounce(value, delay) {\n  const [debounced, setDebounced] = useState(value);\n  useEffect(() => {\n    const id = setTimeout(() => setDebounced(value), delay);\n    return () => clearTimeout(id);\n  }, [value, delay]);\n  return debounced;\n}\n",
				TestCases:   [][2]string{{"react 300", "react"}, {"next 0", "next"}, {"a,ab,abc 200", "abc"}},
				Results: []submissionSpec{
					{StudentEmail: "student1@demo.com", Verdict: model.VerdictAccepted, Passed: 3, DaysBeforeDue: 1, Score: 9, Feedback: "Đã nhớ clearTimeout trong cleanup. Có thể thêm test cho delay = 0."},
				},
			},
			{
				Title:       "Mini project: Trang danh sách khoá học",
				Description: "Xây trang /courses bằng Next.js App Router: lấy danh sách khoá học từ API, có ô tìm kiếm dùng useDebounce và phân trang.",
				Type:        "project", Difficulty: model.DifficultyHard, LiveIndex: 1, DueOffsetDays: 6, Published: true,
				StarterCode: "export default async function CoursesPage() {\n  // TODO: fetch và render danh sách khoá học\n}\n",
				Solution:    "export default async function CoursesPage({ searchParams }) {\n  const res = await fetch(`${process.env.API_URL}/courses?page=${searchParams.page ?? 1}`);\n  const { data } = await res.json();\n  return <CourseList courses={data} />;\n}\n",
				TestCases:   [][2]string{{"page=1", "200 OK"}, {"page=2", "200 OK"}, {"q=react", "có kết quả"}},
				Results: []submissionSpec{
					{StudentEmail: "student1@demo.com", Verdict: model.VerdictPending, Passed: 0, DaysBeforeDue: 4},
				},
			},
			{
				Title:       "Form đăng ký có kiểm tra dữ liệu",
				Description: "Làm form đăng ký học thử với React Hook Form: kiểm tra email, số điện thoại Việt Nam và hiển thị lỗi ngay dưới ô nhập.",
				Type:        "homework", Difficulty: model.DifficultyMedium, LiveIndex: 2, DueOffsetDays: 10, Published: false,
				StarterCode: "export function SignupForm() {\n  // TODO\n}\n",
				Solution:    "export function SignupForm() {\n  return null;\n}\n",
				TestCases:   [][2]string{{"email=abc", "Email không hợp lệ"}, {"phone=0912345678", "hợp lệ"}, {"phone=123", "Số điện thoại không hợp lệ"}},
			},
		},
		ExtraGrades: []extraGradeSpec{
			{StudentEmail: "student1@demo.com", Type: model.GradeMidterm, Title: "Kiểm tra giữa kỳ - React cơ bản", Score: 8.5, Weight: 0.3, DaysAgo: 6, Feedback: "Nắm chắc hook, cần luyện thêm phần tối ưu render."},
			{StudentEmail: "student2@demo.com", Type: model.GradeMidterm, Title: "Kiểm tra giữa kỳ - React cơ bản", Score: 6, Weight: 0.3, DaysAgo: 6, Feedback: "Còn nhầm giữa props và state, nên xem lại bài 3."},
		},
	}
}
