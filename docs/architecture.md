# Go Lua VM Architecture Design

## Overview

This document describes the architecture for implementing a Lua 5.x compatible virtual machine in Go. The design is based on the official Lua C implementation (lua-master directory) but adapted to follow Go idioms and best practices.

### Design Goals

1. **Compatibility**: Execute Lua 5.x bytecode and source code correctly
2. **Performance**: Competitive performance with reasonable Go optimizations
3. **Maintainability**: Clean module boundaries, well-tested code
4. **Go Idioms**: Use Go conventions (interfaces, error handling, naming)
5. **Testability**: Each module can be tested independently

---

## Module Directory Structure

```
go-lua/
├── go.mod
├── go.sum
├── docs/
│   └── architecture.md          # This document
├── pkg/
│   ├── api/                     # Public API (lua_State, lua_* functions)
│   │   ├── api.go               # Main API entry points
│   │   ├── stack.go             # Stack manipulation API
│   │   ├── call.go              # Function call API
│   │   └── table.go             # Table API
│   ├── lexer/                   # Lexical analyzer
│   │   ├── lexer.go             # Main lexer struct and methods
│   │   ├── token.go             # Token types and definitions
│   │   ├── keywords.go          # Reserved words
│   │   └── scanner.go           # Character scanning
│   ├── parser/                  # Syntax analyzer
│   │   ├── parser.go            # Main parser struct
│   │   ├── expr.go              # Expression parsing
│   │   ├── stmt.go              # Statement parsing
│   │   ├── func.go              # Function definition parsing
│   │   └── ast.go               # AST node definitions
│   ├── codegen/                 # Code generator
│   │   ├── codegen.go           # Main code generator
│   │   ├── expr.go              # Expression code generation
│   │   ├── stmt.go              # Statement code generation
│   │   └── func.go              # Function prototype generation
│   ├── object/                  # Lua object system
│   │   ├── value.go             # TValue, Value types
│   │   ├── types.go             # Lua type definitions
│   │   ├── gc.go                # Garbage collection interfaces
│   │   └── upvalue.go           # Upvalue structures
│   ├── table/                   # Table implementation
│   │   ├── table.go             # Main table struct
│   │   ├── getset.go            # Get/set operations
│   │   └── iter.go              # Iteration support
│   ├── vm/                      # Virtual machine core
│   │   ├── vm.go                # VM struct and main loop
│   │   ├── opcodes.go           # Instruction definitions
│   │   ├── dispatch.go          # Instruction dispatch
│   │   ├── call.go              # Call/return handling
│   │   └── arith.go             # Arithmetic operations
│   └── state/                   # Global state
│       ├── state.go             # GlobalState struct
│       └── registry.go          # Registry implementation
├── cmd/
│   └── lua/                     # CLI interpreter
│       └── main.go
├── testes/                      # Lua test scripts (copied from lua-master/testes)
└── lua-master/                  # Reference C implementation
```

---

## Core Data Structures

### TValue - Lua Value Representation

In C (lobject.h):
```c
typedef union Value {
  struct GCObject *gc;
  void *p;
  lua_CFunction f;
  lua_Integer i;
  lua_Number n;
  lu_byte ub;
} Value;

typedef struct TValue {
  Value value_;
  lu_byte tt_;
} TValue;
```

In Go (pkg/object/value.go):
```go
// Type represents Lua type tags
type Type uint8

const (
    TypeNil Type = iota
    TypeBoolean
    TypeLightUserData
    TypeNumber
    TypeString
    TypeTable
    TypeFunction
    TypeUserData
    TypeThread
)

// Value holds the actual data of a Lua value
type Value struct {
    // Union-like structure using interface{}
    // Specific types are stored directly for performance
    Num   float64       // For TypeNumber
    Bool  bool          // For TypeBoolean
    Str   string        // For TypeString (interned)
    Int   int64         // For integer numbers
    Ptr   unsafe.Pointer // For light userdata
    GC    GCObject      // For collectable objects
}

// TValue represents a tagged Lua value
type TValue struct {
    Value Value
    Type  Type
}

// Helper methods
func (v *TValue) IsNil() bool
func (v *TValue) IsNumber() bool
func (v *TValue) IsString() bool
func (v *TValue) IsTable() bool
func (v *TValue) IsFunction() bool
func (v *TValue) ToNumber() (float64, bool)
func (v *TValue) ToString() (string, bool)
```

