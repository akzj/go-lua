// Package api provides the public Lua API
package api

import (
	"fmt"
	"os"
	"strconv"

	"github.com/akzj/go-lua/pkg/object"
)

// OpenLibs registers standard library functions as globals.
//
// This registers the following Lua standard library functions:
//   - print: prints values to stdout
//   - type: returns the type name of a value
//   - tostring: converts a value to its string representation
//   - tonumber: converts a string to a number
//   - assert: raises an error if the first argument is falsy
//   - error: raises an error with the given message
//   - pcall: calls a function in protected mode
//   - pairs: returns an iterator for table traversal
//   - ipairs: returns an iterator for array traversal
//   - next: table traversal function
//   - select: returns arguments from a given index
//   - unpack: unpacks a table into individual values
func (s *State) OpenLibs() {
	// Basic functions
	s.Register("print", stdPrint)
	s.Register("type", stdType)
	s.Register("tostring", stdTostring)
	s.Register("tonumber", stdTonumber)
	s.Register("assert", stdAssert)
	s.Register("error", stdError)
	s.Register("pcall", stdPcall)
	s.Register("next", stdNext)
	s.Register("pairs", stdPairs)
	s.Register("ipairs", stdIpairs)
	s.Register("select", stdSelect)
	s.Register("unpack", stdUnpack)
	s.Register("setmetatable", stdSetmetatable)
	s.Register("getmetatable", stdGetmetatable)
	s.Register("rawset", stdRawset)
	s.Register("rawget", stdRawget)

	// Standard library modules
	s.openStringLib()
	s.openTableLib()
	s.openMathLib()
	s.openIOLib()
}

// stdPrint implements the Lua print() function.
// Prints all arguments to stdout, tab-separated, with a trailing newline.
func stdPrint(L *State) int {
	n := L.GetTop()
	for i := 1; i <= n; i++ {
		if i > 1 {
			fmt.Fprint(os.Stdout, "\t")
		}
		v := L.vm.GetStack(i)
		switch v.Type {
		case object.TypeNil:
			fmt.Fprint(os.Stdout, "nil")
		case object.TypeBoolean:
			b, _ := v.ToBoolean()
			fmt.Fprintf(os.Stdout, "%t", b)
		case object.TypeNumber:
			num, _ := v.ToNumber()
			fmt.Fprintf(os.Stdout, "%g", num)
		case object.TypeString:
			str, _ := v.ToString()
			fmt.Fprint(os.Stdout, str)
		default:
			fmt.Fprintf(os.Stdout, "%s: %p", v.Type.String(), v.Value.GC)
		}
	}
	fmt.Fprintln(os.Stdout)
	return 0
}

// stdType implements the Lua type() function.
// Returns the type name of the argument as a string.
func stdType(L *State) int {
	v := L.vm.GetStack(1)
	L.PushString(v.Type.String())
	return 1
}

// stdTostring implements the Lua tostring() function.
// Converts a value to its string representation.
func stdTostring(L *State) int {
	v := L.vm.GetStack(1)
	
	// Check for __tostring metamethod for tables
	if v.IsTable() {
		t, _ := v.ToTable()
		mm := L.vm.GetMetamethod(t, "__tostring")
		if mm != nil && mm.IsFunction() {
			// Call the metamethod
			result := L.vm.CallMetamethod(mm, []object.TValue{*v})
			if result != nil {
				L.vm.Stack[L.vm.StackTop].CopyFrom(result)
				L.vm.StackTop++
				return 1
			}
		}
	}
	
	switch v.Type {
	case object.TypeNil:
		L.PushString("nil")
	case object.TypeBoolean:
		b, _ := v.ToBoolean()
		if b {
			L.PushString("true")
		} else {
			L.PushString("false")
		}
	case object.TypeNumber:
		num, _ := v.ToNumber()
		L.PushString(fmt.Sprintf("%g", num))
	case object.TypeString:
		str, _ := v.ToString()
		L.PushString(str)
	default:
		L.PushString(fmt.Sprintf("%s: %p", v.Type.String(), v.Value.GC))
	}
	return 1
}

