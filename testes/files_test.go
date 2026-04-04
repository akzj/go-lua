package testes

import (
	"testing"
)

// TestLuaMasterFiles tests the files.lua test suite.
// files.lua tests Lua's file I/O operations including:
// - io.open, io.input, io.output
// - File reading (read modes: "l", "L", "n", "a", number)
// - File writing
// - io.lines iterator
// - os.tmpname, os.remove, os.rename
// - loadfile, dofile with files
// - File buffering (setvbuf)
// - io.popen, io.tmpfile
// - io.type and file handle validation
func TestLuaMasterFiles(t *testing.T) {
	// files.lua is already covered by TestRunner (shared execution engine)
	// This explicit test verifies files.lua specifically passes
	runner := NewRunner("../lua-master/testes")
	
	result := runner.RunFile("../lua-master/testes/files.lua")
	
	if !result.Passed {
		t.Errorf("files.lua failed: %s", result.Error)
	}
	
	// Verify the result name
	if result.Name != "files.lua" {
		t.Errorf("expected result name 'files.lua', got %q", result.Name)
	}
}
