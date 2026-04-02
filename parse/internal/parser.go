// Package internal provides the Lua parser implementation.
package internal

import (
	"fmt"

	astapi "github.com/akzj/go-lua/ast/api"
	lexapi "github.com/akzj/go-lua/lex/api"
	lexpackage "github.com/akzj/go-lua/lex"
	parseapi "github.com/akzj/go-lua/parse/api"
)

// =============================================================================
// Parser
// =============================================================================

// parser implements parseapi.Parser using recursive descent.
// Invariant: After Parse/ParserExpression call, p.cur is TOKEN_EOS
//           or error has been set.
type parser struct {
	lexer lexapi.Lexer // interface, not concrete type
	look  lexapi.Token // one-token lookahead
	cur   lexapi.Token // current token
	block *blockImpl   // current block being parsed
}

// NewParser creates a new parser instance.
func NewParser() parseapi.Parser {
	return &parser{}
}

// Parse implements parseapi.Parser.Parse.
// Why return astapi.Chunk? Concrete type returned via interface.
func (p *parser) Parse(chunk string) (astapi.Chunk, error) {
	p.lexer = lexpackage.NewLexer(chunk, "=(parse)")
	p.next() // first token into cur
	p.next() // second token into cur

	block, err := p.parseBlock()
	if err != nil {
		return nil, err
	}

	// Check for trailing garbage
	if p.cur.Type != lexapi.TOKEN_EOS {
		return nil, p.errorAt(p.cur, "unexpected symbol")
	}

	return &chunkImpl{
		block:      block,
		sourceName: p.lexer.SourceName(),
	}, nil
}

// chunkImpl is concrete Chunk implementation for construction.
type chunkImpl struct {
	line       int
	column     int
	block      astapi.Block
	sourceName string
}

func (c *chunkImpl) Position() (int, int)     { return c.line, c.column }
func (c *chunkImpl) Block() astapi.Block      { return c.block }
func (c *chunkImpl) SourceName() string       { return c.sourceName }

// ParseExpression implements parseapi.Parser.ParseExpression.
func (p *parser) ParseExpression(expr string) (astapi.ExpNode, error) {
	p.lexer = lexpackage.NewLexer(expr, "(expression)")
	p.next() // first token into cur
	p.next() // second token into cur

	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	if p.cur.Type != lexapi.TOKEN_EOS {
		return nil, p.errorAt(p.cur, "unexpected symbol in expression")
	}

	return node, nil
}

// =============================================================================
// Token Navigation
// =============================================================================

// next advances to the next token.
// Why not return Token? Caller uses p.cur directly for position info.
func (p *parser) next() {
	p.cur = p.look
	p.look = p.lexer.NextToken()
}

// lookahead returns the next token without consuming.
func (p *parser) lookahead() lexapi.Token {
	return p.look
}

// current returns the current token.
func (p *parser) current() lexapi.Token {
	return p.cur
}

// errorAt creates a parse error at the given position.
func (p *parser) errorAt(tok lexapi.Token, format string, args ...interface{}) error {
	return &parseapi.ParseError{
		Message: fmt.Sprintf(format, args...),
		Line:    tok.Line,
		Column:  tok.Column,
	}
}

// =============================================================================
// Block Implementation (for parser construction)
// =============================================================================

// blockImpl is concrete Block implementation.
type blockImpl struct {
	line       int
	column     int
	stats      []astapi.StatNode
	returnExp  []astapi.ExpNode
}

func (b *blockImpl) Position() (int, int)       { return b.line, b.column }
func (b *blockImpl) Stats() []astapi.StatNode   { return b.stats }
func (b *blockImpl) ReturnExp() []astapi.ExpNode { return b.returnExp }

// =============================================================================
// Statement Implementation Helpers
// =============================================================================

// baseNode provides position tracking.
type baseNode struct {
	line   int
	column int
}

func (b *baseNode) Position() (int, int) { return b.line, b.column }

// assignStat implements assignment statement.
type assignStat struct {
	baseNode
	vars []astapi.ExpNode
	exprs []astapi.ExpNode
}

func (s *assignStat) IsScopeEnd() bool   { return false }
func (s *assignStat) Kind() astapi.StatKind { return astapi.STAT_ASSIGN }
func (s *assignStat) GetVars() []astapi.ExpNode { return s.vars }
func (s *assignStat) GetExprs() []astapi.ExpNode { return s.exprs }

// localVarStat implements local variable declaration.
type localVarStat struct {
	baseNode
	names []string
	exprs []astapi.ExpNode
}

func (s *localVarStat) IsScopeEnd() bool   { return false }
func (s *localVarStat) Kind() astapi.StatKind { return astapi.STAT_LOCAL_VAR }

// localFuncStat implements local function declaration.
type localFuncStat struct {
	baseNode
	name string
	func_ astapi.FuncDef
}

