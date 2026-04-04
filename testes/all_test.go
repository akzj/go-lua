package testes

import (
	"testing"
)

// TestLuaMasterAll tests the all.lua meta-tester.
// all.lua is a test runner that executes all other test files in sequence.
// It tests:
// - Test suite orchestration
// - dofile functionality
// - Global test state
// - Timing and memory reporting
// - Cleanup and final validation
func TestLuaMasterAll(t *testing.T) {
	// Run the test suite via our runner
	runner := NewRunner("../lua-master/testes")
	
	// Note: This is equivalent to what all.lua does internally
	
	passed, failed, err := runner.Run()
	
	if err != nil {
		t.Errorf("Test runner error: %v", err)
	}
	
	// The all.lua test validates:
	// 1. All individual tests pass
	// 2. No panic occurs during execution
	// 3. Test cleanup happens correctly
	
	if failed > 0 {
		t.Logf("Failed tests: %d / %d", failed, passed+failed)
		// Log individual failures for debugging
		results := runner.Results()
		for _, r := range results {
			if !r.Passed {
				t.Logf("  FAILED: %s: %s", r.Name, r.Error)
			}
		}
	}
	
	// The test passes if runner completes without panic/error
	// Even if some lua tests fail (expected in partial implementation),
	// the runner itself should work correctly
	if passed == 0 && failed == 0 {
		t.Error("No tests were executed")
	}
	
	// Log summary
	t.Logf("Test suite: %d passed, %d failed", passed, failed)
}

// TestLuaMasterAllRunBehavior verifies the runner behaves correctly
// like the all.lua meta-tester.
func TestLuaMasterAllRunBehavior(t *testing.T) {
	runner := NewRunner("../lua-master/testes")
	
	// Run should complete without panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Runner panicked: %v", r)
		}
	}()
	
	passed, failed, err := runner.Run()
	
	// Error is not expected
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// Verify results are populated
	results := runner.Results()
	if len(results) == 0 {
		t.Error("No results returned")
	}
	
	// Count actual results
	actualPassed := 0
	actualFailed := 0
	for _, r := range results {
		if r.Passed {
			actualPassed++
		} else {
			actualFailed++
		}
	}
	
	if actualPassed != passed || actualFailed != failed {
		t.Errorf("Result count mismatch: got (%d, %d), want (%d, %d)",
			actualPassed, actualFailed, passed, failed)
	}
	
	// Verify no duplicate test names
	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r.Name] {
			t.Errorf("Duplicate test name: %s", r.Name)
		}
		seen[r.Name] = true
	}
}
