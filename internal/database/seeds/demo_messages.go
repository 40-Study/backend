package seeds

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type demoChatLine struct {
	From       string // email người gửi
	Text       string
	MinutesAgo int
}

// demoConversationSpec: GroupSlug != "" là chat nhóm (hội thoại do SeedDemoGroups tạo), ngược lại là
// chat 1-1 giữa Pair. Unread = số tin CUỐI mà người đó chưa đọc (các tin đó không phải do họ gửi).
type demoConversationSpec struct {
	GroupSlug string
	Pair      [2]string
	Lines     []demoChatLine
	Unread    map[string]int
}

var demoConversationSpecs = []demoConversationSpec{
	{
		Pair: [2]string{"student1@demo.com", "teacher1@demo.com"},
		Lines: []demoChatLine{
			{From: "student1@demo.com", Text: "Thầy ơi, bài State & Hooks em chưa hiểu vì sao useEffect chạy 2 lần khi dev ạ?", MinutesAgo: 2900},
			{From: "teacher1@demo.com", Text: "Do React StrictMode cố ý gọi effect 2 lần ở môi trường dev để phát hiện effect thiếu cleanup. Build production chỉ chạy 1 lần.", MinutesAgo: 2860},
			{From: "student1@demo.com", Text: "Dạ em hiểu rồi, em sẽ thêm hàm cleanup cho interval ạ.", MinutesAgo: 2850},
			{From: "teacher1@demo.com", Text: "Chuẩn rồi. Tối thứ 5 có buổi live chữa bài Todo App, em nhớ vào nhé.", MinutesAgo: 95},
			{From: "teacher1@demo.com", Text: "Nộp bài trước 20h thứ 4 để thầy kịp xem trước.", MinutesAgo: 94},
		},
		Unread: map[string]int{"student1@demo.com": 2},
	},
	{
		Pair: [2]string{"student2@demo.com", "teacher2@demo.com"},
		Lines: []demoChatLine{
			{From: "teacher2@demo.com", Text: "Chào em, cô thấy em mới học tới phần Python cơ bản. Có chỗ nào vướng thì cứ nhắn cô.", MinutesAgo: 4300},
			{From: "student2@demo.com", Text: "Dạ em cảm ơn cô. Em hay bị lỗi IndentationError khi copy code từ slide ạ.", MinutesAgo: 4200},
			{From: "teacher2@demo.com", Text: "Em bật hiển thị khoảng trắng trong VS Code và đổi tab thành 4 dấu cách là hết nhé.", MinutesAgo: 4190},
			{From: "student2@demo.com", Text: "Em làm được rồi ạ, notebook chạy ngon rồi cô.", MinutesAgo: 30},
		},
		Unread: map[string]int{"teacher2@demo.com": 1},
	},
	{
		Pair: [2]string{"student1@demo.com", "student2@demo.com"},
		Lines: []demoChatLine{
			{From: "student2@demo.com", Text: "Cậu gửi mình link repo đồ án với, mình clone về chạy thử.", MinutesAgo: 600},
			{From: "student1@demo.com", Text: "Mình vừa ghim trong nhóm đồ án rồi đó, nhánh develop nhé.", MinutesAgo: 590},
		},
		Unread: map[string]int{"student2@demo.com": 1},
	},
	{
		GroupSlug: "nhom-hoc-react-nextjs",
		Lines: []demoChatLine{
			{From: "teacher1@demo.com", Text: "Chào cả nhóm! Tuần này mình học Server Components, mọi người xem trước video chương 3 nhé.", MinutesAgo: 3000},
			{From: "student2@demo.com", Text: "Server Component có dùng được useState không thầy?", MinutesAgo: 1500},
			{From: "student1@demo.com", Text: "Không được đâu bạn, cần state thì tách ra Client Component có \"use client\".", MinutesAgo: 1480},
			{From: "teacher1@demo.com", Text: "Bạn C trả lời đúng rồi. Thầy sẽ minh hoạ kỹ hơn trong buổi live.", MinutesAgo: 1400},
			{From: "teacher1@demo.com", Text: "Đã mở slot hỏi đáp tối thứ 5 lúc 20h, ai có câu hỏi thì để lại dưới tin này.", MinutesAgo: 120},
		},
		Unread: map[string]int{"student1@demo.com": 1, "student2@demo.com": 2},
	},
	{
		GroupSlug: "data-science-cho-nguoi-moi",
		Lines: []demoChatLine{
			{From: "teacher2@demo.com", Text: "Nhóm nộp notebook bài DataFrame vào đây nhé, cô chấm trong tuần.", MinutesAgo: 5000},
			{From: "student2@demo.com", Text: "Em nộp rồi ạ, phần xử lý dữ liệu thiếu em dùng fillna theo trung vị.", MinutesAgo: 2000},
			{From: "teacher2@demo.com", Text: "Tốt lắm, nhớ giải thích vì sao chọn trung vị thay vì trung bình nhé.", MinutesAgo: 240},
		},
		Unread: map[string]int{"student2@demo.com": 1},
	},
	{
		GroupSlug: "do-an-tot-nghiep-k66",
		Lines: []demoChatLine{
			{From: "student1@demo.com", Text: "Repo đồ án: github.com/k66-40study/do-an (nhánh develop).", MinutesAgo: 620},
			{From: "student2@demo.com", Text: "Mình nhận phần giao diện đăng nhập, cậu làm API nhé.", MinutesAgo: 610},
			{From: "student1@demo.com", Text: "Ok, thứ 7 họp online 9h sáng chốt tiến độ.", MinutesAgo: 45},
		},
		Unread: map[string]int{"student2@demo.com": 1},
	},
}

