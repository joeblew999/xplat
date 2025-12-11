package env

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const envFile = ".env"
const envFileTest = ".env.test"

// currentEnvFile holds the active env file path (can be changed for testing)
var currentEnvFile = envFile

// SetEnvFileForTesting sets a custom env file path for testing
func SetEnvFileForTesting(path string) {
	currentEnvFile = path
}

// ResetEnvFile resets to the default .env file
func ResetEnvFile() {
	currentEnvFile = envFile
}

// GetTestEnvFile returns the test env file path
func GetTestEnvFile() string {
	return envFileTest
}

// Environment variable keys used throughout the codebase
const (
	KeyCloudflareAPIToken     = "CLOUDFLARE_API_TOKEN"
	KeyCloudflareAPITokenName = "CLOUDFLARE_API_TOKEN_NAME"
	KeyCloudflareAccountID    = "CLOUDFLARE_ACCOUNT_ID"
	KeyCloudflareDomain       = "CLOUDFLARE_DOMAIN"
	KeyCloudflareZoneID       = "CLOUDFLARE_ZONE_ID"
	KeyCloudflarePageProject  = "CLOUDFLARE_PAGE_PROJECT_NAME"
	KeyClaudeAPIKey           = "CLAUDE_API_KEY"
	KeyClaudeWorkspaceName    = "CLAUDE_WORKSPACE_NAME"
)

// Placeholder values used in .env.example and validation
const (
	PlaceholderToken = "your-token-here"
	PlaceholderKey   = "your-api-key-here"
)

// EnvConfig holds environment configuration
type EnvConfig struct {
	CloudflareToken     string
	CloudflareTokenName string
	CloudflareAccount   string
	CloudflareDomain    string
	CloudflareZoneID    string
	CloudflareProject   string
	ClaudeAPIKey        string
	ClaudeWorkspace     string
}

// FieldInfo holds metadata about an environment variable field
type FieldInfo struct {
	Key          string
	Default      string
	Description  string // Inline comment describing the field
	DisplayName  string // Human-readable label for web GUI
	SyncToGitHub bool   // Should sync to GitHub secrets (for CI/CD deployment)
	Validate     bool   // Should validate the value before GitHub sync
}

// envFieldsInOrder defines all env vars with their metadata in display order
var envFieldsInOrder = []FieldInfo{
	{Key: KeyCloudflareAPIToken, Default: "your-token-here", Description: "Cloudflare API token (required for deployment)", DisplayName: "Cloudflare API Token", SyncToGitHub: true, Validate: true},
	{Key: KeyCloudflareAPITokenName, Default: "your-token-name", Description: "Cloudflare token name (helps you remember which token)", DisplayName: "Cloudflare API Token Name", SyncToGitHub: false, Validate: true},
	{Key: KeyCloudflareAccountID, Default: "your-account-id", Description: "Cloudflare Account ID", DisplayName: "Cloudflare Account ID", SyncToGitHub: true, Validate: true},
	{Key: KeyCloudflareDomain, Default: "your-domain.com", Description: "Cloudflare domain name for Hugo site", DisplayName: "Cloudflare Domain", SyncToGitHub: true, Validate: false},
	{Key: KeyCloudflareZoneID, Default: "your-zone-id", Description: "Cloudflare Zone ID for the domain", DisplayName: "Cloudflare Zone ID", SyncToGitHub: true, Validate: false},
	{Key: KeyCloudflarePageProject, Default: "your-project-name", Description: "Cloudflare Pages project name", DisplayName: "Cloudflare Pages Project", SyncToGitHub: true, Validate: true},
	{Key: KeyClaudeAPIKey, Default: "your-api-key-here", Description: "Claude API key (required for translation)", DisplayName: "Claude API Key", SyncToGitHub: false, Validate: true},
	{Key: KeyClaudeWorkspaceName, Default: "", Description: "Claude workspace name", DisplayName: "Claude Workspace Name", SyncToGitHub: false, Validate: true},
}

// GetDisplayName returns the display name for a given environment variable key
func GetDisplayName(key string) string {
	for _, field := range envFieldsInOrder {
		if field.Key == key {
			return field.DisplayName
		}
	}
	return key // Fallback to key if not found
}

// GetFieldInfo returns the complete FieldInfo for a given environment variable key
func GetFieldInfo(key string) *FieldInfo {
	for _, field := range envFieldsInOrder {
		if field.Key == key {
			return &field
		}
	}
	return nil
}

// GetAllFieldsInOrder returns all field metadata in display order
func GetAllFieldsInOrder() []FieldInfo {
	// Return a copy to prevent modification
	result := make([]FieldInfo, len(envFieldsInOrder))
	copy(result, envFieldsInOrder)
	return result
}

