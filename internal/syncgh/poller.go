package syncgh

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v67/github"
)

// RepoConfig holds configuration for checking a repository
type RepoConfig struct {
	Subsystem string
	UseTag    bool   // true = check tag, false = check branch
	Branch    string // branch name if UseTag=false
	Tag       string // tag to check if UseTag=true
}

// Poller checks GitHub repositories for updates periodically
type Poller struct {
	client   *github.Client
	interval time.Duration
	repos    []RepoConfig
	onUpdate func(subsystem, oldVersion, newVersion string) // callback on update
}

// NewPoller creates a new poller with specified interval
// Set GITHUB_TOKEN env var for authenticated requests (5000/hour vs 60/hour)
func NewPoller(interval time.Duration, repos []RepoConfig) *Poller {
	var client *github.Client
	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		client = github.NewClient(nil).WithAuthToken(token)
		log.Printf("sync-gh: Using authenticated GitHub API (5000 req/hour)")
	} else {
		client = github.NewClient(nil)
		log.Printf("sync-gh: Using unauthenticated GitHub API (60 req/hour). Set GITHUB_TOKEN for higher limits.")
	}

	return &Poller{
		client:   client,
		interval: interval,
		repos:    repos,
	}
}

// OnUpdate sets the callback for when an update is detected
func (p *Poller) OnUpdate(callback func(subsystem, oldVersion, newVersion string)) {
	p.onUpdate = callback
}

// Start begins the polling loop (blocking)
func (p *Poller) Start() error {
	log.Printf("sync-gh: Starting poller (interval: %v)", p.interval)

	// Do initial check immediately
	p.checkAll()

	// Then poll on interval
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for range ticker.C {
		p.checkAll()
	}

	return nil
}

// StartAsync begins the polling loop in background
func (p *Poller) StartAsync() {
	go func() {
		if err := p.Start(); err != nil {
			log.Printf("sync-gh: Poller error: %v", err)
		}
	}()
}

// checkAll checks all configured repositories for updates
func (p *Poller) checkAll() {
	log.Printf("sync-gh: Polling repositories for updates...")

	for _, config := range p.repos {
		if err := p.checkRepo(config); err != nil {
			log.Printf("sync-gh: Failed to check %s: %v", config.Subsystem, err)
		}
	}
	log.Printf("sync-gh: Polling cycle complete")
}

// checkRepo checks a single repository for updates
func (p *Poller) checkRepo(config RepoConfig) error {
	ctx := context.Background()

	// Parse repo format "owner/repo" from subsystem or explicit config
	owner, repoName := parseRepo(config.Subsystem)
	if owner == "" || repoName == "" {
		return fmt.Errorf("invalid repo format: %s (expected owner/repo)", config.Subsystem)
	}

	var latestHash string
	var err error

	if config.UseTag && config.Tag != "" {
		// Check specific tag
		log.Printf("sync-gh: Fetching tag %s from %s/%s", config.Tag, owner, repoName)
		latestHash, err = p.getTagCommit(ctx, owner, repoName, config.Tag)
		if err != nil {
			return fmt.Errorf("failed to get tag commit: %w", err)
		}
	} else {
		// Check latest commit on branch
		branch := config.Branch
		if branch == "" {
			branch = "main"
		}
		log.Printf("sync-gh: Fetching latest commit from %s/%s [%s]", owner, repoName, branch)
		latestHash, err = p.getLatestCommit(ctx, owner, repoName, branch)
		if err != nil {
			return fmt.Errorf("failed to get latest commit: %w", err)
		}
	}

	log.Printf("sync-gh: %s latest: %s", config.Subsystem, latestHash)

	// If callback set, trigger it (caller can compare with local state)
	if p.onUpdate != nil {
		p.onUpdate(config.Subsystem, "", latestHash)
	}

	return nil
}

// getTagCommit gets the commit hash for a specific tag
func (p *Poller) getTagCommit(ctx context.Context, owner, repo, tag string) (string, error) {
	ref, _, err := p.client.Git.GetRef(ctx, owner, repo, "tags/"+tag)
	if err != nil {
		return "", fmt.Errorf("failed to get tag ref: %w", err)
	}

	commitHash := ref.GetObject().GetSHA()
	if len(commitHash) > 8 {
		commitHash = commitHash[:8]
	}

	return commitHash, nil
}

// getLatestCommit gets the latest commit hash from a branch
func (p *Poller) getLatestCommit(ctx context.Context, owner, repo, branch string) (string, error) {
	commits, _, err := p.client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
		SHA:         branch,
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil {
		return "", fmt.Errorf("failed to list commits: %w", err)
	}

	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found")
	}

	commitHash := commits[0].GetSHA()
	if len(commitHash) > 8 {
		commitHash = commitHash[:8]
	}

	return commitHash, nil
}

// parseRepo splits "owner/repo" into (owner, repo)
func parseRepo(repo string) (string, string) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// GetLatestRelease gets the latest release tag for a repository
func GetLatestRelease(owner, repo string) (string, error) {
	client := github.NewClient(nil)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		client = client.WithAuthToken(token)
	}

	release, _, err := client.Repositories.GetLatestRelease(context.Background(), owner, repo)
	if err != nil {
		return "", fmt.Errorf("failed to get latest release: %w", err)
	}

	return release.GetTagName(), nil
}
