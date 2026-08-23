package seeds

// lessonSpec mô tả một bài học và loại nội dung của nó.
type lessonSpec struct {
	Title       string
	DurationMin int
	IsPreview   bool
	ContentType string // video | livestream | exercise
}

// sectionSpec mô tả một chương gồm nhiều bài học.
type sectionSpec struct {
	Title   string
	Lessons []lessonSpec
}

// courseSpec mô tả một khoá học demo đầy đủ.
type courseSpec struct {
	Slug             string
	Title            string
	ShortDescription string
	Description      string
	InstructorEmail  string
	CategorySlug     string
	TagNames         []string
	Level            string
	Price            int64
	DiscountPrice    int64 // 0 = không giảm giá
	IsFree           bool
	IsFeatured       bool
	TotalStudents    int
	Rating           float64
	TotalReviews     int
	Requirements     []string
	Objectives       []string
	TargetAudience   []string
	Sections         []sectionSpec
}

var demoCourses = []courseSpec{
	{
		Slug:             "react-nextjs-tu-co-ban-den-nang-cao",
		Title:            "React + Next.js từ cơ bản đến nâng cao",
		ShortDescription: "Xây dựng ứng dụng web hiện đại với React 19 và Next.js App Router.",
		Description:      "Khoá học đưa bạn đi từ nền tảng React (component, props, state, hooks) đến kiến trúc Next.js App Router với Server Components, data fetching và tối ưu hiệu năng. Kết thúc khoá học bạn tự tin xây dựng và triển khai một sản phẩm web hoàn chỉnh.",
		InstructorEmail:  "teacher1@demo.com",
		CategorySlug:     "lap-trinh-web",
		TagNames:         []string{"JavaScript", "TypeScript", "React", "Next.js"},
		Level:            "intermediate",
		Price:            999000,
		DiscountPrice:    499000,
		IsFeatured:       true,
		TotalStudents:    1250,
		Rating:           4.8,
		TotalReviews:     318,
		Requirements:     []string{"Biết HTML/CSS cơ bản", "Có nền tảng JavaScript ES6"},
		Objectives:       []string{"Thành thạo React Hooks", "Hiểu rõ Server Components", "Triển khai ứng dụng Next.js lên production"},
		TargetAudience:   []string{"Sinh viên CNTT", "Lập trình viên backend muốn học frontend"},
		Sections: []sectionSpec{
			{Title: "Giới thiệu & Cài đặt", Lessons: []lessonSpec{
				{Title: "Giới thiệu khoá học", DurationMin: 10, IsPreview: true, ContentType: "video"},
				{Title: "Cài đặt môi trường phát triển", DurationMin: 12, ContentType: "video"},
				{Title: "Tạo dự án đầu tiên", DurationMin: 15, ContentType: "video"},
			}},
			{Title: "React Fundamentals", Lessons: []lessonSpec{
				{Title: "Components & Props", DurationMin: 20, ContentType: "video"},
				{Title: "State & Hooks", DurationMin: 25, ContentType: "video"},
				{Title: "Bài tập thực hành: Todo App", DurationMin: 30, ContentType: "exercise"},
			}},
			{Title: "Next.js App Router", Lessons: []lessonSpec{
				{Title: "File-based Routing", DurationMin: 15, ContentType: "video"},
				{Title: "Server Components", DurationMin: 20, ContentType: "video"},
				{Title: "Data Fetching & Caching", DurationMin: 25, ContentType: "video"},
			}},
		},
	},
	{
		Slug:             "python-cho-khoa-hoc-du-lieu",
		Title:            "Python cho Khoa học Dữ liệu",
		ShortDescription: "Phân tích và trực quan hoá dữ liệu với NumPy, Pandas, Matplotlib.",
		Description:      "Bắt đầu từ Python cơ bản, khoá học dẫn bạn qua toàn bộ quy trình làm việc với dữ liệu: làm sạch, biến đổi, trực quan hoá và huấn luyện mô hình Machine Learning đầu tiên.",
		InstructorEmail:  "teacher2@demo.com",
		CategorySlug:     "khoa-hoc-du-lieu",
		TagNames:         []string{"Python", "Machine Learning"},
		Level:            "beginner",
		Price:            699000,
		TotalStudents:    890,
		Rating:           4.6,
		TotalReviews:     204,
		Requirements:     []string{"Không yêu cầu kiến thức lập trình trước đó"},
		Objectives:       []string{"Sử dụng thành thạo Pandas", "Trực quan hoá dữ liệu", "Huấn luyện mô hình ML cơ bản"},
		TargetAudience:   []string{"Người mới bắt đầu", "Nhân sự chuyển ngành sang Data"},
		Sections: []sectionSpec{
			{Title: "Python Cơ bản", Lessons: []lessonSpec{
				{Title: "Biến, kiểu dữ liệu và toán tử", DurationMin: 18, IsPreview: true, ContentType: "video"},
				{Title: "Cấu trúc điều khiển và hàm", DurationMin: 22, ContentType: "video"},
			}},
			{Title: "NumPy & Pandas", Lessons: []lessonSpec{
				{Title: "Mảng NumPy và vector hoá", DurationMin: 24, ContentType: "video"},
				{Title: "DataFrame và xử lý dữ liệu thiếu", DurationMin: 28, ContentType: "video"},
			}},
			{Title: "Trực quan hoá & ML", Lessons: []lessonSpec{
				{Title: "Matplotlib & Seaborn", DurationMin: 20, ContentType: "video"},
				{Title: "Mô hình hồi quy đầu tiên", DurationMin: 26, ContentType: "video"},
			}},
		},
	},
	{
		Slug:             "flutter-mobile-development",
		Title:            "Flutter Mobile Development",
		ShortDescription: "Một codebase, hai nền tảng iOS và Android.",
		Description:      "Học Dart và Flutter để xây dựng ứng dụng di động đa nền tảng với giao diện mượt mà, quản lý state hiệu quả và tích hợp API thực tế.",
		InstructorEmail:  "teacher1@demo.com",
		CategorySlug:     "lap-trinh-mobile",
		TagNames:         []string{"Flutter", "Kotlin", "Swift"},
		Level:            "beginner",
		Price:            599000,
		IsFeatured:       true,
		TotalStudents:    640,
		Rating:           4.7,
		TotalReviews:     150,
		Requirements:     []string{"Biết lập trình hướng đối tượng cơ bản"},
		Objectives:       []string{"Xây dựng UI với Flutter widget", "Quản lý state", "Phát hành ứng dụng lên store"},
		TargetAudience:   []string{"Lập trình viên mobile mới", "Sinh viên làm đồ án"},
		Sections: []sectionSpec{
			{Title: "Dart & Flutter Cơ bản", Lessons: []lessonSpec{
				{Title: "Ngôn ngữ Dart trong 30 phút", DurationMin: 30, IsPreview: true, ContentType: "video"},
				{Title: "Widget tree và layout", DurationMin: 25, ContentType: "video"},
			}},
			{Title: "State & Networking", Lessons: []lessonSpec{
				{Title: "Quản lý state với Provider", DurationMin: 22, ContentType: "video"},
				{Title: "Gọi REST API và xử lý lỗi", DurationMin: 24, ContentType: "video"},
			}},
		},
	},
	{
		Slug:             "docker-kubernetes-thuc-chien",
		Title:            "Docker & Kubernetes thực chiến",
		ShortDescription: "Đóng gói, triển khai và vận hành ứng dụng ở quy mô production.",
		Description:      "Khoá học nâng cao về container: viết Dockerfile tối ưu, orchestration với Kubernetes, cấu hình CI/CD và giám sát hệ thống.",
		InstructorEmail:  "teacher2@demo.com",
		CategorySlug:     "devops-cloud",
		TagNames:         []string{"Docker", "Kubernetes", "AWS"},
		Level:            "advanced",
		Price:            799000,
		TotalStudents:    412,
		Rating:           4.9,
		TotalReviews:     97,
		Requirements:     []string{"Thành thạo Linux command line", "Đã từng triển khai ứng dụng web"},
		Objectives:       []string{"Viết Dockerfile tối ưu", "Triển khai cluster Kubernetes", "Thiết lập pipeline CI/CD"},
		TargetAudience:   []string{"Backend developer", "DevOps engineer"},
		Sections: []sectionSpec{
			{Title: "Docker nền tảng", Lessons: []lessonSpec{
				{Title: "Image, container và layer", DurationMin: 20, IsPreview: true, ContentType: "video"},
				{Title: "Docker Compose cho môi trường dev", DurationMin: 22, ContentType: "video"},
			}},
			{Title: "Kubernetes", Lessons: []lessonSpec{
				{Title: "Pod, Deployment, Service", DurationMin: 28, ContentType: "video"},
				{Title: "Buổi live: Debug cluster thực tế", DurationMin: 60, ContentType: "livestream"},
			}},
		},
	},
	{
		Slug:             "git-github-cho-nguoi-moi-bat-dau",
		Title:            "Git & GitHub cho người mới bắt đầu",
		ShortDescription: "Khoá học miễn phí về quản lý mã nguồn và làm việc nhóm.",
		Description:      "Nắm vững Git từ commit đầu tiên đến quy trình làm việc nhóm với branch, pull request và code review trên GitHub.",
		InstructorEmail:  "teacher1@demo.com",
		CategorySlug:     "devops-cloud",
		TagNames:         []string{"Git"},
		Level:            "beginner",
		Price:            0,
		IsFree:           true,
		TotalStudents:    2100,
		Rating:           4.5,
		TotalReviews:     530,
		Requirements:     []string{"Chỉ cần một máy tính"},
		Objectives:       []string{"Sử dụng Git hằng ngày", "Làm việc nhóm qua pull request"},
		TargetAudience:   []string{"Sinh viên năm nhất", "Người mới vào nghề"},
		Sections: []sectionSpec{
			{Title: "Bắt đầu với Git", Lessons: []lessonSpec{
				{Title: "Cài đặt và cấu hình Git", DurationMin: 10, IsPreview: true, ContentType: "video"},
				{Title: "Commit, log và undo", DurationMin: 18, ContentType: "video"},
			}},
			{Title: "Làm việc nhóm với GitHub", Lessons: []lessonSpec{
				{Title: "Branch và merge", DurationMin: 20, ContentType: "video"},
				{Title: "Pull request và code review", DurationMin: 16, ContentType: "video"},
			}},
		},
	},
}
