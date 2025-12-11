package env

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// CloudflareVerifyResponse represents the token verification API response
type CloudflareVerifyResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Messages []interface{} `json:"messages"`
	Result   struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"result"`
}

// CloudflareTokenResponse represents a token details API response
type CloudflareTokenResponse struct {
	Success bool `json:"success"`
	Result  struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"result"`
}

// CloudflareAccountResponse represents the account info API response
type CloudflareAccountResponse struct {
	Success bool `json:"success"`
	Result  struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"result"`
}

// CloudflareAccountsResponse represents the accounts list API response
type CloudflareAccountsResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"result"`
}

// ValidateCloudflareToken validates a Cloudflare API token and returns the token name
func ValidateCloudflareToken(token string) (string, error) {
	if token == "" || token == PlaceholderToken {
		return "", fmt.Errorf("no token to validate")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Verify token
	req, err := http.NewRequest("GET", CloudflareAPITokenVerifyURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to verify token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var verifyResp CloudflareVerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !verifyResp.Success {
		if len(verifyResp.Errors) > 0 {
			return "", fmt.Errorf("invalid token: %s", verifyResp.Errors[0].Message)
		}
		return "", fmt.Errorf("token verification failed")
	}

	// Get token details to retrieve the name
	// This requires "User: API Tokens: Read" permission
	// If token lacks this permission, validation will still succeed but won't show the name
	tokenID := verifyResp.Result.ID
	tokenReq, err := http.NewRequest("GET", fmt.Sprintf(CloudflareAPITokenInfoURL, tokenID), nil)
	if err != nil {
		// Can't create request - return without name
		return "", nil
	}

	tokenReq.Header.Set("Authorization", "Bearer "+token)
	tokenReq.Header.Set("Content-Type", "application/json")

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		// Can't fetch details - return without name
		return "", nil
	}
	defer tokenResp.Body.Close()

	tokenBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		// Can't read response - return without name
		return "", nil
	}

	var tokenDetails CloudflareTokenResponse
	if err := json.Unmarshal(tokenBody, &tokenDetails); err != nil {
		// Can't parse response - return without name
		return "", nil
	}

	if !tokenDetails.Success {
		// Token doesn't have permission to read its own details - return without name
		return "", nil
	}

	return tokenDetails.Result.Name, nil
}

