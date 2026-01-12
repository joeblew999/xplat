package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/a8m/envsubst"
	"github.com/spf13/cobra"
)

var (
	envsubstNoUnset bool
	envsubstNoEmpty bool
	envsubstEnvFile string
	envsubstOutput  string
	envsubstVars    []string
)

// EnvsubstCmd substitutes environment variables in text
var EnvsubstCmd = &cobra.Command{
	Use:   "envsubst [file]",
	Short: "Substitute environment variables in text",
	Long: `Substitute environment variables in text.

Reads from stdin or a file and replaces ${VAR} and $VAR references
with their values from the environment.

Works identically on macOS, Linux, and Windows.

Extended syntax supported:
  ${VAR}           - substitute, empty if unset
  ${VAR:-default}  - use default if unset or empty
  ${VAR:=default}  - set and use default if unset or empty
  ${VAR:+alt}      - use alt if VAR is set and non-empty
  ${VAR:?error}    - error if unset or empty
  ${#VAR}          - length of value
  ${VAR:offset}    - substring from offset
  ${VAR:offset:length} - substring

Examples:
  # From stdin
  echo 'Hello $USER' | xplat os envsubst

  # From file
  xplat os envsubst config.template > config.yaml

  # With env file (like .env)
  xplat os envsubst --env-file .env config.template

  # Strict mode (fail on unset variables)
  xplat os envsubst --no-unset config.template

  # Only substitute specific variables
  xplat os envsubst -v HOME -v USER template.txt`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEnvsubst,
}

func init() {
	EnvsubstCmd.Flags().BoolVar(&envsubstNoUnset, "no-unset", false, "Fail if a variable is not set")
	EnvsubstCmd.Flags().BoolVar(&envsubstNoEmpty, "no-empty", false, "Fail if a variable is empty")
	EnvsubstCmd.Flags().StringVar(&envsubstEnvFile, "env-file", "", "Load environment variables from file")
	EnvsubstCmd.Flags().StringVarP(&envsubstOutput, "output", "o", "", "Write output to file instead of stdout")
	EnvsubstCmd.Flags().StringArrayVarP(&envsubstVars, "var", "v", nil, "Only substitute these variables (can be repeated)")
}

func runEnvsubst(cmd *cobra.Command, args []string) error {
	// Load env file if specified
	if envsubstEnvFile != "" {
		if err := loadEnvFile(envsubstEnvFile); err != nil {
			return fmt.Errorf("failed to load env file: %w", err)
		}
	}

	// Determine input source
	var input io.Reader
	if len(args) == 0 {
		input = os.Stdin
	} else {
		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer f.Close()
		input = f
	}

	// Read all input
	content, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	// Build restrictions if specific vars are requested
	var restrictions []string
	if len(envsubstVars) > 0 {
		restrictions = envsubstVars
	}

	// Perform substitution
	result, err := substituteEnv(string(content), restrictions)
	if err != nil {
		return err
	}

	// Determine output destination
	var output io.Writer
	if envsubstOutput != "" {
		f, err := os.Create(envsubstOutput)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		output = f
	} else {
		output = os.Stdout
	}

	_, err = output.Write([]byte(result))
	return err
}

func substituteEnv(content string, restrictions []string) (string, error) {
	// If specific variables are requested, we need to handle this differently
	// by temporarily clearing other env vars or using a custom approach
	if len(restrictions) > 0 {
		return substituteRestricted(content, restrictions)
	}

	result, err := envsubst.StringRestricted(content, envsubstNoUnset, envsubstNoEmpty)
	if err != nil {
		return "", fmt.Errorf("substitution failed: %w", err)
	}

	return result, nil
}

// substituteRestricted only substitutes the specified variables
func substituteRestricted(content string, vars []string) (string, error) {
	// Build a map of allowed vars
	allowed := make(map[string]bool)
	for _, v := range vars {
		allowed[v] = true
	}

	// Get current environment
	env := make(map[string]string)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	// Clear all env vars
	os.Clearenv()

	// Set only allowed vars
	for _, v := range vars {
		if val, ok := env[v]; ok {
			os.Setenv(v, val)
		}
	}

	// Perform substitution
	result, err := envsubst.StringRestricted(content, envsubstNoUnset, envsubstNoEmpty)

	// Restore all env vars
	for k, v := range env {
		os.Setenv(k, v)
	}

	if err != nil {
		return "", fmt.Errorf("substitution failed: %w", err)
	}

	return result, nil
}

// loadEnvFile loads environment variables from a file in KEY=VALUE format
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		os.Setenv(key, value)
	}

	return scanner.Err()
}
