package service

// oauth_username.go — user_name duy nhất cho tài khoản OAuth không có username riêng (Google).
// Cách sinh tên nằm ở utils.NewSafeUserName (dùng chung với migration đổi tên cũ).

import (
	"context"
	"errors"

	"study.com/v1/internal/utils"
)

const oauthUserNameMaxRetries = 8

var errOAuthUserNameExhausted = errors.New("oauth: không sinh được user_name duy nhất")

// uniqueOAuthUserName sinh user_name chưa ai dùng. Cột user_name không có unique index (nhiều tài khoản
// cũ đã trùng tên) nên không thể trông vào lỗi trùng của DB: phải kiểm tra tồn tại. Với hậu tố 36^6
// (~2 tỷ) thử lại là hiếm; vẫn giới hạn số lần để không lặp vô hạn nếu repo lỗi.
func (s *OAuthService) uniqueOAuthUserName(ctx context.Context, fullName *string) (string, error) {
	for i := 0; i < oauthUserNameMaxRetries; i++ {
		name, err := utils.NewSafeUserName(fullName)
		if err != nil {
			return "", err
		}
		taken, err := s.userRepo.UserNameExists(ctx, name)
		if err != nil {
			return "", err
		}
		if !taken {
			return name, nil
		}
	}
	return "", errOAuthUserNameExhausted
}
