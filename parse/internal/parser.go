// Package internal provides the Lua parser implementation.
package internal

import (
	"fmt"

	astapi "github.com/akzj/go-lua/ast/api"
	lexapi "github.com/akzj/go-lua/lex/api"
	lexpackage "github.com/akzj/go-lua/lex"
	parseapi "github.com/akzj/go-lua/parse/api"
	typesapi "github.com/akzj/go-lua/types/api"
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

// gotoStat implements goto statement.
type gotoStat struct {
	baseNode
	name string
}

func (s *gotoStat) IsScopeEnd() bool           { return false }
func (s *gotoStat) Kind() astapi.StatKind      { return astapi.STAT_GOTO }
func (s *gotoStat) GetName() string            { return s.name }

// labelStat implements label statement (::name::).
type labelStat struct {
	baseNode
	name string
}

func (s *labelStat) IsScopeEnd() bool          { return false }
func (s *labelStat) Kind() astapi.StatKind      { return astapi.STAT_LABEL }
func (s *labelStat) GetName() string            { return s.name }

// globalVarStat implements global variable declaration (Lua 5.4).
type globalVarStat struct {
	baseNode
	name    string
	isConst bool
	exprs   []astapi.ExpNode
}

func (s *globalVarStat) IsScopeEnd() bool           { return false }
func (s *globalVarStat) Kind() astapi.StatKind      { return astapi.STAT_GLOBAL_VAR }
func (s *globalVarStat) GetName() string            { return s.name }
func (s *globalVarStat) IsConst() bool               { return s.isConst }
func (s *globalVarStat) GetExprs() []astapi.ExpNode { return s.exprs }

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

	for !p.peek(lexapi.TOKEN_THEN) && !p.peek(lexapi.TOKEN_END) && !p.peek(lexapi.TOKEN_EOS) && !p.peek(lexapi.TOKEN_ELSEIF) && !p.peek(lexapi.TOKEN_ELSE) && !p.peek(lexapi.TOKEN_UNTIL) && !p.peek(lexapi.TOKEN_BREAK) && !p.peek(lexapi.TOKEN_RETURN) {
		if !p.parseStatement() {
			break
		}
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

	case lexapi.TOKEN_GOTO:
		p.parseGoto()
		return true

	case lexapi.TOKEN_DBCOLON:
		p.parseLabel()
		return true

	case lexapi.TOKEN_GLOBAL:
		p.parseGlobal()
		return true

	case lexapi.TOKEN_RETURN:
		// Return ends block - caller handles it
		return false

	case lexapi.TOKEN_BREAK:
		// Control flow - ends block
		return false

	case lexapi.TOKEN_END, lexapi.TOKEN_EOS, lexapi.TOKEN_ELSEIF, lexapi.TOKEN_ELSE, lexapi.TOKEN_UNTIL:
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
	
	// Save parent block reference before parsing nested blocks
	parentBlock := p.block
	
	// Parse condition
	cond, err := p.parseExpr()
	if err != nil {
		return
	}
	
	if !p.peek(lexapi.TOKEN_THEN) {
		p.errorAt(p.current(), "'then' expected")
		return
	}
	p.next() // consume 'then'
	
	// Parse then block
	thenBlock, err := p.parseBlock()
	p.block = parentBlock // Restore parent block reference
	if err != nil {
		return
	}
	
	// Handle elseif/else chain
	var elseBlock astapi.Block = nil
	
	for p.peek(lexapi.TOKEN_ELSEIF) {
		// Parse elseif block
		p.next() // consume 'elseif'
		elseIfCond, err := p.parseExpr()
		if err != nil {
			return
		}
		if !p.peek(lexapi.TOKEN_THEN) {
			p.errorAt(p.current(), "'then' expected")
			return
		}
		p.next() // consume 'then'
		elseIfBlock, err := p.parseBlock()
		if err != nil {
			return
		}
		// Create nested if for elseif
		nestedIf := &ifStat{
			baseNode: baseNode{line: p.current().Line, column: p.current().Column},
			condition: elseIfCond,
			thenBlock: elseIfBlock,
			elseBlock: elseBlock, // chain to previous elseBlock
		}
		elseBlock = &blockImpl{
			line:   p.current().Line,
			column: p.current().Column,
			stats:  []astapi.StatNode{nestedIf},
		}
	}
	
	if p.peek(lexapi.TOKEN_ELSE) {
		p.next() // consume 'else'
		elseBlock, err = p.parseBlock()
		if err != nil {
			return
		}
	}
	
	if !p.peek(lexapi.TOKEN_END) {
		p.errorAt(p.current(), "'end' expected")
		return
	}
	p.next() // consume 'end'
	
	stat := &ifStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		condition: cond,
		thenBlock: thenBlock,
		elseBlock: elseBlock,
	}
	// Use saved parent block reference
		parentBlock.stats = append(parentBlock.stats, stat)
}

