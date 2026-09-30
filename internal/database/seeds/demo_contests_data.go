package seeds

import (
	"time"

	"study.com/v1/internal/model"
)

// contestPrizeSpec — một khoảng hạng nhận chứng nhận (CHECK chk_contests_prizes_rank: from >= 1, to >= from).
type contestPrizeSpec struct{ From, To int }

// contestSeedSpec mô tả một cuộc thi demo bổ sung. Mốc thời gian là độ lệch so với lúc seed:
//   - ReassignWindow=true: ghi lại start/end MỖI lần seed (UPCOMING/DRAFT/PENDING_REVIEW luôn ở
//     tương lai — service chỉ cho gửi duyệt/duyệt khi start_time > now).
//   - ReassignWindow=false: chỉ đặt lúc tạo; dùng cho cuộc thi ENDED, mốc quá khứ giữ nguyên nên
//     attempt đã nộp luôn nằm trong cửa sổ thi.
type contestSeedSpec struct {
	Slug, Title, Description string
	QuizTitle                string
	CreatorEmail             string
	Status                   string
	StartOffset, EndOffset   time.Duration
	ReassignWindow           bool
	DurationMinutes          int
	MaxParticipants          int
	Prizes                   []contestPrizeSpec
	Questions                []demoQuestionSpec
}

// contestJoinSpec — một lượt đăng ký. Correct < 0: chỉ đăng ký, chưa làm bài (NOT_STARTED);
// Correct >= 0: đã nộp bài với đúng Correct câu đầu tiên (SUBMITTED).
type contestJoinSpec struct {
	Slug, Email string
	Correct     int
}

const contestDay = 24 * time.Hour

var demoExtraContests = []contestSeedSpec{
	{
		Slug: "demo-thi-javascript-sap-dien-ra", Title: "Thử thách JavaScript ES6+ tháng 10",
		Description:  "45 phút ôn lại let/const, arrow function, destructuring và Promise. Top 3 nhận chứng nhận.",
		QuizTitle:    "Đề thi demo: JavaScript ES6+ (sắp diễn ra)",
		CreatorEmail: "teacher1@demo.com", Status: model.ContestStatusPublished,
		StartOffset: 3 * contestDay, EndOffset: 4 * contestDay, ReassignWindow: true,
		DurationMinutes: 45, MaxParticipants: 200,
		Prizes:    []contestPrizeSpec{{1, 1}, {2, 3}},
		Questions: contestQuestionsJS,
	},
	{
		Slug: "demo-thi-react-hooks-sap-dien-ra", Title: "React Hooks Challenge",
		Description:  "Kiểm tra hiểu biết về useState, useEffect và quy tắc của Hooks. Mở cho mọi học viên.",
		QuizTitle:    "Đề thi demo: React Hooks (sắp diễn ra)",
		CreatorEmail: "teacher2@demo.com", Status: model.ContestStatusPublished,
		StartOffset: 10 * contestDay, EndOffset: 12 * contestDay, ReassignWindow: true,
		DurationMinutes: 30,
		Prizes:          []contestPrizeSpec{{1, 3}},
		Questions:       contestQuestionsReact,
	},
	{
		Slug: "demo-thi-sql-da-ket-thuc", Title: "SQL cơ bản: SELECT và JOIN",
		Description:  "Cuộc thi đã kết thúc, đang chờ ban tổ chức chốt kết quả.",
		QuizTitle:    "Đề thi demo: SQL cơ bản (đã kết thúc)",
		CreatorEmail: "teacher1@demo.com", Status: model.ContestStatusPublished,
		StartOffset: -3 * contestDay, EndOffset: -2 * contestDay,
		DurationMinutes: 30,
		Prizes:          []contestPrizeSpec{{1, 3}},
		Questions:       contestQuestionsSQL,
	},
	{
		Slug: "demo-thi-python-cho-duyet", Title: "Python cho người mới: vòng loại",
		Description:  "Đề gồm câu hỏi về kiểu dữ liệu, list comprehension và hàm. Đang chờ quản trị viên duyệt.",
		QuizTitle:    "Đề thi demo: Python cơ bản (chờ duyệt)",
		CreatorEmail: "teacher2@demo.com", Status: model.ContestStatusPendingReview,
		StartOffset: 14 * contestDay, EndOffset: 15 * contestDay, ReassignWindow: true,
		DurationMinutes: 40, MaxParticipants: 100,
		Prizes:    []contestPrizeSpec{{1, 1}, {2, 5}},
		Questions: contestQuestionsPython,
	},
	{
		Slug: "demo-thi-cau-truc-du-lieu-ban-nhap", Title: "Cấu trúc dữ liệu & giải thuật (bản nháp)",
		Description:  "Bản nháp của giảng viên: stack, queue và độ phức tạp thuật toán.",
		QuizTitle:    "Đề thi demo: Cấu trúc dữ liệu (bản nháp)",
		CreatorEmail: "teacher2@demo.com", Status: model.ContestStatusDraft,
		StartOffset: 21 * contestDay, EndOffset: 22 * contestDay, ReassignWindow: true,
		DurationMinutes: 60,
		Questions:       contestQuestionsDSA,
	},
}

// student1 có mặt ở UPCOMING, ACTIVE, ENDED (+ FINALIZED từ cuộc thi Git đã chốt).
var demoContestJoins = []contestJoinSpec{
	{Slug: "demo-thi-javascript-sap-dien-ra", Email: "student1@demo.com", Correct: -1},
	{Slug: "demo-thi-javascript-sap-dien-ra", Email: "student2@demo.com", Correct: -1},
	{Slug: "demo-thi-git-dang-dien-ra", Email: "student1@demo.com", Correct: -1},
	{Slug: "demo-thi-sql-da-ket-thuc", Email: "student1@demo.com", Correct: 3},
	{Slug: "demo-thi-sql-da-ket-thuc", Email: "student2@demo.com", Correct: 2},
}

