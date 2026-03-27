package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/akzj/go-lua/pkg/api"
)

// TestBisect preprocesses and runs a test file up to a given line.
// Usage: go test ./tests/ -run TestBisect -v -bisect-file=locals.lua -bisect-line=50
func TestBisect(t *testing.T) {
	file := os.Getenv("BISECT_FILE")
	maxLine := 0
	fmt.Sscanf(os.Getenv("BISECT_LINE"), "%d", &maxLine)
	
	if file == "" {
		t.Skip("Set BISECT_FILE env var")
	}
	
	config := DefaultConfig()
	path := config.TestDir + "/" + file
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	
	code := preprocessLua55(string(content))
	
	if maxLine > 0 {
		lines := strings.Split(code, "\n")
		if maxLine < len(lines) {
			lines = lines[:maxLine]
		}
		code = strings.Join(lines, "\n")
	}
	
	L := api.NewState()
	defer L.Close()
	L.OpenLibs()
	
	err = L.DoString(code, "@"+path)
	if err != nil {
		t.Fatalf("Error at/before line %d: %v", maxLine, err)
	}
	t.Logf("OK up to line %d", maxLine)
}

// TestPreprocess dumps preprocessed output for inspection
func TestPreprocess(t *testing.T) {
	file := os.Getenv("BISECT_FILE")
	if file == "" {
		t.Skip("Set BISECT_FILE env var")
	}
	
	config := DefaultConfig()
	path := config.TestDir + "/" + file
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	
	code := preprocessLua55(string(content))
	lines := strings.Split(code, "\n")
	
	startLine := 0
	endLine := 60
	fmt.Sscanf(os.Getenv("START_LINE"), "%d", &startLine)
	fmt.Sscanf(os.Getenv("END_LINE"), "%d", &endLine)
	
	for i := startLine; i < endLine && i < len(lines); i++ {
		t.Logf("%3d: %s", i+1, lines[i])
	}
}
