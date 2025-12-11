// mailerlite - CLI tool for MailerLite subscriber management
//
// Usage:
//
//	mailerlite subscribers list         List all subscribers
//	mailerlite subscribers count        Count total subscribers
//	mailerlite subscribers get EMAIL    Get subscriber by email
//	mailerlite subscribers add EMAIL    Add/update subscriber
//	mailerlite subscribers delete EMAIL Delete subscriber
//	mailerlite groups list              List all groups
//	mailerlite groups create NAME       Create a group
//	mailerlite groups subscribers ID    List subscribers in a group
//	mailerlite groups assign ID EMAIL   Assign subscriber to group
//	mailerlite groups unassign ID EMAIL Remove subscriber from group
//	mailerlite forms list               List all forms
//	mailerlite forms subscribers ID     List subscribers for a form
//	mailerlite stats                    Show account statistics
//
// Flags:
//
//	-github-issue    Output markdown for GitHub issue
//	-v               Verbose output
//	-version         Show version
//
// Environment:
//
//	MAILERLITE_API_KEY    API key (required)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mailerlite/mailerlite-go"
)

var version = "dev"

func main() {
	// Global flags
	var (
		githubIssue = flag.Bool("github-issue", false, "Output markdown for GitHub issue")
		verbose     = flag.Bool("v", false, "Verbose output")
		showVersion = flag.Bool("version", false, "Show version")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("mailerlite %s\n", version)
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	// Get API key
	apiKey := os.Getenv("MAILERLITE_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ERROR: MAILERLITE_API_KEY environment variable is required")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Get your API key from: https://dashboard.mailerlite.com/integrations/api")
		os.Exit(1)
	}

	client := mailerlite.NewClient(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	app := &App{
		client:      client,
		ctx:         ctx,
		githubIssue: *githubIssue,
		verbose:     *verbose,
	}

	if err := app.Run(args); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

type App struct {
	client      *mailerlite.Client
	ctx         context.Context
	githubIssue bool
	verbose     bool
}

func (a *App) Run(args []string) error {
	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "subscribers":
		return a.handleSubscribers(subArgs)
	case "groups":
		return a.handleGroups(subArgs)
	case "forms":
		return a.handleForms(subArgs)
	case "stats":
		return a.handleStats()
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *App) handleSubscribers(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("subscribers subcommand required: list, count, get, add, delete")
	}

	switch args[0] {
	case "list":
		return a.subscribersList()
	case "count":
		return a.subscribersCount()
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("email required: subscribers get EMAIL")
		}
		return a.subscribersGet(args[1])
	case "add":
		if len(args) < 2 {
			return fmt.Errorf("email required: subscribers add EMAIL [NAME]")
		}
		name := ""
		if len(args) >= 3 {
			name = strings.Join(args[2:], " ")
		}
		return a.subscribersAdd(args[1], name)
	case "delete":
		if len(args) < 2 {
			return fmt.Errorf("email required: subscribers delete EMAIL")
		}
		return a.subscribersDelete(args[1])
	default:
		return fmt.Errorf("unknown subscribers subcommand: %s", args[0])
	}
}

