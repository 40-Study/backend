package model

// AllModels trả về tất cả models để dùng với AutoMigrate
func AllModels() []interface{} {
	return []interface{}{
		// Roles & Permissions
		&Permission{},
		&Role{},
		&RolePermission{},
		&SystemRole{},
		&SystemRolePermission{},

		// Organizations
		&Organization{},

		// Users & Auth
		&User{},
		&UserOrganizationRole{},
		&UserSystemRole{},
		&VerificationCode{},
		&UserOAuthProvider{},

		// Course Management
		&Category{},
		&Tag{},
		&Course{},
		&Section{},
		&Lesson{},
		&LessonVideo{},
		&LessonArticle{},
		&LessonAttachment{},

		// Quiz & Assessment
		&Quiz{},
		&Question{},
		&QuestionAnswer{},
		&QuizAttempt{},
		&QuizAttemptAnswer{},

		// Enrollment & Progress
		&Enrollment{},
		&LessonProgress{},
		&Certificate{},
		&UserNote{},

		// Reviews & Discussions
		&Review{},
		&ReviewReaction{},
		&Discussion{},
		&DiscussionVote{},

		// Payment
		&Order{},
		&OrderItem{},
		&Coupon{},
		&CouponUsage{},
		&InstructorPayout{},
		&OrderStatusHistory{},
		&PaymentEvent{},
		&IdempotencyKey{},
		&OrderLock{},

		// Notifications
		&Notification{},
		&NotificationSettings{},

		// Cart & Wishlist
		&Wishlist{},
		&CartItem{},

		// Reports
		&Report{},

		// Gamification - Points
		&UserPoint{},
		&PointTransaction{},
		&PointRule{},

		// Gamification - Checkin & Streak
		&DailyCheckin{},
		&UserStreak{},

		// Gamification - Achievements
		&Achievement{},
		&UserAchievement{},
		&UserAchievementProgress{},

		// Gamification - Leaderboard
		&LeaderboardEntry{},

		// Gamification - Rewards
		&Reward{},
		&RewardRedemption{},

		// Gamification - Goals & Preferences
		&LearningGoal{},
		&UserPreference{},

		// Teacher & Class
		&TeacherProfile{},
		&Class{},
		&TeacherClass{},
		&StudentClass{},
		&ClassSchedule{},
		&Attendance{},
	}
}
