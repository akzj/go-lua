// Package api defines the Lua Abstract Syntax Tree interface.
// NO dependencies - pure interface definitions.
package api

import (
	lexapi "github.com/akzj/go-lua/lex/api"
	typesapi "github.com/akzj/go-lua/types/api"
)

// =============================================================================
// Base Interfaces
// =============================================================================

// Node is the base interface for all AST nodes.
// All nodes must implement Position() to support source mapping.
type Node interface {
	// Position returns the source position of this node.
	// Why not embed Token? Some nodes (like blocks) span multiple tokens.
	Position() (line int, column int)
}

// =============================================================================
// Expression Nodes
// =============================================================================

// ExpNode is the base interface for all expression nodes.
// Expressions produce values and can appear where values are expected.
type ExpNode interface {
	Node
	// IsConstant returns true if this is a compile-time constant.
	// Why not just check specific types? Parser needs uniform check.
	IsConstant() bool
	// Kind returns the expression kind for code generation.
	Kind() ExpKind
}

// =============================================================================
// Expression Kinds (matching lua-master/lparser.h expkind enum)
// =============================================================================

// ExpKind represents the kind of an expression.
// Why not just use interface type switches? Runtime type checks are expensive.
// These constants allow fast comparison during code generation.
type ExpKind int

const (
	EXP_VOID ExpKind = iota // VVOID - empty list marker
	EXP_NIL                  // VNIL - constant nil
	EXP_TRUE                 // VTRUE - constant true
	EXP_FALSE                // VFALSE - constant false
	EXP_K                    // VK - constant in constant table
	EXP_KINTEGER             // VKINT - integer constant
	EXP_KFLOAT               // VKFLT - float constant
	EXP_KSTRING              // VKSTR - string constant
	EXP_NONRELOC             // VNONRELOC - expression in fixed register
	EXP_LOCAL                // VLOCAL - local variable
	EXP_VARARG               // VVARGVAR - vararg parameter
	EXP_GLOBAL               // VGLOBAL - global variable
	EXP_UPVAL                // VUPVAL - upvalue variable
	EXP_CONST                // VCONST - compile-time constant
	EXP_INDEXED              // VINDEXED - indexed variable
	EXP_VARARG_IND           // VVARGIND - indexed vararg
	EXP_INDEX_UPVAL          // VINDEXUP - indexed upvalue
	EXP_INDEX_INT            // VINDEXI - indexed with constant integer
	EXP_INDEX_STR            // VINDEXSTR - indexed with literal string
	EXP_JMP                  // VJMP - test/comparison jump
	EXP_RELOC                // VRELOC - relocatable expression
	EXP_CALL                 // VCALL - function call
	EXP_VARARG_EXP           // VVARARG - vararg expression
)

// ExpDesc describes a potentially delayed expression.
// Similar to lua-master's expdesc structure.
// Why separate from ExpNode? Runtime expression description needs
// patch lists for short-circuit evaluation (and/or).
type ExpDesc interface {
	ExpNode
	Kind() ExpKind
	SetKind(ExpKind)
	// Reg returns the register index (for VLOCAL, VNONRELOC, etc.)
	Reg() int
	SetReg(int)
	// Info returns generic info field (constant index, etc.)
	Info() int
	SetInfo(int)
	// Table returns table register and key register for VINDEXED.
	// For VINDEXUP: tableReg is upval index, keyReg is key's K index.
	// For VINDEXI: keyReg is the constant integer value.
	// For VINDEXSTR: keyReg is the key's K index.
	Table() (tableReg int, keyReg int)
	SetTable(tableReg, keyReg int)
	// KeyIsString returns true if the key is a string constant.
	// Used for VINDEXED, VINDEXSTR to distinguish from integer keys.
	KeyIsString() bool
	SetKeyIsString(bool)
	// TrueJump returns the patch list for "exit when true".
	TrueJump() int
	SetTrueJump(int)
	// FalseJump returns the patch list for "exit when false".
	FalseJump() int
	SetFalseJump(int)
}

// =============================================================================
// Statement Nodes
// =============================================================================

// StatNode is the base interface for all statement nodes.
// Statements are executable units that don't produce values.
type StatNode interface {
	Node
	// IsScopeEnd returns true if this statement ends a scope.
	// Used for local variable lifetime tracking.
	IsScopeEnd() bool
	// Kind returns the statement kind for code generation.
	Kind() StatKind
}

// =============================================================================
// Statement Kinds
// =============================================================================

// StatKind represents the kind of a statement.
type StatKind int

const (
	STAT_ASSIGN StatKind = iota // Assignment statement
	STAT_LOCAL_VAR               // Local variable declaration
	STAT_LOCAL_FUNC              // Local function declaration
	STAT_GLOBAL_FUNC             // Global function declaration
	STAT_GLOBAL_VAR              // Global variable declaration (Lua 5.4)
	STAT_IF                      // If statement
	STAT_WHILE                   // While statement
	STAT_REPEAT                  // Repeat until statement
	STAT_FOR_IN                  // For-in statement
	STAT_FOR_NUM                 // Numeric for statement
	STAT_RETURN                  // Return statement
	STAT_BREAK                   // Break statement
	STAT_GOTO                    // Goto statement
	STAT_LABEL                   // Label statement
	STAT_CALL                    // Expression statement (function call)
	STAT_EMPTY                   // Empty statement (semicolon)
)

