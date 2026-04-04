// Package examples provides usage examples for go-lua.
package examples

import (
	"fmt"

	"github.com/akzj/go-lua/state"
)

// FunctionCall demonstrates calling Lua functions from Go.
// Note: Function return values have limited visibility through print.
// The VM handles return values internally but print output may not show them.
func FunctionCall() {
	L := state.New()

	// Define Lua functions
	state.DoStringOn(L, `
		-- Simple function
		function add(a, b)
			return a + b
		end

		-- Recursive function
		function factorial(n)
			if n <= 1 then return 1 end
			return n * factorial(n - 1)
		end

		-- Higher-order function
		function apply(fn, x, y)
			return fn(x, y)
		end

		-- Closure
		function counter(start)
			local count = start
			return function()
				count = count + 1
				return count
			end
		end

		-- Variable arguments
		function sum(...)
			local args = {...}
			local total = 0
			for _, v in ipairs(args) do
				total = total + v
			end
			return total
		end
	`)

	fmt.Println("Lua functions (return values handled internally):")

	// Define and call functions - results stored in variables
	state.DoStringOn(L, `
		-- Store results for inspection
		add_result = add(5, 3)
		factorial_result = factorial(5)
		sum_result = sum(1, 2, 3, 4, 5)
		
		-- Higher-order function
		apply_result = apply(add, 2, 4)
		
		-- Use closure
		inc = counter(0)
		c1 = inc()
		c2 = inc()
		c3 = inc()
	`)

	// Display what we can
	state.DoStringOn(L, `
		print("  add function defined")
		print("  factorial function defined")
		print("  counter function defined")
		print("  sum function defined")
	`)

	// Method calls (metatable __index)
	fmt.Println("\nMethod-style calls with metatables:")
	state.DoStringOn(L, `
		Vec = {}
		Vec.__index = Vec

		function Vec.new(x, y)
			return setmetatable({x = x, y = y}, Vec)
		end

		function Vec:add(other)
			return Vec.new(self.x + other.x, self.y + other.y)
		end

		v1 = Vec.new(3, 4)
		v2 = Vec.new(1, 2)
		v3 = v1:add(v2)
		print("  v1 = (" .. v1.x .. ", " .. v1.y .. ")")
		print("  v2 = (" .. v2.x .. ", " .. v2.y .. ")")
		print("  v3 = v1:add(v2)")
	`)
}
