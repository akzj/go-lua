// Package api provides the public Lua API
package api

import (
	"os"

	"github.com/akzj/go-lua/pkg/codegen"
	"github.com/akzj/go-lua/pkg/lexer"
	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/parser"
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

	// Parse the code into AST
	block, err := p.ParseChunk()
	if err != nil {
		return SyntaxError(err.Error(), name, l.Line())
	}

	// Compile AST to bytecode
	proto := codegen.CompileChunk(block, name)

	// Create closure
	closure := &object.Closure{
		IsGo:  false,
		Proto: proto,
	}

	// Set up _ENV as upvalue[0] pointing to the global table
	globalTable := s.getGlobalTable()
	globalTValue := &object.TValue{Type: object.TypeTable}
	globalTValue.Value.GC = globalTable
	closure.Upvalues = []*object.Upvalue{
		{Value: globalTValue, Closed: false},
	}

	// Push closure onto stack
	v := object.TValue{Type: object.TypeFunction}
	v.Value.GC = closure
	s.vm.Push(v)

	return nil
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