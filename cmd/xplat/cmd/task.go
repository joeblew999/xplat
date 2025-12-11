// Package cmd provides CLI commands for xplat.
//
// task.go - Embedded Task runner
//
// # Why Embed Task?
//
// Task (go-task) is embedded into xplat to create a single-binary bootstrap tool.
// This eliminates the need for separate Task installation in CI and simplifies
// the toolchain. With embedded Task, the only dependencies are:
//   - Go (provided by actions/setup-go in CI, or installed locally)
//   - xplat (built from source via `go build ./cmd/xplat`)
//
// # The Challenge
//
// Task's CLI entry point (cmd/task/task.go) uses an internal package
// (internal/flags) that cannot be imported externally due to Go's internal
// package restrictions. The internal/flags package:
//   - Parses CLI flags using spf13/pflag
//   - Provides a WithFlags() ExecutorOption that configures the Executor
//   - Contains validation logic for experiments and flag combinations
//
// # Our Approach
//
// Since we cannot import internal/flags, we replicate the flag parsing logic:
//   1. Define our own flags that mirror Task's flags exactly
//   2. Parse them using Cobra's flag parsing
//   3. Apply the parsed values directly to the Executor struct fields
//   4. Use Task's public API (Executor, args.Parse, ListOptions, etc.)
//
// This approach provides ~99% CLI compatibility. Known limitations:
//   - --experiments flag is not supported (rarely used)
//   - --completion flag is not supported (users can use standalone task for this)
//   - Some edge cases in flag validation may differ
//
// # Compatibility Guarantee
//
// The goal is 100% Taskfile compatibility - any Taskfile that works with
// standalone `task` should work identically with `xplat task`. Flag behavior
// should match as closely as possible.
//
// # Bootstrap Sequence
//
// Local development:
//   1. Use existing `task` to run taskfiles (already installed)
//   2. `task xplat:build` creates xplat with embedded Task
//   3. Can then use `xplat task` instead of `task`
//
// CI:
//   1. `go build ./cmd/xplat` creates xplat (no Task needed!)
//   2. `xplat task <taskname>` runs tasks
//
// # Task-UI Note
//
// Task-UI (github.com/titpetric/task-ui) does NOT embed Task - it calls the
// external `task` binary via exec.Command. This means task-ui still requires
// a task binary to be installed. Our approach is different: we truly embed
// Task's functionality.
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/go-task/task/v3"
	"github.com/go-task/task/v3/args"
	"github.com/go-task/task/v3/errors"
	"github.com/go-task/task/v3/taskfile/ast"
)

// TaskCmd embeds the Task runner into xplat.
//
// Usage:
//
//	xplat task [flags] [tasks...]
//
// This is designed to be a drop-in replacement for the standalone `task`
// binary. All flags match Task's CLI interface.
var TaskCmd = &cobra.Command{
	Use:   "task [flags] [tasks...]",
	Short: "Run Taskfile tasks (embedded Task runner)",
	Long: `Runs Taskfile tasks using the embedded Task runner.

This provides the same functionality as the standalone 'task' binary,
but bundled into xplat for simpler bootstrapping.

Examples:
  xplat task build
  xplat task -t taskfiles/Taskfile.dummy.yml release:build
  xplat task --list
  xplat task build -- --some-arg-for-task`,
	DisableFlagParsing: true, // We parse flags ourselves to match Task exactly
	RunE:               runTask,
}

// Task-compatible flags
// These mirror the flags defined in github.com/go-task/task/v3/internal/flags
var (
	taskDir         string
	taskFile        string
	taskForce       bool
	taskForceAll    bool
	taskSilent      bool
	taskVerbose     bool
	taskParallel    bool
	taskDry         bool
	taskSummary     bool
	taskStatus      bool
	taskList        bool
	taskListAll     bool
	taskListJson    bool
	taskColor       bool
	taskConcurrency int
	taskOutput      string
	taskInterval    time.Duration
	taskWatch       bool
	taskVersion     bool
	taskHelp        bool
	taskInit        bool
	taskGlobal      bool
	taskDownload    bool
	taskOffline     bool
	taskTimeout     time.Duration
	taskYes         bool
	taskInsecure    bool
	taskExitCode    bool
	taskClearCache  bool
)

