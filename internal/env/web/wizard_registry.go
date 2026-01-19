package web

import (
	"github.com/joeblew999/xplat/internal/env"
)

// WizardStepInfo defines metadata for a single wizard step
// Each step file exports its own WizardStepInfo which serves as the single source of truth
type WizardStepInfo struct {
	// Metadata
	StepNumber  int
	Path        string // e.g., "/cloudflare/step1"
	Title       string // e.g., "API Token"
	Description string // e.g., "Configure your Cloudflare API token"

	// Fields managed by this step
	Fields []string // env.Key* constants

	// Prerequisites (fields required from previous steps)
	Prerequisites []PrerequisiteCheck
}

// WizardRegistry holds all wizard steps in order
type WizardRegistry struct {
	Name  string            // e.g., "cloudflare"
	Steps []*WizardStepInfo // Pointers to step metadata from individual files
}

// GetStep returns a step by number (1-indexed)
func (r *WizardRegistry) GetStep(stepNumber int) *WizardStepInfo {
	if stepNumber < 1 || stepNumber > len(r.Steps) {
		return nil
	}
	return r.Steps[stepNumber-1]
}

// GetStepByPath returns a step by path
func (r *WizardRegistry) GetStepByPath(path string) *WizardStepInfo {
	for _, step := range r.Steps {
		if step.Path == path {
			return step
		}
	}
	return nil
}

// GetNextStep returns the next step, or nil if at the end
func (r *WizardRegistry) GetNextStep(currentStep int) *WizardStepInfo {
	return r.GetStep(currentStep + 1)
}

// GetPrevStep returns the previous step, or nil if at the beginning
func (r *WizardRegistry) GetPrevStep(currentStep int) *WizardStepInfo {
	return r.GetStep(currentStep - 1)
}

// StepStatus represents the completion status of a step
type StepStatus string

const (
	StepStatusVerified   StepStatus = "verified"   // Fields filled AND API-verified (deep validation)
	StepStatusFilled     StepStatus = "filled"     // All fields filled in .env (fast check only)
	StepStatusIncomplete StepStatus = "incomplete" // Fields missing or invalid
)

// GetStepStatus calculates completion status for a step using FAST validation (no API calls)
// Only checks if fields exist in .env - suitable for landing page display
// Returns: StepStatusFilled if all fields exist, StepStatusIncomplete otherwise
// Note: This does NOT perform API verification - use GetStepStatusDeep for that
func (r *WizardRegistry) GetStepStatus(cfg *env.EnvConfig, step *WizardStepInfo) StepStatus {
	return r.GetStepStatusWithMode(cfg, step, env.ValidationModeFast, false)
}

// GetStepStatusDeep calculates completion status using DEEP validation (includes API calls)
// Verifies fields are not only filled but also valid via API calls
// Returns: StepStatusVerified if all fields are API-verified, StepStatusFilled if only filled, StepStatusIncomplete if missing/invalid
func (r *WizardRegistry) GetStepStatusDeep(cfg *env.EnvConfig, step *WizardStepInfo, mockMode bool) StepStatus {
	return r.GetStepStatusWithMode(cfg, step, env.ValidationModeDeep, mockMode)
}

// GetStepStatusWithMode calculates step status using the specified validation mode
func (r *WizardRegistry) GetStepStatusWithMode(cfg *env.EnvConfig, step *WizardStepInfo, mode env.ValidationMode, mockMode bool) StepStatus {
	// Check prerequisites first using existing CheckPrerequisites helper
	if len(step.Prerequisites) > 0 {
		missing := CheckPrerequisites(cfg, step.Prerequisites)
		if len(missing) > 0 {
			return StepStatusIncomplete
		}
	}

	// Check if step's own fields are filled (not empty and not placeholders)
	for _, fieldKey := range step.Fields {
		value := cfg.Get(fieldKey)
		if value == "" || env.IsPlaceholder(value) {
			return StepStatusIncomplete
		}
	}

	// If using fast mode, return filled (we already checked fields exist above)
	if mode == env.ValidationModeFast {
		return StepStatusFilled
	}

	// Deep mode - validate each field via API
	allValid := true
	for _, fieldKey := range step.Fields {
		value := cfg.Get(fieldKey)
		result := env.ValidateFieldDeep(fieldKey, value, cfg, mockMode)

		// If any field is invalid or skipped, not fully verified
		if !result.Valid || result.Skipped {
			allValid = false
			break
		}
	}

	if allValid {
		return StepStatusVerified
	}

	// Fields are filled but not all API-verified - return filled status
	return StepStatusFilled
}

// CloudflareWizard is the global registry for the Cloudflare setup wizard
// This will be populated with step metadata after all step files are loaded
var CloudflareWizard = WizardRegistry{
	Name:  "cloudflare",
	Steps: nil, // Will be populated in init() after step metadata is defined
}

// init populates the CloudflareWizard registry with pointers to step metadata
// This runs after all package-level variables are initialized
func init() {
	CloudflareWizard.Steps = []*WizardStepInfo{
		&Step1Info,
		&Step2Info,
		&Step3Info,
		&Step4Info,
		&Step5Info,
		&Step6Info,
	}
}