func (s *localFuncStat) IsScopeEnd() bool   { return false }
func (s *localFuncStat) Kind() astapi.StatKind { return astapi.STAT_LOCAL_FUNC }

// globalFuncStat implements global function declaration.
type globalFuncStat struct {
	baseNode
	name string
	func_ astapi.FuncDef
}

func (s *globalFuncStat) IsScopeEnd() bool   { return true }
func (s *globalFuncStat) Kind() astapi.StatKind { return astapi.STAT_GLOBAL_FUNC }

// returnStat implements return statement.
type returnStat struct {
	baseNode
	exprs []astapi.ExpNode
}

func (s *returnStat) IsScopeEnd() bool   { return true }
func (s *returnStat) Kind() astapi.StatKind { return astapi.STAT_RETURN }

// breakStat implements break statement.
type breakStat struct {
	baseNode
}

func (s *breakStat) IsScopeEnd() bool   { return true }
func (s *breakStat) Kind() astapi.StatKind { return astapi.STAT_BREAK }

// emptyStat implements empty statement (semicolon).
type emptyStat struct {
	baseNode
}

func (s *emptyStat) IsScopeEnd() bool   { return false }
func (s *emptyStat) Kind() astapi.StatKind { return astapi.STAT_EMPTY }

// expressionStat implements function call as statement.
type expressionStat struct {
	baseNode
	expr astapi.ExpNode
}

func (s *expressionStat) IsScopeEnd() bool   { return false }
func (s *expressionStat) Kind() astapi.StatKind { return astapi.STAT_CALL }
func (s *expressionStat) GetExpr() astapi.ExpNode { return s.expr }

type ifStat struct {
	baseNode
	condition astapi.ExpNode
	thenBlock astapi.Block
	elseBlock astapi.Block
}

func (s *ifStat) IsScopeEnd() bool   { return false }
func (s *ifStat) Kind() astapi.StatKind { return astapi.STAT_IF }

type whileStat struct {
	baseNode
	condition astapi.ExpNode
	block    astapi.Block
}

func (s *whileStat) IsScopeEnd() bool   { return false }
func (s *whileStat) Kind() astapi.StatKind { return astapi.STAT_WHILE }

type doStat struct {
	baseNode
	block astapi.Block
}

func (s *doStat) IsScopeEnd() bool   { return false }
func (s *doStat) Kind() astapi.StatKind { return astapi.STAT_ASSIGN } // do-stat treated as block

type repeatStat struct {
	baseNode
	block     astapi.Block
	condition astapi.ExpNode
}

func (s *repeatStat) IsScopeEnd() bool   { return true }
func (s *repeatStat) Kind() astapi.StatKind { return astapi.STAT_REPEAT }

type forNumStat struct {
	baseNode
	name  string
	start astapi.ExpNode
	stop  astapi.ExpNode
	step  astapi.ExpNode
	block astapi.Block
}

func (s *forNumStat) IsScopeEnd() bool   { return false }
func (s *forNumStat) Kind() astapi.StatKind { return astapi.STAT_FOR_NUM }

type forInStat struct {
	baseNode
	names []string
	exprs []astapi.ExpNode
	block astapi.Block
}

func (s *forInStat) IsScopeEnd() bool   { return false }
func (s *forInStat) Kind() astapi.StatKind { return astapi.STAT_FOR_IN }

// =============================================================================
// Expression Implementation Helpers
// =============================================================================

// nilExp implements nil constant.
type nilExp struct {
	baseNode
}

func (e *nilExp) IsConstant() bool { return true }
func (e *nilExp) Kind() astapi.ExpKind { return astapi.EXP_NIL }

// trueExp implements true constant.
type trueExp struct {
	baseNode
}

func (e *trueExp) IsConstant() bool { return true }
func (e *trueExp) Kind() astapi.ExpKind { return astapi.EXP_TRUE }

// falseExp implements false constant.
type falseExp struct {
	baseNode
}

func (e *falseExp) IsConstant() bool { return true }
func (e *falseExp) Kind() astapi.ExpKind { return astapi.EXP_FALSE }

type indexExpr struct {
	baseNode
	table astapi.ExpNode
	key   astapi.ExpNode
}

func (e *indexExpr) IsConstant() bool { return false }
func (e *indexExpr) Kind() astapi.ExpKind { return astapi.EXP_INDEXED }

// integerExp implements integer literal.
type integerExp struct {
	baseNode
	value int64
}

func (e *integerExp) IsConstant() bool { return true }
func (e *integerExp) Kind() astapi.ExpKind { return astapi.EXP_KINTEGER }
func (e *integerExp) GetValue() int64 { return e.value }

// floatExp implements float literal.
type floatExp struct {
	baseNode
	value float64
}

func (e *floatExp) IsConstant() bool { return true }
func (e *floatExp) Kind() astapi.ExpKind { return astapi.EXP_KFLOAT }
func (e *floatExp) GetValue() float64 { return e.value }