func (a *App) subscribersList() error {
	options := &mailerlite.ListSubscriberOptions{
		Limit: 100,
		Page:  1,
	}

	subscribers, _, err := a.client.Subscriber.List(a.ctx, options)
	if err != nil {
		return fmt.Errorf("list subscribers: %w", err)
	}

	if a.githubIssue {
		fmt.Println("## Subscribers")
		fmt.Println()
		fmt.Printf("Total: **%d**\n\n", subscribers.Meta.Total)
		if len(subscribers.Data) > 0 {
			fmt.Println("| Email | Status | Subscribed |")
			fmt.Println("|-------|--------|------------|")
			for _, s := range subscribers.Data {
				fmt.Printf("| %s | %s | %s |\n", s.Email, s.Status, s.SubscribedAt)
			}
		}
		return nil
	}

	fmt.Printf("Total subscribers: %d\n\n", subscribers.Meta.Total)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMAIL\tSTATUS\tOPENS\tCLICKS\tSUBSCRIBED")
	for _, s := range subscribers.Data {
		subscribed := ""
		if s.SubscribedAt != "" {
			if t, err := time.Parse(time.RFC3339, s.SubscribedAt); err == nil {
				subscribed = t.Format("2006-01-02")
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", s.Email, s.Status, s.OpensCount, s.ClicksCount, subscribed)
	}
	w.Flush()

	return nil
}

func (a *App) subscribersCount() error {
	count, _, err := a.client.Subscriber.Count(a.ctx)
	if err != nil {
		return fmt.Errorf("count subscribers: %w", err)
	}

	if a.githubIssue {
		fmt.Printf("**Total Subscribers:** %d\n", count.Total)
		return nil
	}

	fmt.Printf("Total subscribers: %d\n", count.Total)
	return nil
}

func (a *App) subscribersGet(email string) error {
	options := &mailerlite.GetSubscriberOptions{
		Email: email,
	}

	subscriber, _, err := a.client.Subscriber.Get(a.ctx, options)
	if err != nil {
		return fmt.Errorf("get subscriber: %w", err)
	}

	s := subscriber.Data
	if a.githubIssue {
		fmt.Printf("## Subscriber: %s\n\n", s.Email)
		fmt.Printf("- **Status:** %s\n", s.Status)
		fmt.Printf("- **Opens:** %d\n", s.OpensCount)
		fmt.Printf("- **Clicks:** %d\n", s.ClicksCount)
		fmt.Printf("- **Subscribed:** %s\n", s.SubscribedAt)
		if len(s.Groups) > 0 {
			fmt.Println("- **Groups:**")
			for _, g := range s.Groups {
				fmt.Printf("  - %s\n", g.Name)
			}
		}
		return nil
	}

	fmt.Printf("Email:      %s\n", s.Email)
	fmt.Printf("Status:     %s\n", s.Status)
	fmt.Printf("Opens:      %d\n", s.OpensCount)
	fmt.Printf("Clicks:     %d\n", s.ClicksCount)
	fmt.Printf("Open Rate:  %.1f%%\n", s.OpenRate*100)
	fmt.Printf("Click Rate: %.1f%%\n", s.ClickRate*100)
	fmt.Printf("Subscribed: %s\n", s.SubscribedAt)
	fmt.Printf("Created:    %s\n", s.CreatedAt)

	if len(s.Groups) > 0 {
		fmt.Println("\nGroups:")
		for _, g := range s.Groups {
			fmt.Printf("  - %s (ID: %s)\n", g.Name, g.ID)
		}
	}

	if len(s.Fields) > 0 && a.verbose {
		fmt.Println("\nCustom Fields:")
		for k, v := range s.Fields {
			if v != nil && v != "" {
				fmt.Printf("  %s: %v\n", k, v)
			}
		}
	}

	return nil
}

func (a *App) subscribersAdd(email, name string) error {
	subscriber := &mailerlite.UpsertSubscriber{
		Email: email,
	}

	// Set name if provided
	if name != "" {
		subscriber.Fields = map[string]interface{}{
			"name": name,
		}
	}

	result, _, err := a.client.Subscriber.Upsert(a.ctx, subscriber)
	if err != nil {
		return fmt.Errorf("add subscriber: %w", err)
	}

	s := result.Data
	if a.githubIssue {
		fmt.Printf("## Subscriber Added\n\n")
		fmt.Printf("- **Email:** %s\n", s.Email)
		fmt.Printf("- **Status:** %s\n", s.Status)
		return nil
	}

	fmt.Printf("Subscriber added/updated:\n")
	fmt.Printf("  Email:  %s\n", s.Email)
	fmt.Printf("  Status: %s\n", s.Status)
	fmt.Printf("  ID:     %s\n", s.ID)

	return nil
}

func (a *App) subscribersDelete(email string) error {
	// First, get the subscriber to find their ID
	options := &mailerlite.GetSubscriberOptions{
		Email: email,
	}

	subscriber, _, err := a.client.Subscriber.Get(a.ctx, options)
	if err != nil {
		return fmt.Errorf("subscriber not found: %w", err)
	}

	// Delete by ID
	_, err = a.client.Subscriber.Delete(a.ctx, subscriber.Data.ID)
	if err != nil {
		return fmt.Errorf("delete subscriber: %w", err)
	}

	if a.githubIssue {
		fmt.Printf("## Subscriber Deleted\n\n")
		fmt.Printf("- **Email:** %s\n", email)
		return nil
	}

	fmt.Printf("Subscriber deleted: %s\n", email)

	return nil
}

func (a *App) handleGroups(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("groups subcommand required: list, create, subscribers, assign, unassign")
	}

	switch args[0] {
	case "list":
		return a.groupsList()
	case "create":
		if len(args) < 2 {
			return fmt.Errorf("group name required: groups create NAME")
		}
		return a.groupsCreate(strings.Join(args[1:], " "))
	case "subscribers":
		if len(args) < 2 {
			return fmt.Errorf("group ID required: groups subscribers ID")
		}
		return a.groupsSubscribers(args[1])
	case "assign":
		if len(args) < 3 {
			return fmt.Errorf("group ID and email required: groups assign GROUP_ID EMAIL")
		}
		return a.groupsAssign(args[1], args[2])
	case "unassign":
		if len(args) < 3 {
			return fmt.Errorf("group ID and email required: groups unassign GROUP_ID EMAIL")
		}
		return a.groupsUnassign(args[1], args[2])
	default:
		return fmt.Errorf("unknown groups subcommand: %s", args[0])
	}
}

func (a *App) groupsList() error {
	options := &mailerlite.ListGroupOptions{
		Page:  1,
		Limit: 100,
		Sort:  mailerlite.SortByName,
	}

	groups, _, err := a.client.Group.List(a.ctx, options)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}

	if a.githubIssue {
		fmt.Println("## Groups")
		fmt.Println()
		fmt.Printf("Total: **%d**\n\n", groups.Meta.Total)
		if len(groups.Data) > 0 {
			fmt.Println("| Name | Active | Sent |")
			fmt.Println("|------|--------|------|")
			for _, g := range groups.Data {
				fmt.Printf("| %s | %d | %d |\n", g.Name, g.ActiveCount, g.SentCount)
			}
		}
		return nil
	}

	fmt.Printf("Total groups: %d\n\n", groups.Meta.Total)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tACTIVE\tSENT")
	for _, g := range groups.Data {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", g.ID, g.Name, g.ActiveCount, g.SentCount)
	}
	w.Flush()

	return nil
}

