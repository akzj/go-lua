// Package parser implements the Lua syntax analyzer.
package parser

import (
	"fmt"

	"github.com/akzj/go-lua/pkg/lexer"
	"github.com/akzj/go-lua/pkg/object"
)

// Parser performs syntax analysis on Lua source code.
//
// The parser uses recursive descent for statements and precedence climbing
// for expressions. It follows the semantics from lua-master/lparser.c.
//
// Fields:
//   - Lexer: The lexical analyzer
//   - Current: The current token being processed
//   - Peeked: The peeked token (if any)
//   - HasPeek: Whether there's a peeked token
//   - Errors: List of parsing errors encountered
//   - Source: Source name for error messages
type Parser struct {
	Lexer   *lexer.Lexer
	Current lexer.Token
	Peeked  lexer.Token
	HasPeek bool
	Errors  []error
	Source  string
}

// NewParser creates a new parser for the given lexer.
//
// The parser is initialized by reading the first token.
func NewParser(lexer *lexer.Lexer) *Parser {
	p := &Parser{
		Lexer:  lexer,
		Source: lexer.Name(),
	}
	// Prime the pump - read first token
	p.advance()
	return p
}

// Parse parses the entire source and returns a function prototype.
//
// This is the main entry point for parsing. It parses the source as a chunk
// (a sequence of statements) and returns the resulting prototype.
//
// Returns:
//   - *object.Prototype: The compiled function prototype
//   - error: Any parsing error encountered
func (p *Parser) Parse() (*object.Prototype, error) {
	// Parse the chunk (main body)
	body := p.parseChunk()

	// Check for errors
	if len(p.Errors) > 0 {
		return nil, p.Errors[0]
	}

	// Create prototype
	proto := &object.Prototype{
		Source:     p.Source,
		Code:       []object.Instruction{},
		Constants:  []object.TValue{},
		Upvalues:   []object.UpvalueDesc{},
		Prototypes: []*object.Prototype{},
		NumParams:  0,
		IsVarArg:   false,
	}

	// TODO: Generate bytecode from AST
	_ = body

	return proto, nil
}

// ParseChunk parses the source as a chunk and returns the AST block.
// This is used by the API layer to get the AST for codegen compilation.
func (p *Parser) ParseChunk() (*BlockStmt, error) {
	block := p.parseChunk()
	if len(p.Errors) > 0 {
		return nil, p.Errors[0]
	}
	return block, nil
}

// parseChunk parses a chunk (sequence of statements).
//
// A chunk is the main body of a Lua program or function.
// It consists of a sequence of statements.
func (p *Parser) parseChunk() *BlockStmt {
	stmts := []Stmt{}

	for !p.isAtEnd() && p.Current.Type != lexer.TK_END {
		// Check for statement terminator
		if p.Current.Type == lexer.TK_SEMICOLON {
			p.advance()
			continue
		}

		// Parse statement
		stmt := p.parseStmt()
		if stmt != nil {
			stmts = append(stmts, stmt)
		}

		// Check for errors
		if len(p.Errors) > 0 {
			break
		}
	}

	line := 1
	if len(stmts) > 0 {
		line = stmts[0].Line()
	}

	return &BlockStmt{
		Stmts: stmts,
		baseStmt: baseStmt{
			line: line,
		},
	}
}

// parseStmt parses a single statement.
func (p *Parser) parseStmt() Stmt {
	switch p.Current.Type {
	case lexer.TK_IF:
		return p.parseIfStmt()
	case lexer.TK_WHILE:
		return p.parseWhileStmt()
	case lexer.TK_DO:
		return p.parseDoBlock()
	case lexer.TK_REPEAT:
		return p.parseRepeatStmt()
	case lexer.TK_FOR:
		return p.parseForStmt()
	case lexer.TK_RETURN:
		return p.parseReturnStmt()
	case lexer.TK_BREAK:
		return p.parseBreakStmt()
	case lexer.TK_LOCAL:
		return p.parseLocalStmt()
	case lexer.TK_FUNCTION:
		return p.parseFuncDefStmt()
	case lexer.TK_GOTO:
		return p.parseGotoStmt()
	case lexer.TK_DBCOLON:
		return p.parseLabelStmt()
	default:
		// Try assignment or expression statement
		return p.parseAssignOrExprStmt()
	}
}

// parseBlock parses a block of statements.
func (p *Parser) parseBlock() *BlockStmt {
	stmts := []Stmt{}

	for !p.isAtEnd() && p.Current.Type != lexer.TK_END &&
		p.Current.Type != lexer.TK_ELSE &&
		p.Current.Type != lexer.TK_ELSEIF &&
		p.Current.Type != lexer.TK_UNTIL {
		// Check for statement terminator
		if p.Current.Type == lexer.TK_SEMICOLON {
			p.advance()
			continue
		}

		// Parse statement
		stmt := p.parseStmt()
		if stmt != nil {
			stmts = append(stmts, stmt)
		}

		// Check for errors
		if len(p.Errors) > 0 {
			break
		}
	}

	line := 1
	if len(stmts) > 0 {
		line = stmts[0].Line()
	}

	return &BlockStmt{
		Stmts: stmts,
		baseStmt: baseStmt{line: line},
	}
}

// advance gets the next token.
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

// peek looks at the next token without consuming it.
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

// match advances if the current token matches any of the given types.
func (p *Parser) match(types ...lexer.TokenType) bool {
	for _, t := range types {
		if p.Current.Type == t {
			p.advance()
			return true
		}
	}
	return false
}

// check returns true if the current token matches any of the given types.
func (p *Parser) check(types ...lexer.TokenType) bool {
	for _, t := range types {
		if p.Current.Type == t {
			return true
		}
	}
	return false
}

// expect advances if the current token matches the expected type.
// Otherwise, it records an error.
func (p *Parser) expect(t lexer.TokenType, message string) bool {
	if p.Current.Type == t {
		p.advance()
		return true
	}
	p.Error("expected %s, got %v", message, p.Current.Type)
	return false
}

// isAtEnd returns true if we've reached the end of the input.
func (p *Parser) isAtEnd() bool {
	return p.Current.Type == lexer.TK_EOF
}

// Error creates a parser error with the current position and adds it to the errors list.
func (p *Parser) Error(format string, args ...interface{}) error {
	err := fmt.Errorf("%s:%d: %s", p.Source, p.Current.Line, fmt.Sprintf(format, args...))
	p.Errors = append(p.Errors, err)
	return err
}

// sync synchronizes to the next statement boundary for error recovery.
func (p *Parser) sync() {
	// Skip tokens until we find a statement boundary
	for !p.isAtEnd() {
		switch p.Current.Type {
		case lexer.TK_IF, lexer.TK_WHILE, lexer.TK_REPEAT,
			lexer.TK_FOR, lexer.TK_RETURN, lexer.TK_BREAK,
			lexer.TK_LOCAL, lexer.TK_FUNCTION, lexer.TK_GOTO,
			lexer.TK_DBCOLON, lexer.TK_END, lexer.TK_ELSE,
			lexer.TK_ELSEIF, lexer.TK_UNTIL, lexer.TK_SEMICOLON:
			return
		default:
			p.advance()
		}
	}
}