func init() {
	// Match Task's flags exactly
	// See: https://github.com/go-task/task/blob/main/internal/flags/flags.go
	TaskCmd.Flags().StringVarP(&taskDir, "dir", "d", "", "Sets directory of execution")
	TaskCmd.Flags().StringVarP(&taskFile, "taskfile", "t", "", "Choose which Taskfile to run")
	TaskCmd.Flags().BoolVarP(&taskForce, "force", "f", false, "Forces execution even when the task is up-to-date")
	TaskCmd.Flags().BoolVar(&taskForceAll, "force-all", false, "Forces execution of all tasks including dependencies")
	TaskCmd.Flags().BoolVarP(&taskSilent, "silent", "s", false, "Disables echoing")
	TaskCmd.Flags().BoolVarP(&taskVerbose, "verbose", "v", false, "Enables verbose mode")
	TaskCmd.Flags().BoolVarP(&taskParallel, "parallel", "p", false, "Executes tasks provided on command line in parallel")
	TaskCmd.Flags().BoolVarP(&taskDry, "dry", "n", false, "Compiles and prints tasks in the order that they would be run, without executing them")
	TaskCmd.Flags().BoolVar(&taskSummary, "summary", false, "Show summary about a task")
	TaskCmd.Flags().BoolVar(&taskStatus, "status", false, "Exits with non-zero exit code if any of the given tasks is not up-to-date")
	TaskCmd.Flags().BoolVarP(&taskList, "list", "l", false, "Lists tasks with description")
	TaskCmd.Flags().BoolVarP(&taskListAll, "list-all", "a", false, "Lists tasks with or without a description")
	TaskCmd.Flags().BoolVar(&taskListJson, "json", false, "Formats task list as JSON")
	TaskCmd.Flags().BoolVarP(&taskColor, "color", "c", true, "Colored output")
	TaskCmd.Flags().IntVarP(&taskConcurrency, "concurrency", "C", 0, "Limit number of tasks to run concurrently")
	TaskCmd.Flags().StringVarP(&taskOutput, "output", "o", "", "Sets output style: interleaved, group, or prefixed")
	TaskCmd.Flags().DurationVar(&taskInterval, "interval", time.Second*5, "Interval to watch for changes")
	TaskCmd.Flags().BoolVarP(&taskWatch, "watch", "w", false, "Enables watch of the given task")
	TaskCmd.Flags().BoolVar(&taskVersion, "version", false, "Show Task version")
	TaskCmd.Flags().BoolVarP(&taskHelp, "help", "h", false, "Shows Task usage")
	TaskCmd.Flags().BoolVarP(&taskInit, "init", "i", false, "Creates a new Taskfile.yml")
	TaskCmd.Flags().BoolVarP(&taskGlobal, "global", "g", false, "Runs global Taskfile")
	TaskCmd.Flags().BoolVar(&taskDownload, "download", false, "Downloads missing remote taskfiles")
	TaskCmd.Flags().BoolVar(&taskOffline, "offline", false, "Runs without trying to update remote taskfiles")
	TaskCmd.Flags().DurationVar(&taskTimeout, "timeout", time.Second*10, "Timeout for downloading remote taskfiles")
	TaskCmd.Flags().BoolVarP(&taskYes, "yes", "y", false, "Assume yes on all prompts")
	TaskCmd.Flags().BoolVar(&taskInsecure, "insecure", false, "Allow insecure connections")
	TaskCmd.Flags().BoolVarP(&taskExitCode, "exit-code", "x", false, "Pass-through the exit code of the task command")
	TaskCmd.Flags().BoolVar(&taskClearCache, "clear-cache", false, "Clear remote taskfile cache")
}

