package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// Branch (Bảng cơ sở/chi nhánh)
type Branch struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code        string      `gorm:"unique;not null" json:"code"`   // Mã cơ sở: "HN01", "HCM01"
	Name        string      `gorm:"not null" json:"name"`          // Tên: "Hà Nội - Trần Duy Hưng"
	Address     string      `gorm:"not null" json:"address"`       // Địa chỉ đầy đủ
	City        string      `gorm:"not null" json:"city"`          // Thành phố: "Hà Nội", "TP HCM"
	District    pgtype.Text `json:"district"`                      // Quận/Huyện
	PhoneNumber pgtype.Text `json:"phone_number"`                  // Số điện thoại cơ sở
	Email       pgtype.Text `json:"email"`                         // Email liên hệ
	ImageUrl    pgtype.Text `json:"image_url"`                     // Hình ảnh cơ sở
	OpenTime    pgtype.Text `json:"open_time"`                     // Giờ mở cửa: "8:00"
	CloseTime   pgtype.Text `json:"close_time"`                    // Giờ đóng cửa: "23:00"
	IsActive    pgtype.Bool `gorm:"default:true" json:"is_active"` // Đang hoạt động
	CreatedAt   time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	BranchZones []BranchZone `gorm:"foreignKey:BranchID;constraint:OnDelete:CASCADE" json:"branch_zones,omitempty"`
}

func (Branch) TableName() string {
	return "branches"
}

// Zone (Bảng khu vực - Định danh loại khu vực: VIP, Hút thuốc, Couple...)
type Zone struct {
	ID               uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code             string      `gorm:"unique;not null" json:"code"`   // Mã loại khu vực: VIP, SMOKING, COUPLE
	Name             string      `gorm:"not null" json:"name"`          // Tên: "VIP", "Khu hút thuốc", "Couple"
	ShortDescription pgtype.Text `json:"short_description"`             // Mô tả ngắn
	FullDescription  pgtype.Text `json:"full_description"`              // Mô tả đầy đủ
	ImageUrl         pgtype.Text `json:"image_url"`                     // URL hình ảnh
	OpenRule         pgtype.Text `json:"open_rule"`                     // Quy định mở cửa
	IsActive         pgtype.Bool `gorm:"default:true" json:"is_active"` // Trạng thái hoạt động
	CreatedAt        time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	BranchZones    []BranchZone        `gorm:"foreignKey:ZoneID;constraint:OnDelete:CASCADE" json:"branch_zones,omitempty"`
	Specifications []ZoneSpecification `gorm:"foreignKey:ZoneID;constraint:OnDelete:CASCADE" json:"specifications,omitempty"`
	Prices         []ZonePrice         `gorm:"foreignKey:ZoneID;constraint:OnDelete:CASCADE" json:"prices,omitempty"`
	Notes          []ZoneNote          `gorm:"foreignKey:ZoneID;constraint:OnDelete:CASCADE" json:"notes,omitempty"`
	DeviceZones    []DeviceZone        `gorm:"foreignKey:ZoneID;constraint:OnDelete:CASCADE" json:"device_zones,omitempty"`
}

func (Zone) TableName() string {
	return "zones"
}

// ZoneSpecification (Bảng thông số khu vực - Key-Value flexible)
type ZoneSpecification struct {
	ID        uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ZoneID    uuid.UUID   `gorm:"type:uuid;not null;index:idx_zone_spec" json:"zone_id"`
	SpecKey   string      `gorm:"not null;index:idx_zone_spec" json:"spec_key"` // "monitor", "gpu", "cpu", "ram", "has_ac"
	SpecValue pgtype.Text `gorm:"not null" json:"spec_value"`                   // "27 inch", "RTX 4090", "true"
	SpecType  string      `gorm:"not null;default:'text'" json:"spec_type"`     // "text", "boolean", "number"
	CreatedAt time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	Zone Zone `gorm:"foreignKey:ZoneID" json:"zone,omitempty"`
}

func (ZoneSpecification) TableName() string {
	return "zone_specifications"
}

// ZonePrice (Bảng giá khu vực)
type ZonePrice struct {
	ID        uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ZoneID    uuid.UUID   `gorm:"type:uuid;not null;index" json:"zone_id"`
	PriceType string      `gorm:"not null" json:"price_type"` // "hourly", "daily", "weekend", "peak_hour", "member"
	Price     pgtype.Int4 `gorm:"not null" json:"price"`      // Giá theo loại (VNĐ): 15000/giờ
	CreatedAt time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	Zone Zone `gorm:"foreignKey:ZoneID" json:"zone,omitempty"`
}

func (ZonePrice) TableName() string {
	return "zone_prices"
}

// ZoneNote (Bảng ghi chú khu vực)
type ZoneNote struct {
	ID        uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ZoneID    uuid.UUID   `gorm:"type:uuid;not null;index" json:"zone_id"`
	NoteType  string      `gorm:"not null" json:"note_type"` // "rule", "warning", "info", "amenity"
	Content   pgtype.Text `gorm:"not null" json:"content"`   // "Không hút thuốc", "Yêu cầu đặt trước"
	CreatedAt time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	Zone Zone `gorm:"foreignKey:ZoneID" json:"zone,omitempty"`
}

func (ZoneNote) TableName() string {
	return "zone_notes"
}

