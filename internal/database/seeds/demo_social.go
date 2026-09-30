package seeds

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"study.com/v1/internal/model"
)

// socialDemoEmails là các tài khoản demo mà lane xã hội/tiền giáo viên cần có sẵn.
var socialDemoEmails = []string{
	"admin@demo.com", "teacher1@demo.com", "teacher2@demo.com",
	"student1@demo.com", "student2@demo.com",
}

// SeedDemoSocialAndPayouts seed toàn bộ dữ liệu "xã hội + tiền giáo viên + lớp học" cho demo:
// nhóm học & chat (trang (app)/groups, (app)/messages), đơn hàng hoàn tất + yêu cầu rút tiền
// ((teacher)/teacher/wallet, (admin)/admin/withdrawals), bài tập code gắn vào bài học thực hành,
// lớp học + livestream, lịch cá nhân ((app)/schedule) và báo cáo kiểm duyệt ((admin)/admin/moderation).
//
// Phụ thuộc: SeedDemoUsers + SeedDemoCourses (+ nên chạy sau SeedDemoEnrollments để đơn hàng khớp
// ghi danh). Không phụ thuộc dữ liệu discussions/reviews của seed khác. Mọi bước idempotent.
func (s *Seeder) SeedDemoSocialAndPayouts(users map[string]model.User, courses map[string]model.Course) error {
	log.Println("Seeding demo social, payouts, exercises, calendar and moderation...")

	for _, email := range socialDemoEmails {
		if _, ok := users[email]; !ok {
			return fmt.Errorf("demo social: user %s not found", email)
		}
	}

	groupConvs, err := s.SeedDemoGroups(users, courses)
	if err != nil {
		return err
	}
	if err := s.SeedDemoConversations(users, groupConvs); err != nil {
		return err
	}
	if err := s.SeedDemoTeacherPayouts(users, courses); err != nil {
		return err
	}
	if err := s.SeedDemoCourseExercises(courses); err != nil {
		return err
	}
	if err := s.SeedDemoClassesAndLivestreams(users, courses); err != nil {
		return err
	}
	if err := s.SeedDemoPersonalEvents(users); err != nil {
		return err
	}
	return s.SeedDemoReports(users, courses)
}

type demoGroupMemberSpec struct {
	Email         string
	Role          model.GroupMemberRole
	JoinedDaysAgo int
}

type demoGroupJoinRequestSpec struct {
	Email   string
	Message string
}

type demoGroupSpec struct {
	Slug         string
	Name         string
	Description  string
	Type         model.GroupType
	Privacy      model.GroupPrivacy
	CreatorEmail string
	CourseSlug   string // rỗng = nhóm không gắn khoá học
	Members      []demoGroupMemberSpec
	JoinRequests []demoGroupJoinRequestSpec
}

// demoGroupSpecs: đủ 3 mức riêng tư để kiểm tra hiển thị — PUBLIC hiện ở "Khám phá", PRIVATE hiện
// nhưng phải xin vào (có 1 yêu cầu chờ duyệt), SECRET chỉ thành viên thấy trong "Nhóm của tôi".
var demoGroupSpecs = []demoGroupSpec{
	{
		Slug: "nhom-hoc-react-nextjs", Name: "Nhóm học React & Next.js",
		Description: "Nơi học viên khoá React + Next.js trao đổi bài tập, chia sẻ tài liệu và hỏi đáp cùng giảng viên.",
		Type:        model.GroupTypeStudy, Privacy: model.GroupPrivacyPublic,
		CreatorEmail: "teacher1@demo.com", CourseSlug: "react-nextjs-tu-co-ban-den-nang-cao",
		Members: []demoGroupMemberSpec{
			{Email: "teacher1@demo.com", Role: model.GroupRoleOwner, JoinedDaysAgo: 25},
			{Email: "student1@demo.com", Role: model.GroupRoleModerator, JoinedDaysAgo: 19},
			{Email: "student2@demo.com", Role: model.GroupRoleMember, JoinedDaysAgo: 6},
		},
	},
	{
		Slug: "data-science-cho-nguoi-moi", Name: "Data Science cho người mới",
		Description: "Nhóm kín của khoá Python cho Khoa học Dữ liệu: nộp notebook, nhận góp ý và lịch học bù.",
		Type:        model.GroupTypeCourse, Privacy: model.GroupPrivacyPrivate,
		CreatorEmail: "teacher2@demo.com", CourseSlug: "python-cho-khoa-hoc-du-lieu",
		Members: []demoGroupMemberSpec{
			{Email: "teacher2@demo.com", Role: model.GroupRoleOwner, JoinedDaysAgo: 28},
			{Email: "student2@demo.com", Role: model.GroupRoleMember, JoinedDaysAgo: 18},
		},
		JoinRequests: []demoGroupJoinRequestSpec{
			{Email: "student1@demo.com", Message: "Em đang tự học Pandas, cho em tham gia nhóm để hỏi bài với ạ."},
		},
	},
	{
		Slug: "do-an-tot-nghiep-k66", Name: "Đồ án tốt nghiệp K66",
		Description: "Nhóm bí mật của 2 thành viên làm đồ án: chia việc, lưu link repo và lịch họp.",
		Type:        model.GroupTypeCustom, Privacy: model.GroupPrivacySecret,
		CreatorEmail: "student1@demo.com",
		Members: []demoGroupMemberSpec{
			{Email: "student1@demo.com", Role: model.GroupRoleOwner, JoinedDaysAgo: 14},
			{Email: "student2@demo.com", Role: model.GroupRoleMember, JoinedDaysAgo: 14},
		},
	},
}

