// Package parser implements the Lua syntax analyzer
package parser

import (
	"fmt"
	"github.com/akzj/go-lua/pkg/lexer"
	"github.com/akzj/go-lua/pkg/object"
)

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
	ExprBinOp
	ExprUnOp
)

// Expr represents an AST expression node
type Expr struct {
	Kind       ExprKind
	Value      interface{}
	LineNumber int
	Left       *Expr
	Right      *Expr
	Op         string
}

// Stmt represents an AST statement node
type Stmt interface {
	stmtNode()
	Line() int
}

// BlockStmt represents a block of statements
type BlockStmt struct {
	Stmts      []Stmt
	LineNumber int
}

func (s *BlockStmt) stmtNode() {}
func (s *BlockStmt) Line() int { return s.LineNumber }

// AssignStmt represents an assignment statement
type AssignStmt struct {
	Left       []*Expr
	Right      []*Expr
	LineNumber int
}

func (s *AssignStmt) stmtNode() {}
func (s *AssignStmt) Line() int { return s.LineNumber }

// LocalStmt represents a local variable declaration
type LocalStmt struct {
	Names      []*Expr
	Values     []*Expr
	LineNumber int
}

func (s *LocalStmt) stmtNode() {}
func (s *LocalStmt) Line() int { return s.LineNumber }

// IfStmt represents an if statement
type IfStmt struct {
	Cond       *Expr
	Then       *BlockStmt
	ElseIf     []struct {
		Cond *Expr
		Then *BlockStmt
	}
	Else       *BlockStmt
	LineNumber int
}

func (s *IfStmt) stmtNode() {}
func (s *IfStmt) Line() int { return s.LineNumber }

// WhileStmt represents a while statement
type WhileStmt struct {
	Cond       *Expr
	Body       *BlockStmt
	LineNumber int
}

func (s *WhileStmt) stmtNode() {}
func (s *WhileStmt) Line() int { return s.LineNumber }

// ForStmt represents a numeric for statement
type ForStmt struct {
	Name       *Expr
	Start      *Expr
	End        *Expr
	Step       *Expr
	Body       *BlockStmt
	LineNumber int
}

func (s *ForStmt) stmtNode() {}
func (s *ForStmt) Line() int { return s.LineNumber }

// FuncStmt represents a function definition
type FuncStmt struct {
	Name       *Expr
	Params     []*Expr
	Body       *BlockStmt
	LineNumber int
}

func (s *FuncStmt) stmtNode() {}
func (s *FuncStmt) Line() int { return s.LineNumber }

// ReturnStmt represents a return statement
type ReturnStmt struct {
	Values     []*Expr
	LineNumber int
}

func (s *ReturnStmt) stmtNode() {}
func (s *ReturnStmt) Line() int { return s.LineNumber }

// Parser performs syntax analysis
type Parser struct {
	Lexer   *lexer.Lexer
	Current lexer.Token
	Peeked  lexer.Token
	HasPeek bool
	Errors  []error
}

// NewParser creates a new parser
func NewParser(lexer *lexer.Lexer) *Parser {
	p := &Parser{
		Lexer: lexer,
	}
	// Prime the pump
	p.advance()
	return p
}

// Parse parses the source and returns a function prototype
func (p *Parser) Parse() (*object.Prototype, error) {
	// TODO: Implement full parser
	return &object.Prototype{
		Source: p.Lexer.Name,
	}, nil
}

// advance gets the next token
func (p *Parser) advance() {
	if p.HasPeek {
		p.Current = p.Peeked
		p.HasPeek = false
	} else {
		token, err := p.Lexer.NextToken()
		if err != nil {
			p.Errors = append(p.Errors, err)
		}
		p.Current = token
	}
}

// peek looks at the next token
func (p *Parser) peek() lexer.Token {
	if !p.HasPeek {
		token, err := p.Lexer.NextToken()
		if err != nil {
			p.Errors = append(p.Errors, err)
		}
		p.Peeked = token
		p.HasPeek = true
	}
	return p.Peeked
}

// Error creates a parser error
func (p *Parser) Error(format string, args ...interface{}) error {
	return fmt.Errorf("%s:%d: %s", p.Lexer.Name, p.Current.Line, fmt.Sprintf(format, args...))
}