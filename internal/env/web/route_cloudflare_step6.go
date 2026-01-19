package web

import (
	"strings"

	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// Step6Info is the metadata for Step 6 - single source of truth
var Step6Info = WizardStepInfo{
	StepNumber:  6,
	Path:        "/cloudflare/step6",
	Title:       "Event Notifications",
	Description: "Enable round-trip validation with sync events",
	Fields: []string{
		env.KeyCloudflareWorkerName,
		env.KeyCloudflareSyncEndpoint,
		env.KeyCloudflareReceiverPort,
	},
	Prerequisites: []PrerequisiteCheck{
		{
			FieldKey:    env.KeyCloudflareAPIToken,
			DisplayName: "Cloudflare API Token",
			StepPath:    "/cloudflare/step1",
			StepLabel:   "Configure in Step 1",
		},
		{
			FieldKey:    env.KeyCloudflareAccountID,
			DisplayName: "Account ID",
			StepPath:    "/cloudflare/step2",
			StepLabel:   "Configure in Step 2",
		},
		{
			FieldKey:    env.KeyCloudflarePageProject,
			DisplayName: "Project Name",
			StepPath:    "/cloudflare/step4",
			StepLabel:   "Configure in Step 4",
		},
	},
}

// cloudflareStep6Page - Event Notifications setup (Step 6 of 6)
func cloudflareStep6Page(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	// Get required values from config
	apiToken := cfg.Get(env.KeyCloudflareAPIToken)
	accountID := cfg.Get(env.KeyCloudflareAccountID)
	projectName := cfg.Get(env.KeyCloudflarePageProject)

	// Get worker settings from config (with defaults)
	workerName := cfg.Get(env.KeyCloudflareWorkerName)
	if workerName == "" {
		workerName = "xplat-sync"
	}
	syncEndpoint := cfg.Get(env.KeyCloudflareSyncEndpoint)
	receiverPort := cfg.Get(env.KeyCloudflareReceiverPort)
	if receiverPort == "" {
		receiverPort = "9091"
	}

	// Signals for UI state
	status := c.Signal("")
	isDeploying := c.Signal(false)
	isStartingReceiver := c.Signal(false)
	isStartingTunnel := c.Signal(false)
	tunnelURL := c.Signal(syncEndpoint)

	// Editable field signals
	workerNameSignal := c.Signal(workerName)
	receiverPortSignal := c.Signal(receiverPort)

	// Save configuration action
	saveConfigAction := c.Action(func() {
		newWorkerName := workerNameSignal.String()
		newReceiverPort := receiverPortSignal.String()

		// Update config
		svc := env.NewService(mockMode)
		err := svc.UpdateFields(map[string]string{
			env.KeyCloudflareWorkerName:   newWorkerName,
			env.KeyCloudflareReceiverPort: newReceiverPort,
		})

		if err != nil {
			status.SetValue("error:Failed to save configuration: " + err.Error())
		} else {
			status.SetValue("success:Configuration saved successfully")
		}
		c.Sync()
	})

	// Deploy worker action (placeholder - will be implemented)
	deployWorkerAction := c.Action(func() {
		if apiToken == "" || env.IsPlaceholder(apiToken) ||
			accountID == "" || env.IsPlaceholder(accountID) {
			status.SetValue("error:Configuration incomplete - need API token and Account ID")
			c.Sync()
			return
		}

		isDeploying.SetValue(true)
		status.SetValue("info:Deploying sync worker to Cloudflare...")
		c.Sync()

		// TODO: Implement worker deployment via Cloudflare API
		// For now, show instructions
		if mockMode {
			isDeploying.SetValue(false)
			status.SetValue("success:Worker deployed successfully (mock mode)")
			c.Sync()
			return
		}

		// Real deployment would go here
		isDeploying.SetValue(false)
		status.SetValue("info:Worker deployment via API coming soon. For now, use: xplat sync-cf worker deploy")
		c.Sync()
	})

	// Start receiver action
	startReceiverAction := c.Action(func() {
		port := receiverPortSignal.String()
		if port == "" {
			port = "9091"
		}

		isStartingReceiver.SetValue(true)
		status.SetValue("info:Starting event receiver on port " + port + "...")
		c.Sync()

		// The receiver runs as a background service via process-compose
		// This action just provides instructions
		isStartingReceiver.SetValue(false)
		status.SetValue("info:Start receiver with: xplat sync-cf receive --port=" + port + " --invalidate")
		c.Sync()
	})

	// Start tunnel action
	startTunnelAction := c.Action(func() {
		port := receiverPortSignal.String()
		if port == "" {
			port = "9091"
		}

		isStartingTunnel.SetValue(true)
		status.SetValue("info:Starting cloudflared tunnel for port " + port + "...")
		c.Sync()

		// The tunnel runs as a background service
		// This action provides instructions
		isStartingTunnel.SetValue(false)

		if mockMode {
			tunnelURL.SetValue("https://mock-tunnel-xyz.trycloudflare.com")
			status.SetValue("success:Tunnel started (mock mode) - URL: https://mock-tunnel-xyz.trycloudflare.com")
			c.Sync()
			return
		}

		status.SetValue("info:Start tunnel with: xplat sync-cf tunnel " + port)
		c.Sync()
	})

	c.View(func() h.H {
		// Use step's own metadata for prerequisite checking
		missingPrereqs := CheckPrerequisites(cfg, Step6Info.Prerequisites)

		return h.Main(
			h.Class("container"),

			// Use metadata for header
			RenderWizardStepHeader(&Step6Info, len(CloudflareWizard.Steps)),

			RenderNavigation("cloudflare"),

			// Add breadcrumb navigation
			RenderWizardBreadcrumbs(&CloudflareWizard, cfg, Step6Info.StepNumber),

			// Show prerequisite error banner if missing
			RenderPrerequisiteError(missingPrereqs),

			// Instructions
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-primary); padding: 1rem; margin-bottom: 1rem;"),
				h.H4(h.Text("What This Step Does")),
				h.P(h.Text("This step enables round-trip validation - when something happens on Cloudflare (deploy, notification, etc.), you'll see it locally. Here's how it works:")),
				h.Ol(
					h.Style("margin: 0.5rem 0 0 1.5rem;"),
					h.Li(
						h.Strong(h.Text("Deploy Worker: ")),
						h.Text("A small Cloudflare Worker receives events from Pages deploys and notifications"),
					),
					h.Li(
						h.Strong(h.Text("Start Receiver: ")),
						h.Text("A local HTTP server receives forwarded events from the Worker"),
					),
					h.Li(
						h.Strong(h.Text("Start Tunnel: ")),
						h.Text("A cloudflared tunnel connects the Worker to your local receiver"),
					),
					h.Li(
						h.Strong(h.Text("Validation: ")),
						h.Text("When you deploy or make changes, you'll see events confirming the operation succeeded"),
					),
				),
			),

			// Architecture diagram
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); padding: 1rem; margin-bottom: 1rem;"),
				h.H4(h.Text("Architecture")),
				h.Pre(
					h.Style("background: var(--pico-code-background-color); padding: 1rem; border-radius: 0.25rem; overflow-x: auto;"),
					h.Code(
						h.Text(`Cloudflare Services
        |
        v
+------------------+
| sync-cf Worker   |  <-- Receives events from Pages, Notifications
| (Edge)           |
+------------------+
        |
        | SYNC_ENDPOINT (tunnel URL)
        v
+------------------+
| cloudflared      |  <-- Quick tunnel (random URL) or named tunnel
| tunnel           |
+------------------+
        |
        v
+------------------+
| xplat sync-cf    |  <-- Local HTTP server
| receive          |
+------------------+
        |
        v
+------------------+
| Task Cache       |  <-- Invalidated on pages_deploy events
| Invalidation     |
+------------------+`),
					),
				),
			),

			// Configuration Form
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); padding: 1rem; margin-bottom: 1rem;"),
				h.H4(h.Text("Configuration")),

				// Worker Name
				h.Label(
					h.For("workerName"),
					h.Text("Worker Name"),
				),
				h.Input(
					h.Type("text"),
					h.Id("workerName"),
					h.Value(workerNameSignal.String()),
					workerNameSignal.Bind(),
					h.Placeholder("xplat-sync"),
				),
				h.Small(h.Text("Name of the Cloudflare Worker to deploy")),

				// Receiver Port
				h.Label(
					h.For("receiverPort"),
					h.Style("margin-top: 1rem;"),
					h.Text("Receiver Port"),
				),
				h.Input(
					h.Type("text"),
					h.Id("receiverPort"),
					h.Value(receiverPortSignal.String()),
					receiverPortSignal.Bind(),
					h.Placeholder("9091"),
				),
				h.Small(h.Text("Local port for the event receiver server")),

				// Tunnel URL (read-only, shows when tunnel is active)
				h.If(tunnelURL.String() != "",
					h.Div(
						h.Style("margin-top: 1rem;"),
						h.Label(h.Text("Tunnel URL")),
						h.Input(
							h.Type("text"),
							h.Value(tunnelURL.String()),
							h.Attr("readonly", "readonly"),
							h.Style("background: var(--pico-card-background-color); color: var(--pico-ins-color);"),
						),
						h.Small(h.Text("Configure this as SYNC_ENDPOINT in your Worker")),
					),
				),

				// Save button
				h.Div(
					h.Style("margin-top: 1rem;"),
					h.Button(
						h.Attr("class", "secondary"),
						h.Text("Save Configuration"),
						saveConfigAction.OnClick(),
					),
				),
			),

			// Setup Steps
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); padding: 1rem; margin-bottom: 1rem;"),
				h.H4(h.Text("Setup Steps")),

				// Step A: Deploy Worker
				h.Div(
					h.Style("display: grid; grid-template-columns: 2fr 1fr; gap: 1rem; align-items: center; margin-bottom: 1rem; padding: 1rem; border: 1px solid var(--pico-muted-border-color); border-radius: 0.25rem;"),
					h.Div(
						h.Strong(h.Text("A. Deploy sync-cf Worker")),
						h.Br(),
						h.Small(h.Text("Deploy the Worker to receive events from Cloudflare services")),
					),
					h.Button(
						h.If(isDeploying.String() == "true", h.Attr("aria-busy", "true")),
						h.If(isDeploying.String() == "true", h.Attr("disabled", "disabled")),
						h.If(len(missingPrereqs) > 0, h.Attr("disabled", "disabled")),
						h.Text(func() string {
							if isDeploying.String() == "true" {
								return "Deploying..."
							}
							return "Deploy Worker"
						}()),
						deployWorkerAction.OnClick(),
					),
				),

				// Step B: Start Receiver
				h.Div(
					h.Style("display: grid; grid-template-columns: 2fr 1fr; gap: 1rem; align-items: center; margin-bottom: 1rem; padding: 1rem; border: 1px solid var(--pico-muted-border-color); border-radius: 0.25rem;"),
					h.Div(
						h.Strong(h.Text("B. Start Event Receiver")),
						h.Br(),
						h.Small(h.Text("Start the local HTTP server to receive forwarded events")),
					),
					h.Button(
						h.Attr("class", "secondary"),
						h.If(isStartingReceiver.String() == "true", h.Attr("aria-busy", "true")),
						h.If(isStartingReceiver.String() == "true", h.Attr("disabled", "disabled")),
						h.Text(func() string {
							if isStartingReceiver.String() == "true" {
								return "Starting..."
							}
							return "Start Receiver"
						}()),
						startReceiverAction.OnClick(),
					),
				),

				// Step C: Start Tunnel
				h.Div(
					h.Style("display: grid; grid-template-columns: 2fr 1fr; gap: 1rem; align-items: center; margin-bottom: 1rem; padding: 1rem; border: 1px solid var(--pico-muted-border-color); border-radius: 0.25rem;"),
					h.Div(
						h.Strong(h.Text("C. Start cloudflared Tunnel")),
						h.Br(),
						h.Small(h.Text("Create a tunnel to expose the receiver to the Worker")),
					),
					h.Button(
						h.Attr("class", "secondary"),
						h.If(isStartingTunnel.String() == "true", h.Attr("aria-busy", "true")),
						h.If(isStartingTunnel.String() == "true", h.Attr("disabled", "disabled")),
						h.Text(func() string {
							if isStartingTunnel.String() == "true" {
								return "Starting..."
							}
							return "Start Tunnel"
						}()),
						startTunnelAction.OnClick(),
					),
				),
			),

			// CLI Commands Reference
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); padding: 1rem; margin-bottom: 1rem;"),
				h.H4(h.Text("CLI Commands")),
				h.P(h.Text("You can also set up event notifications using the command line:")),
				h.Pre(
					h.Style("background: var(--pico-code-background-color); padding: 1rem; border-radius: 0.25rem; overflow-x: auto;"),
					h.Code(
						h.Text(`# 1. Deploy the Worker (requires wrangler)
xplat sync-cf worker deploy

# 2. Start the receiver (in one terminal)
xplat sync-cf receive --port=`+receiverPortSignal.String()+` --invalidate

# 3. Start the tunnel (in another terminal)
xplat sync-cf tunnel `+receiverPortSignal.String()+`

# 4. Copy the tunnel URL and set SYNC_ENDPOINT in Worker`),
					),
				),
			),

			// Current Configuration
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); padding: 1rem; margin-bottom: 1rem;"),
				h.H4(h.Text("Current Configuration")),
				h.Table(
					h.Tr(
						h.Td(h.Strong(h.Text("Account ID:"))),
						h.Td(
							h.Code(
								h.Style("color: var(--pico-color);"),
								h.Text(func() string {
									if env.IsPlaceholder(accountID) {
										return "Not configured"
									}
									return accountID
								}()),
							),
						),
					),
					h.Tr(
						h.Td(h.Strong(h.Text("Project Name:"))),
						h.Td(
							h.Code(
								h.Style("color: var(--pico-color);"),
								h.Text(func() string {
									if env.IsPlaceholder(projectName) {
										return "Not configured"
									}
									return projectName
								}()),
							),
						),
					),
					h.Tr(
						h.Td(h.Strong(h.Text("Worker Name:"))),
						h.Td(
							h.Code(
								h.Style("color: var(--pico-color);"),
								h.Text(workerNameSignal.String()),
							),
						),
					),
					h.Tr(
						h.Td(h.Strong(h.Text("Receiver Port:"))),
						h.Td(
							h.Code(
								h.Style("color: var(--pico-color);"),
								h.Text(receiverPortSignal.String()),
							),
						),
					),
					h.If(tunnelURL.String() != "",
						h.Tr(
							h.Td(h.Strong(h.Text("Tunnel URL:"))),
							h.Td(
								h.Code(
									h.Style("color: var(--pico-ins-color);"),
									h.Text(tunnelURL.String()),
								),
							),
						),
					),
				),
			),

			// Status Messages - use helper functions for proper PicoCSS styling
			RenderErrorMessage(status),
			RenderSuccessMessage(status),
			// Info message (for in-progress states)
			h.If(strings.HasPrefix(status.String(), "info:"),
				h.Article(
					h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-primary); padding: 1rem; margin-top: 1rem;"),
					h.P(
						h.Style("margin: 0; color: var(--pico-color);"),
						h.Text(strings.TrimPrefix(status.String(), "info:")),
					),
				),
			),

			// Use wizard navigation component (Step 6 is the last step)
			RenderWizardNavigation(&CloudflareWizard, &Step6Info, nil),
		)
	})
}
