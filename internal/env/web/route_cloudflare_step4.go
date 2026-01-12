package web

import (
	"fmt"
	"log"
	"strings"

	"github.com/go-via/via"
	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// Step4Info is the metadata for Step 4 - single source of truth
var Step4Info = WizardStepInfo{
	StepNumber:  4,
	Path:        "/cloudflare/step4",
	Title:       "Project Selection",
	Description: "Select or create a Cloudflare Pages project",
	Fields:      []string{env.KeyCloudflarePageProject},
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

// cloudflareStep4Page - Project details (Step 4 of 5)
func cloudflareStep4Page(c *via.Context, cfg *env.EnvConfig, mockMode bool) {
	svc := env.NewService(mockMode)

	// All fields for final save (token fields already set in previous steps)
	fields := CreateFormFields(c, cfg, []string{
		env.KeyCloudflareAPIToken,
		env.KeyCloudflareAPITokenName,
		env.KeyCloudflareAccountID,
		env.KeyCloudflareDomain,
		env.KeyCloudflareZoneID,
		env.KeyCloudflarePageProject,
	})

	saveMessage := c.Signal("")
	projectsMessage := c.Signal("")      // For projects loading status
	deleteMessage := c.Signal("")        // For delete operation feedback
	projectToDelete := c.Signal("")      // Holds project name pending deletion confirmation
	showDeleteConfirm := c.Signal(false) // Controls visibility of delete confirmation dialog
	createMessage := c.Signal("")        // For project creation feedback
	createInProgress := c.Signal(false)  // For project creation in-progress state
	newProjectName := c.Signal("")       // Holds new project name input

	// Projects loader - populated lazily when first accessed
	projectsLoader := NewLazyLoader(func() ([]env.PagesProject, error) {
		token := cfg.Get(env.KeyCloudflareAPIToken)
		accountID := cfg.Get(env.KeyCloudflareAccountID)

		if mockMode {
			// Mock data for testing
			return []env.PagesProject{
				{Name: "my-hugo-site", CreatedOn: "2024-01-15T10:00:00Z"},
				{Name: "ubuntusoftware-net", CreatedOn: "2024-02-20T14:30:00Z"},
				{Name: "test-project", CreatedOn: "2024-03-10T09:15:00Z"},
			}, nil
		}

		if token == "" || accountID == "" || env.IsPlaceholder(token) || env.IsPlaceholder(accountID) {
			return []env.PagesProject{}, nil
		}

		return env.ListPagesProjects(token, accountID)
	})

	// Cancel delete operation
	cancelDeleteAction := c.Action(func() {
		projectToDelete.SetValue("")
		showDeleteConfirm.SetValue(false)
		deleteMessage.SetValue("")
		c.Sync()
	})

	// Confirm and execute delete
	confirmDeleteAction := c.Action(func() {
		projectName := projectToDelete.String()
		if projectName == "" {
			deleteMessage.SetValue("error:No project selected for deletion")
			c.Sync()
			return
		}

		// Get credentials from config
		token := cfg.Get(env.KeyCloudflareAPIToken)
		accountID := cfg.Get(env.KeyCloudflareAccountID)

		if mockMode {
			// Mock mode - simulate successful deletion
			deleteMessage.SetValue("success:Project '" + projectName + "' deleted successfully (mock mode)")
			showDeleteConfirm.SetValue(false)
			projectToDelete.SetValue("")
			c.Sync()
			// In real app, would reload page to refresh project list
			c.ExecScript("setTimeout(() => window.location.reload(), 1500)")
			return
		}

		// Call delete API with automatic custom domain cleanup
		removedDomains, err := env.DeletePagesProjectWithCleanup(token, accountID, projectName)
		if err != nil {
			deleteMessage.SetValue("error:Failed to delete project: " + err.Error())
			c.Sync()
			return
		}

		// Success message includes info about removed domains
		successMsg := "Project '" + projectName + "' deleted successfully!"
		if len(removedDomains) > 0 {
			successMsg += " (Removed " + fmt.Sprintf("%d", len(removedDomains)) + " custom domain(s) first)"
		}
		successMsg += " Refreshing..."

		deleteMessage.SetValue("success:" + successMsg)
		showDeleteConfirm.SetValue(false)
		projectToDelete.SetValue("")
		c.Sync()

		// Reload page to refresh project list
		c.ExecScript("setTimeout(() => window.location.reload(), 1500)")
	})

	// Create project action
	createProjectAction := c.Action(func() {
		projectName := newProjectName.String()
		if projectName == "" {
			createMessage.SetValue("error:Please enter a project name")
			c.Sync()
			return
		}

		createInProgress.SetValue(true)
		createMessage.SetValue("Creating project '" + projectName + "'...\n")
		c.Sync()

		result := env.CreatePagesProject(projectName, mockMode)

		createInProgress.SetValue(false)
		if result.Error != nil {
			createMessage.SetValue("error:" + result.Output + "\nError: " + result.Error.Error())
		} else {
			createMessage.SetValue("success:" + result.Output + "\n\nRefreshing page to show new project...")
			c.Sync()
			// Reload page to refresh project list
			c.ExecScript("setTimeout(() => window.location.reload(), 2000)")
			return
		}
		c.Sync()
	})

	// Finish action - save everything
	finishAction := c.Action(func() {
		saveMessage.SetValue("")

		fieldUpdates := map[string]string{
			env.KeyCloudflareAPIToken:     fields[0].ValueSignal.String(),
			env.KeyCloudflareAPITokenName: fields[1].ValueSignal.String(),
			env.KeyCloudflareAccountID:    fields[2].ValueSignal.String(),
			env.KeyCloudflareDomain:       fields[3].ValueSignal.String(),
			env.KeyCloudflareZoneID:       fields[4].ValueSignal.String(),
			env.KeyCloudflarePageProject:  fields[5].ValueSignal.String(),
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
			saveMessage.SetValue("error:Please fix validation errors before saving")
			c.Sync()
			return
		}

		// Success! Redirect to Step 5 (Custom Domain Setup)
		saveMessage.SetValue("success:✅ Configuration saved successfully!")
		c.Sync()
		c.ExecScript("window.location.href = '/cloudflare/step5'")
	})

	c.View(func() h.H {
		// Use step's own metadata for prerequisite checking
		missingPrereqs := CheckPrerequisites(cfg, Step4Info.Prerequisites)

		// Load projects using LazyLoader
		projectsCache, projectsErr := projectsLoader.Get()
		if projectsErr != nil {
			log.Printf("Failed to fetch Pages projects: %v", projectsErr)
			projectsMessage.SetValue("error:Failed to load projects: " + projectsErr.Error())
		} else if len(projectsCache) == 0 {
			projectsMessage.SetValue("info:No projects found in this account")
		}

		// Build dropdown options from projects
		projectOptions := make([]SelectOption, 0, len(projectsCache)+1)
		projectOptions = append(projectOptions, SelectOption{Value: "", Label: "-- Select a project --"})
		for _, project := range projectsCache {
			projectOptions = append(projectOptions, SelectOption{Value: project.Name, Label: project.Name})
		}

		// Build project list UI elements with delete buttons
		projectListElements := make([]h.H, 0, len(projectsCache))
		for _, project := range projectsCache {
			projectName := project.Name    // Capture in closure
			createdOn := project.CreatedOn // Capture in closure
			deleteAction := c.Action(func() {
				projectToDelete.SetValue(projectName)
				showDeleteConfirm.SetValue(true)
				deleteMessage.SetValue("")
				c.Sync()
			})

			projectListElements = append(projectListElements, h.Div(
				h.Style("display: flex; justify-content: space-between; align-items: center; padding: 0.75rem; background: var(--pico-card-background-color); border-radius: 0.25rem;"),
				h.Div(
					h.Strong(h.Text(projectName)),
					h.Small(
						h.Style("margin-left: 1rem; color: var(--pico-muted-color);"),
						h.Text("Created: "+createdOn),
					),
				),
				h.Button(
					h.Attr("class", "secondary outline"),
					h.Text("Delete"),
					deleteAction.OnClick(),
				),
			))
		}

		return h.Main(
			h.Class("container"),

			// Use metadata for header
			RenderWizardStepHeader(&Step4Info, len(CloudflareWizard.Steps)),

			RenderNavigation("cloudflare"),

			// Add breadcrumb navigation
			RenderWizardBreadcrumbs(&CloudflareWizard, cfg, Step4Info.StepNumber),

			// Show prerequisite error banner if missing
			RenderPrerequisiteError(missingPrereqs),

			h.H2(h.Text("Cloudflare Pages Project")),
			h.P(h.Text("Select a Pages project to deploy your Hugo site to.")),

			// Show projects loading status - error message
			h.If(strings.HasPrefix(projectsMessage.String(), "error:"),
				h.Article(
					h.Style("background-color: var(--pico-del-background); border-left: 4px solid var(--pico-del-color); padding: 1rem; margin-bottom: 1rem;"),
					h.P(
						h.Style("margin: 0; color: var(--pico-del-color);"),
						h.Text(strings.TrimPrefix(projectsMessage.String(), "error:")),
					),
				),
			),

			// Project dropdown (always shown, even if empty - will have placeholder option)
			h.H3(h.Text("Choose Project:")),
			RenderSelectField("Project", fields[5].ValueSignal, projectOptions),
			h.Small(
				h.Style("color: var(--pico-muted-color);"),
				h.Text("Select a Pages project to deploy your Hugo site to"),
			),

			// Manage Projects section - delete existing projects
			h.If(len(projectsCache) > 0,
				h.Div(
					h.Style("margin-top: 3rem; padding-top: 2rem; border-top: 1px solid var(--pico-muted-border-color);"),
					h.H2(h.Text("Manage Existing Projects")),
					h.P(h.Text("Delete projects you no longer need:")),

					// Delete confirmation dialog
					h.If(showDeleteConfirm.String() == "true",
						h.Dialog(
							h.Attr("open", "open"),
							h.Article(
								h.H3(h.Text("Confirm Deletion")),
								h.P(
									h.Text("Are you sure you want to delete the project "),
									h.Strong(h.Text(projectToDelete.String())),
									h.Text("?"),
								),
								h.P(
									h.Style("color: var(--pico-del-color);"),
									h.Text("⚠️ This action cannot be undone. All deployments and settings for this project will be permanently deleted."),
								),
								h.Div(
									h.Style("display: flex; gap: 1rem; justify-content: flex-end;"),
									h.Button(
										h.Attr("class", "secondary"),
										h.Text("Cancel"),
										cancelDeleteAction.OnClick(),
									),
									h.Button(
										h.Attr("class", "contrast"),
										h.Text("Delete Project"),
										confirmDeleteAction.OnClick(),
									),
								),
							),
						),
					),

					// List of projects with delete buttons
					func() h.H {
						listChildren := []h.H{h.Style("display: grid; gap: 0.5rem;")}
						listChildren = append(listChildren, projectListElements...)
						return h.Div(listChildren...)
					}(),

					// Show delete messages
					h.If(strings.HasPrefix(deleteMessage.String(), "success:"),
						h.Article(
							h.Style("background-color: var(--pico-ins-background); border-left: 4px solid var(--pico-ins-color); padding: 1rem; margin-top: 1rem;"),
							h.P(
								h.Style("margin: 0; color: var(--pico-ins-color);"),
								h.Text(strings.TrimPrefix(deleteMessage.String(), "success:")),
							),
						),
					),
					h.If(strings.HasPrefix(deleteMessage.String(), "error:"),
						h.Article(
							h.Style("background-color: var(--pico-del-background); border-left: 4px solid var(--pico-del-color); padding: 1rem; margin-top: 1rem;"),
							h.P(
								h.Style("margin: 0; color: var(--pico-del-color);"),
								h.Text(strings.TrimPrefix(deleteMessage.String(), "error:")),
							),
						),
					),
				),
			),

			// Create new project section
			h.Div(
				h.Style("margin-top: 3rem; padding-top: 2rem; border-top: 1px solid var(--pico-muted-border-color);"),
				h.H2(h.Text("Create New Project")),
				h.P(h.Text("Create a new Cloudflare Pages project using Wrangler CLI.")),

				h.Div(
					h.Style("margin-bottom: 2rem;"),
					h.Div(
						h.Style("display: flex; gap: 1rem; align-items: flex-end;"),
						h.Div(
							h.Style("flex: 1;"),
							h.Label(h.Text("Project Name")),
							h.Input(
								h.Attr("type", "text"),
								h.Attr("placeholder", "my-hugo-site"),
								newProjectName.Bind(),
							),
							h.Small(
								h.Style("color: var(--pico-muted-color);"),
								h.Text("Lowercase letters, numbers, and hyphens only (1-63 chars)"),
							),
						),
						h.Button(
							h.Text("Create Project"),
							h.If(createInProgress.String() == "true", h.Attr("aria-busy", "true")),
							h.If(createInProgress.String() == "true", h.Attr("disabled", "disabled")),
							createProjectAction.OnClick(),
						),
					),

					// Output display for project creation
					h.If(createMessage.String() != "",
						h.Div(
							h.Style("margin-top: 1.5rem;"),
							// Success output
							h.If(strings.HasPrefix(createMessage.String(), "success:"),
								h.Article(
									h.Style("background-color: var(--pico-ins-background); border-left: 4px solid var(--pico-ins-color); padding: 1rem;"),
									h.Pre(
										h.Style("margin: 0; white-space: pre-wrap; font-size: 0.875rem; color: var(--pico-ins-color);"),
										h.Text(strings.TrimPrefix(createMessage.String(), "success:")),
									),
								),
							),
							// Error output
							h.If(strings.HasPrefix(createMessage.String(), "error:"),
								h.Article(
									h.Style("background-color: var(--pico-del-background); border-left: 4px solid var(--pico-del-color); padding: 1rem;"),
									h.Pre(
										h.Style("margin: 0; white-space: pre-wrap; font-size: 0.875rem; color: var(--pico-del-color);"),
										h.Text(strings.TrimPrefix(createMessage.String(), "error:")),
									),
								),
							),
							// In-progress output
							h.If(!strings.HasPrefix(createMessage.String(), "success:") && !strings.HasPrefix(createMessage.String(), "error:"),
								h.Article(
									h.Style("background-color: var(--pico-card-background-color); border-left: 4px solid var(--pico-primary); padding: 1rem;"),
									h.Pre(
										h.Style("margin: 0; white-space: pre-wrap; font-size: 0.875rem;"),
										h.Text(createMessage.String()),
									),
								),
							),
						),
					),
				),
			),

			// Use wizard navigation component
			RenderWizardNavigation(&CloudflareWizard, &Step4Info, finishAction),

			RenderErrorMessage(saveMessage),
			RenderSuccessMessage(saveMessage),
		)
	})
}
