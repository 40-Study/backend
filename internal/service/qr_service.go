package service

import (
	"fmt"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/utils"
)

func GenerateQRPayment(req dto.QRPaymentRequest) dto.QRPaymentResponse {
	paymentCode := utils.GeneratePaymentCode()

	description := fmt.Sprintf("%s %s", req.Description, paymentCode)

	qrContent := fmt.Sprintf(
		"MB|%s|%.0f|%s",
		"0343150904",
		req.Amount,
		description,
	)

	now := time.Now()
	expiresAt := now.Add(15 * time.Minute)

	return dto.QRPaymentResponse{
		Code:        paymentCode,
		Amount:      req.Amount,
		Description: description,
		QRContent:   qrContent,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	}
}
