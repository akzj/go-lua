// Package parser implements the Lua syntax analyzer.
//
// This package provides a recursive descent parser with precedence climbing
// for expressions. It follows the semantics from lua-master/lparser.c.
//
// # Parsing Strategy
//
// The parser uses recursive descent for statements and precedence climbing
// for expressions. Each statement type has its own parsing function.
//
// # AST Structure
//
// The AST consists of two main interfaces:
//   - Expr: All expression nodes
//   - Stmt: All statement nodes
//
// # Example Usage
//
//	lexer := lexer.NewLexer(source, "chunk.lua")
//	parser := parser.NewParser(lexer)
//	proto, err := parser.Parse()
package parser

import (
	"github.com/akzj/go-lua/pkg/object"
)

// ============================================================================
// Expression AST Nodes
// ============================================================================

// ExprKind represents the kind of an expression.
type ExprKind int

const (
	// ExprVoid represents an empty expression (no value).
	ExprVoid ExprKind = iota

	// ExprNil represents the nil literal.
	ExprNil

	// ExprTrue represents the true literal.
	ExprTrue

	// ExprFalse represents the false literal.
	ExprFalse

	// ExprNumber represents a numeric literal.
	ExprNumber

	// ExprString represents a string literal.
	ExprString

	// ExprVar represents a variable reference.
	ExprVar

	// ExprIndex represents an indexed access (table[key]).
	ExprIndex

	// ExprField represents a field access (table.field).
	ExprField

	// ExprCall represents a function call.
	ExprCall

	// ExprMethodCall represents a method call (obj:method(args)).
	ExprMethodCall

	// ExprBinOp represents a binary operation.
	ExprBinOp

	// ExprUnOp represents a unary operation.
	ExprUnOp

	// ExprTable represents a table constructor.
	ExprTable

	// ExprFunc represents an anonymous function.
	ExprFunc

	// ExprDots represents the vararg expression (...).
	ExprDots

	// ExprParen represents a parenthesized expression.
	ExprParen
)

// Expr is the interface for all expression nodes.
type Expr interface {
	exprNode()
	Line() int
}

// baseExpr provides common functionality for expression nodes.
type baseExpr struct {
	line int
}

func (e *baseExpr) Line() int { return e.line }

// NilExpr represents the nil literal.
type NilExpr struct {
	baseExpr
}

func (e *NilExpr) exprNode() {}

// BooleanExpr represents a boolean literal (true/false).
type BooleanExpr struct {
	baseExpr
	Value bool
}

func (e *BooleanExpr) exprNode() {}

// NumberExpr represents a numeric literal.
type NumberExpr struct {
	baseExpr
	Value float64
	Int   int64
	IsInt bool
}

func (e *NumberExpr) exprNode() {}

// StringExpr represents a string literal.
type StringExpr struct {
	baseExpr
	Value string
}

func (e *StringExpr) exprNode() {}

// VarExpr represents a variable reference.
type VarExpr struct {
	baseExpr
	Name string
}

func (e *VarExpr) exprNode() {}

// IndexExpr represents an indexed access: table[key].
type IndexExpr struct {
	baseExpr
	Table Expr
	Index Expr
}

func (e *IndexExpr) exprNode() {}

// FieldExpr represents a field access: table.field.
type FieldExpr struct {
	baseExpr
	Table Expr
	Field string
}

func (e *FieldExpr) exprNode() {}

// CallExpr represents a function call: func(args).
type CallExpr struct {
	baseExpr
	Func Expr
	Args []Expr
}

func (e *CallExpr) exprNode() {}

// MethodCallExpr represents a method call: obj:method(args).
type MethodCallExpr struct {
	baseExpr
	Object Expr
	Method string
	Args   []Expr
}

func (e *MethodCallExpr) exprNode() {}

// BinOpExpr represents a binary operation.
type BinOpExpr struct {
	baseExpr
	Left  Expr
	Op    string
	Right Expr
}

func (e *BinOpExpr) exprNode() {}

// UnOpExpr represents a unary operation.
type UnOpExpr struct {
	baseExpr
	Op   string
	Expr Expr
}

func (e *UnOpExpr) exprNode() {}

// TableExpr represents a table constructor.
type TableExpr struct {
	baseExpr
	Entries []TableEntry
}

func (e *TableExpr) exprNode() {}

// TableEntryKind represents the kind of table entry.
type TableEntryKind int

const (
	// TableEntryField represents a field entry: key = value.
	TableEntryField TableEntryKind = iota

	// TableEntryIndex represents an indexed entry: [key] = value.
	TableEntryIndex

	// TableEntryValue represents a value-only entry: value.
	TableEntryValue

	// TableEntryKey represents a key-value pair with explicit key expression.
	TableEntryKey
)

// TableEntry represents an entry in a table constructor.
type TableEntry struct {
	Kind  TableEntryKind
	Key   Expr  // For Field and Index entries
	Value Expr
}

// FuncExpr represents an anonymous function.
type FuncExpr struct {
	baseExpr
	Params []*VarExpr
	Body *BlockStmt
	IsVarArg bool
}

