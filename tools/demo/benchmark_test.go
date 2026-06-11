package demo

import (
	"os"
	"testing"

	lua "github.com/akzj/go-lua/pkg/lua"
)

// BenchmarkRealistic profiles the realistic Lua workload.
// It runs the full realistic.lua script (config, strings, OOP, coroutines, closures)
// under Go's benchmark framework for CPU profiling.
func BenchmarkRealistic(b *testing.B) {
	code, err := os.ReadFile("realistic.lua")
	if err != nil {
		b.Fatalf("reading realistic.lua: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L := lua.NewState()
		err := L.DoString(string(code))
		if err != nil {
			b.Fatalf("executing realistic.lua: %v", err)
		}
		L.Close()
	}
}

// BenchmarkRealisticNoGC runs the same workload but disables the GC
// to isolate GC overhead from other costs.
func BenchmarkRealisticNoGC(b *testing.B) {
	code, err := os.ReadFile("realistic.lua")
	if err != nil {
		b.Fatalf("reading realistic.lua: %v", err)
	}
	// Prepend GC-disable preamble
	noGC := "collectgarbage(\"stop\")\n" + string(code)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L := lua.NewState()
		err := L.DoString(noGC)
		if err != nil {
			b.Fatalf("executing realistic.lua (no GC): %v", err)
		}
		L.Close()
	}
}
