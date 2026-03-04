# Session Changelog: Restore API, Migration Fix, Org Role Fix

> Ngày: 2026-03-04
> Phạm vi: Role management, Database migration, Postman collection

---

## Mục lục

1. [Tổng quan kiến trúc](#1-tổng-quan-kiến-trúc)
2. [Thêm API Restore cho System Role và Org Role](#2-thêm-api-restore-cho-system-role-và-org-role)
3. [Sửa lỗi Create Org Role — cột `code` NOT NULL](#3-sửa-lỗi-create-org-role--cột-code-not-null)
4. [Thêm bảng thiếu vào Migration](#4-thêm-bảng-thiếu-vào-migration)
5. [Sửa Postman Collection — Revoke API](#5-sửa-postman-collection--revoke-api)
6. [Giải thích các API Organization Role](#6-giải-thích-các-api-organization-role)
7. [Danh sách file thay đổi](#7-danh-sách-file-thay-đổi)

---

## 1. Tổng quan kiến trúc

### 1.1. Kiến trúc 4 Layer

Mỗi feature trong project đều đi qua 4 layer:

```
HTTP Request
    ↓
┌─────────┐
│  Router  │  Đăng ký URL + HTTP method → map đến handler
└────┬────┘
     ↓
┌─────────┐
│ Handler  │  Parse request (params, body, query) → gọi service → trả JSON response
└────┬────┘
     ↓
┌─────────┐
│ Service  │  Business logic (validate, check quyền, xử lý) → gọi repository
└────┬────┘
     ↓
┌──────────┐
│Repository│  Tương tác trực tiếp với database qua GORM
└────┬─────┘
     ↓
  Database
```

**Tại sao tách layer?**

| Layer | Trách nhiệm | Lợi ích |
|-------|-------------|---------|
| Router | Khai báo endpoint, gắn middleware | Thay đổi URL không ảnh hưởng logic |
| Handler | Parse HTTP input, format HTTP output | Đổi framework (Fiber → Gin) chỉ sửa handler |
| Service | Business rules, validation | Test được mà không cần HTTP server |
| Repository | SQL queries | Đổi database (Postgres → MySQL) chỉ sửa repo |

### 1.2. Cấu trúc thư mục liên quan

```
internal/
├── model/          # Định nghĩa struct mapping với bảng database (GORM model)
├── dto/            # Data Transfer Object — struct cho request/response API
├── repository/     # Truy vấn database
├── service/        # Business logic
├── handler/        # Xử lý HTTP request/response
├── router/         # Đăng ký route
├── middleware/      # Auth, logging, etc.
├── database/       # Kết nối DB, migration
└── app/            # Khởi tạo và wire tất cả lại với nhau
```

### 1.3. Hệ thống Role

Project có 2 loại role hoàn toàn tách biệt:

```
┌─────────────────────────────────────────────────────────┐
│                    System Roles                         │
│  Bảng: system_roles                                     │
│  Ví dụ: STUDENT, TEACHER, PARENT, ORG_OWNER            │
│  Phạm vi: Toàn hệ thống (platform-wide)                │
│  Dùng để: Xác định user thuộc nhóm nào trên platform   │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│                   Org Roles                              │
│  Bảng: roles                                             │
│  Ví dụ: Giáo viên chính, Trợ giảng, Quản lý            │
│  Phạm vi: Trong 1 tổ chức cụ thể                       │
│  Dùng để: Phân quyền cụ thể trong org                  │
└─────────────────────────────────────────────────────────┘
```

**Mối quan hệ:**
- 1 User có thể có **nhiều System Role** (vừa là TEACHER vừa là STUDENT)
- 1 User có thể thuộc **nhiều Organization** với **nhiều Org Role** khác nhau
- Junction tables: `user_system_roles`, `user_organization_roles`

---

## 2. Thêm API Restore cho System Role và Org Role

### 2.1. Bối cảnh

Khi xóa role bằng `DELETE /api/system-roles/:id` (không có `?hard_delete=true`), GORM thực hiện **soft delete**:

```sql
-- Soft delete: chỉ đánh dấu, không xóa thật
UPDATE system_roles SET deleted_at = '2026-03-04 10:00:00' WHERE id = 'abc-123';

-- Hard delete: xóa thật khỏi DB
DELETE FROM system_roles WHERE id = 'abc-123';
```

Soft delete giúp:
- Khôi phục được nếu xóa nhầm
- Giữ lại audit trail
- Các bản ghi liên quan (user_system_roles) không bị cascade xóa

**Vấn đề**: Có soft delete nhưng **không có API restore** → role bị xóa mềm thì "chết" luôn, không cách nào khôi phục qua API.

### 2.2. Giải pháp

Thêm endpoint `PATCH /:id/restore` cho cả 2 loại role.

**Tại sao dùng PATCH?**
- `PUT` = thay thế toàn bộ resource
- `PATCH` = update **một phần** resource
- Restore chỉ update 1 field (`deleted_at = NULL`) → `PATCH` phù hợp nhất

### 2.3. Chi tiết thay đổi theo từng layer

#### Layer 1: Repository (truy vấn database)

**File: `internal/repository/system_role_repository.go`**

Thêm vào interface:
```go
RestoreSystemRole(ctx context.Context, id uuid.UUID) error
```
- `ctx context.Context`: context để cancel request nếu client disconnect
- `id uuid.UUID`: ID của system role cần restore
- Return `error`: nil nếu thành công, error nếu thất bại

Thêm implementation:
```go
func (r *SystemRoleRepository) RestoreSystemRole(ctx context.Context, id uuid.UUID) error {
    return r.db.WithContext(ctx).
        Unscoped().
        Model(&model.SystemRole{}).
        Where("id = ?", id).
        Update("deleted_at", nil).
        Error
}
```

Giải thích từng dòng:
| Dòng | Ý nghĩa |
|------|---------|
| `r.db.WithContext(ctx)` | Gắn context vào query — nếu client cancel request, query cũng bị cancel |
| `.Unscoped()` | **BẮT BUỘC** — GORM mặc định tự thêm `WHERE deleted_at IS NULL` vào mọi query. Không có `Unscoped()` thì không tìm được record đã soft-delete |
| `.Model(&model.SystemRole{})` | Chỉ định bảng `system_roles` |
| `.Where("id = ?", id)` | Lọc theo ID. Dùng parameterized query (`?`) để chống SQL injection |
| `.Update("deleted_at", nil)` | Set `deleted_at = NULL` → record "sống lại", GORM sẽ lại thấy nó trong query bình thường |
| `.Error` | Trả về error nếu query thất bại (ví dụ: DB connection lost) |

SQL thực tế được generate:
```sql
UPDATE system_roles SET deleted_at = NULL, updated_at = '2026-03-04T...' WHERE id = 'abc-123';
```

**File: `internal/repository/role_repository.go`**

Tương tự, thêm cho bảng `roles`:
```go
func (r *RoleRepository) RestoreRole(ctx context.Context, id uuid.UUID) error {
    return r.db.WithContext(ctx).Unscoped().Model(&model.Role{}).Where("id = ?", id).Update("deleted_at", nil).Error
}
```

#### Layer 2: Service (business logic)

**File: `internal/service/system_role_service.go`**

Thêm vào interface:
```go
RestoreSystemRole(ctx context.Context, id uuid.UUID) error
```

Thêm implementation:
```go
func (s *SystemRoleService) RestoreSystemRole(ctx context.Context, id uuid.UUID) error {
    return s.repo.RestoreSystemRole(ctx, id)
}
```

- Hiện tại chỉ delegate xuống repo vì chưa có business rule phức tạp
- Sau này có thể thêm: check quyền admin, validate trạng thái, log audit, etc.

**File: `internal/service/role_service.go`** — tương tự.

#### Layer 3: Handler (HTTP request/response)

**File: `internal/handler/system_role_handler.go`**

Thêm vào interface:
```go
RestoreSystemRole(c *fiber.Ctx) error
```

Thêm implementation:
```go
func (h *SystemRoleHandler) RestoreSystemRole(c *fiber.Ctx) error {
    // 1. Parse UUID từ URL parameter ":id"
    id, err := uuid.Parse(c.Params("id"))
    if err != nil {
        // UUID không hợp lệ → 400 Bad Request
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "message": "Invalid system role ID",
            "error":   err.Error(),
        })
    }

    // 2. Gọi service để restore
    if err := h.service.RestoreSystemRole(c.Context(), id); err != nil {
        // Restore thất bại (ví dụ: ID không tồn tại, DB error)
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "message": "Failed to restore system role",
            "error":   err.Error(),
        })
    }

    // 3. Thành công → 200 OK
    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "message": "System role restored successfully",
    })
}
```

Flow xử lý:
```
PATCH /api/system-roles/abc-123/restore
    ↓
c.Params("id") → "abc-123"
    ↓
uuid.Parse("abc-123") → UUID object (hoặc error nếu format sai)
    ↓
service.RestoreSystemRole(ctx, uuid) → repo.RestoreSystemRole(ctx, uuid)
    ↓
UPDATE system_roles SET deleted_at = NULL WHERE id = 'abc-123'
    ↓
200 OK: {"message": "System role restored successfully"}
```

**File: `internal/handler/role_handler.go`** — tương tự cho org role.

#### Layer 4: Router (đăng ký endpoint)

**File: `internal/router/system_role_router.go`**

```go
systemRoles.Patch("/:id/restore", systemRoleHandler.RestoreSystemRole)
```

- `Patch` = HTTP method PATCH
- `"/:id/restore"` = URL pattern, `:id` là dynamic parameter
- Route đầy đủ: `PATCH /api/system-roles/:id/restore` (vì group đã set prefix `/system-roles`)
- Nằm trong group có `middleware.AuthMiddleware` → yêu cầu đăng nhập

**File: `internal/router/role_router.go`**

```go
orgRoles.Patch("/:id/restore", roleHandler.RestoreRole)
```

Route đầy đủ: `PATCH /api/org-roles/:id/restore`

### 2.4. Tổng hợp API mới

| Method | Endpoint | Auth | Mô tả |
|--------|----------|------|-------|
| `PATCH` | `/api/system-roles/:id/restore` | Required | Khôi phục system role đã soft-delete |
| `PATCH` | `/api/org-roles/:id/restore` | Required | Khôi phục org role đã soft-delete |

### 2.5. Ví dụ sử dụng

```bash
# 1. Xóa mềm system role
DELETE /api/system-roles/abc-123
# → role bị ẩn khỏi GET /api/system-roles

# 2. Xem role đã xóa (dùng query status=deleted)
GET /api/system-roles?status=deleted
# → thấy role abc-123 trong danh sách

# 3. Khôi phục
PATCH /api/system-roles/abc-123/restore
# → role xuất hiện lại trong GET /api/system-roles
```

---

## 3. Sửa lỗi Create Org Role — cột `code` NOT NULL

### 3.1. Triệu chứng

Gọi `POST /api/org-roles` → lỗi:
```json
{
    "error": "ERROR: null value in column \"code\" of relation \"roles\" violates not-null constraint (SQLSTATE 23502)",
    "message": "Failed to create role"
}
```

### 3.2. Phân tích nguyên nhân gốc

**Database SQL gốc** (`docs/database/40study_complete_schema.sql`) định nghĩa bảng `roles` với:
```sql
CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(50) UNIQUE NOT NULL,      -- ← CÓ cột này
    name VARCHAR(100) NOT NULL,
    is_system_role BOOLEAN DEFAULT FALSE,  -- ← CÓ cột này
    organization_id UUID REFERENCES organizations(id),
    ...
);
```

**Go model hiện tại** (`internal/model/role.go`):
```go
type Role struct {
    BaseModel
    Name           string         // ← có
    OrganizationID uuid.UUID      // ← có
    Description    sql.NullString // ← có
    Status         string         // ← có
    // THIẾU: Code, IsSystemRole
}
```

**Mâu thuẫn**: Database có cột `code` (NOT NULL), Go model không có field `Code`.

Khi GORM insert:
```sql
INSERT INTO roles (id, name, organization_id, description, status, created_at, updated_at)
VALUES ('...', 'Senior Teacher', '...', NULL, 'active', NOW(), NOW());
-- Cột "code" không được set → NULL → vi phạm NOT NULL constraint!
```

**Tại sao mâu thuẫn xảy ra?**
- Thiết kế cũ: 1 bảng `roles` chứa cả system role (`is_system_role = true`) và org role
- Thiết kế mới: tách thành 2 bảng `system_roles` + `roles`
- Database vẫn giữ schema cũ, Go model đã theo thiết kế mới

### 3.3. Quyết định thiết kế

**Có nên giữ `code` không?**

| Giữ `code` | Bỏ `code` |
|-------------|-----------|
| Dùng cho tham chiếu cố định trong code | Org role do user tự tạo, không cần code cố định |
| Thêm 1 field phải quản lý | Đã có `name` (unique trong org) + `id` |
| Phù hợp cho system role cố định | Không phù hợp cho role dynamic |

**Kết luận**: Bỏ `code` vì org role là dynamic (user tự tạo/đặt tên), khác với system role cố định.

### 3.4. Thay đổi thực hiện

Chạy SQL trên database:
```sql
ALTER TABLE roles DROP COLUMN IF EXISTS code;
ALTER TABLE roles DROP COLUMN IF EXISTS is_system_role;
```

- `DROP COLUMN IF EXISTS`: an toàn — không lỗi nếu cột đã bị xóa trước đó
- Drop cả `is_system_role` vì đã tách bảng, cột này vô nghĩa trong thiết kế mới

**Không cần thay đổi Go code** vì model đã đúng (không có field `Code`).

### 3.5. Kết quả

```bash
POST /api/org-roles
Body: {"name": "Senior Teacher", "organization_id": "..."}
# → 201 Created ✅ (trước đó: 400 Error)
```

---

## 4. Thêm bảng thiếu vào Migration

### 4.1. Triệu chứng

Nhiều API liên quan đến permission bị lỗi:
```json
{
    "error": "ERROR: relation \"permissions\" does not exist (SQLSTATE 42P01)"
}
```

### 4.2. Nguyên nhân

**File: `internal/database/postgres.go`**

Hàm `Migrate` chỉ tạo 5 bảng:
```go
// TRƯỚC — THIẾU nhiều bảng
func Migrate(db *gorm.DB) error {
    return db.AutoMigrate(
        &model.User{},
        &model.Role{},
        &model.SystemRole{},
        &model.UserOrganizationRole{},
        &model.UserSystemRole{},
    )
}
```

Thiếu: `permissions`, `organizations`, `system_role_permissions`, `role_permissions`.

GORM `AutoMigrate` chỉ tạo bảng cho các model được liệt kê. Nếu model không nằm trong danh sách → bảng không được tạo.

### 4.3. Giải pháp

```go
// SAU — Đầy đủ các bảng core
func Migrate(db *gorm.DB) error {
    return db.AutoMigrate(
        &model.Permission{},            // 1
        &model.Organization{},          // 2
        &model.SystemRole{},            // 3
        &model.Role{},                  // 4
        &model.SystemRolePermission{},  // 5
        &model.RolePermission{},        // 6
        &model.User{},                  // 7
        &model.UserOrganizationRole{},  // 8
        &model.UserSystemRole{},        // 9
    )
}
```

### 4.4. Thứ tự Migration — Tại sao quan trọng?

Các bảng có foreign key phải được tạo **SAU** bảng nó tham chiếu:

```
Permission ──────────────────┐
                             ├──→ SystemRolePermission (FK: permission_id, system_role_id)
SystemRole ──────────────────┘

Permission ──────────────────┐
                             ├──→ RolePermission (FK: permission_id, role_id)
Role ────────────────────────┘
  ↑
  └── Organization (Role.organization_id → organizations.id)

User ────────────────────────┐
Role ────────────────────────┤
Organization ────────────────┤──→ UserOrganizationRole (FK: user_id, role_id, organization_id)
                             │
SystemRole ──────────────────┤──→ UserSystemRole (FK: user_id, system_role_id)
```

Nếu tạo `RolePermission` trước `Permission` → lỗi: foreign key tham chiếu đến bảng chưa tồn tại.

### 4.5. GORM AutoMigrate hoạt động như thế nào?

```
AutoMigrate chạy cho mỗi model:
    ↓
Bảng chưa tồn tại? → CREATE TABLE ...
    ↓
Bảng đã tồn tại?
    → Model có field mới? → ALTER TABLE ADD COLUMN ...
    → Model thiếu field so với DB? → KHÔNG LÀM GÌ (an toàn, không xóa cột)
    → Kiểu dữ liệu thay đổi? → KHÔNG LÀM GÌ (phải ALTER thủ công)
```

**Đặc điểm quan trọng**: AutoMigrate **KHÔNG BAO GIỜ xóa cột hoặc bảng**. Nó chỉ thêm, không bớt → an toàn cho production.

---

## 5. Sửa Postman Collection — Revoke API

### 5.1. Vấn đề

**File: `postman/Study_API_Collection.postman_collection.json`**

```json
{
    "name": "Revoke Org Role from User",
    "request": {
        "method": "DELETE",
        "url": "{{base_url}}/api/users/{{user_id}}/org-roles/{{role_id}}"
    }
}
```

`{{role_id}}` là ID của bảng `roles` (vai trò là gì), nhưng API cần **assignment ID** — ID của bảng `user_organization_roles` (bản ghi gán role cho user).

### 5.2. Tại sao cần assignment ID thay vì role_id?

Xem ví dụ:

```
User A thuộc org "Galaxy" với role "Giáo viên"    → assignment ID = aaa-111
User A thuộc org "STEM" với role "Giáo viên"      → assignment ID = bbb-222
User B thuộc org "Galaxy" với role "Giáo viên"    → assignment ID = ccc-333
```

Cả 3 đều cùng `role_id` (Giáo viên). Nếu revoke bằng `role_id`:
- Revoke của User A ở Galaxy hay STEM? Hay cả hai?
- Revoke của User A hay User B?

→ **Không xác định được**. Phải dùng assignment ID để chỉ rõ.

### 5.3. Giải pháp

Sửa Postman:
```json
{
    "name": "Revoke Org Role from User",
    "request": {
        "method": "DELETE",
        "url": "{{base_url}}/api/users/{{user_id}}/org-roles/{{org_role_assignment_id}}"
    },
    "description": "org_role_assignment_id là ID của bản ghi trong user_organization_roles, lấy từ response của Get User Org Roles"
}
```

### 5.4. Cách lấy assignment ID

```
Step 1: GET /api/users/:user_id/org-roles

Response:
{
    "data": [
        {
            "id": "1d51f1a7-...",              ← ĐÂY LÀ assignment ID
            "role_id": "3e757687-...",          ← ID của role (Senior Teacher)
            "organization_id": "bbbbbbbb-...",  ← ID của org (Galaxy)
            "role": { "name": "Senior Teacher" },
            "organization": { "name": "Galaxy" },
            "status": "active"
        }
    ]
}

Step 2: DELETE /api/users/:user_id/org-roles/1d51f1a7-...
                                              ↑
                                     dùng "id" từ step 1
```

### 5.5. Phân biệt 3 loại ID

| Field | Bảng | Ý nghĩa | Ví dụ |
|-------|------|---------|-------|
| `id` | `user_organization_roles` | **Ai** được gán **gì** ở **đâu** (bản ghi assignment) | `1d51f1a7-...` |
| `role_id` | `roles` | Vai trò là **gì** | `3e757687-...` = "Senior Teacher" |
| `organization_id` | `organizations` | Ở tổ chức **nào** | `bbbbbbbb-...` = "Galaxy" |

---

## 6. Giải thích các API Organization Role

### 6.1. CRUD Org Role

| Method | Endpoint | Mô tả |
|--------|----------|-------|
| `POST` | `/api/org-roles` | Tạo role mới trong org |
| `GET` | `/api/org-roles` | Danh sách tất cả org role (filter: `?organization_id=`, `?keyword=`, `?status=`) |
| `GET` | `/api/org-roles/:id` | Chi tiết 1 org role |
| `PUT` | `/api/org-roles/:id` | Cập nhật org role |
| `DELETE` | `/api/org-roles/:id` | Xóa org role (soft delete, thêm `?hard_delete=true` để xóa cứng) |
| `PATCH` | `/api/org-roles/:id/restore` | **MỚI** — Khôi phục org role đã soft-delete |

### 6.2. Gán/Thu hồi Org Role cho User

| Method | Endpoint | Mô tả |
|--------|----------|-------|
| `POST` | `/api/users/:user_id/org-roles` | Gán org role cho user |
| `GET` | `/api/users/:user_id/org-roles` | Xem tất cả org role của user |
| `DELETE` | `/api/users/:user_id/org-roles/:assignment_id` | Thu hồi org role (dùng assignment ID) |

### 6.3. Query Users theo Role/Org

| Method | Endpoint | Mô tả | Phạm vi |
|--------|----------|-------|---------|
| `GET` | `/api/org-roles/:role_id/users` | Users có role này | **Tất cả org** |
| `GET` | `/api/organizations/:org_id/roles/:role_id/users` | Users có role này trong org | **1 org cụ thể** |
| `GET` | `/api/organizations/:org_id/members` | Tất cả thành viên của org | **1 org**, mọi role |

**Ví dụ cụ thể:**

Role "Senior Teacher" tồn tại ở Galaxy, Nguyễn Huệ, STEM.

```
GET /api/org-roles/senior-teacher-id/users
→ Trả về: User A (Galaxy), User B (Nguyễn Huệ), User C (STEM)
  (TẤT CẢ Senior Teacher ở MỌI org)

GET /api/organizations/galaxy-id/roles/senior-teacher-id/users
→ Trả về: User A
  (Chỉ Senior Teacher ở GALAXY)

GET /api/organizations/galaxy-id/members
→ Trả về: User A (Senior Teacher), User D (Trợ giảng), User E (Quản lý)
  (TẤT CẢ thành viên Galaxy, BẤT KỂ role)
```

---

## 7. Danh sách file thay đổi

### 7.1. Files đã thay đổi

| File | Thay đổi |
|------|---------|
| `internal/repository/system_role_repository.go` | Thêm `RestoreSystemRole` vào interface + implementation |
| `internal/repository/role_repository.go` | Thêm `RestoreRole` vào interface + implementation |
| `internal/service/system_role_service.go` | Thêm `RestoreSystemRole` vào interface + implementation |
| `internal/service/role_service.go` | Thêm `RestoreRole` vào interface + implementation |
| `internal/handler/system_role_handler.go` | Thêm `RestoreSystemRole` vào interface + implementation |
| `internal/handler/role_handler.go` | Thêm `RestoreRole` vào interface + implementation |
| `internal/router/system_role_router.go` | Thêm route `PATCH /:id/restore` |
| `internal/router/role_router.go` | Thêm route `PATCH /:id/restore` |
| `internal/database/postgres.go` | Thêm `Permission`, `Organization`, junction tables vào `Migrate` |
| `postman/Study_API_Collection.postman_collection.json` | Sửa Revoke API dùng `{{org_role_assignment_id}}` |

### 7.2. Database changes (chạy thủ công)

```sql
ALTER TABLE roles DROP COLUMN IF EXISTS code;
ALTER TABLE roles DROP COLUMN IF EXISTS is_system_role;
```

### 7.3. Không thay đổi

| File | Lý do |
|------|-------|
| `internal/model/role.go` | Đã đúng — không có field `Code` (khớp với DB sau khi drop) |
| `internal/dto/permissionDTO.go` | User yêu cầu không động đến permission |
| `internal/service/auth_service.go` | Không liên quan đến session này |

---

## Appendix: GORM Soft Delete

### Cách hoạt động

Model có field `DeletedAt gorm.DeletedAt` (nằm trong `BaseModel`):

```go
type BaseModel struct {
    ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    CreatedAt time.Time      `gorm:"autoCreateTime"`
    UpdatedAt time.Time      `gorm:"autoUpdateTime"`
    DeletedAt gorm.DeletedAt `gorm:"index"` // ← Soft delete field
}
```

Khi GORM thấy field `DeletedAt`:

| Thao tác | SQL thực tế |
|----------|-------------|
| `db.Delete(&role)` | `UPDATE roles SET deleted_at = NOW() WHERE id = ?` |
| `db.Find(&roles)` | `SELECT * FROM roles WHERE deleted_at IS NULL` |
| `db.Unscoped().Find(&roles)` | `SELECT * FROM roles` (bao gồm cả đã xóa) |
| `db.Unscoped().Delete(&role)` | `DELETE FROM roles WHERE id = ?` (hard delete) |

### Tại sao Restore cần Unscoped?

```go
// KHÔNG CÓ Unscoped — SAI
db.Model(&SystemRole{}).Where("id = ?", id).Update("deleted_at", nil)
// SQL: UPDATE system_roles SET deleted_at = NULL
//      WHERE id = 'abc-123' AND deleted_at IS NULL
//                                ↑ GORM tự thêm điều kiện này!
// → Không tìm thấy record (vì nó đã bị delete, deleted_at IS NOT NULL)

// CÓ Unscoped — ĐÚNG
db.Unscoped().Model(&SystemRole{}).Where("id = ?", id).Update("deleted_at", nil)
// SQL: UPDATE system_roles SET deleted_at = NULL
//      WHERE id = 'abc-123'
// → Tìm thấy và update thành công
```