// SeedDemoConversations tạo hội thoại 1-1 (student–teacher, student–student) và nạp tin nhắn cho cả hội
// thoại 1-1 lẫn hội thoại nhóm, kèm last_message/message_count/unread_count khớp số tin thật — trang
// (app)/messages đọc thẳng các cột này (không đếm lại). Tin chỉ nạp khi hội thoại chưa có tin nào.
func (s *Seeder) SeedDemoConversations(users map[string]model.User, groupConvs map[string]uuid.UUID) error {
	for _, spec := range demoConversationSpecs {
		convID, err := s.resolveDemoConversation(spec, users, groupConvs)
		if err != nil {
			return err
		}
		if err := s.seedDemoMessages(convID, spec, users); err != nil {
			return err
		}
	}
	log.Printf("Seeded %d demo conversations\n", len(demoConversationSpecs))
	return nil
}

func (s *Seeder) resolveDemoConversation(spec demoConversationSpec, users map[string]model.User, groupConvs map[string]uuid.UUID) (uuid.UUID, error) {
	if spec.GroupSlug != "" {
		id, ok := groupConvs[spec.GroupSlug]
		if !ok {
			return uuid.Nil, fmt.Errorf("conversation of group %s not found", spec.GroupSlug)
		}
		return id, nil
	}

	a, b := users[spec.Pair[0]].ID, users[spec.Pair[1]].ID
	var conv model.Conversation
	// Cùng điều kiện với ConversationRepository.GetDirectBetweenUsers để không tạo hội thoại 1-1 thứ hai.
	err := s.db.Where("type = ? AND id IN (SELECT conversation_id FROM conversation_participants WHERE user_id = ? "+
		"INTERSECT SELECT conversation_id FROM conversation_participants WHERE user_id = ?)",
		model.ConversationTypeDirect, a, b).First(&conv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		conv = model.Conversation{Type: model.ConversationTypeDirect}
		err = s.db.Create(&conv).Error
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to seed direct conversation %s-%s: %w", spec.Pair[0], spec.Pair[1], err)
	}
	for _, id := range []uuid.UUID{a, b} {
		if err := s.ensureDemoParticipant(conv.ID, id, daysAgo(4)); err != nil {
			return uuid.Nil, err
		}
	}
	return conv.ID, nil
}

func (s *Seeder) seedDemoMessages(convID uuid.UUID, spec demoConversationSpec, users map[string]model.User) error {
	var existing int64
	if err := s.db.Model(&model.Message{}).Where("conversation_id = ?", convID).Count(&existing).Error; err != nil {
		return fmt.Errorf("failed to count messages of conversation %s: %w", convID, err)
	}
	if existing > 0 || len(spec.Lines) == 0 {
		return nil // đã có tin (seed trước hoặc người dùng thật) — không chèn thêm, không ghi đè số chưa đọc
	}

	msgs := make([]model.Message, 0, len(spec.Lines))
	for _, line := range spec.Lines {
		sender := users[line.From].ID
		msg := model.Message{ConversationID: convID, SenderID: &sender, Type: model.MessageTypeText,
			Content: ptr(line.Text), Metadata: datatypes.JSON("{}"), Status: model.MessageStatusSent}
		msg.CreatedAt = time.Now().Add(-time.Duration(line.MinutesAgo) * time.Minute)
		if err := s.db.Create(&msg).Error; err != nil {
			return fmt.Errorf("failed to seed message in conversation %s: %w", convID, err)
		}
		msgs = append(msgs, msg)
	}

	last := msgs[len(msgs)-1]
	if err := s.db.Model(&model.Conversation{}).Where("id = ?", convID).Updates(map[string]interface{}{
		"last_message_id": last.ID, "last_message_at": last.CreatedAt, "message_count": len(msgs),
	}).Error; err != nil {
		return fmt.Errorf("failed to update last message of conversation %s: %w", convID, err)
	}

	var participants []model.ConversationParticipant
	if err := s.db.Where("conversation_id = ? AND left_at IS NULL", convID).Find(&participants).Error; err != nil {
		return fmt.Errorf("failed to load participants of conversation %s: %w", convID, err)
	}
	for _, p := range participants {
		unread := spec.Unread[emailOfDemoUser(users, p.UserID)]
		fields := map[string]interface{}{"unread_count": unread}
		if idx := len(msgs) - 1 - unread; idx >= 0 {
			fields["last_read_message_id"] = msgs[idx].ID
			fields["last_read_at"] = msgs[idx].CreatedAt
		}
		if err := s.db.Model(&model.ConversationParticipant{}).Where("id = ?", p.ID).Updates(fields).Error; err != nil {
			return fmt.Errorf("failed to set unread for participant %s: %w", p.ID, err)
		}
	}
	return nil
}

// emailOfDemoUser tra ngược email từ ID trong map user demo ("" nếu không phải user demo).
func emailOfDemoUser(users map[string]model.User, id uuid.UUID) string {
	for email, u := range users {
		if u.ID == id {
			return email
		}
	}
	return ""
}
