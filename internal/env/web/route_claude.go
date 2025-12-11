package web

import (
	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// claudePage creates the Claude-only setup page
func claudePage(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	// Create service for config operations
	svc := env.NewService(mockMode)

	// Create form fields using helper
	fields := CreateFormFields(c, cfg, []string{
		env.KeyClaudeAPIKey,
		env.KeyClaudeWorkspaceName,
	})

	// Save message signal
	saveMessage := c.Signal("")

	// Save and validate action using helper
	saveAction := c.Action(CreateSaveAction(c, svc, fields, saveMessage))

	c.View(func() h.H {
		return h.Main(
			h.Class("container"),
			h.H1(h.Text("Claude AI Setup")),
			h.P(h.Text("Configure your Claude AI credentials for content translation")),

			// Navigation using helper
			RenderNavigation("claude"),

			// Setup instructions
			h.H2(h.Text("Setup Instructions")),
			h.P(h.Strong(h.Text("Step 1: Sign up for Claude API"))),
			h.Ul(
				h.Li(RenderExternalLink(env.AnthropicConsoleURL, "Claude Console")),
				h.Li(h.Text("Create an account if you don't have one")),
				h.Li(h.Text("Verify your email address")),
				h.Li(RenderExternalLinkWithCustomPrefix(env.AnthropicBillingURL, "Add billing information at: ", "Billing Settings")),
			),
			h.P(h.Strong(h.Text("Step 2: Create API Key"))),
			h.Ul(
				h.Li(RenderExternalLink(env.AnthropicAPIKeysURL, "API Keys")),
				h.Li(h.Text("Click 'Create Key' button")),
				h.Li(h.Text("Give your key a descriptive name")),
				h.Li(h.Text("Copy the API key (save it securely - you won't see it again)")),
			),
			h.P(h.Strong(h.Text("Step 3: Find Workspace Name (Optional)"))),
			h.Ul(
				h.Li(RenderExternalLink(env.AnthropicWorkspacesURL, "Workspaces")),
				h.Li(h.Text("Find your workspace name in the list")),
				h.Li(h.Text("This is optional but helps organize your API usage")),
			),
			h.P(h.Strong(h.Text("Step 4: Enter Credentials Below"))),
			h.Ul(
				h.Li(h.Text("Paste your API key into the field below")),
				h.Li(h.Text("Optionally enter your workspace name")),
				h.Li(h.Text("Click 'Save Claude Configuration' to validate and save")),
			),

			// Claude Section - render form fields using helpers
			h.H2(h.Text("Claude API Credentials")),
			RenderFormField(fields[0]),
			RenderFormField(fields[1]),

			// Action buttons
			h.Div(
				h.Button(h.Text("Save Claude Configuration"), saveAction.OnClick()),
			),

			// Save message using helper
			RenderErrorMessage(saveMessage),
			RenderSuccessMessage(saveMessage),
		)
	})
}
