// auxiliary.go — Auxiliary library functions (luaL_* equivalents).
package api

import (
	"fmt"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/object"

	"github.com/akzj/go-lua/internal/vm"
)

// ---------------------------------------------------------------------------
// Auxiliary functions (luaL_*)
// ---------------------------------------------------------------------------

// tagError raises a type error for argument arg.
func (L *State) tagError(arg int, tag object.Type) {
	L.TypeError(arg, L.TypeName(tag))
}

// CheckString checks that argument at idx is a string and returns it.
func (L *State) CheckString(idx int) string {
	s, ok := L.ToString(idx)
	if !ok {
		L.tagError(idx, object.TypeString)
	}
	return s
}

// CheckInteger checks that argument at idx is an integer and returns it.
// If the value is a float that can't be represented as an int64,
// raises "has no integer representation" error. If the value is not a
// number at all, raises "number expected" error.
// Mirrors: luaL_checkinteger → luaO_str2intX in lauxlib.c / lobject.c.
func (L *State) CheckInteger(idx int) int64 {
	v := L.index2val(idx)
	switch v.Tt {
	case object.TagInteger:
		return v.Val.(int64)
	case object.TagFloat:
		f := v.Val.(float64)
		if i, ok := object.FloatToInteger(f); ok {
			return i
		}
		L.ArgError(idx, fmt.Sprintf("number (%.10g) has no integer representation", f))
	case object.TagShortStr, object.TagLongStr:
		if i, ok := object.StringToInteger(v.Val.(*object.LuaString).Data); ok {
			return i
		}
		L.ArgError(idx, "malformed number")
	default:
		L.tagError(idx, object.TypeNumber)
	}
	return 0 // unreachable
}

// CheckNumber checks that argument at idx is a number and returns it.
func (L *State) CheckNumber(idx int) float64 {
	n, ok := L.ToNumber(idx)
	if !ok {
		L.tagError(idx, object.TypeNumber)
	}
	return n
}

// CheckType checks that argument at idx has the given type.
func (L *State) CheckType(idx int, tp object.Type) {
	if L.Type(idx) != tp {
		L.tagError(idx, tp)
	}
}

// CheckAny checks that there is an argument at idx.
func (L *State) CheckAny(idx int) {
	if L.Type(idx) == TypeNone {
		L.ArgError(idx, "value expected")
	}
}

// OptString returns the string at idx, or def if nil/none.
func (L *State) OptString(idx int, def string) string {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return L.CheckString(idx)
}

// OptInteger returns the integer at idx, or def if nil/none.
func (L *State) OptInteger(idx int, def int64) int64 {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return L.CheckInteger(idx)
}

// OptNumber returns the number at idx, or def if nil/none.
func (L *State) OptNumber(idx int, def float64) float64 {
	if L.IsNoneOrNil(idx) {
		return def
	}
	return L.CheckNumber(idx)
}

// ArgError raises an error for argument arg.
// Mirrors: luaL_argerror in lauxlib.c
func (L *State) ArgError(arg int, extraMsg string) int {
	ar, ok := L.GetStack(0)
	if !ok {
		// No stack frame
		msg := fmt.Sprintf("bad argument #%d (%s)", arg, extraMsg)
		L.PushString(msg)
		L.Error()
		return 0
	}
	L.GetInfo("nt", ar)
	argword := "argument"
	if arg <= ar.ExtraArgs {
		argword = "extra argument"
	} else {
		arg -= ar.ExtraArgs // do not count extra arguments
		if ar.NameWhat == "method" {
			arg-- // do not count 'self'
			if arg == 0 {
				msg := fmt.Sprintf("calling '%s' on bad self (%s)", ar.Name, extraMsg)
				L.PushString(msg)
				L.Error()
				return 0
			}
		}
	}
	name := ar.Name
	if name == "" {
		// Fallback: search loaded modules for the function name.
		// Mirrors pushglobalfuncname in lauxlib.c.
		if gname := L.pushGlobalFuncName(ar); gname != "" {
			name = gname
		} else {
			name = "?"
		}
	}
	msg := fmt.Sprintf("bad %s #%d to '%s' (%s)", argword, arg, name, extraMsg)
	L.PushString(msg)
	L.Error()
	return 0
}

