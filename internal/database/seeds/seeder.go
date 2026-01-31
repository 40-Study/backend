package seeds

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gorm.io/gorm"
	"study.com/v1/internal/model"
)

type PermissionSeed struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RoleSeed struct {
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
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

	log.Printf("Successfully seeded %d permissions\n", len(permissions))
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
			Name: r.Role,
		}
		role.Description.String = r.Description
		role.Description.Valid = r.Description != ""

		// Use FirstOrCreate to insert new role or get existing
		result := s.db.Where("name = ?", r.Role).FirstOrCreate(&role)
		if result.Error != nil {
			return fmt.Errorf("failed to seed role %s: %w", r.Role, result.Error)
		}

		// Always update description
		if err := s.db.Model(&role).Update("description", r.Description).Error; err != nil {
			return fmt.Errorf("failed to update role %s: %w", r.Role, err)
		}

		// Refresh role to get updated data
		s.db.Where("name = ?", r.Role).First(&role)

		var rolePermissions []model.Permission
		for _, permKey := range r.Permissions {
			if perm, exists := permissionMap[permKey]; exists {
				rolePermissions = append(rolePermissions, perm)
			} else {
				log.Printf("Warning: Permission %s not found for role %s\n", permKey, r.Role)
			}
		}

		// Replace permissions for this role
		if err := s.db.Model(&role).Association("Permissions").Replace(rolePermissions); err != nil {
			return fmt.Errorf("failed to assign permissions to role %s: %w", r.Role, err)
		}

		log.Printf("Seeded role: %s with %d permissions\n", r.Role, len(rolePermissions))
	}

	log.Printf("Successfully seeded %d roles\n", len(roles))
	return nil
}

func (s *Seeder) SeedAll(dataDir string) error {
	// Seed all permission files from permissions folder
	permissionsDir := filepath.Join(dataDir, "permissions")
	files, err := ioutil.ReadDir(permissionsDir)
	if err != nil {
		return fmt.Errorf("failed to read permissions directory: %w", err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			filePath := filepath.Join(permissionsDir, file.Name())
			if err := s.SeedPermissions(filePath); err != nil {
				return fmt.Errorf("failed to seed permissions from %s: %w", file.Name(), err)
			}
		}
	}

	// Seed roles
	rolesFilePath := filepath.Join(dataDir, "roles.json")
	if err := s.SeedRoles(rolesFilePath); err != nil {
		return err
	}

	log.Println("All seeds completed successfully!")
	return nil
}
