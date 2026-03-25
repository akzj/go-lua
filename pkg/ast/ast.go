// Package ast defines the Abstract Syntax Tree (AST) node types for Lua.
package ast

import (
	"github.com/akzj/go-lua/pkg/lexer"
)

// Expr represents an expression node in the AST.
type Expr interface {
	exprNode()
	Line() int
}

// Stmt represents a statement node in the AST.
type Stmt interface {
	stmtNode()
	Line() int
}

// Base types
type baseExpr struct {
	line int
}

func (b *baseExpr) Line() int { return b.line }

type baseStmt struct {
	line int
}

func (b *baseStmt) Line() int { return b.line }

// Expression types

// NilExpr represents nil literal.
type NilExpr struct {
	baseExpr
}

func (*NilExpr) exprNode() {}

// BooleanExpr represents boolean literal (true/false).
type BooleanExpr struct {
	baseExpr
	Value bool
}

func (*BooleanExpr) exprNode() {}

// NumberExpr represents number literal.
type NumberExpr struct {
	baseExpr
	Value float64
}

func (*NumberExpr) exprNode() {}

// StringExpr represents string literal.
type StringExpr struct {
	baseExpr
	Value string
}

func (*StringExpr) exprNode() {}

// VarExpr represents variable reference.
type VarExpr struct {
	baseExpr
	Name string
}

func (*VarExpr) exprNode() {}

// IndexExpr represents table[key] indexing.
type IndexExpr struct {
	baseExpr
	Table Expr
	Key   Expr
}

func (*IndexExpr) exprNode() {}

// FieldExpr represents table.field field access.
type FieldExpr struct {
	baseExpr
	Table Expr
	Name  string
}

func (*FieldExpr) exprNode() {}

// BinaryExpr represents binary operations (+, -, *, /, etc.).
type BinaryExpr struct {
	baseExpr
	Op    lexer.TokenType
	Left  Expr
	Right Expr
}

func (*BinaryExpr) exprNode() {}

// UnaryExpr represents unary operations (-, not, #).
type UnaryExpr struct {
	baseExpr
	Op   lexer.TokenType
	Expr Expr
}

func (*UnaryExpr) exprNode() {}

// TableExpr represents table constructor {a, b, c}.
type TableExpr struct {
	baseExpr
	Fields []TableEntry
}

func (*TableExpr) exprNode() {}

// TableEntry represents a single entry in a table constructor.
type TableEntry struct {
	Key   Expr // nil for array entries
	Value Expr
}

func (*TableEntry) exprNode() {}

// CallExpr represents function call func(args).
type CallExpr struct {
	baseExpr
	Fn    Expr
	Args  []Expr
}

func (*CallExpr) exprNode() {}

// MethodCallExpr represents method call obj:method(args).
type MethodCallExpr struct {
	baseExpr
	Obj  Expr
	Name string
	Args []Expr
}

func (*MethodCallExpr) exprNode() {}

// FuncExpr represents anonymous function function(...) end.
type FuncExpr struct {
	baseExpr
	Params   []*ParamDecl
	Body     *BlockStmt
	IsVarArg bool
}

func (*FuncExpr) exprNode() {}

// ParamDecl represents a function parameter.
type ParamDecl struct {
	Name string
}

// DotsExpr represents vararg ...
type DotsExpr struct {
	baseExpr
}

func (*DotsExpr) exprNode() {}

// ParenExpr represents parenthesized expression (expr).
type ParenExpr struct {
	baseExpr
	Expr Expr
}

func (*ParenExpr) exprNode() {}

// Statement types

// BlockStmt represents a block of statements.
type BlockStmt struct {
	baseStmt
	Stmts []Stmt
}

func (*BlockStmt) stmtNode() {}

// AssignStmt represents assignment x = y.
type AssignStmt struct {
	baseStmt
	Targets []Expr
	Values  []Expr
}

func (*AssignStmt) stmtNode() {}

// LocalDeclStmt represents local variable declaration.
type LocalDeclStmt struct {
	baseStmt
	Names  []*VarExpr
	Values []Expr
}

func (*LocalDeclStmt) stmtNode() {}

// IfStmt represents if/elseif/else conditional.
type IfStmt struct {
	baseStmt
	Condition Expr
	Then      *BlockStmt
	ElseIfs   []*ElseIfClause
	Else      *BlockStmt
}

// ElseIfClause represents an elseif branch.
type ElseIfClause struct {
	Condition Expr
	Then      *BlockStmt
}

func (*IfStmt) stmtNode() {}

// WhileStmt represents while loop.
type WhileStmt struct {
	baseStmt
	Condition Expr
	Body      *BlockStmt
}

func (*WhileStmt) stmtNode() {}

// RepeatStmt represents repeat-until loop.
type RepeatStmt struct {
	baseStmt
	Body    *BlockStmt
	Condition Expr
}

func (*RepeatStmt) stmtNode() {}

// ForNumericStmt represents numeric for loop.
type ForNumericStmt struct {
	baseStmt
	Var   *VarExpr
	Start Expr
	End   Expr
	Step  Expr // optional
	Body  *BlockStmt
}

func (*ForNumericStmt) stmtNode() {}

// ForGenericStmt represents generic for loop.
type ForGenericStmt struct {
	baseStmt
	Vars  []*VarExpr
	Exprs []Expr
	Body  *BlockStmt
}

func (*ForGenericStmt) stmtNode() {}

// ReturnStmt represents return statement.
type ReturnStmt struct {
	baseStmt
	Expressions []Expr
}

func (*ReturnStmt) stmtNode() {}

// BreakStmt represents break statement.
type BreakStmt struct {
	baseStmt
}

func (*BreakStmt) stmtNode() {}

// FuncDefStmt represents function definition.
type FuncDefStmt struct {
	baseStmt
	Name     []*VarExpr
	Params   []*ParamDecl
	Body     *BlockStmt
	IsVarArg bool
}

func (*FuncDefStmt) stmtNode() {}

// GotoStmt represents goto label.
type GotoStmt struct {
	baseStmt
	Label string
}

func (*GotoStmt) stmtNode() {}

// LabelStmt represents label ::name::.
type LabelStmt struct {
	baseStmt
	Name string
}

func (*LabelStmt) stmtNode() {}