**Design Notes:**
- Go's type system allows us to use a struct with specific fields instead of a union
- We maintain separate `Num` (float64) and `Int` (int64) for Lua's number type
- GC objects are referenced through an interface
- Type tag is explicit (no bit manipulation needed)

### lua_State - Thread State

In C (lstate.h):
```c
typedef struct lua_State {
  CommonHeader;
  unsigned short nci;  /* number of C calls */
  StkId top;  /* first free slot in the stack */
  StkId stack_last;  /* last valid slot in the stack */
  StkId stack;  /* stack base */
  ...
} lua_State;
```

In Go (pkg/vm/vm.go):
```go
// CallInfo tracks function call information
type CallInfo struct {
    Func      *TValue      // Function being called
    Base      int          // Stack base for this call
    Top       int          // Stack top for this call
    PC        int          // Program counter
    NResults  int          // Expected number of results
    Status    CallStatus   // Call status
}

// Prototype represents a Lua function prototype
type Prototype struct {
    Code        []Instruction  // Bytecode
    Constants   []TValue       // Constant table
    Upvalues    []UpvalueDesc  // Upvalue information
    Prototypes  []*Prototype   // Nested prototypes
    Source      string         // Source file name
    LineInfo    []int          // Line number information
    NumParams   int            // Number of parameters
    IsVarArg    bool           // Vararg function
    MaxStackSize int           // Max stack size needed
}

// VM represents a Lua thread/state
type VM struct {
    // Stack
    Stack       []TValue       // Value stack
    StackTop    int            // Current stack top
    StackSize   int            // Current stack size
    
    // Execution state
    PC          int            // Program counter
    Base        int            // Current function base
    
    // Call info
    CallInfo    []*CallInfo    // Call info stack
    CI          int            // Current call info index
    
    // Function
    Prototype   *Prototype     // Current function prototype
    
    // Global state reference
    Global      *GlobalState
    
    // Hook support
    HookFunc    HookFunction
    HookMask    HookMask
    HookCount   int
}
```

### GlobalState - Shared State

In Go (pkg/state/state.go):
```go
// GlobalState holds shared state across all VMs
type GlobalState struct {
    // Memory management
    TotalBytes   int64
    GCState      GCState
    GCThreshold  int64
    
    // String interning
    StringTable  map[string]*GCString
    
    // Registry
    Registry     *Table
    
    // Main thread
    MainThread   *VM
    
    // Allocator
    Allocator    Allocator
    
    // Panic function
    PanicFunc    PanicFunction
    
    // Version
    Version      string
}
```

---

## Bytecode Instruction Set

### Instruction Format

Lua uses 32-bit instructions with the following formats:

```
31       24 23     16 15      8 7       0
+----------+----------+----------+----------+
|    A     |    B     |    C     |  Opcode  |  iABC format
+----------+----------+----------+----------+

31       24 23             6 5       0
+----------+-------------------+----------+
|    A     |       Bx          |  Opcode  |  iABx format
+----------+-------------------+----------+

31       24 23             6 5       0
+----------+-------------------+----------+
|    A     |      sBx          |  Opcode  |  iAsBx format
+----------+-------------------+----------+

31                          6 5       0
+----------------------------+----------+
|            Ax              |  Opcode  |  iAx format
+----------------------------+----------+
```

### Opcode Definitions (pkg/vm/opcodes.go)

