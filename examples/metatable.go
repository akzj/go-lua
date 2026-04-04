// Package examples provides usage examples for go-lua.
package examples

import (
	"fmt"

	"github.com/akzj/go-lua/state"
)

// Metatable demonstrates metatable operations and metamethods.
func Metatable() {
	L := state.New()

	// __index metamethod - custom property access
	fmt.Println("Metatable __index:")
	state.DoStringOn(L, `
		-- Define a class-like table with __index
		Point = {}
		Point.__index = Point

		function Point.new(x, y)
			return setmetatable({x = x, y = y}, Point)
		end

		-- Add a computed property via __index
		setmetatable(Point, {
			__index = function(t, k)
				if k == "origin" then
					return t.new(0, 0)
				end
				return nil
			end
		})

		p = Point.new(3, 4)
		o = Point.origin  -- uses __index
		print("  p = (" .. p.x .. ", " .. p.y .. ")")
		print("  Point.origin = (" .. o.x .. ", " .. o.y .. ")")
	`)

	// __add metamethod - custom addition
	fmt.Println("\nMetatable __add:")
	state.DoStringOn(L, `
		Vector = {}
		Vector.__index = Vector

		function Vector.new(x, y)
			return setmetatable({x = x, y = y}, Vector)
		end

		Vector.__add = function(a, b)
			return Vector.new(a.x + b.x, a.y + b.y)
		end

		function Vector:__tostring()
			return "(" .. self.x .. ", " .. self.y .. ")"
		end

		v1 = Vector.new(1, 2)
		v2 = Vector.new(3, 4)
		v3 = v1 + v2  -- uses __add
		print("  v1 = " .. tostring(v1))
		print("  v2 = " .. tostring(v2))
		print("  v1 + v2 = " .. tostring(v3))
	`)

	// __len metamethod
	fmt.Println("\nMetatable __len:")
	state.DoStringOn(L, `
		mt = {
			__len = function(t)
				local count = 0
				for _ in pairs(t) do count = count + 1 end
				return count
			end
		}

		t = setmetatable({a = 1, b = 2, c = 3}, mt)
		print("  #t = " .. #t)  -- calls __len, returns 3
	`)

	// __tostring metamethod
	fmt.Println("\nMetatable __tostring:")
	state.DoStringOn(L, `
		mt = {
			__tostring = function(t)
				local parts = {}
				for k, v in pairs(t) do
					table.insert(parts, k .. "=" .. tostring(v))
				end
				return "{" .. table.concat(parts, ", ") .. "}"
			end
		}

		obj = setmetatable({name = "Alice", age = 30}, mt)
		print("  obj = " .. tostring(obj))
	`)

	// __call metamethod - making table callable
	fmt.Println("\nMetatable __call:")
	state.DoStringOn(L, `
		Accumulator = {}
		Accumulator.__index = Accumulator

		function Accumulator.new()
			return setmetatable({sum = 0, count = 0}, Accumulator)
		end

		Accumulator.__call = function(self, value)
			self.sum = self.sum + value
			self.count = self.count + 1
			return self.sum / self.count
		end

		acc = Accumulator.new()
		print("  acc(10) = " .. acc(10))
		print("  acc(20) = " .. acc(20))
		print("  acc(30) = " .. acc(30))
	`)

	// __eq metamethod
	fmt.Println("\nMetatable __eq:")
	state.DoStringOn(L, `
		Box = {}
		Box.__index = Box

		function Box.new(w, h)
			return setmetatable({width = w, height = h}, Box)
		end

		Box.__eq = function(a, b)
			return a.width == b.width and a.height == b.height
		end

		b1 = Box.new(10, 20)
		b2 = Box.new(10, 20)
		b3 = Box.new(10, 30)

		print("  b1 == b2: " .. tostring(b1 == b2))
		print("  b1 == b3: " .. tostring(b1 == b3))
	`)

	// Inheritance via metatable
	fmt.Println("\nInheritance via metatable:")
	state.DoStringOn(L, `
		-- Base class
		Animal = {}

		function Animal.speak(self)
			print("  " .. self.name .. " makes a sound")
		end

		function Animal.new(name)
			return setmetatable({name = name}, Animal)
		end

		-- Derived class
		Dog = Animal.new("Dog")  -- prototype
		Dog.__index = Dog

		function Dog.new(name, breed)
			local self = Animal.new(name)
			self.breed = breed
			return setmetatable(self, Dog)
		end

		function Dog.speak(self)
			print("  " .. self.name .. " barks: Woof!")
		end

		d = Dog.new("Buddy", "Golden Retriever")
		d:speak()  -- Dog's speak, not Animal's
	`)
}
