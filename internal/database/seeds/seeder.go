package seeds

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type PermissionSeed struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RoleSeed struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Permissions interface{} `json:"permissions"`
}

type Seeder struct {
	db *gorm.DB
}

func NewSeeder(db *gorm.DB) *Seeder {
	return &Seeder{db: db}
}

func (s *Seeder) SeedPermissions(filePath string) error {
	log.Println("Seeding permissions...")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read permissions file: %w", err)
	}

	var permissions []PermissionSeed
	if err := json.Unmarshal(data, &permissions); err != nil {
		return fmt.Errorf("failed to parse permissions JSON: %w", err)
	}

	for _, p := range permissions {
		permission := model.Permission{
			Name: p.Name,
		}
		permission.Description.String = p.Description
		permission.Description.Valid = p.Description != ""

		result := s.db.Where("name = ?", p.Name).FirstOrCreate(&permission)
		if result.Error != nil {
			return fmt.Errorf("failed to seed permission %s: %w", p.Name, result.Error)
		}

		if result.RowsAffected == 0 {
			s.db.Model(&permission).Where("name = ?", p.Name).Update("description", p.Description)
		}
	}

	log.Printf("Successfully seeded %d permissions", len(permissions))
	return nil
}

func (s *Seeder) SeedRoles(filePath string) error {
	log.Println("Seeding roles...")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read roles file: %w", err)
	}

	var roles []RoleSeed
	if err := json.Unmarshal(data, &roles); err != nil {
		return fmt.Errorf("failed to parse roles JSON: %w", err)
	}

	var allPermissions []model.Permission
	if err := s.db.Find(&allPermissions).Error; err != nil {
		return fmt.Errorf("failed to load permissions: %w", err)
	}

	permissionMap := make(map[string]model.Permission)
	for _, p := range allPermissions {
		permissionMap[p.Name] = p
	}

	for _, r := range roles {
		role := model.Role{
			Name: r.Name,
		}
		role.Description.String = r.Description
		role.Description.Valid = r.Description != ""

		result := s.db.Where("name = ?", r.Name).FirstOrCreate(&role)
		if result.Error != nil {
			return fmt.Errorf("failed to seed role %s: %w", r.Name, result.Error)
		}

		if result.RowsAffected == 0 {
			s.db.Model(&role).Where("name = ?", r.Name).Update("description", r.Description)
			s.db.Where("name = ?", r.Name).First(&role)
		}

		var rolePermissions []model.Permission

		switch perms := r.Permissions.(type) {
		case string:
			if perms == "*" {
				rolePermissions = allPermissions
			}
		case []interface{}:
			for _, permName := range perms {
				if name, ok := permName.(string); ok {
					if perm, exists := permissionMap[name]; exists {
						rolePermissions = append(rolePermissions, perm)
					} else {
						log.Printf("Warning: Permission %s not found for role %s", name, r.Name)
					}
				}
			}
		}

		if err := s.db.Model(&role).Association("Permissions").Replace(rolePermissions); err != nil {
			return fmt.Errorf("failed to assign permissions to role %s: %w", r.Name, err)
		}
	}

	log.Printf("Successfully seeded %d roles", len(roles))
	return nil
}

func (s *Seeder) SeedAll(dataDir string) error {
	if err := s.SeedPermissions(dataDir + "/permissions.json"); err != nil {
		return err
	}

	if err := s.SeedRoles(dataDir + "/roles.json"); err != nil {
		return err
	}

	log.Println("All seeds completed successfully!")
	return nil
}
