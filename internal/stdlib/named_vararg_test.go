package stdlib

import (
	"testing"
	luaapi "github.com/akzj/go-lua/internal/api"
)

func TestNamedVararg(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"basic", `
			local function f(a, ...t)
				assert(t.n == 2, "t.n should be 2, got " .. tostring(t.n))
				assert(t[1] == 20, "t[1] should be 20")
				assert(t[2] == 30, "t[2] should be 30")
				assert(a == 10, "a should be 10")
			end
			f(10, 20, 30)
		`},
		{"empty_varargs", `
			local function g(...t)
				assert(t.n == 0, "t.n should be 0, got " .. tostring(t.n))
			end
			g()
		`},
		{"single_vararg", `
			local function h(x, ...t)
				assert(t.n == 1)
				assert(t[1] == "hello")
			end
			h(42, "hello")
		`},
		{"vararg_with_select", `
			local function f(a, ...t)
				local x = {n = select('#', ...), ...}
				assert(x.n == t.n, "x.n=" .. tostring(x.n) .. " t.n=" .. tostring(t.n))
				for i = 1, x.n do
					assert(x[i] == t[i], "mismatch at " .. i)
				end
			end
			f({10, 20, 30}, 10, 20, 30)
		`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L := luaapi.NewState()
			OpenAll(L)
			if err := L.DoString(tt.code); err != nil {
				t.Fatalf("FAIL: %v", err)
			}
		})
	}
}
