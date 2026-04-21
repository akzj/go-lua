package stdlib

import (
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api"
)

// BenchmarkFibonacci — recursive function calls + arithmetic
func BenchmarkFibonacci(b *testing.B) {
	for i := 0; i < b.N; i++ {
		L := luaapi.NewState()
		OpenAll(L)
		err := L.DoString(`
            local function fib(n)
                if n < 2 then return n end
                return fib(n-1) + fib(n-2)
            end
            fib(20)
        `)
		if err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}

// BenchmarkTableOps — table creation, read, write
func BenchmarkTableOps(b *testing.B) {
	for i := 0; i < b.N; i++ {
		L := luaapi.NewState()
		OpenAll(L)
		err := L.DoString(`
            local t = {}
            for i = 1, 10000 do
                t[i] = i * 2
            end
            local sum = 0
            for i = 1, 10000 do
                sum = sum + t[i]
            end
        `)
		if err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}

// BenchmarkStringConcat — string operations
func BenchmarkStringConcat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		L := luaapi.NewState()
		OpenAll(L)
		err := L.DoString(`
            local t = {}
            for i = 1, 1000 do
                t[i] = tostring(i)
            end
            local s = table.concat(t, ",")
        `)
		if err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}

// BenchmarkPatternMatch — string.find/gsub
func BenchmarkPatternMatch(b *testing.B) {
	for i := 0; i < b.N; i++ {
		L := luaapi.NewState()
		OpenAll(L)
		err := L.DoString(`
            local s = string.rep("hello world ", 100)
            for i = 1, 100 do
                string.find(s, "(%w+)%s(%w+)")
                string.gsub(s, "%w+", string.upper)
            end
        `)
		if err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}

// BenchmarkForLoop — tight numeric loop (VM dispatch speed)
func BenchmarkForLoop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		L := luaapi.NewState()
		OpenAll(L)
		err := L.DoString(`
            local sum = 0
            for i = 1, 1000000 do
                sum = sum + i
            end
        `)
		if err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}

// BenchmarkClosureCreation — closure/upvalue overhead
func BenchmarkClosureCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		L := luaapi.NewState()
		OpenAll(L)
		err := L.DoString(`
            local function make_counter()
                local n = 0
                return function()
                    n = n + 1
                    return n
                end
            end
            for i = 1, 10000 do
                local c = make_counter()
                c()
                c()
            end
        `)
		if err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}

// BenchmarkMethodCall — table method dispatch (OOP pattern)
func BenchmarkMethodCall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		L := luaapi.NewState()
		OpenAll(L)
		err := L.DoString(`
            local Point = {}
            Point.__index = Point
            function Point.new(x, y)
                return setmetatable({x=x, y=y}, Point)
            end
            function Point:dist()
                return (self.x^2 + self.y^2)^0.5
            end
            local p = Point.new(3, 4)
            for i = 1, 100000 do
                p:dist()
            end
        `)
		if err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}

// BenchmarkGC — allocation + GC pressure
func BenchmarkGC(b *testing.B) {
	for i := 0; i < b.N; i++ {
		L := luaapi.NewState()
		OpenAll(L)
		err := L.DoString(`
            for i = 1, 10000 do
                local t = {i, i*2, i*3}
            end
            collectgarbage("collect")
        `)
		if err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}