// parseElseIfChain handles elseif/else clauses after the then block
func (p *parser) parseElseIfChain() (astapi.Block, error) {
	block := &blockImpl{
		line:   p.current().Line,
		column: p.current().Column,
		stats:  []astapi.StatNode{},
	}
	
	for p.peek(lexapi.TOKEN_ELSEIF) || p.peek(lexapi.TOKEN_ELSE) {
		if p.peek(lexapi.TOKEN_ELSEIF) {
			p.next() // consume 'elseif'
			cond, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.peek(lexapi.TOKEN_THEN) {
				p.errorAt(p.current(), "'then' expected")
				return nil, p.errorAt(p.current(), "'then' expected")
			}
			p.next() // consume 'then'
			elseIfBlock, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			// Create nested if statement for elseif
			nestedIf := &ifStat{
				baseNode: baseNode{line: p.current().Line, column: p.current().Column},
				condition: cond,
				thenBlock: elseIfBlock,
				elseBlock: nil,
			}
			block.stats = append(block.stats, nestedIf)
			continue
		}
		if p.peek(lexapi.TOKEN_ELSE) {
			p.next() // consume 'else'
			elseBlock, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			block.stats = append(block.stats, &doStat{
				baseNode: baseNode{line: p.current().Line, column: p.current().Column},
				block:    elseBlock,
			})
			break
		}
	}
	
	return block, nil
}

// =============================================================================
// While Statement: while <expr> do <block> end
// =============================================================================

func (p *parser) parseWhile() {
	p.next() // consume 'while'
	
	// Save parent block reference
	parentBlock := p.block
	
	// Parse condition
	cond, err := p.parseExpr()
	if err != nil {
		return
	}
	
	if !p.peek(lexapi.TOKEN_DO) {
		p.errorAt(p.current(), "'do' expected")
		return
	}
	p.next() // consume 'do'
	
	// Parse body block
	body, err := p.parseBlock()
	p.block = parentBlock // Restore parent block reference
	if err != nil {
		return
	}
	
	if !p.peek(lexapi.TOKEN_END) {
		p.errorAt(p.current(), "'end' expected")
		return
	}
	p.next() // consume 'end'
	
	stat := &whileStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		condition: cond,
		block: body,
	}
	// Use saved parent block reference
	parentBlock.stats = append(parentBlock.stats, stat)
}

// =============================================================================
// Do Statement: do <block> end
// =============================================================================

func (p *parser) parseDo() {
	p.next() // consume 'do'
	
	// Save parent block reference
	parentBlock := p.block
	
	// Parse body block
	body, err := p.parseBlock()
	p.block = parentBlock // Restore parent block reference
	if err != nil {
		return
	}
	
	if !p.peek(lexapi.TOKEN_END) {
		p.errorAt(p.current(), "'end' expected")
		return
	}
	p.next() // consume 'end'
	
	stat := &doStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		block: body,
	}
	// Use saved parent block reference
	parentBlock.stats = append(parentBlock.stats, stat)
}

// =============================================================================
// For Statement: for <name> = <expr>, <expr> [, <expr>] do <block> end
//                  for <name> [, <name]* in <expr> [, <expr]*] do <block> end
// =============================================================================

func (p *parser) parseFor() {
	p.next() // consume 'for'
	
	nameTok := p.current()
	if !p.peek(lexapi.TOKEN_NAME) {
		p.errorAt(p.current(), "variable name expected")
		return
	}
	name := p.current().Value
	p.next()
	
	// Save parent block reference
	parentBlock := p.block

	// Check if numeric or generic for
	if p.peek(lexapi.TOKEN_ASSIGN) {
		// Numeric for: for name = start, stop [, step] do body end
		p.next() // consume '='
		
		start, err := p.parseExpr()
		if err != nil {
			return
		}
		
		if !p.peek(lexapi.TOKEN_COMMA) {
			p.errorAt(p.current(), "',' expected")
			return
		}
		p.next() // consume ','
		
		stop, err := p.parseExpr()
		if err != nil {
			return
		}
		
		var step astapi.ExpNode = nil
		if p.peek(lexapi.TOKEN_COMMA) {
			p.next() // consume ','
			step, err = p.parseExpr()
			if err != nil {
				return
			}
		}
		
		if !p.peek(lexapi.TOKEN_DO) {
			p.errorAt(p.current(), "'do' expected")
			return
		}
		p.next() // consume 'do'
		
		body, err := p.parseBlock()
		p.block = parentBlock // Restore parent block reference
		if err != nil {
			return
		}
		
		if !p.peek(lexapi.TOKEN_END) {
			p.errorAt(p.current(), "'end' expected")
			return
		}
		p.next() // consume 'end'
		
		stat := &forNumStat{
			baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
			name: name,
			start: start,
			stop: stop,
			step: step,
			block: body,
		}
		// Use saved parent block reference
		parentBlock.stats = append(parentBlock.stats, stat)
	} else {
		// Generic for: for names in exprs do body end
		var names []string
		names = append(names, name)
		
		for p.peek(lexapi.TOKEN_COMMA) {
			p.next() // consume ','
			if !p.peek(lexapi.TOKEN_NAME) {
				p.errorAt(p.current(), "variable name expected")
				return
			}
			names = append(names, p.current().Value)
			p.next()
		}
		
		if !p.peek(lexapi.TOKEN_IN) {
			p.errorAt(p.current(), "'in' expected")
			return
		}
		p.next() // consume 'in'
		
		exprs, err := p.parseExprList()
		if err != nil {
			return
		}
		
		if !p.peek(lexapi.TOKEN_DO) {
			p.errorAt(p.current(), "'do' expected")
			return
		}
		p.next() // consume 'do'
		
		body, err := p.parseBlock()
		p.block = parentBlock // Restore parent block reference
		if err != nil {
			return
		}
		
		if !p.peek(lexapi.TOKEN_END) {
			p.errorAt(p.current(), "'end' expected")
			return
		}
		p.next() // consume 'end'
		
		stat := &forInStat{
			baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
			names: names,
			exprs: exprs,
			block: body,
		}
		// Use saved parent block reference
		parentBlock.stats = append(parentBlock.stats, stat)
	}
}

