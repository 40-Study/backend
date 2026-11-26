package handler

import (
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"
)

// sessionListOptions (QA8 T06, C21) đọc `order` (asc mặc định | desc) và `around` (today) của GET /classes/:classId/sessions.
// Giá trị lạ là 400 ERR_VALIDATION nêu đúng tham số, không im lặng bỏ qua (web tưởng đã sắp xếp theo ý mình).
func sessionListOptions(order, around string) (repository.SessionListOptions, []utils.ValidationError) {
	var opts repository.SessionListOptions
	var errs []utils.ValidationError
	switch order {
	case "", "asc":
	case "desc":
		opts.Desc = true
	default:
		errs = append(errs, utils.ValidationError{Field: "order", Tag: "oneof", Message: "Thứ tự sắp xếp chỉ nhận asc hoặc desc."})
	}
	switch around {
	case "":
	case "today":
		opts.AroundToday = true
	default:
		errs = append(errs, utils.ValidationError{Field: "around", Tag: "oneof", Message: "Tham số around chỉ nhận today."})
	}
	return opts, errs
}
