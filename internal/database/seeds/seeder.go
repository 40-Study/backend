package seeds

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// SeedRoles đồng bộ role + quyền từ roles.json trong MỘT transaction (S2): hoặc mọi role được xử
// lý, hoặc không role nào bị đụng — không còn khoảng thời gian role trống quyền giữa "xoá" và
// "chèn lại" mà request đang chạy có thể chạm phải.
//
// Nguyên tắc: seed chỉ THÊM cái còn thiếu, KHÔNG BAO GIỜ gỡ. Quyền admin đã chỉnh tay (gỡ bớt
// quyền của một role) phải sống qua mọi lần khởi động lại; muốn gỡ quyền khỏi role thì làm ở DB/
// API quản trị chứ không phải bằng cách xoá dòng khỏi roles.json.
func (s *Seeder) SeedRoles(filePath string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return (&Seeder{db: tx}).seedRoles(filePath)
	})
}

func (s *Seeder) seedRoles(filePath string) error {
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

		// Chỉ làm mới description. KHÔNG ép status = active nữa (S2): role tạo mới đã mang
		// status "active" từ struct ở trên, còn role admin đã khoá/tắt thì phải giữ nguyên.
		if err := s.db.Model(&role).Update("description", r.Description).Error; err != nil {
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

		// Chỉ cấp cặp (role, quyền) CHƯA TỪNG được seeder cấp (xem SystemRolePermissionSeed), rồi
		// đánh dấu. Cặp đã đánh dấu mà dòng system_role_permissions không còn = admin gỡ tay, giữ
		// nguyên. ON CONFLICT DO NOTHING vì khoá chính kép nên dòng đã có (cấp từ trước khi có bảng
		// dấu) không lỗi. Lỗi được trả về để cả transaction rollback — trong transaction Postgres
		// một câu lỗi đã làm hỏng cả phiên, nên nuốt lỗi bằng log.Printf như trước là sai.
		if len(rolePermissions) > 0 {
			var seeded []model.SystemRolePermissionSeed
			if err := s.db.Where("system_role_id = ?", role.ID).Find(&seeded).Error; err != nil {
				return fmt.Errorf("failed to load seeded permissions of role %s: %w", r.Role, err)
			}
			alreadySeeded := make(map[uuid.UUID]bool, len(seeded))
			for _, m := range seeded {
				alreadySeeded[m.PermissionID] = true
			}

			var grants []model.SystemRolePermission
			var marks []model.SystemRolePermissionSeed
			for _, perm := range rolePermissions {
				if alreadySeeded[perm.ID] {
					continue
				}
				grants = append(grants, model.SystemRolePermission{SystemRoleID: role.ID, PermissionID: perm.ID})
				marks = append(marks, model.SystemRolePermissionSeed{SystemRoleID: role.ID, PermissionID: perm.ID})
			}
			if len(grants) > 0 {
				onConflict := clause.OnConflict{DoNothing: true}
				if err := s.db.Omit(clause.Associations).Clauses(onConflict).Create(&grants).Error; err != nil {
					return fmt.Errorf("failed to assign permissions to role %s: %w", r.Role, err)
				}
				if err := s.db.Clauses(onConflict).Create(&marks).Error; err != nil {
					return fmt.Errorf("failed to mark seeded permissions of role %s: %w", r.Role, err)
				}
			}
		}

		log.Printf("Seeded role: %s with %d permissions\n", r.Role, len(rolePermissions))
	}

	log.Printf("Successfully seeded %d roles\n", len(roles))
	return nil
}

// SeedAll chạy toàn bộ seed quyền + role trong MỘT transaction (S2). Idempotent và additive: chạy
// bao nhiêu lần cũng không xoá quyền admin đã chỉnh tay (xem SeedRoles).
func (s *Seeder) SeedAll(dataDir string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return (&Seeder{db: tx}).seedAll(dataDir)
	})
}

func (s *Seeder) seedAll(dataDir string) error {
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
	if err := s.seedRoles(rolesFilePath); err != nil {
		return err
	}

	log.Println("All seeds completed successfully!")
	return nil
}
