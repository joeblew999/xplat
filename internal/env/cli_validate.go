package env

import (
	"fmt"

	"github.com/fatih/color"
)

// RunValidateFast performs fast validation (format checks only) and prints results
// Returns exit code: 0 if all valid, 1 if any invalid
func RunValidateFast() int {
	svc := NewService(false)
	cfg, err := svc.GetCurrentConfig()
	if err != nil {
		color.Red("Error loading configuration: %v", err)
		return 1
	}

	results := ValidateAllFast(cfg)

	// Print header
	fmt.Println()
	color.Cyan("=== Fast Validation (Format Checks Only) ===")
	fmt.Println()

	// Print table header
	printValidationTableHeader()

	// Print results
	hasErrors := false
	for _, result := range results {
		if result.Skipped {
			printValidationRow(result.Name, "SKIP", "-", color.FgHiBlack)
		} else if result.Valid {
			printValidationRow(result.Name, "✓ OK", "-", color.FgGreen)
		} else {
			printValidationRow(result.Name, "✗ FAIL", result.Error.Error(), color.FgRed)
			hasErrors = true
		}
	}

	fmt.Println()

	if hasErrors {
		color.Red("❌ Fast validation failed - some fields have invalid formats")
		color.Yellow("💡 Run 'validate-deep' to verify credentials with API calls")
		return 1
	}

	color.Green("✅ Fast validation passed - all field formats are correct")
	color.Yellow("💡 Run 'validate-deep' to verify credentials with API calls")
	return 0
}

// RunValidateDeep performs deep validation (includes API calls) and prints results
// Returns exit code: 0 if all valid, 1 if any invalid
func RunValidateDeep() int {
	svc := NewService(false)
	cfg, err := svc.GetCurrentConfig()
	if err != nil {
		color.Red("Error loading configuration: %v", err)
		return 1
	}

	results := ValidateAllDeep(cfg, false)

	// Print header
	fmt.Println()
	color.Cyan("=== Deep Validation (API Verification) ===")
	color.Yellow("⚠️  This will make API calls to verify credentials")
	fmt.Println()

	// Print table header
	printValidationTableHeader()

	// Print results
	hasErrors := false
	for _, result := range results {
		if result.Skipped {
			printValidationRow(result.Name, "SKIP", "-", color.FgHiBlack)
		} else if result.Valid {
			printValidationRow(result.Name, "✔️ VERIFIED", "-", color.FgGreen)
		} else {
			printValidationRow(result.Name, "✗ FAIL", result.Error.Error(), color.FgRed)
			hasErrors = true
		}
	}

	fmt.Println()

	if hasErrors {
		color.Red("❌ Deep validation failed - some credentials are invalid")
		color.Yellow("💡 Check the errors above and update your .env file")
		return 1
	}

	color.Green("✅ Deep validation passed - all credentials verified via API")
	return 0
}

// printValidationTableHeader prints the table header
func printValidationTableHeader() {
	headerColor := color.New(color.FgCyan, color.Bold)
	headerColor.Printf("%-35s %-15s %s\n", "Field", "Status", "Error")
	fmt.Println("------------------------------------------------------------------------------------")
}

// printValidationRow prints a single validation result row
func printValidationRow(field, status, errorMsg string, statusColor color.Attribute) {
	fieldColor := color.New(color.FgWhite)
	statusColorObj := color.New(statusColor)

	fieldColor.Printf("%-35s ", field)
	statusColorObj.Printf("%-15s ", status)

	if errorMsg != "-" {
		color.New(color.FgRed).Println(errorMsg)
	} else {
		fmt.Println(errorMsg)
	}
}

// RunValidateWithMode runs validation with specified mode and prints results
// This is a utility function for testing or advanced usage
func RunValidateWithMode(mode ValidationMode) int {
	switch mode {
	case ValidationModeFast:
		return RunValidateFast()
	case ValidationModeDeep:
		return RunValidateDeep()
	default:
		color.Red("Unknown validation mode: %s", mode)
		return 1
	}
}
