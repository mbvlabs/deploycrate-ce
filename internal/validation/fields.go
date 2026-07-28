package validation

import "strings"

func WithFieldPrefix(validationErrors ValidationErrors, prefix string) ValidationErrors {
	prefix = strings.Trim(strings.TrimSpace(prefix), ".")
	if prefix == "" {
		return validationErrors
	}

	prefixed := make(ValidationErrors, 0, len(validationErrors))
	for _, validationErr := range validationErrors {
		field := strings.Trim(validationErr.Field, ".")
		if field == "" || field == UnknownField {
			field = prefix
		} else {
			field = prefix + "." + field
		}
		validationErr.Field = field
		prefixed = append(prefixed, validationErr)
	}

	return prefixed
}
