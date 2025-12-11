package web

import (
	"log"
	"strings"

	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// Step3Info is the metadata for Step 3 - single source of truth
var Step3Info = WizardStepInfo{
	StepNumber:  3,
	Path:        "/cloudflare/step3",
	Title:       "Domain Selection",
	Description: "Choose which domain to deploy to",
	Fields:      []string{env.KeyCloudflareDomain, env.KeyCloudflareZoneID},
	Prerequisites: []PrerequisiteCheck{
		{
			FieldKey:    env.KeyCloudflareAPIToken,
			DisplayName: "Cloudflare API Token",
			StepPath:    "/cloudflare/step1",
			StepLabel:   "Configure in Step 1",
		},
		{
			FieldKey:    env.KeyCloudflareAccountID,
			DisplayName: "Cloudflare Account ID",
			StepPath:    "/cloudflare/step2",
			StepLabel:   "Configure in Step 2",
		},
	},
}

// cloudflareStep3Page - Domain selection (Step 3 of 5)
func cloudflareStep3Page(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	svc := env.NewService(mockMode)

	// Fields for all previously entered data plus domain/zone
	fields := CreateFormFields(c, cfg, []string{
		env.KeyCloudflareAPIToken,
		env.KeyCloudflareAccountID,
		env.KeyCloudflareDomain,
		env.KeyCloudflareZoneID,
	})

	// Pre-populate domain dropdown with saved domain+zone if both exist
	// fields[2] is the domain field - we need to set it to "domain|zoneID" format for the dropdown
	savedDomain := cfg.Get(env.KeyCloudflareDomain)
	savedZoneID := cfg.Get(env.KeyCloudflareZoneID)
	if savedDomain != "" && savedZoneID != "" && !env.IsPlaceholder(savedDomain) && !env.IsPlaceholder(savedZoneID) {
		// Replace fields[2] with the combined value for dropdown binding
		fields[2] = FormFieldData{
			EnvKey:       env.KeyCloudflareDomain,
			ValueSignal:  c.Signal(savedDomain + "|" + savedZoneID),
			StatusSignal: fields[2].StatusSignal,
		}
	}

	saveMessage := c.Signal("")
	zonesMessage := c.Signal("") // For zones loading status

	// Zones loader - populated lazily when first accessed
	zonesLoader := NewLazyLoader(func() ([]env.Zone, error) {
		token := cfg.Get(env.KeyCloudflareAPIToken)
		accountID := cfg.Get(env.KeyCloudflareAccountID)

		if mockMode {
			// Mock data for testing
			return []env.Zone{
				{ID: "mock-zone-1", Name: "example.com"},
				{ID: "mock-zone-2", Name: "example.net"},
				{ID: "mock-zone-3", Name: "example.org"},
				{ID: "mock-zone-4", Name: "ubuntusoftware.net"},
				{ID: "mock-zone-5", Name: "mysite.com"},
				{ID: "mock-zone-6", Name: "testdomain.io"},
			}, nil
		}

		if token == "" || accountID == "" || env.IsPlaceholder(token) || env.IsPlaceholder(accountID) {
			return []env.Zone{}, nil
		}

		return env.ListZones(token, accountID)
	})

	// Next action - save domain selection and go to step 4
	nextAction := c.Action(func() {
		saveMessage.SetValue("")

		// Parse selected domain value (format: "domain.com|zone-id")
		selectedValue := fields[2].ValueSignal.String()
		if selectedValue == "" {
			saveMessage.SetValue("error:Please select a domain")
			c.Sync()
			return
		}

		// Split domain|zone-id
		parts := strings.Split(selectedValue, "|")
		if len(parts) != 2 {
			saveMessage.SetValue("error:Invalid domain selection")
			c.Sync()
			return
		}

		domain := parts[0]
		zoneID := parts[1]

		fieldUpdates := map[string]string{
			env.KeyCloudflareAPIToken:  fields[0].ValueSignal.String(),
			env.KeyCloudflareAccountID: fields[1].ValueSignal.String(),
			env.KeyCloudflareDomain:    domain,
			env.KeyCloudflareZoneID:    zoneID,
		}

		results, err := svc.ValidateAndUpdateFields(fieldUpdates)

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

		// Success - redirect to step 4
		saveMessage.SetValue("success:Domain selected! Moving to step 4...")
		c.Sync()
		c.ExecScript("window.location.href = '/cloudflare/step4'")
	})

	c.View(func() h.H {
		// Use step's own metadata for prerequisite checking
		missingPrereqs := CheckPrerequisites(cfg, Step3Info.Prerequisites)

		// Load zones using LazyLoader
		zonesCache, zonesErr := zonesLoader.Get()
		if zonesErr != nil {
			log.Printf("Failed to fetch zones: %v", zonesErr)
			zonesMessage.SetValue("error:Failed to load domains: " + zonesErr.Error())
		} else if len(zonesCache) == 0 {
			zonesMessage.SetValue("info:No domains found in this account")
		}

		// Build dropdown options from zones
		domainOptions := make([]SelectOption, 0, len(zonesCache)+1)
		domainOptions = append(domainOptions, SelectOption{Value: "", Label: "-- Select a domain --"})
		for _, zone := range zonesCache {
			domainOptions = append(domainOptions, SelectOption{Value: zone.Name + "|" + zone.ID, Label: zone.Name})
		}

		// Build smart "Add Site" URL with account ID if available
		accountID := cfg.Get(env.KeyCloudflareAccountID)
		addSiteURL := BuildCloudflareURL(env.CloudflareAddSiteURL, accountID)

		return h.Main(
			h.Class("container"),

			// Use metadata for header
			RenderWizardStepHeader(&Step3Info, len(CloudflareWizard.Steps)),

			RenderNavigation("cloudflare"),

			// Add breadcrumb navigation
			RenderWizardBreadcrumbs(&CloudflareWizard, cfg, Step3Info.StepNumber),

			// Show prerequisite error banner if missing
			RenderPrerequisiteError(missingPrereqs),

			h.H2(h.Text("Select Your Domain")),
			h.P(h.Text("Choose which domain you want to deploy your Hugo site to.")),

			// Show zones loading status - info message
			h.If(zonesMessage.String() == "info:No domains found in this account",
				h.Article(
					h.Style("background-color: var(--pico-ins-background); border-left: 4px solid var(--pico-ins-color); padding: 1rem; margin-bottom: 1rem;"),
					h.P(
						h.Style("margin: 0;"),
						h.Text("No domains found in this account. "),
						RenderExternalLink(addSiteURL, "Add a domain"),
						h.Text(" to Cloudflare first."),
					),
				),
			),
			// Show zones loading status - error message
			h.If(strings.HasPrefix(zonesMessage.String(), "error:"),
				h.Article(
					h.Style("background-color: var(--pico-del-background); border-left: 4px solid var(--pico-del-color); padding: 1rem; margin-bottom: 1rem;"),
					h.P(
						h.Style("margin: 0; color: var(--pico-del-color);"),
						h.Text(strings.TrimPrefix(zonesMessage.String(), "error:")),
					),
				),
			),

			// Domain dropdown
			h.If(len(domainOptions) > 1,
				h.Div(
					h.H3(h.Text("Choose Domain:")),
					RenderSelectField("Domain", fields[2].ValueSignal, domainOptions),
					h.Small(
						h.Style("color: var(--pico-muted-color);"),
						h.Text("The domain where your Hugo site will be deployed"),
					),
				),
			),

			// Use wizard navigation component - only show next button if domains available
			h.If(len(domainOptions) > 1,
				RenderWizardNavigation(&CloudflareWizard, &Step3Info, nextAction),
			),
			h.If(len(domainOptions) <= 1,
				RenderWizardNavigation(&CloudflareWizard, &Step3Info, nil),
			),

			RenderErrorMessage(saveMessage),
			RenderSuccessMessage(saveMessage),
		)
	})
}
