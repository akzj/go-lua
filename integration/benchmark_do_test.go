package integration

import (
	"testing"

	"github.com/akzj/go-lua/state"
)

// =============================================================================
// DoString Benchmarks
// =============================================================================

func BenchmarkDoStringSimple(b *testing.B) {
	for i := 0; i < b.N; i++ {
		state.DoString("return 1 + 1")
	}
}

func BenchmarkDoStringArithmetic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		state.DoString("return 1 + 2 * 3 - 4 / 5")
	}
}

func BenchmarkDoStringLoop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		state.DoString("for i = 1, 100 do end")
	}
}

func BenchmarkDoStringTable(b *testing.B) {
	for i := 0; i < b.N; i++ {
		state.DoString("local t = {a = 1, b = 2}; return t.a + t.b")
	}
}

func BenchmarkDoStringFunction(b *testing.B) {
	for i := 0; i < b.N; i++ {
		state.DoString("function f(x) return x * 2 end; return f(5)")
	}
}

func BenchmarkDoStringClosure(b *testing.B) {
	for i := 0; i < b.N; i++ {
		state.DoString("local f = function(x) return x + 1 end; return f(10)")
	}
}

// =============================================================================
// State Creation Benchmarks
// =============================================================================

func BenchmarkNewState(b *testing.B) {
	for i := 0; i < b.N; i++ {
		state.New()
	}
}

// =============================================================================
// Stack Operations Benchmarks (LuaStateInterface: PushValue, Pop, Top, SetTop)
// =============================================================================

func BenchmarkStackTop(b *testing.B) {
	L := state.New()
	for i := 0; i < b.N; i++ {
		_ = L.Top()
	}
}

func BenchmarkStackPushPop(b *testing.B) {
	L := state.New()
	L.SetTop(10)
	for i := 0; i < b.N; i++ {
		L.PushValue(1)
		L.Pop()
	}
}

func BenchmarkStackSetTopResize(b *testing.B) {
	L := state.New()
	for i := 0; i < b.N; i++ {
		L.SetTop(10)
		L.SetTop(0)
	}
}
