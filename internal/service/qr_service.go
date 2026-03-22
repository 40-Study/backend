package service

import (
	"context"
	"fmt"
	"time"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/utils"
)

type QrServiceInterface interface {
	GenerateQrUrl(ctx context.Context, req dto.QRPaymentRequest, qrType string) (dto.QRPaymentResponse, error)
}

type QrService struct {
}

func NewQrService() *QrService {
	return &QrService{}
}

func (s *QrService) GenerateQrUrl(ctx context.Context, req dto.QRPaymentRequest, qrType string) (dto.QRPaymentResponse, error) {
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
	}, nil
}
