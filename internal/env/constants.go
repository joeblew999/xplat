package env

// API Endpoints
const (
	// Anthropic API endpoints
	AnthropicAPIMessagesURL = "https://api.anthropic.com/v1/messages"

	// Cloudflare API endpoints
	CloudflareAPITokenVerifyURL = "https://api.cloudflare.com/client/v4/user/tokens/verify"
	CloudflareAPITokenInfoURL   = "https://api.cloudflare.com/client/v4/user/tokens/%s"         // requires tokenID
	CloudflareAPIAccountURL     = "https://api.cloudflare.com/client/v4/accounts/%s"            // requires accountID
	CloudflareAPIAccountsURL    = "https://api.cloudflare.com/client/v4/accounts"
	CloudflareAPIZonesURL       = "https://api.cloudflare.com/client/v4/zones"                  // GET zones (domains)
	CloudflareAPIPagesURL           = "https://api.cloudflare.com/client/v4/accounts/%s/pages/projects"              // requires accountID
	CloudflareAPIPagesDeleteURL     = "https://api.cloudflare.com/client/v4/accounts/%s/pages/projects/%s"          // requires accountID, projectName
	CloudflareAPIPagesDomainsURL    = "https://api.cloudflare.com/client/v4/accounts/%s/pages/projects/%s/domains"  // requires accountID, projectName
	CloudflareAPIPagesDeleteDomainURL = "https://api.cloudflare.com/client/v4/accounts/%s/pages/projects/%s/domains/%s" // requires accountID, projectName, domainName
)

// Console URLs
const (
	// Anthropic Console URLs
	AnthropicConsoleURL       = "https://console.anthropic.com/"
	AnthropicAPIKeysURL       = "https://console.anthropic.com/settings/keys"
	AnthropicBillingURL       = "https://console.anthropic.com/settings/billing"
	AnthropicWorkspacesURL    = "https://console.anthropic.com/settings/workspaces"
	AnthropicWorkspaceNameURL = "https://console.anthropic.com/settings/workspaces"

	// Cloudflare Console URLs
	CloudflareDashboardURL = "https://dash.cloudflare.com"
	CloudflareLoginURL     = "https://dash.cloudflare.com/login"
	CloudflareAPITokensURL = "https://dash.cloudflare.com/profile/api-tokens"
	CloudflareAddSiteURL   = "https://dash.cloudflare.com/:account/add-site"
	CloudflarePagesURL     = "https://dash.cloudflare.com/:account/workers-and-pages"

	// GitHub URLs
	GitHubCLIInstallURL      = "https://cli.github.com/"
	GitHubRepoURLTemplate    = "https://github.com/%s/%s"           // requires owner, name
	GitHubSecretsURLTemplate = "%s/settings/secrets/actions"        // requires repo URL
)

// Default Values
const (
	DefaultProjectName = "my-cloudflare-project"
)

// Sync Status Constants
const (
	SyncStatusSynced    = "synced"
	SyncStatusWouldSync = "would-sync"
	SyncStatusSkipped   = "skipped"
	SyncStatusFailed    = "failed"

	SyncReasonCreated        = "created"
	SyncReasonWouldCreateNew = "would create new"
	SyncReasonPlaceholder    = "placeholder value"
)
