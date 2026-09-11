package model

import (
	"time"

	"github.com/google/uuid"
)

// BankTransactionUsage (M-06, audit 260909 vòng 2): ghi lại MỖI giao dịch chuyển khoản ngân
// hàng (theo TransactionID do gRPC transaction service trả về qua CheckTransaction) đã được
// dùng để xác nhận thanh toán cho ĐƠN HÀNG khóa học HOẶC MUA XU — chống replay: một giao dịch
// chuyển khoản thật (khớp payment code + số tiền) không thể được dùng để xác nhận 2 đơn hàng
// hoặc 2 lượt mua xu khác nhau (cố ý gửi lại request xác nhận, hoặc trùng payment code do va
// chạm ngẫu nhiên). Dùng CHUNG 1 bảng cho cả 2 luồng (order + coin purchase) vì cùng 1 khái
// niệm "giao dịch ngân hàng này đã tiêu thụ chưa" — nếu tách 2 bảng riêng thì 1 giao dịch ngân
// hàng vẫn có thể bị dùng lần 2 ở luồng còn lại.
//
// Unique index trên BankTransactionID là chốt chặn Ở TẦNG DB (không phải check-rồi-ghi ở tầng
// app, vốn vẫn có thể race giữa 2 request xác nhận song song cho CÙNG giao dịch ngân hàng):
// request thắng cuộc INSERT thành công, request thua bị lỗi unique violation
// (gorm.ErrDuplicatedKey — xem TranslateError: true trong postgres.go) và phải rollback toàn
// bộ transaction xử lý thanh toán của nó.
type BankTransactionUsage struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatedAt         time.Time `json:"created_at"`
	BankTransactionID string    `gorm:"type:varchar(255);uniqueIndex:idx_bank_txn_usage_unique;not null" json:"bank_transaction_id"`
	// ReferenceType: "order" | "coin_purchase" — loại thực thể đã tiêu thụ giao dịch này.
	ReferenceType string    `gorm:"type:varchar(30);not null" json:"reference_type"`
	ReferenceID   uuid.UUID `gorm:"type:uuid;not null;index" json:"reference_id"`
}

func (BankTransactionUsage) TableName() string {
	return "bank_transaction_usages"
}