```go
// Opcode defines the instruction type
type Opcode uint8

const (
    OP_MOVE      Opcode = 0  // R(A) := R(B)
    OP_LOADI     Opcode = 1  // R(A) := KsBx
    OP_LOADF     Opcode = 2  // R(A) := KsBx (float)
    OP_LOADK     Opcode = 3  // R(A) := K(Bx)
    OP_LOADKX    Opcode = 4  // R(A) := K(extra arg)
    OP_LOADBOOL  Opcode = 5  // R(A) := (Bool)B; if (C) pc++
    OP_LOADNIL   Opcode = 6  // R(A) := R(A+1) := ... := nil
    OP_GETUPVAL  Opcode = 7  // R(A) := UpValue[B]
    OP_SETUPVAL  Opcode = 8  // UpValue[B] := R(A)
    OP_GETTABUP  Opcode = 9  // R(A) := UpValue[B][K(C)]
    OP_GETTABLE  Opcode = 10 // R(A) := R(B)[R(C)]
    OP_GETI      Opcode = 11 // R(A) := R(B)[C]
    OP_GETFIELD  Opcode = 12 // R(A) := R(B)[K(C)]
    OP_SETTABUP  Opcode = 13 // UpValue[A][K(B)] := RK(C)
    OP_SETTABLE  Opcode = 14 // R(A)[R(B)] := RK(C)
    OP_SETI      Opcode = 15 // R(A)[B] := RK(C)
    OP_SETFIELD  Opcode = 16 // R(A)[K(B)] := RK(C)
    OP_NEWTABLE  Opcode = 17 // R(A) := {}
    OP_SELF        Opcode = 18 // R(A+1) := R(B); R(A) := R(B)[RK(C)]
    OP_ADDI        Opcode = 19 // R(A) := R(B) + KsC
    OP_ADD         Opcode = 20 // R(A) := R(B) + R(C)
    OP_SUB         Opcode = 21 // R(A) := R(B) - R(C)
    OP_MUL         Opcode = 22 // R(A) := R(B) * R(C)
    OP_MOD         Opcode = 23 // R(A) := R(B) % R(C)
    OP_POW         Opcode = 24 // R(A) := R(B) ^ R(C)
    OP_DIV         Opcode = 25 // R(A) := R(B) / R(C)
    OP_IDIV        Opcode = 26 // R(A) := R(B) // R(C)
    OP_BAND        Opcode = 27 // R(A) := R(B) & R(C)
    OP_BOR         Opcode = 28 // R(A) := R(B) | R(C)
    OP_BXOR        Opcode = 29 // R(A) := R(B) ~ R(C)
    OP_SHL         Opcode = 30 // R(A) := R(B) << R(C)
    OP_SHR         Opcode = 31 // R(A) := R(B) >> R(C)
    OP_UNM         Opcode = 32 // R(A) := -R(B)
    OP_BNOT        Opcode = 33 // R(A) := ~R(B)
    OP_NOT         Opcode = 34 // R(A) := not R(B)
    OP_LEN         Opcode = 35 // R(A) := length of R(B)
    OP_CONCAT      Opcode = 36 // R(A) := R(B).. ... ..R(C)
    OP_CLOSE       Opcode = 37 // Close upvalues
    OP_TBC         Opcode = 38 // Mark variable as to-be-closed
    OP_JMP         Opcode = 39 // pc+=sBx
    OP_EQ          Opcode = 40 // if ((R(B) == R(C)) ~= A) then pc++
    OP_LT          Opcode = 41 // if ((R(B) <  R(C)) ~= A) then pc++
    OP_LE          Opcode = 42 // if ((R(B) <= R(C)) ~= A) then pc++
    OP_EQI         Opcode = 43 // if ((R(A) == KsC) ~= B) then pc++
    OP_LEI         Opcode = 44 // if ((R(A) <= KsC) ~= B) then pc++
    OP_LTI         Opcode = 45 // if ((R(A) <  KsC) ~= B) then pc++
    OP_GTI         Opcode = 46 // if ((R(A) >  KsC) ~= B) then pc++
    OP_TEST        Opcode = 47 // if not (R(A) ~= B) then pc++
    OP_FORPREP     Opcode = 48 // Prepare numeric for loop
    OP_FORLOOP     Opcode = 49 // Execute numeric for loop
    OP_FORGPREP    Opcode = 50 // Prepare generic for loop
    OP_FORGLOOP    Opcode = 51 // Execute generic for loop
    OP_SETLIST     Opcode = 52 // R(A)[C+i] := R(A+i)
    OP_CLOSURE     Opcode = 53 // R(A) := closure(KPROTO[Bx])
    OP_VARARG      Opcode = 54 // R(A) := R(A+1)..R(A+N-1) (vararg)
    OP_VARARGPREP  Opcode = 55 // Prepare vararg
    OP_EXTRAARG    Opcode = 56 // Extra argument
    // ... more opcodes as needed
)

// Instruction is a 32-bit bytecode instruction
type Instruction uint32

// Instruction decoding helpers
func (i Instruction) Opcode() Opcode
func (i Instruction) A() int
func (i Instruction) B() int
func (i Instruction) C() int
func (i Instruction) Bx() int
func (i Instruction) sBx() int
func (i Instruction) Ax() int
func (i Instruction) RK(index int) *TValue  // Resolve R(K) constant
```

---

## Module Interfaces

### Lexer Module (pkg/lexer)

