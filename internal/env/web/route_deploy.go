package web

import (
	"strings"

	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// deployPage - Build and deploy Hugo site to Cloudflare Pages
func deployPage(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	// Get project name from config
	projectName := cfg.Get(env.KeyCloudflarePageProject)

	// Deployment signals
	deployOutput := c.Signal("")
	buildInProgress := c.Signal(false)      // "Build Site Only" button progress
	previewInProgress := c.Signal(false)    // "Deploy to Preview" button progress
	productionInProgress := c.Signal(false) // "Deploy to Production" button progress
	localURL := c.Signal("")
	lanURL := c.Signal("")        // LAN URL for mobile testing
	previewURL := c.Signal("")    // Cloudflare preview URL (*.pages.dev)
	deploymentURL := c.Signal("") // Custom domain URL (production)

	// Deploy to Preview action (no branch flag)
	buildDeployPreviewAction := c.Action(func() {
		// Check prerequisites using centralized helper
		missingPrereqs := CheckPrerequisites(cfg, []PrerequisiteCheck{
			{FieldKey: env.KeyCloudflareAPIToken, DisplayName: "Cloudflare API Token", StepPath: "/cloudflare/step1", StepLabel: "Configure in Step 1"},
			{FieldKey: env.KeyCloudflareAccountID, DisplayName: "Account ID", StepPath: "/cloudflare/step2", StepLabel: "Configure in Step 2"},
			{FieldKey: env.KeyCloudflarePageProject, DisplayName: "Project Name", StepPath: "/cloudflare/step4", StepLabel: "Configure in Step 4"},
		})
		if len(missingPrereqs) > 0 {
			// Build error message listing missing items
			errorMsg := "Missing configuration: "
			for i, prereq := range missingPrereqs {
				if i > 0 {
					errorMsg += ", "
				}
				errorMsg += prereq.DisplayName
			}
			deployOutput.SetValue("error:" + errorMsg)
			c.Sync()
			return
		}

		currentProject := cfg.Get(env.KeyCloudflarePageProject)

		previewInProgress.SetValue(true)
		deployOutput.SetValue("Starting build and preview deployment...\n")
		c.Sync()

		// Run build and deploy (no branch = preview only)
		result := env.BuildAndDeploy(currentProject, "", mockMode)

		previewInProgress.SetValue(false)
		if result.Error != nil {
			deployOutput.SetValue("error:" + result.Output + "\nError: " + result.Error.Error())
		} else {
			deployOutput.SetValue("success:" + result.Output)
		}
		localURL.SetValue(result.LocalURL)
		lanURL.SetValue(result.LANURL)
		previewURL.SetValue(result.PreviewURL)
		deploymentURL.SetValue("") // Clear production URL for preview deployments
		c.Sync()
	})

	// Deploy to Production action (with --branch=main)
	buildDeployProductionAction := c.Action(func() {
		// Check prerequisites using centralized helper
		missingPrereqs := CheckPrerequisites(cfg, []PrerequisiteCheck{
			{FieldKey: env.KeyCloudflareAPIToken, DisplayName: "Cloudflare API Token", StepPath: "/cloudflare/step1", StepLabel: "Configure in Step 1"},
			{FieldKey: env.KeyCloudflareAccountID, DisplayName: "Account ID", StepPath: "/cloudflare/step2", StepLabel: "Configure in Step 2"},
			{FieldKey: env.KeyCloudflarePageProject, DisplayName: "Project Name", StepPath: "/cloudflare/step4", StepLabel: "Configure in Step 4"},
		})
		if len(missingPrereqs) > 0 {
			// Build error message listing missing items
			errorMsg := "Missing configuration: "
			for i, prereq := range missingPrereqs {
				if i > 0 {
					errorMsg += ", "
				}
				errorMsg += prereq.DisplayName
			}
			deployOutput.SetValue("error:" + errorMsg)
			c.Sync()
			return
		}

		currentProject := cfg.Get(env.KeyCloudflarePageProject)

		productionInProgress.SetValue(true)
		deployOutput.SetValue("Starting build and production deployment...\n")
		c.Sync()

		// Run build and deploy (branch=main = production)
		result := env.BuildAndDeploy(currentProject, "main", mockMode)

		productionInProgress.SetValue(false)
		if result.Error != nil {
			deployOutput.SetValue("error:" + result.Output + "\nError: " + result.Error.Error())
		} else {
			deployOutput.SetValue("success:" + result.Output)
		}
		localURL.SetValue(result.LocalURL)
		lanURL.SetValue(result.LANURL)
		previewURL.SetValue(result.PreviewURL)

		// Set production URL from config (custom domain)
		customDomain := cfg.Get(env.KeyCloudflareDomain)
		if customDomain != "" && !env.IsPlaceholder(customDomain) && result.Error == nil {
			deploymentURL.SetValue("https://" + customDomain)
		} else {
			deploymentURL.SetValue(result.DeploymentURL)
		}
		c.Sync()
	})

	// Build only action
	buildOnlyAction := c.Action(func() {
		buildInProgress.SetValue(true)
		deployOutput.SetValue("Building Hugo site...\n")
		c.Sync()

		result := env.BuildHugoSite(mockMode)

		buildInProgress.SetValue(false)
		if result.Error != nil {
			deployOutput.SetValue("error:" + result.Output + "\nError: " + result.Error.Error())
		} else {
			deployOutput.SetValue("success:" + result.Output)
		}
		localURL.SetValue(result.LocalURL)
		lanURL.SetValue(result.LANURL)
		c.Sync()
	})

	c.View(func() h.H {
		// Check prerequisites for deployment
		missingPrereqs := CheckPrerequisites(cfg, []PrerequisiteCheck{
			{FieldKey: env.KeyCloudflareAPIToken, DisplayName: "Cloudflare API Token", StepPath: "/cloudflare/step1", StepLabel: "Configure in Step 1"},
			{FieldKey: env.KeyCloudflareAccountID, DisplayName: "Account ID", StepPath: "/cloudflare/step2", StepLabel: "Configure in Step 2"},
			{FieldKey: env.KeyCloudflarePageProject, DisplayName: "Project Name", StepPath: "/cloudflare/step4", StepLabel: "Configure in Step 4"},
		})

		return h.Main(
			h.Class("container"),
			h.H1(h.Text("Deploy to Cloudflare Pages")),

			RenderNavigation("deploy"),

			// Show prerequisite error banner if missing
			RenderPrerequisiteError(missingPrereqs),

			// Project info section
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); padding: 1rem; margin-bottom: 2rem;"),
				h.H3(h.Text("Current Configuration")),
				h.P(
					h.Strong(h.Text("Project: ")),
					h.If(projectName != "" && !env.IsPlaceholder(projectName),
						h.Text(projectName),
					),
					h.If(projectName == "" || env.IsPlaceholder(projectName),
						h.Span(
							h.Style("color: var(--pico-del-color);"),
							h.Text("Not configured"),
						),
					),
				),
			),

			// Deployment section
			h.Div(
				h.H2(h.Text("Build & Deploy")),
				h.P(h.Text("Build your Hugo site and deploy it to Cloudflare Pages.")),

				// Build and deploy buttons
				h.Div(
					h.Style("display: flex; gap: 1rem; margin-bottom: 1rem; flex-wrap: wrap;"),
					h.Button(
						h.Attr("class", "secondary"),
						h.Text("Build Site Only"),
						h.If(buildInProgress.String() == "true", h.Attr("aria-busy", "true")),
						h.If(buildInProgress.String() == "true", h.Attr("disabled", "disabled")),
						h.If(projectName == "" || env.IsPlaceholder(projectName), h.Attr("disabled", "disabled")),
						buildOnlyAction.OnClick(),
					),
					h.Button(
						h.Attr("class", "secondary"),
						h.Text("Deploy to Preview"),
						h.If(previewInProgress.String() == "true", h.Attr("aria-busy", "true")),
						h.If(previewInProgress.String() == "true", h.Attr("disabled", "disabled")),
						h.If(projectName == "" || env.IsPlaceholder(projectName), h.Attr("disabled", "disabled")),
						buildDeployPreviewAction.OnClick(),
					),
					h.Button(
						h.Text("Deploy to Production"),
						h.If(productionInProgress.String() == "true", h.Attr("aria-busy", "true")),
						h.If(productionInProgress.String() == "true", h.Attr("disabled", "disabled")),
						h.If(projectName == "" || env.IsPlaceholder(projectName), h.Attr("disabled", "disabled")),
						buildDeployProductionAction.OnClick(),
					),
				),

				// Output display
				h.If(deployOutput.String() != "",
					h.Div(
						h.Style("margin-top: 1.5rem;"),
						// Success output
						h.If(strings.HasPrefix(deployOutput.String(), "success:"),
							h.Article(
								h.Style("background-color: var(--pico-ins-background); border-left: 4px solid var(--pico-ins-color); padding: 1rem;"),
								h.Pre(
									h.Style("margin: 0; white-space: pre-wrap; font-size: 0.875rem; color: var(--pico-ins-color);"),
									h.Text(strings.TrimPrefix(deployOutput.String(), "success:")),
								),
							),
						),
						// Error output
						h.If(strings.HasPrefix(deployOutput.String(), "error:"),
							h.Article(
								h.Style("background-color: var(--pico-del-background); border-left: 4px solid var(--pico-del-color); padding: 1rem;"),
								h.Pre(
									h.Style("margin: 0; white-space: pre-wrap; font-size: 0.875rem; color: var(--pico-del-color);"),
									h.Text(strings.TrimPrefix(deployOutput.String(), "error:")),
								),
							),
						),
						// In-progress output
						h.If(!strings.HasPrefix(deployOutput.String(), "success:") && !strings.HasPrefix(deployOutput.String(), "error:"),
							h.Article(
								h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-primary); padding: 1rem;"),
								h.Pre(
									h.Style("margin: 0; white-space: pre-wrap; font-size: 0.875rem;"),
									h.Text(deployOutput.String()),
								),
							),
						),

						// Preview URLs section
						h.If(localURL.String() != "" || lanURL.String() != "" || previewURL.String() != "" || deploymentURL.String() != "",
							h.Div(
								h.Style("margin-top: 1.5rem;"),
								h.H3(h.Text("Preview URLs")),

								// Local preview URL
								h.If(localURL.String() != "",
									RenderURLLink(localURL.String(), "Local Preview", "🌐"),
								),

								// LAN preview URL for mobile testing
								h.If(lanURL.String() != "",
									RenderURLLink(lanURL.String(), "LAN Preview (Mobile)", "📱"),
								),

								// Cloudflare preview URL (*.pages.dev)
								h.If(previewURL.String() != "",
									RenderURLLink(previewURL.String(), "Preview Deployment", "🔗"),
								),

								// Production deployment URL (custom domain)
								h.If(deploymentURL.String() != "",
									RenderURLLink(deploymentURL.String(), "Production (Custom Domain)", "☁️"),
								),
							),
						),
					),
				),
			),

			h.Div(
				h.Style("margin-top: 2rem;"),
				h.A(h.Href("/"), h.Text("← Back to Overview")),
			),
		)
	})
}
