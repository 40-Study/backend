package dto

import "time"

type QRPaymentRequest struct {
	Amount      float64
	Description string
	UserID      string
	OrderID     string
}

type QRPaymentResponse struct {
	Code        string    `json:"code"`
	Amount      float64   `json:"amount"`
	Description string    `json:"description"`
	QRContent   string    `json:"qr_content"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}
