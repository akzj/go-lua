package api

import (
	"strings"
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

func TestLineTracking(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		expect string // substring expected in error
	}{
		{"error_line1", `error("boom")`, "(dostring):1:"},
		{"mod_zero", `local x = 1 % 0`, "(dostring):1:"},
		{"nil_arith", `local x = nil + 1`, "(dostring):1:"},
		{"error_line4", "local a = 1\nlocal b = 2\nlocal c = 3\nerror('line4')", "(dostring):4:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := luaapi.NewState()
			OpenAll(L)
			err := L.DoString(tt.code)
			if tt.expect == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expect)
				}
				errStr := err.Error()
				if !strings.Contains(errStr, tt.expect) {
					t.Errorf("expected error containing %q, got: %s", tt.expect, errStr)
				}
				t.Logf("OK: %s", errStr)
			}
		})
	}
}

func TestLineTrackingPcall(t *testing.T) {
	L := luaapi.NewState()
	OpenAll(L)
	err := L.DoString(`
local ok, msg = pcall(function()
  error("inside pcall")
end)
assert(type(msg) == "string", "msg not string: " .. tostring(msg))
assert(msg:find(":3:"), "expected :3: in msg: " .. tostring(msg))
`)
	if err != nil {
		t.Fatalf("pcall line tracking failed: %v", err)
	}
}
