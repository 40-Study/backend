package model

// AllModels trả về tất cả models để dùng với AutoMigrate
func AllModels() []interface{} {
	return []interface{}{
		// Roles & Permissions
		&Permission{},
		&Role{},
		&RolePermission{},

		// Organizations
		&Organization{},

		// Users & Auth
		&User{},
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
		&UserNote{},
		&Certificate{},

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
	}
}
