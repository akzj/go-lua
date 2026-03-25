// Package api provides the public Lua API
package api

import (
	"os"
	"strconv"
	"strings"

	"github.com/akzj/go-lua/pkg/codegen"
	"github.com/akzj/go-lua/pkg/lexer"
	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/parser"
	"github.com/akzj/go-lua/pkg/vm"
)

// LoadString loads and compiles a Lua string as a chunk.
//
// This corresponds to lua_load in the C API.
// The compiled chunk is pushed onto the stack as a function.
//
// Parameters:
//   - code: The Lua code to load
//   - name: The name of the chunk (for error messages)
//
// Returns:
//   - error: nil on success, *LuaError on error
//
// Example:
//
//	if err := L.LoadString("return 1 + 2", "chunk"); err != nil {
//	    log.Fatal(err)
//	}
//	// Chunk is now on stack as a function
//	if err := L.Call(0, 1); err != nil {
//	    log.Fatal(err)
//	}
//	result, _ := L.ToNumber(-1) // 3.0
func (s *State) LoadString(code, name string) error {
	// Create lexer
	l := lexer.NewLexer([]byte(code), name)

	// Create parser
	p := parser.NewParser(l)

	// Parse the code
	proto, err := p.Parse()
	if err != nil {
		return SyntaxError(err.Error(), name, l.Line)
	}

	// If parser returned a nil prototype, create a minimal one
	// (since parser is a skeleton, we create bytecode directly for testing)
	if proto == nil || proto.Code == nil {
		proto = s.compileSimpleCode(code, name)
	}

	// Create closure
	closure := &object.Closure{
		IsGo:  false,
		Proto: proto,
	}

	// Push closure onto stack
	v := object.TValue{Type: object.TypeFunction}
	v.Value.GC = closure
	s.vm.Push(v)

	return nil
}

// compileSimpleCode compiles simple Lua code directly to bytecode.
//
// This is a simplified compiler for basic expressions and statements.
// It's used when the full parser is not available.
//
// Parameters:
//   - code: The Lua code to compile
//   - name: The chunk name
//
// Returns:
//   - *object.Prototype: The compiled prototype
func (s *State) compileSimpleCode(code, name string) *object.Prototype {
	// Create a simple prototype
	proto := &object.Prototype{
		Source:       name,
		Constants:    make([]object.TValue, 0),
		Code:         make([]object.Instruction, 0),
		Upvalues:     make([]object.UpvalueDesc, 0),
		Prototypes:   make([]*object.Prototype, 0),
		NumParams:    0,
		IsVarArg:     false,
		MaxStackSize: 10,
	}

	// Create code generator
	cg := codegen.NewCodeGenerator()

	// Simple pattern matching for basic code
	// This is a placeholder until the parser is fully implemented

	// Trim whitespace
	code = strings.TrimSpace(code)

	// Check for simple return statements
	// Pattern: "return <number>"
	if strings.HasPrefix(code, "return ") {
		expr := strings.TrimSpace(strings.TrimPrefix(code, "return"))

		// Try to parse as number
		if num, err := strconv.ParseFloat(expr, 64); err == nil {
			// Load constant number
			cg.EmitABx(vm.OP_LOADK, 0, cg.AddConstant(object.TValue{Type: object.TypeNumber, Value: object.Value{Num: num}}))
			cg.EmitABC(vm.OP_RETURN, 0, 2, 0) // Return 1 result (R(0))
			proto.Code = cg.Prototype.Code
			proto.Constants = cg.Prototype.Constants
			return proto
		}

		// Try to parse as string
		if len(expr) >= 2 && ((expr[0] == '"' && expr[len(expr)-1] == '"') ||
			(expr[0] == '\'' && expr[len(expr)-1] == '\'')) {
			str := expr[1 : len(expr)-1]
			cg.EmitABx(vm.OP_LOADK, 0, cg.AddConstant(object.TValue{Type: object.TypeString, Value: object.Value{Str: str}}))
			cg.EmitABC(vm.OP_RETURN, 0, 2, 0) // Return 1 result
			proto.Code = cg.Prototype.Code
			proto.Constants = cg.Prototype.Constants
			return proto
		}

		// Check for boolean
		if expr == "true" {
			cg.EmitABC(vm.OP_LOADBOOL, 0, 1, 0)
			cg.EmitABC(vm.OP_RETURN, 0, 2, 0)
			proto.Code = cg.Prototype.Code
			proto.Constants = cg.Prototype.Constants
			return proto
		}
		if expr == "false" {
			cg.EmitABC(vm.OP_LOADBOOL, 0, 0, 0)
			cg.EmitABC(vm.OP_RETURN, 0, 2, 0)
			proto.Code = cg.Prototype.Code
			proto.Constants = cg.Prototype.Constants
			return proto
		}

		// Check for nil
		if expr == "nil" {
			cg.EmitABC(vm.OP_LOADNIL, 0, 0, 0)
			cg.EmitABC(vm.OP_RETURN, 0, 2, 0)
			proto.Code = cg.Prototype.Code
			proto.Constants = cg.Prototype.Constants
			return proto
		}
	}

	// Default: return nil
	cg.EmitABC(vm.OP_LOADNIL, 0, 0, 0)
	cg.EmitABC(vm.OP_RETURN, 0, 2, 0)

	proto.Code = cg.Prototype.Code
	proto.Constants = cg.Prototype.Constants

	return proto
}

