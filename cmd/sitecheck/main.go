// sitecheck checks site reachability from multiple global locations.
//
// Uses the check-host.net API to verify a URL is accessible from
// different geographic regions (US, EU, Asia, etc.).
//
// Usage:
//
//	go run cmd/sitecheck/main.go                           # HTTP check (default)
//	go run cmd/sitecheck/main.go -type dns                 # DNS resolution check
//	go run cmd/sitecheck/main.go -type tcp                 # TCP port 443 check
//	go run cmd/sitecheck/main.go -type redirect            # Apex->www redirect check
//	go run cmd/sitecheck/main.go -type all                 # Run all checks
//	go run cmd/sitecheck/main.go -url https://example.com  # Check custom URL
//	go run cmd/sitecheck/main.go -github-issue             # Output markdown for GitHub Issue
//	task site:check                                        # Via Taskfile
//
// GitHub Actions:
//
//	Runs every 6 hours via .github/workflows/site-monitor.yml
//	Creates a GitHub Issue when failures detected or significant changes occur
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	// Fallback values if SITE_URL env var not set
	fallbackSiteURL = "https://www.ubuntusoftware.net"

	defaultNodes = 56                        // Use all available nodes
	defaultWait  = 8                         // Seconds to wait for global responses
	apiBase      = "https://check-host.net"
	stateFile    = ".sitecheck-state.json"
)

// version is set via ldflags at build time
var version = "dev"

// Package-level config derived from SITE_URL environment variable
var (
	defaultURL  string // Full URL to check (e.g., https://www.example.com/robots.txt)
	defaultHost string // Host portion (e.g., www.example.com)
	apexURL     string // Apex domain URL for redirect check (e.g., http://example.com)
)

func init() {
	siteURL := os.Getenv("SITE_URL")
	if siteURL == "" {
		siteURL = fallbackSiteURL
	}
	// Ensure no trailing slash
	siteURL = strings.TrimSuffix(siteURL, "/")

	// Derive check URL (robots.txt for HTTP check)
	defaultURL = siteURL + "/robots.txt"

	// Extract host from URL
	if parsed, err := url.Parse(siteURL); err == nil {
		defaultHost = parsed.Host
		// Derive apex URL (remove www. prefix, use http for redirect check)
		apexHost := strings.TrimPrefix(defaultHost, "www.")
		apexURL = "http://" + apexHost
	} else {
		defaultHost = "www.ubuntusoftware.net"
		apexURL = "http://ubuntusoftware.net"
	}
}

// Check type to API endpoint mapping
var checkEndpoints = map[string]string{
	"http":     "check-http",
	"dns":      "check-dns",
	"tcp":      "check-tcp",
	"redirect": "check-http", // Uses HTTP check but expects 301/302
}

// CheckResponse is the initial response from check-host.net
type CheckResponse struct {
	OK            int              `json:"ok"`
	RequestID     string           `json:"request_id"`
	Nodes         map[string][]any `json:"nodes"`
	PermanentLink string           `json:"permanent_link"`
}

// Result represents a single check result from a node
type Result struct {
	Node    string
	Success bool
	Time    float64 // seconds
	Status  string  // HTTP status, DNS records, or error
	IP      string
	Pending bool
}

// State represents stored check state for comparison
type State struct {
	Timestamp    time.Time `json:"timestamp"`
	CheckType    string    `json:"check_type"`
	TotalNodes   int       `json:"total_nodes"`
	OKCount      int       `json:"ok_count"`
	FailedCount  int       `json:"failed_count"`
	FailedNodes  []string  `json:"failed_nodes"`
	AvgResponseMS float64  `json:"avg_response_ms"`
	MaxResponseMS float64  `json:"max_response_ms"`
}

