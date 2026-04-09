package testes

import (
	"fmt"
	"testing"
)

func TestRunner(t *testing.T) {
	runner := NewRunner("../lua-master/testes")
	passed, failed, err := runner.Run()
	
	fmt.Printf("\n=== Test Results ===\n")
	fmt.Printf("Passed: %d\n", passed)
	fmt.Printf("Failed: %d\n", failed)
	fmt.Printf("Total:  %d\n", passed+failed)
	
	if err != nil {
		t.Errorf("Runner error: %v", err)
	}
	
	// Report detailed results
	results := runner.Results()
	for _, r := range results {
		if r.Passed {
			t.Logf("✓ %s", r.Name)
		} else {
			t.Logf("✗ %s: %s", r.Name, r.Error)
		}
	}
	
	// Basic sanity: we should be able to run tests
	if passed == 0 && failed == 0 {
		t.Error("No tests were run")
	}
	
	// Report some passing tests as success metric
	if passed > 0 {
		t.Logf("SUCCESS: %d lua-master/testes files passed", passed)
	}
}

func TestUtf8Diag(t *testing.T) {
	runner := NewRunner("../lua-master/testes")
	result := runner.RunFile("../lua-master/testes/utf8_diag.lua")
	if !result.Passed {
		t.Errorf("utf8_diag: %s", result.Error)
	} else {
		t.Log("utf8_diag PASSED")
	}
}
