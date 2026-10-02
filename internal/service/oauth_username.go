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

// pickUniqueUserName gọi gen tới khi ra tên mà exists báo chưa ai dùng, tối đa maxTries lần. Cột user_name không
// có unique index (nhiều tài khoản cũ đã trùng tên) nên không thể trông vào lỗi trùng của DB: bước kiểm tra tồn
// tại là thứ DUY NHẤT bảo đảm duy nhất. Tách thành hàm thuần (gen/exists tiêm vào) để test được nhánh va chạm và
// nhánh hết số lần thử.
func pickUniqueUserName(gen func() (string, error), exists func(string) (bool, error), maxTries int) (string, error) {
	for i := 0; i < maxTries; i++ {
		name, err := gen()
		if err != nil {
			return "", err
		}
		taken, err := exists(name)
		if err != nil {
			return "", err
		}
		if !taken {
			return name, nil
		}
	}
	return "", errOAuthUserNameExhausted
}

// uniqueOAuthUserName sinh user_name chưa ai dùng. Với hậu tố 36^6 (~2 tỷ) va chạm là hiếm; vẫn giới hạn số lần
// thử để không lặp vô hạn nếu repo lỗi. s.userNameGen chỉ để test ép va chạm (mặc định utils.NewSafeUserName).
func (s *OAuthService) uniqueOAuthUserName(ctx context.Context, fullName *string) (string, error) {
	gen := utils.NewSafeUserName
	if s.userNameGen != nil {
		gen = s.userNameGen
	}
	return pickUniqueUserName(
		func() (string, error) { return gen(fullName) },
		func(name string) (bool, error) { return s.userRepo.UserNameExists(ctx, name) },
		oauthUserNameMaxRetries,
	)
}