// =============================================================================
// Variable Descriptor (matching lua-master Vardesc)
// =============================================================================

// VarDesc describes an active variable.
type VarDesc interface {
	// Name returns the variable name.
	Name() string
	// IsLocal returns true if this is a local variable.
	IsLocal() bool
	// IsGlobal returns true if this is a global variable.
	IsGlobal() bool
	// Reg returns the register holding this variable.
	Reg() int
	// Index returns the variable's index in actvar array.
	Index() int
}

// =============================================================================
// Label Descriptor (matching lua-master Labeldesc)
// =============================================================================

// LabelDesc describes a label statement.
type LabelDesc interface {
	// Name returns the label identifier.
	Name() string
	// Line returns the line where label appeared.
	Line() int
}

// =============================================================================
// Function Definition
// =============================================================================

// FuncDef represents a function definition.
// Why separate from FunctionExp? FuncDef can appear in multiple contexts.
type FuncDef interface {
	// IsLocal returns true if this is a local function.
	IsLocal() bool
	// Line returns the line of the 'function' keyword.
	Line() int
	// LastLine returns the line of the 'end' keyword.
	LastLine() int
	// Proto returns the function prototype.
	// Why return typesapi.Proto? Function body compilation produces Proto.
	Proto() *typesapi.Proto
}

// =============================================================================
// Table Constructor
// =============================================================================

// TableConstructor represents a table constructor expression.
type TableConstructor interface {
	ExpNode
	// NumFields returns the number of array fields.
	NumFields() int
	// NumRecords returns the number of record fields.
	NumRecords() int
	// AddArrayField adds an array field.
	AddArrayField(ExpNode)
	// AddRecordField adds a record field (key, value).
	AddRecordField(key, value ExpNode)
}

// =============================================================================
// Function Call
// =============================================================================

// FuncCall represents a function call expression.
type FuncCall interface {
	ExpNode
	// Func returns the function being called.
	Func() ExpNode
	// Args returns the call arguments.
	Args() []ExpNode
	// NumResults is the expected number of return values.
	NumResults() int
}

// =============================================================================
// Binary/Unary Operators
// =============================================================================

// BinopKind represents binary operators.
type BinopKind int

const (
	BINOP_ADD BinopKind = iota //
	BINOP_SUB                   //
	BINOP_MUL                   //
	BINOP_DIV                   //
	BINOP_IDIV                  //
	BINOP_MOD                   //
	BINOP_POW                   //
	BINOP_AND                   //
	BINOP_OR                    //
	BINOP_LT                    //
	BINOP_GT                    //
	BINOP_LE                    //
	BINOP_GE                    //
	BINOP_NE                    //
	BINOP_EQ                    //
	BINOP_SHL                   // << shift left
	BINOP_SHR                   // >> shift right
	BINOP_BAND                  // & bitwise and
	BINOP_BOR                   // | bitwise or
	BINOP_BXOR                  // ~ bitwise xor
	BINOP_CONCAT                //
)

// UnopKind represents unary operators.
type UnopKind int

const (
	UNOP_NEG UnopKind = iota //
	UNOP_NOT                  //
	UNOP_BNOT                 //
	UNOP_LEN                  //
)

// =============================================================================
// Chunk / Block
// =============================================================================

// Chunk represents a complete Lua chunk (file or string).
type Chunk interface {
	// Block returns the chunk's block.
	Block() Block
	// SourceName returns the source name.
	SourceName() string
}

// Block represents a sequence of statements.
type Block interface {
	Node
	// Stats returns all statements in this block.
	Stats() []StatNode
	// ReturnExp returns the return expression list (nil if no return).
	ReturnExp() []ExpNode
}

// =============================================================================
// Local Variable List (matching lua-master Dyndata.actvar)
// =============================================================================

// LocalVars maintains the list of active local variables.
type LocalVars interface {
	// Add adds a new local variable.
	Add(name string, reg int)
	// Get returns the variable at the given index.
	Get(index int) VarDesc
	// Count returns the number of active local variables.
	Count() int
	// Reset clears all variables.
	Reset()
}

// =============================================================================
// Goto/Label List (matching lua-master Labellist)
// =============================================================================

// GotoList maintains pending goto statements.
type GotoList interface {
	// Add adds a goto statement.
	Add(name string, pc, line, nactvar int, close bool)
	// Get returns the goto at the given index.
	Get(index int) LabelDesc
	// Count returns the number of pending gotos.
	Count() int
}

// LabelList maintains active labels.
type LabelList interface {
	// Add adds a label.
	Add(name string, pc, line, nactvar int)
	// Get returns the label at the given index.
	Get(index int) LabelDesc
	// Count returns the number of active labels.
	Count() int
}

// =============================================================================
// Parser Interface
// =============================================================================

// Parser parses Lua source code into an AST.
type Parser interface {
	// ParseChunk parses a complete Lua chunk.
	ParseChunk(source string) (Chunk, error)
	// ParseExpression parses a single expression.
	// Used for -e flag and loadstring.
	ParseExpression(source string) (ExpNode, error)
}

// =============================================================================
// Token Helper
// =============================================================================

// HasToken provides token access for position reporting.
type HasToken interface {
	// Token returns the first token of this node.
	Token() lexapi.Token
}