func main() {
	urlFlag := flag.String("url", defaultURL, "URL to check (for HTTP) or domain (for DNS/TCP)")
	typeFlag := flag.String("type", "http", "Check type: http, dns, tcp, redirect, or all")
	nodesFlag := flag.Int("nodes", defaultNodes, "Maximum number of global nodes to check from")
	waitFlag := flag.Int("wait", defaultWait, "Seconds to wait for results")
	githubIssue := flag.Bool("github-issue", false, "Output markdown for GitHub Issue (exits 1 if issues detected)")
	ver := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *ver {
		fmt.Printf("sitecheck %s\n", version)
		os.Exit(0)
	}

	checkType := strings.ToLower(*typeFlag)

	// GitHub Issue mode: run HTTP check and output markdown
	if *githubIssue {
		runGitHubIssueMode(*urlFlag, *nodesFlag, *waitFlag)
		return
	}

	if checkType == "all" {
		// Run all checks sequentially
		allPassed := true
		for _, ct := range []string{"dns", "tcp", "redirect", "http"} {
			fmt.Printf("=== %s Check ===\n", strings.ToUpper(ct))
			passed := runCheck(ct, *urlFlag, *nodesFlag, *waitFlag)
			if !passed {
				allPassed = false
			}
			fmt.Println()
		}
		if !allPassed {
			os.Exit(1)
		}
		fmt.Println("Summary: All checks passed")
		return
	}

	if _, ok := checkEndpoints[checkType]; !ok {
		fmt.Fprintf(os.Stderr, "Unknown check type: %s (use http, dns, tcp, redirect, or all)\n", checkType)
		os.Exit(1)
	}

	if !runCheck(checkType, *urlFlag, *nodesFlag, *waitFlag) {
		os.Exit(1)
	}
}

// runCheck executes a single check type and returns true if passed
func runCheck(checkType, targetURL string, maxNodes, waitSecs int) bool {
	// Prepare the host parameter based on check type
	host := prepareHost(checkType, targetURL)

	fmt.Printf("Checking %s from %d global locations...\n\n", host, maxNodes)

	// Initiate the check
	requestID, nodes, err := initiateCheck(checkType, host, maxNodes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initiate check: %v\n", err)
		return false
	}

	fmt.Printf("Waiting %d seconds for %d nodes...\n", waitSecs, len(nodes))

	time.Sleep(time.Duration(waitSecs) * time.Second)

	// Get results
	results, err := getResults(requestID, checkType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get results: %v\n", err)
		return false
	}

	// Sort by node name for consistent output
	sort.Slice(results, func(i, j int) bool {
		return results[i].Node < results[j].Node
	})

	// Count and collect failures, track response times
	var failures []Result
	var times []float64
	pending := 0
	for _, r := range results {
		if r.Pending {
			pending++
		} else if !r.Success {
			failures = append(failures, r)
		} else if r.Time > 0 {
			times = append(times, r.Time)
		}
	}

	ok := len(results) - len(failures) - pending

	// Only print failures (if any)
	if len(failures) > 0 {
		fmt.Println("Failures:")
		for _, r := range failures {
			fmt.Printf("  ✗ %s: %s\n", r.Node, r.Status)
		}
		fmt.Println()
	}

	// Summary line
	fmt.Printf("✓ %d/%d nodes OK", ok, len(results))
	if len(failures) > 0 {
		fmt.Printf(", %d failed", len(failures))
	}
	if pending > 0 {
		fmt.Printf(", %d pending", pending)
	}
	// Show response times if available
	if len(times) > 0 {
		var sum, max float64
		for _, t := range times {
			sum += t
			if t > max {
				max = t
			}
		}
		avg := sum / float64(len(times))
		fmt.Printf(" (avg %.0fms, max %.0fms)", avg*1000, max*1000)
	}
	fmt.Printf(" - %s/check-report/%s\n", apiBase, requestID)

	// Exit 1 only if 3+ failures (1-2 is noise)
	return len(failures) < 3
}

// prepareHost converts the URL to the appropriate format for each check type
func prepareHost(checkType, targetURL string) string {
	switch checkType {
	case "dns":
		// DNS check needs just the domain
		return extractDomain(targetURL)
	case "tcp":
		// TCP check needs domain:port
		return extractDomain(targetURL) + ":443"
	case "redirect":
		// Redirect check always uses apex domain
		return apexURL
	default:
		// HTTP check needs full URL
		return targetURL
	}
}

// extractDomain pulls the domain from a URL
func extractDomain(targetURL string) string {
	// If it's already just a domain, return it
	if !strings.Contains(targetURL, "://") {
		return strings.Split(targetURL, ":")[0] // Remove port if present
	}
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return defaultHost
	}
	return parsed.Host
}