```go
// TokenType represents lexical token types
type TokenType int

const (
    TK_EOF TokenType = iota
    TK_NAME
    TK_STRING
    TK_INT
    TK_FLOAT
    // ... reserved words
    TK_AND
    TK_OR
    TK_NOT
    // ... operators
    TK_PLUS
    TK_MINUS
)

// Token represents a lexical token
type Token struct {
    Type    TokenType
    Value   interface{}  // string, int64, float64
    Line    int
    Column  int
}

// Lexer performs lexical analysis
type Lexer struct {
    Source    []byte
    Pos       int
    Line      int
    Column    int
    Current   byte
}

// NewLexer creates a new lexer
func NewLexer(source []byte, name string) *Lexer

// NextToken returns the next token
func (l *Lexer) NextToken() (Token, error)

// Peek returns the next token without consuming
func (l *Lexer) Peek() Token
```

### Parser Module (pkg/parser)

```go
// ExprKind represents expression kinds
type ExprKind int

const (
    ExprVoid ExprKind = iota
    ExprNil
    ExprTrue
    ExprFalse
    ExprNumber
    ExprString
    ExprVar
    ExprIndex
    ExprCall
    // ...
)

// Expr represents an AST expression node
type Expr struct {
    Kind     ExprKind
    Value    interface{}
    Line     int
    // ... expression-specific fields
}

// Stmt represents an AST statement node
type Stmt interface {
    stmtNode()
    Line() int
}

// Parser performs syntax analysis
type Parser struct {
    Lexer    *Lexer
    Current  Token
    Peeked   Token
    Errors   []Error
    // ...
}

// NewParser creates a new parser
func NewParser(lexer *Lexer) *Parser

// Parse returns the function prototype
func (p *Parser) Parse() (*Prototype, error)

// ParseExpr parses an expression
func (p *Parser) ParseExpr() *Expr

// ParseStmt parses a statement
func (p *Parser) ParseStmt() Stmt
```

### Code Generator Module (pkg/codegen)

```go
// CodeGenerator generates bytecode from AST
type CodeGenerator struct {
    Prototype  *Prototype
    PC         int
    StackSize  int
    // ...
}

// NewCodeGenerator creates a code generator
func NewCodeGenerator() *CodeGenerator

// Generate generates bytecode for a function
func (cg *CodeGenerator) Generate(ast *FuncDef) *Prototype

// Emit emits an instruction
func (cg *CodeGenerator) Emit(op Opcode, a, b, c int) int

// EmitABC emits iABC format instruction
func (cg *CodeGenerator) EmitABC(op Opcode, a, b, c int)
```

### VM Module (pkg/vm)

```go
// VM executes Lua bytecode
type VM struct {
    // ... (see GlobalState section above)
}

// NewVM creates a new VM instance
func NewVM(global *GlobalState) *VM

// Run starts executing bytecode
func (vm *VM) Run() error

// Call calls a function
func (vm *VM) Call(funcIdx int, nargs, nresults int) error

// GetStack returns the value at stack index
func (vm *VM) GetStack(index int) *TValue

// SetStack sets the value at stack index
func (vm *VM) SetStack(index int, value TValue)

// ExecuteInstruction executes a single instruction
func (vm *VM) ExecuteInstruction(instr Instruction) error
```

### API Module (pkg/api)

```go
// State represents a Lua state (public API)
type State struct {
    vm *vm.VM
}

// NewState creates a new Lua state
func NewState() *State

// Close closes the state
func (s *State) Close()

// PushNil pushes nil onto the stack
func (s *State) PushNil()

// PushBoolean pushes a boolean
func (s *State) PushBoolean(b bool)

// PushNumber pushes a number
func (s *State) PushNumber(n float64)

// PushString pushes a string
func (s *State) PushString(str string)

// PushFunction pushes a Go function
func (s *State) PushFunction(fn Function)

// GetTop returns the stack top index
func (s *State) GetTop() int

// SetTop sets the stack top
func (s *State) SetTop(index int)

// Call calls a function
func (s *State) Call(nargs, nresults int) error

// LoadString loads and compiles a string
func (s *State) LoadString(code, name string) error

// DoString loads and executes a string
func (s *State) DoString(code, name string) error

// Register registers a Go function
func (s *State) Register(name string, fn Function)
```

---

## C to Go Implementation Mapping

