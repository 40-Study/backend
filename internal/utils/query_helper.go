package utils

import (
	"strings"

	"gorm.io/gorm"
)

// ApplyPagination applies OFFSET and LIMIT to a query.
func ApplyPagination(query *gorm.DB, page, pageSize int) *gorm.DB {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return query.Offset(offset).Limit(pageSize)
}

// likeKeywordEscaper escape các ký tự wildcard của LIKE/ILIKE (`%`, `_`) và ký tự escape mặc
// định (`\`) để keyword người dùng nhập được khớp LITERAL — nếu không, keyword "%" khớp mọi dòng,
// "a_b" khớp cả "axb" (review-260928-users-pr72-pr28.md finding #3, chuyển từ user_repository.go
// sang đây ở QA vòng 2 G5 để mọi ô tìm kiếm dùng chung). `\` PHẢI đứng đầu vì chính nó là ký tự
// escape; NewReplacer chạy 1 lượt duy nhất qua chuỗi gốc nên không escape lặp ký tự vừa chèn.
var likeKeywordEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// EscapeLikeKeyword trả keyword đã escape cho LIKE/ILIKE (dùng với ESCAPE mặc định `\` của Postgres).
func EscapeLikeKeyword(keyword string) string {
	return likeKeywordEscaper.Replace(keyword)
}

// ContainsLikePattern trả pattern "%<keyword đã escape>%" cho điều kiện "cột ILIKE ?" kiểu
// "chứa chuỗi". Mọi truy vấn tìm theo keyword người dùng phải đi qua hàm này, không tự nối "%".
func ContainsLikePattern(keyword string) string {
	return "%" + EscapeLikeKeyword(keyword) + "%"
}

// ApplyKeywordSearch applies ILIKE search across the given columns (keyword khớp literal).
func ApplyKeywordSearch(query *gorm.DB, keyword string, columns ...string) *gorm.DB {
	if keyword == "" || len(columns) == 0 {
		return query
	}

	pattern := ContainsLikePattern(keyword)
	condition := ""
	args := make([]interface{}, len(columns))
	for i, col := range columns {
		if i > 0 {
			condition += " OR "
		}
		condition += col + " ILIKE ?"
		args[i] = pattern
	}

	return query.Where(condition, args...)
}

// ApplySoftDeleteStatus filters by soft-delete status for models using gorm.DeletedAt.
func ApplySoftDeleteStatus(query *gorm.DB, status string) *gorm.DB {
	switch status {
	case "deleted":
		return query.Unscoped().Where("deleted_at IS NOT NULL")
	case "all":
		return query.Unscoped()
	default: // "active" or empty — GORM already excludes soft-deleted
		return query
	}
}

// ApplyActiveStatus filters by is_active column for models using boolean active flag.
// tablePrefix is optional, e.g. "users" to produce "users.is_active".
func ApplyActiveStatus(query *gorm.DB, status string, tablePrefix string) *gorm.DB {
	col := "is_active"
	if tablePrefix != "" {
		col = tablePrefix + ".is_active"
	}

	switch status {
	case "inactive":
		return query.Where(col+" = ?", false)
	case "all":
		return query
	default: // "active" or empty
		return query.Where(col+" = ?", true)
	}
}

// SplitAndTrim splits a string by delimiter and trims whitespace from each part.
func SplitAndTrim(s string, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
