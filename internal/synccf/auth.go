package synccf

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// CFCredentials holds all Cloudflare authentication credentials
type CFCredentials struct {
	AccountID   string
	APIToken    string // For general CF API access (optional)
	R2AccessKey string // For R2 S3-compatible API
	R2SecretKey string
}

// RunAuth runs the interactive authentication flow for Cloudflare
func RunAuth(w io.Writer) error {
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Cloudflare Authentication Setup")
	_, _ = fmt.Fprintln(w, "================================")
	_, _ = fmt.Fprintln(w, "")

	// Try to detect existing values from environment
	existingAccountID := getExistingEnv("CF_ACCOUNT_ID", "CLOUDFLARE_ACCOUNT_ID")
	existingAPIToken := getExistingEnv("CF_API_TOKEN", "CLOUDFLARE_API_TOKEN")
	existingR2Access := os.Getenv("R2_ACCESS_KEY")
	existingR2Secret := os.Getenv("R2_SECRET_KEY")

	reader := bufio.NewReader(os.Stdin)

	// Step 1: Get Account ID
	_, _ = fmt.Fprintln(w, "Step 1: Account ID")
	_, _ = fmt.Fprintln(w, "  Find your Account ID in the Cloudflare dashboard URL:")
	_, _ = fmt.Fprintln(w, "  https://dash.cloudflare.com/<ACCOUNT_ID>/...")
	_, _ = fmt.Fprintln(w, "")

	accountID := promptWithDefault(w, reader, "CF_ACCOUNT_ID", existingAccountID)
	if accountID == "" {
		return fmt.Errorf("account ID is required")
	}

	// Step 2: API Token (optional but recommended)
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Step 2: API Token (optional)")
	_, _ = fmt.Fprintln(w, "  For general Cloudflare API access (Workers, Pages, DNS, etc.)")
	_, _ = fmt.Fprintln(w, "  Create at: https://dash.cloudflare.com/profile/api-tokens")
	_, _ = fmt.Fprintln(w, "")
	_ = openBrowser("https://dash.cloudflare.com/profile/api-tokens")

	apiToken := promptWithDefault(w, reader, "CF_API_TOKEN", existingAPIToken)

	// Step 3: R2 Credentials
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Step 3: R2 Credentials")
	_, _ = fmt.Fprintln(w, "  For R2 object storage access (S3-compatible)")
	_, _ = fmt.Fprintln(w, "  Create at: R2 > Manage R2 API Tokens")
	_, _ = fmt.Fprintln(w, "")
	_ = openBrowser(fmt.Sprintf("https://dash.cloudflare.com/%s/r2/api-tokens", accountID))

	_, _ = fmt.Fprintln(w, "  Create a token with 'Object Read & Write' permission")
	_, _ = fmt.Fprintln(w, "")

	r2AccessKey := promptWithDefault(w, reader, "R2_ACCESS_KEY", existingR2Access)
	r2SecretKey := promptWithDefault(w, reader, "R2_SECRET_KEY", existingR2Secret)

	creds := CFCredentials{
		AccountID:   accountID,
		APIToken:    apiToken,
		R2AccessKey: r2AccessKey,
		R2SecretKey: r2SecretKey,
	}

	// Validate
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprint(w, "Validating credentials... ")

	if err := validateCredentials(creds); err != nil {
		_, _ = fmt.Fprintln(w, "WARNING")
		_, _ = fmt.Fprintf(w, "  %v\n", err)
		_, _ = fmt.Fprintln(w, "  Continuing anyway...")
	} else {
		_, _ = fmt.Fprintln(w, "OK")
	}

	// Save to .env
	_, _ = fmt.Fprint(w, "Saving to .env... ")
	if err := saveAllCredentialsToEnv(creds); err != nil {
		_, _ = fmt.Fprintln(w, "FAILED")
		return fmt.Errorf("failed to save credentials: %w", err)
	}
	_, _ = fmt.Fprintln(w, "OK")

	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Authentication complete!")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Credentials saved to .env:")
	_, _ = fmt.Fprintf(w, "  CF_ACCOUNT_ID=%s\n", accountID)
	if apiToken != "" {
		_, _ = fmt.Fprintf(w, "  CF_API_TOKEN=%s...%s\n", apiToken[:4], apiToken[len(apiToken)-4:])
	}
	if r2AccessKey != "" {
		_, _ = fmt.Fprintf(w, "  R2_ACCESS_KEY=%s...\n", r2AccessKey[:8])
		_, _ = fmt.Fprintln(w, "  R2_SECRET_KEY=****")
	}
	_, _ = fmt.Fprintln(w, "")

	return nil
}

