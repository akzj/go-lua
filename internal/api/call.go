// call.go — Call and load operations (Call, PCall, Load, DoFile, DoString).
package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akzj/go-lua/internal/closure"
	"github.com/akzj/go-lua/internal/object"

	"github.com/akzj/go-lua/internal/state"
	"github.com/akzj/go-lua/internal/vm"
)

// ---------------------------------------------------------------------------
// Call/Load functions
// ---------------------------------------------------------------------------

// Call calls a function. nArgs arguments are on the stack above the function.
func (L *State) Call(nArgs, nResults int) {
	ls := L.ls()
	funcIdx := ls.Top - nArgs - 1
	// C Lua's lua_callk with k==NULL calls luaD_callnoyield.
	// This marks the call as non-yieldable.
	vm.CallNoYield(ls, funcIdx, nResults)
	// Ensure Top >= CI.Func + 1 so the API stack is valid
	base := ls.CI.Func + 1
	if ls.Top < base {
		ls.Top = base
	}
}

// CallK calls a function with a continuation for yielding.
// If k is non-nil and the coroutine is yieldable, the call is made yieldable
// and the continuation k will be invoked upon resume after a yield.
// Otherwise behaves identically to Call (non-yieldable).
// Mirrors: lua_callk in lapi.c
func (L *State) CallK(nArgs, nResults int, ctx int, k state.KFunction) {
	ls := L.ls()
	funcIdx := ls.Top - nArgs - 1
	if k != nil && ls.Yieldable() {
		// Set continuation on the current CallInfo so that if the called
		// function (or anything it calls) yields, the VM can resume via k.
		ls.CI.K = k
		ls.CI.Ctx = ctx
		vm.Call(ls, funcIdx, nResults)
	} else {
		// No continuation or not yieldable — same as Call().
		vm.CallNoYield(ls, funcIdx, nResults)
	}
	// Ensure Top >= CI.Func + 1 so the API stack is valid
	base := ls.CI.Func + 1
	if ls.Top < base {
		ls.Top = base
	}
}

// PCall performs a protected call. Returns status code.
func (L *State) PCall(nArgs, nResults, msgHandler int) int {
	ls := L.ls()
	funcIdx := ls.Top - nArgs - 1
	errFunc := 0
	if msgHandler != 0 {
		errFunc = L.index2stack(msgHandler)
	}
	status := vm.PCall(ls, funcIdx, nResults, errFunc)
	// Ensure Top >= CI.Func + 1 so the API stack is valid
	base := ls.CI.Func + 1
	if ls.Top < base {
		ls.Top = base
	}
	return status
}

// Load loads a Lua chunk from a string. Pushes the compiled function.
func (L *State) Load(code string, name string, mode string) int {
	ls := L.ls()
	if mode == "" {
		mode = "bt"
	}
	// Validate mode: "b", "t", "bt", "tb" are standard; "B" = fixed-buffer binary (Lua 5.5)
	validMode := true
	for _, c := range mode {
		if c != 'b' && c != 't' && c != 'B' {
			validMode = false
			break
		}
	}
	if !validMode || len(mode) == 0 || len(mode) > 2 {
		L.PushString(fmt.Sprintf("invalid mode '%s'", mode))
		return StatusErrSyntax
	}
	isBinary := len(code) > 0 && code[0] == '\x1b' // LUA_SIGNATURE

	if isBinary {
		if !strings.Contains(mode, "b") && !strings.Contains(mode, "B") {
			L.PushString(fmt.Sprintf("%s: attempt to load a binary chunk", name))
			return StatusErrSyntax
		}
		// Binary chunk — use undump
		cl, err := vm.UndumpProto(ls, []byte(code), name)
		if err != nil {
			L.PushString(err.Error())
			return StatusErrSyntax
		}
		// Push the closure onto the stack
		L.push(object.TValue{Tt: object.TagLuaClosure, Obj: cl})
		// Set _ENV (first upvalue) to the global table.
		// C Lua's lua_load does this after luaD_protectedparser:
		//   if (f->nupvalues >= 1) { setobj(L, f->upvals[0]->v.p, &gt); }
		if len(cl.UpVals) > 0 && cl.UpVals[0] != nil {
			gt := vm.GetGlobalTable(ls)
			cl.UpVals[0].Own = object.TValue{
				Tt:  object.TagTable,
				Obj: gt,
			}
		}
		return StatusOK
	}

	// Text chunk
	if !strings.Contains(mode, "t") {
		L.PushString(fmt.Sprintf("%s: attempt to load a text chunk", name))
		return StatusErrSyntax
	}
	reader := &stringReader{data: code}
	return vm.Load(ls, reader, name)
}

