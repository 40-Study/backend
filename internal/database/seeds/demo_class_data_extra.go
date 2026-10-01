package seeds

import "study.com/v1/internal/model"

// demo_class_data_extra.go — hai lớp demo còn lại (Flutter của teacher1, Python của teacher2).
// Tách khỏi demo_class_data.go để mỗi file dưới ~250 dòng.

func flutterClassSpec() classSpec {
	return classSpec{
		Name:          "Lớp Flutter Mobile K5 - Sáng thứ 7",
		Description:   "Lớp cuối tuần học online, làm app đặt lịch học từ đầu đến khi build được file APK.",
		CourseSlug:    "flutter-mobile-development",
		TeacherEmail:  "teacher1@demo.com",
		StudentEmails: []string{"student1@demo.com"},
		Days:          []int{6},
		StartTime:     "08:30:00",
		EndTime:       "11:30:00",
		Room:          "Online qua Google Meet",
		Location:      "online",
		MaxStudents:   20,
		StartDaysAgo:  35,
		DurationWeeks: 10,
		RoomKey:       "flutter-k5",
		Language:      "dart",
		Topics: []string{
			"Dart cơ bản: biến, hàm, null safety",
			"Widget tree, StatelessWidget và StatefulWidget",
			"Layout với Row, Column, Stack",
			"Điều hướng giữa các màn hình",
			"Quản lý state với Provider",
			"Gọi REST API bằng package http",
		},
		Lives: []liveSpec{
			{Title: "Live chữa bài: Layout màn hình đăng nhập", Description: "Chữa lỗi tràn màn hình (overflow) và cách dùng Expanded.", OffsetDays: -8, Hour: 10, Min: 0},
			{Title: "Live code cùng giảng viên: Provider", Description: "Cùng viết giỏ hàng bằng Provider, giải thích notifyListeners.", OffsetDays: 5, Hour: 9, Min: 0},
		},
		Assignments: []assignmentSpec{
			{
				Title:       "Định dạng giá tiền VNĐ",
				Description: "Viết hàm formatVnd(int amount) trả về chuỗi có dấu chấm phân cách hàng nghìn và hậu tố ' ₫', ví dụ 1500000 -> '1.500.000 ₫'.",
				Type:        "homework", Difficulty: model.DifficultyEasy, LiveIndex: 0, DueOffsetDays: -5, Published: true,
				StarterCode: "String formatVnd(int amount) {\n  // TODO\n}\n",
				Solution:    "String formatVnd(int amount) {\n  final s = amount.toString().replaceAllMapped(RegExp(r'\\B(?=(\\d{3})+(?!\\d))'), (m) => '.');\n  return '$s ₫';\n}\n",
				TestCases:   [][2]string{{"1500000", "1.500.000 ₫"}, {"0", "0 ₫"}, {"999", "999 ₫"}},
				Results: []submissionSpec{
					{StudentEmail: "student1@demo.com", Verdict: model.VerdictAccepted, Passed: 3, DaysBeforeDue: 1, Score: 9.5, Feedback: "Regex gọn, xử lý đúng số 0. Rất tốt."},
				},
			},
			{
				Title:       "Màn hình danh sách khoá học",
				Description: "Dựng màn hình ListView hiển thị khoá học (ảnh, tên, giá) từ dữ liệu JSON mẫu, bấm vào mở trang chi tiết.",
				Type:        "homework", Difficulty: model.DifficultyMedium, LiveIndex: 0, DueOffsetDays: 2, Published: true,
				StarterCode: "class CourseListScreen extends StatelessWidget {\n  // TODO\n}\n",
				Solution:    "class CourseListScreen extends StatelessWidget {\n  const CourseListScreen({super.key});\n  @override\n  Widget build(BuildContext context) => const Placeholder();\n}\n",
				TestCases:   [][2]string{{"3 khoá", "hiển thị 3 item"}, {"0 khoá", "hiển thị trạng thái trống"}, {"tap item 1", "mở trang chi tiết"}},
			},
			{
				Title:       "Giỏ hàng với Provider",
				Description: "Thêm tính năng giỏ hàng: thêm/xoá khoá học, hiển thị tổng tiền trên AppBar, dùng ChangeNotifierProvider.",
				Type:        "homework", Difficulty: model.DifficultyMedium, LiveIndex: 1, DueOffsetDays: 12, Published: false,
				StarterCode: "class CartModel extends ChangeNotifier {\n  // TODO\n}\n",
				Solution:    "class CartModel extends ChangeNotifier {}\n",
				TestCases:   [][2]string{{"add 1", "tổng 1 item"}, {"add 1 remove 1", "giỏ trống"}, {"add 2", "tổng tiền đúng"}},
			},
		},
	}
}

