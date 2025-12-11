package web

import (
	"strings"

	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// FormFieldData holds the data needed to render a form field
// We can't reference via.signal directly as it's unexported,
// so we define interfaces matching the actual signal methods
type FormFieldData struct {
	EnvKey       string
	ValueSignal  interface{ String() string; Bind() h.H }
	StatusSignal interface{ String() string; SetValue(any) }
}

// CreateFormFields creates signals for a set of form fields
func CreateFormFields(c *via.Context, cfg *env.EnvConfig, envKeys []string) []FormFieldData {
	fields := make([]FormFieldData, len(envKeys))
	for i, key := range envKeys {
		value := cfg.Get(key)
		// Clear placeholder values for cleaner UX - start with empty fields
		if env.IsPlaceholder(value) {
			value = ""
		}
		fields[i] = FormFieldData{
			EnvKey:       key,
			ValueSignal:  c.Signal(value),
			StatusSignal: c.Signal(""),
		}
	}
	return fields
}

// RenderFormField renders a single form field with label, input, and validation status
func RenderFormField(field FormFieldData) h.H {
	status := field.StatusSignal.String()
	isError := strings.HasPrefix(status, "error:")
	isValid := status == "valid"

	// Build input attributes
	inputAttrs := []h.H{
		h.Type("text"),
		h.Value(field.ValueSignal.String()),
		field.ValueSignal.Bind(),
	}

	// Add PicoCSS validation styling
	if isError {
		inputAttrs = append(inputAttrs, h.Attr("aria-invalid", "true")) // Red border
	} else if isValid {
		inputAttrs = append(inputAttrs, h.Attr("aria-invalid", "false")) // Green border
	}

	return h.Div(
		h.Label(h.Text(env.GetFieldLabel(field.EnvKey))),
		h.Input(inputAttrs...),
		// Error message as <small> helper text (PicoCSS styling)
		h.If(isError,
			h.Small(
				h.Style("color: var(--pico-del-color);"), // Use PicoCSS error color
				h.Text(strings.TrimPrefix(status, "error:")),
			),
		),
	)
}

// UpdateValidationStatus updates validation status signals from results
func UpdateValidationStatus(results map[string]env.ValidationResult, fields []FormFieldData, c *via.Context) {
	for i := range fields {
		result, ok := results[fields[i].EnvKey]
		if !ok {
			continue
		}

		if result.Skipped {
			fields[i].StatusSignal.SetValue("")
		} else if result.Valid {
			fields[i].StatusSignal.SetValue("valid")
		} else {
			// Set error message with "error:" prefix for conditional display
			errorMsg := "error:"
			if result.Error != nil {
				errorMsg += result.Error.Error()
			} else {
				errorMsg += "Invalid value"
			}
			fields[i].StatusSignal.SetValue(errorMsg)
		}
	}
	// Use Sync() instead of SyncSignals() to re-render the view and show validation icons/messages
	c.Sync()
}

// HasValidationErrors checks if there are any validation errors in the results
func HasValidationErrors(results map[string]env.ValidationResult, fieldUpdates map[string]string) bool {
	for key := range fieldUpdates {
		if result, exists := results[key]; exists {
			if !result.Skipped && !result.Valid {
				return true
			}
		}
	}
	return false
}

// CreateSaveAction creates a save action for form fields
func CreateSaveAction(c *via.Context, svc *env.Service, fields []FormFieldData, saveMessage interface{ String() string; SetValue(any) }) func() {
	return func() {
		// Prepare field updates map
		fieldUpdates := make(map[string]string)
		for _, field := range fields {
			fieldUpdates[field.EnvKey] = field.ValueSignal.String()
		}

		// Use service to validate and save atomically
		results, err := svc.ValidateAndUpdateFields(fieldUpdates)

		// Update validation status from results
		UpdateValidationStatus(results, fields, c)

		// Handle save result
		if err != nil {
			saveMessage.SetValue("error:" + err.Error())
		} else {
			// Check if there were validation errors
			if env.HasInvalidCredentialsMap(results) {
				saveMessage.SetValue("error:Please fix validation errors before saving")
			} else {
				saveMessage.SetValue("success:Configuration saved successfully!")
			}
		}

		// Note: UpdateValidationStatus already called c.Sync() above which re-renders
		// the entire view including the saveMessage, so no need to sync again here
	}
}

// SelectOption represents a dropdown option
type SelectOption struct {
	Value string
	Label string
}

// RenderSelectField renders a dropdown select field with label and options
func RenderSelectField(label string, selectedValue interface{ String() string; Bind() h.H }, options []SelectOption) h.H {
	// Build option elements
	optionElements := make([]h.H, len(options))
	for i, opt := range options {
		optAttrs := []h.H{
			h.Value(opt.Value),
			h.Text(opt.Label),
		}
		// Mark selected option
		if opt.Value == selectedValue.String() {
			optAttrs = append(optAttrs, h.Attr("selected", "selected"))
		}
		optionElements[i] = h.Option(optAttrs...)
	}

	// Flatten the option elements with the bind attribute
	selectChildren := []h.H{selectedValue.Bind()}
	selectChildren = append(selectChildren, optionElements...)

	return h.Div(
		h.Label(h.Text(label)),
		h.Select(selectChildren...),
	)
}

// PrerequisiteCheck holds information about a missing prerequisite
type PrerequisiteCheck struct {
	FieldKey    string // Environment variable key (e.g., env.KeyCloudflareAPIToken)
	DisplayName string // Human-readable name (e.g., "Cloudflare API Token")
	StepPath    string // Path to configure step (e.g., "/cloudflare/step1")
	StepLabel   string // Label for link (e.g., "Configure in Step 1")
}

// CheckPrerequisites validates that required fields are configured and not placeholders.
// Returns a list of missing prerequisites with navigation information.
func CheckPrerequisites(cfg *env.EnvConfig, requiredChecks []PrerequisiteCheck) []PrerequisiteCheck {
	missing := []PrerequisiteCheck{}

	for _, check := range requiredChecks {
		value := cfg.Get(check.FieldKey)
		if value == "" || env.IsPlaceholder(value) {
			missing = append(missing, check)
		}
	}

	return missing
}

// RenderPrerequisiteError renders a warning banner with clickable navigation links
// to configure missing prerequisites. Returns nil if no missing items.
func RenderPrerequisiteError(missing []PrerequisiteCheck) h.H {
	if len(missing) == 0 {
		return nil
	}

	// Build list items with clickable links
	listItems := make([]h.H, 0, len(missing)+1)
	listItems = append(listItems, h.Style("margin: 0.5rem 0 0 1.5rem;"))
	for _, item := range missing {
		listItems = append(listItems, h.Li(
			h.Text(item.DisplayName + " - "),
			h.A(
				h.Href(item.StepPath),
				h.Text(item.StepLabel),
			),
		))
	}

	return h.Article(
		h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-muted-color); padding: 1rem; margin-bottom: 1rem;"),
		h.H4(h.Text("⚠️ Missing Prerequisites")),
		h.P(h.Text("The following configuration is required before using this page:")),
		h.Ul(listItems...),
	)
}
