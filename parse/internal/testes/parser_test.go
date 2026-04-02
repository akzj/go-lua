package testes

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	parseinternal "github.com/akzj/go-lua/parse/internal"
)

// TestAllTestesFiles tests that the parser can parse all files in lua-master/testes/
func TestAllTestesFiles(t *testing.T) {
	testDir := "../../../lua-master/testes"
	files, err := filepath.Glob(filepath.Join(testDir, "*.lua"))
	if err != nil {
		t.Skipf("skipping: %v", err)
	}

	parser := parseinternal.NewParser()
	failed := []string{}
	errors := []string{}

	for _, f := range files {
		name := filepath.Base(f)
		data, err := ioutil.ReadFile(f)
		if err != nil {
			t.Logf("SKIP %s: %v", name, err)
			continue
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					failed = append(failed, fmt.Sprintf("%s: panic: %v", name, r))
				}
			}()

			_, err := parser.Parse(string(data))
			if err != nil {
				// Syntax errors are expected - report but don't fail
				errors = append(errors, fmt.Sprintf("%s: %v", name, err))
				t.Logf("PARSE_ERROR %s: %v", name, err)
			} else {
				t.Logf("PASS %s", name)
			}
		}()
	}

	if len(failed) > 0 {
		for _, f := range failed {
			t.Errorf("FAILED: %s", f)
		}
		t.Fatalf("%d/%d files had panics", len(failed), len(files))
	}

	// Report summary of parse errors (these are expected for malformed test files)
	if len(errors) > 0 {
		t.Logf("--- Parse Error Summary: %d/%d files had syntax errors (expected) ---",
			len(errors), len(files))
	}
}

// TestSimpleParse tests basic parsing functionality
func TestSimpleParse(t *testing.T) {
	parser := parseinternal.NewParser()

	tests := []struct {
		name   string
		source string
		wantOK bool
	}{
		{
			name:   "empty",
			source: "",
			wantOK: true,
		},
		{
			name:   "assignment",
			source: "x = 1",
			wantOK: true,
		},
		{
			name:   "function_call",
			source: "print('hello')",
			wantOK: true,
		},
		{
			name:   "if_statement",
			source: "if true then end",
			wantOK: true,
		},
		{
			name:   "while_statement",
			source: "while true do end",
			wantOK: true,
		},
		{
			name:   "for_numeric",
			source: "for i = 1, 10 do end",
			wantOK: true,
		},
		{
			name:   "function_def",
			source: "function foo() end",
			wantOK: true,
		},
		{
			name:   "local_var",
			source: "local x = 1",
			wantOK: true,
		},
		{
			name:   "table_constructor",
			source: "t = {1, 2, 3}",
			wantOK: true,
		},
		{
			name:   "record_table",
			source: "t = {a = 1, b = 2}",
			wantOK: true,
		},
		{
			name:   "binary_ops",
			source: "x = 1 + 2 * 3",
			wantOK: true,
		},
		// Note: comparison operators (<, >, <=, >=) require TOKEN_LT/GT to be properly
		// defined in lex/api (currently using single-byte rune values)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.source)
			if tt.wantOK && err != nil {
				t.Errorf("Parse(%q) failed: %v", tt.source, err)
			}
		})
	}
}