// SeedDemoGroups tạo nhóm + thành viên + yêu cầu tham gia + hội thoại nhóm (giống GroupService.Create:
// mỗi nhóm có đúng 1 conversation GROUP, thành viên ACTIVE là participant). Trả về slug -> conversation ID
// để SeedDemoConversations nạp tin nhắn nhóm.
func (s *Seeder) SeedDemoGroups(users map[string]model.User, courses map[string]model.Course) (map[string]uuid.UUID, error) {
	convs := make(map[string]uuid.UUID, len(demoGroupSpecs))
	for _, spec := range demoGroupSpecs {
		group, err := s.upsertDemoGroup(spec, users, courses)
		if err != nil {
			return nil, err
		}
		conv := model.Conversation{Type: model.ConversationTypeGroup, GroupID: &group.ID, Name: ptr(group.Name)}
		if err := s.db.Where("group_id = ?", group.ID).Attrs(conv).FirstOrCreate(&conv).Error; err != nil {
			return nil, fmt.Errorf("failed to seed conversation of group %s: %w", spec.Slug, err)
		}
		for _, m := range spec.Members {
			if err := s.ensureDemoParticipant(conv.ID, users[m.Email].ID, daysAgo(m.JoinedDaysAgo)); err != nil {
				return nil, err
			}
		}
		convs[spec.Slug] = conv.ID
	}
	log.Printf("Seeded %d demo groups\n", len(demoGroupSpecs))
	return convs, nil
}

func (s *Seeder) upsertDemoGroup(spec demoGroupSpec, users map[string]model.User, courses map[string]model.Course) (model.Group, error) {
	creator, ok := users[spec.CreatorEmail]
	if !ok {
		return model.Group{}, fmt.Errorf("group %s: creator %s not found", spec.Slug, spec.CreatorEmail)
	}
	group := model.Group{
		Name: spec.Name, Slug: spec.Slug, Description: ptr(spec.Description),
		AvatarURL: ptr(fmt.Sprintf("https://picsum.photos/seed/%s/200/200", spec.Slug)),
		Type:      spec.Type, Privacy: spec.Privacy, MaxMembers: 100,
		Settings: datatypes.JSON("{}"), CreatedBy: creator.ID,
	}
	if spec.CourseSlug != "" {
		course, ok := courses[spec.CourseSlug]
		if !ok {
			return model.Group{}, fmt.Errorf("group %s: course %s not found", spec.Slug, spec.CourseSlug)
		}
		group.ReferenceType, group.ReferenceID = ptr("course"), &course.ID
	}
	if err := s.db.Where("slug = ?", spec.Slug).Attrs(group).FirstOrCreate(&group).Error; err != nil {
		return model.Group{}, fmt.Errorf("failed to seed group %s: %w", spec.Slug, err)
	}

	for _, m := range spec.Members {
		joined := daysAgo(m.JoinedDaysAgo)
		member := model.GroupMember{GroupID: group.ID, UserID: users[m.Email].ID, Role: m.Role,
			Status: model.GroupMemberActive, JoinedAt: &joined, NotificationEnabled: true}
		if err := s.db.Where("group_id = ? AND user_id = ?", group.ID, member.UserID).
			Attrs(member).FirstOrCreate(&member).Error; err != nil {
			return model.Group{}, fmt.Errorf("failed to seed member %s of group %s: %w", m.Email, spec.Slug, err)
		}
	}
	for _, r := range spec.JoinRequests {
		req := model.GroupJoinRequest{GroupID: group.ID, UserID: users[r.Email].ID,
			Message: ptr(r.Message), Status: model.JoinRequestPending}
		if err := s.db.Where("group_id = ? AND user_id = ?", group.ID, req.UserID).
			Attrs(req).FirstOrCreate(&req).Error; err != nil {
			return model.Group{}, fmt.Errorf("failed to seed join request of group %s: %w", spec.Slug, err)
		}
	}

	// member_count là cột mà GroupService tăng/giảm tay (không đếm lại khi đọc) — ghi đúng bằng số
	// thành viên ACTIVE thật để trang nhóm không hiện "0 thành viên" hay vượt MaxMembers sai.
	var active int64
	if err := s.db.Model(&model.GroupMember{}).
		Where("group_id = ? AND status = ?", group.ID, model.GroupMemberActive).Count(&active).Error; err != nil {
		return model.Group{}, fmt.Errorf("failed to count members of group %s: %w", spec.Slug, err)
	}
	if err := s.db.Model(&model.Group{}).Where("id = ?", group.ID).
		UpdateColumn("member_count", active).Error; err != nil {
		return model.Group{}, fmt.Errorf("failed to sync member_count of group %s: %w", spec.Slug, err)
	}
	group.MemberCount = int(active)
	return group, nil
}

// ensureDemoParticipant thêm user vào hội thoại nếu chưa là participant (khoá tự nhiên conversation+user).
func (s *Seeder) ensureDemoParticipant(convID, userID uuid.UUID, joinedAt time.Time) error {
	p := model.ConversationParticipant{ConversationID: convID, UserID: userID, JoinedAt: joinedAt}
	if err := s.db.Where("conversation_id = ? AND user_id = ?", convID, userID).
		Attrs(p).FirstOrCreate(&p).Error; err != nil {
		return fmt.Errorf("failed to seed participant %s of conversation %s: %w", userID, convID, err)
	}
	return nil
}
