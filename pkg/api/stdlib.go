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
	// Save the arguments (skip function at position 1)
	top := L.GetTop()
	args := make([]object.TValue, 0, top-1)
	for i := 2; i <= top; i++ {
		v := L.vm.GetStack(i)
		args = append(args, *v)
	}

	// Get the function value
	fv := L.vm.GetStack(1)
	if !fv.IsFunction() {
		L.vm.SetTop(0)
		L.PushBoolean(false)
		L.PushString("attempt to call a non-function value")
		return 2
	}

	funcVal := *fv

	var callErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				if le, ok := r.(*LuaError); ok {
					callErr = le
				} else if s, ok := r.(string); ok {
					callErr = newLuaError(s)
				} else {
					callErr = newLuaError(fmt.Sprintf("%v", r))
				}
			}
		}()

		fn, _ := funcVal.ToFunction()
		if fn.IsGo {
			// Set up stack for the Go function call
			oldBase := L.vm.Base
			oldTop := L.vm.StackTop

			// Clear current stack and push args for the called function
			L.vm.SetTop(0)
			for _, arg := range args {
				L.vm.Push(arg)
			}

			newBase := L.vm.Base
			L.vm.Base = newBase
			L.vm.StackTop = newBase + len(args)

			err := fn.GoFn(L.vm)

			L.vm.Base = oldBase
			_ = oldTop

			if err != nil {
				panic(err)
			}
		} else {
			// Lua function: use ProtectedCall
			// The function is already at Stack[Base] (position 1)
			// Args are already at Stack[Base+1], ... (positions 2, 3, ...)
			// Call ProtectedCall with function at position 0 (relative to Base)
			// Note: we need to adjust the stack to have function at Base+0
			// The function arg is at position 1, which is Stack[Base]
			err := L.vm.ProtectedCall(0, len(args), -1)
			if err != nil {
				panic(err)
			}
		}
	}()

	if callErr != nil {
		L.vm.SetTop(0)
		L.PushBoolean(false)
		L.PushString(callErr.Error())
		return 2
	}

	// Success: collect results, clear stack, push true + results
	nresults := L.GetTop()
	results := make([]object.TValue, nresults)
	for i := 1; i <= nresults; i++ {
		v := L.vm.GetStack(i)
		results[i-1] = *v
	}
	L.vm.SetTop(0)
	L.PushBoolean(true)
	for _, r := range results {
		L.vm.Push(r)
	}
	return 1 + len(results)
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
func stdPairs(L *State) int {
	// Get table from arg 1
	tbl := *L.vm.GetStack(1)

	// Push results on top of args: next function, table, nil
	L.GetGlobal("next")
	L.vm.Push(tbl)
	L.PushNil()
	return 3
}

// stdIpairs implements the Lua ipairs() function.
// Returns an iterator function, the table, and 0.
func stdIpairs(L *State) int {
	// Get table from arg 1
	tbl := *L.vm.GetStack(1)

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
	L.vm.Push(tbl)
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