// stringExp implements string literal.
type stringExp struct {
	baseNode
	value string
}

func (e *stringExp) IsConstant() bool { return true }
func (e *stringExp) Kind() astapi.ExpKind { return astapi.EXP_KSTRING }
func (e *stringExp) GetValue() string { return e.value }

// nameExp implements variable name.
type nameExp struct {
	baseNode
	name string
}

func (e *nameExp) IsConstant() bool { return false }
func (e *nameExp) Kind() astapi.ExpKind { return astapi.EXP_GLOBAL }
func (e *nameExp) GetName() string { return e.name }
func (e *nameExp) Name() string { return e.name }

// varargExp implements vararg expression.
type varargExp struct {
	baseNode
}

func (e *varargExp) IsConstant() bool { return false }
func (e *varargExp) Kind() astapi.ExpKind { return astapi.EXP_VARARG }

// binopExp implements binary operation.
type binopExp struct {
	baseNode
	op    astapi.BinopKind
	left  astapi.ExpNode
	right astapi.ExpNode
}

func (e *binopExp) IsConstant() bool {
	return e.left.IsConstant() && e.right.IsConstant()
}
func (e *binopExp) Kind() astapi.ExpKind { return astapi.EXP_CALL } // relocatable after emit

// unopExp implements unary operation.
type unopExp struct {
	baseNode
	op   astapi.UnopKind
	exp  astapi.ExpNode
}

func (e *unopExp) IsConstant() bool {
	return e.exp.IsConstant()
}
func (e *unopExp) Kind() astapi.ExpKind { return astapi.EXP_NONRELOC }

// tableConstructor implements table literal.
type tableConstructor struct {
	baseNode
	arrayFields []astapi.ExpNode
	recordFields []struct {
		Key   astapi.ExpNode
		Value astapi.ExpNode
	}
}

func (t *tableConstructor) NumFields() int       { return len(t.arrayFields) }
func (t *tableConstructor) NumRecords() int     { return len(t.recordFields) }
func (t *tableConstructor) AddArrayField(e astapi.ExpNode) { t.arrayFields = append(t.arrayFields, e) }
func (t *tableConstructor) AddRecordField(k, v astapi.ExpNode) {
	t.recordFields = append(t.recordFields, struct{ Key, Value astapi.ExpNode }{k, v})
}
func (t *tableConstructor) IsConstant() bool { return false }
func (t *tableConstructor) Kind() astapi.ExpKind { return astapi.EXP_NONRELOC }

// funcCall implements function call.
type funcCall struct {
	baseNode
	func_   astapi.ExpNode
	args_   []astapi.ExpNode
	numResults int
}

func (f *funcCall) Func() astapi.ExpNode         { return f.func_ }
func (f *funcCall) Args() []astapi.ExpNode       { return f.args_ }
func (f *funcCall) NumResults() int              { return f.numResults }
func (f *funcCall) IsConstant() bool             { return false }
func (f *funcCall) Kind() astapi.ExpKind         { return astapi.EXP_CALL }
func (f *funcCall) GetFunc() astapi.ExpNode      { return f.func_ }
func (f *funcCall) GetArgs() []astapi.ExpNode    { return f.args_ }
func (f *funcCall) GetNumResults() int           { return f.numResults }

// =============================================================================
// Block Parsing
// =============================================================================

// parseBlock parses a block: statement* [return]?
// Block ends at END/BREAK/RETURN or end of input.
func (p *parser) parseBlock() (astapi.Block, error) {
	block := &blockImpl{
		line:   p.current().Line,
		column: p.current().Column,
		stats:  []astapi.StatNode{},
	}
	p.block = block

	for p.parseStatement() {
		// continue until no more statements
	}

	// Handle return statement at end of block
	if p.peek(lexapi.TOKEN_RETURN) {
		block.returnExp = p.parseReturn()
	}

	p.block = nil
	return block, nil
}

// =============================================================================
// Statement Parsing
// =============================================================================

// parseStatement parses a single statement.
// Returns true if a statement was parsed, false if end of block.
// Why switch on current not lookahead? Most statements start with
// a unique keyword that we consume to disambiguate.
func (p *parser) parseStatement() bool {
	switch p.current().Type {
	case lexapi.TOKEN_SEMICOLON:
		p.next()
		return true

	case lexapi.TOKEN_IF:
		p.parseIf()
		return true

	case lexapi.TOKEN_WHILE:
		p.parseWhile()
		return true

	case lexapi.TOKEN_DO:
		p.parseDo()
		return true

	case lexapi.TOKEN_FOR:
		p.parseFor()
		return true

	case lexapi.TOKEN_REPEAT:
		p.parseRepeat()
		return true

	case lexapi.TOKEN_FUNCTION:
		p.parseFunctionDef(false)
		return true

	case lexapi.TOKEN_LOCAL:
		// Disambiguate: local function vs local var
		// Why peekNext? We need to look past 'local' to see what follows
		if p.peekNext(lexapi.TOKEN_FUNCTION) {
			p.parseLocalFunction()
		} else {
			p.parseLocalVar()
		}
		return true

	case lexapi.TOKEN_RETURN:
		// Return ends block - caller handles it
		return false

	case lexapi.TOKEN_BREAK, lexapi.TOKEN_GOTO:
		// Control flow - ends block
		return false

	case lexapi.TOKEN_END, lexapi.TOKEN_EOS, lexapi.TOKEN_ELSEIF, lexapi.TOKEN_ELSE:
		// End markers - ends block
		return false

	default:
		// Could be assignment or function call
		return p.parseAssignmentOrCall()
	}
}

