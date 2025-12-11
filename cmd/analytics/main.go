// analytics fetches Cloudflare Web Analytics and reports changes.
//
// Compares current metrics to the previous run (stored in .analytics-state.json)
// and reports significant changes (>20% threshold). Can run locally or in GitHub
// Actions to create issues when traffic changes significantly.
//
// Usage:
//
//	go run cmd/analytics/main.go                   # Print report to terminal
//	go run cmd/analytics/main.go -webhook URL     # Post to webhook if changed
//	go run cmd/analytics/main.go -days 14         # Compare last 14 days
//	go run cmd/analytics/main.go -github-issue   # Output markdown for GitHub Issue
//	task seo:report                               # Via Taskfile
//
// GitHub Actions:
//
//	Runs weekly via .github/workflows/analytics-report.yml
//	Creates a GitHub Issue when visits or pageviews change >20%
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	cfGraphQLEndpoint = "https://api.cloudflare.com/client/v4/graphql"
	stateFile         = ".analytics-state.json"
	changeThreshold   = 0.20 // 20% change triggers alert

	// Default values (fallbacks if env vars not set)
	defaultAccountTag = "7384af54e33b8a54ff240371ea368440"
	defaultSiteTag    = "4c28a6bfb5514996914a603c999d5c79"
)

// version is set via ldflags at build time
var version = "dev"

// getConfig returns Cloudflare account and site tags from environment variables,
// falling back to defaults for backward compatibility.
func getConfig() (accountTag, siteTag string) {
	accountTag = os.Getenv("CF_ACCOUNT_ID")
	if accountTag == "" {
		accountTag = defaultAccountTag
	}
	siteTag = os.Getenv("CF_WEB_ANALYTICS_SITE_TAG")
	if siteTag == "" {
		siteTag = defaultSiteTag
	}
	return
}

// State represents the stored analytics state from previous run
type State struct {
	Timestamp  time.Time         `json:"timestamp"`
	Period     string            `json:"period"`
	Visits     int64             `json:"visits"`
	PageViews  int64             `json:"pageviews"`
	TopPages   map[string]int64  `json:"top_pages"`
	Countries  map[string]int64  `json:"countries"`
}

// GraphQL query for Cloudflare Web Analytics
const analyticsQuery = `
query WebAnalytics($accountTag: string!, $filter: AccountRumPageloadEventsAdaptiveGroupsFilter_InputObject!) {
  viewer {
    accounts(filter: {accountTag: $accountTag}) {
      rumPageloadEventsAdaptiveGroups(
        filter: $filter
        limit: 5000
      ) {
        sum {
          visits
        }
        count
        dimensions {
          requestPath
          countryName
        }
      }
    }
  }
}
`

