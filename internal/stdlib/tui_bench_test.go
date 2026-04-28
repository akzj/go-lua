package stdlib

import (
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api"
)

// TUI render benchmark: simulates a 60fps UI engine that creates 1000 element
// tables per frame, then discards them — matching the user's production workload.

const tuiRenderScript = `
function render_frame(n)
    local frame = {}
    for i = 1, n do
        frame[i] = {
            type = "div",
            x = i,
            y = i * 2,
            width = 100,
            height = 50,
            text = "item" .. i,
            visible = true,
            children = {}
        }
    end
    return frame
end
`

const tuiRenderDeepScript = `
function render_deep(n)
    local frame = {}
    for i = 1, n do
        frame[i] = {
            type = "div", x = i, y = i * 2,
            children = {
                {type = "span", text = "a" .. i, children = {
                    {type = "text", value = "x"},
                    {type = "text", value = "y"}
                }},
                {type = "span", text = "b" .. i, children = {
                    {type = "text", value = "z"},
                    {type = "text", value = "w"}
                }},
                {type = "span", text = "c" .. i}
            }
        }
    end
    return frame
end
`

// BenchmarkTUIRender — flat frame: 1000 element tables per frame, discard each frame.
func BenchmarkTUIRender(b *testing.B) {
	b.ReportAllocs()
	L := luaapi.NewState()
	OpenAll(L)
	if err := L.DoString(tuiRenderScript); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.GetGlobal("render_frame")
		L.PushInteger(1000)
		if status := L.PCall(1, 1, 0); status != luaapi.StatusOK {
			msg, _ := L.ToString(-1)
			L.Pop(1)
			b.Fatalf("render_frame failed: %s", msg)
		}
		L.Pop(1) // discard frame table
	}
	b.StopTimer()
	L.Close()
}

// BenchmarkTUIRenderWithGC — same as TUIRender but triggers a full GC every 60 frames.
func BenchmarkTUIRenderWithGC(b *testing.B) {
	b.ReportAllocs()
	L := luaapi.NewState()
	OpenAll(L)
	if err := L.DoString(tuiRenderScript); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.GetGlobal("render_frame")
		L.PushInteger(1000)
		if status := L.PCall(1, 1, 0); status != luaapi.StatusOK {
			msg, _ := L.ToString(-1)
			L.Pop(1)
			b.Fatalf("render_frame failed: %s", msg)
		}
		L.Pop(1) // discard frame table

		// Every 60 frames, trigger a full GC (simulating ~1 second at 60fps)
		if (i+1)%60 == 0 {
			if err := L.DoString(`collectgarbage("collect")`); err != nil {
				b.Fatalf("collectgarbage failed: %s", err)
			}
		}
	}
	b.StopTimer()
	L.Close()
}

// BenchmarkTUIRenderDeep — nested element tree: each of 1000 elements has
// 3 children, two of which have 2 grandchildren (real UI tree structure).
func BenchmarkTUIRenderDeep(b *testing.B) {
	b.ReportAllocs()
	L := luaapi.NewState()
	OpenAll(L)
	if err := L.DoString(tuiRenderDeepScript); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L.GetGlobal("render_deep")
		L.PushInteger(1000)
		if status := L.PCall(1, 1, 0); status != luaapi.StatusOK {
			msg, _ := L.ToString(-1)
			L.Pop(1)
			b.Fatalf("render_deep failed: %s", msg)
		}
		L.Pop(1) // discard frame table
	}
	b.StopTimer()
	L.Close()
}
