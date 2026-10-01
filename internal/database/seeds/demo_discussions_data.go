package seeds

// discussionReplySpec — một trả lời trực tiếp dưới bài gốc (parent_id = bài gốc, lesson_id NULL,
// không title/slug/category — đúng shape DiscussionService.AddComment tạo ra).
type discussionReplySpec struct {
	Author, Content string
	Instructor      bool
	HoursAfter      int               // giờ sau bài gốc
	Votes           map[string]string // email -> "upvote" | "downvote"
}

// discussionPostSpec — một bài gốc. CourseSlug/LessonTitle rỗng: bài diễn đàn chung
// (GET /discussions); có giá trị: câu hỏi Q&A gắn bài học (GET /lessons/:id/discussions).
type discussionPostSpec struct {
	Slug, Title, Category, Author, Content string
	Pinned                                 bool
	DaysAgo                                int
	CourseSlug, LessonTitle                string
	VideoSecond                            int
	Votes                                  map[string]string
	Replies                                []discussionReplySpec
}

const (
	dS1, dS2, dT1, dT2 = "student1@demo.com", "student2@demo.com", "teacher1@demo.com", "teacher2@demo.com"
	dAdmin, dParent    = "admin@demo.com", "parent1@demo.com"
	dUp, dDown         = "upvote", "downvote"
)

