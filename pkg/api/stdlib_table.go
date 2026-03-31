// Package api provides the public Lua API
// This file implements the table standard library module
package api

import (
	"sort"

	"github.com/akzj/go-lua/pkg/object"
)

// openTableLib registers the table module
func (s *State) openTableLib() {
	funcs := map[string]Function{
		"insert": stdTableInsert,
		"remove": stdTableRemove,
		"sort":   stdTableSort,
		"concat": stdTableConcat,
		"pack":   stdTablePack,
		"unpack": stdTableUnpack,
		"create": stdTableCreate, // Lua 5.5 function
	}
	s.RegisterModule("table", funcs)
}

// stdTableCreate implements table.create(n) - Lua 5.5 function
// Creates a table with pre-allocated array part for n elements.
func stdTableCreate(L *State) int {
	// Just create a new table - Go tables don't need pre-allocation
	L.NewTable()
	return 1
}

// stdTableInsert implements table.insert(t, [pos,] value)
// Inserts value at position pos (or at end if pos not given).
func stdTableInsert(L *State) int {
	// Get table from arg 1
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		L.PushString("table.insert: argument 1 must be a table")
		L.Error()
		return 0
	}

	top := L.GetTop()
	if top < 2 {
		L.PushString("table.insert: wrong number of arguments")
		L.Error()
		return 0
	}

	if top == 2 {
		// table.insert(t, value) - append to end
		value := *L.vm.GetStack(2)
		len := t.Len()
		t.SetI(len+1, value)
	} else {
		// table.insert(t, pos, value) - insert at position
		pos, ok := L.ToNumber(2)
		if !ok {
			L.PushString("table.insert: position must be a number")
			L.Error()
			return 0
		}

		value := *L.vm.GetStack(3)
		posInt := int(pos)

		// Shift elements up
		len := t.Len()
		for i := len; i >= posInt; i-- {
			val := t.GetI(i)
			if val != nil {
				t.SetI(i+1, *val)
			}
		}

		// Insert at position
		t.SetI(posInt, value)
	}

	return 0
}

// stdTableRemove implements table.remove(t [, pos])
// Removes element at pos (or last element if pos not given).
// Returns the removed element.
func stdTableRemove(L *State) int {
	// Get table from arg 1
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		L.PushString("table.remove: argument 1 must be a table")
		L.Error()
		return 0
	}

	len := t.Len()
	if len == 0 {
		L.PushNil()
		return 1
	}

	// Get position (default: last element)
	pos := len
	if L.GetTop() >= 2 {
		if num, ok := L.ToNumber(2); ok {
			pos = int(num)
		}
	}

	if pos < 1 || pos > len {
		L.PushNil()
		return 1
	}

	// Get the value to return
	val := t.GetI(pos)
	if val == nil {
		L.PushNil()
	} else {
		L.vm.Push(*val)
	}

	// Shift elements down
	for i := pos; i < len; i++ {
		nextVal := t.GetI(i + 1)
		if nextVal != nil {
			t.SetI(i, *nextVal)
		} else {
			// Set to nil - need to handle this
			t.SetI(i, object.TValue{Type: object.TypeNil})
		}
	}

	// Remove last element
	t.SetI(len, object.TValue{Type: object.TypeNil})

	return 1
}

// stdTableSort implements table.sort(t [, comp])
// Sorts table elements in place.
func stdTableSort(L *State) int {
	// Get table from arg 1
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		L.PushString("table.sort: argument 1 must be a table")
		L.Error()
		return 0
	}

	// Check for comparison function
	var compFn Function
	if L.GetTop() >= 2 && L.IsFunction(2) {
		// We have a custom comparison function
		// This is complex - need to call Lua function from Go
		// For now, use default comparison
	}
	_ = compFn // We use default comparison for now

	// Get array elements
	len := t.Len()
	if len <= 1 {
		return 0
	}

	// Collect elements
	elements := make([]object.TValue, len)
	for i := 1; i <= len; i++ {
		val := t.GetI(i)
		if val != nil {
			elements[i-1] = *val
		} else {
			elements[i-1] = object.TValue{Type: object.TypeNil}
		}
	}

	// Sort using default comparison (<)
	sort.Slice(elements, func(i, j int) bool {
		a := elements[i]
		b := elements[j]

		// Handle nil values
		if a.IsNil() {
			return false
		}
		if b.IsNil() {
			return true
		}

		// Compare numbers
		if a.IsNumber() && b.IsNumber() {
			an, _ := a.ToNumber()
			bn, _ := b.ToNumber()
			return an < bn
		}

		// Compare strings
		if a.IsString() && b.IsString() {
			as, _ := a.ToString()
			bs, _ := b.ToString()
			return as < bs
		}

		// Default: compare by type
		return a.Type < b.Type
	})

	// Write back to table
	for i := 0; i < len; i++ {
		t.SetI(i+1, elements[i])
	}

	return 0
}

