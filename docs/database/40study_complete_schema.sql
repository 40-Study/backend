-- =============================================================================
-- 40STUDY - COMPLETE PRODUCTION DATABASE SCHEMA
-- =============================================================================
-- Platform: PostgreSQL 15+
-- Encoding: UTF-8
-- Collation: vi_VN.UTF-8 (Vietnamese)
--
-- Mô tả: Schema đầy đủ cho nền tảng giáo dục thông minh 40Study
-- Bao gồm: RBAC/ABAC Authorization, Organizations, Payment Installments,
--          Code Sandbox, Study Sessions, External Certificates, Age Verification
-- =============================================================================

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";  -- Cho full-text search tiếng Việt

-- =============================================================================
-- MODULE 1: ROLES & PERMISSIONS (RBAC + ABAC HYBRID)
-- =============================================================================
-- Thiết kế: Hybrid RBAC + ABAC
-- - RBAC: Roles gán cho users, roles có nhiều permissions
-- - ABAC: Permissions có thể bị override ở cấp user hoặc resource
-- - Context-aware: Permissions có thể phụ thuộc vào organization/course
--
-- Lý do chọn Hybrid:
-- 1. RBAC đơn thuần không đủ cho hệ thống giáo dục phức tạp
--    (VD: Teacher A có quyền edit course X nhưng không có quyền edit course Y)
-- 2. ABAC đơn thuần quá phức tạp để maintain
-- 3. Hybrid cho phép: base permissions từ role + fine-grained overrides
-- =============================================================================

-- Bảng định nghĩa các quyền hệ thống
-- Mục đích: Lưu trữ tất cả các quyền có thể có trong hệ thống
-- Tại sao tách riêng: Cho phép thêm/sửa quyền mà không cần sửa code
CREATE TABLE permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Mã quyền duy nhất, format: resource.action (VD: course.create, user.delete)
    code VARCHAR(100) UNIQUE NOT NULL,

    -- Tên hiển thị của quyền
    name VARCHAR(255) NOT NULL,

    -- Mô tả chi tiết quyền này cho phép làm gì
    description TEXT,

    -- Nhóm quyền để dễ quản lý (VD: course_management, user_management)
    category VARCHAR(50) NOT NULL,

    -- Loại resource mà quyền này áp dụng
    -- NULL = system-wide, 'course' = per-course, 'organization' = per-org
    resource_type VARCHAR(50),

    -- Quyền này có thể được gán trực tiếp cho user không (override role)
    is_assignable_to_user BOOLEAN DEFAULT TRUE,

    -- Metadata bổ sung (VD: required_subscription_level)
    metadata JSONB DEFAULT '{}',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE permissions IS 'Bảng định nghĩa tất cả quyền trong hệ thống. Mỗi quyền đại diện cho một hành động cụ thể trên một loại resource.';
COMMENT ON COLUMN permissions.code IS 'Mã quyền duy nhất, format: resource.action. VD: course.create, lesson.edit';
COMMENT ON COLUMN permissions.resource_type IS 'Loại resource: NULL=system-wide, course=per-course, organization=per-org';

-- Bảng vai trò trong hệ thống
-- Mục đích: Định nghĩa các vai trò cơ bản và vai trò tùy chỉnh
-- Tại sao cần: Gom nhóm permissions thành roles để dễ quản lý
CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Mã vai trò duy nhất
    code VARCHAR(50) UNIQUE NOT NULL,

    -- Tên hiển thị
    name VARCHAR(100) NOT NULL,

    -- Mô tả vai trò
    description TEXT,

    -- Vai trò hệ thống không thể xóa (student, teacher, admin, parent, ta)
    is_system_role BOOLEAN DEFAULT FALSE,

    -- Vai trò này thuộc organization nào (NULL = global role)
    -- Cho phép organizations tạo custom roles riêng
    organization_id UUID,

    -- Cấp độ ưu tiên khi có conflict (số cao = ưu tiên hơn)
    priority_level INT DEFAULT 0,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

COMMENT ON TABLE roles IS 'Bảng vai trò. Bao gồm vai trò hệ thống (student, teacher, admin) và vai trò tùy chỉnh của organization.';
COMMENT ON COLUMN roles.is_system_role IS 'TRUE = vai trò hệ thống, không thể xóa/sửa đổi cấu trúc';
COMMENT ON COLUMN roles.organization_id IS 'NULL = vai trò toàn cục, có giá trị = vai trò riêng của tổ chức đó';

-- Bảng liên kết Role - Permission (Many-to-Many)
-- Mục đích: Xác định role nào có những permissions nào
CREATE TABLE role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,

    -- Cho phép hoặc từ chối quyền này
    -- TRUE = grant, FALSE = deny (explicit deny)
    is_granted BOOLEAN DEFAULT TRUE,

    -- Điều kiện ABAC bổ sung (JSON)
    -- VD: {"own_resource_only": true} = chỉ áp dụng cho resource của chính user
    conditions JSONB DEFAULT '{}',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (role_id, permission_id)
);

COMMENT ON TABLE role_permissions IS 'Bảng liên kết vai trò với quyền. Một vai trò có nhiều quyền, một quyền thuộc nhiều vai trò.';
COMMENT ON COLUMN role_permissions.is_granted IS 'TRUE=cấp quyền, FALSE=từ chối rõ ràng (override grant từ role khác)';
COMMENT ON COLUMN role_permissions.conditions IS 'Điều kiện ABAC: {"own_resource_only":true, "max_items":10}';

-- =============================================================================
-- MODULE 2: ORGANIZATIONS (TỔ CHỨC GIÁO DỤC)
-- =============================================================================
-- Mục đích: Hỗ trợ trường học, trung tâm, doanh nghiệp đào tạo
-- Tại sao cần: 40Study không chỉ cho cá nhân mà còn cho tổ chức
-- =============================================================================

-- Bảng tổ chức giáo dục
CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Thông tin cơ bản
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    logo_url VARCHAR(500),
    banner_url VARCHAR(500),
    website VARCHAR(255),

    -- Loại tổ chức
    org_type VARCHAR(50) NOT NULL DEFAULT 'school',
    -- school: Trường học (tiểu học, THCS, THPT, ĐH)
    -- center: Trung tâm đào tạo
    -- company: Doanh nghiệp (đào tạo nội bộ)
    -- individual: Giảng viên cá nhân

    -- Thông tin liên hệ
    email VARCHAR(255),
    phone VARCHAR(20),
    address TEXT,
    city VARCHAR(100),
    country VARCHAR(100) DEFAULT 'Vietnam',

    -- Cài đặt
    settings JSONB DEFAULT '{}',
    -- VD: {"allow_public_courses": true, "require_approval": true}

    -- Subscription/License
    subscription_plan VARCHAR(50) DEFAULT 'free',
    subscription_expires_at TIMESTAMP,
    max_members INT DEFAULT 100,
    max_courses INT DEFAULT 10,

    -- Trạng thái
    is_verified BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    verified_at TIMESTAMP,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

COMMENT ON TABLE organizations IS 'Bảng tổ chức giáo dục: trường học, trung tâm, doanh nghiệp. Cho phép quản lý users, courses theo nhóm.';
COMMENT ON COLUMN organizations.org_type IS 'Loại: school=trường học, center=trung tâm, company=doanh nghiệp, individual=cá nhân';
COMMENT ON COLUMN organizations.settings IS 'Cài đặt JSON: allow_public_courses, require_approval, custom_branding...';

-- Index cho tìm kiếm organization
CREATE INDEX idx_organizations_slug ON organizations(slug);
CREATE INDEX idx_organizations_org_type ON organizations(org_type);
CREATE INDEX idx_organizations_is_active ON organizations(is_active) WHERE deleted_at IS NULL;

-- =============================================================================
-- MODULE 3: USERS & IDENTITY
-- =============================================================================

-- Bảng người dùng chính
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Thông tin đăng nhập
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255), -- NULL nếu chỉ dùng OAuth
    phone VARCHAR(20) UNIQUE,

    -- Thông tin cá nhân
    username VARCHAR(100) UNIQUE NOT NULL,
    full_name VARCHAR(255),
    avatar_url VARCHAR(500),
    date_of_birth DATE,
    gender VARCHAR(10), -- male, female, other, prefer_not_to_say
    bio TEXT,

    -- Địa chỉ
    address TEXT,
    city VARCHAR(100),
    country VARCHAR(100) DEFAULT 'Vietnam',

    -- Trạng thái tài khoản
    is_email_verified BOOLEAN DEFAULT FALSE,
    is_phone_verified BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    is_locked BOOLEAN DEFAULT FALSE,
    locked_reason TEXT,
    locked_until TIMESTAMP,

    -- Xác minh tuổi (quan trọng cho học sinh dưới 18)
    is_age_verified BOOLEAN DEFAULT FALSE,
    age_verification_method VARCHAR(50), -- document, parent_consent, school_verification
    age_verified_at TIMESTAMP,

    -- Metadata
    timezone VARCHAR(50) DEFAULT 'Asia/Ho_Chi_Minh',
    locale VARCHAR(10) DEFAULT 'vi',
    last_login_at TIMESTAMP,
    last_login_ip VARCHAR(45),
    login_count INT DEFAULT 0,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

COMMENT ON TABLE users IS 'Bảng người dùng chính. Lưu thông tin cơ bản, không lưu role trực tiếp (dùng user_roles).';
COMMENT ON COLUMN users.is_age_verified IS 'Xác minh tuổi: bắt buộc cho học sinh < 18 tuổi để tuân thủ COPPA/GDPR';
COMMENT ON COLUMN users.age_verification_method IS 'Phương thức xác minh: document=CMND/CCCD, parent_consent, school_verification';

-- Indexes cho users
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_phone ON users(phone) WHERE phone IS NOT NULL;
CREATE INDEX idx_users_is_active ON users(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_created_at ON users(created_at);

-- Bảng liên kết User - Role (Many-to-Many)
-- Mục đích: Một user có thể có nhiều roles (VD: vừa là student vừa là TA)
CREATE TABLE user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,

    -- Scope của role này
    -- NULL = global (role áp dụng toàn hệ thống)
    -- organization_id = role chỉ có hiệu lực trong org đó
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,

    -- Thời hạn của role (NULL = vĩnh viễn)
    granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,

    -- Ai cấp role này
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Ghi chú
    notes TEXT,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Một user chỉ có một instance của mỗi role trong mỗi context
    UNIQUE (user_id, role_id, organization_id)
);

COMMENT ON TABLE user_roles IS 'Liên kết user với role. Hỗ trợ role theo organization (VD: Teacher ở trường A, Student ở trường B).';
COMMENT ON COLUMN user_roles.organization_id IS 'NULL=role toàn cục, có giá trị=role chỉ trong organization đó';
COMMENT ON COLUMN user_roles.expires_at IS 'Thời điểm hết hạn role. VD: TA chỉ trong 1 học kỳ';

CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX idx_user_roles_role_id ON user_roles(role_id);
CREATE INDEX idx_user_roles_org_id ON user_roles(organization_id) WHERE organization_id IS NOT NULL;

-- Bảng override permission cho user cụ thể
-- Mục đích: Cấp/thu hồi quyền đặc biệt cho user mà không cần tạo role mới
CREATE TABLE user_permission_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,

    -- Context của override
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    resource_type VARCHAR(50), -- course, lesson, etc.
    resource_id UUID, -- ID của resource cụ thể

    -- Grant hoặc Deny
    is_granted BOOLEAN NOT NULL,

    -- Lý do override
    reason TEXT,

    -- Ai tạo override
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Thời hạn
    granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE user_permission_overrides IS 'Override quyền cho user cụ thể. Ưu tiên cao hơn role_permissions.';
COMMENT ON COLUMN user_permission_overrides.resource_id IS 'ID của resource cụ thể. VD: course_id để cấp quyền edit chỉ 1 course';

CREATE INDEX idx_user_perm_override_user ON user_permission_overrides(user_id);
CREATE INDEX idx_user_perm_override_resource ON user_permission_overrides(resource_type, resource_id)
    WHERE resource_id IS NOT NULL;

-- Bảng liên kết User - Organization (Many-to-Many)
CREATE TABLE organization_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Vai trò trong organization (khác với system role)
    member_role VARCHAR(50) NOT NULL DEFAULT 'member',
    -- owner: Chủ sở hữu (full quyền)
    -- admin: Quản trị viên
    -- manager: Quản lý (courses, members)
    -- teacher: Giảng viên
    -- ta: Trợ giảng
    -- student: Học viên
    -- member: Thành viên thường

    -- Phòng ban/Lớp học (tùy chọn)
    department VARCHAR(100),
    student_id VARCHAR(50), -- Mã học sinh/sinh viên

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'active',
    -- pending: Chờ duyệt
    -- active: Đang hoạt động
    -- suspended: Tạm ngưng
    -- left: Đã rời

    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    left_at TIMESTAMP,

    -- Audit
    invited_by UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_by UUID REFERENCES users(id) ON DELETE SET NULL,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (organization_id, user_id)
);

COMMENT ON TABLE organization_members IS 'Thành viên của tổ chức. Một user có thể thuộc nhiều organizations với vai trò khác nhau.';
COMMENT ON COLUMN organization_members.member_role IS 'Vai trò trong org: owner, admin, manager, teacher, ta, student, member';
COMMENT ON COLUMN organization_members.student_id IS 'Mã học sinh/sinh viên do organization cấp';

CREATE INDEX idx_org_members_org_id ON organization_members(organization_id);
CREATE INDEX idx_org_members_user_id ON organization_members(user_id);
CREATE INDEX idx_org_members_role ON organization_members(member_role);
CREATE INDEX idx_org_members_status ON organization_members(status) WHERE status = 'active';

-- =============================================================================
-- MODULE 4: AUTHENTICATION & VERIFICATION
-- =============================================================================

-- Bảng Refresh Tokens cho JWT
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Token hash (không lưu token gốc)
    token_hash VARCHAR(255) NOT NULL,

    -- Thông tin thiết bị
    device_id VARCHAR(255),
    device_name VARCHAR(255),
    device_type VARCHAR(50), -- mobile, desktop, tablet
    os VARCHAR(50),
    browser VARCHAR(50),

    -- Thông tin phiên
    ip_address VARCHAR(45),
    user_agent TEXT,

    -- Thời hạn
    expires_at TIMESTAMP NOT NULL,

    -- Trạng thái
    is_revoked BOOLEAN DEFAULT FALSE,
    revoked_at TIMESTAMP,
    revoked_reason VARCHAR(100),

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP
);

COMMENT ON TABLE refresh_tokens IS 'Lưu refresh tokens cho JWT authentication. Hỗ trợ đăng nhập đa thiết bị.';

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens(expires_at) WHERE is_revoked = FALSE;

-- Bảng OAuth Providers
CREATE TABLE user_oauth_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Provider info
    provider VARCHAR(50) NOT NULL, -- google, facebook, github, microsoft
    provider_user_id VARCHAR(255) NOT NULL,
    provider_email VARCHAR(255),

    -- Tokens (encrypted)
    access_token_encrypted TEXT,
    refresh_token_encrypted TEXT,
    token_expires_at TIMESTAMP,

    -- Profile từ provider
    profile_data JSONB,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (provider, provider_user_id)
);

COMMENT ON TABLE user_oauth_providers IS 'Liên kết tài khoản với OAuth providers (Google, Facebook...).';

CREATE INDEX idx_oauth_provider_user ON user_oauth_providers(user_id);
CREATE INDEX idx_oauth_provider_lookup ON user_oauth_providers(provider, provider_user_id);

-- Bảng Verification Codes
CREATE TABLE verification_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,

    -- Email/phone cần xác minh (có thể khác với user hiện tại khi đổi email)
    target VARCHAR(255) NOT NULL,

    -- Code và loại
    code VARCHAR(10) NOT NULL,
    code_type VARCHAR(30) NOT NULL,
    -- email_verification: Xác minh email
    -- phone_verification: Xác minh SĐT
    -- password_reset: Đặt lại mật khẩu
    -- two_factor: 2FA
    -- age_verification: Xác minh tuổi (gửi cho phụ huynh)

    -- Thời hạn và sử dụng
    expires_at TIMESTAMP NOT NULL,
    is_used BOOLEAN DEFAULT FALSE,
    used_at TIMESTAMP,

    -- Chống brute force
    attempts INT DEFAULT 0,
    max_attempts INT DEFAULT 5,

    -- Metadata
    ip_address VARCHAR(45),
    user_agent TEXT,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE verification_codes IS 'Mã xác thực cho email, phone, reset password, 2FA.';

