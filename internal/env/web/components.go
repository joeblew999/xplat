package web

import (
	"strings"

	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// RenderNavigation renders the shared navigation menu
// currentPage: "home", "cloudflare", "claude", or "deploy"
func RenderNavigation(currentPage string) h.H {
	// Helper to render a nav item (link or bold text)
	navItem := func(page, label, href string) h.H {
		if currentPage == page {
			return h.Li(h.Strong(h.Text(label)))
		}
		return h.Li(h.A(h.Href(href), h.Text(label)))
	}

	return h.Nav(
		h.Ul(
			navItem("home", "Overview", "/"),
			navItem("cloudflare", "Cloudflare", "/cloudflare"),
			navItem("claude", "Claude AI", "/claude"),
			navItem("deploy", "Deploy", "/deploy"),
		),
	)
}

// BuildCloudflareURL builds a Cloudflare dashboard URL, replacing :account placeholder with actual account ID when available
func BuildCloudflareURL(baseURL, accountID string) string {
	if accountID != "" && !env.IsPlaceholder(accountID) {
		return strings.Replace(baseURL, ":account", accountID, 1)
	}
	return baseURL
}

// LazyLoader handles one-time loading of expensive data in Via pages.
// Use this for API calls or expensive operations that should only run
// when a page is actually visited, not during Via's startup validation.
//
// Via validates all page init functions at startup by running them,
// which can trigger unwanted API calls. LazyLoader defers the expensive
// operation until Get() is called inside c.View().
type LazyLoader[T any] struct {
	data   T
	loaded bool
	loader func() (T, error)
}

// NewLazyLoader creates a new LazyLoader with the given loader function.
// The loader function will be called exactly once, the first time Get() is called.
func NewLazyLoader[T any](loader func() (T, error)) *LazyLoader[T] {
	return &LazyLoader[T]{loader: loader}
}

// Get returns the loaded data, calling the loader function if this is the first call.
// Subsequent calls return the cached data without calling the loader again.
func (l *LazyLoader[T]) Get() (T, error) {
	if !l.loaded {
		data, err := l.loader()
		if err != nil {
			var zero T
			return zero, err
		}
		l.data = data
		l.loaded = true
	}
	return l.data, nil
}

// Reload forces the loader to run again, refreshing the cached data.
// Use this when you need to update data without recreating the LazyLoader.
func (l *LazyLoader[T]) Reload() (T, error) {
	data, err := l.loader()
	if err != nil {
		var zero T
		return zero, err
	}
	l.data = data
	l.loaded = true
	return l.data, nil
}

// ConfigTableRow represents a row in the configuration overview table
type ConfigTableRow struct {
	Display   string // Human-readable display name
	Key       string // Environment variable key name
	Value     string // The actual value (formatted/masked)
	Required  string // "Yes" or "-"
	Validated string // "✓", "✗", "-"
	Error     string // Error message or "-"
}

// BuildConfigTableRows builds the configuration overview table data
// validationMode: "none" (no validation), "fast" (format checks only), "deep" (API calls)
func BuildConfigTableRows(mockMode bool, validationMode string) ([]ConfigTableRow, string, error) {
	svc := env.NewService(mockMode)

	// Get current config
	cfg, err := svc.GetCurrentConfig()
	if err != nil {
		return nil, "", err
	}

	// Get env file path
	envPath, err := env.GetEnvPath()
	if err != nil {
		envPath = ".env" // fallback
	}

	var webRows []ConfigTableRow

	if validationMode == "none" {
		// No validation: Build rows without validation - just show config values
		allFields := env.GetAllFieldsInOrder()
		webRows = make([]ConfigTableRow, 0, len(allFields))

		for _, field := range allFields {
			// Determine "Required" status
			required := "-"
			if field.SyncToGitHub {
				required = "Yes"
			}

			// Get display value (formatted/masked)
			value := cfg.Get(field.Key)
			displayValue := formatValueForDisplay(value)

			webRows = append(webRows, ConfigTableRow{
				Display:   field.DisplayName,
				Key:       field.Key,
				Value:     displayValue,
				Required:  required,
				Validated: "-", // No validation performed
				Error:     "-", // No validation performed
			})
		}
	} else {
		// Validation requested: Use fast or deep mode
		var validationResults []env.ValidationResult

		if validationMode == "fast" {
			// Fast validation: Format checks only, no API calls
			validationResults = env.ValidateAllFast(cfg)
		} else {
			// Deep validation: Full validation including API calls (default)
			validationResults = env.ValidateAllWithMode(cfg, mockMode)
		}

		// Build table rows from validation results
		webRows = make([]ConfigTableRow, 0, len(validationResults))
		for _, result := range validationResults {
			// Get the key name from the display name (result.Name is DisplayName)
			keyName := env.GetKeyFromDisplayName(result.Name)
			fieldInfo := env.GetFieldInfo(keyName)

			// Determine "Required" status
			required := "-"
			if fieldInfo != nil && fieldInfo.SyncToGitHub {
				required = "Yes"
			}

			// Determine "Validated" status
			validated := "-"
			if !result.Skipped {
				if result.Valid {
					validated = "✓"
				} else {
					validated = "✗"
				}
			}

			// Get error message
			errorMsg := "-"
			if result.Error != nil {
				errorMsg = result.Error.Error()
			}

			// Get display value (formatted/masked)
			value := cfg.Get(keyName)
			displayValue := formatValueForDisplay(value)

			webRows = append(webRows, ConfigTableRow{
				Display:   result.Name,
				Key:       keyName,
				Value:     displayValue,
				Required:  required,
				Validated: validated,
				Error:     errorMsg,
			})
		}
	}

	return webRows, envPath, nil
}

// formatValueForDisplay formats a value for display (masks sensitive data)
func formatValueForDisplay(value string) string {
	if env.IsPlaceholder(value) {
		return "(not set)"
	}
	// Show preview for secrets
	if len(value) > 24 {
		preview := value[:10] + "..." + value[len(value)-10:]
		return preview
	}
	return value
}

// RenderURLLink renders a clickable URL link with an icon
func RenderURLLink(url, label, icon string) h.H {
	if url == "" {
		return h.Text("")
	}

	return h.Div(
		h.Style("margin: 1rem 0; padding: 1rem; background-color: var(--pico-card-background-color); border-radius: 0.5rem;"),
		h.P(
			h.Style("margin: 0; display: flex; align-items: center; gap: 0.5rem;"),
			h.Span(
				h.Style("font-size: 1.5rem;"),
				h.Text(icon),
			),
			h.Strong(h.Text(label+": ")),
			h.A(
				h.Href(url),
				h.Attr("target", "_blank"),
				h.Attr("rel", "noopener noreferrer"),
				h.Style("color: var(--pico-primary);"),
				h.Text(url),
			),
		),
	)
}

// RenderErrorMessage renders an error message if status starts with "error:"
// Accepts any type with a String() method (including via signals)
func RenderErrorMessage(message interface{ String() string }) h.H {
	return h.If(strings.HasPrefix(message.String(), "error:"),
		h.Article(
			h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-del-color); padding: 1rem; margin-top: 1rem;"),
			h.P(
				h.Style("margin: 0; color: var(--pico-del-color);"),
				h.Text(strings.TrimPrefix(message.String(), "error:")),
			),
		),
	)
}

