package stdlib

import (
	"fmt"
	"testing"
	luaapi "github.com/akzj/go-lua/internal/api"
)

func TestGlobalKeyword(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"global_const_star", `global <const> *; print("global const star OK")`},
		{"global_const_star_then_use", `global <const> *; local x = 42; assert(x == 42); print("OK")`},
		{"global_const_names", `global <const> print, assert; assert(true); print("global const names OK")`},
		{"global_decl_after_star", `global <const> *; global x; x = 42; assert(x == 42); print("global decl OK")`},
		{"global_function", `global <const> *; global function gfoo() return 99 end; assert(gfoo() == 99); print("global function OK")`},
		{"global_assign", `global <const> *; global y = 123; assert(y == 123); print("global assign OK")`},
		{"global_multi", `
			global <const> *
			global a, b
			a = 10
			b = 20
			assert(a + b == 30)
			print("global multi OK")
		`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := luaapi.NewState()
			OpenAll(L)
			err := L.DoString(tt.code)
			if err != nil {
				fmt.Printf("  %s: FAIL — %v\n", tt.name, err)
				t.Fatalf("failed: %v", err)
			}
		})
	}
}