func initiateCheck(checkType, host string, maxNodes int) (string, map[string][]any, error) {
	endpoint := checkEndpoints[checkType]
	apiURL := fmt.Sprintf("%s/%s?host=%s&max_nodes=%d",
		apiBase, endpoint, url.QueryEscape(host), maxNodes)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	var checkResp CheckResponse
	if err := json.Unmarshal(body, &checkResp); err != nil {
		return "", nil, fmt.Errorf("failed to parse response: %w\n%s", err, string(body))
	}

	if checkResp.RequestID == "" {
		return "", nil, fmt.Errorf("no request ID in response: %s", string(body))
	}

	return checkResp.RequestID, checkResp.Nodes, nil
}

func getResults(requestID, checkType string) ([]Result, error) {
	apiURL := fmt.Sprintf("%s/check-result/%s", apiBase, requestID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse the dynamic JSON structure
	var rawResults map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawResults); err != nil {
		return nil, fmt.Errorf("failed to parse results: %w", err)
	}

	var results []Result
	for node, raw := range rawResults {
		r := Result{Node: node}

		// Check if null (pending)
		if string(raw) == "null" {
			r.Pending = true
			results = append(results, r)
			continue
		}

		// Parse based on check type
		switch checkType {
		case "dns":
			parseDNSResult(&r, raw)
		case "tcp":
			parseTCPResult(&r, raw)
		case "redirect":
			parseRedirectResult(&r, raw)
		default:
			parseHTTPResult(&r, raw)
		}

		results = append(results, r)
	}

	return results, nil
}

// parseHTTPResult parses HTTP check results
func parseHTTPResult(r *Result, raw json.RawMessage) {
	// Parse the array structure: [[status, time_or_error, status_text, http_code, ip]]
	var nodeResult [][]any
	if err := json.Unmarshal(raw, &nodeResult); err != nil {
		r.Status = "parse error"
		return
	}

	if len(nodeResult) == 0 || len(nodeResult[0]) < 3 {
		r.Status = "incomplete data"
		return
	}

	data := nodeResult[0]

	// First element is success indicator (1 = success)
	if status, ok := data[0].(float64); ok && status == 1 {
		r.Success = true
		// Second element is response time in seconds
		if t, ok := data[1].(float64); ok {
			r.Time = t
		}
		// Third element is status text
		if s, ok := data[2].(string); ok {
			r.Status = s
		}
		// Fourth element is HTTP code (can be string or number)
		if len(data) > 3 {
			switch v := data[3].(type) {
			case string:
				r.Status = v
			case float64:
				r.Status = fmt.Sprintf("%d", int(v))
			}
		}
		// Fifth element is IP
		if len(data) > 4 {
			if ip, ok := data[4].(string); ok {
				r.IP = ip
			}
		}
	} else {
		// Failure - second element contains error message
		r.Success = false
		if len(data) > 2 {
			if errMsg, ok := data[2].(string); ok {
				r.Status = errMsg
			}
		}
		if r.Status == "" {
			r.Status = "unknown error"
		}
	}
}

// parseRedirectResult parses HTTP check results expecting a 301/302 redirect
func parseRedirectResult(r *Result, raw json.RawMessage) {
	var nodeResult [][]any
	if err := json.Unmarshal(raw, &nodeResult); err != nil {
		r.Status = "parse error"
		return
	}

	if len(nodeResult) == 0 || len(nodeResult[0]) < 3 {
		r.Status = "incomplete data"
		return
	}

	data := nodeResult[0]

	// First element is success indicator (1 = HTTP request succeeded)
	if status, ok := data[0].(float64); ok && status == 1 {
		// Get the HTTP status code from element 3
		var httpCode int
		if len(data) > 3 {
			switch v := data[3].(type) {
			case string:
				// Try to parse string as int
				fmt.Sscanf(v, "%d", &httpCode)
			case float64:
				httpCode = int(v)
			}
		}

		// For redirect check: 301/302 = success, anything else = failure
		if httpCode == 301 || httpCode == 302 {
			r.Success = true
			r.Status = fmt.Sprintf("%d redirect", httpCode)
		} else if httpCode == 200 {
			r.Success = false
			r.Status = "200 (no redirect)"
		} else {
			r.Success = false
			r.Status = fmt.Sprintf("%d (expected 301/302)", httpCode)
		}

		// Get response time
		if t, ok := data[1].(float64); ok {
			r.Time = t
		}
		// Get IP
		if len(data) > 4 {
			if ip, ok := data[4].(string); ok {
				r.IP = ip
			}
		}
	} else {
		// HTTP request failed
		r.Success = false
		if len(data) > 2 {
			if errMsg, ok := data[2].(string); ok {
				r.Status = errMsg
			}
		}
		if r.Status == "" {
			r.Status = "request failed"
		}
	}
}

