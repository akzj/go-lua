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
//   - debug: debug library (getinfo, getlocal, traceback, etc.)
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
	s.Register("rawequal", stdRawequal)
	s.Register("rawlen", stdRawlen)
	s.Register("dofile", stdDofile)
	s.Register("loadfile", stdLoadfile)
	s.Register("require", stdRequire)
	s.Register("load", stdLoad)
	s.Register("collectgarbage", stdCollectgarbage)

	// Standard library modules
	s.openStringLib()
	s.openTableLib()
	s.openMathLib()
	s.openIOLib()
	s.openDebugLib()

	// Set standard global variables
	// _G is a reference to the global table (same as _ENV)
	globalTable := s.getGlobalTable()
	tv := object.TValue{Type: object.TypeTable}
	tv.Value.GC = globalTable
	s.vm.Push(tv)
	s.SetGlobal("_G")
	// _VERSION is the Lua version string
	s.PushString("Lua 5.4")
	s.SetGlobal("_VERSION")
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
// error(message [, level])
// level 0 = no stack trace
// level 1 = add stack trace (default)
func stdError(L *State) int {
	msg := "error"
	level := 1 // Default level
	
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
	
	if L.GetTop() >= 2 {
		levelFloat, ok := L.ToNumber(2)
		if ok {
			level = int(levelFloat)
		}
	}
	
	if level == 0 {
		// No stack trace - just panic with the message
		panic(&LuaError{Message: msg})
	} else {
		L.PushString(msg)
		L.Error()
	}
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

	nargs := top - 1
	
	// Save the StackTop that the outer CALL handler set
	// This is where our results should start
	savedStackTop := L.vm.StackTop
	
	// Call the function - it's at Stack[Base], so funcIdx=0
	err := L.vm.ProtectedCall(0, nargs, -1)

	if err != nil {
		// Restore StackTop and push error results
		L.vm.StackTop = savedStackTop
		L.PushBoolean(false)
		L.PushString(err.Error())
		return 2
	}

	// Success: ProtectedCall placed results starting at Stack[Base]
	// Count the results before we change StackTop
	numResults := L.vm.StackTop - L.vm.Base
	
	// Restore StackTop to where the outer CALL expects results
	L.vm.StackTop = savedStackTop
	
	// Push true
	L.PushBoolean(true)
	
	// Copy results from Stack[Base] to stack top
	for i := 0; i < numResults; i++ {
		L.vm.Push(L.vm.Stack[L.vm.Base+i])
	}

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

// stdRequire implements a minimal require() function.
// For now, returns the global module table if it exists, or an empty table for unknown modules.
func stdRequire(L *State) int {
	// Get module name (argument 1)
	if L.GetTop() < 1 {
		L.NewTable()
		return 1
	}
	
	nameVal := L.vm.GetStack(1)
	if !nameVal.IsString() {
		L.NewTable()
		return 1
	}
	
	name, _ := nameVal.ToString()
	
	// Check if the module exists as a global
	L.GetGlobal(name)
	modType := L.vm.GetStack(-1).Type
	
	if modType == object.TypeTable {
		// Module exists, return it
		return 1
	}
	
	// Pop the nil/non-table value
	L.vm.Pop()
	
	// Return an empty table for unknown modules
	L.NewTable()
	return 1
}

// stdLoad implements the Lua load() function.
// load(chunk [, chunkname [, mode [, env]]])
// Loads a chunk of Lua code and returns it as a function.
func stdLoad(L *State) int {
	top := L.GetTop()
	if top < 1 {
		L.PushNil()
		L.PushString("bad argument #1 to 'load' (string expected, got no value)")
		return 2
	}

	// Get the chunk (argument 1)
	chunkVal := L.vm.GetStack(1)

	// Check if chunk is a string or function
	var code string
	var isFunction bool

	if chunkVal.IsString() {
		code, _ = chunkVal.ToString()
	} else if chunkVal.IsFunction() {
		isFunction = true
	} else {
		L.PushNil()
		L.PushString("bad argument #1 to 'load' (string or function expected)")
		return 2
	}

	// Get optional chunkname (argument 2)
	name := "load"
	if top >= 2 {
		v := L.vm.GetStack(2)
		if v != nil && !v.IsNil() {
			if s, ok := v.ToString(); ok {
				name = s
			}
		}
	}

	// mode (argument 3) is ignored for now - we only support text mode

	// Get optional env (argument 4)
	var envTable *object.Table
	if top >= 4 {
		envVal := L.vm.GetStack(4)
		if envVal != nil && envVal.IsTable() {
			envTable, _ = envVal.ToTable()
		}
	}

	// Handle function chunk (reader function)
	if isFunction {
		// Call the function repeatedly to get the chunk
		// For simplicity, we'll collect all results into a string
		var chunks []byte

		for {
			// Push function and call it
			L.vm.Push(*chunkVal)
			if err := L.Call(0, 1); err != nil {
				L.PushNil()
				L.PushString(err.Error())
				return 2
			}

			// Get result
			result := L.vm.GetStack(-1)
			if result.IsNil() || result.IsString() {
				if result.IsString() {
					s, _ := result.ToString()
					chunks = append(chunks, []byte(s)...)
				}
				L.Pop(1) // Pop the result
				break // End of chunk
			}

			L.Pop(1) // Pop the result
		}
		code = string(chunks)
	}

	// Load the code
	err := L.LoadString(code, name)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	// If env is provided, set it as the _ENV upvalue
	if envTable != nil {
		// Get the function from the stack
		funcVal := L.vm.GetStack(-1)
		if funcVal.IsFunction() {
			closure, ok := funcVal.ToFunction()
			if ok && !closure.IsGo && len(closure.Upvalues) > 0 {
				// Set the _ENV upvalue (index 0) to point to the provided env table
				envTValue := &object.TValue{Type: object.TypeTable}
				envTValue.Value.GC = envTable
				closure.Upvalues[0].Value = envTValue
			}
		}
	}

	// Return the function
	return 1
}

// stdCollectgarbage implements the collectgarbage() function.
// This is a minimal implementation.
// Lua's collectgarbage with no args runs "collect" and returns nothing.
func stdCollectgarbage(L *State) int {
	// Check if there's an argument
	if L.GetTop() > 0 {
		opt := L.vm.GetStack(1)
		if opt.IsString() {
			optStr, _ := opt.ToString()
			switch optStr {
			case "count":
				// Return memory usage in KB (approximate)
				L.PushNumber(1024.0) // placeholder
				return 1
			case "step":
				// Perform a GC step
				return 0
			}
		}
	}
	// Default: collect, return nothing
	return 0
}

// stdRawequal implements rawequal(v1, v2)
func stdRawequal(L *State) int {
	if L.GetTop() < 2 {
		L.PushBoolean(false)
		return 1
	}
	v1 := L.vm.GetStack(1)
	v2 := L.vm.GetStack(2)
	L.PushBoolean(object.Equal(v1, v2))
	return 1
}

// stdRawlen implements rawlen(v)
func stdRawlen(L *State) int {
	if L.GetTop() < 1 {
		L.PushNumber(0)
		return 1
	}
	v := L.vm.GetStack(1)
	if v.IsString() {
		s, _ := v.ToString()
		L.PushNumber(float64(len(s)))
	} else if v.IsTable() {
		t, _ := v.ToTable()
		L.PushNumber(float64(t.Len()))
	} else {
		L.PushNumber(0)
	}
	return 1
}

// stdDofile implements dofile([filename])
func stdDofile(L *State) int {
	// Minimal implementation - just return nil
	L.PushNil()
	return 1
}

// stdLoadfile implements loadfile([filename [, mode [, env]]])
func stdLoadfile(L *State) int {
	// Minimal implementation - just return nil
	L.PushNil()
	L.PushString("loadfile not implemented")
	return 2
}