// DoString loads and executes a string.
func (L *State) DoString(code string) error {
	status := L.Load(code, "=(dostring)", "t")
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		return fmt.Errorf("load error: %s", msg)
	}
	status = L.PCall(0, MultiRet, 0)
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		return fmt.Errorf("runtime error: %s", msg)
	}
	return nil
}

// LoadFile loads a Lua file without executing it.
// Pushes the compiled chunk as a function on success.
// Returns a status code (StatusOK on success, or an error code).
func (L *State) LoadFile(filename string, mode string) int {
	data, err := os.ReadFile(filename)
	if err != nil {
		L.PushString(fmt.Sprintf("cannot open %s: %v", filename, err))
		return StatusErrFile
	}
	code := string(data)
	if len(code) > 1 && code[0] == '#' {
		if idx := strings.Index(code, "\n"); idx >= 0 {
			code = code[idx+1:]
		}
	}
	dir := filepath.Dir(filename)
	if dir != "" {
		L.GetGlobal("package")
		if !L.IsNil(-1) {
			L.GetField(-1, "path")
			oldPath, _ := L.ToString(-1)
			L.Pop(1)
			newPath := dir + string(filepath.Separator) + "?.lua;" + oldPath
			L.PushString(newPath)
			L.SetField(-2, "path")
		}
		L.Pop(1)
	}
	source := "@" + filename
	if mode == "" {
		mode = "bt"
	}
	return L.Load(code, source, mode)
}

// DoFile loads and executes a file.
func (L *State) DoFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("cannot open %s: %v", filename, err)
	}
	// Strip shebang line if present
	code := string(data)
	if len(code) > 1 && code[0] == '#' {
		if idx := strings.Index(code, "\n"); idx >= 0 {
			code = code[idx+1:]
		}
	}

	// Prepend script's directory to package.path so require can find
	// sibling .lua modules (e.g., tracegc.lua in the testes directory).
	dir := filepath.Dir(filename)
	if dir != "" {
		L.GetGlobal("package")
		if !L.IsNil(-1) {
			L.GetField(-1, "path")
			oldPath, _ := L.ToString(-1)
			L.Pop(1) // pop old path
			newPath := dir + string(filepath.Separator) + "?.lua;" + oldPath
			L.PushString(newPath)
			L.SetField(-2, "path")
		}
		L.Pop(1) // pop package table
	}

	source := "@" + filename
	status := L.Load(code, source, "t")
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		return fmt.Errorf("load error: %s", msg)
	}
	status = L.PCall(0, MultiRet, 0)
	if status != StatusOK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		return fmt.Errorf("runtime error: %s", msg)
	}
	return nil
}

// Error raises a Lua error with the value at the top of the stack.
func (L *State) Error() int {
	vm.ErrorMsg(L.ls())
	return 0 // unreachable
}

// Dump dumps the function at the top of the stack as a binary chunk.
// If strip is true, debug information is removed.
// Returns the binary chunk bytes, or nil if the value is not a Lua function.
// Mirrors: lua_dump in lapi.c
func (L *State) Dump(strip bool) []byte {
	v := L.index2val(-1)
	if v == nil || v.Tt != object.TagLuaClosure {
		return nil
	}
	cl := v.Obj.(*closure.LClosure)
	return vm.DumpProto(cl.Proto, strip)
}