CREATE INDEX idx_verification_codes_user ON verification_codes(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_verification_codes_target ON verification_codes(target, code_type);
CREATE INDEX idx_verification_codes_expires ON verification_codes(expires_at) WHERE is_used = FALSE;

-- Bảng xác minh tuổi
-- Tại sao tách riêng: Xác minh tuổi phức tạp, cần lưu nhiều thông tin hơn
CREATE TABLE age_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Phương thức xác minh
    verification_method VARCHAR(50) NOT NULL,
    -- document: Upload CMND/CCCD/Passport
    -- parent_consent: Phụ huynh xác nhận
    -- school_verification: Trường học xác nhận
    -- self_declaration: Tự khai (chỉ cho > 18)

    -- Thông tin từ document
    document_type VARCHAR(50), -- national_id, passport, birth_certificate
    document_number VARCHAR(100),
    document_url VARCHAR(500), -- Link đến file đã upload (encrypted storage)
    extracted_dob DATE, -- Ngày sinh trích xuất từ document

    -- Thông tin phụ huynh (nếu parent_consent)
    parent_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    parent_email VARCHAR(255),
    parent_phone VARCHAR(20),
    parent_consent_at TIMESTAMP,

    -- Thông tin trường (nếu school_verification)
    organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,
    verified_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Kết quả
    status VARCHAR(20) DEFAULT 'pending',
    -- pending: Chờ xử lý
    -- reviewing: Đang review
    -- approved: Đã duyệt
    -- rejected: Từ chối
    -- expired: Hết hạn

    rejection_reason TEXT,
    verified_at TIMESTAMP,
    expires_at TIMESTAMP, -- Một số loại cần xác minh lại định kỳ

    -- Audit
    reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    review_notes TEXT,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE age_verifications IS 'Xác minh tuổi người dùng. Bắt buộc cho học sinh < 18 tuổi theo quy định bảo vệ trẻ em.';
COMMENT ON COLUMN age_verifications.verification_method IS 'document=CMND, parent_consent=phụ huynh, school_verification=trường';

CREATE INDEX idx_age_verify_user ON age_verifications(user_id);
CREATE INDEX idx_age_verify_status ON age_verifications(status) WHERE status IN ('pending', 'reviewing');
CREATE INDEX idx_age_verify_parent ON age_verifications(parent_user_id) WHERE parent_user_id IS NOT NULL;

-- =============================================================================
-- MODULE 5: PARENT-STUDENT RELATIONSHIPS
-- =============================================================================

-- Bảng quan hệ Phụ huynh - Học sinh
CREATE TABLE parent_student_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    student_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Mối quan hệ
    relationship VARCHAR(50) NOT NULL DEFAULT 'parent',
    -- parent: Bố/Mẹ
    -- guardian: Người giám hộ
    -- grandparent: Ông/Bà
    -- sibling: Anh/Chị/Em (đôi khi cần thiết)

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'pending',
    -- pending: Chờ học sinh/hệ thống xác nhận
    -- active: Đang hoạt động
    -- revoked: Đã thu hồi

    -- Quyền của phụ huynh
    can_view_progress BOOLEAN DEFAULT TRUE,
    can_view_grades BOOLEAN DEFAULT TRUE,
    can_view_attendance BOOLEAN DEFAULT TRUE,
    can_contact_teachers BOOLEAN DEFAULT TRUE,
    can_make_payments BOOLEAN DEFAULT TRUE, -- Thanh toán thay học sinh
    can_manage_account BOOLEAN DEFAULT FALSE, -- Quản lý tài khoản học sinh

    -- Xác nhận
    confirmed_at TIMESTAMP,
    confirmed_by VARCHAR(50), -- student, system, admin

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (parent_user_id, student_user_id)
);

COMMENT ON TABLE parent_student_relations IS 'Quan hệ phụ huynh-học sinh. Cho phép phụ huynh theo dõi tiến độ, thanh toán.';
COMMENT ON COLUMN parent_student_relations.can_manage_account IS 'Quyền quản lý tài khoản: chỉ TRUE khi học sinh < 13 tuổi';

CREATE INDEX idx_parent_student_parent ON parent_student_relations(parent_user_id);
CREATE INDEX idx_parent_student_student ON parent_student_relations(student_user_id);
CREATE INDEX idx_parent_student_status ON parent_student_relations(status) WHERE status = 'active';

-- =============================================================================
-- MODULE 6: CATEGORIES & TAGS
-- =============================================================================

-- Danh mục khóa học (hỗ trợ nested)
CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id UUID REFERENCES categories(id) ON DELETE SET NULL,

    -- Thông tin
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,

    -- Hiển thị
    icon_url VARCHAR(500),
    color VARCHAR(7), -- Hex color
    display_order INT DEFAULT 0,

    -- Cấp độ trong cây (0 = root)
    depth INT DEFAULT 0,

    -- Đường dẫn đầy đủ (materialized path cho query nhanh)
    -- VD: /programming/web/frontend
    path VARCHAR(500),

    -- Trạng thái
    is_active BOOLEAN DEFAULT TRUE,

    -- Số lượng courses (denormalized cho performance)
    course_count INT DEFAULT 0,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE categories IS 'Danh mục khóa học. Hỗ trợ cấu trúc cây với materialized path cho query hiệu quả.';
COMMENT ON COLUMN categories.path IS 'Materialized path: /parent/child/grandchild để query cây nhanh hơn recursive CTE';

CREATE INDEX idx_categories_parent ON categories(parent_id);
CREATE INDEX idx_categories_slug ON categories(slug);
CREATE INDEX idx_categories_path ON categories(path);
CREATE INDEX idx_categories_active ON categories(is_active, display_order) WHERE is_active = TRUE;

-- Tags
CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL,
    slug VARCHAR(50) UNIQUE NOT NULL,

    -- Loại tag
    tag_type VARCHAR(30) DEFAULT 'general',
    -- general: Tag chung
    -- skill: Kỹ năng (Python, JavaScript)
    -- level: Cấp độ (beginner, advanced)
    -- topic: Chủ đề (AI, Web Development)

    -- Số lượng sử dụng (cho trending/popular)
    usage_count INT DEFAULT 0,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE tags IS 'Tags cho khóa học. Phân loại theo type để filter và hiển thị phù hợp.';

CREATE INDEX idx_tags_slug ON tags(slug);
CREATE INDEX idx_tags_type ON tags(tag_type);
CREATE INDEX idx_tags_popular ON tags(usage_count DESC);

-- =============================================================================
-- MODULE 7: COURSES & MODULES
-- =============================================================================

-- Bảng khóa học
CREATE TABLE courses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Ownership
    instructor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,

    -- Phân loại
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,

    -- Thông tin cơ bản
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    subtitle VARCHAR(500),
    description TEXT,

    -- Media
    thumbnail_url VARCHAR(500),
    preview_video_url VARCHAR(500),
    promo_video_url VARCHAR(500),

    -- Cấp độ và ngôn ngữ
    difficulty_level VARCHAR(20) DEFAULT 'beginner',
    -- beginner, intermediate, advanced, all_levels
    primary_language VARCHAR(10) DEFAULT 'vi',
    subtitle_languages VARCHAR(50)[], -- ['en', 'vi', 'zh']

    -- Yêu cầu và mục tiêu
    requirements TEXT[], -- Yêu cầu trước khi học
    objectives TEXT[], -- Mục tiêu đạt được sau khóa học
    target_audience TEXT[], -- Đối tượng phù hợp

    -- Thống kê (denormalized)
    total_modules INT DEFAULT 0,
    total_lessons INT DEFAULT 0,
    total_duration_minutes INT DEFAULT 0,
    total_students INT DEFAULT 0,
    average_rating DECIMAL(2, 1) DEFAULT 0.0,
    total_reviews INT DEFAULT 0,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'draft',
    -- draft: Nháp
    -- pending_review: Chờ duyệt
    -- published: Đã xuất bản
    -- archived: Lưu trữ
    -- rejected: Bị từ chối

    is_published BOOLEAN DEFAULT FALSE,
    published_at TIMESTAMP,
    is_featured BOOLEAN DEFAULT FALSE,

    -- Cài đặt
    is_free BOOLEAN DEFAULT FALSE,
    allow_certificate BOOLEAN DEFAULT TRUE,
    certificate_template_id UUID,
    completion_threshold DECIMAL(5,2) DEFAULT 80.00, -- % cần hoàn thành

    -- SEO
    meta_title VARCHAR(255),
    meta_description VARCHAR(500),
    meta_keywords VARCHAR(255)[],

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

COMMENT ON TABLE courses IS 'Khóa học. Mỗi course thuộc 1 instructor, có thể thuộc 1 organization.';
COMMENT ON COLUMN courses.completion_threshold IS 'Phần trăm hoàn thành tối thiểu để nhận certificate (mặc định 80%)';