func (a *App) groupsSubscribers(groupID string) error {
	options := &mailerlite.ListGroupSubscriberOptions{
		GroupID: groupID,
		Page:    1,
		Limit:   100,
	}

	subscribers, _, err := a.client.Group.Subscribers(a.ctx, options)
	if err != nil {
		return fmt.Errorf("list group subscribers: %w", err)
	}

	if a.githubIssue {
		fmt.Printf("## Group Subscribers (ID: %s)\n\n", groupID)
		fmt.Printf("Total: **%d**\n\n", subscribers.Meta.Total)
		if len(subscribers.Data) > 0 {
			fmt.Println("| Email | Status |")
			fmt.Println("|-------|--------|")
			for _, s := range subscribers.Data {
				fmt.Printf("| %s | %s |\n", s.Email, s.Status)
			}
		}
		return nil
	}

	fmt.Printf("Group %s - %d subscribers\n\n", groupID, subscribers.Meta.Total)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMAIL\tSTATUS")
	for _, s := range subscribers.Data {
		fmt.Fprintf(w, "%s\t%s\n", s.Email, s.Status)
	}
	w.Flush()

	return nil
}

func (a *App) groupsCreate(name string) error {
	result, _, err := a.client.Group.Create(a.ctx, name)
	if err != nil {
		return fmt.Errorf("create group: %w", err)
	}

	g := result.Data
	if a.githubIssue {
		fmt.Printf("## Group Created\n\n")
		fmt.Printf("- **Name:** %s\n", g.Name)
		fmt.Printf("- **ID:** %s\n", g.ID)
		return nil
	}

	fmt.Printf("Group created:\n")
	fmt.Printf("  Name: %s\n", g.Name)
	fmt.Printf("  ID:   %s\n", g.ID)

	return nil
}

