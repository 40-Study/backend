package dto

import "github.com/google/uuid"

// RoleDto - Role information in responses
type RoleDto struct {
	ID   string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Code string `json:"code" example:"student"`
	Name string `json:"name" example:"Học sinh"`
}

type DeviceInfoDTO struct {
	DeviceID   string `json:"device_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	DeviceName string `json:"device_name" validate:"required,min=2,max=100" example:"iPhone 14 Pro"`
	OS         string `json:"os" validate:"required,max=50" example:"iOS 16.5"`
	AppVersion string `json:"app_version,omitempty" validate:"omitempty,max=50" example:"1.0.0"`
	UserAgent  string `json:"user_agent,omitempty" validate:"omitempty,max=512" example:"Mozilla/5.0..."`
}


type LoginRequestDto struct {
	Email      string        `json:"email" validate:"required,email" example:"student@example.com"`
	Password   string        `json:"password" validate:"required,min=8" example:"ResilientPass123!"`
	DeviceInfo DeviceInfoDTO `json:"device_info" validate:"required"`
}

type UserResponseDto struct {
	ID          uuid.UUID `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Username    string    `json:"username" example:"student123"`
	Email       string    `json:"email" example:"student@example.com"`
	FullName    *string   `json:"full_name,omitempty" example:"Nguyen Van A"`
	Phone       *string   `json:"phone,omitempty" example:"+84901234567"`
	AvatarUrl   *string   `json:"avatar_url,omitempty" example:"https://example.com/avatar.jpg"`
	DateOfBirth *string   `json:"date_of_birth,omitempty" example:"2005-01-01"`
	Bio         *string   `json:"bio,omitempty" example:"Sinh viên PTIT"`
	IsActive    bool      `json:"is_active" example:"true"`
	CreatedAt   string    `json:"created_at" example:"2023-01-01T00:00:00Z"`
}

type DeviceSessionDto struct {
	DeviceID   string `json:"device_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	DeviceName string `json:"device_name" example:"iPhone 14 Pro"`
	UserAgent  string `json:"user_agent,omitempty" example:"Mozilla/5.0..."`
	LoggedInAt string `json:"logged_in_at" example:"2024-01-01T00:00:00Z"`
	IsCurrent  bool   `json:"is_current,omitempty"`
}

// SystemRoleDto - System role (STUDENT, TEACHER, PARENT, ORG_OWNER)
type SystemRoleDto struct {
	ID   string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name string `json:"name" example:"STUDENT"`
}

// OrgRoleDto - Organization role với context
type OrgRoleDto struct {
	ID               string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	RoleName         string `json:"role_name" example:"Admin"`
	OrganizationID   string `json:"organization_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	OrganizationName string `json:"organization_name" example:"Trường THPT ABC"`
}

// ProfileDto - Unified profile (gộp từ SystemRole và OrgRole)
// Type: "system" = SystemRole, "org" = OrgRole
type ProfileDto struct {
	ID               string     `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Type             string     `json:"type,omitempty" example:"system"`
	SystemRoleID     string     `json:"system_role_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
	DisplayName      string     `json:"display_name,omitempty" example:"Giáo viên Trung tâm A"`
	RoleName         string     `json:"role_name" example:"TEACHER"`
	Description      *string    `json:"description,omitempty" example:"Giáo viên trong hệ thống"`
	Status           string     `json:"status,omitempty" example:"active"`
	CreatedAt        string     `json:"created_at,omitempty" example:"2024-01-01T00:00:00Z"`
	OrganizationID   *string    `json:"organization_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
	OrganizationName *string    `json:"organization_name,omitempty" example:"Trung tâm A"`
}

// LastProfileDto - Lưu vào Redis để track profile cuối cùng
type LastProfileDto struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"` 
}

// OrgContextDto - Organization context trong login flow
type OrgContextDto struct {
	ID   string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name string `json:"name" example:"Trường THPT ABC"`
}

// EntryContext - Gợi ý FE navigate đến đâu sau login
type EntryContext struct {
	PrimaryRole   string `json:"primary_role" example:"STUDENT"`
	RequiresSetup bool   `json:"requires_setup" example:"false"`
	SetupEndpoint string `json:"setup_endpoint,omitempty" example:"/me/children"`
}

// LoginResponseDto - Multi-step login:
// Step 1: Nếu có nhiều role (system + org) → trả session_token + roles để chọn
// Auto-complete khi chỉ có 1 role
type LoginResponseDto struct {
	Completed    bool   `json:"completed"`
	SessionToken string `json:"session_token,omitempty"`

	// Hiển thị: "Giáo viên tự do", "Kế toán - Trường PTIT", etc.
	Roles                []UnifiedRoleDto `json:"roles,omitempty"`
	RequiresOrgSelection bool             `json:"requires_org_selection,omitempty"`
	Organizations        []OrgContextDto `json:"organizations,omitempty"`

	// Chỉ có khi Completed = true
	AccessToken   string            `json:"access_token,omitempty"`
	RefreshToken  string            `json:"refresh_token,omitempty"`
	User          *UserResponseDto  `json:"user,omitempty"`
	ActiveRole    *UnifiedRoleDto   `json:"active_role,omitempty"` // Changed to UnifiedRoleDto
	EntryContext  *EntryContext     `json:"entry_context,omitempty"` // Gợi ý FE nên vào đâu sau login
	CurrentDevice *DeviceSessionDto `json:"current_device,omitempty"` // Thông tin thiết bị đang login, để FE hiển thị và quản lý phiên đăng nhập
}