// parseDNSResult parses DNS check results
// Format: {"A":["104.21.x.x","172.67.x.x"],"AAAA":["2606:4700:..."],"TTL":300}
func parseDNSResult(r *Result, raw json.RawMessage) {
	// DNS results are wrapped in an array
	var wrapper []json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		r.Status = "parse error"
		return
	}

	if len(wrapper) == 0 {
		r.Status = "no data"
		return
	}

	// Parse the actual DNS result object
	var dnsResult map[string]any
	if err := json.Unmarshal(wrapper[0], &dnsResult); err != nil {
		r.Status = "parse error"
		return
	}

	// Check for A records
	var ips []string
	if aRecords, ok := dnsResult["A"].([]any); ok {
		for _, ip := range aRecords {
			if ipStr, ok := ip.(string); ok {
				ips = append(ips, ipStr)
			}
		}
	}

	if len(ips) > 0 {
		r.Success = true
		r.IP = strings.Join(ips, ", ")
		r.Status = fmt.Sprintf("A: %s", r.IP)

		// Add AAAA if present
		if aaaaRecords, ok := dnsResult["AAAA"].([]any); ok && len(aaaaRecords) > 0 {
			r.Status += " (+AAAA)"
		}
	} else {
		r.Success = false
		r.Status = "no A records"
	}
}

// parseTCPResult parses TCP check results
// Format: [{"address":"ip","time":0.123}] or [{"error":"message"}] on failure
func parseTCPResult(r *Result, raw json.RawMessage) {
	var nodeResult []map[string]any
	if err := json.Unmarshal(raw, &nodeResult); err != nil {
		r.Status = "parse error"
		return
	}

	if len(nodeResult) == 0 {
		r.Status = "incomplete data"
		return
	}

	data := nodeResult[0]

	// Check for error field (failure case)
	if errMsg, ok := data["error"].(string); ok {
		r.Success = false
		r.Status = errMsg
		return
	}

	// Success case: object with address and time
	r.Success = true
	if addr, ok := data["address"].(string); ok {
		r.IP = addr
	}
	if t, ok := data["time"].(float64); ok {
		r.Time = t
		r.Status = fmt.Sprintf("%.0fms", t*1000)
	} else {
		r.Status = "connected"
	}
}

// runGitHubIssueMode runs HTTP check in GitHub Issue mode
// Outputs markdown report and exits 1 if issues detected
func runGitHubIssueMode(targetURL string, maxNodes, waitSecs int) {
	host := prepareHost("http", targetURL)

	// Initiate HTTP check
	requestID, _, err := initiateCheck("http", host, maxNodes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initiate check: %v\n", err)
		os.Exit(1)
	}

	time.Sleep(time.Duration(waitSecs) * time.Second)

	// Get results
	results, err := getResults(requestID, "http")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get results: %v\n", err)
		os.Exit(1)
	}

	// Build current state
	current := buildState("http", results)

	// Load previous state
	previous, _ := loadState()

	// Generate markdown report
	report, hasIssues := generateMarkdownReport(current, previous, results, requestID)
	fmt.Println(report)

	// Save current state
	if err := saveState(current); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save state: %v\n", err)
	}

	// Exit 1 if issues detected (triggers GitHub Issue creation)
	if hasIssues {
		os.Exit(1)
	}
}

// buildState creates a State from check results
func buildState(checkType string, results []Result) *State {
	state := &State{
		Timestamp:  time.Now().UTC(),
		CheckType:  checkType,
		TotalNodes: len(results),
	}

	var times []float64
	for _, r := range results {
		if r.Pending {
			continue
		}
		if r.Success {
			state.OKCount++
			if r.Time > 0 {
				times = append(times, r.Time*1000) // Convert to ms
			}
		} else {
			state.FailedCount++
			state.FailedNodes = append(state.FailedNodes, r.Node)
		}
	}

	// Calculate response time stats
	if len(times) > 0 {
		var sum float64
		for _, t := range times {
			sum += t
			if t > state.MaxResponseMS {
				state.MaxResponseMS = t
			}
		}
		state.AvgResponseMS = sum / float64(len(times))
	}

	return state
}