// RenderSuccessMessage renders a success message if status starts with "success:"
// Accepts any type with a String() method (including via signals)
func RenderSuccessMessage(message interface{ String() string }) h.H {
	return h.If(strings.HasPrefix(message.String(), "success:"),
		h.Article(
			h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-ins-color); padding: 1rem; margin-top: 1rem;"),
			h.P(
				h.Style("margin: 0; color: var(--pico-ins-color);"),
				h.Text(strings.TrimPrefix(message.String(), "success:")),
			),
		),
	)
}

// RenderExternalLink renders a consistent external link with "Visit: [Label] ↗" pattern
// Includes security best practices (target="_blank" with rel="noopener noreferrer")
//
// Example usage:
//
//	h.Li(RenderExternalLink(env.CloudflareAPITokensURL, "Cloudflare API Tokens"))
//
// Renders: "Visit: Cloudflare API Tokens ↗" as a clickable external link
func RenderExternalLink(url, label string) h.H {
	return h.Span(
		h.Text("Visit: "),
		h.A(
			h.Href(url),
			h.Attr("target", "_blank"),
			h.Attr("rel", "noopener noreferrer"),
			h.Text(label+" ↗"),
		),
	)
}

// RenderExternalLinkWithCustomPrefix renders an external link with custom prefix text
// Use this for special cases where "Visit:" doesn't fit the context
//
// Example usage:
//
//	h.Li(RenderExternalLinkWithCustomPrefix(env.AnthropicBillingURL, "Add billing information at: ", "Billing Settings"))
//
// Renders: "Add billing information at: Billing Settings ↗" as a clickable external link
func RenderExternalLinkWithCustomPrefix(url, prefix, label string) h.H {
	return h.Span(
		h.Text(prefix),
		h.A(
			h.Href(url),
			h.Attr("target", "_blank"),
			h.Attr("rel", "noopener noreferrer"),
			h.Text(label+" ↗"),
		),
	)
}