// stdTonumber implements the Lua tonumber() function.
// Converts a string to a number. Returns nil if conversion fails.
func stdTonumber(L *State) int {
	v := L.vm.GetStack(1)
	if v.IsNumber() {
		num, _ := v.ToNumber()
		L.PushNumber(num)
		return 1
	}
	if v.IsString() {
		str, _ := v.ToString()
		num, err := strconv.ParseFloat(str, 64)
		if err == nil {
			L.PushNumber(num)
			return 1
		}
	}
	L.PushNil()
	return 1
}

// stdAssert implements the Lua assert() function.
// If the first argument is falsy (nil or false), raises an error.
// Otherwise returns all arguments.
func stdAssert(L *State) int {
	v := L.vm.GetStack(1)
	if object.IsFalse(v) {
		msg := "assertion failed!"
		if L.GetTop() >= 2 {
			if s, ok := L.ToString(2); ok {
				msg = s
			}
		}
		L.PushString(msg)
		L.Error()
	}
	return L.GetTop()
}

// stdError implements the Lua error() function.
// Raises an error with the given message.
func stdError(L *State) int {
	msg := "error"
	if L.GetTop() >= 1 {
		v := L.vm.GetStack(1)
		switch v.Type {
		case object.TypeString:
			msg, _ = v.ToString()
		case object.TypeNumber:
			num, _ := v.ToNumber()
			msg = fmt.Sprintf("%g", num)
		}
	}
	L.PushString(msg)
	L.Error()
	return 0 // never reached
}

// stdPcall implements the Lua pcall() function.
// Calls a function in protected mode. Returns true, results... on success,
// or false, errmsg on error.
func stdPcall(L *State) int {
	// Get the function and arguments
	top := L.GetTop()
	
	if top < 1 {
		L.PushBoolean(false)
		L.PushString("bad argument #1 to 'pcall' (value expected)")
		return 2
	}

	fv := L.vm.GetStack(1)
	if !fv.IsFunction() {
		L.PushBoolean(false)
		L.PushString("attempt to call a non-function value")
		return 2
	}

	funcVal := *fv
	nargs := top - 1

	// The VM saves StackTop before calling us, then moves results from savedStackTop to funcSlot.
	savedStackTop := L.vm.StackTop

	// Push function and args for ProtectedCall
	funcPos := L.vm.StackTop
	L.vm.Push(funcVal)
	funcIdx := funcPos - L.vm.Base

	for i := 0; i < nargs; i++ {
		arg := L.vm.Stack[L.vm.Base+1+i]
		L.vm.Push(arg)
	}

	err := L.vm.ProtectedCall(funcIdx, nargs, -1)

	if err != nil {
		// Error case: reset stack to savedStackTop, then place false + error message
		L.vm.StackTop = savedStackTop
		L.vm.Stack[savedStackTop] = object.TValue{Type: object.TypeBoolean}
		L.vm.Stack[savedStackTop].Value.Bool = false
		L.vm.Stack[savedStackTop+1] = object.TValue{Type: object.TypeString}
		L.vm.Stack[savedStackTop+1].Value.Str = err.Error()
		L.vm.StackTop = savedStackTop + 2
		return 2
	}

	// Success: ProtectedCall placed results at Stack[funcPos]
	// Note: funcPos == savedStackTop, so we need to copy results first
	numResults := L.vm.StackTop - funcPos
	if numResults < 0 {
		numResults = 0
	}

	// Copy results to temp slice to avoid overwriting
	results := make([]object.TValue, numResults)
	for i := 0; i < numResults; i++ {
		results[i] = L.vm.Stack[funcPos+i]
	}

	// Place true at savedStackTop
	L.vm.Stack[savedStackTop] = object.TValue{Type: object.TypeBoolean}
	L.vm.Stack[savedStackTop].Value.Bool = true

	// Copy results after true
	for i := 0; i < numResults; i++ {
		L.vm.Stack[savedStackTop+1+i] = results[i]
	}

	L.vm.StackTop = savedStackTop + 1 + numResults

	return 1 + numResults
}