-- Indexes cho courses
CREATE INDEX idx_courses_instructor ON courses(instructor_id);
CREATE INDEX idx_courses_organization ON courses(organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_courses_category ON courses(category_id);
CREATE INDEX idx_courses_status ON courses(status);
CREATE INDEX idx_courses_slug ON courses(slug);
CREATE INDEX idx_courses_published ON courses(is_published, published_at DESC) WHERE is_published = TRUE;
CREATE INDEX idx_courses_featured ON courses(is_featured, average_rating DESC) WHERE is_featured = TRUE;
CREATE INDEX idx_courses_rating ON courses(average_rating DESC) WHERE is_published = TRUE;

-- Full-text search cho title và description
CREATE INDEX idx_courses_title_search ON courses USING gin(to_tsvector('simple', title));
CREATE INDEX idx_courses_desc_search ON courses USING gin(to_tsvector('simple', description));

-- Liên kết Course - Tag
CREATE TABLE course_tags (
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (course_id, tag_id)
);

COMMENT ON TABLE course_tags IS 'Liên kết many-to-many giữa courses và tags.';

CREATE INDEX idx_course_tags_tag ON course_tags(tag_id);

-- Bảng định giá khóa học
-- Tách riêng để hỗ trợ nhiều loại giá, nhiều currency
CREATE TABLE course_pricing (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,

    -- Loại giá
    pricing_type VARCHAR(30) NOT NULL DEFAULT 'one_time',
    -- one_time: Mua 1 lần, truy cập vĩnh viễn
    -- subscription: Đăng ký theo tháng/năm
    -- installment: Trả góp theo module

    -- Giá gốc
    currency VARCHAR(3) DEFAULT 'VND',
    original_price DECIMAL(12, 2) NOT NULL,

    -- Giá khuyến mãi
    sale_price DECIMAL(12, 2),
    sale_starts_at TIMESTAMP,
    sale_ends_at TIMESTAMP,

    -- Cho subscription
    billing_period VARCHAR(20), -- monthly, quarterly, yearly
    trial_days INT DEFAULT 0,

    -- Cho installment
    min_installments INT DEFAULT 1,
    max_installments INT,
    installment_fee_percent DECIMAL(5, 2) DEFAULT 0, -- Phí trả góp (%)

    -- Trạng thái
    is_active BOOLEAN DEFAULT TRUE,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE course_pricing IS 'Định giá khóa học. Hỗ trợ one-time, subscription, installment.';
COMMENT ON COLUMN course_pricing.installment_fee_percent IS 'Phí trả góp tính theo %. VD: 5% = 5.00';

CREATE INDEX idx_course_pricing_course ON course_pricing(course_id);
CREATE INDEX idx_course_pricing_active ON course_pricing(course_id, is_active) WHERE is_active = TRUE;

-- Bảng Module (Chương) trong khóa học
CREATE TABLE modules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,

    -- Thông tin
    title VARCHAR(255) NOT NULL,
    description TEXT,

    -- Thứ tự hiển thị
    display_order INT NOT NULL,

    -- Thống kê
    total_lessons INT DEFAULT 0,
    total_duration_minutes INT DEFAULT 0,

    -- Cho bán lẻ module
    is_purchasable_separately BOOLEAN DEFAULT FALSE,
    price DECIMAL(12, 2),
    currency VARCHAR(3) DEFAULT 'VND',

    -- Yêu cầu hoàn thành module trước
    prerequisite_module_id UUID REFERENCES modules(id) ON DELETE SET NULL,

    -- Trạng thái
    is_published BOOLEAN DEFAULT FALSE,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE modules IS 'Module/Chương trong khóa học. Có thể bán riêng lẻ hoặc yêu cầu prerequisite.';
COMMENT ON COLUMN modules.is_purchasable_separately IS 'TRUE = có thể mua riêng module này mà không cần mua cả course';

CREATE INDEX idx_modules_course ON modules(course_id);
CREATE INDEX idx_modules_order ON modules(course_id, display_order);
CREATE INDEX idx_modules_purchasable ON modules(is_purchasable_separately) WHERE is_purchasable_separately = TRUE;

-- =============================================================================
-- MODULE 8: LESSONS & CONTENT
-- =============================================================================

-- Bảng bài học
CREATE TABLE lessons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id UUID NOT NULL REFERENCES modules(id) ON DELETE CASCADE,

    -- Thông tin
    title VARCHAR(255) NOT NULL,
    description TEXT,
    slug VARCHAR(255),

    -- Loại nội dung
    content_type VARCHAR(30) NOT NULL,
    -- video: Bài giảng video
    -- article: Bài viết/đọc
    -- quiz: Bài kiểm tra
    -- assignment: Bài tập
    -- livestream: Livestream
    -- code_exercise: Bài tập code
    -- download: Tài liệu tải về

    -- Thứ tự và thời lượng
    display_order INT NOT NULL,
    duration_minutes INT DEFAULT 0,

    -- Cài đặt
    is_preview BOOLEAN DEFAULT FALSE, -- Cho xem preview không cần mua
    is_mandatory BOOLEAN DEFAULT TRUE, -- Bắt buộc hoàn thành
    is_downloadable BOOLEAN DEFAULT FALSE, -- Cho phép tải

    -- Yêu cầu
    prerequisite_lesson_id UUID REFERENCES lessons(id) ON DELETE SET NULL,

    -- Trạng thái
    is_published BOOLEAN DEFAULT FALSE,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE lessons IS 'Bài học trong module. Hỗ trợ nhiều loại content: video, article, quiz, code_exercise.';
COMMENT ON COLUMN lessons.is_preview IS 'TRUE = học viên có thể xem preview mà không cần đăng ký/mua';

CREATE INDEX idx_lessons_module ON lessons(module_id);
CREATE INDEX idx_lessons_order ON lessons(module_id, display_order);
CREATE INDEX idx_lessons_type ON lessons(content_type);
CREATE INDEX idx_lessons_preview ON lessons(is_preview) WHERE is_preview = TRUE;

-- Nội dung Video của bài học
CREATE TABLE lesson_videos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lesson_id UUID UNIQUE NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,

    -- URLs
    video_url VARCHAR(500) NOT NULL, -- URL gốc trên MinIO
    video_hls_url VARCHAR(500), -- HLS streaming URL
    video_dash_url VARCHAR(500), -- DASH streaming URL
    thumbnail_url VARCHAR(500),

    -- Thông tin video
    duration_seconds INT NOT NULL,
    resolution VARCHAR(20), -- 1080p, 720p, 480p
    aspect_ratio VARCHAR(10) DEFAULT '16:9',
    file_size_bytes BIGINT,
    format VARCHAR(20), -- mp4, webm

    -- Xử lý
    processing_status VARCHAR(20) DEFAULT 'pending',
    -- pending, processing, completed, failed
    processing_error TEXT,

    -- Transcription (AI-generated)
    transcription TEXT,
    transcription_vtt_url VARCHAR(500), -- WebVTT file URL
    transcription_status VARCHAR(20) DEFAULT 'pending',
    transcription_language VARCHAR(10) DEFAULT 'vi',

    -- Chapters/Timestamps
    chapters JSONB, -- [{time: 0, title: "Intro"}, {time: 120, title: "Part 1"}]

    -- Analytics
    view_count INT DEFAULT 0,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE lesson_videos IS 'Nội dung video của bài học. Hỗ trợ HLS/DASH streaming, auto transcription.';
COMMENT ON COLUMN lesson_videos.chapters IS 'Chapters dạng JSON: [{time: 0, title: "Giới thiệu"}, ...]';

CREATE INDEX idx_lesson_videos_status ON lesson_videos(processing_status) WHERE processing_status != 'completed';

-- Nội dung Article của bài học
CREATE TABLE lesson_articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lesson_id UUID UNIQUE NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,

    -- Nội dung
    content TEXT NOT NULL, -- Markdown
    content_html TEXT, -- Pre-rendered HTML

    -- Thông tin
    reading_time_minutes INT DEFAULT 5,
    word_count INT DEFAULT 0,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE lesson_articles IS 'Nội dung bài viết/đọc của bài học. Lưu cả Markdown và HTML đã render.';

-- Tài liệu đính kèm
CREATE TABLE lesson_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lesson_id UUID NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,

    -- Thông tin file
    file_name VARCHAR(255) NOT NULL,
    file_url VARCHAR(500) NOT NULL,
    file_type VARCHAR(100), -- MIME type
    file_extension VARCHAR(20),
    file_size_bytes BIGINT,

    -- Mô tả
    description TEXT,

    -- Thứ tự hiển thị
    display_order INT DEFAULT 0,

    -- Analytics
    download_count INT DEFAULT 0,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE lesson_attachments IS 'Tài liệu đính kèm bài học: PDF, slides, source code, etc.';

CREATE INDEX idx_lesson_attachments_lesson ON lesson_attachments(lesson_id);

-- =============================================================================
-- MODULE 9: LIVESTREAM
-- =============================================================================

CREATE TABLE livestreams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Ownership
    instructor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    course_id UUID REFERENCES courses(id) ON DELETE SET NULL,
    lesson_id UUID REFERENCES lessons(id) ON DELETE SET NULL,

    -- Thông tin
    title VARCHAR(255) NOT NULL,
    description TEXT,
    thumbnail_url VARCHAR(500),

    -- Streaming
    stream_key VARCHAR(100) UNIQUE,
    rtmp_url VARCHAR(500), -- RTMP ingest URL
    playback_url VARCHAR(500), -- HLS playback URL

    -- Lịch trình
    scheduled_at TIMESTAMP,
    started_at TIMESTAMP,
    ended_at TIMESTAMP,
    actual_duration_minutes INT,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'scheduled',
    -- scheduled: Đã lên lịch
    -- live: Đang phát
    -- ended: Đã kết thúc
    -- cancelled: Đã hủy

    -- Cài đặt
    is_recorded BOOLEAN DEFAULT TRUE,
    recording_url VARCHAR(500),
    max_viewers_allowed INT, -- NULL = unlimited
    is_chat_enabled BOOLEAN DEFAULT TRUE,
    is_qa_enabled BOOLEAN DEFAULT TRUE,

    -- Thống kê
    peak_viewers INT DEFAULT 0,
    total_viewers INT DEFAULT 0,
    total_messages INT DEFAULT 0,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE livestreams IS 'Livestream sessions. Hỗ trợ scheduled, live streaming với chat và Q&A.';

CREATE INDEX idx_livestreams_instructor ON livestreams(instructor_id);
CREATE INDEX idx_livestreams_course ON livestreams(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_livestreams_status ON livestreams(status);
CREATE INDEX idx_livestreams_scheduled ON livestreams(scheduled_at) WHERE status = 'scheduled';

-- Người xem livestream
CREATE TABLE livestream_viewers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    livestream_id UUID NOT NULL REFERENCES livestreams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Thời gian xem
    joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    left_at TIMESTAMP,
    total_watch_seconds INT DEFAULT 0,

    -- Thiết bị
    device_type VARCHAR(30),
    ip_address VARCHAR(45),

    UNIQUE (livestream_id, user_id, joined_at)
);

COMMENT ON TABLE livestream_viewers IS 'Tracking người xem livestream để tính toán peak/total viewers.';

CREATE INDEX idx_livestream_viewers_stream ON livestream_viewers(livestream_id);
CREATE INDEX idx_livestream_viewers_user ON livestream_viewers(user_id);

-- Chat trong livestream
CREATE TABLE livestream_chats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    livestream_id UUID NOT NULL REFERENCES livestreams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Nội dung
    message TEXT NOT NULL,
    message_type VARCHAR(20) DEFAULT 'text',
    -- text: Tin nhắn thường
    -- question: Câu hỏi Q&A
    -- announcement: Thông báo từ instructor
    -- highlight: Được highlight

    -- Trạng thái
    is_pinned BOOLEAN DEFAULT FALSE,
    is_answered BOOLEAN DEFAULT FALSE, -- Cho questions
    is_deleted BOOLEAN DEFAULT FALSE,
    deleted_at TIMESTAMP,
    deleted_by UUID REFERENCES users(id),

    -- Reply (Adjacency List)
    parent_id UUID REFERENCES livestream_chats(id) ON DELETE CASCADE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE livestream_chats IS 'Chat messages trong livestream. Sử dụng Adjacency List cho replies.';

CREATE INDEX idx_livestream_chats_stream ON livestream_chats(livestream_id);
CREATE INDEX idx_livestream_chats_user ON livestream_chats(user_id);
CREATE INDEX idx_livestream_chats_parent ON livestream_chats(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_livestream_chats_pinned ON livestream_chats(livestream_id, is_pinned) WHERE is_pinned = TRUE;

-- =============================================================================
-- MODULE 10: QUIZZES & ASSESSMENTS
-- =============================================================================

-- Bảng Quiz
CREATE TABLE quizzes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Liên kết (có thể thuộc lesson hoặc course)
    lesson_id UUID UNIQUE REFERENCES lessons(id) ON DELETE CASCADE,
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,

    -- Thông tin
    title VARCHAR(255) NOT NULL,
    description TEXT,
    instructions TEXT, -- Hướng dẫn làm bài

    -- Cài đặt thời gian
    time_limit_minutes INT, -- NULL = không giới hạn
    available_from TIMESTAMP, -- Thời gian mở
    available_until TIMESTAMP, -- Thời gian đóng

    -- Cài đặt điểm
    pass_percentage DECIMAL(5, 2) DEFAULT 70.00,
    max_score DECIMAL(8, 2),
    scoring_method VARCHAR(20) DEFAULT 'highest',
    -- highest: Lấy điểm cao nhất
    -- latest: Lấy điểm lần cuối
    -- average: Trung bình các lần

    -- Cài đặt lần làm
    max_attempts INT DEFAULT 3, -- NULL = unlimited
    attempt_cooldown_minutes INT, -- Thời gian chờ giữa các lần

    -- Cài đặt hiển thị
    shuffle_questions BOOLEAN DEFAULT TRUE,
    shuffle_answers BOOLEAN DEFAULT TRUE,
    show_correct_answers VARCHAR(20) DEFAULT 'after_submit',
    -- never, after_submit, after_deadline, always
    show_score_immediately BOOLEAN DEFAULT TRUE,

    -- Loại quiz
    quiz_type VARCHAR(30) DEFAULT 'practice',
    -- practice: Luyện tập (không tính điểm chính)
    -- graded: Tính điểm
    -- survey: Khảo sát
    -- placement: Đánh giá đầu vào

    -- AI
    is_ai_generated BOOLEAN DEFAULT FALSE,
    ai_generation_params JSONB, -- Params đã dùng để generate

    -- Thống kê
    total_questions INT DEFAULT 0,
    total_attempts INT DEFAULT 0,
    average_score DECIMAL(5, 2),

    -- Trạng thái
    is_published BOOLEAN DEFAULT FALSE,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE quizzes IS 'Quiz/Bài kiểm tra. Có thể thuộc lesson hoặc course level. Hỗ trợ AI-generated.';
COMMENT ON COLUMN quizzes.scoring_method IS 'highest=điểm cao nhất, latest=lần cuối, average=trung bình';
COMMENT ON COLUMN quizzes.show_correct_answers IS 'never, after_submit, after_deadline, always';

CREATE INDEX idx_quizzes_lesson ON quizzes(lesson_id) WHERE lesson_id IS NOT NULL;
CREATE INDEX idx_quizzes_course ON quizzes(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_quizzes_type ON quizzes(quiz_type);
CREATE INDEX idx_quizzes_published ON quizzes(is_published) WHERE is_published = TRUE;

-- Question Bank (Ngân hàng câu hỏi)
-- Tại sao tách riêng: Cho phép reuse câu hỏi giữa các quiz
CREATE TABLE question_bank (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Ownership
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

    -- Thông tin
    question_text TEXT NOT NULL,
    question_type VARCHAR(30) NOT NULL,
    -- single_choice: Chọn 1
    -- multiple_choice: Chọn nhiều
    -- true_false: Đúng/Sai
    -- fill_blank: Điền vào chỗ trống
    -- essay: Tự luận
    -- matching: Nối cặp
    -- ordering: Sắp xếp thứ tự
    -- code: Câu hỏi code

    -- Media
    image_url VARCHAR(500),
    audio_url VARCHAR(500),
    video_url VARCHAR(500),

    -- Giải thích đáp án
    explanation TEXT,
    explanation_media_url VARCHAR(500),

    -- Điểm và độ khó
    default_points DECIMAL(5, 2) DEFAULT 1.00,
    difficulty_level VARCHAR(20) DEFAULT 'medium',
    -- easy, medium, hard, expert

    -- Phân loại
    topic VARCHAR(100), -- Chủ đề
    subtopic VARCHAR(100),
    tags VARCHAR(50)[],

    -- Bloom's Taxonomy
    cognitive_level VARCHAR(30),
    -- remember, understand, apply, analyze, evaluate, create

    -- Thống kê
    times_used INT DEFAULT 0,
    correct_rate DECIMAL(5, 2), -- % trả lời đúng

    -- AI
    is_ai_generated BOOLEAN DEFAULT FALSE,

    -- Trạng thái
    is_active BOOLEAN DEFAULT TRUE,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE question_bank IS 'Ngân hàng câu hỏi. Câu hỏi có thể reuse trong nhiều quizzes.';
COMMENT ON COLUMN question_bank.cognitive_level IS 'Bloom Taxonomy: remember, understand, apply, analyze, evaluate, create';

CREATE INDEX idx_question_bank_course ON question_bank(course_id);
CREATE INDEX idx_question_bank_creator ON question_bank(created_by);
CREATE INDEX idx_question_bank_type ON question_bank(question_type);
CREATE INDEX idx_question_bank_difficulty ON question_bank(difficulty_level);
CREATE INDEX idx_question_bank_topic ON question_bank(topic) WHERE topic IS NOT NULL;
CREATE INDEX idx_question_bank_tags ON question_bank USING gin(tags);

-- Đáp án cho câu hỏi trong Question Bank
CREATE TABLE question_bank_answers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id UUID NOT NULL REFERENCES question_bank(id) ON DELETE CASCADE,

    -- Nội dung
    answer_text TEXT NOT NULL,
    answer_image_url VARCHAR(500),

    -- Đúng/sai
    is_correct BOOLEAN DEFAULT FALSE,

    -- Cho matching: ID của item để nối
    match_target_id UUID,

    -- Cho ordering: Thứ tự đúng
    correct_order INT,

    -- Điểm một phần (cho partial credit)
    partial_credit_percent DECIMAL(5, 2),

    -- Thứ tự hiển thị
    display_order INT NOT NULL DEFAULT 0,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE question_bank_answers IS 'Đáp án của câu hỏi trong ngân hàng câu hỏi.';

CREATE INDEX idx_qb_answers_question ON question_bank_answers(question_id);

-- Câu hỏi trong Quiz (liên kết Quiz với Question Bank)
CREATE TABLE quiz_questions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quiz_id UUID NOT NULL REFERENCES quizzes(id) ON DELETE CASCADE,

    -- Có thể link từ question bank hoặc tạo inline
    question_bank_id UUID REFERENCES question_bank(id) ON DELETE SET NULL,

    -- Nếu không dùng question bank, lưu inline
    question_text TEXT,
    question_type VARCHAR(30),
    image_url VARCHAR(500),
    explanation TEXT,

    -- Override điểm từ question bank
    points DECIMAL(5, 2) DEFAULT 1.00,

    -- Thứ tự
    display_order INT NOT NULL,

    -- Bắt buộc trả lời
    is_required BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE quiz_questions IS 'Câu hỏi trong quiz. Có thể link từ question_bank hoặc inline.';

CREATE INDEX idx_quiz_questions_quiz ON quiz_questions(quiz_id);
CREATE INDEX idx_quiz_questions_bank ON quiz_questions(question_bank_id) WHERE question_bank_id IS NOT NULL;
CREATE INDEX idx_quiz_questions_order ON quiz_questions(quiz_id, display_order);

-- Đáp án cho câu hỏi inline trong quiz
CREATE TABLE quiz_question_answers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quiz_question_id UUID NOT NULL REFERENCES quiz_questions(id) ON DELETE CASCADE,

    answer_text TEXT NOT NULL,
    answer_image_url VARCHAR(500),
    is_correct BOOLEAN DEFAULT FALSE,
    match_target_id UUID,
    correct_order INT,
    partial_credit_percent DECIMAL(5, 2),
    display_order INT NOT NULL DEFAULT 0,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE quiz_question_answers IS 'Đáp án cho câu hỏi inline (không từ question bank).';

CREATE INDEX idx_quiz_q_answers_question ON quiz_question_answers(quiz_question_id);

-- Lần làm quiz của user
CREATE TABLE quiz_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    quiz_id UUID NOT NULL REFERENCES quizzes(id) ON DELETE CASCADE,

    -- Số lần thử
    attempt_number INT NOT NULL DEFAULT 1,

    -- Điểm
    score DECIMAL(8, 2),
    max_score DECIMAL(8, 2),
    percentage DECIMAL(5, 2),
    is_passed BOOLEAN,

    -- Thời gian
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    submitted_at TIMESTAMP,
    time_spent_seconds INT,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'in_progress',
    -- in_progress, submitted, graded, abandoned

    -- Thông tin thiết bị (chống gian lận)
    ip_address VARCHAR(45),
    user_agent TEXT,
    browser_fingerprint VARCHAR(255),

    -- Cờ nghi ngờ gian lận
    suspicious_activity JSONB,
    -- VD: {tab_switches: 5, copy_paste_detected: true}

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE quiz_attempts IS 'Lần làm quiz của user. Lưu điểm, thời gian, trạng thái.';
COMMENT ON COLUMN quiz_attempts.suspicious_activity IS 'Ghi nhận hoạt động nghi ngờ gian lận: tab switches, copy-paste...';

CREATE INDEX idx_quiz_attempts_user ON quiz_attempts(user_id);
CREATE INDEX idx_quiz_attempts_quiz ON quiz_attempts(quiz_id);
CREATE INDEX idx_quiz_attempts_user_quiz ON quiz_attempts(user_id, quiz_id);
CREATE INDEX idx_quiz_attempts_status ON quiz_attempts(status) WHERE status = 'in_progress';

-- Câu trả lời trong mỗi attempt
CREATE TABLE quiz_attempt_answers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id UUID NOT NULL REFERENCES quiz_attempts(id) ON DELETE CASCADE,
    quiz_question_id UUID NOT NULL REFERENCES quiz_questions(id) ON DELETE CASCADE,

    -- Câu trả lời
    selected_answer_ids UUID[], -- Cho choice questions
    text_answer TEXT, -- Cho fill_blank, essay
    code_answer TEXT, -- Cho code questions
    ordering_answer INT[], -- Cho ordering questions
    matching_answer JSONB, -- Cho matching: {source_id: target_id}

    -- Kết quả chấm
    is_correct BOOLEAN,
    points_earned DECIMAL(5, 2) DEFAULT 0,
    auto_graded BOOLEAN DEFAULT TRUE,
    manual_feedback TEXT, -- Feedback từ instructor

    -- Thời gian trả lời
    answered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    time_spent_seconds INT,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE quiz_attempt_answers IS 'Câu trả lời của user trong mỗi lần làm quiz.';

CREATE INDEX idx_quiz_attempt_answers_attempt ON quiz_attempt_answers(attempt_id);
CREATE INDEX idx_quiz_attempt_answers_question ON quiz_attempt_answers(quiz_question_id);

-- =============================================================================
-- MODULE 11: CODE SANDBOX & PROGRAMMING EXERCISES
-- =============================================================================

-- Môi trường chạy code
CREATE TABLE code_environments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Thông tin
    name VARCHAR(100) NOT NULL, -- Python 3.11, Node.js 20, etc.
    slug VARCHAR(50) UNIQUE NOT NULL,
    description TEXT,

    -- Runtime
    language VARCHAR(50) NOT NULL, -- python, javascript, java, go, etc.
    version VARCHAR(20) NOT NULL,
    runtime_image VARCHAR(255), -- Docker image

    -- Cài đặt
    default_timeout_seconds INT DEFAULT 30,
    max_timeout_seconds INT DEFAULT 60,
    memory_limit_mb INT DEFAULT 256,
    cpu_limit_percent INT DEFAULT 50,

    -- File templates
    boilerplate_code TEXT, -- Code mẫu ban đầu
    test_runner_code TEXT, -- Code chạy test

    -- Libraries/packages có sẵn
    available_packages JSONB,
    -- VD: {"numpy": "1.24", "pandas": "2.0"}

    -- Trạng thái
    is_active BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE code_environments IS 'Môi trường chạy code (sandbox). Mỗi ngôn ngữ có thể có nhiều versions.';
COMMENT ON COLUMN code_environments.runtime_image IS 'Docker image dùng để chạy code trong sandbox';

CREATE INDEX idx_code_env_language ON code_environments(language);
CREATE INDEX idx_code_env_active ON code_environments(is_active) WHERE is_active = TRUE;

-- Bài tập code
CREATE TABLE code_exercises (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Liên kết
    lesson_id UUID UNIQUE REFERENCES lessons(id) ON DELETE CASCADE,
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,

    -- Thông tin
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL, -- Markdown
    instructions TEXT, -- Hướng dẫn chi tiết

    -- Môi trường
    environment_id UUID NOT NULL REFERENCES code_environments(id) ON DELETE RESTRICT,

    -- Code templates
    starter_code TEXT, -- Code khởi đầu cho user
    solution_code TEXT, -- Đáp án (ẩn với user)

    -- Cài đặt
    difficulty_level VARCHAR(20) DEFAULT 'medium',
    estimated_minutes INT DEFAULT 30,
    max_submissions INT, -- NULL = unlimited

    -- Test cases
    test_cases_visible JSONB, -- Test cases user có thể thấy
    test_cases_hidden JSONB, -- Test cases ẩn để chấm điểm
    -- Format: [{input: "...", expected_output: "...", points: 10, description: "..."}]

    -- Tiêu chí chấm điểm
    grading_criteria JSONB,
    -- VD: {correctness: 70, efficiency: 20, style: 10}

    -- Hints
    hints JSONB, -- [{text: "...", penalty_percent: 5}]

    -- Thống kê
    total_submissions INT DEFAULT 0,
    success_rate DECIMAL(5, 2),

    -- Trạng thái
    is_published BOOLEAN DEFAULT FALSE,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE code_exercises IS 'Bài tập lập trình với sandbox. Hỗ trợ test cases, auto-grading.';
COMMENT ON COLUMN code_exercises.test_cases_hidden IS 'Test cases ẩn: user không biết nội dung, chỉ biết pass/fail';

CREATE INDEX idx_code_exercises_lesson ON code_exercises(lesson_id) WHERE lesson_id IS NOT NULL;
CREATE INDEX idx_code_exercises_course ON code_exercises(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_code_exercises_env ON code_exercises(environment_id);
CREATE INDEX idx_code_exercises_difficulty ON code_exercises(difficulty_level);

-- Lần submit code
CREATE TABLE code_submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    exercise_id UUID NOT NULL REFERENCES code_exercises(id) ON DELETE CASCADE,

    -- Code
    submitted_code TEXT NOT NULL,
    language VARCHAR(50) NOT NULL,

    -- Kết quả chạy
    execution_status VARCHAR(30) DEFAULT 'pending',
    -- pending, running, completed, error, timeout, memory_exceeded
    execution_time_ms INT,
    memory_used_kb INT,

    -- Output
    stdout TEXT,
    stderr TEXT,
    compile_error TEXT,

    -- Test results
    test_results JSONB,
    -- [{test_id: 1, passed: true, output: "...", expected: "...", time_ms: 50}]
    tests_passed INT DEFAULT 0,
    tests_total INT DEFAULT 0,

    -- Điểm
    score DECIMAL(5, 2),
    max_score DECIMAL(5, 2),

    -- Hints đã sử dụng
    hints_used INT DEFAULT 0,
    hint_penalty_percent DECIMAL(5, 2) DEFAULT 0,

    -- Metadata
    submission_number INT NOT NULL DEFAULT 1,
    ip_address VARCHAR(45),

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE code_submissions IS 'Lần submit code của user. Lưu code, kết quả chạy, test results.';

CREATE INDEX idx_code_submissions_user ON code_submissions(user_id);
CREATE INDEX idx_code_submissions_exercise ON code_submissions(exercise_id);
CREATE INDEX idx_code_submissions_user_exercise ON code_submissions(user_id, exercise_id);
CREATE INDEX idx_code_submissions_status ON code_submissions(execution_status) WHERE execution_status IN ('pending', 'running');

-- =============================================================================
-- MODULE 12: ENROLLMENTS & PROGRESS
-- =============================================================================

-- Đăng ký khóa học
CREATE TABLE enrollments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,

    -- Loại đăng ký
    enrollment_type VARCHAR(30) DEFAULT 'full_course',
    -- full_course: Mua cả khóa
    -- module_only: Chỉ mua một số modules
    -- subscription: Đăng ký gói
    -- free_trial: Dùng thử
    -- gift: Được tặng
    -- organization: Qua organization

    -- Source
    enrolled_via VARCHAR(50), -- web, mobile, api, admin, organization
    organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,

    -- Thời hạn
    enrolled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP, -- NULL = lifetime
    trial_ends_at TIMESTAMP,

    -- Tiến độ
    progress_percentage DECIMAL(5, 2) DEFAULT 0,
    completed_lessons INT DEFAULT 0,
    total_lessons INT DEFAULT 0,
    completed_at TIMESTAMP,

    -- Thời gian học
    total_time_spent_minutes INT DEFAULT 0,
    last_accessed_at TIMESTAMP,
    last_lesson_id UUID REFERENCES lessons(id) ON DELETE SET NULL,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'active',
    -- active: Đang học
    -- completed: Hoàn thành
    -- expired: Hết hạn
    -- suspended: Tạm dừng
    -- refunded: Đã hoàn tiền

    -- Certificate
    certificate_issued BOOLEAN DEFAULT FALSE,
    certificate_id UUID,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, course_id)
);

COMMENT ON TABLE enrollments IS 'Đăng ký học khóa học. Hỗ trợ full course, module-only, subscription.';
COMMENT ON COLUMN enrollments.enrollment_type IS 'full_course, module_only, subscription, free_trial, gift, organization';

CREATE INDEX idx_enrollments_user ON enrollments(user_id);
CREATE INDEX idx_enrollments_course ON enrollments(course_id);
CREATE INDEX idx_enrollments_status ON enrollments(status);
CREATE INDEX idx_enrollments_org ON enrollments(organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_enrollments_expires ON enrollments(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_enrollments_active ON enrollments(user_id, status) WHERE status = 'active';

-- Đăng ký module riêng lẻ (cho trường hợp mua từng module)
CREATE TABLE module_enrollments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module_id UUID NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    enrollment_id UUID REFERENCES enrollments(id) ON DELETE SET NULL,

    -- Thời hạn
    enrolled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,

    -- Tiến độ
    progress_percentage DECIMAL(5, 2) DEFAULT 0,
    completed_at TIMESTAMP,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'active',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, module_id)
);

COMMENT ON TABLE module_enrollments IS 'Đăng ký module riêng lẻ. Cho phép mua từng module thay vì cả course.';

CREATE INDEX idx_module_enrollments_user ON module_enrollments(user_id);
CREATE INDEX idx_module_enrollments_module ON module_enrollments(module_id);
CREATE INDEX idx_module_enrollments_enrollment ON module_enrollments(enrollment_id) WHERE enrollment_id IS NOT NULL;

-- Tiến độ học từng bài
CREATE TABLE lesson_progress (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id UUID NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    enrollment_id UUID NOT NULL REFERENCES enrollments(id) ON DELETE CASCADE,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'not_started',
    -- not_started, in_progress, completed

    -- Tiến độ chi tiết
    progress_percentage DECIMAL(5, 2) DEFAULT 0,

    -- Cho video
    video_watched_seconds INT DEFAULT 0,
    video_total_seconds INT,
    video_last_position INT DEFAULT 0, -- Vị trí dừng lại

    -- Cho article
    scroll_percentage DECIMAL(5, 2) DEFAULT 0,

    -- Cho quiz/code
    best_score DECIMAL(5, 2),
    attempts_count INT DEFAULT 0,

    -- Thời gian
    first_accessed_at TIMESTAMP,
    last_accessed_at TIMESTAMP,
    completed_at TIMESTAMP,
    time_spent_seconds INT DEFAULT 0,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, lesson_id)
);

COMMENT ON TABLE lesson_progress IS 'Tiến độ học từng bài. Track video position, scroll, quiz score.';
COMMENT ON COLUMN lesson_progress.video_last_position IS 'Vị trí dừng lại để tiếp tục học';

CREATE INDEX idx_lesson_progress_user ON lesson_progress(user_id);
CREATE INDEX idx_lesson_progress_lesson ON lesson_progress(lesson_id);
CREATE INDEX idx_lesson_progress_enrollment ON lesson_progress(enrollment_id);
CREATE INDEX idx_lesson_progress_status ON lesson_progress(status);
CREATE INDEX idx_lesson_progress_user_lesson ON lesson_progress(user_id, lesson_id);

-- Ghi chú của học viên
CREATE TABLE user_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id UUID NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,

    -- Nội dung
    content TEXT NOT NULL, -- Markdown
    content_html TEXT,

    -- Vị trí trong video (nếu có)
    video_timestamp_seconds INT,

    -- Highlight text từ article
    highlighted_text TEXT,
    highlight_start_offset INT,
    highlight_end_offset INT,

    -- Bookmark
    is_bookmarked BOOLEAN DEFAULT FALSE,

    -- Tags
    tags VARCHAR(50)[],

    -- Trạng thái
    is_private BOOLEAN DEFAULT TRUE, -- Private vs shared

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE user_notes IS 'Ghi chú của học viên trong bài học. Hỗ trợ timestamp, highlight.';

CREATE INDEX idx_user_notes_user ON user_notes(user_id);
CREATE INDEX idx_user_notes_lesson ON user_notes(lesson_id);
CREATE INDEX idx_user_notes_bookmarked ON user_notes(user_id, is_bookmarked) WHERE is_bookmarked = TRUE;

-- =============================================================================
-- MODULE 13: STUDY SESSIONS (FOCUS TIME TRACKING)
-- =============================================================================

-- Phiên học tập
CREATE TABLE study_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Context
    course_id UUID REFERENCES courses(id) ON DELETE SET NULL,
    module_id UUID REFERENCES modules(id) ON DELETE SET NULL,
    lesson_id UUID REFERENCES lessons(id) ON DELETE SET NULL,

    -- Thời gian
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMP,
    duration_seconds INT,

    -- Loại phiên
    session_type VARCHAR(30) DEFAULT 'learning',
    -- learning: Học bài
    -- quiz: Làm quiz
    -- coding: Làm bài code
    -- review: Ôn tập
    -- livestream: Xem livestream

    -- Focus tracking
    is_focused BOOLEAN DEFAULT TRUE,
    focus_score DECIMAL(5, 2), -- 0-100, dựa trên activity
    idle_seconds INT DEFAULT 0, -- Thời gian không hoạt động
    tab_switches INT DEFAULT 0, -- Số lần chuyển tab

    -- Hoạt động
    activities JSONB,
    -- [{time: "...", action: "video_play"}, {time: "...", action: "note_created"}]

    -- Device
    device_type VARCHAR(30),
    platform VARCHAR(30), -- web, ios, android
    browser VARCHAR(50),
    ip_address VARCHAR(45),

    -- Goals
    goal_minutes INT, -- Mục tiêu học trong session
    goal_achieved BOOLEAN,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE study_sessions IS 'Phiên học tập. Track focus time, activities, để analytics và gamification.';
COMMENT ON COLUMN study_sessions.focus_score IS 'Điểm tập trung 0-100, dựa trên activity patterns';

CREATE INDEX idx_study_sessions_user ON study_sessions(user_id);
CREATE INDEX idx_study_sessions_course ON study_sessions(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_study_sessions_started ON study_sessions(started_at);
CREATE INDEX idx_study_sessions_user_date ON study_sessions(user_id, started_at);

-- Điểm danh / Attendance (cho classes trong organization)
CREATE TABLE attendance_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Liên kết
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,

    -- Có thể điểm danh cho nhiều loại
    attendance_type VARCHAR(30) NOT NULL,
    -- class: Lớp học
    -- livestream: Livestream
    -- exam: Kỳ thi
    -- event: Sự kiện

    -- Reference
    reference_type VARCHAR(30), -- livestream, scheduled_class, exam
    reference_id UUID,

    -- Thời gian
    scheduled_at TIMESTAMP, -- Thời gian theo lịch
    checked_in_at TIMESTAMP, -- Thời gian thực tế check-in
    checked_out_at TIMESTAMP,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'present',
    -- present: Có mặt
    -- absent: Vắng
    -- late: Đi trễ
    -- excused: Có phép
    -- early_leave: Về sớm

    -- Ghi chú
    notes TEXT,
    excuse_reason TEXT,
    approved_by UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE attendance_records IS 'Điểm danh cho lớp học, livestream, exam trong organization.';

CREATE INDEX idx_attendance_user ON attendance_records(user_id);
CREATE INDEX idx_attendance_org ON attendance_records(organization_id);
CREATE INDEX idx_attendance_scheduled ON attendance_records(scheduled_at);
CREATE INDEX idx_attendance_status ON attendance_records(status);

-- Lịch học / Nhắc nhở
CREATE TABLE study_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Loại lịch
    schedule_type VARCHAR(30) NOT NULL,
    -- course_reminder: Nhắc học course
    -- lesson_reminder: Nhắc học lesson
    -- quiz_deadline: Deadline quiz
    -- livestream: Lịch livestream
    -- custom: Tự đặt

    -- Reference
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,
    lesson_id UUID REFERENCES lessons(id) ON DELETE CASCADE,
    livestream_id UUID REFERENCES livestreams(id) ON DELETE CASCADE,

    -- Thời gian
    scheduled_at TIMESTAMP NOT NULL,
    duration_minutes INT,

    -- Nhắc nhở
    reminder_minutes_before INT[], -- [60, 30, 15, 5]
    reminder_sent_at TIMESTAMP[],

    -- Lặp lại
    is_recurring BOOLEAN DEFAULT FALSE,
    recurrence_rule VARCHAR(255), -- RRULE format
    recurrence_end_date DATE,

    -- Thông tin
    title VARCHAR(255),
    description TEXT,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'scheduled',
    -- scheduled, completed, cancelled, missed

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE study_schedules IS 'Lịch học và nhắc nhở. Hỗ trợ recurring với RRULE.';
COMMENT ON COLUMN study_schedules.recurrence_rule IS 'RRULE format: FREQ=WEEKLY;BYDAY=MO,WE,FR';

CREATE INDEX idx_study_schedules_user ON study_schedules(user_id);
CREATE INDEX idx_study_schedules_scheduled ON study_schedules(scheduled_at);
CREATE INDEX idx_study_schedules_upcoming ON study_schedules(user_id, scheduled_at) WHERE status = 'scheduled';

-- =============================================================================
-- MODULE 14: PAYMENTS, ORDERS & VOUCHERS
-- =============================================================================

-- Đơn hàng
CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

    -- Mã đơn hàng
    order_number VARCHAR(50) UNIQUE NOT NULL,

    -- Tổng tiền
    subtotal DECIMAL(12, 2) NOT NULL, -- Trước giảm giá
    discount_amount DECIMAL(12, 2) DEFAULT 0,
    voucher_discount DECIMAL(12, 2) DEFAULT 0,
    tax_amount DECIMAL(12, 2) DEFAULT 0,
    total_amount DECIMAL(12, 2) NOT NULL, -- Sau giảm giá + thuế
    currency VARCHAR(3) DEFAULT 'VND',

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'pending',
    -- pending: Chờ thanh toán
    -- processing: Đang xử lý
    -- completed: Hoàn thành
    -- failed: Thất bại
    -- refunded: Đã hoàn tiền
    -- partially_refunded: Hoàn một phần
    -- cancelled: Đã hủy

    -- Thanh toán
    payment_method VARCHAR(50),
    -- credit_card, bank_transfer, momo, zalopay, vnpay, paypal

    payment_status VARCHAR(20) DEFAULT 'pending',
    -- pending, processing, completed, failed, refunded

    paid_at TIMESTAMP,

    -- Voucher đã dùng
    voucher_id UUID,
    voucher_code VARCHAR(50),

    -- Cho installment
    is_installment BOOLEAN DEFAULT FALSE,
    installment_plan_id UUID,
    total_installments INT,
    paid_installments INT DEFAULT 0,

    -- Metadata
    ip_address VARCHAR(45),
    user_agent TEXT,
    notes TEXT,
    admin_notes TEXT,

    -- Billing info
    billing_name VARCHAR(255),
    billing_email VARCHAR(255),
    billing_phone VARCHAR(20),
    billing_address TEXT,

    -- Invoice
    invoice_number VARCHAR(50),
    invoice_url VARCHAR(500),

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE orders IS 'Đơn hàng. Hỗ trợ thanh toán full, trả góp, nhiều payment methods.';
COMMENT ON COLUMN orders.is_installment IS 'TRUE nếu đơn hàng này thanh toán trả góp';

CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_payment_status ON orders(payment_status);
CREATE INDEX idx_orders_created ON orders(created_at);
CREATE INDEX idx_orders_number ON orders(order_number);

-- Chi tiết đơn hàng
CREATE TABLE order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,

    -- Sản phẩm (course hoặc module)
    item_type VARCHAR(30) NOT NULL,
    -- course: Mua cả khóa
    -- module: Mua riêng module
    -- subscription: Gói subscription

    course_id UUID REFERENCES courses(id) ON DELETE RESTRICT,
    module_id UUID REFERENCES modules(id) ON DELETE RESTRICT,

    -- Giá
    original_price DECIMAL(12, 2) NOT NULL,
    discount_amount DECIMAL(12, 2) DEFAULT 0,
    final_price DECIMAL(12, 2) NOT NULL,

    -- Cho subscription
    subscription_months INT,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'pending',
    -- pending, activated, refunded

    activated_at TIMESTAMP,
    refunded_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE order_items IS 'Chi tiết sản phẩm trong đơn hàng. Có thể là course, module, hoặc subscription.';

CREATE INDEX idx_order_items_order ON order_items(order_id);
CREATE INDEX idx_order_items_course ON order_items(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_order_items_module ON order_items(module_id) WHERE module_id IS NOT NULL;

-- Giao dịch thanh toán
CREATE TABLE payment_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

    -- Thông tin giao dịch
    transaction_type VARCHAR(30) NOT NULL,
    -- payment: Thanh toán
    -- refund: Hoàn tiền
    -- chargeback: Chargeback
    -- installment: Trả góp

    amount DECIMAL(12, 2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'VND',

    -- Payment gateway
    gateway VARCHAR(50) NOT NULL, -- vnpay, momo, zalopay, stripe, paypal
    gateway_transaction_id VARCHAR(255),
    gateway_response JSONB, -- Response từ gateway

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'pending',
    -- pending, processing, completed, failed, cancelled

    -- Thông tin thanh toán
    payment_method VARCHAR(50),
    card_last_four VARCHAR(4),
    card_brand VARCHAR(20), -- visa, mastercard, etc.
    bank_code VARCHAR(20),

    -- Thời gian
    initiated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    failed_at TIMESTAMP,

    -- Error handling
    error_code VARCHAR(50),
    error_message TEXT,

    -- Metadata
    ip_address VARCHAR(45),
    user_agent TEXT,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE payment_transactions IS 'Giao dịch thanh toán với payment gateways. Lưu chi tiết từng transaction.';

CREATE INDEX idx_payment_trans_order ON payment_transactions(order_id);
CREATE INDEX idx_payment_trans_user ON payment_transactions(user_id);
CREATE INDEX idx_payment_trans_gateway ON payment_transactions(gateway, gateway_transaction_id);
CREATE INDEX idx_payment_trans_status ON payment_transactions(status);
CREATE INDEX idx_payment_trans_created ON payment_transactions(created_at);

-- Kế hoạch trả góp
CREATE TABLE installment_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

    -- Tổng quan
    total_amount DECIMAL(12, 2) NOT NULL,
    total_installments INT NOT NULL,
    installment_amount DECIMAL(12, 2) NOT NULL, -- Số tiền mỗi kỳ
    fee_amount DECIMAL(12, 2) DEFAULT 0, -- Phí trả góp

    -- Chu kỳ
    frequency VARCHAR(20) DEFAULT 'monthly',
    -- weekly, biweekly, monthly

    -- Tiến độ
    paid_amount DECIMAL(12, 2) DEFAULT 0,
    paid_installments INT DEFAULT 0,
    remaining_amount DECIMAL(12, 2),
    next_due_date DATE,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'active',
    -- active, completed, defaulted, cancelled

    -- Thời gian
    start_date DATE NOT NULL,
    end_date DATE,
    completed_at TIMESTAMP,

    -- Grace period
    grace_period_days INT DEFAULT 7,
    late_fee_percent DECIMAL(5, 2) DEFAULT 0,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE installment_plans IS 'Kế hoạch trả góp. Theo dõi tiến độ thanh toán từng kỳ.';
COMMENT ON COLUMN installment_plans.grace_period_days IS 'Số ngày ân hạn trước khi tính phí trễ hạn';

CREATE INDEX idx_installment_plans_order ON installment_plans(order_id);
CREATE INDEX idx_installment_plans_user ON installment_plans(user_id);
CREATE INDEX idx_installment_plans_status ON installment_plans(status) WHERE status = 'active';
CREATE INDEX idx_installment_plans_due ON installment_plans(next_due_date) WHERE status = 'active';

-- Chi tiết từng kỳ trả góp
CREATE TABLE installment_payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES installment_plans(id) ON DELETE CASCADE,

    -- Thông tin kỳ
    installment_number INT NOT NULL,
    amount DECIMAL(12, 2) NOT NULL,
    late_fee DECIMAL(12, 2) DEFAULT 0,
    total_due DECIMAL(12, 2) NOT NULL,

    -- Thời gian
    due_date DATE NOT NULL,
    paid_at TIMESTAMP,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'pending',
    -- pending, paid, overdue, waived

    -- Payment
    transaction_id UUID REFERENCES payment_transactions(id) ON DELETE SET NULL,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE installment_payments IS 'Chi tiết từng kỳ trả góp trong kế hoạch.';

CREATE INDEX idx_installment_payments_plan ON installment_payments(plan_id);
CREATE INDEX idx_installment_payments_due ON installment_payments(due_date) WHERE status IN ('pending', 'overdue');
CREATE INDEX idx_installment_payments_status ON installment_payments(status);

-- Vouchers / Mã giảm giá
CREATE TABLE vouchers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Mã voucher
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Loại giảm giá
    discount_type VARCHAR(20) NOT NULL,
    -- percentage: Giảm %
    -- fixed_amount: Giảm số tiền cố định
    -- free_shipping: Miễn phí ship (nếu có)

    discount_value DECIMAL(12, 2) NOT NULL,
    max_discount_amount DECIMAL(12, 2), -- Giới hạn giảm tối đa (cho %)

    -- Điều kiện áp dụng
    min_order_amount DECIMAL(12, 2), -- Đơn hàng tối thiểu
    min_items INT, -- Số items tối thiểu

    -- Phạm vi áp dụng
    applicable_to VARCHAR(30) DEFAULT 'all',
    -- all: Tất cả
    -- courses: Chỉ courses cụ thể
    -- modules: Chỉ modules cụ thể
    -- categories: Chỉ categories cụ thể
    -- first_purchase: Đơn hàng đầu tiên

    -- Giới hạn sử dụng
    usage_limit INT, -- Tổng số lần sử dụng
    usage_count INT DEFAULT 0,
    per_user_limit INT DEFAULT 1, -- Mỗi user dùng tối đa

    -- Thời gian hiệu lực
    starts_at TIMESTAMP,
    expires_at TIMESTAMP,

    -- Trạng thái
    is_active BOOLEAN DEFAULT TRUE,
    is_public BOOLEAN DEFAULT TRUE, -- Hiện công khai hay chỉ cho user cụ thể

    -- Tạo bởi
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE vouchers IS 'Mã giảm giá/voucher. Hỗ trợ giảm %, fixed amount, với nhiều điều kiện.';
COMMENT ON COLUMN vouchers.applicable_to IS 'all, courses, modules, categories, first_purchase';

CREATE INDEX idx_vouchers_code ON vouchers(code);
CREATE INDEX idx_vouchers_active ON vouchers(is_active, starts_at, expires_at) WHERE is_active = TRUE;
CREATE INDEX idx_vouchers_org ON vouchers(organization_id) WHERE organization_id IS NOT NULL;

-- Điều kiện áp dụng chi tiết cho voucher
CREATE TABLE voucher_conditions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    voucher_id UUID NOT NULL REFERENCES vouchers(id) ON DELETE CASCADE,

    -- Loại điều kiện
    condition_type VARCHAR(30) NOT NULL,
    -- include_courses: Áp dụng cho courses này
    -- exclude_courses: Không áp dụng cho courses này
    -- include_categories: Áp dụng cho categories này
    -- include_modules: Áp dụng cho modules này
    -- user_segment: Chỉ cho segment user cụ thể

    -- Giá trị
    entity_type VARCHAR(30), -- course, module, category
    entity_id UUID,
    entity_ids UUID[], -- Cho multiple values

    -- Cho user segment
    user_condition JSONB,
    -- VD: {"min_orders": 1, "membership_level": "gold"}

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE voucher_conditions IS 'Điều kiện chi tiết áp dụng voucher. Cho phép include/exclude courses, categories.';

CREATE INDEX idx_voucher_conditions_voucher ON voucher_conditions(voucher_id);
CREATE INDEX idx_voucher_conditions_entity ON voucher_conditions(entity_type, entity_id);

-- Lịch sử sử dụng voucher
CREATE TABLE voucher_usages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    voucher_id UUID NOT NULL REFERENCES vouchers(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,

    -- Số tiền đã giảm
    discount_amount DECIMAL(12, 2) NOT NULL,

    -- Thời điểm sử dụng
    used_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE voucher_usages IS 'Lịch sử sử dụng voucher. Tracking ai dùng, dùng cho đơn nào.';

CREATE INDEX idx_voucher_usages_voucher ON voucher_usages(voucher_id);
CREATE INDEX idx_voucher_usages_user ON voucher_usages(user_id);
CREATE INDEX idx_voucher_usages_order ON voucher_usages(order_id);
CREATE UNIQUE INDEX idx_voucher_usages_unique ON voucher_usages(voucher_id, user_id, order_id);

-- Thanh toán cho Instructor
CREATE TABLE instructor_earnings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instructor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

    -- Nguồn
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    order_item_id UUID NOT NULL REFERENCES order_items(id) ON DELETE RESTRICT,
    course_id UUID REFERENCES courses(id) ON DELETE SET NULL,

    -- Số tiền
    gross_amount DECIMAL(12, 2) NOT NULL, -- Tổng doanh thu
    platform_fee DECIMAL(12, 2) NOT NULL, -- Phí platform
    payment_processing_fee DECIMAL(12, 2) DEFAULT 0,
    tax_amount DECIMAL(12, 2) DEFAULT 0,
    net_amount DECIMAL(12, 2) NOT NULL, -- Instructor nhận được

    -- Tỷ lệ chia
    revenue_share_percent DECIMAL(5, 2) NOT NULL, -- % instructor nhận

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'pending',
    -- pending: Chờ xác nhận
    -- confirmed: Đã xác nhận
    -- paid: Đã thanh toán
    -- held: Giữ lại (dispute)

    -- Payout
    payout_id UUID,
    paid_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE instructor_earnings IS 'Thu nhập của instructor từ mỗi đơn hàng. Track chi tiết revenue share.';

CREATE INDEX idx_instructor_earnings_instructor ON instructor_earnings(instructor_id);
CREATE INDEX idx_instructor_earnings_order ON instructor_earnings(order_id);
CREATE INDEX idx_instructor_earnings_course ON instructor_earnings(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_instructor_earnings_status ON instructor_earnings(status);

-- Chi trả cho Instructor
CREATE TABLE instructor_payouts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instructor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

    -- Số tiền
    amount DECIMAL(12, 2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'VND',

    -- Chu kỳ thanh toán
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,

    -- Thông tin ngân hàng
    payout_method VARCHAR(50) NOT NULL, -- bank_transfer, paypal, momo
    bank_name VARCHAR(100),
    bank_account_number VARCHAR(50),
    bank_account_name VARCHAR(255),
    bank_branch VARCHAR(255),

    -- Giao dịch
    transaction_id VARCHAR(255),
    transaction_reference VARCHAR(255),

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'pending',
    -- pending, processing, completed, failed, cancelled

    processed_at TIMESTAMP,
    failed_at TIMESTAMP,
    failure_reason TEXT,

    -- Audit
    processed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    notes TEXT,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE instructor_payouts IS 'Chi trả tiền cho instructor theo chu kỳ.';

CREATE INDEX idx_instructor_payouts_instructor ON instructor_payouts(instructor_id);
CREATE INDEX idx_instructor_payouts_status ON instructor_payouts(status);
CREATE INDEX idx_instructor_payouts_period ON instructor_payouts(period_start, period_end);

-- =============================================================================
-- MODULE 15: CERTIFICATES & EXTERNAL EXAMS
-- =============================================================================

-- Nhà cung cấp chứng chỉ bên ngoài
CREATE TABLE exam_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Thông tin
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    logo_url VARCHAR(500),
    website VARCHAR(255),

    -- Loại
    provider_type VARCHAR(50) NOT NULL,
    -- certification_body: Tổ chức cấp chứng chỉ (AWS, Google, Microsoft)
    -- testing_center: Trung tâm thi
    -- university: Đại học
    -- professional_org: Tổ chức nghề nghiệp

    -- Thông tin liên hệ
    contact_email VARCHAR(255),
    contact_phone VARCHAR(20),

    -- Cài đặt tích hợp
    integration_type VARCHAR(50), -- api, manual, oauth
    api_endpoint VARCHAR(500),
    api_credentials_encrypted TEXT, -- Encrypted credentials

    -- Trạng thái
    is_active BOOLEAN DEFAULT TRUE,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE exam_providers IS 'Nhà cung cấp chứng chỉ/kỳ thi bên ngoài: AWS, Google, Microsoft, etc.';

CREATE INDEX idx_exam_providers_slug ON exam_providers(slug);
CREATE INDEX idx_exam_providers_type ON exam_providers(provider_type);

-- Kỳ thi bên ngoài
CREATE TABLE external_exams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES exam_providers(id) ON DELETE CASCADE,

    -- Thông tin
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50), -- Mã kỳ thi (VD: AWS-SAA-C03)
    description TEXT,

    -- Cấp độ
    level VARCHAR(50), -- foundational, associate, professional, expert
    difficulty_level VARCHAR(20), -- beginner, intermediate, advanced, expert

    -- Yêu cầu
    prerequisites TEXT[], -- Các yêu cầu trước khi thi
    recommended_experience TEXT,

    -- Chi tiết thi
    duration_minutes INT,
    passing_score DECIMAL(5, 2),
    total_questions INT,
    question_types VARCHAR(50)[], -- multiple_choice, hands_on, essay

    -- Chi phí
    exam_fee DECIMAL(12, 2),
    currency VARCHAR(3) DEFAULT 'USD',
    retake_fee DECIMAL(12, 2),

    -- Hiệu lực chứng chỉ
    validity_years INT, -- Số năm hiệu lực

    -- Courses liên quan trong 40Study
    related_course_ids UUID[],

    -- URLs
    registration_url VARCHAR(500),
    preparation_url VARCHAR(500),

    -- Trạng thái
    is_active BOOLEAN DEFAULT TRUE,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE external_exams IS 'Kỳ thi chứng chỉ bên ngoài. Liên kết với courses để recommend học.';

CREATE INDEX idx_external_exams_provider ON external_exams(provider_id);
CREATE INDEX idx_external_exams_code ON external_exams(code) WHERE code IS NOT NULL;
CREATE INDEX idx_external_exams_level ON external_exams(level);

-- Template chứng chỉ
CREATE TABLE certificate_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Thông tin
    name VARCHAR(255) NOT NULL,
    description TEXT,

    -- Design
    template_html TEXT, -- HTML template
    template_css TEXT,
    background_image_url VARCHAR(500),
    thumbnail_url VARCHAR(500),

    -- Các trường có thể điền
    available_fields JSONB,
    -- VD: ["student_name", "course_title", "completion_date", "instructor_name", "grade"]

    -- Cài đặt
    paper_size VARCHAR(20) DEFAULT 'A4', -- A4, Letter, Custom
    orientation VARCHAR(20) DEFAULT 'landscape', -- portrait, landscape
    margins JSONB, -- {top: 20, right: 20, bottom: 20, left: 20}

    -- Scope
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    is_system_template BOOLEAN DEFAULT FALSE, -- Template hệ thống

    -- Trạng thái
    is_active BOOLEAN DEFAULT TRUE,

    -- Audit
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE certificate_templates IS 'Template chứng chỉ. Có thể custom theo organization.';

CREATE INDEX idx_cert_templates_org ON certificate_templates(organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_cert_templates_system ON certificate_templates(is_system_template) WHERE is_system_template = TRUE;

-- Chứng chỉ đã cấp
CREATE TABLE certificates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Loại chứng chỉ
    certificate_type VARCHAR(30) NOT NULL,
    -- course_completion: Hoàn thành khóa học
    -- module_completion: Hoàn thành module
    -- exam_pass: Đậu kỳ thi
    -- external: Chứng chỉ bên ngoài
    -- achievement: Thành tích

    -- Nguồn
    course_id UUID REFERENCES courses(id) ON DELETE SET NULL,
    module_id UUID REFERENCES modules(id) ON DELETE SET NULL,
    enrollment_id UUID REFERENCES enrollments(id) ON DELETE SET NULL,
    external_exam_id UUID REFERENCES external_exams(id) ON DELETE SET NULL,

    -- Template
    template_id UUID REFERENCES certificate_templates(id) ON DELETE SET NULL,

    -- Thông tin chứng chỉ
    certificate_number VARCHAR(50) UNIQUE NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,

    -- Điểm số (nếu có)
    score DECIMAL(5, 2),
    grade VARCHAR(20), -- A, B, C or Pass/Fail

    -- Thời gian
    issued_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP, -- NULL = không hết hạn

    -- File
    certificate_url VARCHAR(500), -- PDF URL
    certificate_image_url VARCHAR(500), -- Image URL
    verification_url VARCHAR(500), -- URL để verify

    -- Verification
    verification_code VARCHAR(100) UNIQUE,
    is_verified BOOLEAN DEFAULT TRUE,
    blockchain_hash VARCHAR(255), -- Nếu dùng blockchain verification

    -- Sharing
    is_public BOOLEAN DEFAULT FALSE,
    linkedin_added BOOLEAN DEFAULT FALSE,

    -- Metadata
    metadata JSONB,
    -- VD: {instructor_name: "...", skills: [...], hours_completed: 40}

    -- Audit
    issued_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE certificates IS 'Chứng chỉ đã cấp cho user. Hỗ trợ internal và external certificates.';
COMMENT ON COLUMN certificates.blockchain_hash IS 'Hash trên blockchain để verify tính xác thực (optional)';

CREATE INDEX idx_certificates_user ON certificates(user_id);
CREATE INDEX idx_certificates_course ON certificates(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_certificates_type ON certificates(certificate_type);
CREATE INDEX idx_certificates_number ON certificates(certificate_number);
CREATE INDEX idx_certificates_verification ON certificates(verification_code);

-- Liên kết foreign key cho enrollments
ALTER TABLE enrollments
ADD CONSTRAINT fk_enrollments_certificate
FOREIGN KEY (certificate_id) REFERENCES certificates(id) ON DELETE SET NULL;

-- Chứng chỉ bên ngoài của user (không qua 40Study)
CREATE TABLE user_external_certificates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Nhà cung cấp (có thể không có trong hệ thống)
    provider_id UUID REFERENCES exam_providers(id) ON DELETE SET NULL,
    provider_name VARCHAR(255), -- Nếu không có trong hệ thống
    exam_id UUID REFERENCES external_exams(id) ON DELETE SET NULL,

    -- Thông tin chứng chỉ
    certificate_name VARCHAR(255) NOT NULL,
    certificate_number VARCHAR(100),
    credential_id VARCHAR(255),
    credential_url VARCHAR(500),

    -- Thời gian
    issued_at DATE NOT NULL,
    expires_at DATE,

    -- File upload
    certificate_file_url VARCHAR(500),

    -- Verification
    verification_status VARCHAR(20) DEFAULT 'pending',
    -- pending, verified, rejected, expired
    verified_at TIMESTAMP,
    verified_by UUID REFERENCES users(id) ON DELETE SET NULL,
    verification_notes TEXT,

    -- Trạng thái
    is_public BOOLEAN DEFAULT TRUE,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE user_external_certificates IS 'Chứng chỉ bên ngoài do user tự upload/khai báo.';

CREATE INDEX idx_user_ext_certs_user ON user_external_certificates(user_id);
CREATE INDEX idx_user_ext_certs_provider ON user_external_certificates(provider_id) WHERE provider_id IS NOT NULL;
CREATE INDEX idx_user_ext_certs_status ON user_external_certificates(verification_status);

-- =============================================================================
-- MODULE 16: AI & LEARNING PATHS
-- =============================================================================

-- Lộ trình học tập
CREATE TABLE learning_paths (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Ownership
    user_id UUID REFERENCES users(id) ON DELETE CASCADE, -- NULL nếu là template
    organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,

    -- Thông tin
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255),
    description TEXT,
    thumbnail_url VARCHAR(500),

    -- Mục tiêu
    goal TEXT, -- Mục tiêu cần đạt
    target_role VARCHAR(100), -- VD: "Frontend Developer", "Data Scientist"
    target_skills VARCHAR(100)[],

    -- Loại
    path_type VARCHAR(30) DEFAULT 'custom',
    -- template: Template có sẵn
    -- custom: User tự tạo
    -- ai_generated: AI tạo
    -- organization: Của organization

    -- AI
    is_ai_generated BOOLEAN DEFAULT FALSE,
    ai_generation_params JSONB,
    ai_confidence_score DECIMAL(3, 2),

    -- Thống kê
    estimated_duration_hours INT,
    total_courses INT DEFAULT 0,
    total_modules INT DEFAULT 0,
    difficulty_level VARCHAR(20),

    -- Tiến độ (cho user-specific paths)
    progress_percentage DECIMAL(5, 2) DEFAULT 0,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'active',
    -- draft, active, completed, paused, abandoned
    is_public BOOLEAN DEFAULT FALSE, -- Chia sẻ công khai

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE learning_paths IS 'Lộ trình học tập. Có thể là template, user-created, hoặc AI-generated.';
COMMENT ON COLUMN learning_paths.target_role IS 'Vai trò mục tiêu: Frontend Developer, Data Scientist, etc.';

CREATE INDEX idx_learning_paths_user ON learning_paths(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_learning_paths_org ON learning_paths(organization_id) WHERE organization_id IS NOT NULL;
CREATE INDEX idx_learning_paths_type ON learning_paths(path_type);
CREATE INDEX idx_learning_paths_public ON learning_paths(is_public) WHERE is_public = TRUE;
CREATE INDEX idx_learning_paths_status ON learning_paths(status);

-- Items trong Learning Path (courses, modules, external resources)
CREATE TABLE learning_path_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    learning_path_id UUID NOT NULL REFERENCES learning_paths(id) ON DELETE CASCADE,

    -- Loại item
    item_type VARCHAR(30) NOT NULL,
    -- course: Khóa học
    -- module: Module cụ thể
    -- external_resource: Tài liệu bên ngoài
    -- external_exam: Kỳ thi chứng chỉ
    -- milestone: Mốc quan trọng

    -- Reference
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,
    module_id UUID REFERENCES modules(id) ON DELETE CASCADE,
    external_exam_id UUID REFERENCES external_exams(id) ON DELETE SET NULL,

    -- Cho external resources
    external_url VARCHAR(500),
    external_title VARCHAR(255),
    external_description TEXT,

    -- Thứ tự và phân nhóm
    display_order INT NOT NULL,
    section_name VARCHAR(100), -- Nhóm items theo section

    -- Cài đặt
    is_required BOOLEAN DEFAULT TRUE,
    is_optional BOOLEAN DEFAULT FALSE,

    -- AI explanation
    reason TEXT, -- Tại sao recommend item này
    skills_gained VARCHAR(100)[], -- Skills học được

    -- Tiến độ
    status VARCHAR(20) DEFAULT 'pending',
    -- pending, in_progress, completed, skipped
    completed_at TIMESTAMP,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE learning_path_items IS 'Items trong lộ trình: courses, modules, external resources, milestones.';

CREATE INDEX idx_lp_items_path ON learning_path_items(learning_path_id);
CREATE INDEX idx_lp_items_course ON learning_path_items(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_lp_items_order ON learning_path_items(learning_path_id, display_order);

-- AI Recommendations
CREATE TABLE ai_recommendations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Loại recommendation
    recommendation_type VARCHAR(30) NOT NULL,
    -- course: Recommend khóa học
    -- module: Recommend module
    -- learning_path: Recommend lộ trình
    -- lesson_review: Recommend ôn tập bài
    -- quiz_practice: Recommend làm quiz
    -- next_step: Bước tiếp theo
    -- skill_gap: Lỗ hổng kỹ năng

    -- Target
    target_type VARCHAR(30) NOT NULL, -- course, module, lesson, quiz, learning_path
    target_id UUID NOT NULL,
    target_title VARCHAR(255),

    -- Context
    context_type VARCHAR(30), -- after_lesson, daily, weekly, goal_based
    context_data JSONB,

    -- AI reasoning
    reason TEXT, -- Lý do AI recommend
    ai_model VARCHAR(50), -- Model đã dùng
    confidence_score DECIMAL(3, 2), -- 0.00 - 1.00
    factors JSONB, -- Các yếu tố quyết định

    -- Priority
    priority INT DEFAULT 0, -- Cao hơn = quan trọng hơn
    display_position INT,

    -- User interaction
    is_viewed BOOLEAN DEFAULT FALSE,
    viewed_at TIMESTAMP,
    is_acted_upon BOOLEAN DEFAULT FALSE, -- User đã click/enroll
    acted_at TIMESTAMP,
    is_dismissed BOOLEAN DEFAULT FALSE,
    dismissed_at TIMESTAMP,
    feedback VARCHAR(20), -- helpful, not_helpful, irrelevant

    -- Expiry
    expires_at TIMESTAMP,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE ai_recommendations IS 'AI recommendations cho users. Track interaction để improve model.';
COMMENT ON COLUMN ai_recommendations.factors IS 'Các yếu tố: {learning_history, quiz_scores, time_spent, goals}';

CREATE INDEX idx_ai_recs_user ON ai_recommendations(user_id);
CREATE INDEX idx_ai_recs_type ON ai_recommendations(recommendation_type);
CREATE INDEX idx_ai_recs_target ON ai_recommendations(target_type, target_id);
CREATE INDEX idx_ai_recs_pending ON ai_recommendations(user_id, is_viewed, expires_at)
    WHERE is_viewed = FALSE AND is_dismissed = FALSE;

-- AI Chat Sessions
CREATE TABLE ai_chat_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Context
    context_type VARCHAR(30), -- general, course, lesson, code_help
    course_id UUID REFERENCES courses(id) ON DELETE SET NULL,
    lesson_id UUID REFERENCES lessons(id) ON DELETE SET NULL,

    -- Session info
    title VARCHAR(255),
    summary TEXT, -- AI-generated summary

    -- Thống kê
    message_count INT DEFAULT 0,
    total_tokens_used INT DEFAULT 0,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'active',
    -- active, archived, deleted

    -- Audit
    last_message_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE ai_chat_sessions IS 'Phiên chat với AI assistant. Có thể gắn với course/lesson context.';

CREATE INDEX idx_ai_chat_sessions_user ON ai_chat_sessions(user_id);
CREATE INDEX idx_ai_chat_sessions_course ON ai_chat_sessions(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_ai_chat_sessions_recent ON ai_chat_sessions(user_id, last_message_at DESC);

-- AI Chat Messages
CREATE TABLE ai_chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES ai_chat_sessions(id) ON DELETE CASCADE,

    -- Message
    role VARCHAR(20) NOT NULL, -- user, assistant, system
    content TEXT NOT NULL,

    -- Metadata
    tokens_used INT,
    model_used VARCHAR(50),

    -- Cho code blocks
    code_blocks JSONB, -- [{language: "python", code: "..."}]

    -- References
    referenced_lessons UUID[],
    referenced_documents JSONB,

    -- Feedback
    feedback VARCHAR(20), -- helpful, not_helpful
    feedback_text TEXT,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE ai_chat_messages IS 'Messages trong AI chat session.';

CREATE INDEX idx_ai_chat_messages_session ON ai_chat_messages(session_id);
CREATE INDEX idx_ai_chat_messages_created ON ai_chat_messages(session_id, created_at);

-- AI Tasks/Jobs (cho background AI processing)
CREATE TABLE ai_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Task type
    task_type VARCHAR(50) NOT NULL,
    -- quiz_generation: Tạo quiz từ content
    -- transcription: Transcribe video
    -- summarization: Tóm tắt nội dung
    -- translation: Dịch content
    -- recommendation_batch: Tính toán recommendations
    -- knowledge_extraction: Trích xuất knowledge

    -- Input
    input_type VARCHAR(30), -- lesson, course, video, text
    input_id UUID,
    input_data JSONB,

    -- Cài đặt
    config JSONB, -- Task-specific configuration
    priority INT DEFAULT 0,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'pending',
    -- pending, queued, processing, completed, failed, cancelled

    -- Progress
    progress_percentage DECIMAL(5, 2) DEFAULT 0,
    progress_message TEXT,

    -- Output
    output_data JSONB,
    output_url VARCHAR(500),

    -- Timing
    queued_at TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    estimated_completion TIMESTAMP,

    -- Error handling
    error_code VARCHAR(50),
    error_message TEXT,
    retry_count INT DEFAULT 0,
    max_retries INT DEFAULT 3,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE ai_tasks IS 'AI background tasks: quiz generation, transcription, etc.';

CREATE INDEX idx_ai_tasks_user ON ai_tasks(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_ai_tasks_type ON ai_tasks(task_type);
CREATE INDEX idx_ai_tasks_status ON ai_tasks(status) WHERE status IN ('pending', 'queued', 'processing');
CREATE INDEX idx_ai_tasks_priority ON ai_tasks(priority DESC, created_at) WHERE status = 'pending';

-- Knowledge Segments (AI-extracted knowledge từ content)
CREATE TABLE knowledge_segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Source
    source_type VARCHAR(30) NOT NULL, -- lesson, video, article, document
    source_id UUID NOT NULL,
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,

    -- Content
    segment_type VARCHAR(30) NOT NULL,
    -- concept: Khái niệm
    -- definition: Định nghĩa
    -- example: Ví dụ
    -- formula: Công thức
    -- code_snippet: Đoạn code
    -- key_point: Điểm quan trọng

    title VARCHAR(255),
    content TEXT NOT NULL,
    summary TEXT,

    -- Position trong source
    start_time_seconds INT, -- Cho video
    end_time_seconds INT,
    start_offset INT, -- Cho text
    end_offset INT,

    -- Metadata
    keywords VARCHAR(100)[],
    topics VARCHAR(100)[],
    difficulty_level VARCHAR(20),

    -- AI info
    ai_extracted BOOLEAN DEFAULT TRUE,
    extraction_confidence DECIMAL(3, 2),
    ai_task_id UUID REFERENCES ai_tasks(id) ON DELETE SET NULL,

    -- Relationships
    related_segments UUID[], -- Liên quan đến segments khác
    prerequisite_segments UUID[], -- Cần học trước

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE knowledge_segments IS 'Knowledge được AI trích xuất từ content. Dùng cho search và recommendations.';

CREATE INDEX idx_knowledge_segments_source ON knowledge_segments(source_type, source_id);
CREATE INDEX idx_knowledge_segments_course ON knowledge_segments(course_id);
CREATE INDEX idx_knowledge_segments_type ON knowledge_segments(segment_type);
CREATE INDEX idx_knowledge_segments_keywords ON knowledge_segments USING gin(keywords);
CREATE INDEX idx_knowledge_segments_topics ON knowledge_segments USING gin(topics);

-- User Learning Analytics (AI-generated insights)
CREATE TABLE user_learning_analytics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Chu kỳ phân tích
    analysis_date DATE NOT NULL,
    analysis_type VARCHAR(20) DEFAULT 'daily',
    -- daily, weekly, monthly

    -- Thời gian học
    total_study_minutes INT DEFAULT 0,
    avg_session_minutes DECIMAL(8, 2),
    study_sessions_count INT DEFAULT 0,

    -- Tiến độ
    lessons_completed INT DEFAULT 0,
    lessons_started INT DEFAULT 0,
    courses_active INT DEFAULT 0,
    courses_completed INT DEFAULT 0,

    -- Quiz performance
    quizzes_taken INT DEFAULT 0,
    quizzes_passed INT DEFAULT 0,
    average_quiz_score DECIMAL(5, 2),
    best_quiz_score DECIMAL(5, 2),

    -- Code exercises
    code_submissions INT DEFAULT 0,
    code_success_rate DECIMAL(5, 2),

    -- Streak
    current_streak_days INT DEFAULT 0,
    longest_streak_days INT DEFAULT 0,

    -- Learning patterns
    peak_study_hours INT[], -- Giờ học nhiều nhất
    preferred_content_types VARCHAR(30)[], -- video, article, quiz
    avg_focus_score DECIMAL(5, 2),

    -- AI Insights
    strong_topics VARCHAR(100)[],
    weak_topics VARCHAR(100)[],
    learning_style VARCHAR(30), -- visual, auditory, reading, kinesthetic
    ai_insights TEXT, -- AI-generated text insights
    ai_suggestions JSONB, -- Structured suggestions

    -- Goals
    weekly_goal_minutes INT,
    goal_progress_percent DECIMAL(5, 2),

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, analysis_date, analysis_type)
);

COMMENT ON TABLE user_learning_analytics IS 'AI-generated learning analytics. Aggregated daily/weekly/monthly.';

CREATE INDEX idx_user_analytics_user ON user_learning_analytics(user_id);
CREATE INDEX idx_user_analytics_date ON user_learning_analytics(analysis_date);
CREATE INDEX idx_user_analytics_user_date ON user_learning_analytics(user_id, analysis_date DESC);

-- =============================================================================
-- MODULE 17: COMMENTS & DISCUSSIONS (Adjacency List Model)
-- =============================================================================

-- Thảo luận trong bài học
CREATE TABLE discussions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Context
    context_type VARCHAR(30) NOT NULL,
    -- lesson: Thảo luận trong bài học
    -- course: Thảo luận chung của course
    -- livestream: Q&A trong livestream

    lesson_id UUID REFERENCES lessons(id) ON DELETE CASCADE,
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,
    livestream_id UUID REFERENCES livestreams(id) ON DELETE CASCADE,

    -- Reply (Adjacency List - NOT Nested Set)
    -- Tại sao Adjacency List: Đơn giản, dễ maintain, phù hợp với comment systems
    -- có nhiều writes và shallow depth (thường không quá 3-4 levels)
    parent_id UUID REFERENCES discussions(id) ON DELETE CASCADE,
    root_id UUID REFERENCES discussions(id) ON DELETE CASCADE, -- Root comment để query thread nhanh

    -- Nội dung
    content TEXT NOT NULL,
    content_html TEXT, -- Pre-rendered HTML

    -- Vị trí trong video (nếu có)
    video_timestamp_seconds INT,

    -- Mentions
    mentioned_user_ids UUID[],

    -- Thống kê
    upvote_count INT DEFAULT 0,
    downvote_count INT DEFAULT 0,
    reply_count INT DEFAULT 0,

    -- Trạng thái đặc biệt
    is_pinned BOOLEAN DEFAULT FALSE,
    is_instructor_answer BOOLEAN DEFAULT FALSE, -- Marked as answer by instructor
    is_accepted_answer BOOLEAN DEFAULT FALSE, -- Accepted by question author
    is_featured BOOLEAN DEFAULT FALSE,

    -- Moderation
    is_hidden BOOLEAN DEFAULT FALSE,
    hidden_reason VARCHAR(100),
    hidden_by UUID REFERENCES users(id) ON DELETE SET NULL,
    hidden_at TIMESTAMP,

    is_edited BOOLEAN DEFAULT FALSE,
    edited_at TIMESTAMP,
    edit_history JSONB, -- [{content: "...", edited_at: "..."}]

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE discussions IS 'Thảo luận/Comments. Sử dụng Adjacency List cho replies (parent_id + root_id).';
COMMENT ON COLUMN discussions.root_id IS 'ID của comment gốc trong thread. NULL nếu đây là root comment.';

CREATE INDEX idx_discussions_user ON discussions(user_id);
CREATE INDEX idx_discussions_lesson ON discussions(lesson_id) WHERE lesson_id IS NOT NULL;
CREATE INDEX idx_discussions_course ON discussions(course_id) WHERE course_id IS NOT NULL;
CREATE INDEX idx_discussions_parent ON discussions(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_discussions_root ON discussions(root_id) WHERE root_id IS NOT NULL;
CREATE INDEX idx_discussions_pinned ON discussions(lesson_id, is_pinned) WHERE is_pinned = TRUE;
CREATE INDEX idx_discussions_timestamp ON discussions(lesson_id, video_timestamp_seconds)
    WHERE video_timestamp_seconds IS NOT NULL;

-- Votes cho discussions
CREATE TABLE discussion_votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    discussion_id UUID NOT NULL REFERENCES discussions(id) ON DELETE CASCADE,

    vote_type VARCHAR(10) NOT NULL, -- upvote, downvote

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, discussion_id)
);

COMMENT ON TABLE discussion_votes IS 'Upvote/Downvote cho discussions.';

CREATE INDEX idx_discussion_votes_discussion ON discussion_votes(discussion_id);
CREATE INDEX idx_discussion_votes_user ON discussion_votes(user_id);

-- Reviews khóa học
CREATE TABLE reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    enrollment_id UUID REFERENCES enrollments(id) ON DELETE SET NULL,

    -- Rating
    rating INT NOT NULL CHECK (rating >= 1 AND rating <= 5),

    -- Nội dung
    title VARCHAR(255),
    content TEXT,

    -- Breakdown ratings (optional)
    rating_content INT CHECK (rating_content >= 1 AND rating_content <= 5),
    rating_instructor INT CHECK (rating_instructor >= 1 AND rating_instructor <= 5),
    rating_support INT CHECK (rating_support >= 1 AND rating_support <= 5),
    rating_value INT CHECK (rating_value >= 1 AND rating_value <= 5),

    -- Verification
    is_verified_purchase BOOLEAN DEFAULT TRUE,
    progress_at_review DECIMAL(5, 2), -- % hoàn thành khi review

    -- Helpful votes
    helpful_count INT DEFAULT 0,
    not_helpful_count INT DEFAULT 0,

    -- Instructor response
    instructor_reply TEXT,
    instructor_replied_at TIMESTAMP,

    -- Moderation
    is_featured BOOLEAN DEFAULT FALSE,
    is_hidden BOOLEAN DEFAULT FALSE,
    hidden_reason VARCHAR(100),
    reported_count INT DEFAULT 0,

    -- Audit
    is_edited BOOLEAN DEFAULT FALSE,
    edited_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, course_id)
);

COMMENT ON TABLE reviews IS 'Reviews/Đánh giá khóa học. Mỗi user chỉ được review 1 lần per course.';

CREATE INDEX idx_reviews_course ON reviews(course_id);
CREATE INDEX idx_reviews_user ON reviews(user_id);
CREATE INDEX idx_reviews_rating ON reviews(course_id, rating);
CREATE INDEX idx_reviews_featured ON reviews(course_id, is_featured) WHERE is_featured = TRUE;
CREATE INDEX idx_reviews_recent ON reviews(course_id, created_at DESC) WHERE is_hidden = FALSE;

-- Helpful votes cho reviews
CREATE TABLE review_reactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    review_id UUID NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,

    reaction_type VARCHAR(20) NOT NULL, -- helpful, not_helpful, report

    -- Cho report
    report_reason VARCHAR(50),
    report_description TEXT,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, review_id)
);

COMMENT ON TABLE review_reactions IS 'Reactions cho reviews: helpful, not_helpful, report.';

CREATE INDEX idx_review_reactions_review ON review_reactions(review_id);

-- =============================================================================
-- MODULE 18: NOTIFICATIONS
-- =============================================================================

-- Notifications
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Loại notification
    notification_type VARCHAR(50) NOT NULL,
    -- course_update: Course được update
    -- new_lesson: Bài học mới
    -- quiz_reminder: Nhắc làm quiz
    -- assignment_due: Deadline bài tập
    -- discussion_reply: Có reply trong discussion
    -- discussion_mention: Được mention
    -- review_response: Instructor trả lời review
    -- certificate_earned: Nhận certificate
    -- payment_success: Thanh toán thành công
    -- payment_failed: Thanh toán thất bại
    -- installment_due: Đến hạn trả góp
    -- promotion: Khuyến mãi
    -- system: Thông báo hệ thống
    -- ai_recommendation: AI recommend

    -- Nội dung
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    image_url VARCHAR(500),

    -- Action
    action_url VARCHAR(500), -- URL khi click
    action_type VARCHAR(30), -- navigate, external_link, in_app_action

    -- Reference
    reference_type VARCHAR(30), -- course, lesson, discussion, order, etc.
    reference_id UUID,

    -- Sender (nếu có)
    sender_id UUID REFERENCES users(id) ON DELETE SET NULL,
    sender_type VARCHAR(30), -- user, system, ai

    -- Priority
    priority VARCHAR(20) DEFAULT 'normal',
    -- low, normal, high, urgent

    -- Trạng thái
    is_read BOOLEAN DEFAULT FALSE,
    read_at TIMESTAMP,
    is_archived BOOLEAN DEFAULT FALSE,
    archived_at TIMESTAMP,

    -- Delivery
    channels VARCHAR(20)[], -- in_app, email, push, sms
    email_sent BOOLEAN DEFAULT FALSE,
    email_sent_at TIMESTAMP,
    push_sent BOOLEAN DEFAULT FALSE,
    push_sent_at TIMESTAMP,

    -- Expiry
    expires_at TIMESTAMP,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE notifications IS 'Notifications cho users. Hỗ trợ multiple delivery channels.';

CREATE INDEX idx_notifications_user ON notifications(user_id);
CREATE INDEX idx_notifications_unread ON notifications(user_id, is_read, created_at DESC)
    WHERE is_read = FALSE AND is_archived = FALSE;
CREATE INDEX idx_notifications_type ON notifications(notification_type);
CREATE INDEX idx_notifications_reference ON notifications(reference_type, reference_id)
    WHERE reference_id IS NOT NULL;

-- Cài đặt notification của user
CREATE TABLE notification_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Email notifications
    email_enabled BOOLEAN DEFAULT TRUE,
    email_course_updates BOOLEAN DEFAULT TRUE,
    email_new_lessons BOOLEAN DEFAULT TRUE,
    email_quiz_reminders BOOLEAN DEFAULT TRUE,
    email_discussion_replies BOOLEAN DEFAULT TRUE,
    email_promotions BOOLEAN DEFAULT TRUE,
    email_newsletter BOOLEAN DEFAULT TRUE,
    email_digest_frequency VARCHAR(20) DEFAULT 'daily', -- none, daily, weekly

    -- Push notifications
    push_enabled BOOLEAN DEFAULT TRUE,
    push_course_updates BOOLEAN DEFAULT TRUE,
    push_new_lessons BOOLEAN DEFAULT TRUE,
    push_quiz_reminders BOOLEAN DEFAULT TRUE,
    push_discussion_replies BOOLEAN DEFAULT TRUE,
    push_livestream_start BOOLEAN DEFAULT TRUE,
    push_promotions BOOLEAN DEFAULT FALSE,

    -- In-app notifications
    in_app_enabled BOOLEAN DEFAULT TRUE,

    -- Quiet hours
    quiet_hours_enabled BOOLEAN DEFAULT FALSE,
    quiet_hours_start TIME, -- VD: 22:00
    quiet_hours_end TIME, -- VD: 08:00
    quiet_hours_timezone VARCHAR(50) DEFAULT 'Asia/Ho_Chi_Minh',

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE notification_settings IS 'Cài đặt notification preferences cho user.';

-- Push notification tokens
CREATE TABLE push_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Token
    token TEXT NOT NULL,
    token_type VARCHAR(30) NOT NULL, -- fcm, apns, web_push

    -- Device info
    device_id VARCHAR(255),
    device_name VARCHAR(255),
    device_type VARCHAR(30), -- ios, android, web
    os_version VARCHAR(50),
    app_version VARCHAR(20),

    -- Status
    is_active BOOLEAN DEFAULT TRUE,
    last_used_at TIMESTAMP,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, token)
);

COMMENT ON TABLE push_tokens IS 'Push notification tokens cho từng device.';

CREATE INDEX idx_push_tokens_user ON push_tokens(user_id);
CREATE INDEX idx_push_tokens_active ON push_tokens(user_id, is_active) WHERE is_active = TRUE;

-- =============================================================================
-- MODULE 19: ANALYTICS, AUDIT & REPORTS
-- =============================================================================

-- Activity Logs (Audit Trail)
CREATE TABLE activity_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Actor
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,

    -- Action
    action VARCHAR(50) NOT NULL,
    -- user.login, user.logout, user.password_change
    -- course.create, course.update, course.publish
    -- enrollment.create, enrollment.complete
    -- payment.success, payment.fail
    -- admin.user_ban, admin.course_approve

    action_category VARCHAR(30), -- auth, course, enrollment, payment, admin

    -- Target
    entity_type VARCHAR(50),
    entity_id UUID,
    entity_name VARCHAR(255), -- Cached name for display

    -- Changes
    old_values JSONB,
    new_values JSONB,
    changes_summary TEXT, -- Human-readable summary

    -- Context
    ip_address VARCHAR(45),
    user_agent TEXT,
    device_type VARCHAR(30),
    location_country VARCHAR(100),
    location_city VARCHAR(100),

    -- Request info
    request_id VARCHAR(100), -- For tracing
    request_path VARCHAR(500),
    request_method VARCHAR(10),

    -- Result
    status VARCHAR(20) DEFAULT 'success', -- success, failure, error
    error_message TEXT,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE activity_logs IS 'Audit trail cho tất cả actions quan trọng trong hệ thống.';

CREATE INDEX idx_activity_logs_user ON activity_logs(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_activity_logs_action ON activity_logs(action);
CREATE INDEX idx_activity_logs_entity ON activity_logs(entity_type, entity_id) WHERE entity_id IS NOT NULL;
CREATE INDEX idx_activity_logs_created ON activity_logs(created_at);
CREATE INDEX idx_activity_logs_request ON activity_logs(request_id) WHERE request_id IS NOT NULL;

-- Partition by month for better performance (optional, depends on volume)
-- CREATE TABLE activity_logs_2024_01 PARTITION OF activity_logs
--     FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');

-- Reports (Báo cáo vi phạm)
CREATE TABLE reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Target
    reported_type VARCHAR(30) NOT NULL,
    -- user, course, review, discussion, livestream_chat
    reported_id UUID NOT NULL,
    reported_user_id UUID REFERENCES users(id) ON DELETE SET NULL, -- User bị report

    -- Lý do
    reason VARCHAR(50) NOT NULL,
    -- spam, inappropriate, copyright, harassment, misinformation, other
    reason_detail TEXT,

    -- Evidence
    evidence_urls VARCHAR(500)[],
    screenshots JSONB,

    -- Trạng thái
    status VARCHAR(20) DEFAULT 'pending',
    -- pending, reviewing, resolved, dismissed, escalated
    priority VARCHAR(20) DEFAULT 'normal', -- low, normal, high, critical

    -- Resolution
    resolution VARCHAR(50),
    -- no_action, warning_issued, content_removed, user_suspended, user_banned
    resolution_notes TEXT,
    resolved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    resolved_at TIMESTAMP,

    -- Tracking
    assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
    escalated_to UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Audit
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE reports IS 'Báo cáo vi phạm từ users. Workflow: pending -> reviewing -> resolved/dismissed.';

CREATE INDEX idx_reports_reporter ON reports(reporter_id);
CREATE INDEX idx_reports_reported ON reports(reported_type, reported_id);
CREATE INDEX idx_reports_status ON reports(status) WHERE status IN ('pending', 'reviewing');
CREATE INDEX idx_reports_priority ON reports(priority, created_at) WHERE status = 'pending';

-- Wishlist
CREATE TABLE wishlists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,

    -- Notification
    notify_on_sale BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, course_id)
);

COMMENT ON TABLE wishlists IS 'Danh sách yêu thích của user.';

CREATE INDEX idx_wishlists_user ON wishlists(user_id);
CREATE INDEX idx_wishlists_course ON wishlists(course_id);

-- Cart Items
CREATE TABLE cart_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Item
    item_type VARCHAR(30) NOT NULL, -- course, module
    course_id UUID REFERENCES courses(id) ON DELETE CASCADE,
    module_id UUID REFERENCES modules(id) ON DELETE CASCADE,

    -- Price snapshot
    price_at_add DECIMAL(12, 2),
    currency VARCHAR(3) DEFAULT 'VND',

    added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (user_id, course_id),
    UNIQUE (user_id, module_id)
);

COMMENT ON TABLE cart_items IS 'Giỏ hàng của user.';

CREATE INDEX idx_cart_items_user ON cart_items(user_id);

-- =============================================================================
-- MODULE 20: TRIGGERS & FUNCTIONS
-- =============================================================================

-- Function: Auto update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_updated_at_column() IS 'Tự động cập nhật updated_at khi row được update.';

-- Apply trigger to all tables with updated_at
DO $$
DECLARE
    t text;
BEGIN
    FOR t IN
        SELECT table_name
        FROM information_schema.columns
        WHERE column_name = 'updated_at'
        AND table_schema = 'public'
    LOOP
        EXECUTE format('
            DROP TRIGGER IF EXISTS trigger_update_%I_updated_at ON %I;
            CREATE TRIGGER trigger_update_%I_updated_at
            BEFORE UPDATE ON %I
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column();
        ', t, t, t, t);
    END LOOP;
END;
$$;

-- Function: Update course statistics khi có enrollment mới
CREATE OR REPLACE FUNCTION update_course_stats_on_enrollment()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE courses
        SET total_students = total_students + 1
        WHERE id = NEW.course_id;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE courses
        SET total_students = GREATEST(total_students - 1, 0)
        WHERE id = OLD.course_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_enrollment_course_stats
AFTER INSERT OR DELETE ON enrollments
FOR EACH ROW
EXECUTE FUNCTION update_course_stats_on_enrollment();

COMMENT ON FUNCTION update_course_stats_on_enrollment() IS 'Cập nhật total_students của course khi có enrollment mới/bị xóa.';

-- Function: Update course rating khi có review mới
CREATE OR REPLACE FUNCTION update_course_rating_on_review()
RETURNS TRIGGER AS $$
DECLARE
    target_course_id UUID;
BEGIN
    target_course_id := COALESCE(NEW.course_id, OLD.course_id);

    UPDATE courses
    SET
        average_rating = (
            SELECT COALESCE(AVG(rating)::DECIMAL(2,1), 0)
            FROM reviews
            WHERE course_id = target_course_id AND is_hidden = FALSE
        ),
        total_reviews = (
            SELECT COUNT(*)
            FROM reviews
            WHERE course_id = target_course_id AND is_hidden = FALSE
        )
    WHERE id = target_course_id;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_review_course_rating
AFTER INSERT OR UPDATE OR DELETE ON reviews
FOR EACH ROW
EXECUTE FUNCTION update_course_rating_on_review();

COMMENT ON FUNCTION update_course_rating_on_review() IS 'Cập nhật average_rating và total_reviews của course khi review thay đổi.';

-- Function: Update discussion vote counts
CREATE OR REPLACE FUNCTION update_discussion_vote_counts()
RETURNS TRIGGER AS $$
DECLARE
    target_discussion_id UUID;
BEGIN
    target_discussion_id := COALESCE(NEW.discussion_id, OLD.discussion_id);

    UPDATE discussions
    SET
        upvote_count = (
            SELECT COUNT(*) FROM discussion_votes
            WHERE discussion_id = target_discussion_id AND vote_type = 'upvote'
        ),
        downvote_count = (
            SELECT COUNT(*) FROM discussion_votes
            WHERE discussion_id = target_discussion_id AND vote_type = 'downvote'
        )
    WHERE id = target_discussion_id;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_discussion_vote_counts
AFTER INSERT OR UPDATE OR DELETE ON discussion_votes
FOR EACH ROW
EXECUTE FUNCTION update_discussion_vote_counts();

-- Function: Update discussion reply count
CREATE OR REPLACE FUNCTION update_discussion_reply_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' AND NEW.parent_id IS NOT NULL THEN
        UPDATE discussions
        SET reply_count = reply_count + 1
        WHERE id = NEW.parent_id;

        -- Also update root comment
        IF NEW.root_id IS NOT NULL THEN
            UPDATE discussions
            SET reply_count = reply_count + 1
            WHERE id = NEW.root_id AND id != NEW.parent_id;
        END IF;
    ELSIF TG_OP = 'DELETE' AND OLD.parent_id IS NOT NULL THEN
        UPDATE discussions
        SET reply_count = GREATEST(reply_count - 1, 0)
        WHERE id = OLD.parent_id;

        IF OLD.root_id IS NOT NULL THEN
            UPDATE discussions
            SET reply_count = GREATEST(reply_count - 1, 0)
            WHERE id = OLD.root_id AND id != OLD.parent_id;
        END IF;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_discussion_reply_count
AFTER INSERT OR DELETE ON discussions
FOR EACH ROW
EXECUTE FUNCTION update_discussion_reply_count();

-- Function: Update module/course statistics
CREATE OR REPLACE FUNCTION update_module_stats()
RETURNS TRIGGER AS $$
DECLARE
    target_module_id UUID;
BEGIN
    target_module_id := COALESCE(NEW.module_id, OLD.module_id);

    UPDATE modules
    SET
        total_lessons = (
            SELECT COUNT(*) FROM lessons WHERE module_id = target_module_id
        ),
        total_duration_minutes = (
            SELECT COALESCE(SUM(duration_minutes), 0) FROM lessons WHERE module_id = target_module_id
        )
    WHERE id = target_module_id;

    -- Also update course
    UPDATE courses
    SET
        total_lessons = (
            SELECT COALESCE(SUM(m.total_lessons), 0)
            FROM modules m
            WHERE m.course_id = (SELECT course_id FROM modules WHERE id = target_module_id)
        ),
        total_duration_minutes = (
            SELECT COALESCE(SUM(m.total_duration_minutes), 0)
            FROM modules m
            WHERE m.course_id = (SELECT course_id FROM modules WHERE id = target_module_id)
        ),
        total_modules = (
            SELECT COUNT(*)
            FROM modules m
            WHERE m.course_id = (SELECT course_id FROM modules WHERE id = target_module_id)
        )
    WHERE id = (SELECT course_id FROM modules WHERE id = target_module_id);

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_lesson_module_stats
AFTER INSERT OR UPDATE OR DELETE ON lessons
FOR EACH ROW
EXECUTE FUNCTION update_module_stats();

-- Function: Update voucher usage count
CREATE OR REPLACE FUNCTION update_voucher_usage_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE vouchers
        SET usage_count = usage_count + 1
        WHERE id = NEW.voucher_id;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE vouchers
        SET usage_count = GREATEST(usage_count - 1, 0)
        WHERE id = OLD.voucher_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_voucher_usage_count
AFTER INSERT OR DELETE ON voucher_usages
FOR EACH ROW
EXECUTE FUNCTION update_voucher_usage_count();

-- Function: Update tag usage count
CREATE OR REPLACE FUNCTION update_tag_usage_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE tags SET usage_count = usage_count + 1 WHERE id = NEW.tag_id;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE tags SET usage_count = GREATEST(usage_count - 1, 0) WHERE id = OLD.tag_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_tag_usage_count
AFTER INSERT OR DELETE ON course_tags
FOR EACH ROW
EXECUTE FUNCTION update_tag_usage_count();

-- Function: Auto-set root_id for discussions
CREATE OR REPLACE FUNCTION set_discussion_root_id()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.parent_id IS NOT NULL THEN
        -- Find the root of the thread
        SELECT COALESCE(root_id, id) INTO NEW.root_id
        FROM discussions
        WHERE id = NEW.parent_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_set_discussion_root_id
BEFORE INSERT ON discussions
FOR EACH ROW
EXECUTE FUNCTION set_discussion_root_id();

-- =============================================================================
-- SEED DATA: Default System Roles & Permissions
-- =============================================================================

-- Insert default permissions
INSERT INTO permissions (code, name, description, category, resource_type) VALUES
-- User Management
('user.view', 'Xem user', 'Xem thông tin user', 'user_management', NULL),
('user.create', 'Tạo user', 'Tạo user mới', 'user_management', NULL),
('user.update', 'Sửa user', 'Sửa thông tin user', 'user_management', NULL),
('user.delete', 'Xóa user', 'Xóa user', 'user_management', NULL),
('user.ban', 'Ban user', 'Khóa tài khoản user', 'user_management', NULL),

-- Course Management
('course.view', 'Xem khóa học', 'Xem chi tiết khóa học', 'course_management', 'course'),
('course.create', 'Tạo khóa học', 'Tạo khóa học mới', 'course_management', NULL),
('course.update', 'Sửa khóa học', 'Sửa nội dung khóa học', 'course_management', 'course'),
('course.delete', 'Xóa khóa học', 'Xóa khóa học', 'course_management', 'course'),
('course.publish', 'Publish khóa học', 'Xuất bản khóa học', 'course_management', 'course'),
('course.review', 'Duyệt khóa học', 'Duyệt khóa học pending', 'course_management', NULL),

-- Lesson Management
('lesson.create', 'Tạo bài học', 'Tạo bài học mới', 'course_management', 'course'),
('lesson.update', 'Sửa bài học', 'Sửa nội dung bài học', 'course_management', 'course'),
('lesson.delete', 'Xóa bài học', 'Xóa bài học', 'course_management', 'course'),

-- Enrollment
('enrollment.view', 'Xem enrollment', 'Xem danh sách học viên', 'enrollment', 'course'),
('enrollment.manage', 'Quản lý enrollment', 'Thêm/xóa học viên', 'enrollment', 'course'),

-- Quiz Management
('quiz.create', 'Tạo quiz', 'Tạo quiz mới', 'quiz_management', 'course'),
('quiz.update', 'Sửa quiz', 'Sửa nội dung quiz', 'quiz_management', 'course'),
('quiz.grade', 'Chấm điểm', 'Chấm điểm essay/manual', 'quiz_management', 'course'),

-- Discussion
('discussion.moderate', 'Moderate thảo luận', 'Pin, hide, delete comments', 'discussion', 'course'),

-- Organization
('org.view', 'Xem organization', 'Xem thông tin organization', 'organization', 'organization'),
('org.manage', 'Quản lý organization', 'Quản lý settings organization', 'organization', 'organization'),
('org.members.view', 'Xem thành viên', 'Xem danh sách thành viên', 'organization', 'organization'),
('org.members.manage', 'Quản lý thành viên', 'Thêm/xóa/sửa thành viên', 'organization', 'organization'),

-- Payment
('payment.view', 'Xem payment', 'Xem lịch sử thanh toán', 'payment', NULL),
('payment.refund', 'Hoàn tiền', 'Thực hiện hoàn tiền', 'payment', NULL),

-- Reports
('report.view', 'Xem báo cáo', 'Xem báo cáo vi phạm', 'moderation', NULL),
('report.resolve', 'Xử lý báo cáo', 'Xử lý báo cáo vi phạm', 'moderation', NULL),

-- System
('system.settings', 'Cài đặt hệ thống', 'Quản lý cài đặt hệ thống', 'system', NULL),
('system.analytics', 'Xem analytics', 'Xem thống kê hệ thống', 'system', NULL);

-- Insert default roles
INSERT INTO roles (code, name, description, is_system_role, priority_level) VALUES
('student', 'Học viên', 'Người dùng đăng ký học các khóa học', TRUE, 0),
('instructor', 'Giảng viên', 'Người tạo và giảng dạy khóa học', TRUE, 10),
('ta', 'Trợ giảng', 'Hỗ trợ giảng viên trong việc quản lý khóa học', TRUE, 5),
('parent', 'Phụ huynh', 'Phụ huynh theo dõi tiến độ học tập của con', TRUE, 0),
('org_admin', 'Admin tổ chức', 'Quản trị viên của một tổ chức giáo dục', TRUE, 20),
('moderator', 'Moderator', 'Kiểm duyệt nội dung và xử lý vi phạm', TRUE, 15),
('admin', 'Admin hệ thống', 'Quản trị viên toàn hệ thống', TRUE, 100);

-- Assign permissions to roles
-- Student
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'student' AND p.code IN ('course.view');

-- Instructor
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'instructor' AND p.code IN (
    'course.view', 'course.create', 'course.update', 'course.delete', 'course.publish',
    'lesson.create', 'lesson.update', 'lesson.delete',
    'quiz.create', 'quiz.update', 'quiz.grade',
    'enrollment.view', 'discussion.moderate'
);

-- Add condition: instructors can only manage their own courses
UPDATE role_permissions
SET conditions = '{"own_resource_only": true}'
WHERE role_id = (SELECT id FROM roles WHERE code = 'instructor')
AND permission_id IN (SELECT id FROM permissions WHERE resource_type = 'course');

-- Teaching Assistant
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'ta' AND p.code IN (
    'course.view', 'enrollment.view', 'quiz.grade', 'discussion.moderate'
);

-- Parent
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'parent' AND p.code IN ('course.view');

-- Organization Admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'org_admin' AND p.code IN (
    'org.view', 'org.manage', 'org.members.view', 'org.members.manage',
    'course.view', 'course.review', 'enrollment.view', 'enrollment.manage'
);

-- Moderator
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'moderator' AND p.code IN (
    'course.view', 'discussion.moderate', 'report.view', 'report.resolve'
);

-- Admin (all permissions)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.code = 'admin';

-- =============================================================================
-- ARCHITECTURAL EXPLANATION
-- =============================================================================
/*
## GIẢI THÍCH KIẾN TRÚC DATABASE

### 1. MÔ HÌNH PHÂN QUYỀN: HYBRID RBAC + ABAC

#### Tại sao chọn Hybrid?

RBAC đơn thuần (Role-Based Access Control):
- ƯU ĐIỂM: Đơn giản, dễ hiểu, dễ quản lý
- NHƯỢC ĐIỂM: Không đủ linh hoạt cho hệ thống giáo dục phức tạp

VÍ DỤ VẤN ĐỀ:
- Teacher A có quyền edit Course X (của họ) nhưng KHÔNG có quyền edit Course Y (của teacher khác)
- TA chỉ được grade quiz trong khoảng thời gian nhất định
- Student chỉ xem được courses đã enroll

ABAC đơn thuần (Attribute-Based Access Control):
- ƯU ĐIỂM: Rất linh hoạt, xử lý được mọi trường hợp
- NHƯỢC ĐIỂM: Phức tạp, khó maintain, performance kém

GIẢI PHÁP HYBRID:
- Base permissions từ ROLES (90% use cases)
- Fine-grained control qua CONDITIONS trong role_permissions
- Override cấp user qua user_permission_overrides

#### Cấu trúc bảng:
```
permissions: Định nghĩa TẤT CẢ quyền có thể
    └── code: 'course.update'
    └── resource_type: 'course' (NULL = system-wide)

roles: Định nghĩa các vai trò
    └── is_system_role: TRUE = không xóa được
    └── organization_id: Custom role của org

role_permissions: Role có những quyền nào
    └── conditions: {"own_resource_only": true}

user_roles: User có những roles nào
    └── organization_id: Role trong scope nào

user_permission_overrides: Override quyền cho user cụ thể
    └── resource_id: Quyền chỉ cho 1 resource cụ thể
```

### 2. TRADE-OFFS ĐÃ CÂN NHẮC

#### Comments: Adjacency List vs Nested Set

CHỌN: Adjacency List (parent_id + root_id)

LÝ DO:
- Comments thường shallow (không quá 3-4 levels)
- Nhiều WRITES (thêm comment mới, reply)
- Nested Set tốt cho READ-heavy, deep hierarchies
- Adjacency List đơn giản, dễ maintain

OPTIMIZATION:
- Thêm root_id để query toàn bộ thread nhanh hơn
- Thêm reply_count để không cần COUNT() mỗi lần

#### Denormalization

CÁC TRƯỜNG DENORMALIZED:
- courses.total_students, total_reviews, average_rating
- modules.total_lessons, total_duration_minutes
- discussions.upvote_count, reply_count

LÝ DO:
- Tránh expensive JOINs và aggregations
- Data được cập nhật qua triggers để đảm bảo consistency
- Trade-off: Write slower, Read faster (phù hợp với use case)

#### JSONB vs Separate Tables

DÙNG JSONB CHO:
- conditions trong role_permissions
- test_cases trong code_exercises
- activities trong study_sessions
- metadata có cấu trúc thay đổi

KHÔNG DÙNG JSONB CHO:
- Data cần index và query thường xuyên
- Relationships giữa entities
- Data quan trọng cần integrity constraints

### 3. SCALABILITY CONSIDERATIONS

#### Indexes Strategy

- Partial indexes cho filtered queries:
  WHERE is_active = TRUE, WHERE deleted_at IS NULL
- Composite indexes cho common query patterns
- GIN indexes cho array và full-text search

#### Future Microservice Extraction

Schema được thiết kế để dễ tách module:
- User Service: users, auth tables, roles
- Course Service: courses, modules, lessons
- Payment Service: orders, transactions, vouchers
- Analytics Service: study_sessions, analytics tables

#### High Concurrency Handling

- Optimistic locking qua updated_at
- Triggers cho denormalized counts (đảm bảo consistency)
- Partial indexes giảm index size
- UUID primary keys cho distributed systems

### 4. BẢO MẬT

#### Age Verification
- Bắt buộc cho users < 18 tuổi (COPPA/GDPR compliance)
- Hỗ trợ nhiều phương thức: document, parent_consent, school

#### Sensitive Data
- password_hash: Không lưu plain password
- api_credentials_encrypted: Encrypted trong DB
- Soft delete cho audit trail

### 5. PAYMENT FLEXIBILITY

#### Module-based Installments
- Hỗ trợ mua cả course hoặc từng module
- Trả góp theo kỳ với grace period
- Voucher có thể áp dụng cho course/module/category cụ thể

#### Voucher Conditions
- Tách riêng voucher_conditions để linh hoạt
- Hỗ trợ include/exclude rules
- User segment targeting

### 6. AI INTEGRATION

#### Knowledge Extraction
- knowledge_segments: AI trích xuất concepts từ content
- Hỗ trợ search và recommendations

#### Recommendation System
- ai_recommendations: Track user interactions
- factors, confidence_score cho ML pipeline
- Feedback loop để improve model

---
Tổng số tables: 65+
Tổng số indexes: 150+
*/
