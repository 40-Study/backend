package utils

import (
	"net/url"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()

	// Register custom URL validation
	validate.RegisterValidation("safe_url", validateSafeURL)
}

// validateSafeURL validates that a URL is safe (http/https only, no javascript/data URIs)
func validateSafeURL(fl validator.FieldLevel) bool {
	urlStr := fl.Field().String()
	if urlStr == "" {
		return true // Empty is valid, use "required" tag for mandatory
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Only allow http and https schemes
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}

	// Must have a host
	if parsedURL.Host == "" {
		return false
	}

	return true
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
	case "safe_url":
		return field + " must be a valid URL (http or https)"
	case "url":
		return field + " must be a valid URL"
	default:
		return field + " is invalid"
	}
}

// toSnakeCase converts a string from PascalCase/camelCase to snake_case
// Handles consecutive uppercase (e.g., "InstructorID" -> "instructor_id", "HTTPServer" -> "http_server")
func toSnakeCase(str string) string {
	var result strings.Builder
	runes := []rune(str)
	for i, r := range runes {
		if i > 0 && r >= 'A' && r <= 'Z' {
			// Add underscore only if:
			// - Previous char is lowercase, OR
			// - Next char exists and is lowercase (handles "HTTPServer" -> "http_server")
			prevLower := runes[i-1] >= 'a' && runes[i-1] <= 'z'
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if prevLower || nextLower {
				result.WriteRune('_')
			}
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func NormalizeEmail(email string) string {
	return strings.TrimSpace(strings.ToLower(email))
}

func NormalizePhone(phone string) string {
	return strings.TrimSpace(phone)
}

func NompareEmails(email1, email2 string) bool {
	return strings.EqualFold(NormalizeEmail(email1), NormalizeEmail(email2))
}