// stdNext implements the Lua next() function.
// next(table, key) returns the next key, value pair.
func stdNext(L *State) int {
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		L.PushNil()
		return 1
	}

	var key *object.TValue
	if L.GetTop() >= 2 {
		key = L.vm.GetStack(2)
	} else {
		nilVal := object.TValue{Type: object.TypeNil}
		key = &nilVal
	}

	nextKey, nextVal := t.Next(key)
	if nextKey == nil {
		L.PushNil()
		return 1
	}
	L.vm.Push(*nextKey)
	L.vm.Push(*nextVal)
	return 2
}

// stdPairs implements the Lua pairs() function.
// Returns the next function, the table, and nil (for generic for).
// If the table has a __pairs metamethod, calls that instead.
func stdPairs(L *State) int {
	// Get table from arg 1
	tbl := L.vm.GetStack(1)

	// Check for __pairs metamethod
	if tbl.IsTable() {
		t, _ := tbl.ToTable()
		mt := t.GetMetatable()
		if mt != nil {
			key := object.TValue{Type: object.TypeString}
			key.Value.Str = "__pairs"
			mm := mt.Get(key)
			if mm != nil && mm.IsFunction() {
				// Call __pairs(table)
				L.vm.Stack[L.vm.StackTop].CopyFrom(mm)
				L.vm.Stack[L.vm.StackTop+1].CopyFrom(tbl)
				L.vm.StackTop += 2
				L.vm.Base = L.vm.StackTop - 1
				
				closure, _ := mm.ToFunction()
				if closure.IsGo {
					closure.GoFn(L.vm)
					return L.vm.StackTop - L.vm.Base
				}
			}
		}
	}

	// Default behavior: push next function, table, nil
	L.GetGlobal("next")
	L.vm.Push(*tbl)
	L.PushNil()
	return 3
}

// stdIpairs implements the Lua ipairs() function.
// Returns an iterator function, the table, and 0.
func stdIpairs(L *State) int {
	// Get table from arg 1
	tbl := L.vm.GetStack(1)

	// Check for __ipairs metamethod
	if tbl.IsTable() {
		t, _ := tbl.ToTable()
		mt := t.GetMetatable()
		if mt != nil {
			key := object.TValue{Type: object.TypeString}
			key.Value.Str = "__ipairs"
			mm := mt.Get(key)
			if mm != nil && mm.IsFunction() {
				// Call __ipairs(table)
				L.vm.Stack[L.vm.StackTop].CopyFrom(mm)
				L.vm.Stack[L.vm.StackTop+1].CopyFrom(tbl)
				L.vm.StackTop += 2
				L.vm.Base = L.vm.StackTop - 1
				
				closure, _ := mm.ToFunction()
				if closure.IsGo {
					closure.GoFn(L.vm)
					return L.vm.StackTop - L.vm.Base
				}
			}
		}
	}

	// Default behavior: create iterator function
	// Create iterator function that reads control variable from stack
	L.PushFunction(func(L *State) int {
		// Get control variable from stack (arg 2 = control)
		ctrl := L.vm.GetStack(2)
		idx := 0
		if ctrl != nil && ctrl.IsNumber() {
			num, _ := ctrl.ToNumber()
			idx = int(num)
		}
		idx++ // next index

		// Get table from arg 1 (state)
		tv := L.vm.GetStack(1)
		t, ok := tv.ToTable()
		if !ok {
			L.PushNil()
			return 1
		}
		val := t.GetI(idx)
		if val == nil || val.IsNil() {
			L.PushNil()
			return 1
		}
		L.PushNumber(float64(idx))
		L.vm.Push(*val)
		return 2
	})
	// Push results on top of args: function, table, 0
	L.vm.Push(*tbl)
	L.PushNumber(0)
	return 3
}