// GraphQL response structures
type GraphQLResponse struct {
	Data   ResponseData   `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

type GraphQLError struct {
	Message string `json:"message"`
}

type ResponseData struct {
	Viewer Viewer `json:"viewer"`
}

type Viewer struct {
	Accounts []Account `json:"accounts"`
}

type Account struct {
	RumGroups []RumGroup `json:"rumPageloadEventsAdaptiveGroups"`
}

type RumGroup struct {
	Sum        SumData    `json:"sum"`
	Dimensions Dimensions `json:"dimensions"`
	Count      int64      `json:"count"`
}

type SumData struct {
	Visits int64 `json:"visits"`
}

type Dimensions struct {
	RequestPath string `json:"requestPath"`
	CountryName string `json:"countryName"`
}

func main() {
	webhookURL := flag.String("webhook", "", "Webhook URL to post changes (Slack/Discord)")
	days := flag.Int("days", 7, "Number of days to analyze")
	verbose := flag.Bool("v", false, "Verbose output")
	githubIssue := flag.Bool("github-issue", false, "Output markdown for GitHub Issue (exits 1 if changes detected)")
	ver := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *ver {
		fmt.Printf("analytics %s\n", version)
		os.Exit(0)
	}

	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "Error: CLOUDFLARE_API_TOKEN environment variable not set")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Create a token at: https://dash.cloudflare.com/profile/api-tokens")
		fmt.Fprintln(os.Stderr, "Required permissions: Account Analytics:Read")
		os.Exit(1)
	}

	// Calculate date range
	until := time.Now().UTC().Truncate(24 * time.Hour)
	since := until.AddDate(0, 0, -*days)

	if *verbose {
		fmt.Printf("Fetching analytics for %s to %s...\n", since.Format("2006-01-02"), until.Format("2006-01-02"))
	}

	// Fetch current analytics
	current, err := fetchAnalytics(token, since, until)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching analytics: %v\n", err)
		os.Exit(1)
	}
	current.Period = fmt.Sprintf("%s to %s", since.Format("Jan 2"), until.Format("Jan 2"))

	// Load previous state
	previous, err := loadState()
	if err != nil && *verbose {
		fmt.Printf("No previous state found (first run)\n")
	}

	// Generate report
	report := generateReport(current, previous)

	// GitHub Issue mode: output markdown and exit with code based on changes
	if *githubIssue {
		fmt.Println(generateMarkdownReport(current, previous, report))
		// Save state for next run
		if err := saveState(current); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save state: %v\n", err)
		}
		if report.HasChanges {
			os.Exit(1) // Signal to workflow that issue should be created
		}
		return
	}

	// Print report
	fmt.Println(report.Summary)
	if len(report.Changes) > 0 {
		fmt.Println("\nSignificant Changes:")
		for _, change := range report.Changes {
			fmt.Printf("  %s\n", change)
		}
	}

	// Save current state for next comparison
	if err := saveState(current); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save state: %v\n", err)
	}

	// Post to webhook if configured and there are changes
	if *webhookURL != "" && len(report.Changes) > 0 {
		if err := postToWebhook(*webhookURL, report); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to post to webhook: %v\n", err)
		} else if *verbose {
			fmt.Println("Posted to webhook")
		}
	}
}

func fetchAnalytics(token string, since, until time.Time) (*State, error) {
	// Get config from environment (with fallbacks)
	accountTag, siteTag := getConfig()

	// Build GraphQL request with proper filter structure
	filter := map[string]any{
		"AND": []map[string]any{
			{
				"datetime_geq": since.Format(time.RFC3339),
				"datetime_leq": until.Format(time.RFC3339),
			},
			{"bot": 0}, // Exclude bots
			{
				"OR": []map[string]any{
					{"siteTag": siteTag},
				},
			},
		},
	}

	reqBody := map[string]any{
		"query": analyticsQuery,
		"variables": map[string]any{
			"accountTag": accountTag,
			"filter":     filter,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", cfGraphQLEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var gqlResp GraphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		errMsg := gqlResp.Errors[0].Message
		if strings.Contains(errMsg, "not authorized") {
			return nil, fmt.Errorf("not authorized - ensure your API token has 'Account Analytics:Read' permission\nCreate/edit token at: https://dash.cloudflare.com/profile/api-tokens")
		}
		return nil, fmt.Errorf("GraphQL error: %s", errMsg)
	}

	// Aggregate results
	state := &State{
		Timestamp: time.Now().UTC(),
		TopPages:  make(map[string]int64),
		Countries: make(map[string]int64),
	}

	if len(gqlResp.Data.Viewer.Accounts) == 0 {
		return state, nil // No data
	}

	for _, group := range gqlResp.Data.Viewer.Accounts[0].RumGroups {
		state.Visits += group.Sum.Visits
		state.PageViews += group.Count // Count is pageviews in this API

		if group.Dimensions.RequestPath != "" {
			state.TopPages[group.Dimensions.RequestPath] += group.Count
		}
		if group.Dimensions.CountryName != "" {
			state.Countries[group.Dimensions.CountryName] += group.Count
		}
	}

	return state, nil
}

// Report contains the generated analytics report
type Report struct {
	Summary string
	Changes []string
	HasChanges bool
}

func generateReport(current, previous *State) Report {
	report := Report{}

	// Build summary
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Analytics Report (%s)\n", current.Period))
	sb.WriteString(strings.Repeat("=", 40) + "\n")
	sb.WriteString(fmt.Sprintf("Visits:     %d\n", current.Visits))
	sb.WriteString(fmt.Sprintf("Page Views: %d\n", current.PageViews))

	// Top pages
	if len(current.TopPages) > 0 {
		sb.WriteString("\nTop Pages:\n")
		pages := sortMapByValue(current.TopPages, 5)
		for _, p := range pages {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", p.Key, p.Value))
		}
	}

	// Top countries
	if len(current.Countries) > 0 {
		sb.WriteString("\nTop Countries:\n")
		countries := sortMapByValue(current.Countries, 5)
		for _, c := range countries {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", c.Key, c.Value))
		}
	}

	report.Summary = sb.String()

	// Compare with previous if available
	if previous != nil {
		// Check visits change
		if change := percentChange(previous.Visits, current.Visits); math.Abs(change) >= changeThreshold*100 {
			direction := "increased"
			if change < 0 {
				direction = "decreased"
			}
			report.Changes = append(report.Changes,
				fmt.Sprintf("Visits %s %.0f%% (%d -> %d)", direction, math.Abs(change), previous.Visits, current.Visits))
			report.HasChanges = true
		}

		// Check pageviews change
		if change := percentChange(previous.PageViews, current.PageViews); math.Abs(change) >= changeThreshold*100 {
			direction := "increased"
			if change < 0 {
				direction = "decreased"
			}
			report.Changes = append(report.Changes,
				fmt.Sprintf("Page views %s %.0f%% (%d -> %d)", direction, math.Abs(change), previous.PageViews, current.PageViews))
			report.HasChanges = true
		}
	}

	return report
}

func percentChange(old, new int64) float64 {
	if old == 0 {
		if new > 0 {
			return 100
		}
		return 0
	}
	return float64(new-old) / float64(old) * 100
}

type kv struct {
	Key   string
	Value int64
}

func sortMapByValue(m map[string]int64, limit int) []kv {
	var sorted []kv
	for k, v := range m {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted
}

func loadState() (*State, error) {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func saveState(state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0644)
}

func generateMarkdownReport(current, previous *State, report Report) string {
	var sb strings.Builder

	sb.WriteString("## Analytics Change Detected\n\n")
	sb.WriteString(fmt.Sprintf("**Period:** %s\n\n", current.Period))

	// Changes section
	if len(report.Changes) > 0 {
		sb.WriteString("### Changes\n")
		for _, change := range report.Changes {
			sb.WriteString(fmt.Sprintf("- **%s**\n", change))
		}
		sb.WriteString("\n")
	}

	// Comparison table
	sb.WriteString("### Current Stats\n\n")
	sb.WriteString("| Metric | Previous | Current | Change |\n")
	sb.WriteString("|--------|----------|---------|--------|\n")

	if previous != nil {
		visitChange := percentChange(previous.Visits, current.Visits)
		pvChange := percentChange(previous.PageViews, current.PageViews)
		sb.WriteString(fmt.Sprintf("| Visits | %d | %d | %+.0f%% |\n", previous.Visits, current.Visits, visitChange))
		sb.WriteString(fmt.Sprintf("| Page Views | %d | %d | %+.0f%% |\n", previous.PageViews, current.PageViews, pvChange))
	} else {
		sb.WriteString(fmt.Sprintf("| Visits | - | %d | (first run) |\n", current.Visits))
		sb.WriteString(fmt.Sprintf("| Page Views | - | %d | (first run) |\n", current.PageViews))
	}

	// Top pages
	if len(current.TopPages) > 0 {
		sb.WriteString("\n### Top Pages\n")
		pages := sortMapByValue(current.TopPages, 5)
		for i, p := range pages {
			sb.WriteString(fmt.Sprintf("%d. `%s` - %d views\n", i+1, p.Key, p.Value))
		}
	}

	// Top countries
	if len(current.Countries) > 0 {
		sb.WriteString("\n### Top Countries\n")
		countries := sortMapByValue(current.Countries, 5)
		for i, c := range countries {
			sb.WriteString(fmt.Sprintf("%d. %s - %d\n", i+1, c.Key, c.Value))
		}
	}

	sb.WriteString("\n---\n*Generated by analytics change detection workflow*\n")

	return sb.String()
}

func postToWebhook(url string, report Report) error {
	// Build webhook payload (works for Slack/Discord)
	payload := map[string]any{
		"text": fmt.Sprintf("*Analytics Alert*\n%s\n\n*Changes:*\n%s",
			report.Summary,
			strings.Join(report.Changes, "\n")),
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