// pushGlobalFuncName searches all loaded modules for the function at ar's
// call frame and returns its dotted name (e.g. "io.write"), or "" if not found.
// Mirrors: pushglobalfuncname + findfield in lauxlib.c.
func (L *State) pushGlobalFuncName(ar *DebugInfo) string {
	top := L.GetTop()
	// Push the function value from the debug info's call frame
	L.GetInfo("f", ar) // pushes the function onto the stack
	funcIdx := L.GetTop()
	// Get _LOADED table from registry
	L.GetField(RegistryIndex, "_LOADED")
	if !L.IsTable(-1) {
		L.SetTop(top) // restore stack
		return ""
	}
	name := L.findField(funcIdx, 2)
	if name != "" {
		// Strip "_G." prefix if present
		if len(name) > 3 && name[:3] == "_G." {
			name = name[3:]
		}
	}
	L.SetTop(top) // restore stack
	return name
}

// findField recursively searches the table at the top of the stack for
// a value that is rawequal to the value at objIdx. Returns the dotted
// field name or "" if not found. level limits recursion depth.
// Mirrors: findfield in lauxlib.c.
func (L *State) findField(objIdx int, level int) string {
	if level == 0 || !L.IsTable(-1) {
		return ""
	}
	L.PushNil() // start iteration
	// Stack: ..., table, nil(key)
	for L.Next(-2) {
		// Stack: ..., table, key, value
		if L.Type(-2) == object.TypeString { // ignore non-string keys
			if L.RawEqual(objIdx, -1) {
				// Found! Key is at -2, value at -1
				name, _ := L.ToString(-2)
				L.Pop(2) // pop value and key
				return name
			}
			if level > 1 {
				// Value is at -1; try recursively into it (if it's a table)
				if sub := L.findField(objIdx, level-1); sub != "" {
					// Stack: ..., table, key, value
					parentName, _ := L.ToString(-2) // the key
					L.Pop(2)                        // pop value and key
					return parentName + "." + sub
				}
			}
		}
		L.Pop(1) // pop value, keep key for next iteration
		// Stack: ..., table, key
	}
	return ""
}

// TypeError raises a type error for argument arg.
// Mirrors: luaL_typeerror in lauxlib.c — checks __name, then light userdata, then standard name.
func (L *State) TypeError(arg int, tname string) int {
	var typearg string
	if L.GetMetafield(arg, "__name") {
		typearg, _ = L.ToString(-1)
		L.Pop(1)
	} else if L.Type(arg) == object.TypeLightUserdata {
		typearg = "light userdata"
	} else {
		typearg = L.TypeName(L.Type(arg))
	}
	msg := fmt.Sprintf("%s expected, got %s", tname, typearg)
	return L.ArgError(arg, msg)
}

// Where pushes "source:line: " for the given call level.
func (L *State) Where(level int) {
	// Mirrors: luaL_where in lauxlib.c
	ls := L.ls()
	ci := ls.CI
	// Walk up 'level' call frames
	for i := 0; i < level && ci.Prev != nil; i++ {
		ci = ci.Prev
	}
	if ci.IsLua() {
		fval := ls.Stack[ci.Func].Val
		if fval.Tt == object.TagLuaClosure {
			cl := fval.Val.(*closure.LClosure)
			pc := ci.SavedPC - 1
			if pc < 0 {
				pc = 0
			}
			line := vm.GetFuncLine(cl.Proto, pc)
			srcName := "?"
			if cl.Proto.Source != nil {
				srcName = vm.ShortSrc(cl.Proto.Source.Data)
			}
			if line <= 0 {
				L.PushString(fmt.Sprintf("%s:?: ", srcName))
			} else {
				L.PushString(fmt.Sprintf("%s:%d: ", srcName, line))
			}
			return
		}
	}
	L.PushString("")
}

// Errorf raises a formatted error.
func (L *State) Errorf(format string, args ...interface{}) int {
	L.Where(1)
	L.PushFString(format, args...)
	// Concatenate where + message
	ls := L.ls()
	if ls.Top >= 2 {
		where, _ := L.ToString(-2)
		msg, _ := L.ToString(-1)
		ls.Top -= 2
		L.PushString(where + msg)
	}
	L.Error()
	return 0
}

