package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/gitops"
	"github.com/spf13/cobra"
)

// GitCmd is the parent command for git operations
var GitCmd = &cobra.Command{
	Use:   "git",
	Short: "Git operations (no git binary required)",
	Long: `Git operations using go-git - no git binary needed.

Works identically on macOS, Linux, and Windows without requiring
git to be installed. Perfect for CI/CD and Docker environments.

Most commands accept an optional [path] argument to work with repos
at different locations (defaults to current directory).

Examples:
  xplat os git status              # Status of current repo
  xplat os git add .               # Stage all changes
  xplat os git commit -m "message" # Create commit
  xplat os git push                # Push to origin
  xplat os git pull                # Pull from origin
  xplat os git clone https://github.com/user/repo .src
  xplat os git checkout .src v2.0.0
  xplat os git hash .src
  xplat os git tags .src`,
}

var gitCloneCmd = &cobra.Command{
	Use:   "clone <url> <path> [version]",
	Short: "Clone a repository (shallow)",
	Args:  cobra.RangeArgs(2, 3),
	Run: func(cmd *cobra.Command, args []string) {
		url := args[0]
		path := args[1]
		version := ""
		if len(args) > 2 {
			version = args[2]
		}

		fmt.Printf("Cloning %s to %s", url, path)
		if version != "" {
			fmt.Printf(" @ %s", version)
		}
		fmt.Println()

		if err := gitops.Clone(url, path, version); err != nil {
			fmt.Fprintf(os.Stderr, "git clone: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Clone completed")
	},
}

var gitPullCmd = &cobra.Command{
	Use:   "pull <path>",
	Short: "Pull updates from origin",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]

		hash, err := gitops.Pull(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "git pull: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Updated to %s\n", hash)
	},
}

var gitFetchTags bool

var gitFetchCmd = &cobra.Command{
	Use:   "fetch <path>",
	Short: "Fetch updates from origin",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]

		if err := gitops.Fetch(path, gitFetchTags); err != nil {
			fmt.Fprintf(os.Stderr, "git fetch: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Fetch completed")
	},
}

var gitCheckoutCmd = &cobra.Command{
	Use:   "checkout <path> <ref>",
	Short: "Checkout tag/branch/commit",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]
		ref := args[1]

		if err := gitops.Checkout(path, ref); err != nil {
			fmt.Fprintf(os.Stderr, "git checkout: %v\n", err)
			os.Exit(1)
		}

		// Show new commit hash
		hash, _ := gitops.GetCommitHash(path)
		fmt.Printf("Checked out %s (%s)\n", ref, hash)
	},
}

var gitHashFull bool

var gitHashCmd = &cobra.Command{
	Use:   "hash <path>",
	Short: "Get commit hash of HEAD",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]

		var hash string
		var err error
		if gitHashFull {
			hash, err = gitops.GetFullCommitHash(path)
		} else {
			hash, err = gitops.GetCommitHash(path)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "git hash: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(hash)
	},
}

var gitTagsCmd = &cobra.Command{
	Use:   "tags <path>",
	Short: "List all tags",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]

		tags, err := gitops.GetTags(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "git tags: %v\n", err)
			os.Exit(1)
		}

		for _, tag := range tags {
			fmt.Println(tag)
		}
	},
}

var gitBranchCmd = &cobra.Command{
	Use:   "branch <path>",
	Short: "Get current branch name",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]

		branch, err := gitops.GetBranch(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "git branch: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(branch)
	},
}

var gitIsRepoCmd = &cobra.Command{
	Use:   "is-repo <path>",
	Short: "Check if path is a git repository",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := args[0]

		if gitops.IsRepo(path) {
			fmt.Println("true")
		} else {
			fmt.Println("false")
			os.Exit(1)
		}
	},
}

var gitStatusCmd = &cobra.Command{
	Use:   "status [path]",
	Short: "Show working tree status",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		status, err := gitops.Status(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "git status: %v\n", err)
			os.Exit(1)
		}

		if status.IsClean() {
			fmt.Println("Nothing to commit, working tree clean")
			return
		}

		for file, s := range status {
			var staging, worktree string
			switch s.Staging {
			case 'M':
				staging = "M"
			case 'A':
				staging = "A"
			case 'D':
				staging = "D"
			case 'R':
				staging = "R"
			case 'C':
				staging = "C"
			default:
				staging = " "
			}
			switch s.Worktree {
			case 'M':
				worktree = "M"
			case 'D':
				worktree = "D"
			case '?':
				worktree = "?"
			default:
				worktree = " "
			}
			fmt.Printf("%s%s %s\n", staging, worktree, file)
		}
	},
}

var gitAddCmd = &cobra.Command{
	Use:   "add [path] <files...>",
	Short: "Stage files for commit",
	Long: `Stage files for commit. Use "." to add all changes.

Examples:
  xplat os git add .              # Add all changes in current dir
  xplat os git add /repo .        # Add all changes in /repo
  xplat os git add . file.go      # Add specific file`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// If only one arg and it's ".", use current dir
		path := "."
		pattern := args[0]

		// If two args, first is path, second is pattern
		if len(args) > 1 {
			path = args[0]
			pattern = args[1]
		}

		if err := gitops.Add(path, pattern); err != nil {
			fmt.Fprintf(os.Stderr, "git add: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Staged changes")
	},
}

var gitCommitCmd = &cobra.Command{
	Use:   "commit [path] -m <message>",
	Short: "Create a commit",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		message, _ := cmd.Flags().GetString("message")
		if message == "" {
			fmt.Fprintf(os.Stderr, "git commit: message required (-m)\n")
			os.Exit(1)
		}

		hash, err := gitops.Commit(path, message)
		if err != nil {
			fmt.Fprintf(os.Stderr, "git commit: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Committed %s\n", hash)
	},
}

var gitPushCmd = &cobra.Command{
	Use:   "push [path]",
	Short: "Push commits to origin",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		if err := gitops.Push(path); err != nil {
			fmt.Fprintf(os.Stderr, "git push: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Pushed to origin")
	},
}

func init() {
	gitFetchCmd.Flags().BoolVar(&gitFetchTags, "tags", false, "Fetch tags as well")
	gitHashCmd.Flags().BoolVar(&gitHashFull, "full", false, "Show full commit hash")
	gitCommitCmd.Flags().StringP("message", "m", "", "Commit message")

	GitCmd.AddCommand(gitCloneCmd)
	GitCmd.AddCommand(gitPullCmd)
	GitCmd.AddCommand(gitFetchCmd)
	GitCmd.AddCommand(gitCheckoutCmd)
	GitCmd.AddCommand(gitHashCmd)
	GitCmd.AddCommand(gitTagsCmd)
	GitCmd.AddCommand(gitBranchCmd)
	GitCmd.AddCommand(gitIsRepoCmd)
	GitCmd.AddCommand(gitStatusCmd)
	GitCmd.AddCommand(gitAddCmd)
	GitCmd.AddCommand(gitCommitCmd)
	GitCmd.AddCommand(gitPushCmd)
}
