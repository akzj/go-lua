// Package examples provides usage examples for go-lua.
package examples

import (
	"fmt"

	"github.com/akzj/go-lua/state"
)

// TableOperations demonstrates table creation and manipulation.
func TableOperations() {
	L := state.New()

	// Create a table from Lua code
	state.DoStringOn(L, `
		-- Create a table with mixed keys
		person = {
			name = "Alice",
			age = 30,
			address = {
				city = "Beijing",
				country = "China"
			}
		}

		-- Create an array-style table
		animals = {"dog", "cat", "bird", "fish"}
	`)

	// Access table fields from Lua
	fmt.Println("Table access from Lua:")
	state.DoStringOn(L, `
		print("  Name: " .. person.name)
		print("  Age: " .. person.age)
		print("  City: " .. person.address.city)
	`)

	// Access table fields programmatically
	fmt.Println("\nTable iteration:")
	state.DoStringOn(L, `
		-- Iterate over array part with ipairs
		print("  Animals (ipairs):")
		for i, animal in ipairs(animals) do
			print("    " .. i .. ": " .. animal)
		end

		-- Iterate over hash part with pairs
		print("  Person (pairs):")
		for k, v in pairs(person) do
			if type(v) ~= "table" then
				print("    " .. k .. ": " .. v)
			end
		end
	`)

	// Table operations
	fmt.Println("\nTable operations:")
	state.DoStringOn(L, `
		-- Table length
		print("  #animals = " .. #animals)

		-- Table concat
		print("  table.concat(animals, ', ') = " .. table.concat(animals, ", "))

		-- Add element
		table.insert(animals, "rabbit")
		print("  After insert: " .. table.concat(animals, ", "))

		-- Sort
		table.sort(animals)
		print("  After sort: " .. table.concat(animals, ", "))
	`)

	// Nested tables
	fmt.Println("\nNested tables:")
	state.DoStringOn(L, `
		-- Matrix as nested tables
		matrix = {
			{1, 2, 3},
			{4, 5, 6},
			{7, 8, 9}
		}
		print("  matrix[2][2] = " .. matrix[2][2])

		-- Iterate matrix
		for i, row in ipairs(matrix) do
			print("  Row " .. i .. ": " .. table.concat(row, ", "))
		end
	`)
}
