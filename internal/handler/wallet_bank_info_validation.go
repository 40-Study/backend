package handler

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"study.com/v1/internal/dto"
	"study.com/v1/internal/utils"
)

// QA8 T04 (C20): tài khoản nhận tiền của giảng viên dùng làm thông tin chuyển khoản khi rút nên phải đúng ngay lúc lưu.
// Giới hạn độ dài theo cột của teacher_profiles (bank_name varchar(100), bank_account_name varchar(255)); số tài khoản
// chỉ gồm chữ số, 6 đến 20 ký tự. Lỗi nêu đúng trường (`field`) bằng tiếng Việt.
const (
	bankNameMaxRunes        = 100
	bankAccountNameMaxRunes = 255
)

var bankAccountNumberPattern = regexp.MustCompile(`^[0-9]{6,20}$`)

// normalizeBankInfo cắt khoảng trắng đầu/cuối ba trường (giá trị lưu là giá trị đã cắt).
func normalizeBankInfo(req *dto.UpdateBankInfoRequest) {
	req.BankName = strings.TrimSpace(req.BankName)
	req.BankAccountNumber = strings.TrimSpace(req.BankAccountNumber)
	req.BankAccountName = strings.TrimSpace(req.BankAccountName)
}

// bankInfoErrors kiểm ba trường (đã qua normalizeBankInfo); rỗng nghĩa là hợp lệ.
func bankInfoErrors(req dto.UpdateBankInfoRequest) []utils.ValidationError {
	var errs []utils.ValidationError
	add := func(field, tag, msg string) {
		errs = append(errs, utils.ValidationError{Field: field, Tag: tag, Message: msg})
	}
	switch n := utf8.RuneCountInString(req.BankName); {
	case n == 0:
		add("bank_name", "required", "Vui lòng nhập tên ngân hàng.")
	case n > bankNameMaxRunes:
		add("bank_name", "max", "Tên ngân hàng tối đa 100 ký tự.")
	}
	if req.BankAccountNumber == "" {
		add("bank_account_number", "required", "Vui lòng nhập số tài khoản.")
	} else if !bankAccountNumberPattern.MatchString(req.BankAccountNumber) {
		add("bank_account_number", "format", "Số tài khoản chỉ gồm chữ số, từ 6 đến 20 chữ số.")
	}
	switch n := utf8.RuneCountInString(req.BankAccountName); {
	case n == 0:
		add("bank_account_name", "required", "Vui lòng nhập tên chủ tài khoản.")
	case n > bankAccountNameMaxRunes:
		add("bank_account_name", "max", "Tên chủ tài khoản tối đa 255 ký tự.")
	}
	return errs
}