func (e *FuncExpr) exprNode() {}

// DotsExpr represents the vararg expression (...).
type DotsExpr struct {
	baseExpr
}

func (e *DotsExpr) exprNode() {}

// ParenExpr represents a parenthesized expression.
type ParenExpr struct {
	baseExpr
	Expr Expr
}

func (e *ParenExpr) exprNode() {}

// ============================================================================
// Statement AST Nodes
// ============================================================================

// Stmt is the interface for all statement nodes.
type Stmt interface {
	stmtNode()
	Line() int
}

// baseStmt provides common functionality for statement nodes.
type baseStmt struct {
	line int
}

func (s *baseStmt) Line() int { return s.line }

// BlockStmt represents a block of statements.
type BlockStmt struct {
	baseStmt
	Stmts []Stmt
}

func (s *BlockStmt) stmtNode() {}

// AssignStmt represents an assignment statement.
type AssignStmt struct {
	baseStmt
	Left  []Expr
	Right []Expr
}

func (s *AssignStmt) stmtNode() {}

// LocalStmt represents a local variable declaration.
type LocalStmt struct {
	baseStmt
	Names []*VarExpr
	Attrs []string // Optional attributes (e.g., "const")
	Values []Expr
}

func (s *LocalStmt) stmtNode() {}

// ElseIfClause represents an elseif clause in an if statement.
type ElseIfClause struct {
	Cond Expr
	Then *BlockStmt
}

// IfStmt represents an if-then-elseif-else-end statement.
type IfStmt struct {
	baseStmt
	Cond   Expr
	Then   *BlockStmt
	ElseIf []ElseIfClause
	Else   *BlockStmt
}

func (s *IfStmt) stmtNode() {}

// WhileStmt represents a while-do-end loop.
type WhileStmt struct {
	baseStmt
	Cond Expr
	Body *BlockStmt
}

func (s *WhileStmt) stmtNode() {}

// RepeatStmt represents a repeat-until loop.
type RepeatStmt struct {
	baseStmt
	Body *BlockStmt
	Cond Expr
}

func (s *RepeatStmt) stmtNode() {}

// ForNumericStmt represents a numeric for loop: for i = start, end, step do ... end.
type ForNumericStmt struct {
	baseStmt
	Var  *VarExpr
	From Expr
	To   Expr
	Step Expr // Optional
	Body *BlockStmt
}

func (s *ForNumericStmt) stmtNode() {}

// ForGenericStmt represents a generic for loop: for k, v in pairs(t) do ... end.
type ForGenericStmt struct {
	baseStmt
	Vars  []*VarExpr
	Exprs []Expr
	Body  *BlockStmt
}

func (s *ForGenericStmt) stmtNode() {}

// BreakStmt represents a break statement.
type BreakStmt struct {
	baseStmt
}

func (s *BreakStmt) stmtNode() {}

// ReturnStmt represents a return statement.
type ReturnStmt struct {
	baseStmt
	Values []Expr
}

func (s *ReturnStmt) stmtNode() {}

// GotoStmt represents a goto statement.
type GotoStmt struct {
	baseStmt
	Label string
}

func (s *GotoStmt) stmtNode() {}

// LabelStmt represents a label ::name::.
type LabelStmt struct {
	baseStmt
	Name string
}

func (s *LabelStmt) stmtNode() {}

// FuncDefStmt represents a function definition statement.
type FuncDefStmt struct {
	baseStmt
	Name   []*VarExpr
	Params []*VarExpr
	Body   *BlockStmt
	IsVarArg bool
	IsLocal bool
	Func   *FuncExpr
}

func (s *FuncDefStmt) stmtNode() {}

// ExprStmt represents a standalone expression (function call, etc.).
type ExprStmt struct {
	baseStmt
	Expr Expr
}

func (s *ExprStmt) stmtNode() {}

// DoStmt represents a standalone do...end block.
// A do block creates a new scope for local variables.
type DoStmt struct {
	baseStmt
	Body *BlockStmt
}

func (s *DoStmt) stmtNode() {}

// GlobalStmt represents a Lua 5.4 global statement.
// Syntax: global [attr] ('*' | namelist) [= exprlist]
// When names and values are provided, assigns to _ENV.
type GlobalStmt struct {
	baseStmt
	Names  []string // Variable names (nil for wildcard *)
	Values []Expr   // Initial values (nil if no assignment)
}

func (s *GlobalStmt) stmtNode() {}// ============================================================================
// Helper Functions
// ============================================================================

// exprNode is a marker method for Expr interface.
func exprNode() {}

// stmtNode is a marker method for Stmt interface.
func stmtNode() {}

// Prototype wraps the object.Prototype with parser-specific information.
type Prototype struct {
	*object.Prototype
	Params []*VarExpr
}

// NewPrototype creates a new Prototype.
func NewPrototype(source string) *Prototype {
	return &Prototype{
		Prototype: &object.Prototype{
			Source: source,
		},
	}
}