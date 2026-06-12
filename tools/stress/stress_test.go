package stress

import (
	"os"
	"testing"

	lua "github.com/akzj/go-lua/pkg/lua"
)

// BenchmarkServer profiles a long-lived Lua state handling many requests.
// The state is created ONCE and reused across all iterations — simulating a server process.
func BenchmarkServer(b *testing.B) {
	code, err := os.ReadFile("server.lua")
	if err != nil {
		b.Fatal(err)
	}

	L := lua.NewState()
	defer L.Close()

	// Enable generational GC
	if err := L.DoString("collectgarbage('generational')"); err != nil {
		b.Fatal(err)
	}

	// Load server code
	if err := L.DoString(string(code)); err != nil {
		b.Fatal(err)
	}

	// Warm up — let generational GC settle
	for i := 0; i < 100; i++ {
		L.GetGlobal("handle_request")
		L.PushInteger(int64(i))
		L.PushString("status=200")
		L.Call(2, 1)
		L.Pop(1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.GetGlobal("handle_request")
		L.PushInteger(int64(i))
		L.PushString("status=200")
		L.Call(2, 1)
		L.Pop(1)
	}
}

// BenchmarkServerIncremental is identical but uses incremental GC for comparison.
func BenchmarkServerIncremental(b *testing.B) {
	code, err := os.ReadFile("server.lua")
	if err != nil {
		b.Fatal(err)
	}

	L := lua.NewState()
	defer L.Close()

	// Explicitly use incremental GC (default)
	if err := L.DoString("collectgarbage('incremental')"); err != nil {
		b.Fatal(err)
	}

	// Load server code
	if err := L.DoString(string(code)); err != nil {
		b.Fatal(err)
	}

	// Warm up
	for i := 0; i < 100; i++ {
		L.GetGlobal("handle_request")
		L.PushInteger(int64(i))
		L.PushString("status=200")
		L.Call(2, 1)
		L.Pop(1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.GetGlobal("handle_request")
		L.PushInteger(int64(i))
		L.PushString("status=200")
		L.Call(2, 1)
		L.Pop(1)
	}
}

// BenchmarkServerNoGC disables GC entirely to isolate GC overhead.
func BenchmarkServerNoGC(b *testing.B) {
	code, err := os.ReadFile("server.lua")
	if err != nil {
		b.Fatal(err)
	}

	L := lua.NewState()
	defer L.Close()

	// Disable GC entirely
	if err := L.DoString("collectgarbage('stop')"); err != nil {
		b.Fatal(err)
	}

	// Load server code
	if err := L.DoString(string(code)); err != nil {
		b.Fatal(err)
	}

	// Warm up
	for i := 0; i < 100; i++ {
		L.GetGlobal("handle_request")
		L.PushInteger(int64(i))
		L.PushString("status=200")
		L.Call(2, 1)
		L.Pop(1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.GetGlobal("handle_request")
		L.PushInteger(int64(i))
		L.PushString("status=200")
		L.Call(2, 1)
		L.Pop(1)
	}
}
