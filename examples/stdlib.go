// Package examples provides usage examples for go-lua.
package examples

import (
	"fmt"

	"github.com/akzj/go-lua/state"
)

// Stdlib demonstrates using the standard library.
func Stdlib() {
	L := state.New()

	// Note: OpenLibs is not fully implemented, so we demonstrate
	// available functionality through DoString

	// Math library
	fmt.Println("Math library:")
	state.DoStringOn(L, `
		print("  math.pi = " .. math.pi)
		print("  math.sin(math.pi/2) = " .. math.sin(math.pi/2))
		print("  math.floor(3.7) = " .. math.floor(3.7))
		print("  math.ceil(3.2) = " .. math.ceil(3.2))
		print("  math.max(1, 5, 3) = " .. math.max(1, 5, 3))
		print("  math.min(1, 5, 3) = " .. math.min(1, 5, 3))
		print("  math.abs(-5) = " .. math.abs(-5))
		print("  math.sqrt(16) = " .. math.sqrt(16))
	`)

	// String library
	fmt.Println("\nString library:")
	state.DoStringOn(L, `
		s = "Hello, World!"
		print("  s: " .. s)
		print("  string.len(s): " .. string.len(s))
		print("  string.sub(s, 1, 5): " .. string.sub(s, 1, 5))
		print("  string.upper(s): " .. string.upper(s))
		print("  string.lower(s): " .. string.lower(s))
		print("  string.format('%d + %d = %d', 2, 3, 5): " .. string.format("%d + %d = %d", 2, 3, 5))
		print("  string.rep('ab', 3): " .. string.rep("ab", 3))
		print("  string.find('hello world', 'world'): " .. string.find("hello world", "world"))
	`)

	// Table library
	fmt.Println("\nTable library:")
	state.DoStringOn(L, `
		t = {3, 1, 4, 1, 5, 9, 2, 6}
		print("  Original: " .. table.concat(t, ", "))
		table.sort(t)
		print("  After sort: " .. table.concat(t, ", "))

		t2 = {"a", "b", "c"}
		table.insert(t2, "d")
		print("  After insert: " .. table.concat(t2, ", "))

		table.remove(t2, 2)
		print("  After remove(2): " .. table.concat(t2, ", "))

		-- Manual array access
		t3 = {1, 2, 3}
		print("  t3[1]=" .. t3[1] .. " t3[2]=" .. t3[2] .. " t3[3]=" .. t3[3])
	`)

	// Pairs and ipairs
	fmt.Println("\nIteration:")
	state.DoStringOn(L, `
		-- ipairs for sequential tables
		arr = {"one", "two", "three"}
		print("  ipairs:")
		for i, v in ipairs(arr) do
			print("    " .. i .. ": " .. v)
		end

		-- pairs for all keys
		t = {a = 1, b = 2, c = 3}
		print("  pairs:")
		for k, v in pairs(t) do
			print("    " .. k .. ": " .. v)
		end
	`)

	// Basic I/O (file operations)
	fmt.Println("\nFile operations:")
	state.DoStringOn(L, `
		-- Write file
		f = io.open("test_example.txt", "w")
		if f then
			f:write("Hello from go-lua!\n")
			f:write("Line 2\n")
			f:write("Line 3\n")
			f:close()
			print("  Wrote to test_example.txt")
		else
			print("  Failed to create file")
		end

		-- Read file
		f = io.open("test_example.txt", "r")
		if f then
			print("  Contents:")
			for line in f:lines() do
				print("    " .. line)
			end
			f:close()
		else
			print("  Failed to read file")
		end
	`)

	// OS library basics
	fmt.Println("\nOS library:")
	state.DoStringOn(L, `
		print("  os.clock(): " .. os.clock())
		print("  os.date(): " .. os.date())
		print("  os.time(): " .. os.time())
		print("  os.date('!%Y-%m-%d'): " .. os.date("!%Y-%m-%d"))
	`)

	// Coroutines
	fmt.Println("\nCoroutine library:")
	state.DoStringOn(L, `
		function demo()
			for i = 1, 3 do
				coroutine.yield(i * 10)
			end
		end

		co = coroutine.create(demo)
		print("  Resume results:")
		for i = 1, 4 do
			ok, val = coroutine.resume(co)
			if ok then
				print("    " .. tostring(val))
			else
				print("    finished")
			end
		end
	`)
}