// GetKeyFromDisplayName returns the env key for a given display name
func GetKeyFromDisplayName(displayName string) string {
	for _, field := range envFieldsInOrder {
		if field.DisplayName == displayName {
			return field.Key
		}
	}
	return displayName // Fallback to display name if not found
}

// GetFieldLabel returns the display name with optional " *" suffix for mandatory fields
// Used by web GUI to show which fields are required
func GetFieldLabel(key string) string {
	field := GetFieldInfo(key)
	if field == nil {
		return key
	}

	label := field.DisplayName
	if field.Validate {
		label += " *"
	}
	return label
}

// Get returns the value of a field by env key
func (cfg *EnvConfig) Get(key string) string {
	switch key {
	case KeyCloudflareAPIToken:
		return cfg.CloudflareToken
	case KeyCloudflareAPITokenName:
		return cfg.CloudflareTokenName
	case KeyCloudflareAccountID:
		return cfg.CloudflareAccount
	case KeyCloudflareDomain:
		return cfg.CloudflareDomain
	case KeyCloudflareZoneID:
		return cfg.CloudflareZoneID
	case KeyCloudflarePageProject:
		return cfg.CloudflareProject
	case KeyClaudeAPIKey:
		return cfg.ClaudeAPIKey
	case KeyClaudeWorkspaceName:
		return cfg.ClaudeWorkspace
	}
	return ""
}

// Set sets the value of a field by env key
func (cfg *EnvConfig) Set(key, value string) bool {
	switch key {
	case KeyCloudflareAPIToken:
		cfg.CloudflareToken = value
	case KeyCloudflareAPITokenName:
		cfg.CloudflareTokenName = value
	case KeyCloudflareAccountID:
		cfg.CloudflareAccount = value
	case KeyCloudflareDomain:
		cfg.CloudflareDomain = value
	case KeyCloudflareZoneID:
		cfg.CloudflareZoneID = value
	case KeyCloudflarePageProject:
		cfg.CloudflareProject = value
	case KeyClaudeAPIKey:
		cfg.ClaudeAPIKey = value
	case KeyClaudeWorkspaceName:
		cfg.ClaudeWorkspace = value
	default:
		return false
	}
	return true
}

// parseEnvLine parses a key=value line from .env file
// Returns (key, value, ok) where ok is true if line is valid
func parseEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)

	// Skip empty lines and comments
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}

	// Split on first =
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	// Strip inline comments (e.g., "value  # Updated: timestamp")
	if idx := strings.Index(value, "#"); idx != -1 {
		value = strings.TrimSpace(value[:idx])
	}

	return key, value, true
}

// LoadEnv reads the .env file and returns the configuration
func LoadEnv() (*EnvConfig, error) {
	cfg := &EnvConfig{}

	file, err := os.Open(currentEnvFile)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Return empty config if file doesn't exist
		}
		return nil, fmt.Errorf("failed to open .env: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseEnvLine(scanner.Text())
		if !ok {
			continue
		}
		cfg.Set(key, value)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .env: %w", err)
	}

	return cfg, nil
}

// CreateEnv creates a new .env file with default values
func CreateEnv() error {
	cfg := &EnvConfig{}
	return WriteEnv(cfg)
}

// UpdateEnv updates a specific key in the .env file
func UpdateEnv(key, value string) error {
	cfg, err := LoadEnv()
	if err != nil {
		return err
	}

	// Update field by env key
	if !cfg.Set(key, value) {
		return fmt.Errorf("unknown environment key: %s", key)
	}

	// Write back the entire file
	return WriteEnv(cfg)
}

// UpdateEnvPartial updates only the non-empty fields from the provided config
// This preserves all other fields in the .env file
func UpdateEnvPartial(updates *EnvConfig) error {
	// Load current config
	current, err := LoadEnv()
	if err != nil {
		return err
	}

	// Apply updates for each field that has a value
	for _, field := range envFieldsInOrder {
		updateValue := updates.Get(field.Key)
		if updateValue != "" && !IsPlaceholder(updateValue) {
			current.Set(field.Key, updateValue)
		}
	}

	// Write back the entire file
	return WriteEnv(current)
}

// writeEnvHeader writes the .env file header
func writeEnvHeader(b *strings.Builder) {
	b.WriteString("# Environment Configuration\n")
	b.WriteString("# DO NOT commit this file to git\n")
}

// writeEnvLine writes a key=value line with optional inline comment
func writeEnvLine(b *strings.Builder, key, value, description string) {
	b.WriteString(key)
	b.WriteString("=")
	b.WriteString(value)

	// Add inline description comment
	if description != "" {
		b.WriteString("  # ")
		b.WriteString(description)
	}

	b.WriteString("\n")
}

