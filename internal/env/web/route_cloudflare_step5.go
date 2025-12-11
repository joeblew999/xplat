package web

import (
	"strings"

	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// Step5Info is the metadata for Step 5 - single source of truth
var Step5Info = WizardStepInfo{
	StepNumber:  5,
	Path:        "/cloudflare/step5",
	Title:       "Custom Domain",
	Description: "Attach your custom domain to the Pages project",
	Fields:      nil, // This step doesn't add new fields, it just uses existing domain
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
			FieldKey:    env.KeyCloudflareDomain,
			DisplayName: "Custom Domain",
			StepPath:    "/cloudflare/step3",
			StepLabel:   "Configure in Step 3",
		},
		{
			FieldKey:    env.KeyCloudflarePageProject,
			DisplayName: "Project Name",
			StepPath:    "/cloudflare/step4",
			StepLabel:   "Configure in Step 4",
		},
	},
}

// cloudflareStep5Page - Custom Domain setup (Step 5 of 5)
func cloudflareStep5Page(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	// Get required values from config
	apiToken := cfg.Get(env.KeyCloudflareAPIToken)
	accountID := cfg.Get(env.KeyCloudflareAccountID)
	projectName := cfg.Get(env.KeyCloudflarePageProject)
	customDomain := cfg.Get(env.KeyCloudflareDomain)

	// Closure variable to store domains list (not a signal - signals are for browser reactivity only)
	var domainsList []env.PagesDomain

	// Signals for UI state
	status := c.Signal("")
	isAttaching := c.Signal(false)
	isRemoving := c.Signal(false)
	isRefreshing := c.Signal(false)

	// Refresh domains action - fetches domains from Cloudflare API
	refreshDomainsAction := c.Action(func() {
		if apiToken == "" || env.IsPlaceholder(apiToken) ||
			accountID == "" || env.IsPlaceholder(accountID) ||
			projectName == "" || env.IsPlaceholder(projectName) {
			status.SetValue("error:Configuration incomplete")
			c.Sync()
			return
		}

		isRefreshing.SetValue(true)
		status.SetValue("info:Refreshing domain status...")
		c.Sync()

		// Fetch domains from Cloudflare Pages API
		var err error

		if mockMode {
			// Mock data for testing
			domainsList = []env.PagesDomain{
				{Name: "ubuntusoftware.net", Status: "active"},
				{Name: "example.com", Status: "pending"},
			}
		} else {
			domainsList, err = env.ListPagesDomains(apiToken, accountID, projectName)
		}

		isRefreshing.SetValue(false)
		if err != nil {
			status.SetValue("error:Failed to refresh domain status: " + err.Error())
		} else {
			status.SetValue("success:Domain status refreshed successfully")
		}
		c.Sync()
	})

	// Attach domain action
	attachDomainAction := c.Action(func() {
		// Check prerequisites using step metadata
		missingPrereqs := CheckPrerequisites(cfg, Step5Info.Prerequisites)
		if len(missingPrereqs) > 0 {
			// Build error message listing missing items
			errorMsg := "Missing configuration: "
			for i, prereq := range missingPrereqs {
				if i > 0 {
					errorMsg += ", "
				}
				errorMsg += prereq.DisplayName
			}
			status.SetValue("error:" + errorMsg)
			c.Sync()
			return
		}

		isAttaching.SetValue(true)
		status.SetValue("info:Attaching custom domain...")
		c.Sync()

		// Add domain via Cloudflare Pages API
		err := env.AddPagesDomain(apiToken, accountID, projectName, customDomain)

		isAttaching.SetValue(false)
		if err != nil {
			status.SetValue("error:Failed to attach domain: " + err.Error())
		} else {
			status.SetValue("success:Successfully attached " + customDomain + " - Reloading page to show updated domains...")
			c.Sync()
			// Reload page to refresh domains list
			c.ExecScript("setTimeout(function() { window.location.reload(); }, 1500);")
			return
		}
		c.Sync()
	})

	c.View(func() h.H {
		// Use step's own metadata for prerequisite checking
		missingPrereqs := CheckPrerequisites(cfg, Step5Info.Prerequisites)

		// domainsList is a closure variable - starts empty, populated only when user clicks "Refresh Status"

		// Check if custom domain is already attached
		domainAlreadyAttached := false
		for _, domain := range domainsList {
			if domain.Name == customDomain {
				domainAlreadyAttached = true
				break
			}
		}

		// Build domain list UI elements
		domainListElements := make([]h.H, 0, len(domainsList))
		for _, domain := range domainsList {
			domainName := domain.Name     // Capture in closure
			domainStatus := domain.Status // Capture in closure

			// Create remove action for this specific domain
			removeAction := c.Action(func() {
				if apiToken == "" || env.IsPlaceholder(apiToken) ||
					accountID == "" || env.IsPlaceholder(accountID) ||
					projectName == "" || env.IsPlaceholder(projectName) {
					status.SetValue("error:Configuration incomplete")
					c.Sync()
					return
				}

				isRemoving.SetValue(true)
				status.SetValue("info:Removing domain " + domainName + "...")
				c.Sync()

				err := env.DeletePagesDomain(apiToken, accountID, projectName, domainName)

				isRemoving.SetValue(false)
				if err != nil {
					status.SetValue("error:Failed to remove domain: " + err.Error())
				} else {
					status.SetValue("success:Successfully removed " + domainName + " - Reloading page to show updated domains...")
					c.Sync()
					// Reload page to refresh domains list
					c.ExecScript("setTimeout(function() { window.location.reload(); }, 1500);")
					return
				}
				c.Sync()
			})

			domainListElements = append(domainListElements, h.Article(
				h.Style("display: grid; grid-template-columns: 1fr auto; gap: 1rem; align-items: start; margin-bottom: 1rem;"),
				h.Div(
					h.Div(
						h.Style("display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem;"),
						h.Strong(
							h.Style("font-family: monospace; font-size: 1.1em;"),
							h.Text(domainName),
						),
						// Inline status badge
						func() h.H {
							color := "var(--pico-muted-color)"
							bg := "rgba(128, 128, 128, 0.1)"
							switch domainStatus {
							case "active":
								color = "var(--pico-ins-color)"
								bg = "rgba(46, 125, 50, 0.1)"
							case "pending", "initializing":
								color = "#f59e0b"
								bg = "rgba(245, 158, 11, 0.1)"
							}
							return h.Span(
								h.Style(
									"display: inline-block; "+
										"padding: 0.25rem 0.75rem; "+
										"border-radius: 1rem; "+
										"font-size: 0.875rem; "+
										"font-weight: 600; "+
										"text-transform: uppercase; "+
										"letter-spacing: 0.05em; "+
										"color: "+color+"; "+
										"background-color: "+bg+";",
								),
								h.Text(domainStatus),
							)
						}(),
					),
					h.Small(
						h.Text(func() string {
							switch domainStatus {
							case "active":
								return "✓ Domain is active and serving traffic"
							case "pending":
								return "⏳ Waiting for SSL certificate provisioning (typically 10-30 minutes)"
							case "initializing":
								return "🔄 Domain initialization in progress"
							default:
								return "Status: " + domainStatus
							}
						}()),
					),
				),
				h.Button(
					h.Attr("class", "secondary outline"),
					h.If(isRemoving.String() == "true", h.Attr("disabled", "disabled")),
					h.Text("Remove"),
					removeAction.OnClick(),
				),
			))
		}

		return h.Main(
			h.Class("container"),

			// Use metadata for header
			RenderWizardStepHeader(&Step5Info, len(CloudflareWizard.Steps)),

			RenderNavigation("cloudflare"),

			// Add breadcrumb navigation
			RenderWizardBreadcrumbs(&CloudflareWizard, cfg, Step5Info.StepNumber),

			// Show prerequisite error banner if missing
			RenderPrerequisiteError(missingPrereqs),

			// Instructions
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-primary); padding: 1rem; margin-bottom: 1rem;"),
				h.H4(h.Text("📖 What This Step Does")),
				h.P(h.Text("This step attaches your custom domain to the Cloudflare Pages project via the Cloudflare API. Here's what happens:")),
				h.Ol(
					h.Style("margin: 0.5rem 0 0 1.5rem;"),
					h.Li(
						h.Strong(h.Text("Immediate: ")),
						h.Text("Domain attachment via API - your domain is registered with the Pages project"),
					),
					h.Li(
						h.Strong(h.Text("0-5 minutes: ")),
						h.Text("Status changes from 'initializing' to 'pending' - Cloudflare begins SSL certificate provisioning"),
					),
					h.Li(
						h.Strong(h.Text("10-30 minutes: ")),
						h.Text("Status changes to 'active' - SSL certificate is provisioned, DNS is fully configured"),
					),
					h.Li(
						h.Strong(h.Text("After 'active': ")),
						h.Text("Your custom domain will serve traffic without Error 1014"),
					),
				),
				h.P(
					h.Style("margin-top: 1rem; padding: 0.75rem; background: var(--pico-ins-color); color: white; border-radius: 0.25rem;"),
					h.Strong(h.Text("💡 Tip: ")),
					h.Text("Use the preview URL (*.pages.dev) to access your site immediately while waiting for the custom domain to activate."),
				),
			),

			// Error 1014 Troubleshooting
			h.If(len(domainsList) > 0,
				h.Article(
					h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-muted-color); padding: 1rem; margin-bottom: 1rem;"),
					h.H4(h.Text("⚠️ Troubleshooting Error 1014")),
					h.P(h.Text("If you see \"Error 1014: CNAME Cross-User Banned\" when visiting your custom domain:")),
					h.Ul(
						h.Style("margin: 0.5rem 0 0 1.5rem;"),
						h.Li(h.Text("The domain status is not yet 'active' (still initializing or pending)")),
						h.Li(h.Text("Cloudflare is provisioning SSL certificates and configuring DNS")),
						h.Li(h.Text("This typically takes 10-30 minutes after attachment")),
						h.Li(h.Text("Use the 'Refresh Status' button below to check if domain status has changed to 'active'")),
						h.Li(h.Text("Once status shows 'active', Error 1014 will be resolved")),
					),
					h.P(
						h.Style("margin-top: 0.5rem;"),
						h.Text("Use the preview URL (*.pages.dev) while waiting for custom domain activation."),
					),
				),
			),

			// Current Configuration
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); padding: 1rem; margin-bottom: 1rem;"),
				h.H4(h.Text("🔧 Current Configuration")),
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
						h.Td(h.Strong(h.Text("Custom Domain:"))),
						h.Td(
							h.Code(
								h.Style("color: var(--pico-color);"),
								h.Text(func() string {
									if env.IsPlaceholder(customDomain) {
										return "Not configured"
									}
									return customDomain
								}()),
							),
						),
					),
				),
			),

			// Quick Links Section
			h.Article(
				h.Style("background-color: var(--pico-card-background-color); padding: 1rem; margin-bottom: 1rem;"),
				h.H4(h.Text("🔗 Quick Links")),
				h.P(
					h.Text("📦 Preview URL: "),
					h.A(
						h.Href("https://"+projectName+".pages.dev"),
						h.Attr("target", "_blank"),
						h.Attr("rel", "noopener noreferrer"),
						h.Text(projectName+".pages.dev"),
					),
					h.Br(),
					h.Small(h.Text("Always available - use this while waiting for custom domain activation")),
				),
				h.If(customDomain != "" && !env.IsPlaceholder(customDomain),
					h.P(
						h.Text("🌐 Custom Domain: "),
						h.A(
							h.Href("https://"+customDomain),
							h.Attr("target", "_blank"),
							h.Attr("rel", "noopener noreferrer"),
							h.Text(customDomain),
						),
						h.Br(),
						h.If(domainAlreadyAttached,
							h.Small(
								h.Style("color: var(--pico-muted-color);"),
								h.Text("Note: May show Error 1014 if domain status is not 'active' yet"),
							),
						),
						h.If(!domainAlreadyAttached,
							h.Small(
								h.Style("color: var(--pico-muted-color);"),
								h.Text("Not yet attached - click 'Attach Domain' button below"),
							),
						),
					),
				),
			),

			// Action Buttons - Check Status & Attach Domain
			h.Div(
				h.Style("margin-bottom: 1rem; display: flex; gap: 1rem; flex-wrap: wrap;"),
				// Check Domain Status button (always visible)
				h.Button(
					h.Attr("class", "secondary outline"),
					h.If(isRefreshing.String() == "true", h.Attr("aria-busy", "true")),
					h.If(isRefreshing.String() == "true", h.Attr("disabled", "disabled")),
					h.Text(func() string {
						if isRefreshing.String() == "true" {
							return "Checking..."
						}
						return "🔍 Check Domain Status"
					}()),
					refreshDomainsAction.OnClick(),
				),
				// Attach Domain Button - only show if domain is configured but NOT yet attached
				h.If(customDomain != "" && !env.IsPlaceholder(customDomain) && !domainAlreadyAttached,
					h.Button(
						h.If(isAttaching.String() == "true", h.Attr("aria-busy", "true")),
						h.If(isAttaching.String() == "true", h.Attr("disabled", "disabled")),
						h.Text(func() string {
							if isAttaching.String() == "true" {
								return "Attaching Domain..."
							}
							return "🔗 Attach " + customDomain
						}()),
						attachDomainAction.OnClick(),
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

			// Attached Domains List
			h.If(len(domainsList) > 0,
				h.Div(
					h.Style("margin-top: 2rem;"),
					h.Div(
						h.Style("display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;"),
						h.H3(
							h.Style("margin: 0;"),
							h.Text("🌐 Attached Custom Domains"),
						),
						h.Button(
							h.Attr("class", "secondary outline"),
							h.If(isRefreshing.String() == "true", h.Attr("aria-busy", "true")),
							h.If(isRefreshing.String() == "true", h.Attr("disabled", "disabled")),
							h.Text(func() string {
								if isRefreshing.String() == "true" {
									return "Refreshing..."
								}
								return "🔄 Refresh Status"
							}()),
							refreshDomainsAction.OnClick(),
						),
					),
					h.Div(domainListElements...),
				),
			),

			// Use wizard navigation component (Step 5 is last step - shows completion link)
			RenderWizardNavigation(&CloudflareWizard, &Step5Info, nil),
		)
	})
}