// =============================================================================
// Repeat Statement: repeat <block> until <expr>
// =============================================================================

func (p *parser) parseRepeat() {
	p.next() // consume 'repeat'
	
	// Save parent block reference
	parentBlock := p.block
	
	// Parse body block
	body, err := p.parseBlock()
	p.block = parentBlock // Restore parent block reference
	if err != nil {
		return
	}
	
	if !p.peek(lexapi.TOKEN_UNTIL) {
		p.errorAt(p.current(), "'until' expected")
		return
	}
	p.next() // consume 'until'
	
	// Parse condition
	cond, err := p.parseExpr()
	if err != nil {
		return
	}
	
	stat := &repeatStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		block: body,
		condition: cond,
	}
	// Use saved parent block reference
	parentBlock.stats = append(parentBlock.stats, stat)
}

// =============================================================================
// Function Definition: function <name> ['.' <name>]* [':' <name>] '(' [params] ')' <block> end
// =============================================================================

func (p *parser) parseFunctionDef(isLocal bool) {
	p.next() // consume 'function' (or skip 'local function')
	
	// Save parent block reference
	parentBlock := p.block
	
	// Parse function name
	name := p.current().Value
	nameTok := p.current()
	p.next()
	
	// Parse parameters
	if !p.peek(lexapi.TOKEN_LPAREN) {
		p.errorAt(p.current(), "'(' expected")
		return
	}
	p.next() // consume '('
	
	// Skip to ')' - for stub, we don't need to parse parameters
	// Fixed: don't consume token after closing paren
	depth := 1
	for {
		if p.peek(lexapi.TOKEN_LPAREN) {
			depth++
			p.next()
		} else if p.peek(lexapi.TOKEN_RPAREN) {
			depth--
			p.next() // consume ')'
			if depth == 0 {
				break // stop after consuming ')'
			}
		} else if p.peek(lexapi.TOKEN_EOS) {
			break
		} else {
			p.next()
		}
	}
	
	if p.peek(lexapi.TOKEN_EOS) || p.current().Type == lexapi.TOKEN_EOS {
		p.errorAt(p.current(), "'(' expected")
		return
	}
	
	// Parse function body block
	// Save outer block reference since parseBlock will set p.block = nil
	savedBlock := p.block
	body, err := p.parseBlock()
	p.block = savedBlock // Restore outer block reference
	if err != nil {
		return
	}
	
	
	if !p.peek(lexapi.TOKEN_END) {
		p.errorAt(p.current(), "'end' expected")
		return
	}
	p.next() // consume 'end'
	
	stat := &globalFuncStat{
		baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
		name:     name,
		func_: &funcDefImpl{
			baseNode:  baseNode{line: nameTok.Line, column: nameTok.Column},
			isLocal:   false,
			params:    []string{},
			varArg:    false,
			block:     body,
			lastLine:  nameTok.Line,
		},
	}
	parentBlock.stats = append(parentBlock.stats, stat)
}

// funcDefImpl is a stub FuncDef implementation
type funcDefImpl struct {
	baseNode
	isLocal bool
	params  []string
	varArg  bool
	block   astapi.Block
	lastLine int
}

func (f *funcDefImpl) IsLocal() bool              { return f.isLocal }
func (f *funcDefImpl) Line() int                 { return f.line }
func (f *funcDefImpl) LastLine() int             { return f.lastLine }
func (f *funcDefImpl) GetParams() []string       { return f.params }
func (f *funcDefImpl) IsVarArg() bool            { return f.varArg }
func (f *funcDefImpl) GetBlock() astapi.Block    { return f.block }
func (f *funcDefImpl) Proto() *typesapi.Proto    { return nil }