// generateMarkdownReport creates markdown output and returns whether issues were detected
func generateMarkdownReport(current, previous *State, results []Result, requestID string) (string, bool) {
	var sb strings.Builder
	hasIssues := false

	sb.WriteString("## Site Reachability Check\n\n")
	sb.WriteString(fmt.Sprintf("**Target:** %s\n", defaultURL))
	sb.WriteString(fmt.Sprintf("**Time:** %s\n\n", current.Timestamp.Format("2006-01-02 15:04 UTC")))

	// Check for issues
	var issues []string

	// Issue 1: Any failures
	if current.FailedCount > 0 {
		issues = append(issues, fmt.Sprintf("**%d nodes failed** to reach the site", current.FailedCount))
		hasIssues = true
	}

	// Issue 2: New failures compared to previous
	if previous != nil {
		if previous.FailedCount == 0 && current.FailedCount > 0 {
			issues = append(issues, "**New failures detected** (was 100% reachable)")
			hasIssues = true
		} else if current.FailedCount >= previous.FailedCount+3 {
			issues = append(issues, fmt.Sprintf("**Failure count increased** from %d to %d", previous.FailedCount, current.FailedCount))
			hasIssues = true
		}

		// Issue 3: Response time degradation (>50% increase)
		if previous.AvgResponseMS > 0 && current.AvgResponseMS > previous.AvgResponseMS*1.5 {
			issues = append(issues, fmt.Sprintf("**Response time degraded** from %.0fms to %.0fms avg (+%.0f%%)",
				previous.AvgResponseMS, current.AvgResponseMS,
				(current.AvgResponseMS-previous.AvgResponseMS)/previous.AvgResponseMS*100))
			hasIssues = true
		}
	}

	// Issues section
	if len(issues) > 0 {
		sb.WriteString("### Issues Detected\n")
		for _, issue := range issues {
			sb.WriteString(fmt.Sprintf("- %s\n", issue))
		}
		sb.WriteString("\n")
	}

	// Results summary
	sb.WriteString("### Results\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Nodes Checked | %d |\n", current.TotalNodes))
	sb.WriteString(fmt.Sprintf("| Successful | %d |\n", current.OKCount))
	sb.WriteString(fmt.Sprintf("| Failed | %d |\n", current.FailedCount))
	if current.AvgResponseMS > 0 {
		sb.WriteString(fmt.Sprintf("| Avg Response | %.0fms |\n", current.AvgResponseMS))
		sb.WriteString(fmt.Sprintf("| Max Response | %.0fms |\n", current.MaxResponseMS))
	}

	// Comparison with previous
	if previous != nil {
		sb.WriteString("\n### Comparison with Previous Check\n\n")
		sb.WriteString("| Metric | Previous | Current | Change |\n")
		sb.WriteString("|--------|----------|---------|--------|\n")
		sb.WriteString(fmt.Sprintf("| Failed Nodes | %d | %d | %+d |\n",
			previous.FailedCount, current.FailedCount, current.FailedCount-previous.FailedCount))
		if previous.AvgResponseMS > 0 && current.AvgResponseMS > 0 {
			change := (current.AvgResponseMS - previous.AvgResponseMS) / previous.AvgResponseMS * 100
			sb.WriteString(fmt.Sprintf("| Avg Response | %.0fms | %.0fms | %+.0f%% |\n",
				previous.AvgResponseMS, current.AvgResponseMS, change))
		}
	}

	// Failed nodes detail
	if current.FailedCount > 0 {
		sb.WriteString("\n### Failed Nodes\n")
		for _, r := range results {
			if !r.Success && !r.Pending {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", r.Node, r.Status))
			}
		}
	}

	sb.WriteString(fmt.Sprintf("\n---\n[Full Report](%s/check-report/%s) | *Generated by site monitor workflow*\n", apiBase, requestID))

	return sb.String(), hasIssues
}

// loadState loads the previous state from disk
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

// saveState saves the current state to disk
func saveState(state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0644)
}