// WriteEnv writes the complete configuration to .env
func WriteEnv(cfg *EnvConfig) error {
	var content strings.Builder

	writeEnvHeader(&content)
	content.WriteString("\n")

	for _, field := range envFieldsInOrder {
		// Get field value or default
		value := cfg.Get(field.Key)
		if value == "" {
			value = field.Default
		}

		// Write the key=value line with description
		writeEnvLine(&content, field.Key, value, field.Description)
	}

	if err := os.WriteFile(currentEnvFile, []byte(content.String()), 0600); err != nil {
		return fmt.Errorf("failed to write .env: %w", err)
	}

	return nil
}

// EnvExists checks if .env file exists
func EnvExists() bool {
	_, err := os.Stat(currentEnvFile)
	return err == nil
}

// GetEnvPath returns the absolute path to the .env file
func GetEnvPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", wd, currentEnvFile), nil
}

// IsPlaceholder checks if a value is a placeholder/default value
func IsPlaceholder(value string) bool {
	return value == "" || strings.HasPrefix(value, "your-") || strings.HasPrefix(value, "your_")
}

// Service provides a unified interface for config operations used by both CLI and web
type Service struct {
	mockMode bool
}

// NewService creates a new config service
func NewService(mockMode bool) *Service {
	return &Service{mockMode: mockMode}
}

// GetCurrentConfig safely loads the current configuration from disk
func (s *Service) GetCurrentConfig() (*EnvConfig, error) {
	return LoadEnv()
}

// ValidateConfig validates all fields in the provided config using deep validation
// This is the default validation method (includes API calls)
func (s *Service) ValidateConfig(cfg *EnvConfig) map[string]ValidationResult {
	results := make(map[string]ValidationResult)

	for _, field := range envFieldsInOrder {
		value := cfg.Get(field.Key)
		result := ValidateField(field.Key, value, cfg, s.mockMode)
		results[field.Key] = result
	}

	return results
}

// ValidateConfigFast validates all fields using fast validation (format checks only, no API calls)
func (s *Service) ValidateConfigFast(cfg *EnvConfig) map[string]ValidationResult {
	results := make(map[string]ValidationResult)

	for _, field := range envFieldsInOrder {
		value := cfg.Get(field.Key)
		result := ValidateFieldFast(field.Key, value, cfg)
		results[field.Key] = result
	}

	return results
}

// ValidateConfigDeep validates all fields using deep validation (includes API calls)
// This is an explicit alias for ValidateConfig for clarity
func (s *Service) ValidateConfigDeep(cfg *EnvConfig) map[string]ValidationResult {
	return s.ValidateConfig(cfg)
}

// ValidateConfigWithMode validates all fields using the specified validation mode
func (s *Service) ValidateConfigWithMode(cfg *EnvConfig, mode ValidationMode) map[string]ValidationResult {
	results := make(map[string]ValidationResult)

	for _, field := range envFieldsInOrder {
		value := cfg.Get(field.Key)
		result := ValidateFieldWithMode(field.Key, value, cfg, mode, s.mockMode)
		results[field.Key] = result
	}

	return results
}

// ValidateAndUpdateFields validates and atomically updates the specified fields
// This always reloads from disk before saving to prevent stale data issues
func (s *Service) ValidateAndUpdateFields(fieldUpdates map[string]string) (map[string]ValidationResult, error) {
	// Load current config from disk to preserve existing values for validation dependencies
	updateCfg, err := s.GetCurrentConfig()
	if err != nil {
		// If config doesn't exist yet, start with empty config
		updateCfg = &EnvConfig{}
	}

	// Apply updates to the current config
	for key, value := range fieldUpdates {
		updateCfg.Set(key, value)
	}

	// Validate the complete config with all dependencies available
	results := s.ValidateConfig(updateCfg)

	// Check if all required validations passed for fields being updated
	allValid := true
	for key := range fieldUpdates {
		result, exists := results[key]
		if !exists {
			continue // Field not in results (shouldn't happen, but skip if it does)
		}
		if !result.Skipped && !result.Valid {
			allValid = false
			break
		}
	}

	if !allValid {
		return results, nil // Return validation errors, no save
	}

	// All valid - perform atomic update
	// UpdateEnvPartial reloads from disk, so no stale data issues
	if err := UpdateEnvPartial(updateCfg); err != nil {
		return results, err
	}

	return results, nil
}

// UpdateFields updates the specified fields without validation
// Useful when you've already validated or for non-validated fields
func (s *Service) UpdateFields(fieldUpdates map[string]string) error {
	updateCfg := &EnvConfig{}
	for key, value := range fieldUpdates {
		updateCfg.Set(key, value)
	}

	return UpdateEnvPartial(updateCfg)
}

// ResultsToSlice converts map results to slice format for legacy functions
// This maintains field order from envFieldsInOrder
func ResultsToSlice(resultsMap map[string]ValidationResult) []ValidationResult {
	results := make([]ValidationResult, 0, len(resultsMap))
	for _, field := range envFieldsInOrder {
		if result, ok := resultsMap[field.Key]; ok {
			results = append(results, result)
		}
	}
	return results
}
