package env

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CommandOutput represents streaming command output
type CommandOutput struct {
	Output        string // Combined stdout/stderr output
	Error         error  // Command execution error
	LocalURL      string // Local preview URL (e.g., "https://localhost:1313")
	LANURL        string // LAN preview URL for mobile testing (e.g., "https://192.168.1.100:1313")
	PreviewURL    string // Cloudflare Pages preview URL (e.g., "https://abc123.project.pages.dev")
	DeploymentURL string // Cloudflare Pages production URL (custom domain, e.g., "https://www.ubuntusoftware.net")
}

// extractDeploymentURL extracts the Cloudflare Pages deployment URL from wrangler output
func extractDeploymentURL(output string) string {
	// Wrangler output typically contains: "✨ Deployment complete! Take a peek over at https://abc123.project-name.pages.dev"
	// Pattern: https://[hash].[project].pages.dev (e.g., https://d59e4628.bbb-4ha.pages.dev)
	re := regexp.MustCompile(`https://[a-z0-9-]+\.[a-z0-9-]+\.pages\.dev`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// extractCustomDomainURL extracts custom domain URLs from wrangler output
func extractCustomDomainURL(output string) string {
	// Wrangler may output custom domain URLs when deploying to production branch
	// Look for HTTPS URLs that are NOT .pages.dev domains
	// Pattern: https://(www.)?{domain}.{tld} where tld is alphabetic only
	// Explicitly exclude .pages.dev by checking the full match
	re := regexp.MustCompile(`https://(?:www\.)?[a-z0-9-]+\.[a-z]+(?:\.[a-z]+)?(?:/[^\s]*)?`)

	// Find all matches
	matches := re.FindAllString(output, -1)

	// Filter out .pages.dev URLs and partial matches
	for _, match := range matches {
		// Skip if it contains .pages.dev
		if strings.Contains(match, ".pages.dev") {
			continue
		}
		// Skip if it's suspiciously short (likely a partial match)
		if len(match) < 15 {
			continue
		}
		// Return the first valid custom domain
		return match
	}

	return ""
}

// DeployToPages runs `bunx wrangler pages deploy public --project-name={projectName}` and returns streaming output
// If branch is empty, deploys as preview. If branch is "main", deploys to production (custom domain).
func DeployToPages(projectName string, branch string, mockMode bool) CommandOutput {
	if mockMode {
		mockURL := fmt.Sprintf("https://%s.pages.dev", projectName)
		return CommandOutput{
			Output:        fmt.Sprintf("Deploying to Cloudflare Pages (mock mode)...\nProject: %s\nBranch: %s\nDeployment complete! (mock)\nURL: %s", projectName, branch, mockURL),
			Error:         nil,
			DeploymentURL: mockURL,
		}
	}

	if projectName == "" {
		return CommandOutput{
			Output: "",
			Error:  fmt.Errorf("project name is required"),
		}
	}

	// Build wrangler command arguments
	args := []string{"wrangler", "pages", "deploy", "public", "--project-name=" + projectName}
	if branch != "" {
		args = append(args, "--branch="+branch) // Add branch flag for production deployment
	}

	result := runCommand("bunx", args...)

	// Extract both preview and custom domain URLs from output
	if result.Error == nil {
		// Preview URL (*.pages.dev)
		previewURL := extractDeploymentURL(result.Output)
		result.PreviewURL = previewURL

		// Custom domain URL (production)
		customDomainURL := extractCustomDomainURL(result.Output)
		result.DeploymentURL = customDomainURL
	}

	return result
}

// CreatePagesProject runs `bunx wrangler pages project create {projectName} --production-branch=main`
// Returns success if project already exists (idempotent)
func CreatePagesProject(projectName string, mockMode bool) CommandOutput {
	if mockMode {
		return CommandOutput{
			Output: fmt.Sprintf("Creating Cloudflare Pages project (mock mode)...\nProject '%s' created successfully (mock)", projectName),
			Error:  nil,
		}
	}

	if projectName == "" {
		return CommandOutput{
			Output: "",
			Error:  fmt.Errorf("project name is required"),
		}
	}

	result := runCommand("bunx", "wrangler", "pages", "project", "create", projectName, "--production-branch=main")

	// Wrangler returns error if project exists - make it idempotent
	if result.Error != nil && strings.Contains(result.Output, "already exists") {
		return CommandOutput{
			Output: result.Output + "\n✓ Project already exists (idempotent success)",
			Error:  nil,
		}
	}

	return result
}

// BuildAndDeploy runs Hugo build followed by Wrangler deploy
// If branch is empty, deploys as preview. If branch is "main", deploys to production (custom domain).
func BuildAndDeploy(projectName string, branch string, mockMode bool) CommandOutput {
	// Step 1: Build Hugo site
	buildResult := BuildHugoSite(mockMode)
	if buildResult.Error != nil {
		return CommandOutput{
			Output: buildResult.Output,
			Error:  fmt.Errorf("build failed: %w", buildResult.Error),
		}
	}

	// Step 2: Deploy to Pages
	deployResult := DeployToPages(projectName, branch, mockMode)
	if deployResult.Error != nil {
		return CommandOutput{
			Output:   buildResult.Output + "\n\n" + deployResult.Output,
			Error:    fmt.Errorf("deployment failed: %w", deployResult.Error),
			LocalURL: buildResult.LocalURL, // Preserve local URL even if deploy fails
			LANURL:   buildResult.LANURL,
		}
	}

	// Success - combine outputs and URLs
	return CommandOutput{
		Output:        buildResult.Output + "\n\n" + deployResult.Output,
		Error:         nil,
		LocalURL:      buildResult.LocalURL,
		LANURL:        buildResult.LANURL,
		PreviewURL:    deployResult.PreviewURL,    // Cloudflare preview URL
		DeploymentURL: deployResult.DeploymentURL, // Custom domain URL
	}
}

// createLogFile creates a timestamped log file in the logs/ directory
func createLogFile() (*os.File, error) {
	// Create logs directory if it doesn't exist
	logsDir := "logs"
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Generate timestamped filename
	timestamp := time.Now().Format("2006-01-02-150405")
	logPath := filepath.Join(logsDir, fmt.Sprintf("deployment-%s.log", timestamp))

	// Create log file
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	return logFile, nil
}

// runCommand executes a command and captures streaming output
// Output is written to both memory (for web UI) and log file (for debugging)
func runCommand(name string, args ...string) CommandOutput {
	cmd := exec.Command(name, args...)

	// Create log file for this command
	logFile, err := createLogFile()
	if err != nil {
		// If logging fails, continue without it (non-fatal)
		fmt.Fprintf(os.Stderr, "Warning: failed to create log file: %v\n", err)
	}
	if logFile != nil {
		defer logFile.Close()
		// Write header to log file
		fmt.Fprintf(logFile, "=== Command: %s %v ===\n", name, args)
		fmt.Fprintf(logFile, "=== Started: %s ===\n\n", time.Now().Format(time.RFC3339))
	}

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return CommandOutput{
			Output: "",
			Error:  fmt.Errorf("failed to create stdout pipe: %w", err),
		}
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return CommandOutput{
			Output: "",
			Error:  fmt.Errorf("failed to create stderr pipe: %w", err),
		}
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return CommandOutput{
			Output: "",
			Error:  fmt.Errorf("failed to start command: %w", err),
		}
	}

	// Read output from both pipes
	var output strings.Builder
	done := make(chan error)

	// Create multi-writer for dual output (memory + log file)
	var multiWriter io.Writer
	if logFile != nil {
		multiWriter = io.MultiWriter(&output, logFile)
	} else {
		multiWriter = &output
	}

	// Read stdout
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(multiWriter, line)
		}
	}()

	// Read stderr
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(multiWriter, line)
		}
	}()

	// Wait for command to finish
	go func() {
		done <- cmd.Wait()
	}()

	// Wait for completion
	err = <-done

	// Ensure all output is read
	io.Copy(multiWriter, stdoutPipe)
	io.Copy(multiWriter, stderrPipe)

	// Write footer to log file
	if logFile != nil {
		fmt.Fprintf(logFile, "\n=== Finished: %s ===\n", time.Now().Format(time.RFC3339))
		if err != nil {
			fmt.Fprintf(logFile, "=== Exit Status: FAILED ===\n")
		} else {
			fmt.Fprintf(logFile, "=== Exit Status: SUCCESS ===\n")
		}
	}

	if err != nil {
		return CommandOutput{
			Output: output.String(),
			Error:  fmt.Errorf("command failed: %w", err),
		}
	}

	return CommandOutput{
		Output: output.String(),
		Error:  nil,
	}
}
