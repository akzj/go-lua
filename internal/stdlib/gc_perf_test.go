package stdlib

import (
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api"
)

func BenchmarkGCCollectEmpty(b *testing.B) {
	// Measure GC collect after creating garbage (all dead)
	L := luaapi.NewState()
	OpenAll(L)
	if err := L.DoString(`for i = 1, 100000 do local t = {i, i*2, i*3} end`); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := L.DoString(`collectgarbage("collect")`); err != nil {
			b.Fatal(err)
		}
	}
	L.Close()
}

func BenchmarkGCCollectLive(b *testing.B) {
	// Measure GC collect with live objects (must traverse all)
	L := luaapi.NewState()
	OpenAll(L)
	if err := L.DoString(`
		_G.data = {}
		for i = 1, 50000 do _G.data[i] = {val=i} end
	`); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := L.DoString(`collectgarbage("collect")`); err != nil {
			b.Fatal(err)
		}
	}
	L.Close()
}

func BenchmarkTableCreation100K(b *testing.B) {
	for i := 0; i < b.N; i++ {
		L := luaapi.NewState()
		OpenAll(L)
		if err := L.DoString(`for i = 1, 100000 do local t = {i, i*2, i*3, tostring(i)} end`); err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}
