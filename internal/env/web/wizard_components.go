package web

import (
	"fmt"

	"github.com/go-via/via/h"
	"github.com/joeblew999/xplat/internal/env"
)

// RenderWizardStepHeader renders the consistent step header
func RenderWizardStepHeader(step *WizardStepInfo, totalSteps int) h.H {
	return h.Div(
		h.H1(h.Text(fmt.Sprintf("Cloudflare Setup - Step %d of %d", step.StepNumber, totalSteps))),
		h.P(h.Text(step.Title)),
	)
}

// RenderWizardNavigation renders previous/next navigation buttons
// nextAction should be a via.Action if validation is needed before proceeding, or nil for direct navigation
func RenderWizardNavigation(registry *WizardRegistry, currentStep *WizardStepInfo, nextAction interface{}) h.H {
	prevStep := registry.GetPrevStep(currentStep.StepNumber)
	nextStep := registry.GetNextStep(currentStep.StepNumber)

	elements := []h.H{h.Style("margin-top: 2rem;")}

	// Back button
	if prevStep != nil {
		elements = append(elements,
			h.A(h.Href(prevStep.Path), h.Text("← Back: "+prevStep.Title)),
			h.Text(" "),
		)
	}

	// Next button
	if nextStep != nil {
		if nextAction != nil {
			// Action-based navigation (validates before proceeding)
			// nextAction must have an OnClick() method
			type clickable interface{ OnClick() h.H }
			if action, ok := nextAction.(clickable); ok {
				elements = append(elements,
					h.Button(h.Text("Next: "+nextStep.Title+" →"), action.OnClick()),
					h.Text(" or "),
					h.A(h.Href(nextStep.Path), h.Text("Skip")),
				)
			} else {
				// Fallback to direct link if type assertion fails
				elements = append(elements,
					h.A(h.Href(nextStep.Path), h.Attr("role", "button"), h.Text("Next: "+nextStep.Title+" →")),
				)
			}
		} else {
			// Direct link navigation
			elements = append(elements,
				h.A(h.Href(nextStep.Path), h.Attr("role", "button"), h.Text("Next: "+nextStep.Title+" →")),
			)
		}
	} else {
		// Last step - show completion link
		elements = append(elements,
			h.A(
				h.Href("/deploy"),
				h.Attr("role", "button"),
				h.Text("✅ Complete Setup - Go to Deploy →"),
			),
		)
	}

	return h.Div(elements...)
}

// RenderWizardBreadcrumbs renders a visual breadcrumb navigation with completion status
func RenderWizardBreadcrumbs(registry *WizardRegistry, cfg *env.EnvConfig, currentStepNumber int) h.H {
	breadcrumbItems := make([]h.H, 0)

	for i, step := range registry.Steps {
		status := registry.GetStepStatus(cfg, step)
		isCurrent := step.StepNumber == currentStepNumber

		// Build breadcrumb item
		var itemContent h.H
		if isCurrent {
			// Current step - bold, not clickable
			itemContent = h.Strong(
				h.Style("color: var(--pico-primary);"),
				h.Text(fmt.Sprintf("%d. %s", step.StepNumber, step.Title)),
			)
		} else {
			// Other steps - clickable link
			itemContent = h.A(
				h.Href(step.Path),
				h.Text(fmt.Sprintf("%d. %s", step.StepNumber, step.Title)),
			)
		}

		// Add status indicator
		var statusIcon string
		var statusColor string
		switch status {
		case StepStatusVerified:
			statusIcon = "✔️"
			statusColor = "var(--pico-ins-color)"
		case StepStatusFilled:
			statusIcon = "✓"
			statusColor = "var(--pico-primary)"
		case StepStatusIncomplete:
			statusIcon = "○"
			statusColor = "var(--pico-muted-color)"
		}

		breadcrumbItems = append(breadcrumbItems, h.Div(
			h.Style("display: flex; align-items: center; gap: 0.5rem;"),
			h.Span(
				h.Style(fmt.Sprintf("color: %s; font-weight: bold;", statusColor)),
				h.Text(statusIcon),
			),
			itemContent,
		))

		// Add separator except after last item
		if i < len(registry.Steps)-1 {
			breadcrumbItems = append(breadcrumbItems, h.Div(
				h.Style("color: var(--pico-muted-color); margin: 0 0.5rem;"),
				h.Text("→"),
			))
		}
	}

	// Build final div with breadcrumb items
	breadcrumbDiv := func() h.H {
		elements := []h.H{h.Style("display: flex; flex-wrap: wrap; align-items: center; gap: 0.5rem;")}
		elements = append(elements, breadcrumbItems...)
		return h.Div(elements...)
	}()

	return h.Nav(
		h.Style("background-color: var(--pico-card-background-color); padding: 1rem; border-radius: 0.5rem; margin-bottom: 2rem;"),
		breadcrumbDiv,
	)
}

// RenderStepProgress renders a progress bar showing overall completion
func RenderStepProgress(completedSteps, totalSteps int) h.H {
	percentage := float64(completedSteps) / float64(totalSteps) * 100

	return h.Div(
		h.Style("margin-bottom: 2rem;"),
		h.Div(
			h.Style("background-color: var(--pico-muted-border-color); height: 0.5rem; border-radius: 0.25rem; overflow: hidden;"),
			h.Div(
				h.Style(fmt.Sprintf(
					"background-color: var(--pico-primary); height: 100%%; width: %.1f%%; transition: width 0.3s ease;",
					percentage,
				)),
			),
		),
		h.Small(
			h.Style("color: var(--pico-muted-color);"),
			h.Text(fmt.Sprintf("Progress: %d of %d steps complete", completedSteps, totalSteps)),
		),
	)
}

// RenderStepCard renders a single step overview card for the landing page
func RenderStepCard(step *WizardStepInfo, status StepStatus) h.H {
	// Determine status styling
	var statusText, statusColor, statusBg, statusBorder string
	switch status {
	case StepStatusVerified:
		statusText = "✔️ Verified"
		statusColor = "var(--pico-ins-color)"
		statusBg = "rgba(46, 125, 50, 0.15)"
		statusBorder = "var(--pico-ins-color)"
	case StepStatusFilled:
		statusText = "✓ Filled"
		statusColor = "var(--pico-primary)"
		statusBg = "rgba(59, 130, 246, 0.1)"
		statusBorder = "var(--pico-primary)"
	case StepStatusIncomplete:
		statusText = "○ Incomplete"
		statusColor = "var(--pico-muted-color)"
		statusBg = "var(--pico-card-background-color)"
		statusBorder = "var(--pico-muted-border-color)"
	}

	return h.Article(
		h.Style("border-left: 4px solid "+statusBorder+"; background: "+statusBg+";"),
		h.Div(
			h.Style("display: flex; justify-content: space-between; align-items: start;"),
			h.Div(
				h.H4(
					h.Style("margin: 0 0 0.5rem 0;"),
					h.Text(fmt.Sprintf("Step %d: %s", step.StepNumber, step.Title)),
				),
				h.P(
					h.Style("margin: 0; color: var(--pico-muted-color);"),
					h.Text(step.Description),
				),
			),
			h.Div(
				h.Style("display: flex; flex-direction: column; align-items: flex-end; gap: 0.5rem;"),
				h.Span(
					h.Style("color: "+statusColor+"; font-weight: 600;"),
					h.Text(statusText),
				),
				h.A(
					h.Href(step.Path),
					h.Attr("role", "button"),
					h.Attr("class", "outline"),
					h.Text("Configure"),
				),
			),
		),
	)
}