| C Module | Go Package | Go Files | Notes |
|----------|-----------|----------|-------|
| llex.c/h | pkg/lexer | lexer.go, token.go, scanner.go | Use Go bufio for I/O |
| lparser.c/h | pkg/parser | parser.go, expr.go, stmt.go | Recursive descent in Go |
| lcode.c/h | pkg/codegen | codegen.go, expr.go, stmt.go | Slice-based code buffer |
| lvm.c/h | pkg/vm | vm.go, dispatch.go, opcodes.go | Interpreted loop |
| ldo.c/h | pkg/vm | call.go | Call/return handling |
| lobject.h | pkg/object | value.go, types.go | Struct-based TValue |
| ltable.c/h | pkg/table | table.go, getset.go | Map + array hybrid |
| lstate.h | pkg/state | state.go | GlobalState struct |
| lapi.c/h | pkg/api | api.go, stack.go | Public API |
| lauxlib.c/h | pkg/api | aux.go | Auxiliary functions |
| lgc.c/h | pkg/object | gc.go | Concurrent GC (Go native) |

---

## Garbage Collection Strategy

Lua uses a generational incremental GC. Go has a concurrent mark-sweep GC built-in.

**Approach:**
1. **Leverage Go's GC**: Use Go's native garbage collector for most objects
2. **Weak References**: Use `sync.Cond` and finalizers for weak tables
3. **Manual Tracking**: Track Lua-specific memory for GC metrics
4. **Finalizers**: Use `runtime.SetFinalizer` for __gc metamethods

```go
// GCObject interface for collectable objects
type GCObject interface {
    gcObject()
}

// GCString is an interned string
type GCString struct {
    Value string
    Hash  uint32
}

// GCTable is a collectable table
type GCTable struct {
    *Table
}

// GCFunction is a collectable function
type GCFunction struct {
    *Prototype
    Upvalues []*TValue
}

// Register finalizer for __gc metamethod
func registerFinalizer(obj GCObject, gcFunc Function) {
    runtime.SetFinalizer(obj, func(o GCObject) {
        // Call __gc metamethod
    })
}
```

---

## Table Implementation

Lua tables are hybrid array-map structures.

```go
// Table represents a Lua table
type Table struct {
    // Array part for integer keys 1..N
    Array []TValue
    
    // Map part for other keys
    Map map[Value]*TValue
    
    // Metatable
    Metatable *Table
    
    // Flags
    IsArray bool  // True if only has array part
    Length  int   // Cached length
}

// NewTable creates a new table
func NewTable(arraySize, mapSize int) *Table

// Get retrieves a value
func (t *Table) Get(key TValue) *TValue

// Set sets a value
func (t *Table) Set(key, value TValue)

// Len returns the length operator result
func (t *Table) Len() int

// Next returns the next key-value pair
func (t *Table) Next(key *TValue) (*TValue, *TValue)
```

---

## Implementation Roadmap

### Phase 1: Core VM (MVP)

**Goal**: Execute simple Lua scripts with basic operations

**Tasks:**
1. **Object System** (pkg/object)
   - TValue implementation
   - Type system
   - Basic GC interfaces

2. **VM Core** (pkg/vm)
   - VM struct and initialization
   - Instruction decoding
   - Basic instruction implementations (MOVE, LOAD, ADD, etc.)
   - Stack management

3. **Table** (pkg/table)
   - Basic table implementation
   - Get/set operations
   - Length operator

4. **API** (pkg/api)
   - Basic lua_State API
   - Stack manipulation
   - Simple function calls

5. **Testing**
   - Unit tests for each module
   - Integration tests with simple scripts

**Acceptance Criteria:**
- Can execute: `print(1 + 2)`
- Can execute: `local x = 10; print(x * 2)`
- Can execute: `local t = {1, 2, 3}; print(#t)`
- All unit tests pass

### Phase 2: Parser and Code Generator

**Goal**: Compile and execute Lua source code

**Tasks:**
1. **Lexer** (pkg/lexer)
   - Token definitions
   - Scanner implementation
   - Number/string parsing

2. **Parser** (pkg/parser)
   - Expression parsing
   - Statement parsing
   - Function definition parsing

3. **Code Generator** (pkg/codegen)
   - Expression code generation
   - Statement code generation
   - Prototype generation

4. **Integration**
   - LoadString implementation
   - DoString implementation

**Acceptance Criteria:**
- Can load and execute Lua source files
- Passes basic Lua test suite tests

### Phase 3: Advanced Features

**Goal**: Full Lua 5.x compatibility

