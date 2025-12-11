package web

import (
	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// Step2Info is the metadata for Step 2 - single source of truth
var Step2Info = WizardStepInfo{
	StepNumber:  2,
	Path:        "/cloudflare/step2",
	Title:       "Account ID",
	Description: "Enter your Cloudflare account ID",
	Fields:      []string{env.KeyCloudflareAccountID},
	Prerequisites: []PrerequisiteCheck{
		{
			FieldKey:    env.KeyCloudflareAPIToken,
			DisplayName: "Cloudflare API Token",
			StepPath:    "/cloudflare/step1",
			StepLabel:   "Configure in Step 1",
		},
	},
}

// cloudflareStep2Page - Account ID setup (Step 2 of 5)
func cloudflareStep2Page(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	svc := env.NewService(mockMode)

	// Create fields for token (needed for validation) and account ID
	fields := CreateFormFields(c, cfg, []string{
		env.KeyCloudflareAPIToken,
		env.KeyCloudflareAccountID,
	})

	saveMessage := c.Signal("")

	// Next action - validate and go to step 3
	nextAction := c.Action(func() {
		saveMessage.SetValue("")

		fieldUpdates := map[string]string{
			env.KeyCloudflareAPIToken:  fields[0].ValueSignal.String(),
			env.KeyCloudflareAccountID: fields[1].ValueSignal.String(),
		}

		results, err := svc.ValidateAndUpdateFields(fieldUpdates)
		UpdateValidationStatus(results, []FormFieldData{fields[1]}, c)

		if err != nil {
			saveMessage.SetValue("error:" + err.Error())
			c.Sync()
			return
		}

		// Check for validation errors
		if HasValidationErrors(results, fieldUpdates) {
			saveMessage.SetValue("error:Please fix validation errors before continuing")
			c.Sync()
			return
		}

		// Validation passed - redirect to step 3
		saveMessage.SetValue("success:Account ID validated! Moving to step 3...")
		c.Sync()
		c.ExecScript("window.location.href = '/cloudflare/step3'")
	})

	c.View(func() h.H {
		// Use step's own metadata for prerequisite checking
		missingPrereqs := CheckPrerequisites(cfg, Step2Info.Prerequisites)

		return h.Main(
			h.Class("container"),

			// Use metadata for header
			RenderWizardStepHeader(&Step2Info, len(CloudflareWizard.Steps)),

			RenderNavigation("cloudflare"),

			// Add breadcrumb navigation
			RenderWizardBreadcrumbs(&CloudflareWizard, cfg, Step2Info.StepNumber),

			// Show prerequisite error banner if missing
			RenderPrerequisiteError(missingPrereqs),

			h.H2(h.Text("Find Account ID")),
			h.Ol(
				h.Li(RenderExternalLink(env.CloudflareDashboardURL, "Cloudflare Dashboard")),
				h.Li(h.Text("Find 'Account Home' in the left sidebar")),
				h.Li(h.Text("The Account ID is in the right sidebar")),
				h.Li(h.Text("It's a 32-character hex string - click copy icon")),
			),

			h.H3(h.Text("Enter your Account ID:")),
			RenderFormField(fields[1]),

			// Use wizard navigation component
			RenderWizardNavigation(&CloudflareWizard, &Step2Info, nextAction),

			RenderErrorMessage(saveMessage),
			RenderSuccessMessage(saveMessage),
		)
	})
}
