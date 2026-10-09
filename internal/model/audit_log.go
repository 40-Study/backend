package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Mã hành động quản trị được ghi vào nhật ký (contract C2, plan D6). SSOT là slice AuditActions
// bên dưới: endpoint GET /admin/audit-logs/actions và nhãn tiếng Việt ở web đều đọc từ đó.
const (
	AuditActionUserLock                  = "user.lock"
	AuditActionUserUnlock                = "user.unlock"
	AuditActionUserRoleAssign            = "user.role_assign"
	AuditActionUserRoleRevoke            = "user.role_revoke"
	AuditActionSystemRoleCreate          = "system_role.create"
	AuditActionSystemRoleUpdate          = "system_role.update"
	AuditActionSystemRoleDelete          = "system_role.delete"
	AuditActionSystemRoleRestore         = "system_role.restore"
	AuditActionSystemRolePermissions     = "system_role.permissions_change"
	AuditActionPermissionUpdate          = "permission.update"
	AuditActionCourseApprove             = "course.approve"
	AuditActionCourseReject              = "course.reject"
	AuditActionTeacherApplicationApprove = "teacher_application.approve"
	AuditActionTeacherApplicationReject  = "teacher_application.reject"
	AuditActionReportStatusUpdate        = "report.status_update"
	AuditActionReportDelete              = "report.delete"
	AuditActionOrderRefund               = "order.refund"
	AuditActionOrderLateRefund           = "order.late_refund"
	AuditActionWithdrawalApprove         = "withdrawal.approve"
	AuditActionWithdrawalReject          = "withdrawal.reject"
	AuditActionWithdrawalMarkCompleted   = "withdrawal.mark_completed"
	AuditActionSettingPlatformFeeUpdate  = "setting.platform_fee_update"
	AuditActionNotificationBroadcast     = "notification.broadcast"
)

// AuditActions: toàn bộ mã hành động hợp lệ, theo thứ tự nhóm trong bảng của phase 8.
var AuditActions = []string{
	AuditActionUserLock, AuditActionUserUnlock, AuditActionUserRoleAssign, AuditActionUserRoleRevoke,
	AuditActionSystemRoleCreate, AuditActionSystemRoleUpdate, AuditActionSystemRoleDelete,
	AuditActionSystemRoleRestore, AuditActionSystemRolePermissions,
	AuditActionPermissionUpdate,
	AuditActionCourseApprove, AuditActionCourseReject,
	AuditActionTeacherApplicationApprove, AuditActionTeacherApplicationReject,
	AuditActionReportStatusUpdate, AuditActionReportDelete,
	AuditActionOrderRefund, AuditActionOrderLateRefund,
	AuditActionWithdrawalApprove, AuditActionWithdrawalReject, AuditActionWithdrawalMarkCompleted,
	AuditActionSettingPlatformFeeUpdate,
	AuditActionNotificationBroadcast,
}

// AuditLog là một dòng nhật ký hoạt động quản trị. Chỉ-thêm (append-only): không có đường update
// hay delete. Tên + email người thực hiện KHÔNG lưu ở đây mà JOIN từ users khi đọc (không có cột
// dẫn xuất); cũng không khai báo FK tới users để lịch sử không bị ràng buộc bởi vòng đời tài khoản.
type AuditLog struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CreatedAt  time.Time      `gorm:"not null;index:idx_audit_logs_created_at,sort:desc;index:idx_audit_logs_actor_created,priority:2,sort:desc"`
	ActorID    uuid.UUID      `gorm:"type:uuid;not null;index:idx_audit_logs_actor_created,priority:1"`
	Action     string         `gorm:"type:varchar(60);not null;index"`
	TargetType string         `gorm:"type:varchar(40);not null;default:'';index:idx_audit_logs_target,priority:1"`
	TargetID   *string        `gorm:"type:varchar(64);index:idx_audit_logs_target,priority:2"`
	StatusCode int            `gorm:"not null"`
	IP         *string        `gorm:"type:varchar(45)"`
	Metadata   datatypes.JSON `gorm:"type:jsonb"`
}

func (AuditLog) TableName() string { return "audit_logs" }

// AuditEntry là dữ liệu middleware/handler gửi cho recorder; service làm sạch Metadata rồi mới
// chuyển thành AuditLog. TargetID rỗng = không có đối tượng đích; IP rỗng = không xác định.
type AuditEntry struct {
	ActorID    uuid.UUID
	Action     string
	TargetType string
	TargetID   string
	StatusCode int
	IP         string
	Metadata   map[string]any
}