var demoForumPosts = []discussionPostSpec{
	{Slug: "noi-quy-dien-dan-va-cach-dat-cau-hoi", Title: "Nội quy diễn đàn 40Study và cách đặt câu hỏi hiệu quả",
		Category: "learning-tips", Author: dAdmin, Pinned: true, DaysAgo: 30,
		Content: "Chào mừng các bạn đến với diễn đàn! Khi đặt câu hỏi, hãy ghi rõ bạn đang học khoá nào, đã thử những gì và dán thông báo lỗi đầy đủ (đừng chụp màn hình code). " +
			"Tôn trọng người trả lời, không đăng lời giải bài kiểm tra đang diễn ra. Bài vi phạm sẽ bị ẩn.",
		Votes: map[string]string{dS1: dUp, dS2: dUp, dT1: dUp, dT2: dUp},
		Replies: []discussionReplySpec{
			{Author: dS1, Content: "Cảm ơn admin, mình sẽ ghi rõ phiên bản Node và hệ điều hành khi hỏi.", HoursAfter: 5},
		}},
	{Slug: "hoi-dap-nop-bai-va-nhan-chung-chi", Title: "Hỏi đáp: cách nộp bài tập và nhận chứng chỉ khoá học",
		Category: "qna", Author: dT1, Pinned: true, DaysAgo: 25,
		Content: "Chứng chỉ được cấp tự động khi bạn hoàn thành 100% bài học bắt buộc và đạt bài kiểm tra cuối khoá. " +
			"Bài tập thực hành nộp ngay trong trang bài học, giảng viên chấm trong vòng 3 ngày làm việc. Có thắc mắc gì cứ hỏi dưới bài này nhé.",
		Votes: map[string]string{dS1: dUp, dS2: dUp},
		Replies: []discussionReplySpec{
			{Author: dS2, Content: "Thầy ơi, bài tập nộp trễ hạn có còn được chấm không ạ?", HoursAfter: 20},
			{Author: dT1, Content: "Vẫn được chấm em nhé, chỉ là không tính vào bảng xếp hạng tuần. Chứng chỉ không bị ảnh hưởng.", Instructor: true, HoursAfter: 26,
				Votes: map[string]string{dS2: dUp}},
		}},
	{Slug: "nen-hoc-javascript-hay-typescript-truoc", Title: "Nên học JavaScript hay TypeScript trước?",
		Category: "programming", Author: dS1, DaysAgo: 12,
		Content: "Mình mới học xong HTML/CSS, định học React. Có bạn khuyên học thẳng TypeScript cho đỡ phải học lại. Mọi người nghĩ sao?",
		Votes:   map[string]string{dS2: dUp, dT1: dUp, dT2: dUp},
		Replies: []discussionReplySpec{
			{Author: dT1, Content: "Nên nắm chắc JavaScript trước: scope, closure, Promise, async/await. TypeScript chỉ là lớp kiểu bọc ngoài, học sau 2-3 tuần là theo kịp. " +
				"Trong khoá React thầy dùng JS ở phần đầu rồi mới chuyển sang TS.", Instructor: true, HoursAfter: 3,
				Votes: map[string]string{dS1: dUp, dS2: dUp}},
			{Author: dS2, Content: "Mình thấy TypeScript chỉ làm chậm tiến độ, học JS là đủ đi làm rồi.", HoursAfter: 6,
				Votes: map[string]string{dT2: dDown, dS1: dUp}},
			{Author: dT2, Content: "Dự án thật từ vài nghìn dòng trở lên hầu hết đều dùng TypeScript, bắt lỗi kiểu sớm tiết kiệm rất nhiều thời gian debug. Cứ học JS trước nhưng đừng bỏ TS.",
				Instructor: true, HoursAfter: 8, Votes: map[string]string{dS1: dUp}},
		}},
	{Slug: "loi-cors-khi-goi-api-tu-nextjs-sang-go", Title: "Lỗi CORS khi gọi API từ Next.js sang backend Go",
		Category: "programming", Author: dS2, DaysAgo: 6,
		Content: "Frontend chạy ở localhost:3000, backend Fiber ở localhost:8080. Gọi fetch thì trình duyệt báo " +
			"`No 'Access-Control-Allow-Origin' header is present`. Postman gọi vẫn được. Mình cần sửa ở đâu?",
		Votes: map[string]string{dS1: dUp, dT2: dUp},
		Replies: []discussionReplySpec{
			{Author: dT1, Content: "Postman không áp dụng CORS nên không lỗi. Ở Fiber em thêm middleware `cors.New(cors.Config{AllowOrigins: \"http://localhost:3000\", AllowCredentials: true})` " +
				"trước khi khai báo route. Nếu gửi cookie thì không được dùng `*` cho AllowOrigins.", Instructor: true, HoursAfter: 2,
				Votes: map[string]string{dS2: dUp, dS1: dUp}},
			{Author: dS2, Content: "Chạy được rồi ạ, em đặt middleware sau route nên không ăn. Cảm ơn thầy!", HoursAfter: 4},
		}},
	{Slug: "bo-mau-va-typography-cho-dashboard-hoc-tap", Title: "Chia sẻ bộ màu và typography cho dashboard học tập",
		Category: "design", Author: dT2, DaysAgo: 15,
		Content: "Mình dùng màu chính xanh dương đậm cho hành động, xanh lá cho tiến độ hoàn thành và cam cho hạn nộp sắp tới. " +
			"Font Be Vietnam Pro cho tiêu đề, Inter cho nội dung, cỡ chữ tối thiểu 14px để đọc lâu không mỏi mắt.",
		Votes: map[string]string{dS1: dUp, dS2: dUp, dT1: dUp},
		Replies: []discussionReplySpec{
			{Author: dS1, Content: "Be Vietnam Pro hiển thị dấu tiếng Việt đẹp thật. Cô có file Figma mẫu không ạ?", HoursAfter: 10},
		}},
	{Slug: "giao-dien-mobile-bi-vo-tren-man-hinh-nho", Title: "Giao diện mobile bị vỡ trên màn hình nhỏ, xử lý sao?",
		Category: "design", Author: dS1, DaysAgo: 4,
		Content: "App Flutter của mình trên máy 5 inch bị tràn chữ và nút bấm chồng lên nhau, còn máy 6.5 inch thì ổn. Có nguyên tắc nào để thiết kế co giãn không?",
		Votes:   map[string]string{dT1: dUp},
		Replies: []discussionReplySpec{
			{Author: dT2, Content: "Đừng đặt chiều rộng cố định bằng pixel. Dùng `Expanded`/`Flexible` trong Row, `LayoutBuilder` để đổi bố cục theo bề rộng, " +
				"và test trên thiết bị nhỏ nhất bạn hỗ trợ ngay từ đầu.", Instructor: true, HoursAfter: 5,
				Votes: map[string]string{dS1: dUp}},
		}},
	{Slug: "pomodoro-giup-hoc-deu-moi-ngay", Title: "Phương pháp Pomodoro giúp mình học đều mỗi ngày",
		Category: "learning-tips", Author: dS2, DaysAgo: 9,
		Content: "Mình học 25 phút, nghỉ 5 phút, sau 4 lượt thì nghỉ dài 20 phút. Mỗi lượt chỉ làm đúng một bài học hoặc một bài tập. Sau 3 tuần mình đã theo kịp lộ trình khoá Python.",
		Votes:   map[string]string{dS1: dUp, dParent: dUp},
		Replies: []discussionReplySpec{
			{Author: dS1, Content: "Mình thử rồi, hiệu quả nhất là tắt thông báo điện thoại trong 25 phút đó.", HoursAfter: 7},
			{Author: dParent, Content: "Con nhà tôi cũng áp dụng, phụ huynh nhìn lịch học rõ ràng hơn hẳn.", HoursAfter: 30},
		}},
	{Slug: "dong-hanh-cung-con-tu-hoc-lap-trinh", Title: "Làm sao để đồng hành cùng con khi con tự học lập trình?",
		Category: "learning-tips", Author: dParent, DaysAgo: 7,
		Content: "Tôi không biết lập trình nhưng muốn theo dõi và động viên con. Nên xem những chỉ số nào và nói chuyện với con thế nào cho đúng?",
		Votes:   map[string]string{dT1: dUp, dS2: dUp},
		Replies: []discussionReplySpec{
			{Author: dT1, Content: "Anh/chị xem tiến độ hoàn thành và điểm bài kiểm tra trong trang phụ huynh là đủ. Thay vì hỏi \"học đến đâu rồi\", " +
				"hãy nhờ con demo sản phẩm con làm được mỗi tuần, con sẽ có động lực hơn nhiều.", Instructor: true, HoursAfter: 4,
				Votes: map[string]string{dParent: dUp}},
		}},
	{Slug: "showcase-app-quan-ly-chi-tieu-flutter", Title: "Showcase: ứng dụng quản lý chi tiêu viết bằng Flutter",
		Category: "project", Author: dS1, DaysAgo: 3,
		Content: "Sau khoá Flutter mình làm app ghi chi tiêu: thêm/sửa khoản chi, biểu đồ theo tháng, lưu offline bằng Hive. Mọi người góp ý giúp mình phần kiến trúc nhé.",
		Votes:   map[string]string{dS2: dUp, dT1: dUp, dT2: dUp},
		Replies: []discussionReplySpec{
			{Author: dT1, Content: "Làm tốt lắm! Gợi ý: tách phần truy cập Hive ra một repository để sau này đổi sang API không phải sửa UI, và viết test cho hàm tính tổng theo tháng.",
				Instructor: true, HoursAfter: 6, Votes: map[string]string{dS1: dUp}},
			{Author: dS2, Content: "Biểu đồ đẹp quá, bạn dùng thư viện fl_chart à?", HoursAfter: 9},
		}},
	{Slug: "tim-ban-lam-do-an-web-ban-sach", Title: "Tìm bạn cùng làm đồ án web bán sách (Next.js + Go)",
		Category: "project", Author: dS2, DaysAgo: 2,
		Content: "Nhóm mình đang cần thêm 1 bạn làm frontend Next.js cho đồ án môn học: trang danh mục, giỏ hàng và thanh toán giả lập. Họp online tối thứ 3 và thứ 6.",
		Votes:   map[string]string{dS1: dUp},
		Replies: []discussionReplySpec{
			{Author: dS1, Content: "Mình đang học khoá React + Next.js, muốn tham gia phần giỏ hàng. Mình nhắn riêng nhé!", HoursAfter: 3},
		}},
}