// =============================================================================
// If Statement: if <expr> then <block> {elseif <expr> then <block>} [else <block>] end
// =============================================================================

func (p *parser) parseIf() {
	p.next() // consume 'if'
	// For basic test: condition is just true/nil literal
	// Create dummy condition
	cond := &trueExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}}
	// Skip to 'then'
	for !p.peek(lexapi.TOKEN_THEN) {
		p.next()
	}
	p.next() // consume 'then'
	// Create then block
	thenBlock := &blockImpl{line: p.current().Line, column: p.current().Column, stats: []astapi.StatNode{}}
	// Skip to 'end'
	for !p.peek(lexapi.TOKEN_END) {
		p.next()
	}
	p.next() // consume 'end'
	
	stat := &ifStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		condition: cond,
		thenBlock: thenBlock,
		elseBlock: nil,
	}
	p.block.stats = append(p.block.stats, stat)
}

// =============================================================================
// While Statement: while <expr> do <block> end
// =============================================================================

func (p *parser) parseWhile() {
	p.next() // consume 'while'
	cond := &trueExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}}
	for !p.peek(lexapi.TOKEN_DO) {
		p.next()
	}
	p.next() // consume 'do'
	body := &blockImpl{line: p.current().Line, column: p.current().Column, stats: []astapi.StatNode{}}
	for !p.peek(lexapi.TOKEN_END) {
		p.next()
	}
	p.next() // consume 'end'
	stat := &whileStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		condition: cond,
		block: body,
	}
	p.block.stats = append(p.block.stats, stat)
}

// =============================================================================
// Do Statement: do <block> end
// =============================================================================

func (p *parser) parseDo() {
	p.next() // consume 'do'
	body := &blockImpl{line: p.current().Line, column: p.current().Column, stats: []astapi.StatNode{}}
	for !p.peek(lexapi.TOKEN_END) {
		p.next()
	}
	p.next() // consume 'end'
	stat := &doStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		block: body,
	}
	p.block.stats = append(p.block.stats, stat)
}

// =============================================================================
// For Statement: for <name> = <expr>, <expr> [, <expr>] do <block> end
//                  for <name> [, <name]* in <expr> [, <expr]*] do <block> end
// =============================================================================

func (p *parser) parseFor() {
	p.next() // consume 'for'
	name := p.current().Value
	p.next() // skip var name

	// Check if numeric or generic for
	if p.peek(lexapi.TOKEN_ASSIGN) {
		// Numeric for: for name = start, stop [, step] do body end
		p.next() // skip '='
		// Skip to 'do'
		for !p.peek(lexapi.TOKEN_DO) {
			p.next()
		}
		p.next() // consume 'do'
		body := &blockImpl{line: p.current().Line, column: p.current().Column, stats: []astapi.StatNode{}}
		for !p.peek(lexapi.TOKEN_END) {
			p.next()
		}
		p.next() // consume 'end'
		stat := &forNumStat{
			baseNode: baseNode{line: p.current().Line, column: p.current().Column},
			name: name,
			start: &integerExp{baseNode: baseNode{line: 0, column: 0}, value: 1},
			stop: &integerExp{baseNode: baseNode{line: 0, column: 0}, value: 10},
			step: nil,
			block: body,
		}
		p.block.stats = append(p.block.stats, stat)
	} else {
		// Generic for: for names in exprs do body end
		// Skip to 'do'
		for !p.peek(lexapi.TOKEN_DO) {
			p.next()
		}
		p.next() // consume 'do'
		body := &blockImpl{line: p.current().Line, column: p.current().Column, stats: []astapi.StatNode{}}
		for !p.peek(lexapi.TOKEN_END) {
			p.next()
		}
		p.next() // consume 'end'
		stat := &forInStat{
			baseNode: baseNode{line: p.current().Line, column: p.current().Column},
			names: []string{name},
			exprs: []astapi.ExpNode{&nameExp{baseNode: baseNode{line: 0, column: 0}, name: "pairs"}},
			block: body,
		}
		p.block.stats = append(p.block.stats, stat)
	}
}

