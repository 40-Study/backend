package seeds

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
	"study.com/v1/internal/model"
)

type UserSeed struct {
	Email       string   `json:"email"`
	Password    string   `json:"password"`
	UserName    string   `json:"user_name"`
	FullName    string   `json:"full_name"`
	SystemRoles []string `json:"system_roles"`
	IsVerified  bool     `json:"is_verified"`
}

func (s *Seeder) SeedUsers(filePath string) error {
	log.Println("Seeding users...")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read users file: %w", err)
	}

	var userSeeds []UserSeed
	if err := json.Unmarshal(data, &userSeeds); err != nil {
		return fmt.Errorf("failed to parse users JSON: %w", err)
	}

	// Load all system roles into map for lookup
	var allRoles []model.SystemRole
	if err := s.db.Find(&allRoles).Error; err != nil {
		return fmt.Errorf("failed to load system roles: %w", err)
	}
	roleMap := make(map[string]model.SystemRole)
	for _, r := range allRoles {
		roleMap[r.Name] = r
	}

	for _, us := range userSeeds {
		// Hash password
		hash, err := bcrypt.GenerateFromPassword([]byte(us.Password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash password for %s: %w", us.Email, err)
		}

		fullName := us.FullName
		user := model.User{
			Email:        us.Email,
			PasswordHash: string(hash),
			UserName:     us.UserName,
			FullName:     &fullName,
			IsVerified:   us.IsVerified,
			IsActive:     true,
		}

		// FirstOrCreate by email
		result := s.db.Where("email = ?", us.Email).FirstOrCreate(&user)
		if result.Error != nil {
			return fmt.Errorf("failed to seed user %s: %w", us.Email, result.Error)
		}

		// Assign system roles
		for _, roleName := range us.SystemRoles {
			role, exists := roleMap[roleName]
			if !exists {
				log.Printf("Warning: system role %s not found for user %s\n", roleName, us.Email)
				continue
			}

			userRole := model.UserSystemRole{
				UserID:       user.ID,
				SystemRoleID: role.ID,
				Status:       model.UserSystemRoleStatusActive,
			}
			if err := s.db.Where("user_id = ? AND system_role_id = ?", user.ID, role.ID).
				FirstOrCreate(&userRole).Error; err != nil {
				log.Printf("Warning: could not assign role %s to %s: %v\n", roleName, us.Email, err)
			}
		}

		log.Printf("Seeded user: %s (%s)\n", us.Email, us.UserName)
	}

	log.Printf("Successfully seeded %d users\n", len(userSeeds))
	return nil
}