**Tasks:**
1. **Closures and Upvalues**
   - Closure creation
   - Upvalue management
   - OP_CLOSURE implementation

2. **Control Structures**
   - Loops (for, while, repeat)
   - Conditionals (if, elseif, else)
   - Break and return

3. **Functions**
   - First-class functions
   - Vararg functions
   - Tail calls

4. **Metatables**
   - Metatable operations
   - Metamethods
   - Operator overloading

**Acceptance Criteria:**
- Can execute complex Lua programs
- Passes majority of Lua test suite

### Phase 4: Standard Library

**Goal**: Complete standard library implementation

**Tasks:**
1. **Base Library** (print, type, tonumber, etc.)
2. **String Library**
3. **Table Library**
4. **Math Library**
5. **IO Library**
6. **OS Library**
7. **Debug Library** (partial)

**Acceptance Criteria:**
- All standard libraries functional
- Can run real-world Lua scripts

### Phase 5: Optimization and Polish

**Goal**: Performance optimization and production readiness

**Tasks:**
1. **Performance**
   - Profile and optimize hot paths
   - Consider JIT compilation (optional)
   
2. **Error Handling**
   - Better error messages
   - Stack traces
   
3. **Documentation**
   - API documentation
   - Usage examples
   
4. **Testing**
   - Full test suite coverage
   - Benchmark tests

---

## Error Handling

Go's error handling vs Lua's error handling:

```go
// Internal errors use Go error type
func (vm *VM) ExecuteInstruction(instr Instruction) error {
    if vm.PC >= len(vm.Prototype.Code) {
        return errors.New("attempt to execute beyond end of code")
    }
    // ...
}

// Lua errors use panic/recover for long jumps
func (vm *VM) RaiseError(format string, args ...interface{}) {
    msg := fmt.Sprintf(format, args...)
    panic(&LuaError{
        Message: msg,
        Stack:   vm.captureStack(),
    })
}

// API catches and converts
func (s *State) DoString(code, name string) (err error) {
    defer func() {
        if r := recover(); r != nil {
            if le, ok := r.(*LuaError); ok {
                err = le
            } else {
                err = fmt.Errorf("panic: %v", r)
            }
        }
    }()
    // ...
}
```

---

## Concurrency Considerations

**Current Design**: Single-threaded VM (like Lua C)

**Future Extensions:**
1. **Multiple VMs**: Each goroutine can have its own VM
2. **Channel-based Communication**: Use Go channels for inter-VM communication
3. **Coroutines**: Lua coroutines map well to Go goroutines (future optimization)

```go
// Thread-safe GlobalState access
type GlobalState struct {
    mu sync.RWMutex
    // ...
}

// VM is NOT thread-safe (like Lua)
// Each goroutine should have its own VM
```

---

## Testing Strategy

### Unit Tests
```go
// pkg/object/value_test.go
func TestTValue_IsNil(t *testing.T) {
    v := TValue{Type: TypeNil}
    if !v.IsNil() {
        t.Error("Expected TValue to be nil")
    }
}

// pkg/vm/opcodes_test.go
func TestInstruction_Decode(t *testing.T) {
    instr := MakeABC(OP_ADD, 1, 2, 3)
    if instr.Opcode() != OP_ADD {
        t.Error("Opcode mismatch")
    }
}
```

### Integration Tests
```go
// integration_test.go
func TestSimpleScript(t *testing.T) {
    L := NewState()
    defer L.Close()
    
    err := L.DoString(`
        local x = 10
        local y = 20
        assert(x + y == 30)
    `, "test")
    
    if err != nil {
        t.Fatal(err)
    }
}
```

### Lua Test Suite
```bash
# Run official Lua tests
go test -run TestLuaSuite ./...
```

---

## Performance Considerations

1. **Stack Allocation**: Use slice for VM stack (avoid pointer chasing)
2. **Instruction Dispatch**: Use direct threading or computed goto pattern
3. **String Interning**: Maintain string table to avoid duplicates
4. **Table Optimization**: Hybrid array/map for common access patterns
5. **Avoid Interfaces**: Use concrete types in hot paths
6. **Inline Functions**: Use Go's linkname or manual inlining for hot functions

---

## References

- [Lua 5.4 Reference Manual](https://www.lua.org/manual/5.4/)
- [Lua 5.4 Source Code](lua-master directory)
- [Go Language Specification](https://go.dev/ref/spec)

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-03-25 | Trunk Node | Initial architecture design |