package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"study.com/v1/internal/config"
	"study.com/v1/internal/model"
)

func Connect(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Ho_Chi_Minh",
		cfg.DBHost,
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBName,
		cfg.DBPort,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql db: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("Connected to Postgres successfully")
	return db, nil
}

func Migrate(db *gorm.DB) error {
	// Drop deprecated column from old schema
	if db.Migrator().HasColumn(&model.Lesson{}, "content_type") {
		if err := db.Migrator().DropColumn(&model.Lesson{}, "content_type"); err != nil {
			log.Printf("[WARN] Failed to drop lessons.content_type column: %v", err)
		}
	}

	return db.AutoMigrate(
		// ===== 1. Base Tables (độc lập) =====
		&model.Organization{},

		// ===== 2. Users & Auth =====
		&model.User{},
		&model.VerificationCode{},
		&model.UserOAuthProvider{},
		&model.ParentStudentRelation{},
		&model.ParentInvitation{},

		// ===== 3. Roles & Permissions (phụ thuộc Organization) =====
		&model.SystemRole{},
		&model.Permission{},
		&model.Role{},
		&model.SystemRolePermission{},
		&model.RolePermission{},

		// ===== 4. User Roles (phụ thuộc User, Organization, Role, SystemRole) =====
		&model.UserSystemRole{},
		&model.UserOrganizationRole{},

		// ===== 5. Course Management (phụ thuộc User, Organization) =====
		&model.Category{},
		&model.Tag{},
		&model.Course{},
		&model.Section{},
		&model.Lesson{},
		&model.LessonContent{},
		&model.LessonVideo{},
		&model.LessonArticle{},
		&model.LessonAttachment{},

		// ===== 6. Enrollment & Progress (phụ thuộc User, Course) =====
		&model.Enrollment{},
		&model.LessonProgress{},

		// ===== 7. Certificates (phụ thuộc User, Course, Enrollment) =====
		&model.Certificate{},
		&model.UserNote{},

		// ===== 8. Reviews & Discussions (phụ thuộc User, Course) =====
		&model.Review{},
		&model.ReviewReaction{},
		&model.Discussion{},
		&model.DiscussionVote{},

		// ===== 9. Cart & Wishlist (phụ thuộc User, Course) =====
		&model.Wishlist{},
		&model.CartItem{},

		// ===== 10. Quiz & Assessment (phụ thuộc Course) =====
		&model.Quiz{},
		&model.Question{},
		&model.QuestionAnswer{},
		&model.QuizAttempt{},
		&model.QuizAttemptAnswer{},

		// ===== 11. Payment & Orders (phụ thuộc User) =====
		&model.Coupon{},
		&model.IdempotencyKey{},
		&model.OrderLock{},
		&model.Order{},
		&model.OrderItem{},
		&model.CouponUsage{},
		&model.InstructorPayout{},
		&model.OrderStatusHistory{},
		&model.PaymentEvent{},

		// ===== 12. Vouchers =====
		&model.Voucher{},
		&model.VoucherApplicability{},
		&model.UserVoucher{},
		&model.VoucherLog{},

		// ===== 13. Livestream (phụ thuộc User) =====
		&model.LivestreamSession{},
		&model.Participant{},
		&model.ChatMessage{},
		&model.LivestreamAnalytics{},

		// ===== 14. Assignment & Submission (phụ thuộc Livestream, User) =====
		&model.Assignment{},
		&model.TestCase{},
		&model.Submission{},

		// ===== 15. Video Upload =====
		&model.VideoUpload{},
		&model.VideoUploadPart{},

		// ===== 16. Whiteboard =====
		&model.WhiteboardSnapshot{},

		// ===== 17. Teacher & Class (phụ thuộc User, Organization) =====
		&model.TeacherProfile{},
		&model.Class{},
		&model.TeacherClass{},
		&model.StudentClass{},
				&model.Attendance{},

		// ===== 18. Notifications (phụ thuộc User) =====
		&model.Notification{},
		&model.NotificationSettings{},

		// ===== 19. Reports (phụ thuộc User) =====
		&model.Report{},

		// ===== 20. Gamification (phụ thuộc User) =====
		&model.PointRule{},
		&model.UserPoint{},
		&model.PointTransaction{},
		&model.DailyCheckin{},
		&model.UserStreak{},
		&model.Achievement{},
		&model.UserAchievement{},
		&model.UserAchievementProgress{},
		&model.LeaderboardEntry{},
		&model.Reward{},
		&model.RewardRedemption{},
		&model.LearningGoal{},
		&model.UserPreference{},

		// ===== 21. Class-Lesson Content & Schedule =====
		&model.ClassLessonContent{},
		&model.ClassSchedule{},
		&model.ClassSession{},
		&model.SessionAttendance{},
		&model.ReminderSetting{},
		&model.ExtensionRequest{},

		// ===== 22. Grades =====
		&model.GradeColumn{},
		&model.Grade{},
		&model.FinalGrade{},

		// ===== 23. Coins (phụ thuộc User) =====
		&model.UserCoinWallet{},
		&model.CoinTransaction{},
		&model.CoinPackage{},
		&model.CoinPurchase{},

		// ===== 24. Groups (phụ thuộc User, Organization) =====
		&model.Group{},
		&model.GroupMember{},
		&model.GroupJoinRequest{},

		// ===== 25. Conversations & Messages (phụ thuộc User, Group) =====
		&model.Conversation{},
		&model.ConversationParticipant{},
		&model.Message{},
		&model.MessageReaction{},
		&model.MessageAttachment{},
		&model.MessageMention{},

		// ===== 26. Contests & Competitions (phụ thuộc User, Organization) =====
		&model.Contest{},
		&model.ContestProblem{},
		&model.ContestParticipant{},
		&model.ContestSubmission{},

		// ===== 27. Personal Calendar Events (phụ thuộc User) =====
		&model.PersonalEvent{},
	)
}

func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql db: %w", err)
	}
	return sqlDB.Close()
}