// ValidateCloudflareAccount validates the account ID with the given token
func ValidateCloudflareAccount(token, accountID string) (string, error) {
	if accountID == "" {
		return "", fmt.Errorf("no account ID to validate")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf(CloudflareAPIAccountURL, accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to verify account: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var accountResp CloudflareAccountResponse
	if err := json.Unmarshal(body, &accountResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !accountResp.Success {
		// Check for specific error in response
		if resp.StatusCode == 403 {
			return "", fmt.Errorf("token lacks permission to access account %s (need Account:Read permission)", accountID)
		}
		if resp.StatusCode == 404 {
			return "", fmt.Errorf("account ID %s not found or not accessible with this token", accountID)
		}
		return "", fmt.Errorf("account validation failed (status: %d)", resp.StatusCode)
	}

	return accountResp.Result.Name, nil
}

// ValidateCloudflareProjectName validates a Cloudflare Pages project name
// Project names must be lowercase alphanumeric with hyphens, 1-63 characters
func ValidateCloudflareProjectName(projectName string) error {
	if projectName == "" {
		return fmt.Errorf("project name cannot be empty")
	}

	// Cloudflare project name requirements
	if len(projectName) > 63 {
		return fmt.Errorf("project name must be 63 characters or less")
	}

	// Must match: lowercase letters, numbers, hyphens only
	// Cannot start or end with hyphen
	matched, err := regexp.MatchString(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, projectName)
	if err != nil {
		return fmt.Errorf("failed to validate project name format: %w", err)
	}

	if !matched {
		return fmt.Errorf("project name must contain only lowercase letters, numbers, and hyphens")
	}

	return nil
}

// GetCloudflareAccounts fetches all accounts accessible by the token
// Returns the first account ID and name if exactly one account is found
func GetCloudflareAccounts(token string) (accountID, accountName string, err error) {
	if token == "" || token == PlaceholderToken {
		return "", "", fmt.Errorf("no token provided")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", CloudflareAPIAccountsURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch accounts: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response: %w", err)
	}

	var accountsResp CloudflareAccountsResponse
	if err := json.Unmarshal(body, &accountsResp); err != nil {
		return "", "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !accountsResp.Success {
		return "", "", fmt.Errorf("failed to fetch accounts (status: %d)", resp.StatusCode)
	}

	if len(accountsResp.Result) == 0 {
		return "", "", fmt.Errorf("no accounts found for this token")
	}

	// Return the first account (most tokens only have access to one account)
	return accountsResp.Result[0].ID, accountsResp.Result[0].Name, nil
}

// Zone represents a Cloudflare DNS zone (domain)
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CloudflareZonesResponse represents the zones list API response
type CloudflareZonesResponse struct {
	Success bool   `json:"success"`
	Result  []Zone `json:"result"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// ListZones fetches all DNS zones (domains) for the account
func ListZones(token, accountID string) ([]Zone, error) {
	if token == "" {
		return nil, fmt.Errorf("no token provided")
	}
	if accountID == "" {
		return nil, fmt.Errorf("no account ID provided")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// List all zones accessible by the token (zones are automatically filtered by token permissions)
	url := fmt.Sprintf("%s?per_page=50", CloudflareAPIZonesURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch zones: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var zonesResp CloudflareZonesResponse
	if err := json.Unmarshal(body, &zonesResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !zonesResp.Success {
		if len(zonesResp.Errors) > 0 {
			return nil, fmt.Errorf("failed to fetch zones: %s", zonesResp.Errors[0].Message)
		}
		return nil, fmt.Errorf("failed to fetch zones (status: %d)", resp.StatusCode)
	}

	return zonesResp.Result, nil
}

// PagesProject represents a Cloudflare Pages project
type PagesProject struct {
	Name      string `json:"name"`
	CreatedOn string `json:"created_on"`
}

// CloudflarePagesResponse represents the Pages projects list API response
type CloudflarePagesResponse struct {
	Success bool           `json:"success"`
	Result  []PagesProject `json:"result"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// PagesDomain represents a custom domain attached to a Pages project
type PagesDomain struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// CloudflarePagesDomainsResponse represents the Pages domains list API response
type CloudflarePagesDomainsResponse struct {
	Success bool          `json:"success"`
	Result  []PagesDomain `json:"result"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// ListPagesProjects fetches all Cloudflare Pages projects for the account
func ListPagesProjects(token, accountID string) ([]PagesProject, error) {
	if token == "" {
		return nil, fmt.Errorf("no token provided")
	}
	if accountID == "" {
		return nil, fmt.Errorf("no account ID provided")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf(CloudflareAPIPagesURL, accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Pages projects: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var pagesResp CloudflarePagesResponse
	if err := json.Unmarshal(body, &pagesResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !pagesResp.Success {
		if len(pagesResp.Errors) > 0 {
			return nil, fmt.Errorf("failed to fetch Pages projects: %s", pagesResp.Errors[0].Message)
		}
		return nil, fmt.Errorf("failed to fetch Pages projects (status: %d)", resp.StatusCode)
	}

	return pagesResp.Result, nil
}

// DeletePagesProject deletes a Cloudflare Pages project
func DeletePagesProject(token, accountID, projectName string) error {
	if token == "" {
		return fmt.Errorf("no token provided")
	}
	if accountID == "" {
		return fmt.Errorf("no account ID provided")
	}
	if projectName == "" {
		return fmt.Errorf("no project name provided")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf(CloudflareAPIPagesDeleteURL, accountID, projectName)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete Pages project: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// DELETE returns 200 OK with a success response, not 204 No Content
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete project %s (status: %d): %s", projectName, resp.StatusCode, string(body))
	}

	// Parse response to check success field
	var deleteResp CloudflareVerifyResponse
	if err := json.Unmarshal(body, &deleteResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !deleteResp.Success {
		if len(deleteResp.Errors) > 0 {
			return fmt.Errorf("failed to delete project %s: %s", projectName, deleteResp.Errors[0].Message)
		}
		return fmt.Errorf("failed to delete project %s", projectName)
	}

	return nil
}

// ListPagesDomains fetches all custom domains for a specific Pages project
func ListPagesDomains(token, accountID, projectName string) ([]PagesDomain, error) {
	if token == "" {
		return nil, fmt.Errorf("no token provided")
	}
	if accountID == "" {
		return nil, fmt.Errorf("no account ID provided")
	}
	if projectName == "" {
		return nil, fmt.Errorf("no project name provided")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf(CloudflareAPIPagesDomainsURL, accountID, projectName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch domains: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var domainsResp CloudflarePagesDomainsResponse
	if err := json.Unmarshal(body, &domainsResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !domainsResp.Success {
		if len(domainsResp.Errors) > 0 {
			return nil, fmt.Errorf("failed to fetch domains: %s", domainsResp.Errors[0].Message)
		}
		return nil, fmt.Errorf("failed to fetch domains (status: %d)", resp.StatusCode)
	}

	return domainsResp.Result, nil
}

// AddPagesDomain adds a custom domain to a Pages project
func AddPagesDomain(token, accountID, projectName, domainName string) error {
	if token == "" {
		return fmt.Errorf("no token provided")
	}
	if accountID == "" {
		return fmt.Errorf("no account ID provided")
	}
	if projectName == "" {
		return fmt.Errorf("no project name provided")
	}
	if domainName == "" {
		return fmt.Errorf("no domain name provided")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf(CloudflareAPIPagesDomainsURL, accountID, projectName)

	// Prepare JSON request body
	requestBody := map[string]string{
		"name": domainName,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// POST request to add domain with JSON body
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add domain: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to add domain %s (status: %d): %s", domainName, resp.StatusCode, string(body))
	}

	// Parse response to check success field
	var addResp CloudflareVerifyResponse
	if err := json.Unmarshal(body, &addResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !addResp.Success {
		if len(addResp.Errors) > 0 {
			return fmt.Errorf("failed to add domain: %s", addResp.Errors[0].Message)
		}
		return fmt.Errorf("failed to add domain %s", domainName)
	}

	return nil
}

// DeletePagesDomain removes a custom domain from a Pages project
func DeletePagesDomain(token, accountID, projectName, domainName string) error {
	if token == "" {
		return fmt.Errorf("no token provided")
	}
	if accountID == "" {
		return fmt.Errorf("no account ID provided")
	}
	if projectName == "" {
		return fmt.Errorf("no project name provided")
	}
	if domainName == "" {
		return fmt.Errorf("no domain name provided")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	url := fmt.Sprintf(CloudflareAPIPagesDeleteDomainURL, accountID, projectName, domainName)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete domain: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// DELETE returns 200 OK with a success response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete domain %s (status: %d): %s", domainName, resp.StatusCode, string(body))
	}

	// Parse response to check success field
	var deleteResp CloudflareVerifyResponse
	if err := json.Unmarshal(body, &deleteResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !deleteResp.Success {
		if len(deleteResp.Errors) > 0 {
			return fmt.Errorf("failed to delete domain %s: %s", domainName, deleteResp.Errors[0].Message)
		}
		return fmt.Errorf("failed to delete domain %s", domainName)
	}

	return nil
}

// DeletePagesProjectWithCleanup deletes a Pages project after removing all custom domains
// Returns the list of domains that were removed, or an error
func DeletePagesProjectWithCleanup(token, accountID, projectName string) ([]string, error) {
	if token == "" {
		return nil, fmt.Errorf("no token provided")
	}
	if accountID == "" {
		return nil, fmt.Errorf("no account ID provided")
	}
	if projectName == "" {
		return nil, fmt.Errorf("no project name provided")
	}

	// Step 1: List all custom domains
	domains, err := ListPagesDomains(token, accountID, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	// Extract domain names
	domainNames := make([]string, len(domains))
	for i, domain := range domains {
		domainNames[i] = domain.Name
	}

	// Step 2: Remove all custom domains (if any)
	for _, domain := range domains {
		if err := DeletePagesDomain(token, accountID, projectName, domain.Name); err != nil {
			return domainNames, fmt.Errorf("failed to delete domain %s: %w", domain.Name, err)
		}
	}

	// Step 3: Delete the project
	if err := DeletePagesProject(token, accountID, projectName); err != nil {
		return domainNames, fmt.Errorf("domains removed but project deletion failed: %w", err)
	}

	return domainNames, nil
}
