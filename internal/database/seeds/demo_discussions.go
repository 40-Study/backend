package seeds

import (
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

// demoReplySpec mô tả một bình luận trả lời trong bài thảo luận.
type demoReplySpec struct {
	AuthorEmail string
	Content     string
	// IsInstructor đánh dấu câu trả lời chính thức từ giảng viên.
	IsInstructor bool
	// Upvoters là email những người đã upvote bình luận này.
	Upvoters []string
	DaysAgo  int
}

// demoPostSpec mô tả một bài thảo luận gốc trên diễn đàn.
// Bài diễn đàn có Title/Slug/Category và KHÔNG gắn với lesson nào,
// vì repository lọc danh sách theo lesson_id IS NULL AND parent_id IS NULL.
type demoPostSpec struct {
	Slug        string
	Title       string
	Category    string
	AuthorEmail string
	Content     string
	IsPinned    bool
	Upvoters    []string
	DaysAgo     int
	Replies     []demoReplySpec
}

// demoDiscussions — dữ liệu diễn đàn demo, trải đều 4 category mà UI đang dùng:
// programming, design, learning-tips, project.
var demoDiscussions = []demoPostSpec{
	{
		Slug:        "lo-trinh-hoc-react-cho-nguoi-moi",
		Title:       "Lộ trình học React cho người mới bắt đầu nên đi theo thứ tự nào?",
		Category:    "programming",
		AuthorEmail: "student1@demo.com",
		Content: "Em mới học xong HTML/CSS và JavaScript cơ bản, giờ muốn vào React nhưng thấy quá nhiều thứ: JSX, hooks, state management, routing, rồi cả Next.js nữa.\n\n" +
			"Em nên học theo thứ tự nào để không bị loạn ạ? Có nhất thiết phải thành thạo JavaScript nâng cao (closure, prototype, async) trước khi đụng vào React không?",
		IsPinned: true,
		DaysAgo:  12,
		Upvoters: []string{"student2@demo.com", "teacher1@demo.com", "parent1@demo.com", "admin@demo.com"},
		Replies: []demoReplySpec{
			{
				AuthorEmail:  "teacher1@demo.com",
				IsInstructor: true,
				DaysAgo:      11,
				Upvoters:     []string{"student1@demo.com", "student2@demo.com", "admin@demo.com"},
				Content: "Thứ tự mình vẫn hướng dẫn học viên:\n\n" +
					"1. JavaScript ES6+ cho vững: arrow function, destructuring, spread, map/filter/reduce, Promise và async/await. Chưa cần đào sâu prototype.\n" +
					"2. JSX + component + props, dựng được UI tĩnh.\n" +
					"3. useState và useEffect. Dừng lại ở đây làm 2-3 bài tập nhỏ.\n" +
					"4. React Router.\n" +
					"5. State management (Context, sau đó Zustand/Redux nếu dự án cần).\n" +
					"6. Next.js, chỉ vào khi đã tự tin với React thuần.\n\n" +
					"Closure và prototype cứ để sau, gặp lỗi thực tế rồi học sẽ nhớ lâu hơn nhiều.",
			},
			{
				AuthorEmail: "student2@demo.com",
				DaysAgo:     10,
				Upvoters:    []string{"student1@demo.com"},
				Content:     "Mình cũng đang ở giai đoạn này. Kinh nghiệm: đừng học hết lý thuyết rồi mới làm. Mình học tới useState là bắt tay làm app todo luôn, vừa làm vừa tra, tiến bộ nhanh hơn hẳn so với hồi xem hết playlist mà không code.",
			},
			{
				AuthorEmail: "student1@demo.com",
				DaysAgo:     9,
				Content:     "Cảm ơn thầy và bạn nhiều ạ. Em sẽ làm theo thứ tự này, đang bắt đầu lại từ phần ES6+.",
			},
		},
	},
	{
		Slug:        "meo-ghi-nho-kien-thuc-lau-hon",
		Title:       "Mẹo nào giúp nhớ kiến thức lâu hơn khi học online?",
		Category:    "learning-tips",
		AuthorEmail: "student2@demo.com",
		Content: "Em học online hay bị tình trạng: lúc xem video thì hiểu hết, gật gù, nhưng vài hôm sau ngồi code lại thì quên sạch, phải mở lại bài xem từ đầu.\n\n" +
			"Mọi người có cách nào để kiến thức đọng lại lâu hơn không ạ?",
		DaysAgo:  8,
		Upvoters: []string{"student1@demo.com", "teacher2@demo.com", "parent1@demo.com"},
		Replies: []demoReplySpec{
			{
				AuthorEmail:  "teacher2@demo.com",
				IsInstructor: true,
				DaysAgo:      7,
				Upvoters:     []string{"student1@demo.com", "student2@demo.com", "parent1@demo.com", "admin@demo.com"},
				Content: "Cảm giác hiểu hết khi xem video là bẫy kinh điển, đó là nhận ra bài giảng chứ không phải nhớ kiến thức. Hai kỹ thuật có bằng chứng rõ ràng:\n\n" +
					"• Active recall: xem xong đóng video lại, lấy giấy viết ra những gì vừa học mà không nhìn bài. Chỗ nào viết không ra chính là chỗ chưa thuộc.\n" +
					"• Spaced repetition: ôn lại sau 1 ngày, 3 ngày, 7 ngày, 21 ngày thay vì ôn dồn một lần.\n\n" +
					"Với lập trình thì thêm một bước nữa: gõ lại code từ đầu, không copy. Tay nhớ khác mắt nhớ.",
			},
			{
				AuthorEmail: "student1@demo.com",
				DaysAgo:     6,
				Upvoters:    []string{"student2@demo.com"},
				Content:     "Mình áp dụng cách viết lại bài bằng lời của mình sau mỗi buổi học, coi như giải thích cho người khác nghe. Chỗ nào giải thích lắp bắp là chỗ mình chưa thực sự hiểu.",
			},
		},
	},
	{
		Slug:        "chon-mau-va-font-cho-giao-dien-hoc-tap",
		Title:       "Chọn màu và font thế nào cho giao diện web học tập đỡ mỏi mắt?",
		Category:    "design",
		AuthorEmail: "student1@demo.com",
		Content: "Em đang làm đồ án một trang web học tập, người dùng sẽ đọc và xem video khá lâu trên đó.\n\n" +
			"Em nên chọn bảng màu và font như thế nào để nhìn lâu không bị mỏi mắt? Nền tối hay nền sáng thì tốt hơn ạ?",
		DaysAgo:  6,
		Upvoters: []string{"student2@demo.com", "teacher1@demo.com"},
		Replies: []demoReplySpec{
			{
				AuthorEmail:  "teacher1@demo.com",
				IsInstructor: true,
				DaysAgo:      5,
				Upvoters:     []string{"student1@demo.com", "student2@demo.com"},
				Content: "Vài nguyên tắc dùng được ngay:\n\n" +
					"• Đừng dùng đen tuyệt đối (#000) trên trắng tuyệt đối (#fff), độ tương phản gắt gây mỏi mắt. Chữ #1e293b trên nền #f8fafc dễ chịu hơn nhiều.\n" +
					"• Tương phản chữ/nền tối thiểu 4.5:1 theo chuẩn WCAG AA. Kiểm tra bằng công cụ contrast checker chứ đừng ước lượng bằng mắt.\n" +
					"• Cỡ chữ nội dung 16-18px, line-height 1.6-1.75, độ rộng dòng khoảng 60-75 ký tự.\n" +
					"• Font sans-serif cho UI (Inter, Be Vietnam Pro vì hỗ trợ tiếng Việt tốt). Quan trọng là phải đủ dấu tiếng Việt, nhiều font đẹp nhưng dấu bị vỡ.\n\n" +
					"Sáng hay tối thì không có đáp án tuyệt đối, tốt nhất là cho người dùng tự chọn và lưu lại lựa chọn đó.",
			},
			{
				AuthorEmail: "student2@demo.com",
				DaysAgo:     4,
				Content:     "Bổ sung: hạn chế số lượng font, 2 font là đủ (1 cho tiêu đề, 1 cho nội dung). Đồ án của mình lúc đầu dùng 4 font, nhìn rối và thiếu chuyên nghiệp hẳn.",
			},
		},
	},
	{
		Slug:        "tim-nhom-lam-do-an-cuoi-ky",
		Title:       "Tìm nhóm làm đồ án cuối kỳ, web quản lý học tập",
		Category:    "project",
		AuthorEmail: "student2@demo.com",
		Content: "Em đang tìm 2-3 bạn cùng làm đồ án cuối kỳ, đề tài web quản lý học tập.\n\n" +
			"Dự kiến stack: Next.js cho frontend, Go cho backend, PostgreSQL. Em nhận phần frontend, đang cần bạn làm backend và bạn lo phần database cùng triển khai.\n\n" +
			"Bạn nào quan tâm thì để lại comment nhé, mình trao đổi thêm.",
		DaysAgo:  4,
		Upvoters: []string{"student1@demo.com"},
		Replies: []demoReplySpec{
			{
				AuthorEmail: "student1@demo.com",
				DaysAgo:     3,
				Upvoters:    []string{"student2@demo.com"},
				Content:     "Mình quan tâm phần backend Go nhé. Mình đã làm qua REST API với Fiber và GORM, có thể nhận thêm phần thiết kế database luôn.",
			},
			{
				AuthorEmail:  "teacher2@demo.com",
				IsInstructor: true,
				DaysAgo:      2,
				Upvoters:     []string{"student1@demo.com", "student2@demo.com"},
				Content: "Nhóm nhớ thống nhất giao diện API trước khi chia việc: định rõ endpoint, tên field và kiểu dữ liệu, viết vào một file chung.\n\n" +
					"Lỗi phổ biến nhất của các nhóm làm song song là mỗi bên tự đặt tên field theo ý mình (camelCase và snake_case lẫn lộn), tới lúc ghép mới phát hiện, sửa rất mất thời gian.",
			},
		},
	},
	{
		Slug:        "hoc-git-den-muc-nao-la-du",
		Title:       "Học Git tới mức nào là đủ để đi làm?",
		Category:    "programming",
		AuthorEmail: "student1@demo.com",
		Content: "Hiện em chỉ biết mấy lệnh cơ bản: clone, add, commit, push, pull. Nghe mọi người nói tới rebase, cherry-pick, stash mà em chưa dùng bao giờ.\n\n" +
			"Đi thực tập thì cần biết tới đâu ạ?",
		DaysAgo:  3,
		Upvoters: []string{"student2@demo.com", "admin@demo.com"},
		Replies: []demoReplySpec{
			{
				AuthorEmail:  "teacher1@demo.com",
				IsInstructor: true,
				DaysAgo:      2,
				Upvoters:     []string{"student1@demo.com", "student2@demo.com", "admin@demo.com"},
				Content: "Mức đủ để đi thực tập:\n\n" +
					"• branch, merge và xử lý được conflict. Cái này bắt buộc, gần như ngày nào cũng gặp.\n" +
					"• Viết commit message rõ ràng, mỗi commit một việc.\n" +
					"• Tạo pull request và sửa theo review.\n" +
					"• git stash khi cần chuyển việc gấp.\n" +
					"• Đọc được git log và git diff.\n\n" +
					"rebase và cherry-pick để sau cũng được, mỗi team một quy ước riêng, vào rồi học theo team. Quan trọng hơn cả là hiểu mình đang ở branch nào và code của mình đang ở đâu, phần lớn sự cố Git của người mới đều bắt nguồn từ chỗ này.",
			},
		},
	},
	{
		Slug:        "cach-doc-tai-lieu-tieng-anh-hieu-qua",
		Title:       "Làm sao đọc tài liệu kỹ thuật tiếng Anh nhanh hơn?",
		Category:    "learning-tips",
		AuthorEmail: "parent1@demo.com",
		Content: "Con tôi học lập trình, hay than là tài liệu toàn tiếng Anh nên đọc rất chậm, phải tra từ điển liên tục và dễ nản.\n\n" +
			"Có cách nào luyện để đọc tài liệu kỹ thuật nhanh hơn không ạ?",
		DaysAgo:  2,
		Upvoters: []string{"student1@demo.com", "teacher2@demo.com"},
		Replies: []demoReplySpec{
			{
				AuthorEmail:  "teacher2@demo.com",
				IsInstructor: true,
				DaysAgo:      1,
				Upvoters:     []string{"parent1@demo.com", "student1@demo.com"},
				Content: "Tin vui là tài liệu kỹ thuật dùng vốn từ khá hẹp và lặp đi lặp lại, nên tiến bộ nhanh hơn tiếng Anh giao tiếp nhiều.\n\n" +
					"• Đừng tra từng từ. Đọc hết đoạn, nắm ý chính trước, chỉ tra những từ chặn hẳn việc hiểu.\n" +
					"• Đọc phần code example trước rồi mới đọc phần chữ, vì code là ngôn ngữ chung, thường đoán được ý cả đoạn.\n" +
					"• Ghi lại các từ lặp lại nhiều (deprecated, retrieve, whether, regardless). Khoảng 200 từ là đọc trôi phần lớn tài liệu.\n" +
					"• Đọc mỗi ngày 15 phút, đều đặn hơn là dồn một buổi dài.",
			},
			{
				AuthorEmail: "student1@demo.com",
				DaysAgo:     1,
				Content:     "Em thấy đọc tài liệu chính thức (official docs) lại dễ hơn đọc blog, vì họ viết câu ngắn và cấu trúc rõ ràng. Hồi đầu em toàn tìm bài dịch, hoá ra vừa chậm vừa hay sai.",
			},
		},
	},
}

// SeedDemoDiscussions tạo bài thảo luận diễn đàn, bình luận và lượt vote.
// Idempotent: bài gốc nhận diện theo slug, bình luận theo (parent_id, user_id, content),
// vote theo unique index (user_id, discussion_id).
func (s *Seeder) SeedDemoDiscussions(users map[string]model.User) error {
	log.Println("Seeding demo discussions...")

	var posts, replies, votes int

	for _, spec := range demoDiscussions {
		author, ok := users[spec.AuthorEmail]
		if !ok {
			return fmt.Errorf("author %s not found for discussion %s", spec.AuthorEmail, spec.Slug)
		}

		post, created, err := s.upsertForumPost(spec, author.ID)
		if err != nil {
			return err
		}
		if created {
			posts++
		}

		n, err := s.seedDiscussionVotes(post.ID, spec.Upvoters, users)
		if err != nil {
			return fmt.Errorf("votes for post %s: %w", spec.Slug, err)
		}
		votes += n

		for _, rSpec := range spec.Replies {
			replyAuthor, ok := users[rSpec.AuthorEmail]
			if !ok {
				return fmt.Errorf("author %s not found for reply on %s", rSpec.AuthorEmail, spec.Slug)
			}

			reply, created, err := s.upsertReply(post.ID, rSpec, replyAuthor.ID)
			if err != nil {
				return fmt.Errorf("reply on post %s: %w", spec.Slug, err)
			}
			if created {
				replies++
			}

			n, err := s.seedDiscussionVotes(reply.ID, rSpec.Upvoters, users)
			if err != nil {
				return fmt.Errorf("votes for reply on %s: %w", spec.Slug, err)
			}
			votes += n
		}
	}

	log.Printf("Seeded %d discussion posts, %d replies, %d votes\n", posts, replies, votes)
	return nil
}

// upsertForumPost tạo bài thảo luận gốc nếu slug chưa tồn tại.
func (s *Seeder) upsertForumPost(spec demoPostSpec, authorID uuid.UUID) (model.Discussion, bool, error) {
	var post model.Discussion

	err := s.db.Where("slug = ?", spec.Slug).First(&post).Error
	if err == nil {
		return post, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Discussion{}, false, fmt.Errorf("failed to look up discussion %s: %w", spec.Slug, err)
	}

	post = model.Discussion{
		UserID:      authorID,
		Content:     spec.Content,
		Title:       ptr(spec.Title),
		Category:    ptr(spec.Category),
		Slug:        ptr(spec.Slug),
		IsPinned:    spec.IsPinned,
		UpvoteCount: len(spec.Upvoters),
		ReplyCount:  len(spec.Replies),
	}
	post.CreatedAt = daysAgo(spec.DaysAgo)
	post.UpdatedAt = post.CreatedAt

	if err := s.db.Create(&post).Error; err != nil {
		return model.Discussion{}, false, fmt.Errorf("failed to seed discussion %s: %w", spec.Slug, err)
	}
	return post, true, nil
}

// upsertReply tạo bình luận cho bài thảo luận nếu chưa tồn tại.
func (s *Seeder) upsertReply(postID uuid.UUID, spec demoReplySpec, authorID uuid.UUID) (model.Discussion, bool, error) {
	var reply model.Discussion

	err := s.db.Where("parent_id = ? AND user_id = ? AND content = ?", postID, authorID, spec.Content).
		First(&reply).Error
	if err == nil {
		return reply, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Discussion{}, false, fmt.Errorf("failed to look up reply: %w", err)
	}

	reply = model.Discussion{
		UserID:             authorID,
		ParentID:           &postID,
		Content:            spec.Content,
		UpvoteCount:        len(spec.Upvoters),
		IsInstructorAnswer: spec.IsInstructor,
	}
	reply.CreatedAt = daysAgo(spec.DaysAgo)
	reply.UpdatedAt = reply.CreatedAt

	if err := s.db.Create(&reply).Error; err != nil {
		return model.Discussion{}, false, fmt.Errorf("failed to seed reply: %w", err)
	}
	return reply, true, nil
}

// seedDiscussionVotes tạo lượt upvote cho một bài viết hoặc bình luận.
// Trả về số vote thực sự được tạo mới.
func (s *Seeder) seedDiscussionVotes(discussionID uuid.UUID, voterEmails []string, users map[string]model.User) (int, error) {
	created := 0

	for _, email := range voterEmails {
		voter, ok := users[email]
		if !ok {
			return created, fmt.Errorf("voter %s not found", email)
		}

		vote := model.DiscussionVote{
			UserID:       voter.ID,
			DiscussionID: discussionID,
			VoteType:     "upvote",
		}

		res := s.db.Where("user_id = ? AND discussion_id = ?", voter.ID, discussionID).
			Attrs(vote).
			FirstOrCreate(&vote)
		if res.Error != nil {
			return created, fmt.Errorf("failed to seed vote by %s: %w", email, res.Error)
		}
		created += int(res.RowsAffected)
	}

	return created, nil
}