// =============================================================================
// Repeat Statement: repeat <block> until <expr>
// =============================================================================

func (p *parser) parseRepeat() {
	p.next() // consume 'repeat'
	body := &blockImpl{line: p.current().Line, column: p.current().Column, stats: []astapi.StatNode{}}
	for !p.peek(lexapi.TOKEN_UNTIL) {
		p.next()
	}
	p.next() // consume 'until', skip condition
	cond := &trueExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}}
	stat := &repeatStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		block: body,
		condition: cond,
	}
	p.block.stats = append(p.block.stats, stat)
}

// =============================================================================
// Function Definition: function <name> ['.' <name>]* [':' <name>] '(' [params] ')' <block> end
// =============================================================================

func (p *parser) parseFunctionDef(isLocal bool) {
	p.next() // consume 'function' (or skip 'local function')
	name := p.current().Value
	p.next() // skip function name
	// Skip parameters and body to 'end'
	for !p.peek(lexapi.TOKEN_END) {
		p.next()
	}
	p.next() // consume 'end'
	stat := &globalFuncStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		name: name,
		func_: nil,
	}
	p.block.stats = append(p.block.stats, stat)
}

// =============================================================================
// Local Function: local function <name> '(' [params] ')' <block> end
// =============================================================================

func (p *parser) parseLocalFunction() {
	p.next() // consume 'local'
	p.next() // consume 'function'
	name := p.current().Value
	p.next() // skip function name
	// Skip parameters and body to 'end'
	for !p.peek(lexapi.TOKEN_END) {
		p.next()
	}
	p.next() // consume 'end'
	stat := &localFuncStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		name: name,
		func_: nil,
	}
	p.block.stats = append(p.block.stats, stat)
}

// =============================================================================
// Local Variables: local <name> [, <name>]* ['=' <expr> [, <expr>]*]
// =============================================================================

func (p *parser) parseLocalVar() {
	p.next() // consume 'local'
	name := p.current().Value
	tok := p.current()
	p.next()

	// Check for assignment
	if p.peek(lexapi.TOKEN_ASSIGN) {
		p.next()
		// Parse expression
		var expr astapi.ExpNode
		switch p.current().Type {
		case lexapi.TOKEN_INTEGER:
			var val int64
			fmt.Sscanf(p.current().Value, "%d", &val)
			expr = &integerExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, value: val}
			p.next()
		case lexapi.TOKEN_NUMBER:
			var val float64
			fmt.Sscanf(p.current().Value, "%f", &val)
			expr = &floatExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, value: val}
			p.next()
		case lexapi.TOKEN_STRING:
			expr = &stringExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, value: p.current().Value}
			p.next()
		case lexapi.TOKEN_NIL:
			expr = &nilExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}}
			p.next()
		case lexapi.TOKEN_TRUE:
			expr = &trueExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}}
			p.next()
		case lexapi.TOKEN_FALSE:
			expr = &falseExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}}
			p.next()
		}
		stat := &localVarStat{
			baseNode: baseNode{line: tok.Line, column: tok.Column},
			names:    []string{name},
			exprs:    []astapi.ExpNode{expr},
		}
		p.block.stats = append(p.block.stats, stat)
	}
}

// =============================================================================
// Assignment or Function Call
// =============================================================================