func (a *App) groupsAssign(groupID, email string) error {
	// First, get the subscriber to find their ID
	options := &mailerlite.GetSubscriberOptions{
		Email: email,
	}

	subscriber, _, err := a.client.Subscriber.Get(a.ctx, options)
	if err != nil {
		return fmt.Errorf("subscriber not found: %w", err)
	}

	_, _, err = a.client.Group.Assign(a.ctx, groupID, subscriber.Data.ID)
	if err != nil {
		return fmt.Errorf("assign to group: %w", err)
	}

	if a.githubIssue {
		fmt.Printf("## Subscriber Assigned to Group\n\n")
		fmt.Printf("- **Email:** %s\n", email)
		fmt.Printf("- **Group ID:** %s\n", groupID)
		return nil
	}

	fmt.Printf("Subscriber %s assigned to group %s\n", email, groupID)

	return nil
}

func (a *App) groupsUnassign(groupID, email string) error {
	// First, get the subscriber to find their ID
	options := &mailerlite.GetSubscriberOptions{
		Email: email,
	}

	subscriber, _, err := a.client.Subscriber.Get(a.ctx, options)
	if err != nil {
		return fmt.Errorf("subscriber not found: %w", err)
	}

	_, err = a.client.Group.UnAssign(a.ctx, groupID, subscriber.Data.ID)
	if err != nil {
		return fmt.Errorf("unassign from group: %w", err)
	}

	if a.githubIssue {
		fmt.Printf("## Subscriber Removed from Group\n\n")
		fmt.Printf("- **Email:** %s\n", email)
		fmt.Printf("- **Group ID:** %s\n", groupID)
		return nil
	}

	fmt.Printf("Subscriber %s removed from group %s\n", email, groupID)

	return nil
}

func (a *App) handleForms(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("forms subcommand required: list, subscribers")
	}

	switch args[0] {
	case "list":
		return a.formsList()
	case "subscribers":
		if len(args) < 2 {
			return fmt.Errorf("form ID required: forms subscribers ID")
		}
		return a.formsSubscribers(args[1])
	default:
		return fmt.Errorf("unknown forms subcommand: %s", args[0])
	}
}

func (a *App) formsList() error {
	// List all form types
	formTypes := []string{"popup", "embedded", "promotion"}

	var allForms []mailerlite.Form
	for _, formType := range formTypes {
		options := &mailerlite.ListFormOptions{
			Type:  formType,
			Page:  1,
			Limit: 100,
		}

		forms, _, err := a.client.Form.List(a.ctx, options)
		if err != nil {
			if a.verbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to list %s forms: %v\n", formType, err)
			}
			continue
		}
		allForms = append(allForms, forms.Data...)
	}

	if a.githubIssue {
		fmt.Println("## Forms")
		fmt.Println()
		fmt.Printf("Total: **%d**\n\n", len(allForms))
		if len(allForms) > 0 {
			fmt.Println("| Name | Type | Opens | Conversions |")
			fmt.Println("|------|------|-------|-------------|")
			for _, f := range allForms {
				fmt.Printf("| %s | %s | %d | %d |\n", f.Name, f.Type, f.OpensCount, f.ConversionsCount)
			}
		}
		return nil
	}

	fmt.Printf("Total forms: %d\n\n", len(allForms))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tTYPE\tOPENS\tCONVERSIONS")
	for _, f := range allForms {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\n", f.Id, f.Name, f.Type, f.OpensCount, f.ConversionsCount)
	}
	w.Flush()

	return nil
}

