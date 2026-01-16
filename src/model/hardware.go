package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
)

// CPU (Bảng danh sách CPU)
type CPU struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name         string         `gorm:"not null;unique" json:"name"`   // "Intel i5-12400F", "AMD Ryzen 5 5600X"
	Manufacturer string         `gorm:"not null" json:"manufacturer"`  // "Intel", "AMD"
	Cores        pgtype.Int4    `json:"cores"`                         // 6, 8, 12...
	Threads      pgtype.Int4    `json:"threads"`                       // 12, 16, 24...
	BaseSpeed    pgtype.Text    `json:"base_speed"`                    // "2.5 GHz"
	BoostSpeed   pgtype.Text    `json:"boost_speed"`                   // "4.4 GHz"
	IsActive     pgtype.Bool    `gorm:"default:true" json:"is_active"` // Còn sử dụng không
	DisplayOrder pgtype.Int4    `json:"display_order"`                 // Thứ tự hiển thị
	CreatedAt    time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (CPU) TableName() string {
	return "cpus"
}

// GPU (Bảng danh sách GPU)
type GPU struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name         string         `gorm:"not null;unique" json:"name"`   // "NVIDIA RTX 3050 (6GB)", "AMD RX 6600"
	Manufacturer string         `gorm:"not null" json:"manufacturer"`  // "NVIDIA", "AMD"
	VRAM         pgtype.Text    `json:"vram"`                          // "6GB", "8GB", "12GB"
	Series       pgtype.Text    `json:"series"`                        // "RTX 30", "RTX 40", "RX 6000"
	IsActive     pgtype.Bool    `gorm:"default:true" json:"is_active"` // Còn sử dụng không
	DisplayOrder pgtype.Int4    `json:"display_order"`                 // Thứ tự hiển thị
	CreatedAt    time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (GPU) TableName() string {
	return "gpus"
}

// Monitor (Bảng danh sách màn hình)
type Monitor struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name         string         `gorm:"not null;unique" json:"name"`   // "24\" 240Hz", "27\" 165Hz IPS"
	Size         pgtype.Text    `json:"size"`                          // "24\"", "27\"", "32\""
	RefreshRate  pgtype.Text    `json:"refresh_rate"`                  // "240Hz", "165Hz", "144Hz"
	PanelType    pgtype.Text    `json:"panel_type"`                    // "IPS", "VA", "TN"
	Resolution   pgtype.Text    `json:"resolution"`                    // "1920x1080", "2560x1440"
	IsActive     pgtype.Bool    `gorm:"default:true" json:"is_active"` // Còn sử dụng không
	DisplayOrder pgtype.Int4    `json:"display_order"`                 // Thứ tự hiển thị
	CreatedAt    time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (Monitor) TableName() string {
	return "monitors"
}