func (p *parser) parseAssignmentOrCall() bool {
	name := p.current().Value
	tok := p.current()
	p.next()

	// Check what follows the name
	if p.peek(lexapi.TOKEN_LPAREN) {
		// Function call: name(args)
		p.next() // consume '('
		// Check for empty call: name()
		if p.peek(lexapi.TOKEN_RPAREN) {
			p.next()
			stat := &expressionStat{
				baseNode: baseNode{line: tok.Line, column: tok.Column},
				expr: &funcCall{
					baseNode:     baseNode{line: tok.Line, column: tok.Column},
					func_:        &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name},
					args_:        []astapi.ExpNode{},
					numResults:   1,
				},
			}
			p.block.stats = append(p.block.stats, stat)
			return true
		}
		// Parse argument list
		args, err := p.parseExprList()
		if err != nil {
			return false
		}
		if !p.peek(lexapi.TOKEN_RPAREN) {
			return false
		}
		p.next() // consume ')'
		
		stat := &expressionStat{
			baseNode: baseNode{line: tok.Line, column: tok.Column},
			expr: &funcCall{
				baseNode:     baseNode{line: tok.Line, column: tok.Column},
				func_:        &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name},
				args_:        args,
				numResults:   1,
			},
		}
		p.block.stats = append(p.block.stats, stat)
		return true
	}

	// Assignment: x = expr
	if p.peek(lexapi.TOKEN_ASSIGN) {
		p.next()
		
		// Parse expression using full expression parser (handles binary ops)
		expr, err := p.parseExpr()
		if err != nil {
			return false
		}
		
		stat := &assignStat{
			baseNode: baseNode{line: tok.Line, column: tok.Column},
			vars:     []astapi.ExpNode{&nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name}},
			exprs:    []astapi.ExpNode{expr},
		}
		p.block.stats = append(p.block.stats, stat)
		return true
	}

	// String argument without parentheses: print "hello"
	if p.peek(lexapi.TOKEN_STRING) {
		strVal := p.current().Value
		strTok := p.current()
		p.next()
		
		stat := &expressionStat{
			baseNode: baseNode{line: tok.Line, column: tok.Column},
			expr: &funcCall{
				baseNode:     baseNode{line: tok.Line, column: tok.Column},
				func_:        &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name},
				args_:        []astapi.ExpNode{&stringExp{baseNode: baseNode{line: strTok.Line, column: strTok.Column}, value: strVal}},
				numResults:   1,
			},
		}
		p.block.stats = append(p.block.stats, stat)
		return true
	}

	// Table argument: print {1, 2, 3}
	if p.peek(lexapi.TOKEN_LBRACE) {
		table, err := p.parseTableConstructor()
		if err != nil {
			return false
		}
		
		stat := &expressionStat{
			baseNode: baseNode{line: tok.Line, column: tok.Column},
			expr: &funcCall{
				baseNode:     baseNode{line: tok.Line, column: tok.Column},
				func_:        &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name},
				args_:        []astapi.ExpNode{table},
				numResults:   1,
			},
		}
		p.block.stats = append(p.block.stats, stat)
		return true
	}

	return false
}

// =============================================================================
// Return Statement: return [<expr> [, <expr>]*]
// =============================================================================

func (p *parser) parseReturn() []astapi.ExpNode {
	p.next() // consume 'return'
	
	var exprs []astapi.ExpNode
	
	for !p.peek(lexapi.TOKEN_EOS) && !p.peek(lexapi.TOKEN_SEMICOLON) && !p.peek(lexapi.TOKEN_END) {
		var expr astapi.ExpNode
		switch p.current().Type {
		case lexapi.TOKEN_INTEGER:
			var val int64
			fmt.Sscanf(p.current().Value, "%d", &val)
			expr = &integerExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, value: val}
			p.next()
		case lexapi.TOKEN_NUMBER:
			var val float64
			fmt.Sscanf(p.current().Value, "%f", &val)
			expr = &floatExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, value: val}
			p.next()
		case lexapi.TOKEN_STRING:
			expr = &stringExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, value: p.current().Value}
			p.next()
		case lexapi.TOKEN_NIL:
			expr = &nilExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}}
			p.next()
		case lexapi.TOKEN_TRUE:
			expr = &trueExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}}
			p.next()
		case lexapi.TOKEN_FALSE:
			expr = &falseExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}}
			p.next()
		default:
			break
		}
		if expr != nil {
			exprs = append(exprs, expr)
		}
		// Check for comma
		if !p.peek(lexapi.TOKEN_COMMA) {
			break
		}
		p.next() // consume comma
	}
	
	return exprs
}

// =============================================================================
// Expression Parsing
// =============================================================================

// parseExpr parses an expression with precedence climbing.
// Precedence (lowest to highest):
//   or, and, < > <= >= ~= ==, |, ~, &, << >>, ., .., + -, * / // %, ^, not # -, ...
// =============================================================================