// stdTableConcat implements table.concat(t [, sep [, i [, j]]])
// Returns concatenated string of table elements.
func stdTableConcat(L *State) int {
	// Get table from arg 1
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		typeName := "nil"
		if v != nil {
			typeName = object.TypeName(v)
		}
		L.PushString("bad argument #1 to 'concat' (table expected, got " + typeName + ")")
		L.Error()
		return 0
	}

	// Get separator (default: "")
	sep := ""
	if L.GetTop() >= 2 {
		sep, _ = L.ToString(2)
	}

	// Get start index (default: 1) - use int64 to handle maxi/mini
	var i int64 = 1
	if L.GetTop() >= 3 {
		sv := L.vm.GetStack(3)
		if sv != nil {
			if ival, ok := sv.ToInteger(); ok {
				i = ival
			} else if num, ok := sv.ToNumber(); ok {
				i = int64(num)
			}
		}
	}

	// Get end index (default: length) - use int64
	var j int64 = int64(t.Len())
	if L.GetTop() >= 4 {
		sv := L.vm.GetStack(4)
		if sv != nil {
			if ival, ok := sv.ToInteger(); ok {
				j = ival
			} else if num, ok := sv.ToNumber(); ok {
				j = int64(num)
			}
		}
	}

	// Empty range: return ""
	if i > j {
		L.PushString("")
		return 1
	}

	// Collect string parts - each element MUST be string or number
	parts := make([]string, 0)
	k := i
	for {
		val := t.GetI(int(k))
		if val == nil || val.IsNil() {
			// Lua raises error for nil values in concat range
			L.PushString("invalid value (nil) at index " + formatInt64(k) + " in table for 'concat'")
			L.Error()
			return 0
		}
		if val.IsString() {
			str, _ := val.ToString()
			parts = append(parts, str)
		} else if val.IsNumber() {
			num, _ := val.ToNumber()
			parts = append(parts, formatNumber(num))
		} else {
			// Cannot concatenate non-string/number
			L.PushString("invalid value (" + object.TypeName(val) + ") at index " + formatInt64(k) + " in table for 'concat'")
			L.Error()
			return 0
		}
		if k == j {
			break
		}
		k++
	}

	// Join and return
	result := ""
	for idx, part := range parts {
		if idx > 0 {
			result += sep
		}
		result += part
	}

	L.PushString(result)
	return 1
}

// formatInt64 formats an int64 as a string
func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		// Handle math.mininteger carefully
		if n == -9223372036854775808 {
			return "-9223372036854775808"
		}
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}

// stdTablePack implements table.pack(...)
// Returns a table with all arguments, with n field set to count.
func stdTablePack(L *State) int {
	// Save original argument count before creating table
	n := L.GetTop()

	// Create new table
	L.NewTable()
	tableIdx := L.GetTop()

	// Pack all arguments into table (from original stack positions)
	for i := 1; i <= n; i++ {
		v := L.vm.GetStack(i)
		L.vm.Push(*v)
		L.SetI(tableIdx, i)
	}

	// Set n field
	L.PushInteger(int64(n))
	L.SetField(tableIdx, "n")

	return 1
}

// stdTableUnpack implements table.unpack(t [, i [, j]])
// Returns elements from table as multiple values.
func stdTableUnpack(L *State) int {
	// Get table from arg 1
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		return 0
	}

	// Get start index (default: 1)
	i := 1
	if L.GetTop() >= 2 {
		if num, ok := L.ToNumber(2); ok {
			i = int(num)
		}
	}

	// Get end index (default: length)
	j := t.Len()
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			j = int(num)
		}
	}

	// Push all values
	count := 0
	for k := i; k <= j; k++ {
		val := t.GetI(k)
		if val != nil {
			L.vm.Push(*val)
		} else {
			L.PushNil()
		}
		count++
	}

	return count
}

// formatNumber formats a number for display
func formatNumber(n float64) string {
	if n == float64(int(n)) {
		return formatInt(int(n))
	}
	return formatFloat(n)
}

// formatInt formats an integer
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}

	neg := false
	if n < 0 {
		neg = true
		n = -n
	}

	var digits []byte
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}

	if neg {
		digits = append(digits, '-')
	}

	// Reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}

	return string(digits)
}

// formatFloat formats a float
func formatFloat(n float64) string {
	// Simple implementation
	str := ""
	whole := int(n)
	frac := n - float64(whole)

	str = formatInt(whole)
	if frac != 0 {
		str += "."
		// Get decimal digits
		frac *= 1000000
		fracInt := int(frac)
		// Remove trailing zeros
		for fracInt%10 == 0 && fracInt > 0 {
			fracInt /= 10
		}
		str += formatInt(fracInt)
	}
	return str
}