func cqSingle(text string, answers ...demoAnswerSpec) demoQuestionSpec {
	return demoQuestionSpec{Text: text, Type: "single_choice", Answers: answers}
}

func cqRight(text string) demoAnswerSpec { return demoAnswerSpec{Text: text, IsCorrect: true} }
func cqWrong(text string) demoAnswerSpec { return demoAnswerSpec{Text: text} }

func cqTrueFalse(text string, isTrue bool) demoQuestionSpec {
	return demoQuestionSpec{Text: text, Type: "true_false",
		Answers: []demoAnswerSpec{{Text: "Đúng", IsCorrect: isTrue}, {Text: "Sai", IsCorrect: !isTrue}}}
}

func cqMulti(text string, answers ...demoAnswerSpec) demoQuestionSpec {
	return demoQuestionSpec{Text: text, Type: "multiple_choice", Answers: answers}
}

var contestQuestionsJS = []demoQuestionSpec{
	cqSingle("Kết quả của `typeof null` trong JavaScript là gì?", cqRight(`"object"`), cqWrong(`"null"`), cqWrong(`"undefined"`), cqWrong(`"number"`)),
	cqTrueFalse("Biến khai báo bằng `const` trỏ tới một object vẫn có thể thay đổi thuộc tính của object đó.", true),
	cqMulti("Những cú pháp nào được giới thiệu từ ES6? (chọn nhiều đáp án)",
		cqRight("Arrow function"), cqRight("Template literal"), cqWrong("`var`"), cqRight("Destructuring")),
	cqSingle("`Promise.all` trả về gì khi một promise trong mảng bị reject?",
		cqRight("Reject ngay với lỗi của promise đó"), cqWrong("Bỏ qua promise lỗi"), cqWrong("Trả về mảng rỗng"), cqWrong("Chờ hết rồi resolve")),
}

var contestQuestionsReact = []demoQuestionSpec{
	cqSingle("Mảng phụ thuộc rỗng `[]` trong `useEffect` có nghĩa là gì?",
		cqRight("Effect chỉ chạy sau lần render đầu tiên"), cqWrong("Effect chạy sau mọi lần render"), cqWrong("Effect không bao giờ chạy"), cqWrong("Effect chạy trước khi render")),
	cqTrueFalse("Có thể gọi Hook bên trong câu lệnh `if` miễn là điều kiện luôn đúng.", false),
	cqMulti("Hook nào có sẵn trong React? (chọn nhiều đáp án)", cqRight("useMemo"), cqRight("useRef"), cqWrong("useFetch"), cqRight("useReducer")),
	cqSingle("Cách cập nhật state dựa trên giá trị trước đó an toàn nhất là gì?",
		cqRight("setCount(c => c + 1)"), cqWrong("setCount(count++)"), cqWrong("count = count + 1"), cqWrong("this.setState(count + 1)")),
}

var contestQuestionsSQL = []demoQuestionSpec{
	cqSingle("Mệnh đề nào dùng để lọc kết quả SAU khi đã GROUP BY?", cqRight("HAVING"), cqWrong("WHERE"), cqWrong("ORDER BY"), cqWrong("LIMIT")),
	cqSingle("`LEFT JOIN` trả về gì với các dòng bên trái không có dòng khớp bên phải?",
		cqRight("Vẫn trả về, cột bên phải là NULL"), cqWrong("Bị loại khỏi kết quả"), cqWrong("Báo lỗi"), cqWrong("Lặp lại dòng bên phải đầu tiên")),
	cqTrueFalse("`COUNT(*)` đếm cả các dòng có giá trị NULL.", true),
	cqMulti("Những hàm nào là hàm tổng hợp (aggregate)? (chọn nhiều đáp án)", cqRight("SUM"), cqRight("AVG"), cqWrong("UPPER"), cqRight("MAX")),
}

var contestQuestionsPython = []demoQuestionSpec{
	cqSingle("Kết quả của `[x * 2 for x in range(3)]` là gì?", cqRight("[0, 2, 4]"), cqWrong("[2, 4, 6]"), cqWrong("[0, 1, 2]"), cqWrong("(0, 2, 4)")),
	cqTrueFalse("Tuple trong Python là kiểu dữ liệu bất biến (immutable).", true),
	cqSingle("Từ khoá nào dùng để định nghĩa hàm?", cqRight("def"), cqWrong("function"), cqWrong("func"), cqWrong("lambda def")),
	cqMulti("Kiểu nào là kiểu có thể thay đổi (mutable)? (chọn nhiều đáp án)", cqRight("list"), cqRight("dict"), cqWrong("str"), cqRight("set")),
}

var contestQuestionsDSA = []demoQuestionSpec{
	cqSingle("Stack hoạt động theo nguyên tắc nào?", cqRight("LIFO - vào sau ra trước"), cqWrong("FIFO - vào trước ra trước"), cqWrong("Ngẫu nhiên"), cqWrong("Theo độ ưu tiên")),
	cqSingle("Độ phức tạp tìm kiếm nhị phân trên mảng đã sắp xếp là?", cqRight("O(log n)"), cqWrong("O(n)"), cqWrong("O(n log n)"), cqWrong("O(1)")),
	cqTrueFalse("Queue phù hợp để cài đặt duyệt đồ thị theo chiều rộng (BFS).", true),
}
