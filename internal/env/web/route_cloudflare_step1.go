package web

import (
	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// Step1Info is the metadata for Step 1 - single source of truth
var Step1Info = WizardStepInfo{
	StepNumber:    1,
	Path:          "/cloudflare/step1",
	Title:         "API Token",
	Description:   "Configure your Cloudflare API token for deployments",
	Fields:        []string{env.KeyCloudflareAPIToken, env.KeyCloudflareAPITokenName},
	Prerequisites: nil, // No prerequisites for first step
}

// cloudflareStep1Page - API Token setup (Step 1 of 5)
func cloudflareStep1Page(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	svc := env.NewService(mockMode)

	// Create fields using step metadata
	fields := CreateFormFields(c, cfg, Step1Info.Fields)

	saveMessage := c.Signal("")

	// Next action - validate and go to step 2
	nextAction := c.Action(func() {
		saveMessage.SetValue("")

		fieldUpdates := map[string]string{
			env.KeyCloudflareAPIToken:     fields[0].ValueSignal.String(),
			env.KeyCloudflareAPITokenName: fields[1].ValueSignal.String(),
		}

		results, err := svc.ValidateAndUpdateFields(fieldUpdates)
		UpdateValidationStatus(results, fields, c)

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

		// Validation passed - redirect to step 2
		saveMessage.SetValue("success:Token validated! Moving to step 2...")
		c.Sync()
		c.ExecScript("window.location.href = '/cloudflare/step2'")
	})

	c.View(func() h.H {
		// Use step's own metadata for prerequisite checking
		missingPrereqs := CheckPrerequisites(cfg, Step1Info.Prerequisites)

		return h.Main(
			h.Class("container"),

			// Use metadata for header
			RenderWizardStepHeader(&Step1Info, len(CloudflareWizard.Steps)),

			RenderNavigation("cloudflare"),

			// Add breadcrumb navigation
			RenderWizardBreadcrumbs(&CloudflareWizard, cfg, Step1Info.StepNumber),

			// Show prerequisite error banner if missing (Step 1 has no prerequisites, so will be empty)
			RenderPrerequisiteError(missingPrereqs),

			h.H2(h.Text("Create API Token")),
			h.Ol(
				h.Li(RenderExternalLink(env.CloudflareAPITokensURL, "Cloudflare API Tokens")),
				h.Li(h.Text("Click 'Create Token'")),
				h.Li(h.Text("Under 'Custom Token', click 'Get started'")),
				h.Li(h.Text("Give your token a descriptive name (e.g., 'Production Deploy Token')")),
				h.Li(h.Text("Under Permissions, add: Account → Cloudflare Pages → Edit")),
				h.Li(h.Text("Click 'Continue to summary' → 'Create Token'")),
				h.Li(h.Text("Copy the token value and paste below (save securely - you won't see it again!)")),
			),

			h.H3(h.Text("Enter your API Token:")),
			RenderFormField(fields[0]),

			h.H3(h.Text("Token Name:")),
			h.P(h.Text("Enter the name you gave this token in Cloudflare (helps you remember which token this is).")),
			RenderFormField(fields[1]),

			// Use wizard navigation component
			RenderWizardNavigation(&CloudflareWizard, &Step1Info, nextAction),

			RenderErrorMessage(saveMessage),
			RenderSuccessMessage(saveMessage),
		)
	})
}