// stdSelect implements the Lua select() function.
// select('#', ...) returns count; select(n, ...) returns all args from n.
func stdSelect(L *State) int {
	top := L.GetTop()
	v := L.vm.GetStack(1)

	// Check if first arg is '#'
	if v.IsString() {
		str, _ := v.ToString()
		if str == "#" {
			L.PushNumber(float64(top - 1))
			return 1
		}
	}

	// Numeric index
	num, ok := v.ToNumber()
	if !ok {
		L.PushNil()
		return 1
	}
	idx := int(num)
	if idx < 1 || idx > top-1 {
		return 0
	}

	// Return all values from index idx+1 to top
	return top - idx
}

// stdUnpack implements the Lua unpack() function.
// unpack(table [, i [, j]]) returns table[i], ..., table[j].
func stdUnpack(L *State) int {
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		return 0
	}

	i := 1
	j := t.Len()

	if L.GetTop() >= 2 {
		if num, ok := L.ToNumber(2); ok {
			i = int(num)
		}
	}
	if L.GetTop() >= 3 {
		if num, ok := L.ToNumber(3); ok {
			j = int(num)
		}
	}

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

// stdSetmetatable implements the Lua setmetatable(table, metatable) function.
// Sets the metatable of a table and returns the table.
// If metatable is nil, removes the metatable.
func stdSetmetatable(L *State) int {
	// Get the table argument
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		L.PushString("setmetatable: argument 1 must be a table")
		L.Error()
		return 0
	}

	// Get the metatable argument (can be nil)
	mtVal := L.vm.GetStack(2)
	if mtVal.IsNil() {
		// Remove metatable
		t.SetMetatable(nil)
	} else {
		mt, ok := mtVal.ToTable()
		if !ok {
			L.PushString("setmetatable: argument 2 must be a table or nil")
			L.Error()
			return 0
		}
		t.SetMetatable(mt)
	}

	// Return the table
	L.vm.Push(*v)
	return 1
}

// stdGetmetatable implements the Lua getmetatable(table) function.
// Returns the metatable of a table, or nil if no metatable.
func stdGetmetatable(L *State) int {
	// Get the table argument
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		// For non-tables, could check for __metatable in type's metatable
		// For now, just return nil
		L.PushNil()
		return 1
	}

	mt := t.GetMetatable()
	if mt == nil {
		L.PushNil()
		return 1
	}

	// Push the metatable
	mtVal := object.TValue{Type: object.TypeTable}
	mtVal.Value.GC = mt
	L.vm.Push(mtVal)
	return 1
}

// stdRawset implements the Lua rawset(table, key, value) function.
// Sets a key in a table without invoking metamethods.
func stdRawset(L *State) int {
	// Get the table argument
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		L.PushString("rawset: argument 1 must be a table")
		L.Error()
		return 0
	}

	// Get the key
	key := L.vm.GetStack(2)

	// Get the value
	val := L.vm.GetStack(3)

	// Set directly without metamethods
	t.Set(*key, *val)

	// Return the table
	L.vm.Push(*v)
	return 1
}

// stdRawget implements the Lua rawget(table, key) function.
// Gets a key from a table without invoking metamethods.
func stdRawget(L *State) int {
	// Get the table argument
	v := L.vm.GetStack(1)
	t, ok := v.ToTable()
	if !ok {
		L.PushString("rawget: argument 1 must be a table")
		L.Error()
		return 0
	}

	// Get the key
	key := L.vm.GetStack(2)

	// Get directly without metamethods
	val := t.Get(*key)
	if val == nil {
		L.PushNil()
	} else {
		L.vm.Push(*val)
	}
	return 1
}