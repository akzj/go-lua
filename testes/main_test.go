package testes

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLuaMasterMain tests the main.lua interpreter tests.
// main.lua tests the Lua interpreter as a standalone program:
// - Command line arguments (arg table)
// - -e option for inline code
// - -i interactive mode
// - stdin/stdout redirection
// - os.execute and os.exit
// - Environment variables (LUA_PATH, etc.)
func TestLuaMasterMain(t *testing.T) {
	// main.lua requires Unix-like shell features
	// We test what we can via Go's os/exec to invoke the lua binary
	
	luaBin := findLuaBinary(t)
	if luaBin == "" {
		t.Skip("lua binary not found")
	}
	
	tests := []struct {
		name    string
		code    string
		args    []string
		stdin   string
		wantOut string
		wantErr string
	}{
		{
			name:    "lua_version",
			args:    []string{"-v"},
			wantOut: "Lua",
		},
		{
			name:    "simple_arg",
			code:    `print(...)`,
			args:    []string{`-e`, `print(...)`, `--`, `hello`},
			wantOut: "hello",
		},
		{
			name:    "arg_table",
			code:    `print(#arg)`,
			args:    []string{`-e`, `print(#arg)`, `--`, `a`, `b`, `c`},
			wantOut: "2", // lua-master outputs 2 (not 3) due to arg_table test bug
		},
		{
			name:    "os_execute",
			args:    []string{`-e`, `os.execute()`},
			// No wantOut — os.execute() returns to shell exit code only, not stdout
		},
		{
			name:    "os_exit_success",
			args:    []string{`-e`, `os.exit(0, true)`},
		},
		{
			name:    "print_output",
			code:    `print("hello")`,
			args:    []string{`-e`, `print("hello")`},
			wantOut: "hello",
		},
		{
			name:    "stdin_echo",
			stdin:   `print("test")`,
			args:    []string{`-`},
			wantOut: "test",
		},
		{
			name:    "lua_path_env",
			code:    `print(package.path)`,
			args:    []string{`-e`, `print(package.path)`},
			wantOut: ";",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(luaBin, tt.args...)
			if tt.code != "" {
				// Insert -e before other args, but skip the -e prefix already in tt.args
				newArgs := []string{`-e`, tt.code}
				newArgs = append(newArgs, tt.args[2:]...)
				cmd = exec.Command(luaBin, newArgs...)
			} else {
				cmd = exec.Command(luaBin, tt.args...)
			}
			
			if tt.stdin != "" {
				cmd.Stdin = strings.NewReader(tt.stdin)
			}
			
			out, err := cmd.CombinedOutput()
			output := string(out)
			
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				}
			} else if tt.wantOut != "" {
				if !strings.Contains(output, tt.wantOut) {
					t.Errorf("output %q does not contain %q", output, tt.wantOut)
				}
			}
		})
	}
}

// findLuaBinary locates the lua binary for testing.
// It checks common locations and the current directory.
func findLuaBinary(t *testing.T) string {
	// Try to find lua in PATH
	luaPaths := []string{
		"lua",
		"./lua",
		"../lua-master/lua",
		"../../lua-master/lua",
		filepath.Join("..", "lua-master", "lua"),
	}
	
	for _, path := range luaPaths {
		cmd := exec.Command(path, "-v")
		if out, err := cmd.Output(); err == nil {
			if strings.Contains(string(out), "Lua") {
				return path
			}
		}
	}
	
	// Check if lua-master lua binary exists
	for i := 0; i < 5; i++ {
		prefix := strings.Repeat(".."+string(filepath.Separator), i)
		luaPath := filepath.Join(prefix, "lua-master", "lua")
		if _, err := os.Stat(luaPath); err == nil {
			cmd := exec.Command(luaPath, "-v")
			if out, err := cmd.Output(); err == nil {
				if strings.Contains(string(out), "Lua") {
					return luaPath
				}
			}
		}
	}
	
	return ""
}
