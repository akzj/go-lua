// Package examples provides usage examples for go-lua.
package examples

import (
	"fmt"

	"github.com/akzj/go-lua/state"
)

// Coroutine demonstrates coroutine usage in go-lua.
func Coroutine() {
	L := state.New()

	// Note: coroutine.yield/resume has limited implementation.
	// These examples show the Lua-side patterns and syntax.

	fmt.Println("Coroutine patterns:")

	// Create and use coroutines
	state.DoStringOn(L, `
		-- Create a coroutine
		function gen()
			for i = 1, 5 do
				coroutine.yield(i)
			end
		end

		co = coroutine.create(gen)
		
		-- Resume multiple times
		print("  Coroutine created, resuming...")
		for i = 1, 6 do
			local ok = coroutine.resume(co)
			if not ok then
				print("    Run " .. i .. ": finished")
				break
			end
			print("    Run " .. i .. ": resumed")
		end
	`)

	// Another pattern: passing values
	fmt.Println("\nCoroutine with value passing:")
	state.DoStringOn(L, `
		function adder()
			local sum = 0
			while true do
				local delta = coroutine.yield(sum)
				sum = sum + delta
			end
		end

		co2 = coroutine.create(adder)
		print("  Adder coroutine created")
		coroutine.resume(co2)
		coroutine.resume(co2, 10)
		coroutine.resume(co2, 5)
		coroutine.resume(co2, 20)
	`)

	// Producer-Consumer pattern
	fmt.Println("\nProducer-Consumer pattern:")
	state.DoStringOn(L, `
		function producer(n)
			for i = 1, n do
				coroutine.yield(i)
			end
		end

		function consumer(prod)
			local sum = 0
			local count = 0
			while true do
				local ok = coroutine.resume(prod)
				if not ok then break end
				count = count + 1
			end
			return count
		end

		prod = coroutine.create(function() producer(5) end)
		local count = consumer(prod)
		print("  Consumed " .. count .. " values")
	`)

	// Paired coroutines
	fmt.Println("\nPaired coroutines:")
	state.DoStringOn(L, `
		function pump()
			for i = 1, 3 do
				coroutine.yield(i)
			end
		end

		function drain(pump_co)
			while true do
				local ok = coroutine.resume(pump_co)
				if not ok then
					print("  Pump exhausted")
					break
				end
			end
		end

		pump_co = coroutine.create(pump)
		print("  Starting pump...")
		drain(pump_co)
	`)
}