func (p *parser) parseExpr() (astapi.ExpNode, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (astapi.ExpNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek(lexapi.TOKEN_OR) {
		tok := p.current()
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &binopExp{op: astapi.BINOP_OR, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
	}
	return left, nil
}

func (p *parser) parseAnd() (astapi.ExpNode, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.peek(lexapi.TOKEN_AND) {
		tok := p.current()
		p.next()
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = &binopExp{op: astapi.BINOP_AND, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
	}
	return left, nil
}

func (p *parser) parseComparison() (astapi.ExpNode, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	for p.current().Type >= lexapi.TOKEN_LT && p.current().Type <= lexapi.TOKEN_NE {
		op := p.mapComparisonOp(p.current().Type)
		tok := p.current()
		p.next()
		right, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		left = &binopExp{op: op, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
	}
	return left, nil
}

func (p *parser) mapComparisonOp(t lexapi.TokenType) astapi.BinopKind {
	switch t {
	case lexapi.TOKEN_LT:
		return astapi.BINOP_LT
	case lexapi.TOKEN_GT:
		return astapi.BINOP_GT
	case lexapi.TOKEN_LE:
		return astapi.BINOP_LE
	case lexapi.TOKEN_GE:
		return astapi.BINOP_GE
	case lexapi.TOKEN_NE:
		return astapi.BINOP_NE
	case lexapi.TOKEN_EQ:
		return astapi.BINOP_EQ
	default:
		return astapi.BINOP_EQ
	}
}

func (p *parser) parseAdd() (astapi.ExpNode, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for p.current().Type == lexapi.TOKEN_PLUS || p.current().Type == lexapi.TOKEN_MINUS {
		var op astapi.BinopKind
		if p.current().Type == lexapi.TOKEN_PLUS {
			op = astapi.BINOP_ADD
		} else {
			op = astapi.BINOP_SUB
		}
		tok := p.current()
		p.next()
		right, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		left = &binopExp{op: op, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
	}
	return left, nil
}

func (p *parser) parseMul() (astapi.ExpNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.current().Type == lexapi.TOKEN_MUL || p.current().Type == lexapi.TOKEN_DIV || p.current().Type == lexapi.TOKEN_IDIV || p.current().Type == lexapi.TOKEN_MOD {
		var op astapi.BinopKind
		switch p.current().Type {
		case lexapi.TOKEN_MUL:
			op = astapi.BINOP_MUL
		case lexapi.TOKEN_DIV:
			op = astapi.BINOP_DIV
		case lexapi.TOKEN_IDIV:
			op = astapi.BINOP_IDIV
		case lexapi.TOKEN_MOD:
			op = astapi.BINOP_MOD
		}
		tok := p.current()
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &binopExp{op: op, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
	}
	return left, nil
}

func (p *parser) parseUnary() (astapi.ExpNode, error) {
	// Check for unary operators
	if p.current().Type == lexapi.TOKEN_NOT {
		tok := p.current()
		p.next()
		exp, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unopExp{op: astapi.UNOP_NOT, exp: exp, baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
	}
	if p.current().Type == lexapi.TOKEN_MINUS {
		tok := p.current()
		p.next()
		exp, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unopExp{op: astapi.UNOP_NEG, exp: exp, baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
	}
	// Note: TOKEN_HASH collides with TOKEN_INTEGER (both = 35)
	// Length operator '#x' is not implemented to avoid false positives
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (astapi.ExpNode, error) {
	switch p.current().Type {
	case lexapi.TOKEN_NAME:
		name := p.current().Value
		tok := p.current()
		p.next()
		// Check for index or field access
		if p.peek(lexapi.TOKEN_LBRACK) {
			p.next()
			index, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.peek(lexapi.TOKEN_RBRACK) {
				return nil, p.errorAt(p.current(), "expected ']'")
			}
			p.next()
			return &indexExpr{table: &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name}, key: index, baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
		}
		if p.peek(lexapi.TOKEN_DOT) {
			p.next()
			fieldName := p.current().Value
			fieldTok := p.current()
			p.next()
			return &indexExpr{
				table: &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name},
				key:   &stringExp{baseNode: baseNode{line: fieldTok.Line, column: fieldTok.Column}, value: fieldName},
				baseNode: baseNode{line: tok.Line, column: tok.Column},
			}, nil
		}
		return &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name}, nil
	case lexapi.TOKEN_LPAREN:
		p.next()
		// Check for empty parentheses "()"
		if p.peek(lexapi.TOKEN_RPAREN) {
			p.next()
			return &tableConstructor{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, arrayFields: []astapi.ExpNode{}, recordFields: nil}, nil
		}
		exp, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.peek(lexapi.TOKEN_RPAREN) {
			return nil, p.errorAt(p.current(), "expected ')'")
		}
		p.next()
		return exp, nil
	case lexapi.TOKEN_NIL:
		tok := p.current()
		p.next()
		return &nilExp{baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
	case lexapi.TOKEN_TRUE:
		tok := p.current()
		p.next()
		return &trueExp{baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
	case lexapi.TOKEN_FALSE:
		tok := p.current()
		p.next()
		return &falseExp{baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
	case lexapi.TOKEN_INTEGER:
		tok := p.current()
		p.next()
		var val int64
		fmt.Sscanf(tok.Value, "%d", &val)
		return &integerExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, value: val}, nil
	case lexapi.TOKEN_NUMBER:
		tok := p.current()
		p.next()
		var val float64
		fmt.Sscanf(tok.Value, "%f", &val)
		return &floatExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, value: val}, nil
	case lexapi.TOKEN_STRING:
		tok := p.current()
		p.next()
		return &stringExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, value: tok.Value}, nil
	case lexapi.TOKEN_DOTS:
		tok := p.current()
		p.next()
		return &varargExp{baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
	case lexapi.TOKEN_LBRACE:
		return p.parseTableConstructor()
	default:
		return nil, p.errorAt(p.current(), "unexpected symbol in expression")
	}
}

func (p *parser) parseTableConstructor() (astapi.ExpNode, error) {
	tok := p.current()
	p.next() // consume '{'
	tc := &tableConstructor{
		baseNode:     baseNode{line: tok.Line, column: tok.Column},
		arrayFields:  []astapi.ExpNode{},
		recordFields: []struct{ Key, Value astapi.ExpNode }{},
	}
	// Skip table contents for basic test
	for !p.peek(lexapi.TOKEN_RBRACE) && !p.peek(lexapi.TOKEN_EOS) {
		p.next()
	}
	if p.peek(lexapi.TOKEN_RBRACE) {
		p.next()
	}
	return tc, nil
}

// parseExprList parses a comma-separated list of expressions.
// Returns at least one expression, or error.
func (p *parser) parseExprList() ([]astapi.ExpNode, error) {
	exprs := []astapi.ExpNode{}
	for {
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, expr)
		if !p.peek(lexapi.TOKEN_COMMA) {
			break
		}
		p.next() // consume comma
	}
	return exprs, nil
}

// parsePrimaryExpr parses primary expressions: names, literals, parentheses.
func (p *parser) parsePrimaryExpr() (astapi.ExpNode, error) {
	switch p.current().Type {
	case lexapi.TOKEN_NAME:
		name := p.current().Value
		tok := p.current()
		p.next()
		return &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name}, nil
	case lexapi.TOKEN_LPAREN:
		p.next()
		exp, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.peek(lexapi.TOKEN_RPAREN) {
			return nil, p.errorAt(p.current(), "expected ')'")
		}
		p.next()
		return exp, nil
	default:
		return nil, p.errorAt(p.current(), "expected expression")
	}
}

// parseArgs parses function arguments: '(' [exprlist] ')' | table | string
func (p *parser) parseArgs() ([]astapi.ExpNode, error) {
	switch p.current().Type {
	case lexapi.TOKEN_LPAREN:
		p.next() // consume '('
		// Check for empty call: f()
		if p.peek(lexapi.TOKEN_RPAREN) {
			p.next()
			return []astapi.ExpNode{}, nil
		}
		args, err := p.parseExprList()
		if err != nil {
			return nil, err
		}
		if !p.peek(lexapi.TOKEN_RPAREN) {
			return nil, p.errorAt(p.current(), "expected ')'")
		}
		p.next()
		return args, nil
	case lexapi.TOKEN_LBRACE:
		// Table argument: f({...})
		table, err := p.parseTableConstructor()
		if err != nil {
			return nil, err
		}
		return []astapi.ExpNode{table}, nil
	case lexapi.TOKEN_STRING:
		// String argument: f"literal"
		tok := p.current()
		p.next()
		return []astapi.ExpNode{&stringExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, value: tok.Value}}, nil
	default:
		return nil, p.errorAt(p.current(), "expected function arguments")
	}
}

// parseFunctionCall parses a function call: prefix name args / prefix : name args
// Why separate from prefix? Method calls obj:method(args) need to parse ':' after prefix.
func (p *parser) parseFunctionCall(prefix astapi.ExpNode) (astapi.ExpNode, error) {
	tok := p.current()
	var funcNode astapi.ExpNode = prefix

	// Check for method call: obj:method()
	if p.peek(lexapi.TOKEN_COLON) {
		p.next()
		methodName := p.current().Value
		methodTok := p.current()
		p.next()
		// Build obj.method expression
		funcNode = &indexExpr{
			table: prefix,
			key:   &stringExp{baseNode: baseNode{line: methodTok.Line, column: methodTok.Column}, value: methodName},
			baseNode: baseNode{line: tok.Line, column: tok.Column},
		}
	} else if p.peek(lexapi.TOKEN_NAME) {
		// Direct function call: prefix.name()
		name := p.current().Value
		nameTok := p.current()
		p.next()
		funcNode = &indexExpr{
			table: prefix,
			key:   &stringExp{baseNode: baseNode{line: nameTok.Line, column: nameTok.Column}, value: name},
			baseNode: baseNode{line: tok.Line, column: tok.Column},
		}
	}

	// Parse arguments
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}

	return &funcCall{
		baseNode:  baseNode{line: tok.Line, column: tok.Column},
		func_:     funcNode,
		args_:     args,
		numResults: 1,
	}, nil
}

// =============================================================================
// Utility Methods
// =============================================================================

// peek returns true if the current token matches.
func (p *parser) peek(t lexapi.TokenType) bool {
	return p.current().Type == t
}

// peekNext returns true if the next token matches.
func (p *parser) peekNext(t lexapi.TokenType) bool {
	return p.lookahead().Type == t
}

// expect consumes the current token and errors if it doesn't match.
func (p *parser) expect(t lexapi.TokenType) lexapi.Token {
	if p.current().Type != t {
		panic(p.errorAt(p.current(), "%s expected", lexapi.TokenTypeName(t)))
	}
	tok := p.current()
	p.next()
	return tok
}

// consume consumes the current token if it matches.
func (p *parser) consume(t lexapi.TokenType) bool {
	if p.peek(t) {
		p.next()
		return true
	}
	return false
}
