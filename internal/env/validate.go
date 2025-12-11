package env

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidationMode controls how validation is performed
type ValidationMode string

const (
	// ValidationModeFast does basic format checks only (no API calls)
	ValidationModeFast ValidationMode = "fast"
	// ValidationModeDeep performs full validation including API calls
	ValidationModeDeep ValidationMode = "deep"
)

// ValidationResult holds the result of validating a credential
type ValidationResult struct {
	Name    string
	Valid   bool
	Error   error
	Skipped bool
}

// ValidateFieldFast performs fast validation (format checks only, no API calls)
func ValidateFieldFast(envKey, value string, cfg *EnvConfig) ValidationResult {
	// Get display name from envFieldsInOrder
	displayName := envKey
	requiresValidation := false
	for _, field := range envFieldsInOrder {
		if field.Key == envKey {
			displayName = field.DisplayName
			requiresValidation = field.Validate
			break
		}
	}

	// Skip placeholder values only for fields that don't require validation
	if IsPlaceholder(value) || value == "" {
		if !requiresValidation {
			return ValidationResult{
				Name:    displayName,
				Skipped: true,
			}
		}
		// For fields that require validation, treat empty/placeholder as invalid
		return ValidationResult{
			Name:  displayName,
			Valid: false,
			Error: fmt.Errorf("%s is required", displayName),
		}
	}

	// Fast validation - format checks only, no API calls
	var err error
	switch envKey {
	case KeyCloudflareAPIToken:
		// Just check it's non-empty and not a placeholder
		if len(value) < 10 {
			err = fmt.Errorf("API token appears too short")
		}
	case KeyCloudflareAPITokenName:
		// Token name is just metadata - just check it exists
		if len(value) == 0 {
			err = fmt.Errorf("token name is required")
		}
	case KeyCloudflareAccountID:
		// Account ID should be 32-character hex string
		if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(value) {
			err = fmt.Errorf("account ID must be a 32-character hexadecimal string")
		}
	case KeyCloudflareDomain:
		// Basic domain format check
		if !regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`).MatchString(value) {
			err = fmt.Errorf("invalid domain format")
		}
	case KeyCloudflareZoneID:
		// Zone ID should be 32-character hex string
		if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(value) {
			err = fmt.Errorf("zone ID must be a 32-character hexadecimal string")
		}
	case KeyCloudflarePageProject:
		// Use existing validation function (it's format-only, no API call)
		err = ValidateCloudflareProjectName(value)
	case KeyClaudeAPIKey:
		// Claude API keys start with "sk-ant-"
		if !strings.HasPrefix(value, "sk-ant-") {
			err = fmt.Errorf("Claude API key must start with 'sk-ant-'")
		}
		if len(value) < 20 {
			err = fmt.Errorf("Claude API key appears too short")
		}
	case KeyClaudeWorkspaceName:
		// Workspace name is just metadata
		if len(value) == 0 {
			err = fmt.Errorf("workspace name is required")
		}
	default:
		// Unknown field - skip validation
		return ValidationResult{
			Name:    displayName,
			Skipped: true,
		}
	}

	return ValidationResult{
		Name:  displayName,
		Valid: err == nil,
		Error: err,
	}
}

// ValidateFieldDeep performs deep validation including API calls
// This is the original ValidateField function, renamed for clarity
func ValidateFieldDeep(envKey, value string, cfg *EnvConfig, mockMode bool) ValidationResult {
	// Get display name and validation requirement from envFieldsInOrder
	displayName := envKey
	requiresValidation := false
	for _, field := range envFieldsInOrder {
		if field.Key == envKey {
			displayName = field.DisplayName
			requiresValidation = field.Validate
			break
		}
	}

	// Skip placeholder values only for fields that don't require validation
	if IsPlaceholder(value) {
		if !requiresValidation {
			return ValidationResult{
				Name:    displayName,
				Skipped: true,
			}
		}
		// For fields that require validation, treat empty/placeholder as invalid
		return ValidationResult{
			Name:  displayName,
			Valid: false,
			Error: fmt.Errorf("%s is required", displayName),
		}
	}

	// Mock mode - simple length check
	if mockMode {
		valid := len(value) > 5
		var err error
		if !valid {
			err = fmt.Errorf("%s must be longer than 5 characters (mock validation)", displayName)
		}
		return ValidationResult{
			Name:  displayName,
			Valid: valid,
			Error: err,
		}
	}

	// Real validation using env key
	var err error
	switch envKey {
	case KeyCloudflareAPIToken:
		_, err = ValidateCloudflareToken(value)
	case KeyCloudflareAPITokenName:
		// Token name is just metadata - just check it exists
		if len(value) == 0 {
			err = fmt.Errorf("token name is required")
		}
	case KeyCloudflareAccountID:
		token := cfg.Get(KeyCloudflareAPIToken)
		_, err = ValidateCloudflareAccount(token, value)
	case KeyCloudflarePageProject:
		err = ValidateCloudflareProjectName(value)
	case KeyClaudeAPIKey:
		err = ValidateClaudeAPIKey(value)
	default:
		// Unknown field - skip validation
		return ValidationResult{
			Name:    displayName,
			Skipped: true,
		}
	}

	return ValidationResult{
		Name:  displayName,
		Valid: err == nil,
		Error: err,
	}
}

// ValidateField validates a single field using deep validation (for backward compatibility)
// This maintains the original function signature so existing code continues to work
func ValidateField(envKey, value string, cfg *EnvConfig, mockMode bool) ValidationResult {
	return ValidateFieldDeep(envKey, value, cfg, mockMode)
}

// ValidateFieldWithMode validates a field using the specified validation mode
func ValidateFieldWithMode(envKey, value string, cfg *EnvConfig, mode ValidationMode, mockMode bool) ValidationResult {
	switch mode {
	case ValidationModeFast:
		return ValidateFieldFast(envKey, value, cfg)
	case ValidationModeDeep:
		return ValidateFieldDeep(envKey, value, cfg, mockMode)
	default:
		return ValidateFieldDeep(envKey, value, cfg, mockMode)
	}
}

// ValidateAllFast validates all credentials using fast validation (format checks only)
func ValidateAllFast(cfg *EnvConfig) []ValidationResult {
	results := []ValidationResult{}

	// Iterate over all fields
	for _, field := range envFieldsInOrder {
		value := cfg.Get(field.Key)
		if field.Validate {
			results = append(results, ValidateFieldFast(field.Key, value, cfg))
		} else {
			// Non-validated fields show as info only
			results = append(results, ValidationResult{
				Name:    field.DisplayName,
				Skipped: true,
			})
		}
	}

	return results
}

// ValidateAllDeep validates all credentials using deep validation (includes API calls)
func ValidateAllDeep(cfg *EnvConfig, mockMode bool) []ValidationResult {
	results := []ValidationResult{}

	// Iterate over all fields
	for _, field := range envFieldsInOrder {
		value := cfg.Get(field.Key)
		if field.Validate {
			results = append(results, ValidateFieldDeep(field.Key, value, cfg, mockMode))
		} else {
			// Non-validated fields show as info only
			results = append(results, ValidationResult{
				Name:    field.DisplayName,
				Skipped: true,
			})
		}
	}

	return results
}

// ValidateAll validates all credentials in the config using deep validation (for backward compatibility)
func ValidateAll(cfg *EnvConfig) []ValidationResult {
	return ValidateAllDeep(cfg, false)
}

// ValidateAllWithMode validates all credentials with optional mock mode
func ValidateAllWithMode(cfg *EnvConfig, mockMode bool) []ValidationResult {
	results := []ValidationResult{}

	// Iterate over all fields (show all, not just validated ones)
	for _, field := range envFieldsInOrder {
		value := cfg.Get(field.Key)
		// Always include the field in results, even if validation is skipped
		if field.Validate {
			results = append(results, ValidateField(field.Key, value, cfg, mockMode))
		} else {
			// Non-validated fields show as info only
			results = append(results, ValidationResult{
				Name:    field.DisplayName,
				Skipped: true,
			})
		}
	}

	return results
}

// HasInvalidCredentials returns true if any credentials are invalid
func HasInvalidCredentials(results []ValidationResult) bool {
	for _, result := range results {
		if !result.Skipped && !result.Valid {
			return true
		}
	}
	return false
}

// GetInvalidFields returns the names of invalid fields
func GetInvalidFields(results []ValidationResult) []string {
	fields := []string{}
	for _, result := range results {
		if !result.Skipped && !result.Valid {
			fields = append(fields, result.Name)
		}
	}
	return fields
}

// HasInvalidCredentialsMap returns true if any credentials are invalid in a map
func HasInvalidCredentialsMap(results map[string]ValidationResult) bool {
	for _, result := range results {
		if !result.Skipped && !result.Valid {
			return true
		}
	}
	return false
}
