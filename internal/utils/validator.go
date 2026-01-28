package utils

import (
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
}

// ValidationError represents a single validation error
type ValidationError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

// ValidateStruct validates a struct and returns a slice of validation errors
func ValidateStruct(s interface{}) []ValidationError {
	var errors []ValidationError

	err := validate.Struct(s)
	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			var element ValidationError
			element.Field = toSnakeCase(err.Field())
			element.Tag = err.Tag()
			element.Message = getErrorMessage(err)
			errors = append(errors, element)
		}
	}

	return errors
}

// getErrorMessage returns a human-readable error message for a validation error
func getErrorMessage(err validator.FieldError) string {
	field := toSnakeCase(err.Field())

	switch err.Tag() {
	case "required":
		return field + " is required"
	case "email":
		return field + " must be a valid email address"
	case "min":
		return field + " must be at least " + err.Param() + " characters"
	case "max":
		return field + " must be at most " + err.Param() + " characters"
	case "len":
		return field + " must be exactly " + err.Param() + " characters"
	case "uuid":
		return field + " must be a valid UUID"
	case "e164":
		return field + " must be a valid phone number in E.164 format"
	case "alphanum":
		return field + " must contain only alphanumeric characters"
	case "numeric":
		return field + " must contain only numeric characters"
	case "oneof":
		return field + " must be one of: " + err.Param()
	case "datetime":
		return field + " must be a valid date in format " + err.Param()
	case "eqfield":
		return field + " must match " + toSnakeCase(err.Param())
	case "nefield":
		return field + " must be different from " + toSnakeCase(err.Param())
	case "containsany":
		return field + " must contain at least one special character"
	default:
		return field + " is invalid"
	}
}

// toSnakeCase converts a string from PascalCase/camelCase to snake_case
func toSnakeCase(str string) string {
	var result strings.Builder
	for i, r := range str {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}