func pythonClassSpec() classSpec {
	return classSpec{
		Name:          "Lớp Python Data K3 - Tối 3-5",
		Description:   "Lớp thực hành phân tích dữ liệu với Pandas trên bộ dữ liệu điểm thi và doanh thu bán hàng thật.",
		CourseSlug:    "python-cho-khoa-hoc-du-lieu",
		TeacherEmail:  "teacher2@demo.com",
		StudentEmails: []string{"student1@demo.com", "student2@demo.com"},
		Days:          []int{2, 4},
		StartTime:     "19:30:00",
		EndTime:       "21:30:00",
		Room:          "Phòng 205 - 40Study Thủ Đức",
		Location:      "offline",
		MaxStudents:   30,
		StartDaysAgo:  21,
		DurationWeeks: 10,
		RoomKey:       "python-k3",
		Language:      "python",
		Topics: []string{
			"Ôn Python: list, dict, comprehension",
			"NumPy: mảng và phép toán vector",
			"Pandas: đọc CSV, lọc và sắp xếp",
			"Xử lý dữ liệu thiếu và trùng lặp",
			"groupby và bảng tổng hợp",
			"Trực quan hoá với Matplotlib",
		},
		Lives: []liveSpec{
			{Title: "Live chữa bài: Làm sạch dữ liệu điểm thi", Description: "Chữa bài tập xử lý NaN, so sánh dropna và fillna.", OffsetDays: -6, Hour: 20, Min: 0},
			{Title: "Live hỏi đáp: groupby nâng cao", Description: "Giải đáp các câu hỏi về agg, pivot_table trước buổi kiểm tra.", OffsetDays: 8, Hour: 20, Min: 0},
		},
		Assignments: []assignmentSpec{
			{
				Title:       "Điểm trung bình theo lớp",
				Description: "Cho DataFrame gồm cột lop, diem. Viết hàm diem_tb(df) trả về Series điểm trung bình mỗi lớp, làm tròn 2 chữ số, bỏ qua giá trị thiếu.",
				Type:        "homework", Difficulty: model.DifficultyEasy, LiveIndex: 0, DueOffsetDays: -3, Published: true,
				StarterCode: "import pandas as pd\n\ndef diem_tb(df: pd.DataFrame) -> pd.Series:\n    pass\n",
				Solution:    "import pandas as pd\n\ndef diem_tb(df: pd.DataFrame) -> pd.Series:\n    return df.dropna(subset=['diem']).groupby('lop')['diem'].mean().round(2)\n",
				TestCases:   [][2]string{{"10A1: 8, 9", "10A1 8.5"}, {"10A2: 7, NaN", "10A2 7.0"}, {"rỗng", "Series rỗng"}},
				Results: []submissionSpec{
					{StudentEmail: "student1@demo.com", Verdict: model.VerdictAccepted, Passed: 3, DaysBeforeDue: 1, Score: 10, Feedback: "Chính xác, nhớ dropna trước groupby. Có thể dùng .agg để tính nhiều chỉ số cùng lúc."},
					{StudentEmail: "student2@demo.com", Verdict: model.VerdictWrongAnswer, Passed: 1, DaysBeforeDue: -1, Score: 5, Feedback: "Nộp muộn 1 ngày và chưa xử lý NaN nên trung bình bị sai. Xem lại bài 4 nhé."},
				},
			},
			{
				Title:       "Top 5 sản phẩm bán chạy",
				Description: "Từ file doanh_thu.csv, tìm 5 sản phẩm có tổng doanh thu cao nhất trong quý 3 và vẽ biểu đồ cột ngang.",
				Type:        "homework", Difficulty: model.DifficultyMedium, LiveIndex: 0, DueOffsetDays: 4, Published: true,
				StarterCode: "import pandas as pd\n\ndef top5(df: pd.DataFrame) -> pd.DataFrame:\n    pass\n",
				Solution:    "import pandas as pd\n\ndef top5(df: pd.DataFrame) -> pd.DataFrame:\n    q3 = df[df['thang'].between(7, 9)]\n    return q3.groupby('san_pham')['doanh_thu'].sum().nlargest(5).reset_index()\n",
				TestCases:   [][2]string{{"6 sản phẩm", "5 dòng"}, {"3 sản phẩm", "3 dòng"}, {"không có quý 3", "rỗng"}},
				Results: []submissionSpec{
					{StudentEmail: "student2@demo.com", Verdict: model.VerdictPending, Passed: 0, DaysBeforeDue: 3},
				},
			},
			{
				Title:       "Dự án nhỏ: Báo cáo doanh thu theo tháng",
				Description: "Làm notebook phân tích doanh thu 12 tháng: làm sạch dữ liệu, bảng tổng hợp theo tháng/khu vực và 3 biểu đồ có nhận xét.",
				Type:        "project", Difficulty: model.DifficultyHard, LiveIndex: 1, DueOffsetDays: 15, Published: false,
				StarterCode: "# Notebook báo cáo doanh thu\n",
				Solution:    "# Notebook báo cáo doanh thu\n",
				TestCases:   [][2]string{{"notebook chạy hết", "không lỗi"}, {"có bảng tổng hợp", "đúng 12 tháng"}, {"có biểu đồ", "3 biểu đồ"}},
			},
		},
		ExtraGrades: []extraGradeSpec{
			{StudentEmail: "student1@demo.com", Type: model.GradeAttendance, Title: "Chuyên cần 3 tuần đầu", Score: 9, Weight: 0.1, DaysAgo: 2, Feedback: "Đi học đầy đủ, tích cực phát biểu."},
			{StudentEmail: "student2@demo.com", Type: model.GradeAttendance, Title: "Chuyên cần 3 tuần đầu", Score: 7, Weight: 0.1, DaysAgo: 2, Feedback: "Vắng 1 buổi không phép, đi muộn 2 buổi."},
		},
	}
}