// runTask is the main entry point for the embedded Task runner.
// It replicates the logic from github.com/go-task/task/v3/cmd/task/task.go
func runTask(cmd *cobra.Command, osArgs []string) error {
	// Extract CLI_ARGS (everything after "--") BEFORE parsing flags
	// This is critical because pflag.Parse() consumes the "--" separator,
	// making it impossible to distinguish CLI_ARGS from task names afterward.
	//
	// Example: xplat task build -- --some-arg
	//   osArgs = ["build", "--", "--some-arg"]
	//   After pflag.Parse(), remainingArgs = ["build", "--some-arg"]
	//   We lose the ability to know "--some-arg" was meant for CLI_ARGS!
	var cliArgs string
	argsForParsing := osArgs
	for i, arg := range osArgs {
		if arg == "--" {
			argsForParsing = osArgs[:i]
			if i+1 < len(osArgs) {
				cliArgs = strings.Join(osArgs[i+1:], " ")
			}
			break
		}
	}

	// Re-parse args since we disabled flag parsing in Cobra
	// This is necessary because we want to handle flags like Task does
	if err := cmd.Flags().Parse(argsForParsing); err != nil {
		return err
	}
	remainingArgs := cmd.Flags().Args()

	// Handle --version
	if taskVersion {
		fmt.Println("Task version 3.43.3 (embedded in xplat)")
		return nil
	}

	// Handle --help
	if taskHelp {
		return cmd.Help()
	}

	// Handle --init (create new Taskfile)
	if taskInit {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		path := wd
		if len(remainingArgs) > 0 {
			path = filepath.Join(wd, remainingArgs[0])
		}
		finalPath, err := task.InitTaskfile(path)
		if err != nil {
			return err
		}
		if !taskSilent {
			fmt.Printf("Taskfile created: %s\n", finalPath)
		}
		return nil
	}

	// Determine working directory
	// --global flag runs from user's home directory
	dir := taskDir
	if taskGlobal {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = home
		}
	}

	// Create and configure the Executor
	// Note: We can't use flags.WithFlags() since it's in an internal package,
	// so we set the Executor fields directly after creation.
	e := task.NewExecutor(
		task.WithVersionCheck(true),
	)

	// Apply our parsed flags to the executor
	// These field names match the Executor struct in executor.go
	e.Dir = dir
	e.Entrypoint = taskFile
	e.Force = taskForce
	e.ForceAll = taskForceAll
	e.Insecure = taskInsecure
	e.Download = taskDownload
	e.Offline = taskOffline
	e.Timeout = taskTimeout
	e.Watch = taskWatch
	e.Verbose = taskVerbose
	e.Silent = taskSilent
	e.AssumeYes = taskYes
	e.Dry = taskDry
	e.Summary = taskSummary
	e.Parallel = taskParallel
	e.Color = taskColor
	e.Concurrency = taskConcurrency
	e.Interval = taskInterval

	// Handle --output style
	if taskOutput != "" {
		switch strings.ToLower(taskOutput) {
		case "interleaved":
			e.OutputStyle = ast.Output{Name: "interleaved"}
		case "group":
			e.OutputStyle = ast.Output{Name: "group"}
		case "prefixed":
			e.OutputStyle = ast.Output{Name: "prefixed"}
		}
	}

	// Setup the executor (loads Taskfile, validates, etc.)
	if err := e.Setup(); err != nil {
		return err
	}

	// Handle --clear-cache
	if taskClearCache {
		cachePath := filepath.Join(e.TempDir.Remote, "remote")
		return os.RemoveAll(cachePath)
	}

	// Handle --list, --list-all, --json
	listOptions := task.NewListOptions(taskList, taskListAll, taskListJson, false)
	if listOptions.ShouldListTasks() {
		if taskSilent {
			return e.ListTaskNames(taskListAll)
		}
		foundTasks, err := e.ListTasks(listOptions)
		if err != nil {
			return err
		}
		if !foundTasks {
			os.Exit(errors.CodeUnknown)
		}
		return nil
	}

	// Parse remaining arguments into task calls
	// args.Parse handles "task:name VAR=value" syntax
	// Note: CLI_ARGS (everything after "--") was already extracted at the top
	// of this function BEFORE pflag.Parse() consumed the "--" separator.
	calls, globals := args.Parse(remainingArgs...)

	// If no tasks specified, run "default" task
	if len(calls) == 0 {
		calls = append(calls, &task.Call{Task: "default"})
	}

	globals.Set("CLI_ARGS", ast.Var{Value: cliArgs})
	globals.Set("CLI_FORCE", ast.Var{Value: taskForce || taskForceAll})
	globals.Set("CLI_SILENT", ast.Var{Value: taskSilent})
	globals.Set("CLI_VERBOSE", ast.Var{Value: taskVerbose})
	globals.Set("CLI_OFFLINE", ast.Var{Value: taskOffline})
	e.Taskfile.Vars.Merge(globals, nil)

	// Setup signal handling for graceful shutdown (Ctrl+C)
	// Don't intercept in watch mode - watch has its own signal handling
	if !taskWatch {
		e.InterceptInterruptSignals()
	}

	ctx := context.Background()

	// Handle --status (check if tasks are up-to-date)
	if taskStatus {
		return e.Status(ctx, calls...)
	}

	// Run the tasks
	return e.Run(ctx, calls...)
}
