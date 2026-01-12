package web

import (
	"fmt"

	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// cloudflarePage - Landing page showing all wizard steps with status
func cloudflarePage(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	// Reactive signals for validation state
	validationMode := c.Signal("fast") // Track validation mode: "fast", "deep"
	validationInProgress := c.Signal(false)
	validationMessage := c.Signal("")

	// Deep validation action - verify all steps via API
	verifyAllAction := c.Action(func() {
		validationInProgress.SetValue(true)
		validationMessage.SetValue("Verifying all credentials with API calls...")
		c.Sync()

		// Set to deep mode - view will regenerate with deep validation
		validationInProgress.SetValue(false)
		validationMode.SetValue("deep")
		validationMessage.SetValue("✔️ Credential verification complete!")
		c.Sync()
	})

	c.View(func() h.H {
		// Determine which validation mode to use based on signal
		useDeepValidation := validationMode.String() == "deep"

		// Calculate overall progress using appropriate validation mode
		completedSteps := 0
		for _, step := range CloudflareWizard.Steps {
			var status StepStatus
			if useDeepValidation {
				status = CloudflareWizard.GetStepStatusDeep(cfg, step, mockMode)
			} else {
				status = CloudflareWizard.GetStepStatus(cfg, step)
			}
			// Count both filled and verified as complete for progress calculation
			if status == StepStatusFilled || status == StepStatusVerified {
				completedSteps++
			}
		}

		totalSteps := len(CloudflareWizard.Steps)

		// Find first incomplete step for "Continue Setup" button
		var firstIncompleteStep *WizardStepInfo
		for _, step := range CloudflareWizard.Steps {
			var status StepStatus
			if useDeepValidation {
				status = CloudflareWizard.GetStepStatusDeep(cfg, step, mockMode)
			} else {
				status = CloudflareWizard.GetStepStatus(cfg, step)
			}
			if status == StepStatusIncomplete {
				firstIncompleteStep = step
				break
			}
		}

		// Build step cards using appropriate validation mode
		stepCards := make([]h.H, 0, totalSteps)
		for _, step := range CloudflareWizard.Steps {
			var status StepStatus
			if useDeepValidation {
				status = CloudflareWizard.GetStepStatusDeep(cfg, step, mockMode)
			} else {
				status = CloudflareWizard.GetStepStatus(cfg, step)
			}
			stepCards = append(stepCards, RenderStepCard(step, status))
		}

		return h.Main(
			h.Class("container"),
			h.H1(h.Text("Cloudflare Pages Setup Wizard")),
			h.P(
				h.Style("font-size: 1.1rem; color: var(--pico-muted-color);"),
				h.Text("Configure your Cloudflare Pages deployment in 5 simple steps"),
			),

			RenderNavigation("cloudflare"),

			// Progress bar
			RenderStepProgress(completedSteps, totalSteps),

			// Breadcrumb navigation
			RenderWizardBreadcrumbs(&CloudflareWizard, cfg, 0), // 0 = no current step (landing page)

			// Overview section
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-primary); padding: 1rem; margin-bottom: 2rem;"),
				h.H3(h.Text("📋 Setup Overview")),
				h.P(h.Text("This wizard will guide you through configuring Cloudflare Pages for deploying your Hugo site:")),
				h.Ol(
					h.Style("margin: 0.5rem 0 0 1.5rem;"),
					h.Li(h.Text("Create and configure a Cloudflare API token with Pages permissions")),
					h.Li(h.Text("Enter your Cloudflare account ID")),
					h.Li(h.Text("Select the domain for your Hugo site")),
					h.Li(h.Text("Choose or create a Cloudflare Pages project")),
					h.Li(h.Text("Attach your custom domain to the Pages project")),
				),
				h.P(
					h.Style("margin-top: 1rem;"),
					h.Text("Each step validates your configuration and saves it to your "),
					h.Code(h.Text(".env")),
					h.Text(" file. You can skip steps if you've already configured them, but prerequisite checks will ensure you complete steps in order."),
				),
			),

			// Quick start buttons and validation
			h.Div(
				h.Style("margin-bottom: 2rem; display: flex; flex-wrap: wrap; gap: 1rem; align-items: center;"),
				func() h.H {
					if firstIncompleteStep != nil {
						buttonText := "🚀 Start Setup"
						if completedSteps > 0 {
							buttonText = fmt.Sprintf("▶️ Continue Setup (Step %d)", firstIncompleteStep.StepNumber)
						}
						return h.A(
							h.Href(firstIncompleteStep.Path),
							h.Attr("role", "button"),
							h.Text(buttonText),
						)
					}
					if completedSteps == totalSteps {
						return h.A(
							h.Href("/deploy"),
							h.Attr("role", "button"),
							h.Attr("class", "contrast"),
							h.Text("✅ Setup Complete - Go to Deploy"),
						)
					}
					return h.Text("")
				}(),
				h.A(
					h.Href(Step1Info.Path),
					h.Attr("role", "button"),
					h.Attr("class", "secondary outline"),
					h.Text("Start from Step 1"),
				),
				// Verify All button - only show if we have some filled steps
				h.If(completedSteps > 0,
					h.Button(
						h.Text("🔍 Verify All Credentials"),
						h.Attr("class", "outline"),
						h.If(validationInProgress.String() == "true", h.Attr("aria-busy", "true")),
						h.If(validationInProgress.String() == "true", h.Attr("disabled", "disabled")),
						h.If(validationMode.String() == "deep", h.Attr("disabled", "disabled")),
						verifyAllAction.OnClick(),
					),
				),
				// Status message
				h.If(validationMessage.String() != "",
					h.Span(
						h.Style("color: var(--pico-ins-color); font-weight: 600;"),
						h.Text(validationMessage.String()),
					),
				),
			),

			// Step cards
			h.H2(h.Text("Setup Steps")),
			func() h.H {
				elements := []h.H{h.Style("display: grid; gap: 1rem;")}
				elements = append(elements, stepCards...)
				return h.Div(elements...)
			}(),

			// Help section
			h.Article(
				h.Style("margin-top: 2rem; background-color: var(--pico-card-background-color); padding: 1rem;"),
				h.H4(h.Text("💡 Need Help?")),
				h.Ul(
					h.Style("margin: 0.5rem 0 0 1.5rem;"),
					h.Li(
						h.Text("Each step includes detailed instructions and links to Cloudflare documentation"),
					),
					h.Li(
						h.Text("You can navigate between steps using the breadcrumb navigation at the top of each page"),
					),
					h.Li(
						h.Text("Status indicators show which steps are complete (✓) and which need attention (○)"),
					),
					h.Li(
						h.Text("Prerequisites are checked automatically - you'll see warnings if required fields are missing"),
					),
				),
			),
		)
	})
}
