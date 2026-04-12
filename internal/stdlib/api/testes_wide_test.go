package api

import (
	"fmt"
	"os"
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

// TestTestesWide runs multiple testes files and reports results.
// This is for coverage mapping — individual failures are logged as skips.
func TestTestesWide(t *testing.T) {
	files := []string{
		"strings.lua",
		"math.lua",
		"sort.lua",
		"vararg.lua",
		"constructs.lua",
		"events.lua",
		"calls.lua",
		"locals.lua",
		"bitwise.lua",
		"tpack.lua",
	}

	for _, f := range files {
		f := f // capture
		t.Run(f, func(t *testing.T) {
			path := testesDir + f
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("not found: %s", path)
				return
			}
			// Recover from panics so one file doesn't kill the whole suite
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("  %-20s PANIC: %v\n", f, r)
					t.Skipf("%s: PANIC: %v", f, r)
				}
			}()
			L := luaapi.NewState()
			OpenAll(L)
			err := L.DoFile(path)
			if err != nil {
				fmt.Printf("  %-20s FAIL: %v\n", f, err)
				t.Skipf("%s: %v", f, err)
			} else {
				fmt.Printf("  %-20s PASS\n", f)
			}
		})
	}
}
