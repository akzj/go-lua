package codegen

import (
	"fmt"
	"github.com/akzj/go-lua/pkg/object"
	"github.com/akzj/go-lua/pkg/parser"
	"github.com/akzj/go-lua/pkg/vm"
)

// CompileChunk compiles a parsed block (main chunk) into a Prototype.
// This is the main entry point called from pkg/api/load.go after parsing.
//
// The main chunk is always a vararg function with _ENV as upvalue[0].
//
// Parameters:
//   - block: The parsed block of statements
//   - source: Source name for debug info
//
// Returns:
//   - *object.Prototype: The compiled prototype with bytecode
//   - error: Compilation error if any
func CompileChunk(block *parser.BlockStmt, source string) (*object.Prototype, error) {
	cg := NewCodeGenerator()
	cg.Prototype.Source = source
	cg.Prototype.IsVarArg = true
	cg.Prototype.NumParams = 0

	// Register _ENV as upvalue index 0
	// _ENV is the global environment table, always at upvalue index 0 for the main chunk
	cg.Upvalues["_ENV"] = 0
	cg.Prototype.Upvalues = append(cg.Prototype.Upvalues, object.UpvalueDesc{
		Index:   0,
		IsLocal: true, // _ENV comes from the "enclosing" scope (set by LoadString)
	})

	// Emit VARARGPREP for the main chunk (0 fixed params)
	cg.EmitABC(vm.OP_VARARGPREP, 0, 0, 0)

	// Generate code for the block
	cg.beginScope()
	cg.genBlock(block)

	// Check for compilation errors
	if cg.hasError() {
		return nil, cg.getError()
	}

	// Check for unresolved forward gotos (labels that were never defined)
	for labelName, gotos := range cg.forwardGotos {
		if len(gotos) > 0 {
			// Use the first goto's line number for the error
			line := gotos[0].Line
			return nil, &CompileError{Message: fmt.Sprintf("%s:%d: no visible label '%s' for <goto>", source, line, labelName)}
		}
	}

	// Emit trailing RETURN 0, 1 (return no values) if not already returned
	cg.emitReturn(0, 1)
	cg.endScope()
	
	// Check for errors again (endScope may have detected scope violations)
	if cg.hasError() {
		return nil, cg.getError()
	}

	// Finalize
	if cg.MaxStackSize < 2 {
		cg.MaxStackSize = 2 // Minimum stack size
	}
	cg.Prototype.MaxStackSize = cg.MaxStackSize

	return cg.Prototype, nil
}

// getUpvalue looks up an upvalue by name.
// Returns the upvalue index and true if found, or -1 and false.
func (cg *CodeGenerator) getUpvalue(name string) (int, bool) {
	idx, ok := cg.Upvalues[name]
	return idx, ok
}

// addUpvalue adds an upvalue and returns its index.
func (cg *CodeGenerator) addUpvalue(name string, desc object.UpvalueDesc) int {
	if idx, ok := cg.Upvalues[name]; ok {
		return idx
	}
	idx := len(cg.Prototype.Upvalues)
	cg.Upvalues[name] = idx
	cg.Prototype.Upvalues = append(cg.Prototype.Upvalues, desc)
	return idx
}