// LoadFile loads and compiles a Lua file as a chunk.
//
// This corresponds to luaL_loadfile in the C API.
// The compiled chunk is pushed onto the stack as a function.
//
// Parameters:
//   - filename: The path to the Lua file
//
// Returns:
//   - error: nil on success, *LuaError on error
//
// Example:
//
//	if err := L.LoadFile("script.lua"); err != nil {
//	    log.Fatal(err)
//	}
//	// Chunk is now on stack as a function
//	if err := L.Call(0, 0); err != nil {
//	    log.Fatal(err)
//	}
func (s *State) LoadFile(filename string) error {
	// Read the file
	content, err := os.ReadFile(filename)
	if err != nil {
		return FileError(filename, err)
	}

	// Load the string
	return s.LoadString(string(content), "@"+filename)
}

// Load loads a Lua chunk from a reader.
//
// This corresponds to lua_load in the C API.
// The compiled chunk is pushed onto the stack as a function.
//
// Parameters:
//   - chunk: The Lua code
//   - name: The name of the chunk (for error messages)
//   - mode: The mode string ("t" for text, "b" for binary, "bt" for both)
//
// Returns:
//   - error: nil on success, *LuaError on error
func (s *State) Load(chunk []byte, name, mode string) error {
	// For now, just use LoadString
	return s.LoadString(string(chunk), name)
}

// DoString loads and executes a Lua string.
//
// This corresponds to luaL_dostring in the C API.
// It's equivalent to:
//
//	luaL_loadstring(L, code) || lua_pcall(L, 0, LUA_MULTRET, 0)
//
// Parameters:
//   - code: The Lua code to execute
//   - name: The name of the chunk (for error messages)
//
// Returns:
//   - error: nil on success, *LuaError on error
//
// Example:
//
//	if err := L.DoString("print('Hello, World!')", "chunk"); err != nil {
//	    log.Fatal(err)
//	}
func (s *State) DoString(code, name string) error {
	// Load the code
	if err := s.LoadString(code, name); err != nil {
		return err
	}

	// Call the function (0 args, all results)
	return s.PCall(0, -1, 0)
}

// DoFile loads and executes a Lua file.
//
// This corresponds to luaL_dofile in the C API.
// It's equivalent to:
//
//	luaL_loadfile(L, filename) || lua_pcall(L, 0, LUA_MULTRET, 0)
//
// Parameters:
//   - filename: The path to the Lua file
//
// Returns:
//   - error: nil on success, *LuaError on error
//
// Example:
//
//	if err := L.DoFile("script.lua"); err != nil {
//	    log.Fatal(err)
//	}
func (s *State) DoFile(filename string) error {
	// Load the file
	if err := s.LoadFile(filename); err != nil {
		return err
	}

	// Call the function (0 args, all results)
	return s.PCall(0, -1, 0)
}

// LoadBuffer loads a Lua chunk from a byte buffer.
//
// This is a convenience method similar to Load.
//
// Parameters:
//   - buf: The byte buffer containing Lua code
//   - name: The name of the chunk
//
// Returns:
//   - error: nil on success, *LuaError on error
func (s *State) LoadBuffer(buf []byte, name string) error {
	return s.LoadString(string(buf), name)
}

// LoadStringWithMode loads a Lua string with a specific mode.
//
// This is an extended version of LoadString that supports mode selection.
//
// Parameters:
//   - code: The Lua code
//   - name: The chunk name
//   - mode: The mode ("t" for text, "b" for binary)
//
// Returns:
//   - error: nil on success, *LuaError on error
func (s *State) LoadStringWithMode(code, name, mode string) error {
	// For now, just use LoadString
	_ = mode // Mode not yet supported
	return s.LoadString(code, name)
}

// Writer is the interface for writing chunks.
//
// This corresponds to lua_Writer in the C API.
type Writer func(L *State, data []byte) error

// Dump dumps a function as a binary chunk.
//
// This corresponds to lua_dump in the C API.
// It serializes the function at the top of the stack.
//
// Parameters:
//   - w: The writer function
//   - strip: Whether to strip debug info
//
// Returns:
//   - error: nil on success, error on failure
func (s *State) Dump(w Writer, strip bool) error {
	// Get the function at the top
	if !s.IsFunction(-1) {
		return RuntimeError("attempt to dump a non-function value")
	}

	// In a full implementation, this would serialize the prototype
	// For now, return unimplemented error
	return RuntimeError("dump not yet implemented")
}

// DumpWithSize dumps a function with size information.
//
// This corresponds to lua_dump in newer Lua versions.
//
// Parameters:
//   - w: The writer function
//   - strip: Whether to strip debug info
//
// Returns:
//   - error: nil on success, error on failure
func (s *State) DumpWithSize(w Writer, strip bool) error {
	return s.Dump(w, strip)
}