// =============================================================================
// Local Function: local function <name> '(' [params] ')' <block> end
// =============================================================================

func (p *parser) parseLocalFunction() {
	p.next() // consume 'local'
	p.next() // consume 'function'
	
	// Save parent block reference
	parentBlock := p.block
	
	// Parse function name
	name := p.current().Value
	nameTok := p.current()
	p.next()
	
	// Parse parameters
	if !p.peek(lexapi.TOKEN_LPAREN) {
		p.errorAt(p.current(), "'(' expected")
		return
	}
	p.next() // consume '('
	
	// Skip to ')' - for stub, we don't need to parse parameters
	// Fixed: don't consume token after closing paren
	depth := 1
	for {
		if p.peek(lexapi.TOKEN_LPAREN) {
			depth++
			p.next()
		} else if p.peek(lexapi.TOKEN_RPAREN) {
			depth--
			p.next() // consume ')'
			if depth == 0 {
				break // stop after consuming ')'
			}
		} else if p.peek(lexapi.TOKEN_EOS) {
			break
		} else {
			p.next()
		}
	}
	
	if p.peek(lexapi.TOKEN_EOS) || p.current().Type == lexapi.TOKEN_EOS {
		p.errorAt(p.current(), "'(' expected")
		return
	}
	
	// Parse function body block
	// Save outer block reference since parseBlock will set p.block = nil
	savedBlock := p.block
	body, err := p.parseBlock()
	p.block = savedBlock // Restore outer block reference
	if err != nil {
		return
	}
	
	
	if !p.peek(lexapi.TOKEN_END) {
		p.errorAt(p.current(), "'end' expected")
		return
	}
	p.next() // consume 'end'
	
	stat := &localFuncStat{
		baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
		name:     name,
		func_: &funcDefImpl{
			baseNode:  baseNode{line: nameTok.Line, column: nameTok.Column},
			isLocal:   false,
			params:    []string{},
			varArg:    false,
			block:     body,
			lastLine:  nameTok.Line,
		},
	}
		parentBlock.stats = append(parentBlock.stats, stat)
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
		p.next() // consume '='
		
		// Parse expression using full expression parser
		expr, err := p.parseExpr()
		if err != nil {
			return
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
// Goto Statement: goto <name>
// =============================================================================

func (p *parser) parseGoto() {
	p.next() // consume 'goto'
	name := p.current().Value
	tok := p.current()
	p.next()
	
	stat := &gotoStat{
		baseNode: baseNode{line: tok.Line, column: tok.Column},
		name:     name,
	}
	p.block.stats = append(p.block.stats, stat)
}

// =============================================================================
// Label Statement: ::<name>::
// =============================================================================

func (p *parser) parseLabel() {
	// p.current() is :: (TOKEN_DBCOLON) - already consumed by switch case
	// p.look is the NAME token (label name)
	// p.look.look (lookahead) is the closing ::
	
	// Validate that the next token is a NAME
	if p.look.Type != lexapi.TOKEN_NAME {
		return
	}
	
	// First next() consumes ::, moves NAME to cur, closing :: to look
	p.next() // consume :: (now cur=NAME, look=closing ::)
	
	// Now peek at look to see if it's the closing ::
	if p.look.Type != lexapi.TOKEN_DBCOLON {
		return
	}
	
	name := p.current().Value
	nameTok := p.current()
	p.next() // consume NAME (now cur=NAME, look=closing ::)
	p.next() // consume closing ::
	
	stat := &labelStat{
		baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
		name:     name,
	}
	p.block.stats = append(p.block.stats, stat)
}

// =============================================================================
// Global Statement: global [<const>] <name> = <expr> | global [<const>] * = <expr>
// Lua 5.4 feature: global const, global const *
// =============================================================================

func (p *parser) parseGlobal() {
	p.next() // consume 'global'
	
	parentBlock := p.block
	
	// Check for Lua 5.5 <const> attribute syntax: global <const> ...
	isConst := false
	if p.peek(lexapi.TOKEN_LT) {
		p.next() // consume '<'
		// Expect 'const' keyword (as NAME) or 'close'
		if p.peek(lexapi.TOKEN_NAME) && p.current().Value == "const" {
			isConst = true
			p.next() // consume 'const'
		} else if p.peek(lexapi.TOKEN_NAME) && p.current().Value == "close" {
			// Lua 5.4 <close> attribute - just consume
			p.next() // consume 'close'
		} else {
			p.errorAt(p.current(), "expected 'const' or 'close'")
			return
		}
		if !p.peek(lexapi.TOKEN_GT) {
			p.errorAt(p.current(), "expected '>'")
			return
		}
		p.next() // consume '>'
	} else if p.peek(lexapi.TOKEN_CONST) {
		// Lua 5.4 "global const" syntax
		isConst = true
		p.next() // consume 'const'
	}
	
	// Check for '*' export-all modifier: global <const> * or global *
	if p.peek(lexapi.TOKEN_MUL) {
		p.next() // consume '*'
		if p.peek(lexapi.TOKEN_ASSIGN) {
			p.next()
			_, err := p.parseExprList()
			if err != nil {
				return
			}
		}
		return
	}
	
	// Parse comma-separated name list: global a, b, c [= exprs]
	for {
		if p.peek(lexapi.TOKEN_NAME) {
			name := p.current().Value
			nameTok := p.current()
			p.next()
			
			if p.peek(lexapi.TOKEN_ASSIGN) {
				p.next()
				exprs, err := p.parseExprList()
				if err != nil {
					return
				}
				stat := &globalVarStat{
					baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
					name:     name,
					isConst:  isConst,
					exprs:    exprs,
				}
				parentBlock.stats = append(parentBlock.stats, stat)
			}
		}
		
		if !p.peek(lexapi.TOKEN_COMMA) {
			break
		}
		p.next() // consume ','
	}
}

// =============================================================================
// Assignment or Function Call
// =============================================================================

func (p *parser) parseAssignmentOrCall() bool {
	// Check if this looks like a valid expression start
	switch p.current().Type {
	case lexapi.TOKEN_NAME, lexapi.TOKEN_INTEGER, lexapi.TOKEN_NUMBER, lexapi.TOKEN_STRING,
		lexapi.TOKEN_LPAREN, lexapi.TOKEN_LBRACE, lexapi.TOKEN_NIL, lexapi.TOKEN_TRUE,
		lexapi.TOKEN_FALSE, lexapi.TOKEN_MINUS, lexapi.TOKEN_NOT, lexapi.TOKEN_DOTS:
		// Valid expression start
	default:
		// Not a valid expression - return false to let caller handle block end
		return false
	}

	// For TOKEN_NAME, check if this is a function call or expression statement
	if p.current().Type == lexapi.TOKEN_NAME {
		name := p.current().Value
		tok := p.current()
		
		// Check if this is a function call: name(args) or name"string" or name{...}
		if p.peek(lexapi.TOKEN_LPAREN) {
			// Function call: name(args)
			p.next() // consume '('
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

		// Assignment: x = expr
		if p.peek(lexapi.TOKEN_ASSIGN) {
			p.next()
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

		// Expression statement with name: a + b, a == b, a.b, etc.
		// Consume the name and start building expression
		p.next() // consume name
		left := &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name}
		
		// Check if there's a suffix (method call, index, etc.)
		expr := p.handleSuffixLoop(left)
		
		// Check for binary operators after the expression
		expr = p.handleBinaryOps(expr, tok.Line, tok.Column)
		
		stat := &expressionStat{
			baseNode: baseNode{line: tok.Line, column: tok.Column},
			expr:     expr,
		}
		p.block.stats = append(p.block.stats, stat)
		return true
	}

	// For non-name expressions (literals, unary, etc.)
	// Parse full expression
	expr, err := p.parseExpr()
	if err != nil {
		return false
	}
	stat := &expressionStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		expr:     expr,
	}
	p.block.stats = append(p.block.stats, stat)
	return true
}

// handleSuffixLoop handles method calls, indexing, and function calls on expressions.
// After parsing a primary expression, we loop to handle suffixes like:
// - .method() (method call)
// - [index] (table indexing)
// - (args) (function call with parens)
// - "string" (function call with string arg)
func (p *parser) handleSuffixLoop(expr astapi.ExpNode) astapi.ExpNode {
	for {
		switch p.current().Type {
		case lexapi.TOKEN_DOT:
			// Table field access: expr.field
			p.next() // consume '.'
			if p.current().Type != lexapi.TOKEN_NAME {
				return expr
			}
			fieldName := p.current().Value
			fieldTok := p.current()
			p.next()
			expr = &indexExpr{
				baseNode: baseNode{line: fieldTok.Line, column: fieldTok.Column},
				table:    expr,
				key:      &stringExp{baseNode: baseNode{line: fieldTok.Line, column: fieldTok.Column}, value: fieldName},
			}
		case lexapi.TOKEN_LBRACK:
			// Table indexing: expr[key]
			p.next() // consume '['
			key, err := p.parseExpr()
			if err != nil {
				return expr
			}
			if !p.peek(lexapi.TOKEN_RBRACK) {
				return expr
			}
			p.next() // consume ']'
			expr = &indexExpr{
				baseNode: baseNode{line: p.current().Line, column: p.current().Column},
				table:    expr,
				key:      key,
			}
		case lexapi.TOKEN_COLON:
			// Method call: expr:method(args) -> expr.method(self, args)
			p.next() // consume ':'
			if p.current().Type != lexapi.TOKEN_NAME {
				return expr
			}
			methodName := p.current().Value
			methodTok := p.current()
			p.next()
			// Build: expr.method(self, args)
			var args []astapi.ExpNode
			args = append(args, expr) // self
			if p.peek(lexapi.TOKEN_LPAREN) {
				p.next() // consume '('
				if !p.peek(lexapi.TOKEN_RPAREN) {
					argList, err := p.parseExprList()
					if err == nil {
						args = append(args, argList...)
					}
				}
				if p.peek(lexapi.TOKEN_RPAREN) {
					p.next() // consume ')'
				}
			} else if p.peek(lexapi.TOKEN_STRING) {
				args = append(args, &stringExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, value: p.current().Value})
				p.next()
			} else if p.peek(lexapi.TOKEN_LBRACE) {
				table, _ := p.parseTableConstructor()
				if table != nil {
					args = append(args, table)
				}
			}
			expr = &funcCall{
				baseNode:   baseNode{line: methodTok.Line, column: methodTok.Column},
				func_:      &indexExpr{baseNode: baseNode{line: methodTok.Line, column: methodTok.Column}, table: expr, key: &stringExp{baseNode: baseNode{line: methodTok.Line, column: methodTok.Column}, value: methodName}},
				args_:      args,
				numResults: 1,
			}
		case lexapi.TOKEN_LPAREN:
			// Function call: expr(args)
			p.next() // consume '('
			var args []astapi.ExpNode
			if !p.peek(lexapi.TOKEN_RPAREN) {
				args, _ = p.parseExprList()
			}
			if p.peek(lexapi.TOKEN_RPAREN) {
				p.next() // consume ')'
			}
			expr = &funcCall{
				baseNode:   baseNode{line: p.current().Line, column: p.current().Column},
				func_:      expr,
				args_:      args,
				numResults: 1,
			}
		case lexapi.TOKEN_STRING:
			// Function call with string arg: expr "string"
			strVal := p.current().Value
			strTok := p.current()
			p.next()
			expr = &funcCall{
				baseNode:   baseNode{line: strTok.Line, column: strTok.Column},
				func_:      expr,
				args_:      []astapi.ExpNode{&stringExp{baseNode: baseNode{line: strTok.Line, column: strTok.Column}, value: strVal}},
				numResults: 1,
			}
		case lexapi.TOKEN_LBRACE:
			// Function call with table arg: expr {1, 2, 3}
			table, _ := p.parseTableConstructor()
			if table != nil {
				expr = &funcCall{
					baseNode:   baseNode{line: p.current().Line, column: p.current().Column},
					func_:      expr,
					args_:      []astapi.ExpNode{table},
					numResults: 1,
				}
			}
		default:
			// No more suffixes
			return expr
		}
	}
}

// handleBinaryOps handles binary operators after an expression.
// Returns the combined expression or the original if no operator found.
func (p *parser) handleBinaryOps(left astapi.ExpNode, line, column int) astapi.ExpNode {
	for {
		var op astapi.BinopKind
		var isBinary bool
		
		switch p.current().Type {
		case lexapi.TOKEN_PLUS:
			op = astapi.BINOP_ADD
			isBinary = true
		case lexapi.TOKEN_MINUS:
			op = astapi.BINOP_SUB
			isBinary = true
		case lexapi.TOKEN_MUL:
			op = astapi.BINOP_MUL
			isBinary = true
		case lexapi.TOKEN_DIV:
			op = astapi.BINOP_DIV
			isBinary = true
		case lexapi.TOKEN_EQ:
			op = astapi.BINOP_EQ
			isBinary = true
		case lexapi.TOKEN_NE:
			op = astapi.BINOP_NE
			isBinary = true
		case lexapi.TOKEN_LT:
			op = astapi.BINOP_LT
			isBinary = true
		case lexapi.TOKEN_LE:
			op = astapi.BINOP_LE
			isBinary = true
		case lexapi.TOKEN_GT:
			op = astapi.BINOP_GT
			isBinary = true
		case lexapi.TOKEN_GE:
			op = astapi.BINOP_GE
			isBinary = true
		case lexapi.TOKEN_AND:
			op = astapi.BINOP_AND
			isBinary = true
		case lexapi.TOKEN_OR:
			op = astapi.BINOP_OR
			isBinary = true
		default:
			isBinary = false
		}
		
		if !isBinary {
			return left
		}
		
		tok := p.current()
		p.next() // consume operator
		right, err := p.parseExpr()
		if err != nil {
			return left
		}
		
		left = &binopExp{
			op:       op,
			left:     left,
			right:    right,
			baseNode: baseNode{line: tok.Line, column: tok.Column},
		}
	}
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
	left, err := p.parseBitwiseOr()
	if err != nil {
		return nil, err
	}
	for p.current().Type >= lexapi.TOKEN_LT && p.current().Type <= lexapi.TOKEN_NE {
		op := p.mapComparisonOp(p.current().Type)
		tok := p.current()
		p.next()
		right, err := p.parseBitwiseOr()
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

// =============================================================================
// Bitwise Operators (Lua 5.3+)
// Precedence: | (lowest) ~ (bxor) & (highest) << >>
// =============================================================================

func (p *parser) parseBitwiseOr() (astapi.ExpNode, error) {
	left, err := p.parseBitwiseXor()
	if err != nil {
		return nil, err
	}
	for p.peek(lexapi.TOKEN_PIPE) { // |
		tok := p.current()
		p.next()
		right, err := p.parseBitwiseXor()
		if err != nil {
			return nil, err
		}
		left = &binopExp{op: astapi.BINOP_BOR, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
	}
	return left, nil
}

func (p *parser) parseBitwiseXor() (astapi.ExpNode, error) {
	left, err := p.parseBitwiseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek(lexapi.TOKEN_TILDE) { // ~ (bxor in Lua 5.3+)
		tok := p.current()
		p.next()
		right, err := p.parseBitwiseAnd()
		if err != nil {
			return nil, err
		}
		left = &binopExp{op: astapi.BINOP_BXOR, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
	}
	return left, nil
}

func (p *parser) parseBitwiseAnd() (astapi.ExpNode, error) {
	left, err := p.parseShift()
	if err != nil {
		return nil, err
	}
	for p.peek(lexapi.TOKEN_AMP) { // &
		tok := p.current()
		p.next()
		right, err := p.parseShift()
		if err != nil {
			return nil, err
		}
		left = &binopExp{op: astapi.BINOP_BAND, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}
	}
	return left, nil
}

func (p *parser) parseShift() (astapi.ExpNode, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	for p.peek(lexapi.TOKEN_SHL) || p.peek(lexapi.TOKEN_SHR) { // << >>
		var op astapi.BinopKind
		if p.peek(lexapi.TOKEN_SHL) {
			op = astapi.BINOP_SHL
		} else {
			op = astapi.BINOP_SHR
		}
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
	// Parse the first part (prefix)
	var expr astapi.ExpNode
	var err error


	switch p.current().Type {
	case lexapi.TOKEN_NAME:
		name := p.current().Value
		tok := p.current()
		p.next()
		expr = &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name}

	case lexapi.TOKEN_LPAREN:
		p.next()
		// Check for empty parentheses "()"
		if p.peek(lexapi.TOKEN_RPAREN) {
			p.next()
			expr = &tableConstructor{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, arrayFields: []astapi.ExpNode{}, recordFields: nil}
		} else {
			exp, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.peek(lexapi.TOKEN_RPAREN) {
				return nil, p.errorAt(p.current(), "expected ')'")
			}
			p.next()
			expr = exp
		}

	case lexapi.TOKEN_NIL:
		tok := p.current()
		p.next()
		expr = &nilExp{baseNode: baseNode{line: tok.Line, column: tok.Column}}

	case lexapi.TOKEN_TRUE:
		tok := p.current()
		p.next()
		expr = &trueExp{baseNode: baseNode{line: tok.Line, column: tok.Column}}

	case lexapi.TOKEN_FALSE:
		tok := p.current()
		p.next()
		expr = &falseExp{baseNode: baseNode{line: tok.Line, column: tok.Column}}

	case lexapi.TOKEN_INTEGER:
		tok := p.current()
		p.next()
		var val int64
		fmt.Sscanf(tok.Value, "%d", &val)
		expr = &integerExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, value: val}

	case lexapi.TOKEN_NUMBER:
		tok := p.current()
		p.next()
		var val float64
		fmt.Sscanf(tok.Value, "%f", &val)
		expr = &floatExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, value: val}

	case lexapi.TOKEN_STRING:
		tok := p.current()
		p.next()
		expr = &stringExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, value: tok.Value}

	case lexapi.TOKEN_DOTS:
		tok := p.current()
		p.next()
		expr = &varargExp{baseNode: baseNode{line: tok.Line, column: tok.Column}}

	case lexapi.TOKEN_LBRACE:
		return p.parseTableConstructor()

	default:
		return nil, p.errorAt(p.current(), "unexpected symbol in expression")
	}

	// For literals (true, false, nil, numbers, strings), return immediately
	// Only prefix expressions (NAME, LPAREN) can have suffixes
	if _, ok := expr.(*trueExp); ok {
		return expr, nil
	}
	if _, ok := expr.(*falseExp); ok {
		return expr, nil
	}
	if _, ok := expr.(*nilExp); ok {
		return expr, nil
	}
	if _, ok := expr.(*integerExp); ok {
		return expr, nil
	}
	if _, ok := expr.(*floatExp); ok {
		return expr, nil
	}
	if _, ok := expr.(*stringExp); ok {
		return expr, nil
	}
	if _, ok := expr.(*varargExp); ok {
		return expr, nil
	}

	// Handle suffixes: function calls, index access, field access
	// Loop to handle chained operations like table.concat({})
	for {
		if p.peek(lexapi.TOKEN_LPAREN) {
			// Function call: expr(args)
			p.next() // consume '('
			var args []astapi.ExpNode
			if !p.peek(lexapi.TOKEN_RPAREN) {
				args, err = p.parseExprList()
				if err != nil {
					return nil, err
				}
			}
			if !p.peek(lexapi.TOKEN_RPAREN) {
				return nil, p.errorAt(p.current(), "expected ')'")
			}
			p.next() // consume ')'
			expr = &funcCall{
				baseNode:   baseNode{line: p.current().Line, column: p.current().Column},
				func_:      expr,
				args_:      args,
				numResults:  1,
			}
		} else if p.peek(lexapi.TOKEN_DOT) {
			// Field access: expr.field
			p.next() // consume '.'
			fieldName := p.current().Value
			fieldTok := p.current()
			p.next()
			expr = &indexExpr{
				table:    expr,
				key:      &stringExp{baseNode: baseNode{line: fieldTok.Line, column: fieldTok.Column}, value: fieldName},
				baseNode: baseNode{line: fieldTok.Line, column: fieldTok.Column},
			}
		} else if p.peek(lexapi.TOKEN_LBRACK) {
			// Index access: expr[index]
			p.next() // consume '['
			index, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.peek(lexapi.TOKEN_RBRACK) {
				return nil, p.errorAt(p.current(), "expected ']'")
			}
			p.next() // consume ']'
			expr = &indexExpr{
				table:     expr,
				key:       index,
				baseNode:  baseNode{line: p.current().Line, column: p.current().Column},
			}
		} else if p.peek(lexapi.TOKEN_COLON) {
			// Method call: expr:method(args)
			p.next() // consume ':'
			methodName := p.current().Value
			methodTok := p.current()
			p.next()
			// Build obj.method expression
			methodExpr := &indexExpr{
				table:    expr,
				key:      &stringExp{baseNode: baseNode{line: methodTok.Line, column: methodTok.Column}, value: methodName},
				baseNode: baseNode{line: methodTok.Line, column: methodTok.Column},
			}
			// Now parse arguments
			if !p.peek(lexapi.TOKEN_LPAREN) {
				return nil, p.errorAt(p.current(), "expected '(' after method name")
			}
			p.next() // consume '('
			var args []astapi.ExpNode
			if !p.peek(lexapi.TOKEN_RPAREN) {
				args, err = p.parseExprList()
				if err != nil {
					return nil, err
				}
			}
			if !p.peek(lexapi.TOKEN_RPAREN) {
				return nil, p.errorAt(p.current(), "expected ')'")
			}
			p.next() // consume ')'
			expr = &funcCall{
				baseNode:   baseNode{line: methodTok.Line, column: methodTok.Column},
				func_:      methodExpr,
				args_:      args,
				numResults:  1,
			}
		} else {
			// No more suffixes
			break
		}
	}

	return expr, nil
}

func (p *parser) parseTableConstructor() (astapi.ExpNode, error) {
	tok := p.current()
	p.next() // consume '{'
	tc := &tableConstructor{
		baseNode:     baseNode{line: tok.Line, column: tok.Column},
		arrayFields:  []astapi.ExpNode{},
		recordFields: []struct{ Key, Value astapi.ExpNode }{},
	}

	// Parse table fields until '}'
	for !p.peek(lexapi.TOKEN_RBRACE) && !p.peek(lexapi.TOKEN_EOS) {
		// Check for field separator or end
		if p.peek(lexapi.TOKEN_COMMA) || p.peek(lexapi.TOKEN_SEMICOLON) {
			p.next() // consume separator
			// Check for immediate '}' after separator
			if p.peek(lexapi.TOKEN_RBRACE) {
				break
			}
			continue
		}

		// Check for '[' which indicates explicit key: [expr] = value
		if p.peek(lexapi.TOKEN_LBRACK) {
			p.next() // consume '['
			key, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.peek(lexapi.TOKEN_RBRACK) {
				return nil, p.errorAt(p.current(), "expected ']'")
			}
			p.next() // consume ']'
			if !p.peek(lexapi.TOKEN_ASSIGN) {
				return nil, p.errorAt(p.current(), "expected '='")
			}
			p.next() // consume '='
			value, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			tc.AddRecordField(key, value)
			continue
		}

		// Check for name followed by '=' which indicates record field: name = value
		if p.peek(lexapi.TOKEN_NAME) && p.peekNext(lexapi.TOKEN_ASSIGN) {
			keyName := p.current().Value
			keyTok := p.current()
			p.next() // consume name
			p.next() // consume '='
			value, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			tc.AddRecordField(&stringExp{baseNode: baseNode{line: keyTok.Line, column: keyTok.Column}, value: keyName}, value)
			continue
		}

		// Otherwise it's an array field
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		tc.AddArrayField(expr)
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
