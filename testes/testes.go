// Package testes provides a test runner for lua-master/testes suite.
// It executes lua test files and reports pass/fail based on execution success.
package testes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akzj/go-lua/state"
)

// TestResult represents the result of running a single test file.
type TestResult struct {
	Name   string
	Passed bool
	Error  string
}

// Runner executes the lua-master/testes suite.
type Runner struct {
	testDir string
	results []TestResult
}

// NewRunner creates a new testes runner.
func NewRunner(testDir string) *Runner {
	return &Runner{testDir: testDir}
}

// Run executes all .lua test files in the test directory.
// Returns the number of passed and failed tests.
func (r *Runner) Run() (passed, failed int, err error) {
	// Find all .lua files
	pattern := filepath.Join(r.testDir, "*.lua")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return 0, 0, fmt.Errorf("glob failed: %w", err)
	}

	r.results = make([]TestResult, 0, len(files))

	for _, file := range files {
		// Skip all.lua (it's a runner that includes others)
		// Skip main.lua (it's for standalone interpreter testing)
		base := filepath.Base(file)
		if base == "all.lua" || base == "main.lua" || base == "memerr.lua" {
			continue
		}

		result := r.RunFile(file)
		r.results = append(r.results, result)

		if result.Passed {
			passed++
			fmt.Printf("✓ %s\n", base)
		} else {
			failed++
			fmt.Printf("✗ %s: %s\n", base, result.Error)
		}
	}

	return passed, failed, nil
}

// RunFile executes a single .lua test file.
// lua-master/testes format: tests use assert() statements.
// If all asserts pass, execution succeeds. If any assert fails, an error is raised.
// Returns the test result.
func (r *Runner) RunFile(path string) (result TestResult) {
	base := filepath.Base(path)

	// Recover from Go panics (e.g. bassert uses panic() since pcall is not yet implemented)
	defer func() {
		if r := recover(); r != nil {
			result = TestResult{
				Name:   base,
				Passed: false,
				Error:  fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	// Read the file
	code, err := os.ReadFile(path)
	if err != nil {
		return TestResult{
			Name:   base,
			Passed: false,
			Error:  fmt.Sprintf("read failed: %v", err),
		}
	}

	// Execute the code
	// lua-master/testes rely on assert() - if execution returns nil, all tests passed
	err = state.DoString(string(code))

	if err != nil {
		// Extract a clean error message
		errMsg := r.cleanError(err)
		return TestResult{
			Name:   base,
			Passed: false,
			Error:  errMsg,
		}
	}

	return TestResult{
		Name:   base,
		Passed: true,
		Error:  "",
	}
}

// cleanError extracts a clean error message from Lua execution errors.
func (r *Runner) cleanError(err error) string {
	msg := err.Error()
	// Remove file path prefixes from error messages
	if idx := strings.LastIndex(msg, ": "); idx != -1 {
		return msg[idx+2:]
	}
	return msg
}

// Results returns all test results.
func (r *Runner) Results() []TestResult {
	return r.results
}
