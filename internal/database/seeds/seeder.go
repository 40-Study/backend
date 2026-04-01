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
		role := model.SystemRole{
			Name:   r.Role,
			Status: "active",
		}
		role.Description.String = r.Description
		role.Description.Valid = r.Description != ""

		// Use FirstOrCreate to insert new role or get existing
		result := s.db.Where("name = ?", r.Role).FirstOrCreate(&role)
		if result.Error != nil {
			return fmt.Errorf("failed to seed role %s: %w", r.Role, result.Error)
		}

		// Always update description
		if err := s.db.Model(&role).Updates(map[string]interface{}{
			"description": r.Description,
			"status":      "active",
		}).Error; err != nil {
			return fmt.Errorf("failed to update role %s: %w", r.Role, err)
		}

		// Refresh role to get updated data
		s.db.Where("name = ?", r.Role).First(&role)

		var rolePermissions []model.Permission
		// "*" means all permissions
		if len(r.Permissions) == 1 && r.Permissions[0] == "*" {
			for _, perm := range permissionMap {
				rolePermissions = append(rolePermissions, perm)
			}
		} else {
			for _, permKey := range r.Permissions {
				if perm, exists := permissionMap[permKey]; exists {
					rolePermissions = append(rolePermissions, perm)
				} else {
					log.Printf("Warning: Permission %s not found for role %s\n", permKey, r.Role)
				}
			}
		}

		// Replace permissions for this role via junction table system_role_permissions
		if len(rolePermissions) > 0 {
			// Delete existing permissions for this role
			if err := s.db.Where("system_role_id = ?", role.ID).Delete(&model.SystemRolePermission{}).Error; err != nil {
				log.Printf("Warning: could not clear permissions for role %s: %v\n", r.Role, err)
			}
			// Insert new permissions
			for _, perm := range rolePermissions {
				rp := model.SystemRolePermission{
					SystemRoleID: role.ID,
					PermissionID: perm.ID,
				}
				if err := s.db.Where("system_role_id = ? AND permission_id = ?", role.ID, perm.ID).
					FirstOrCreate(&rp).Error; err != nil {
					log.Printf("Warning: could not assign permission %s to role %s: %v\n", perm.Name, r.Role, err)
				}
			}
		}

		log.Printf("Seeded role: %s with %d permissions\n", r.Role, len(rolePermissions))
	}

	log.Printf("Successfully seeded %d roles\n", len(roles))
	return nil
}

func (s *Seeder) SeedAll(dataDir string) error {
	// 1. Permissions
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

	// 2. Roles (depends on permissions)
	if err := s.SeedRoles(filepath.Join(dataDir, "roles.json")); err != nil {
		return err
	}

	// 3. Users (depends on roles)
	if err := s.SeedUsers(filepath.Join(dataDir, "users.json")); err != nil {
		return err
	}

	// 4. Organizations (independent)
	if err := s.SeedOrganizations(filepath.Join(dataDir, "organizations.json")); err != nil {
		return err
	}

	// 5. Categories (independent)
	if err := s.SeedCategories(filepath.Join(dataDir, "categories.json")); err != nil {
		return err
	}

	// 6. Tags (independent)
	if err := s.SeedTags(filepath.Join(dataDir, "tags.json")); err != nil {
		return err
	}

	// 7. Courses (depends on users, categories, tags)
	if err := s.SeedCourses(filepath.Join(dataDir, "courses.json")); err != nil {
		return err
	}

	// 8. Vouchers (independent)
	if err := s.SeedVouchers(filepath.Join(dataDir, "vouchers.json")); err != nil {
		return err
	}

	// 9. Enrollments (depends on users, courses)
	if err := s.SeedEnrollments(); err != nil {
		return err
	}

	// 10. Classes (depends on users, courses)
	if err := s.SeedClasses(); err != nil {
		return err
	}

	log.Println("All seeds completed successfully!")
	return nil
}