func (a *App) formsSubscribers(formID string) error {
	options := &mailerlite.ListFormSubscriberOptions{
		FormID: formID,
		Page:   1,
		Limit:  100,
	}

	subscribers, _, err := a.client.Form.Subscribers(a.ctx, options)
	if err != nil {
		return fmt.Errorf("list form subscribers: %w", err)
	}

	if a.githubIssue {
		fmt.Printf("## Form Subscribers (ID: %s)\n\n", formID)
		fmt.Printf("Total: **%d**\n\n", subscribers.Meta.Total)
		if len(subscribers.Data) > 0 {
			fmt.Println("| Email | Status |")
			fmt.Println("|-------|--------|")
			for _, s := range subscribers.Data {
				fmt.Printf("| %s | %s |\n", s.Email, s.Status)
			}
		}
		return nil
	}

	fmt.Printf("Form %s - %d subscribers\n\n", formID, subscribers.Meta.Total)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "EMAIL\tSTATUS")
	for _, s := range subscribers.Data {
		fmt.Fprintf(w, "%s\t%s\n", s.Email, s.Status)
	}
	w.Flush()

	return nil
}

func (a *App) handleStats() error {
	// Get subscriber count
	count, _, err := a.client.Subscriber.Count(a.ctx)
	if err != nil {
		return fmt.Errorf("get subscriber count: %w", err)
	}

	// Get groups
	groupOptions := &mailerlite.ListGroupOptions{
		Page:  1,
		Limit: 100,
	}
	groups, _, err := a.client.Group.List(a.ctx, groupOptions)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}

	// Count active subscribers across groups
	totalActive := 0
	for _, g := range groups.Data {
		totalActive += g.ActiveCount
	}

	if a.githubIssue {
		fmt.Println("## MailerLite Statistics")
		fmt.Println()
		fmt.Printf("| Metric | Value |\n")
		fmt.Printf("|--------|-------|\n")
		fmt.Printf("| Total Subscribers | %d |\n", count.Total)
		fmt.Printf("| Total Groups | %d |\n", groups.Meta.Total)
		fmt.Println()
		fmt.Printf("*Generated: %s*\n", time.Now().Format("2006-01-02 15:04:05"))
		return nil
	}

	fmt.Println("MailerLite Statistics")
	fmt.Println(strings.Repeat("=", 40))
	fmt.Printf("Total Subscribers: %d\n", count.Total)
	fmt.Printf("Total Groups:      %d\n", groups.Meta.Total)
	fmt.Println()
	fmt.Printf("Generated: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	return nil
}

func printUsage() {
	fmt.Println("mailerlite - CLI tool for MailerLite subscriber management")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  mailerlite [flags] <command> [subcommand] [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  subscribers list              List all subscribers")
	fmt.Println("  subscribers count             Count total subscribers")
	fmt.Println("  subscribers get EMAIL         Get subscriber by email")
	fmt.Println("  subscribers add EMAIL [NAME]  Add or update subscriber")
	fmt.Println("  subscribers delete EMAIL      Delete subscriber")
	fmt.Println("  groups list                   List all groups")
	fmt.Println("  groups create NAME            Create a new group")
	fmt.Println("  groups subscribers ID         List subscribers in a group")
	fmt.Println("  groups assign ID EMAIL        Assign subscriber to group")
	fmt.Println("  groups unassign ID EMAIL      Remove subscriber from group")
	fmt.Println("  forms list                    List all forms")
	fmt.Println("  forms subscribers ID          List subscribers for a form")
	fmt.Println("  stats                         Show account statistics")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -github-issue    Output markdown for GitHub issue")
	fmt.Println("  -v               Verbose output")
	fmt.Println("  -version         Show version")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  MAILERLITE_API_KEY    API key (required)")
	fmt.Println()
	fmt.Println("Get API key from: https://dashboard.mailerlite.com/integrations/api")
}