// SetFuncs registers functions from a map into the table at top of stack.
func (L *State) SetFuncs(funcs map[string]CFunction, nUp int) {
	for name, fn := range funcs {
		if fn == nil {
			L.PushBoolean(false)
		} else {
			// Copy upvalues for each function
			for i := 0; i < nUp; i++ {
				L.PushValue(-nUp) // copy upvalue
			}
			L.PushCClosure(fn, nUp)
		}
		L.SetField(-(nUp + 2), name)
	}
	if nUp > 0 {
		L.Pop(nUp) // remove upvalues
	}
}

// NewLib creates a new table and registers functions into it.
func (L *State) NewLib(funcs map[string]CFunction) {
	L.CreateTable(0, len(funcs))
	L.SetFuncs(funcs, 0)
}

// Require calls openf to load a module, stores in package.loaded.
// If the module is already in package.loaded, pushes the cached value.
// Mirrors luaL_requiref in lauxlib.c.
func (L *State) Require(modname string, openf CFunction, global bool) {
	// Get package.loaded table (or create it)
	L.GetSubTable(RegistryIndex, "_LOADED")
	tp := L.GetField(-1, modname)
	if tp != object.TypeNil {
		// Already loaded — remove _LOADED table, keep the module
		L.Remove(-2)
		return
	}
	L.Pop(1) // pop nil

	// Call the opener
	L.PushCFunction(openf)
	L.PushString(modname)
	L.Call(1, 1) // call openf(modname) -> module table on top

	// Store in package.loaded
	L.PushValue(-1)         // copy module
	L.SetField(-3, modname) // _LOADED[modname] = module

	L.Remove(-2) // remove _LOADED table, keep module on top

	if global {
		L.PushValue(-1)      // copy module
		L.SetGlobal(modname) // _G[modname] = module
	}
}

// Ref creates a reference in the table at idx (luaL_ref).
// Pops the top value and stores it in the table, returning an integer key.
// If the value is nil, returns RefNil (-1) and pops the value without storing.
func (L *State) Ref(t int) int {
	if L.IsNil(-1) {
		L.Pop(1) // remove nil from stack
		return RefNil
	}
	// Convert t to absolute index before we manipulate the stack
	absT := t
	if t != RegistryIndex && t > 0 {
		absT = t
	} else if t != RegistryIndex && t < 0 {
		absT = L.GetTop() + t + 1
	}
	// Check free list at t[0]
	L.RawGetI(absT, 0) // push t[0]
	ref, ok := L.ToInteger(-1)
	L.Pop(1) // pop t[0]
	if ok && ref != 0 {
		// Free list is non-empty: reuse this key
		key := int(ref)
		// Update free list: t[0] = t[key] (next in chain)
		L.RawGetI(absT, int64(key)) // push t[key] (next free ref)
		L.RawSetI(absT, 0)          // t[0] = popped value (next free ref)
		// Store the value (currently at top) into t[key]
		// The value to store is now at top-1 (it was pushed before Ref was called)
		// Actually: the original value is still on the stack at the position before our operations
		// Let's re-check: at this point, the original value is at top of stack
		L.RawSetI(absT, int64(key)) // t[key] = top value (pops it)
		return key
	}
	// Free list empty: use next integer key = #t + 1
	key := int(L.RawLen(absT)) + 1
	L.RawSetI(absT, int64(key)) // t[key] = top value (pops it)
	return key
}

// Unref frees a reference in the table at idx (luaL_unref).
// Sets t[ref] = nil and adds ref to the free list at t[0].
func (L *State) Unref(t int, ref int) {
	if ref > 0 {
		// Convert t to absolute index
		absT := t
		if t != RegistryIndex && t < 0 {
			absT = L.GetTop() + t + 1
		}
		// t[ref] = t[0] (link into free list — store old head as value)
		L.RawGetI(absT, 0)          // push current free list head (t[0])
		L.RawSetI(absT, int64(ref)) // t[ref] = old head (pops it)
		// t[0] = ref (new free list head)
		L.PushInteger(int64(ref))
		L.RawSetI(absT, 0) // t[0] = ref (pops it)
	}
}