// Câu hỏi theo bài học — chỉ ở các khoá student1 đã ghi danh (demoEnrollments), giảng viên khoá
// (teacher1) trả lời.
var demoLessonQuestions = []discussionPostSpec{
	{Slug: "hoi-dap-setstate-khong-cap-nhat-ngay", Title: "Tại sao setState không cập nhật ngay giá trị trong cùng một hàm?",
		Category: "qna", Author: dS1, DaysAgo: 8, CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", LessonTitle: "State & Hooks", VideoSecond: 4,
		Content: "Em gọi `setCount(count + 1)` rồi `console.log(count)` ngay sau đó nhưng vẫn ra giá trị cũ. Em làm sai chỗ nào ạ?",
		Replies: []discussionReplySpec{
			{Author: dT1, Content: "Không sai đâu em: state chỉ đổi ở lần render tiếp theo. Muốn dùng giá trị mới thì đọc trong lần render sau hoặc trong `useEffect` phụ thuộc `count`.",
				Instructor: true, HoursAfter: 3},
		}},
	{Slug: "hoi-dap-server-component-dung-usestate", Title: "Server Component có dùng được useState không?",
		Category: "qna", Author: dS1, DaysAgo: 5, CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao", LessonTitle: "Server Components", VideoSecond: 3,
		Content: "Em thêm `useState` vào page.tsx thì Next.js báo lỗi. Vậy muốn có nút bấm đổi trạng thái thì phải làm sao ạ?",
		Replies: []discussionReplySpec{
			{Author: dT1, Content: "Server Component không có state. Em tách nút bấm ra một component riêng có dòng `\"use client\"` ở đầu file rồi import vào page là được.",
				Instructor: true, HoursAfter: 2},
		}},
	{Slug: "hoi-dap-xu-ly-merge-conflict", Title: "Merge conflict khi hai người cùng sửa một dòng thì xử lý thế nào?",
		Category: "qna", Author: dS1, DaysAgo: 20, CourseSlug: "git-github-cho-nguoi-moi-bat-dau", LessonTitle: "Branch và merge", VideoSecond: 2,
		Content: "Em và bạn cùng sửa file README, khi merge thì Git báo CONFLICT. Em nên giữ bản nào và làm các bước gì ạ?",
		Replies: []discussionReplySpec{
			{Author: dT1, Content: "Mở file, tìm các dấu `<<<<<<<`, `=======`, `>>>>>>>`, sửa thành nội dung cuối cùng cả nhóm thống nhất rồi xoá các dấu đó. " +
				"Sau đó `git add README.md` và `git commit` để hoàn tất merge.", Instructor: true, HoursAfter: 5},
		}},
	{Slug: "hoi-dap-expanded-va-flexible", Title: "Khi nào dùng Expanded và khi nào dùng Flexible?",
		Category: "qna", Author: dS1, DaysAgo: 3, CourseSlug: "flutter-mobile-development", LessonTitle: "Widget tree và layout", VideoSecond: 3,
		Content: "Trong video thầy dùng Expanded, em thử Flexible cũng chạy. Hai cái khác nhau ở điểm nào ạ?",
		Replies: []discussionReplySpec{
			{Author: dT1, Content: "`Expanded` bắt con chiếm hết phần còn lại (fit: tight), còn `Flexible` cho con nhỏ hơn nếu nội dung không cần nhiều chỗ (fit: loose).",
				Instructor: true, HoursAfter: 4},
		}},
}