// SelectProfileRequestDto - Chọn role sau khi login
type SelectProfileRequestDto struct {
	SessionToken string `json:"session_token" validate:"required"`
	SystemRoleID string `json:"system_role_id" validate:"required,uuid"`
}

// SwitchProfileResponseDto - Response sau khi switch profile
type SwitchProfileResponseDto struct {
	AccessToken   string           `json:"access_token"`
	RefreshToken  string           `json:"refresh_token"`
	ActiveProfile ProfileDto       `json:"active_profile"`
	CurrentDevice DeviceSessionDto `json:"current_device,omitempty"` // Thông tin thiết bị đang login, để FE hiển thị và quản lý phiên đăng nhập
}

// SelectOrgRequestDto - Chọn org sau khi đã chọn role.
// organization_id rỗng hoặc không gửi = chọn chế độ "Độc lập" (không thuộc org nào).
type SelectOrgRequestDto struct {
	SessionToken   string `json:"session_token" validate:"required"`
	OrganizationID string `json:"organization_id,omitempty" validate:"omitempty,uuid"`
}

// SwitchProfileRequestDto - Đổi profile khi đã đăng nhập
// ProfileType: "system" hoặc "org"
// ProfileID: UserSystemRole.ID hoặc UserOrganizationRole.ID
type SwitchProfileRequestDto struct {
	ProfileType string `json:"profile_type" validate:"required,oneof=system org"`
	ProfileID   string `json:"profile_id" validate:"required,uuid"`
}

// AddSystemProfileRequestDto - Thêm system profile mới (TEACHER, STUDENT...)
type AddSystemProfileRequestDto struct {
	SystemRoleID string `json:"system_role_id" validate:"required,uuid"`
}

// AddSystemProfileResponseDto - Response sau khi thêm profile
type AddSystemProfileResponseDto struct {
	Profile  ProfileDto   `json:"profile"`
	Profiles []ProfileDto `json:"profiles"`
}

type RefreshTokenResponseDto struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}


type RequestPasswordResetDto struct {
	Email string `json:"email" validate:"required,email" example:"student@example.com"`
}

