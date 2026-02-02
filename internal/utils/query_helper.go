package utils

import "gorm.io/gorm"

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

// ApplyKeywordSearch applies ILIKE search across the given columns.
func ApplyKeywordSearch(query *gorm.DB, keyword string, columns ...string) *gorm.DB {
	if keyword == "" || len(columns) == 0 {
		return query
	}

	condition := ""
	args := make([]interface{}, len(columns))
	for i, col := range columns {
		if i > 0 {
			condition += " OR "
		}
		condition += col + " ILIKE ?"
		args[i] = "%" + keyword + "%"
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
