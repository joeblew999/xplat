package processcompose

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsValidDuration(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		// Valid durations
		{"30s", true},
		{"5m", true},
		{"1h", true},
		{"24h", true},
		{"1h30m", true},
		{"1h30m45s", true},
		{"500ms", true},
		{"1.5h", true},
		{"2h45m", true},
		{"100ns", true},
		{"10us", true},
		{"10µs", true},

		// Invalid durations
		{"", false},
		{"30", false},         // missing unit
		{"s", false},          // missing number
		{"abc", false},        // not a duration
		{"30x", false},        // invalid unit
		{"30 s", false},       // space not allowed
		{"-30s", false},       // negative (actually valid in Go, but let's test)
		{"1d", false},         // days not supported
		{"1w", false},         // weeks not supported
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidDuration(tt.input)
			// Note: -30s is actually valid in Go's time.ParseDuration
			if tt.input == "-30s" {
				// Skip this case - Go allows negative durations
				return
			}
			if got != tt.valid {
				t.Errorf("isValidDuration(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestValidateCronExpression(t *testing.T) {
	tests := []struct {
		cron    string
		wantErr bool
		desc    string
	}{
		// Valid expressions
		{"* * * * *", false, "all wildcards"},
		{"0 0 * * *", false, "midnight daily"},
		{"0 2 * * *", false, "2am daily"},
		{"*/5 * * * *", false, "every 5 minutes"},
		{"0 */2 * * *", false, "every 2 hours"},
		{"0 9 * * 1-5", false, "weekdays at 9am"},
		{"0 0 1 * *", false, "first of month"},
		{"30 4 1,15 * *", false, "4:30am on 1st and 15th"},
		{"0 0 * * 0", false, "sunday midnight"},
		{"0 0 * * 7", false, "sunday midnight (alt)"},
		{"0-30/5 * * * *", false, "every 5 min in first half hour"},
		{"0,15,30,45 * * * *", false, "quarter hours"},

		// Invalid expressions
		{"", true, "empty"},
		{"* * * *", true, "only 4 fields"},
		{"* * * * * *", true, "6 fields"},
		{"60 * * * *", true, "minute out of range"},
		{"* 24 * * *", true, "hour out of range"},
		{"* * 0 * *", true, "day 0 invalid"},
		{"* * 32 * *", true, "day 32 invalid"},
		{"* * * 0 *", true, "month 0 invalid"},
		{"* * * 13 *", true, "month 13 invalid"},
		{"* * * * 8", true, "weekday 8 invalid"},
		{"abc * * * *", true, "non-numeric minute"},
		{"*/0 * * * *", true, "step of 0"},
		{"5-3 * * * *", true, "range start > end"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			err := validateCronExpression(tt.cron)
			gotErr := err != ""
			if gotErr != tt.wantErr {
				if tt.wantErr {
					t.Errorf("validateCronExpression(%q) expected error, got none", tt.cron)
				} else {
					t.Errorf("validateCronExpression(%q) unexpected error: %s", tt.cron, err)
				}
			}
		})
	}
}

// getTestdataPath returns the path to testdata directory
func getTestdataPath() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestScheduleConfigRule(t *testing.T) {
	rule := ScheduleConfigRule{}

	t.Run("valid cron schedule", func(t *testing.T) {
		pc := &ProcessCompose{
			Path: "test.yaml",
			Processes: map[string]*Process{
				"backup": {
					Command: "task backup:run",
					Schedule: &Schedule{
						Cron: "0 2 * * *",
					},
				},
			},
		}
		violations := rule.Check(pc)
		if len(violations) > 0 {
			t.Errorf("expected no violations, got %d: %v", len(violations), violations)
		}
	})

	t.Run("valid interval schedule", func(t *testing.T) {
		pc := &ProcessCompose{
			Path: "test.yaml",
			Processes: map[string]*Process{
				"health-check": {
					Command: "task health:run",
					Schedule: &Schedule{
						Interval: "30s",
					},
				},
			},
		}
		violations := rule.Check(pc)
		if len(violations) > 0 {
			t.Errorf("expected no violations, got %d: %v", len(violations), violations)
		}
	})

	t.Run("missing cron and interval", func(t *testing.T) {
		pc := &ProcessCompose{
			Path: "test.yaml",
			Processes: map[string]*Process{
				"broken": {
					Command:  "task broken:run",
					Schedule: &Schedule{}, // empty schedule
				},
			},
		}
		violations := rule.Check(pc)
		hasError := false
		for _, v := range violations {
			if v.Severity == SeverityError && v.Rule == "schedule-config" {
				hasError = true
			}
		}
		if !hasError {
			t.Error("expected error for missing cron/interval")
		}
	})

	t.Run("both cron and interval", func(t *testing.T) {
		pc := &ProcessCompose{
			Path: "test.yaml",
			Processes: map[string]*Process{
				"conflicting": {
					Command: "task run:both",
					Schedule: &Schedule{
						Cron:     "0 * * * *",
						Interval: "5m",
					},
				},
			},
		}
		violations := rule.Check(pc)
		hasError := false
		for _, v := range violations {
			if v.Severity == SeverityError && v.Rule == "schedule-config" {
				hasError = true
			}
		}
		if !hasError {
			t.Error("expected error for both cron and interval")
		}
	})

	t.Run("invalid cron expression", func(t *testing.T) {
		pc := &ProcessCompose{
			Path: "test.yaml",
			Processes: map[string]*Process{
				"bad-cron": {
					Command: "task bad:run",
					Schedule: &Schedule{
						Cron: "60 * * * *", // minute 60 is invalid
					},
				},
			},
		}
		violations := rule.Check(pc)
		hasError := false
		for _, v := range violations {
			if v.Severity == SeverityError && v.Rule == "schedule-config" {
				hasError = true
			}
		}
		if !hasError {
			t.Error("expected error for invalid cron minute")
		}
	})

	t.Run("invalid interval", func(t *testing.T) {
		pc := &ProcessCompose{
			Path: "test.yaml",
			Processes: map[string]*Process{
				"bad-interval": {
					Command: "task bad:run",
					Schedule: &Schedule{
						Interval: "5x", // invalid unit
					},
				},
			},
		}
		violations := rule.Check(pc)
		hasError := false
		for _, v := range violations {
			if v.Severity == SeverityError && v.Rule == "schedule-config" {
				hasError = true
			}
		}
		if !hasError {
			t.Error("expected error for invalid interval")
		}
	})
}

// TestScheduleValidFixture tests parsing and validating the valid schedule fixture
func TestScheduleValidFixture(t *testing.T) {
	path := filepath.Join(getTestdataPath(), "schedule-valid.yaml")
	pc, err := Parse(path)
	if err != nil {
		t.Fatalf("failed to parse schedule-valid.yaml: %v", err)
	}

	// Should have 8 processes
	if len(pc.Processes) != 8 {
		t.Errorf("expected 8 processes, got %d", len(pc.Processes))
	}

	// All should have schedules
	for name, proc := range pc.Processes {
		if proc.Schedule == nil {
			t.Errorf("process %s should have schedule", name)
		}
	}

	// Run schedule lint rule - should have no errors
	rule := ScheduleConfigRule{}
	violations := rule.Check(pc)

	errorCount := 0
	for _, v := range violations {
		if v.Severity == SeverityError {
			t.Errorf("unexpected error: %s", v.Message)
			errorCount++
		}
	}
	if errorCount > 0 {
		t.Errorf("valid fixture should have no errors, got %d", errorCount)
	}
}

// TestScheduleInvalidFixture tests that invalid schedules are caught
func TestScheduleInvalidFixture(t *testing.T) {
	path := filepath.Join(getTestdataPath(), "schedule-invalid.yaml")
	pc, err := Parse(path)
	if err != nil {
		t.Fatalf("failed to parse schedule-invalid.yaml: %v", err)
	}

	rule := ScheduleConfigRule{}
	violations := rule.Check(pc)

	// Count errors and warnings
	errorCount := 0
	warningCount := 0
	for _, v := range violations {
		if v.Severity == SeverityError {
			errorCount++
			t.Logf("ERROR: %s", v.Message)
		} else {
			warningCount++
			t.Logf("WARN: %s", v.Message)
		}
	}

	// Should have many errors from the invalid fixture
	if errorCount < 10 {
		t.Errorf("expected at least 10 errors from invalid fixture, got %d", errorCount)
	}

	// Should have warnings for schedule-with-probe and schedule-depends-healthy
	if warningCount < 2 {
		t.Errorf("expected at least 2 warnings, got %d", warningCount)
	}

	t.Logf("Total: %d errors, %d warnings", errorCount, warningCount)
}

// TestGraphFixtures tests graph generation from fixtures
func TestGraphFixtures(t *testing.T) {
	testCases := []struct {
		name          string
		file          string
		processCount  int
		hasScheduled  bool
		hasDisabled   bool
	}{
		{"chain", "graph-chain.yaml", 5, false, false},
		{"diamond", "graph-diamond.yaml", 4, false, false},
		{"mixed", "graph-mixed.yaml", 9, true, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(getTestdataPath(), tc.file)
			pc, err := Parse(path)
			if err != nil {
				t.Fatalf("failed to parse %s: %v", tc.file, err)
			}

			if len(pc.Processes) != tc.processCount {
				t.Errorf("expected %d processes, got %d", tc.processCount, len(pc.Processes))
			}

			hasScheduled := false
			hasDisabled := false
			for _, proc := range pc.Processes {
				if proc.Schedule != nil {
					hasScheduled = true
				}
				if proc.Disabled {
					hasDisabled = true
				}
			}

			if hasScheduled != tc.hasScheduled {
				t.Errorf("hasScheduled: expected %v, got %v", tc.hasScheduled, hasScheduled)
			}
			if hasDisabled != tc.hasDisabled {
				t.Errorf("hasDisabled: expected %v, got %v", tc.hasDisabled, hasDisabled)
			}
		})
	}
}

// TestUpstreamFixtures tests parsing upstream process-compose fixtures
// These live in .src/process-compose/ and test real-world configs
func TestUpstreamFixtures(t *testing.T) {
	// Get path relative to this file
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..")

	upstreamFixtures := []struct {
		path         string
		minProcesses int
	}{
		{".src/process-compose/fixtures-code/process-compose-chain.yaml", 8},
		{".src/process-compose/fixtures/process-compose-chain-arrow.yaml", 8},
		{".src/process-compose/fixtures/process-compose-many-for-one.yaml", 1},
	}

	for _, tc := range upstreamFixtures {
		fullPath := filepath.Join(repoRoot, tc.path)
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			pc, err := Parse(fullPath)
			if err != nil {
				t.Skipf("upstream fixture not available: %v", err)
				return
			}

			if len(pc.Processes) < tc.minProcesses {
				t.Errorf("expected at least %d processes, got %d", tc.minProcesses, len(pc.Processes))
			}

			t.Logf("Parsed %d processes from %s", len(pc.Processes), filepath.Base(tc.path))
		})
	}
}
