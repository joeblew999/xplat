package web

import (
	"fmt"

	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// homePage creates a welcome page with optional validation
func homePage(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	// Reactive signals for validation state
	validationMode := c.Signal("none") // Track validation mode: "none", "fast", "deep"
	validationInProgress := c.Signal(false)
	validationMessage := c.Signal("")

	// Fast validation action - check format only (instant)
	validateFastAction := c.Action(func() {
		validationInProgress.SetValue(true)
		validationMessage.SetValue("Checking field formats...")
		c.Sync()

		// Set to fast mode - view will regenerate with fast validation
		validationInProgress.SetValue(false)
		validationMode.SetValue("fast")
		validationMessage.SetValue("✓ Format check complete!")
		c.Sync()
	})

	// Deep validation action - verify credentials via API (slow)
	validateDeepAction := c.Action(func() {
		validationInProgress.SetValue(true)
		validationMessage.SetValue("Verifying credentials with API calls...")
		c.Sync()

		// Set to deep mode - view will regenerate with deep validation
		validationInProgress.SetValue(false)
		validationMode.SetValue("deep")
		validationMessage.SetValue("✔️ Credential verification complete!")
		c.Sync()
	})

	c.View(func() h.H {
		// Build table data based on validation mode
		// This runs every time the view is rendered
		tableRows, envPath, err := BuildConfigTableRows(mockMode, validationMode.String())

		var configTableElement h.H
		if err != nil {
			configTableElement = h.Article(
				h.Style("background-color: var(--pico-del-background); border-left: 4px solid var(--pico-del-color); padding: 1rem; margin-bottom: 1rem;"),
				h.P(
					h.Style("margin: 0; color: --pico-del-color);"),
					h.Text("Error loading configuration: "+err.Error()),
				),
			)
		} else {
			configTableElement = renderConfigTable(tableRows, envPath)
		}

		// Calculate Cloudflare progress
		cloudflareCompleted := 0
		for _, step := range CloudflareWizard.Steps {
			status := CloudflareWizard.GetStepStatus(cfg, step)
			if status == StepStatusFilled || status == StepStatusVerified {
				cloudflareCompleted++
			}
		}
		cloudflareTotalSteps := len(CloudflareWizard.Steps)

		// Determine if Claude is configured (simple check for now)
		claudeConfigured := cfg.Get(env.KeyClaudeAPIKey) != "" && !env.IsPlaceholder(cfg.Get(env.KeyClaudeAPIKey))

		// Determine if ready to deploy (Cloudflare complete)
		readyToDeploy := cloudflareCompleted == cloudflareTotalSteps

		return h.Main(
			h.Class("container"),

			// Welcome header
			h.H1(h.Text("Welcome to Your Hugo Site Manager")),
			h.P(
				h.Style("font-size: 1.1rem; color: var(--pico-muted-color); margin-bottom: 2rem;"),
				h.Text("This tool helps you configure Cloudflare Pages deployment and Claude AI translation for your Hugo site. Follow the three-phase workflow below to get started."),
			),

			// Navigation
			RenderNavigation("home"),

			// === PROGRESS DASHBOARD SECTION ===
			h.H2(h.Text("Setup Progress")),
			h.P(
				h.Style("color: var(--pico-muted-color);"),
				h.Text("Track your configuration progress across three phases:"),
			),

			// Progress Cards Grid
			h.Div(
				h.Style("display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 1rem; margin: 2rem 0;"),

				// Cloudflare Progress Card
				renderProgressCard(
					"📦 Phase 1: Cloudflare Setup",
					cloudflareCompleted,
					cloudflareTotalSteps,
					"/cloudflare",
					"Continue Cloudflare Setup",
					cloudflareCompleted == cloudflareTotalSteps,
				),

				// Claude Progress Card
				renderProgressCard(
					"🤖 Phase 2: Claude AI",
					func() int {
						if claudeConfigured {
							return 1
						}
						return 0
					}(),
					1,
					"/claude",
					"Configure Claude AI",
					claudeConfigured,
				),

				// Deploy Readiness Card
				renderProgressCard(
					"🚀 Phase 3: Deploy",
					func() int {
						if readyToDeploy {
							return 1
						}
						return 0
					}(),
					1,
					"/deploy",
					"Deploy to Production",
					readyToDeploy,
				),
			),

			// === DETAILED CONFIGURATION SECTION ===
			h.H2(
				h.Style("margin-top: 3rem;"),
				h.Text("Detailed Configuration"),
			),
			h.P(
				h.Style("color: var(--pico-muted-color);"),
				h.Text("View and validate all environment variables. This table shows the current state of your "),
				h.Code(h.Text(".env")),
				h.Text(" file."),
			),

			// Validation buttons and status message
			h.Div(
				h.Style("margin-bottom: 1rem; display: flex; align-items: center; gap: 1rem;"),
				// Fast validation button (instant format checks)
				h.Button(
					h.Text("Check Format"),
					h.Attr("class", "outline"),
					h.If(validationInProgress.String() == "true", h.Attr("aria-busy", "true")),
					h.If(validationInProgress.String() == "true", h.Attr("disabled", "disabled")),
					h.If(validationMode.String() == "fast" || validationMode.String() == "deep", h.Attr("disabled", "disabled")),
					validateFastAction.OnClick(),
				),
				// Deep validation button (API verification)
				h.Button(
					h.Text("Verify Credentials"),
					h.If(validationInProgress.String() == "true", h.Attr("aria-busy", "true")),
					h.If(validationInProgress.String() == "true", h.Attr("disabled", "disabled")),
					h.If(validationMode.String() == "deep", h.Attr("disabled", "disabled")),
					validateDeepAction.OnClick(),
				),
				// Status message
				h.If(validationMessage.String() != "",
					h.Span(
						h.Style("color: var(--pico-ins-color);"),
						h.Text(validationMessage.String()),
					),
				),
			),

			// Render table
			configTableElement,
		)
	})
}

// renderProgressCard renders a progress card for the home page dashboard
func renderProgressCard(title string, completed, total int, href, buttonText string, isComplete bool) h.H {
	percentage := float64(completed) / float64(total) * 100

	// Determine status badge
	var statusBadge h.H
	if isComplete {
		statusBadge = h.Span(
			h.Style("color: var(--pico-ins-color); font-weight: 600;"),
			h.Text("✓ Complete"),
		)
	} else {
		statusBadge = h.Span(
			h.Style("color: var(--pico-muted-color); font-weight: 600;"),
			h.Text(fmt.Sprintf("%d/%d", completed, total)),
		)
	}

	// Determine border color based on completion
	borderColor := "var(--pico-muted-border-color)"
	if isComplete {
		borderColor = "var(--pico-ins-color)"
	} else if completed > 0 {
		borderColor = "var(--pico-primary)"
	}

	return h.Article(
		h.Style("border-left: 4px solid "+borderColor+";"),
		h.H3(
			h.Style("margin: 0 0 0.5rem 0;"),
			h.Text(title),
		),

		// Progress bar
		h.Div(
			h.Style("margin: 1rem 0;"),
			h.Div(
				h.Style("background-color: var(--pico-muted-border-color); height: 0.5rem; border-radius: 0.25rem; overflow: hidden;"),
				h.Div(
					h.Style(fmt.Sprintf(
						"background-color: var(--pico-primary); height: 100%%; width: %.1f%%; transition: width 0.3s ease;",
						percentage,
					)),
				),
			),
		),

		// Status and action button
		h.Div(
			h.Style("display: flex; justify-content: space-between; align-items: center; gap: 1rem;"),
			statusBadge,
			h.A(
				h.Href(href),
				h.Attr("role", "button"),
				h.Attr("class", "outline"),
				h.Text(buttonText),
			),
		),
	)
}

// renderConfigTable renders the configuration overview table
func renderConfigTable(rows []ConfigTableRow, envPath string) h.H {
	// Build table header
	tableHeader := h.THead(
		h.Tr(
			h.Th(h.Text("Display")),
			h.Th(h.Text("Key")),
			h.Th(h.Text("Value")),
			h.Th(h.Text("Required")),
			h.Th(h.Text("Validated")),
			h.Th(h.Text("Error")),
		),
	)

	// Build table body rows
	tableBodyRows := make([]h.H, len(rows))
	for i, row := range rows {
		// Color code the validation status
		validatedStyle := ""
		if row.Validated == "✓" {
			validatedStyle = "color: var(--pico-ins-color);"
		} else if row.Validated == "✗" {
			validatedStyle = "color: var(--pico-del-color);"
		}

		// Color code the error column
		errorStyle := ""
		if row.Error != "-" {
			errorStyle = "color: var(--pico-del-color); font-size: 0.875rem;"
		}

		tableBodyRows[i] = h.Tr(
			h.Td(h.Text(row.Display)),
			h.Td(h.Code(h.Text(row.Key))),   // Monospace for env var names
			h.Td(h.Code(h.Text(row.Value))), // Monospace for values
			h.Td(h.Text(row.Required)),
			h.Td(h.Style(validatedStyle), h.Text(row.Validated)),
			h.Td(h.Style(errorStyle), h.Text(row.Error)),
		)
	}

	tableBody := h.TBody(tableBodyRows...)

	return h.Div(
		h.P(
			h.Style("margin-bottom: 1rem; color: var(--pico-muted-color);"),
			h.Text(envPath),
		),
		h.Figure(
			h.Table(
				tableHeader,
				tableBody,
			),
		),
	)
}
