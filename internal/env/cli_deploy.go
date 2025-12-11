package env

import (
	"fmt"
)

// RunBuild runs Hugo build only (no deployment) for CLI
func RunBuild() error {
	fmt.Println("Building Hugo site...")
	fmt.Println()

	result := BuildHugoSite(false)

	fmt.Println(result.Output)

	if result.Error != nil {
		return fmt.Errorf("build failed: %w", result.Error)
	}

	if result.LocalURL != "" {
		fmt.Printf("\n✓ Build complete!\n")
		fmt.Printf("\nLocal preview available at:\n  %s\n", result.LocalURL)
		if result.LANURL != "" {
			fmt.Printf("\nMobile/LAN preview available at:\n  %s\n", result.LANURL)
		}
	}

	return nil
}

// RunDeployPreview runs build + deploy to Cloudflare Pages preview for CLI
func RunDeployPreview() error {
	// Load config to get project name
	svc := NewService(false)
	cfg, err := svc.GetCurrentConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	projectName := cfg.Get(KeyCloudflarePageProject)
	if projectName == "" || IsPlaceholder(projectName) {
		return fmt.Errorf("no Cloudflare Pages project configured. Run 'web-gui' and complete Step 4 first")
	}

	fmt.Printf("Building and deploying to Cloudflare Pages (preview)...\n")
	fmt.Printf("Project: %s\n", projectName)
	fmt.Println()

	// Run build and deploy (no branch = preview only)
	result := BuildAndDeploy(projectName, "", false)

	fmt.Println(result.Output)

	if result.Error != nil {
		return fmt.Errorf("deployment failed: %w", result.Error)
	}

	// Print URLs
	fmt.Printf("\n✓ Deployment complete!\n")
	if result.PreviewURL != "" {
		fmt.Printf("\nPreview URL:\n  %s\n", result.PreviewURL)
	}

	return nil
}

// RunDeployProduction runs build + deploy to Cloudflare Pages production for CLI
func RunDeployProduction() error {
	// Load config to get project name and custom domain
	svc := NewService(false)
	cfg, err := svc.GetCurrentConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	projectName := cfg.Get(KeyCloudflarePageProject)
	if projectName == "" || IsPlaceholder(projectName) {
		return fmt.Errorf("no Cloudflare Pages project configured. Run 'web-gui' and complete Step 4 first")
	}

	customDomain := cfg.Get(KeyCloudflareDomain)

	fmt.Printf("Building and deploying to Cloudflare Pages (production)...\n")
	fmt.Printf("Project: %s\n", projectName)
	if customDomain != "" && !IsPlaceholder(customDomain) {
		fmt.Printf("Custom domain: %s\n", customDomain)
	}
	fmt.Println()

	// Run build and deploy (branch=main = production)
	result := BuildAndDeploy(projectName, "main", false)

	fmt.Println(result.Output)

	if result.Error != nil {
		return fmt.Errorf("deployment failed: %w", result.Error)
	}

	// Print URLs
	fmt.Printf("\n✓ Deployment complete!\n")
	if result.PreviewURL != "" {
		fmt.Printf("\nPreview URL:\n  %s\n", result.PreviewURL)
	}

	// Show custom domain if configured
	if customDomain != "" && !IsPlaceholder(customDomain) {
		fmt.Printf("\nProduction URL (custom domain):\n  https://%s\n", customDomain)
	} else if result.DeploymentURL != "" {
		fmt.Printf("\nProduction URL:\n  %s\n", result.DeploymentURL)
	}

	return nil
}

// RunDomainStatus checks and displays custom domain status for CLI
func RunDomainStatus() error {
	// Load config
	svc := NewService(false)
	cfg, err := svc.GetCurrentConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	apiToken := cfg.Get(KeyCloudflareAPIToken)
	accountID := cfg.Get(KeyCloudflareAccountID)
	projectName := cfg.Get(KeyCloudflarePageProject)
	customDomain := cfg.Get(KeyCloudflareDomain)

	// Validate configuration
	if apiToken == "" || IsPlaceholder(apiToken) {
		return fmt.Errorf("Cloudflare API Token not configured. Run 'web-gui' and complete Step 1 first")
	}
	if accountID == "" || IsPlaceholder(accountID) {
		return fmt.Errorf("Account ID not configured. Run 'web-gui' and complete Step 2 first")
	}
	if projectName == "" || IsPlaceholder(projectName) {
		return fmt.Errorf("Project Name not configured. Run 'web-gui' and complete Step 4 first")
	}

	fmt.Printf("Checking custom domain status...\n")
	fmt.Printf("Project: %s\n", projectName)
	if customDomain != "" && !IsPlaceholder(customDomain) {
		fmt.Printf("Configured domain: %s\n", customDomain)
	}
	fmt.Println()

	// Fetch domains from Cloudflare Pages API
	domains, err := ListPagesDomains(apiToken, accountID, projectName)
	if err != nil {
		return fmt.Errorf("failed to fetch domains: %w", err)
	}

	if len(domains) == 0 {
		fmt.Println("No custom domains attached to this project.")
		fmt.Println()
		fmt.Println("To attach a custom domain:")
		fmt.Println("  1. Configure your domain in Step 3 of the web GUI")
		fmt.Println("  2. Attach it in Step 5 of the web GUI")
		return nil
	}

	// Display domains
	fmt.Printf("Attached domains (%d):\n", len(domains))
	fmt.Println()

	for _, domain := range domains {
		statusSymbol := ""
		statusMessage := ""

		switch domain.Status {
		case "active":
			statusSymbol = "✓"
			statusMessage = "Domain is active and ready to use"
		case "initializing":
			statusSymbol = "⏳"
			statusMessage = "Domain is initializing (DNS + SSL certificate provisioning)"
		case "pending":
			statusSymbol = "⏳"
			statusMessage = "Domain is pending (waiting for DNS propagation or certificate)"
		default:
			statusSymbol = "?"
			statusMessage = fmt.Sprintf("Unknown status: %s", domain.Status)
		}

		fmt.Printf("  %s %s\n", statusSymbol, domain.Name)
		fmt.Printf("    Status: %s\n", domain.Status)
		fmt.Printf("    %s\n", statusMessage)

		// Error 1014 troubleshooting if not active
		if domain.Status != "active" {
			fmt.Println()
			fmt.Println("    ⚠️  If you see Error 1014 (CNAME Cross-User Banned):")
			fmt.Println("       - The domain is still being set up by Cloudflare")
			fmt.Println("       - This can take 10-30 minutes for certificate provisioning")
			fmt.Println("       - Check status again in a few minutes")
			fmt.Println("       - Once status is 'active', Error 1014 will be resolved")
		} else {
			fmt.Printf("    ✅ Visit: https://%s\n", domain.Name)
		}
		fmt.Println()
	}

	// Show preview URL
	fmt.Println("Preview URL (always available):")
	fmt.Printf("  https://%s.pages.dev\n", projectName)
	fmt.Println()

	return nil
}
