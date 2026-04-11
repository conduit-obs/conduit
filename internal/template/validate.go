package template

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ValidationError represents a single parameter validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult holds the results of parameter validation.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// ValidateParameters checks parameter values against their definitions.
func ValidateParameters(params []ParameterDef, values map[string]any) *ValidationResult {
	result := &ValidationResult{Valid: true}

	for _, p := range params {
		val, exists := values[p.Name]

		// Check required
		if p.Required && !exists {
			result.addError(p.Name, msgOrDefault(p.ErrorMessage, "required parameter is missing"))
			continue
		}

		if !exists {
			continue
		}

		// Type validation
		switch p.Type {
		case "string":
			s, ok := val.(string)
			if !ok {
				result.addError(p.Name, "expected string value")
				continue
			}
			validateString(result, p, s)

		case "number":
			n, ok := toFloat64(val)
			if !ok {
				result.addError(p.Name, "expected number value")
				continue
			}
			validateNumber(result, p, n)

		case "boolean":
			if _, ok := val.(bool); !ok {
				result.addError(p.Name, "expected boolean value")
			}

		case "array":
			switch val.(type) {
			case []any, []string:
				// valid array type
			default:
				result.addError(p.Name, "expected array value")
			}

		default:
			// Unknown type — skip validation
		}
	}

	return result
}

func validateString(result *ValidationResult, p ParameterDef, s string) {
	if p.Validation == nil {
		return
	}
	v := p.Validation

	if v.MinLength != nil && len(s) < *v.MinLength {
		result.addError(p.Name, msgOrDefault(p.ErrorMessage, fmt.Sprintf("must be at least %d characters", *v.MinLength)))
	}
	if v.MaxLength != nil && len(s) > *v.MaxLength {
		result.addError(p.Name, msgOrDefault(p.ErrorMessage, fmt.Sprintf("must be at most %d characters", *v.MaxLength)))
	}
	if v.Pattern != "" {
		re, err := regexp.Compile(v.Pattern)
		if err == nil && !re.MatchString(s) {
			result.addError(p.Name, msgOrDefault(p.ErrorMessage, fmt.Sprintf("must match pattern %q", v.Pattern)))
		}
	}
	if len(v.Enum) > 0 {
		found := false
		for _, e := range v.Enum {
			if strings.EqualFold(s, e) {
				found = true
				break
			}
		}
		if !found {
			result.addError(p.Name, msgOrDefault(p.ErrorMessage, fmt.Sprintf("must be one of: %s", strings.Join(v.Enum, ", "))))
		}
	}
}

func validateNumber(result *ValidationResult, p ParameterDef, n float64) {
	if p.Validation == nil {
		return
	}
	v := p.Validation

	if v.Minimum != nil && n < *v.Minimum {
		result.addError(p.Name, msgOrDefault(p.ErrorMessage, fmt.Sprintf("must be >= %v", *v.Minimum)))
	}
	if v.Maximum != nil && n > *v.Maximum {
		result.addError(p.Name, msgOrDefault(p.ErrorMessage, fmt.Sprintf("must be <= %v", *v.Maximum)))
	}
}

func (r *ValidationResult) addError(field, message string) {
	r.Valid = false
	r.Errors = append(r.Errors, ValidationError{Field: field, Message: message})
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func msgOrDefault(custom, fallback string) string {
	if custom != "" {
		return custom
	}
	return fallback
}