type ResetPasswordRequestDto struct {
	Email           string `json:"email" validate:"required,email"`
	Otp             string `json:"otp" validate:"required,len=6,numeric"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=72"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}


type ChangePasswordRequestDto struct {
	OldPassword     string        `json:"old_password" validate:"required,min=8,max=72" example:"OldPass123!"`
	NewPassword     string        `json:"new_password" validate:"required,min=8,max=72,nefield=OldPassword,containsany=!@#$%^&*()" example:"NewSecurePass123!"`
	ConfirmPassword string        `json:"confirm_password" validate:"required,eqfield=NewPassword" example:"NewSecurePass123!"`
	DeviceInfo      DeviceInfoDTO `json:"device_info" validate:"required"`
	RevokeOthers    bool          `json:"revoke_others" example:"true"`
}


// RegisterRequestDto - Request body for POST /auth/register/request
// Gửi thông tin đăng ký và nhận OTP qua email. Chỉ chọn 1 system role.
// Sau này có thể thêm role qua profile management hoặc được tổ chức mời.
type RegisterRequestDto struct {
	Email           string `json:"email" validate:"required,email,max=255" example:"student@example.com"`
	Password        string `json:"password" validate:"required,min=8,max=72" example:"SecurePass123!"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=Password" example:"SecurePass123!"`
	UserName        string `json:"user_name" validate:"required,min=3,max=100" example:"student123"`
	FullName        string `json:"full_name,omitempty" validate:"omitempty,min=2,max=255" example:"Nguyen Van A"`
	RoleID          string `json:"role_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// RegisterResponseDto - Response for successful registration
type RegisterResponseDto struct {
	ID       string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Email    string  `json:"email" example:"student@example.com"`
	UserName string  `json:"user_name" example:"student123"`
	FullName *string `json:"full_name,omitempty" example:"Nguyen Van A"`
	RoleID   string  `json:"role_id"`
}

// VerifyOtpRequestDto - Request body for POST /auth/register
// Xác thực OTP và tạo user thật trong database
type VerifyOtpRequestDto struct {
	Email string `json:"email" validate:"required,email" example:"student@example.com"`
	OTP   string `json:"otp" validate:"required,len=6,numeric" example:"123456"`
}

// VerifyOtpResponseDto - Response for OTP verification (kept for backward compatibility)
type VerifyOtpResponseDto struct {
	User UserResponseDto `json:"user"`
}

// UpdateMeRequestDto - Request body for updating user profile
// All fields are optional (partial update)
type UpdateMeRequestDto struct {
	Username    *string `json:"username,omitempty" validate:"omitempty,alphanum,min=3,max=30" example:"student123"`
	FullName    *string `json:"full_name,omitempty" validate:"omitempty,min=2,max=255" example:"Nguyen Van A"`
	Phone       *string `json:"phone,omitempty" validate:"omitempty,e164" example:"+84901234567"`
	DateOfBirth *string `json:"date_of_birth,omitempty" validate:"omitempty,datetime=2006-01-02" example:"2005-01-01"`
	Bio         *string `json:"bio,omitempty" validate:"omitempty,max=1000" example:"Sinh viên PTIT"`
	AvatarURL   *string `json:"avatar_url,omitempty" validate:"omitempty,url,max=500" example:"https://example.com/avatar.jpg"`
}

// MyProfileResponseDto - Full profile response including user info, roles, orgs
type MyProfileResponseDto struct {
	User          UserResponseDto     `json:"user"`
	SystemRoles   []SystemRoleDto     `json:"system_roles"`
	Organizations []MyOrganizationDto `json:"organizations"`
	ActiveRole    *SystemRoleDto      `json:"active_role,omitempty"`
	ActiveOrg     *OrgContextDto      `json:"active_org,omitempty"`
}

// ========== UNIFIED ROLE SELECTION (NEW FLOW) ==========

// UnifiedRoleDto - Một role có thể là SystemRole hoặc OrganizationRole
// Frontend hiển thị list này để user chọn 1 cái
type UnifiedRoleDto struct {
	ID               string  `json:"id"`                          // UserSystemRole.ID hoặc UserOrganizationRole.ID
	Type             string  `json:"type"`                        // "system" hoặc "organization"
	RoleName         string  `json:"role_name"`                   // Tên role: "Giáo viên tự do", "Kế toán", etc.
	OrganizationID   *string `json:"organization_id,omitempty"`   // Chỉ có nếu type="organization"
	OrganizationName *string `json:"organization_name,omitempty"` // Chỉ có nếu type="organization"
	DisplayName      string  `json:"display_name"`                // "Giáo viên tự do" hoặc "Kế toán - Trường PTIT"
}

// GetMyRolesResponseDto - Response cho GET /auth/my-roles
type GetMyRolesResponseDto struct {
	Roles []UnifiedRoleDto `json:"roles"`
}

// SelectRoleRequestDto - Request cho POST /auth/select-role
// Gộp SelectProfile + SelectOrg thành 1 bước
type SelectRoleRequestDto struct {
	SessionToken string `json:"session_token" validate:"required"`
	RoleID       string `json:"role_id" validate:"required,uuid"` // UserSystemRole.ID hoặc UserOrganizationRole.ID
	RoleType     string `json:"role_type" validate:"required,oneof=system organization"`
}

// SelectRoleResponseDto - Response sau khi chọn role xong
type SelectRoleResponseDto struct {
	Completed            bool              `json:"completed"`
	SessionToken         string            `json:"session_token,omitempty"`
	RequiresOrgSelection bool              `json:"requires_org_selection,omitempty"`
	Organizations        []OrgContextDto   `json:"organizations,omitempty"`
	AccessToken          string            `json:"access_token,omitempty"`
	RefreshToken         string            `json:"refresh_token,omitempty"`
	User                 UserResponseDto   `json:"user"`
	ActiveRole           UnifiedRoleDto    `json:"active_role"`
	ActiveOrg            *OrgContextDto    `json:"active_org,omitempty"`
	EntryContext         *EntryContext     `json:"entry_context,omitempty"`
	CurrentDevice        *DeviceSessionDto `json:"current_device,omitempty"`
}

// SwitchRoleRequestDto - Đổi role khi đã đăng nhập
type SwitchRoleRequestDto struct {
	RoleID   string `json:"role_id" validate:"required,uuid"`
	RoleType string `json:"role_type" validate:"required,oneof=system organization"`
}

// SwitchOrgRequestDto - Đổi org khi đã đăng nhập (giữ nguyên role)
// OrganizationID rỗng = chuyển về chế độ "Độc lập" (không thuộc org nào)
type SwitchOrgRequestDto struct {
	OrganizationID string `json:"organization_id,omitempty" validate:"omitempty,uuid"`
}

// ========== PROFILE MANAGEMENT ==========
// Mỗi user có thể tạo nhiều profile (giống FB: cá nhân, page, business)
// Profile = UserSystemRole (liên kết user với 1 system role đã seed sẵn)

// SystemRoleOptionDto - System role có sẵn để tạo profile
type SystemRoleOptionDto struct {
	ID          string  `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Name        string  `json:"name" example:"TEACHER"`
	Description *string `json:"description,omitempty" example:"Giáo viên"`
}

// CreateProfileRequestDto - Tạo profile mới với system role
type CreateProfileRequestDto struct {
	SystemRoleID string `json:"system_role_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
}