// getExistingEnv returns the first non-empty value from the given env var names
func getExistingEnv(keys ...string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return ""
}

// promptWithDefault prompts for input, showing default value if available
func promptWithDefault(w io.Writer, reader *bufio.Reader, name, defaultVal string) string {
	if defaultVal != "" {
		// Mask secrets
		displayVal := defaultVal
		if strings.Contains(strings.ToLower(name), "secret") || strings.Contains(strings.ToLower(name), "token") {
			if len(defaultVal) > 8 {
				displayVal = defaultVal[:4] + "..." + defaultVal[len(defaultVal)-4:]
			} else {
				displayVal = "****"
			}
		}
		_, _ = fmt.Fprintf(w, "  %s [%s]: ", name, displayVal)
	} else {
		_, _ = fmt.Fprintf(w, "  %s: ", name)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}
	return input
}

// validateCredentials validates the provided credentials
func validateCredentials(creds CFCredentials) error {
	// Validate account ID format
	if len(creds.AccountID) != 32 {
		return fmt.Errorf("account ID should be 32 characters (got %d)", len(creds.AccountID))
	}

	// If API token provided, verify it works
	if creds.APIToken != "" {
		if err := verifyAPIToken(creds.AccountID, creds.APIToken); err != nil {
			return fmt.Errorf("API token verification failed: %w", err)
		}
	}

	// Basic format checks for R2 credentials
	if creds.R2AccessKey != "" && len(creds.R2AccessKey) < 10 {
		return fmt.Errorf("R2 access key seems too short")
	}
	if creds.R2SecretKey != "" && len(creds.R2SecretKey) < 10 {
		return fmt.Errorf("R2 secret key seems too short")
	}

	return nil
}

// verifyAPIToken verifies the API token by calling the Cloudflare API
func verifyAPIToken(accountID, apiToken string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s", accountID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 401 {
		return fmt.Errorf("invalid token (401 Unauthorized)")
	}
	if resp.StatusCode == 403 {
		return fmt.Errorf("token lacks permissions (403 Forbidden)")
	}
	if resp.StatusCode != 200 {
		var result struct {
			Success bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && len(result.Errors) > 0 {
			return fmt.Errorf("%s", result.Errors[0].Message)
		}
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
}

// saveAllCredentialsToEnv saves all credentials to .env file
func saveAllCredentialsToEnv(creds CFCredentials) error {
	envPath := ".env"

	// Read existing .env content
	existingContent := ""
	if data, err := os.ReadFile(envPath); err == nil {
		existingContent = string(data)
	}

	// Update or append credentials
	lines := strings.Split(existingContent, "\n")
	updated := make(map[string]bool)

	keysToUpdate := map[string]string{
		"CF_ACCOUNT_ID": creds.AccountID,
		"CF_API_TOKEN":  creds.APIToken,
		"R2_ACCESS_KEY": creds.R2AccessKey,
		"R2_SECRET_KEY": creds.R2SecretKey,
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for key, value := range keysToUpdate {
			if strings.HasPrefix(trimmed, key+"=") && value != "" {
				lines[i] = fmt.Sprintf("%s=%s", key, value)
				updated[key] = true
			}
		}
	}

	// Append missing keys (only if value is non-empty)
	var toAppend []string
	for key, value := range keysToUpdate {
		if !updated[key] && value != "" {
			toAppend = append(toAppend, fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Build final content
	content := strings.Join(lines, "\n")
	if len(toAppend) > 0 {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += strings.Join(toAppend, "\n") + "\n"
	}

	return os.WriteFile(envPath, []byte(content), 0600)
}

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported platform")
	}

	return exec.Command(cmd, args...).Start()
}