// BranchZone (Bảng trung gian Branch - Zone)
type BranchZone struct {
	ID         uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	BranchID   uuid.UUID   `gorm:"type:uuid;not null;index:idx_branch_zone" json:"branch_id"`
	ZoneID     uuid.UUID   `gorm:"type:uuid;not null;index:idx_branch_zone" json:"zone_id"`
	TotalSeats pgtype.Int4 `json:"total_seats"` // Tổng số chỗ của zone này ở branch này
	IsActive   pgtype.Bool `gorm:"default:true" json:"is_active"`
	CreatedAt  time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	Branch Branch `gorm:"foreignKey:BranchID" json:"branch,omitempty"`
	Zone   Zone   `gorm:"foreignKey:ZoneID" json:"zone,omitempty"`
}

func (BranchZone) TableName() string {
	return "branch_zones"
}

// Device (Bảng thiết bị/máy tính)
type Device struct {
	ID        uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code      string      `gorm:"unique;not null" json:"code"`       // Mã máy: "PC-001"
	Name      string      `gorm:"not null" json:"name"`              // Tên: "Máy 1"
	CPUID     uuid.UUID   `gorm:"type:uuid;index" json:"cpu_id"`     // FK -> cpus
	GPUID     uuid.UUID   `gorm:"type:uuid;index" json:"gpu_id"`     // FK -> gpus
	MonitorID uuid.UUID   `gorm:"type:uuid;index" json:"monitor_id"` // FK -> monitors
	RAM       pgtype.Int4 `json:"ram"`                               // GB RAM: 8, 16, 32
	Storage   pgtype.Text `json:"storage"`                           // "512GB SSD", "1TB NVMe"
	Mouse     pgtype.Text `json:"mouse"`                             // "Logitech G102"
	Keyboard  pgtype.Text `json:"keyboard"`                          // "Akko 3068"
	Headset   pgtype.Text `json:"headset"`                           // "HyperX Cloud II"
	Status    string      `gorm:"default:'available'" json:"status"` // "available", "in_use", "maintenance", "broken"
	IsActive  pgtype.Bool `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	CPU         CPU          `gorm:"foreignKey:CPUID" json:"cpu,omitempty"`
	GPU         GPU          `gorm:"foreignKey:GPUID" json:"gpu,omitempty"`
	Monitor     Monitor      `gorm:"foreignKey:MonitorID" json:"monitor,omitempty"`
	DeviceZones []DeviceZone `gorm:"foreignKey:DeviceID;constraint:OnDelete:CASCADE" json:"device_zones,omitempty"`
}

func (Device) TableName() string {
	return "devices"
}

// DeviceZone (Bảng trung gian Device - Zone - Branch)
type DeviceZone struct {
	ID        uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DeviceID  uuid.UUID   `gorm:"type:uuid;not null;index:idx_device_zone" json:"device_id"`
	ZoneID    uuid.UUID   `gorm:"type:uuid;not null;index:idx_device_zone" json:"zone_id"`
	BranchID  uuid.UUID   `gorm:"type:uuid;not null;index" json:"branch_id"` // Device thuộc zone nào tại branch nào
	IsActive  pgtype.Bool `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	Device Device `gorm:"foreignKey:DeviceID" json:"device,omitempty"`
	Zone   Zone   `gorm:"foreignKey:ZoneID" json:"zone,omitempty"`
	Branch Branch `gorm:"foreignKey:BranchID" json:"branch,omitempty"`
}

func (DeviceZone) TableName() string {
	return "device_zones"
}

// Address (Bảng địa chỉ - Dùng cho giao hàng, địa chỉ khách hàng)
type Address struct {
	ID     uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID *uuid.UUID `gorm:"type:uuid;index" json:"user_id"` // Địa chỉ của user nào (nullable cho guest)

	// Thông tin người nhận
	RecipientName  string `gorm:"type:varchar(255);not null" json:"recipient_name"` // Tên người nhận
	RecipientPhone string `gorm:"type:varchar(20);not null" json:"recipient_phone"` // SĐT người nhận

	// Địa chỉ phân cấp theo hành chính VN
	Province string      `gorm:"type:varchar(100);not null;index" json:"province"` // Tỉnh/Thành phố: "Hà Nội", "TP HCM"
	District string      `gorm:"type:varchar(100);not null;index" json:"district"` // Quận/Huyện: "Cầu Giấy", "Quận 1"
	Ward     pgtype.Text `json:"ward"`                                             // Phường/Xã: "Dịch Vọng Hậu"

	// Địa chỉ chi tiết
	Street      string      `gorm:"type:varchar(255);not null" json:"street"` // Số nhà, tên đường: "123 Trần Duy Hưng"
	FullAddress string      `gorm:"type:text;not null" json:"full_address"`   // Địa chỉ đầy đủ (ghép tất cả)
	AddressNote pgtype.Text `json:"address_note"`                             // Ghi chú: "Gần cổng A", "Nhà màu xanh"

	// Tọa độ GPS (optional, cho tính năng bản đồ)
	Latitude  *decimal.Decimal `gorm:"type:numeric(10,8)" json:"latitude"`  // Vĩ độ
	Longitude *decimal.Decimal `gorm:"type:numeric(11,8)" json:"longitude"` // Kinh độ

	// Loại địa chỉ
	AddressType string      `gorm:"type:varchar(20);default:'other'" json:"address_type"` // home, office, other
	IsDefault   pgtype.Bool `gorm:"default:false" json:"is_default"`                      // Địa chỉ mặc định

	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relations
	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (Address) TableName() string {
	return "addresses"
}

// Address type constants
const (
	AddressTypeHome   = "home"
	AddressTypeOffice = "office"
	AddressTypeOther  = "other"
)
