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

	// Parse top-level block(s) - handle orphan 'end' at top level
	var firstBlock astapi.Block
	for {
		block, err := p.parseBlock()
		if err != nil {
			return nil, err
		}

		if firstBlock == nil {
			firstBlock = block
		}

		// If we got an empty block (orphan 'end' at top level), parse another
		if len(block.Stats()) == 0 && block.ReturnExp() == nil {
			// Consume trailing orphan 'end' if present
			if p.peek(lexapi.TOKEN_END) {
				p.next()
				// Consume trailing semicolons
				for p.peek(lexapi.TOKEN_SEMICOLON) {
					p.next()
				}
				// Continue parsing if not at EOS
				if p.peek(lexapi.TOKEN_EOS) {
					break
				}
				continue
			}
		}

		// Check for trailing garbage
		if p.cur.Type != lexapi.TOKEN_EOS {
			return nil, p.errorAt(p.cur, "unexpected symbol")
		}
		break
	}

	return &chunkImpl{
		block:      firstBlock,
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
func (s *localVarStat) GetNames() []string    { return s.names }
func (s *localVarStat) GetExprs() []astapi.ExpNode { return s.exprs }

// localFuncStat implements local function declaration.
type localFuncStat struct {
	baseNode
	name string
	func_ astapi.FuncDef
}

func (s *localFuncStat) IsScopeEnd() bool   { return false }
func (s *localFuncStat) Kind() astapi.StatKind { return astapi.STAT_LOCAL_FUNC }
func (s *localFuncStat) GetFuncDef() astapi.FuncDef { return s.func_ }
func (s *localFuncStat) GetName() string { return s.name }

// globalFuncStat implements global function declaration.
type globalFuncStat struct {
	baseNode
	name     string
	func_    astapi.FuncDef
	isMethod bool // true if defined with colon syntax (function a:m())
}

func (s *globalFuncStat) IsScopeEnd() bool   { return true }
func (s *globalFuncStat) Kind() astapi.StatKind { return astapi.STAT_GLOBAL_FUNC }
func (s *globalFuncStat) GetFuncDef() astapi.FuncDef { return s.func_ }
func (s *globalFuncStat) GetName() string { return s.name }
func (s *globalFuncStat) IsMethod() bool { return s.isMethod }

// returnStat implements return statement.
type returnStat struct {
	baseNode
	exprs []astapi.ExpNode
}

func (s *returnStat) IsScopeEnd() bool   { return true }
func (s *returnStat) Kind() astapi.StatKind { return astapi.STAT_RETURN }
func (s *returnStat) GetExprs() []astapi.ExpNode { return s.exprs }

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
func (s *ifStat) GetCondition() astapi.ExpNode { return s.condition }
func (s *ifStat) GetThenBlock() astapi.Block { return s.thenBlock }
func (s *ifStat) GetElseBlock() astapi.Block { return s.elseBlock }

type whileStat struct {
	baseNode
	condition astapi.ExpNode
	block    astapi.Block
}

func (s *whileStat) IsScopeEnd() bool   { return false }
func (s *whileStat) Kind() astapi.StatKind { return astapi.STAT_WHILE }
func (s *whileStat) GetCondition() astapi.ExpNode { return s.condition }
func (s *whileStat) GetBlock() astapi.Block { return s.block }

type doStat struct {
	baseNode
	block astapi.Block
}

func (s *doStat) IsScopeEnd() bool   { return false }
func (s *doStat) Kind() astapi.StatKind { return astapi.STAT_DO }
func (s *doStat) GetBlock() astapi.Block { return s.block }

type repeatStat struct {
	baseNode
	block     astapi.Block
	condition astapi.ExpNode
}

func (s *repeatStat) IsScopeEnd() bool   { return true }
func (s *repeatStat) Kind() astapi.StatKind { return astapi.STAT_REPEAT }
func (s *repeatStat) GetBlock() astapi.Block      { return s.block }
func (s *repeatStat) GetCondition() astapi.ExpNode { return s.condition }

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
func (s *forNumStat) GetName() string    { return s.name }
func (s *forNumStat) GetStart() astapi.ExpNode { return s.start }
func (s *forNumStat) GetStop() astapi.ExpNode  { return s.stop }
func (s *forNumStat) GetStep() astapi.ExpNode  { return s.step }
func (s *forNumStat) GetBlock() astapi.Block   { return s.block }

type forInStat struct {
	baseNode
	names []string
	exprs []astapi.ExpNode
	block astapi.Block
}

func (s *forInStat) IsScopeEnd() bool   { return false }
func (s *forInStat) Kind() astapi.StatKind { return astapi.STAT_FOR_IN }
func (s *forInStat) GetNames() []string    { return s.names }
func (s *forInStat) GetExprs() []astapi.ExpNode { return s.exprs }
func (s *forInStat) GetBlock() astapi.Block   { return s.block }

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
func (e *indexExpr) GetTable() astapi.ExpNode { return e.table }
func (e *indexExpr) GetKey() astapi.ExpNode { return e.key }

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

// Accessor methods for binary expression
func (e *binopExp) GetOp() astapi.BinopKind  { return e.op }
func (e *binopExp) GetLeft() astapi.ExpNode  { return e.left }
func (e *binopExp) GetRight() astapi.ExpNode { return e.right }

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
func (e *unopExp) GetUnaryOp() (astapi.UnopKind, astapi.ExpNode) { return e.op, e.exp }

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
func (t *tableConstructor) GetArrayField(i int) astapi.ExpNode {
	if i >= 0 && i < len(t.arrayFields) { return t.arrayFields[i] }
	return nil
}
func (t *tableConstructor) GetRecordField(i int) (astapi.ExpNode, astapi.ExpNode) {
	if i >= 0 && i < len(t.recordFields) { return t.recordFields[i].Key, t.recordFields[i].Value }
	return nil, nil
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
	// Save outer block reference since parseBlock will set p.block = nil
	outerBlock := p.block
	
	block := &blockImpl{
		line:   p.current().Line,
		column: p.current().Column,
		stats:  []astapi.StatNode{},
	}
	p.block = block

	// NOTE: TOKEN_END, TOKEN_ELSEIF, TOKEN_ELSE, TOKEN_UNTIL must be in the loop
	// condition to stop parsing when these tokens are encountered. Otherwise,
	// parseStatement() gets called with these tokens and fails.
	// TOKEN_BREAK and TOKEN_RETURN are handled by parseStatement() directly.
	for !p.peek(lexapi.TOKEN_THEN) && !p.peek(lexapi.TOKEN_END) && !p.peek(lexapi.TOKEN_EOS) && !p.peek(lexapi.TOKEN_ELSEIF) && !p.peek(lexapi.TOKEN_ELSE) && !p.peek(lexapi.TOKEN_UNTIL) {
		if !p.parseStatement() {
			break
		}
	}

	// Handle return statement at end of block
	if p.peek(lexapi.TOKEN_RETURN) {
		block.returnExp = p.parseReturn()
	}

	// Restore outer block reference
	p.block = outerBlock
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
		// break statement - consume token and add to block
		tok := p.current()
		p.next() // consume 'break'
		stat := &breakStat{
			baseNode: baseNode{line: tok.Line, column: tok.Column},
		}
		p.block.stats = append(p.block.stats, stat)
		return true

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
	if err != nil {
		p.block = parentBlock // Restore before returning on error
		return
	}
	p.block = parentBlock // Restore parent block reference AFTER block is complete
	
	// Handle elseif/else chain (forward-building)
	elseBlock := p.parseElseIfOrElse()
	
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

// parseElseIfOrElse builds the elseif/else chain FORWARD via recursion.
// Each elseif becomes a nested ifStat whose else is the REST of the chain.
func (p *parser) parseElseIfOrElse() astapi.Block {
	if p.peek(lexapi.TOKEN_ELSEIF) {
		p.next() // consume 'elseif'
		cond, err := p.parseExpr()
		if err != nil {
			return nil
		}
		if !p.peek(lexapi.TOKEN_THEN) {
			p.errorAt(p.current(), "'then' expected")
			return nil
		}
		p.next() // consume 'then'
		parentBlock := p.block
		thenBlock, err := p.parseBlock()
		if err != nil {
			p.block = parentBlock
			return nil
		}
		p.block = parentBlock
		// Recurse: the else of THIS elseif is whatever comes next
		restElse := p.parseElseIfOrElse()
		nestedIf := &ifStat{
			baseNode:  baseNode{line: p.current().Line, column: p.current().Column},
			condition: cond,
			thenBlock: thenBlock,
			elseBlock: restElse,
		}
		return &blockImpl{
			line:   p.current().Line,
			column: p.current().Column,
			stats:  []astapi.StatNode{nestedIf},
		}
	}
	if p.peek(lexapi.TOKEN_ELSE) {
		p.next() // consume 'else'
		parentBlock := p.block
		elseBlock, err := p.parseBlock()
		if err != nil {
			p.block = parentBlock
			return nil
		}
		p.block = parentBlock
		return elseBlock
	}
	return nil
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
	if err != nil {
		p.block = parentBlock // Restore before returning on error
		return
	}
	
	if !p.peek(lexapi.TOKEN_END) {
		p.errorAt(p.current(), "'end' expected")
		p.block = parentBlock // Restore before returning on error
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
	if err != nil {
		p.block = parentBlock // Restore before returning on error
		return
	}
	
	if !p.peek(lexapi.TOKEN_END) {
		p.errorAt(p.current(), "'end' expected")
		p.block = parentBlock // Restore before returning on error
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
		if err != nil {
			p.block = parentBlock // Restore before returning on error
			return
		}
		
		if !p.peek(lexapi.TOKEN_END) {
			p.errorAt(p.current(), "'end' expected")
			p.block = parentBlock // Restore before returning on error
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
		p.block = parentBlock // Restore parent block reference AFTER body is complete
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
		if err != nil {
			p.block = parentBlock // Restore before returning on error
			return
		}
		
		if !p.peek(lexapi.TOKEN_END) {
			p.errorAt(p.current(), "'end' expected")
			p.block = parentBlock // Restore before returning on error
			return
		}
		p.next() // consume 'end'
		
		stat := &forInStat{
			baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
			names: names,
			exprs: exprs,
			block: body,
		}
		p.block = parentBlock // Restore parent block reference AFTER body is complete
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
	if err != nil {
		p.block = parentBlock // Restore before returning on error
		return
	}
	
	if !p.peek(lexapi.TOKEN_UNTIL) {
		p.errorAt(p.current(), "'until' expected")
		p.block = parentBlock // Restore before returning on error
		return
	}
	p.next() // consume 'until'
	
	// Parse condition
	cond, err := p.parseExpr()
	if err != nil {
		p.block = parentBlock // Restore before returning on error
		return
	}
	
	stat := &repeatStat{
		baseNode: baseNode{line: p.current().Line, column: p.current().Column},
		block: body,
		condition: cond,
	}
	p.block = parentBlock // Restore parent block reference
	// Use saved parent block reference
	parentBlock.stats = append(parentBlock.stats, stat)
}

// =============================================================================

// parseParamList parses a parameter list: '(' [name {',' name} [',' '...']] | '...' ')'
// Returns parameter names and whether function is vararg.
// Assumes '(' has already been consumed.
func (p *parser) parseParamList() ([]string, bool) {
	var params []string
	isVarArg := false

	// Empty param list
	if p.peek(lexapi.TOKEN_RPAREN) {
		p.next() // consume ')'
		return params, isVarArg
	}

	// Check for lone '...'
	if p.peek(lexapi.TOKEN_DOTS) {
		isVarArg = true
		p.next() // consume '...'
		if p.peek(lexapi.TOKEN_RPAREN) {
			p.next() // consume ')'
		}
		return params, isVarArg
	}

	// Parse first param name
	if p.current().Type == lexapi.TOKEN_NAME {
		params = append(params, p.current().Value)
		p.next()
	}

	// Parse remaining params
	for p.peek(lexapi.TOKEN_COMMA) {
		p.next() // consume ','
		if p.peek(lexapi.TOKEN_DOTS) {
			isVarArg = true
			p.next() // consume '...'
			break
		}
		if p.current().Type == lexapi.TOKEN_NAME {
			params = append(params, p.current().Value)
			p.next()
		}
	}

	// Consume ')'
	if p.peek(lexapi.TOKEN_RPAREN) {
		p.next()
	}

	return params, isVarArg
}

// Function Definition: function <name> ['.' <name>]* [':' <name>] '(' [params] ')' <block> end
// =============================================================================

func (p *parser) parseFunctionDef(isLocal bool) {
	p.next() // consume 'function' (or skip 'local function')
	
	// Save parent block reference
	parentBlock := p.block
	
	// Parse function name (could be "t" or "t.m" or "t.m.n")
	// For method syntax: function t:m() -> name becomes "t.m" (with implicit method)
	var name string
	var nameTok lexapi.Token
	
	// First name
	if p.current().Type != lexapi.TOKEN_NAME {
		p.errorAt(p.current(), "expected function name")
		return
	}
	name = p.current().Value
	nameTok = p.current()
	p.next()
	
	// Check for method/field syntax: function t:m() or function t.a() or function t.a:m()
	// Loop to handle chained method/field access
	isMethod := false
	for p.peek(lexapi.TOKEN_COLON) || p.peek(lexapi.TOKEN_DOT) {
		if p.peek(lexapi.TOKEN_COLON) {
			p.next() // consume ':'
			if p.current().Type != lexapi.TOKEN_NAME {
				p.errorAt(p.current(), "expected method name after ':'")
				return
			}
			// Build "prefix.method" name
			name = name + "." + p.current().Value
			nameTok = p.current()
			isMethod = true // colon syntax → implicit self parameter
			p.next()
		} else {
			p.next() // consume '.'
			if p.current().Type != lexapi.TOKEN_NAME {
				p.errorAt(p.current(), "expected field name after '.'")
				return
			}
			// Build "prefix.field" name
			name = name + "." + p.current().Value
			nameTok = p.current()
			p.next()
		}
	}
	
	// Parse parameters
	if !p.peek(lexapi.TOKEN_LPAREN) {
		p.errorAt(p.current(), "'(' expected")
		return
	}
	p.next() // consume '('
	params, isVarArg := p.parseParamList()
	
	// Parse function body block
	// Save outer block reference since parseBlock will set p.block = nil
	savedBlock := p.block
	body, err := p.parseBlock()
	p.block = savedBlock // Restore outer block reference
	if err != nil {
		return
	}
	
	// Consume 'end' - may have been consumed by parseBlock for return statement
	if !p.peek(lexapi.TOKEN_END) {
		p.errorAt(p.current(), "'end' expected")
		return
	}
	p.next() // consume 'end'
	
	// For method syntax (colon), prepend implicit "self" parameter
	if isMethod {
		params = append([]string{"self"}, params...)
	}
	stat := &globalFuncStat{
		baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
		name:     name,
		isMethod: isMethod,
		func_: &funcDefImpl{
			baseNode:  baseNode{line: nameTok.Line, column: nameTok.Column},
			isLocal:   false,
			params:    params,
			varArg:    isVarArg,
			block:     body,
			lastLine:  nameTok.Line,
		},
	}
	parentBlock.stats = append(parentBlock.stats, stat)
}

// funcDefImpl is a FuncDef implementation
type funcDefImpl struct {
	baseNode
	isLocal  bool
	params   []string
	varArg   bool
	block    astapi.Block
	lastLine int
}

// ExpNode implementation for anonymous functions
func (f *funcDefImpl) IsConstant() bool { return false }
func (f *funcDefImpl) Kind() astapi.ExpKind { return astapi.EXP_FUNC }

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
	params, isVarArg := p.parseParamList()
	
	// Parse function body block
	// Save outer block reference since parseBlock will set p.block = nil
	savedBlock := p.block
	body, err := p.parseBlock()
	p.block = savedBlock // Restore outer block reference
	if err != nil {
		return
	}
	
	// Consume 'end' - may have been consumed by parseBlock for return statement
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
			params:    params,
			varArg:    isVarArg,
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
	
	// Check for Lua 5.4/5.5 attribute syntax: local <const> ... or local <close> ...
	// This handles: local <const> x = 1 or local <close> x = 1
	var inheritedConst bool
	if p.peek(lexapi.TOKEN_LT) {
		p.next() // consume '<'
		if p.peek(lexapi.TOKEN_NAME) && p.current().Value == "const" {
			inheritedConst = true
			p.next() // consume 'const'
		} else if p.peek(lexapi.TOKEN_NAME) && p.current().Value == "close" {
			// Lua 5.4 <close> attribute - just consume
			p.next() // consume 'close'
		} else if p.peek(lexapi.TOKEN_CONST) {
			inheritedConst = true
			p.next() // consume 'const'
		} else {
			p.errorAt(p.current(), "expected 'const' or 'close'")
			return
		}
		if !p.peek(lexapi.TOKEN_GT) {
			p.errorAt(p.current(), "expected '>'")
			return
		}
		p.next() // consume '>'
	}
	
	// Parse comma-separated variable names with optional <const> after each
	// Syntax: local a<const>, b, c<const> = ...
	type nameWithConst struct {
		name    string
		isConst bool
		tok     lexapi.Token
	}
	var namesWithConst []nameWithConst
	
	for {
		if p.current().Type == lexapi.TOKEN_NAME {
			nameTok := p.current()
			name := p.current().Value
			p.next()
			
			// Check for <const> or <close> immediately after the name
			isConst := inheritedConst // inherit attribute from 'local <const>' if present
			if p.peek(lexapi.TOKEN_LT) {
				p.next() // consume '<'
				if p.peek(lexapi.TOKEN_NAME) && p.current().Value == "const" {
					isConst = true
					p.next() // consume 'const'
				} else if p.peek(lexapi.TOKEN_NAME) && p.current().Value == "close" {
					p.next() // consume 'close'
				} else if p.peek(lexapi.TOKEN_CONST) {
					isConst = true
					p.next() // consume 'const'
				} else {
					p.errorAt(p.current(), "expected 'const' or 'close'")
					return
				}
				if !p.peek(lexapi.TOKEN_GT) {
					p.errorAt(p.current(), "expected '>'")
					return
				}
				p.next() // consume '>'
			}
			
			namesWithConst = append(namesWithConst, nameWithConst{
				name:    name,
				isConst: isConst,
				tok:     nameTok,
			})
		}
		
		if !p.peek(lexapi.TOKEN_COMMA) {
			break
		}
		p.next() // consume ','
	}
	
	// Extract just the names for the stat
	var names []string
	var firstTok lexapi.Token
	for i, nc := range namesWithConst {
		names = append(names, nc.name)
		if i == 0 {
			firstTok = nc.tok
		}
	}
	
	// Check for assignment
	if p.peek(lexapi.TOKEN_ASSIGN) {
		p.next() // consume '='
		
		// Parse expression list
		var exprs []astapi.ExpNode
		for {
			expr, err := p.parseExpr()
			if err != nil {
				return
			}
			exprs = append(exprs, expr)
			if !p.peek(lexapi.TOKEN_COMMA) {
				break
			}
			p.next() // consume ','
		}
		
		stat := &localVarStat{
			baseNode: baseNode{line: firstTok.Line, column: firstTok.Column},
			names:    names,
			exprs:    exprs,
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
		} else if p.peek(lexapi.TOKEN_CONST) {
			// "const" is now a keyword
			isConst = true
			p.next() // consume 'const'
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
		// global <const> * declares all following globals as const
		// We just record it as a special declaration (no specific stat needed for "*")
		if p.peek(lexapi.TOKEN_ASSIGN) {
			p.next()
			_, err := p.parseExprList()
			if err != nil {
				return
			}
		}
		return
	}
	
	// Parse comma-separated name list: global a<const>, b, c [= exprs]
	for {
		if p.peek(lexapi.TOKEN_NAME) {
			name := p.current().Value
			nameTok := p.current()
			p.next()

			// Check for per-variable <const> attribute
			varVarIsConst := isConst // inherit global const flag
			if p.peek(lexapi.TOKEN_LT) {
				p.next() // consume '<'
				if p.peek(lexapi.TOKEN_NAME) && p.current().Value == "const" {
					varVarIsConst = true
					p.next() // consume 'const'
				} else if p.peek(lexapi.TOKEN_NAME) && p.current().Value == "close" {
					p.next() // consume 'close'
				} else if p.peek(lexapi.TOKEN_CONST) {
					varVarIsConst = true
					p.next() // consume 'const'
				} else {
					p.errorAt(p.current(), "expected 'const' or 'close'")
					return
				}
				if !p.peek(lexapi.TOKEN_GT) {
					p.errorAt(p.current(), "expected '>'")
					return
				}
				p.next() // consume '>'
			}

			var exprs []astapi.ExpNode
			if p.peek(lexapi.TOKEN_ASSIGN) {
				p.next()
				var err error
				exprs, err = p.parseExprList()
				if err != nil {
					return
				}
			}

			// Always add stat - even without assignment (declares global without value)
			stat := &globalVarStat{
				baseNode: baseNode{line: nameTok.Line, column: nameTok.Column},
				name:     name,
				isConst:  varVarIsConst,
				exprs:    exprs,
			}
			parentBlock.stats = append(parentBlock.stats, stat)
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
		p.next() // consume name

		// Check if this is a function call: name(args) or name"string" or name{...}
		if p.peek(lexapi.TOKEN_LPAREN) {
			// Function call: name(args)
			p.next() // consume '('
			var args []astapi.ExpNode
			var callErr error
			if !p.peek(lexapi.TOKEN_RPAREN) {
				args, callErr = p.parseExprList()
				if callErr != nil {
					return false
				}
			}
			if !p.peek(lexapi.TOKEN_RPAREN) {
				return false
			}
			p.next() // consume ')'
			
			// Check if this is part of an expression (comparison or field/index access follows)
			// e.g., "getmetatable(xyz).__close = nil" or "f() == g()"
			if p.isComparisonOperator() || p.peek(lexapi.TOKEN_DOT) || p.peek(lexapi.TOKEN_LBRACK) || p.peek(lexapi.TOKEN_COLON) {
				// Build function call expression and let binary ops handle it
				var expr astapi.ExpNode = &funcCall{
					baseNode:     baseNode{line: tok.Line, column: tok.Column},
					func_:        &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name},
					args_:        args,
					numResults:   1,
				}
				// Continue to suffix loop for field/index access, then binary ops
				expr = p.handleSuffixLoop(expr)
				
				// Check if this is an assignment: e.g., "getmetatable(b).__index = 1"
				if p.peek(lexapi.TOKEN_ASSIGN) {
					p.next() // consume '='
					var exprs []astapi.ExpNode
					for {
						e, err := p.parseExpr()
						if err != nil {
							return false
						}
						exprs = append(exprs, e)
						if !p.peek(lexapi.TOKEN_COMMA) {
							break
						}
						p.next() // consume ','
					}
					stat := &assignStat{
						baseNode: baseNode{line: tok.Line, column: tok.Column},
						vars:     []astapi.ExpNode{expr},
						exprs:    exprs,
					}
					p.block.stats = append(p.block.stats, stat)
					return true
				}
				
				fullExpr := p.handleBinaryOps(expr, tok.Line, tok.Column)
				stat := &expressionStat{
					baseNode: baseNode{line: tok.Line, column: tok.Column},
					expr:     fullExpr,
				}
				p.block.stats = append(p.block.stats, stat)
				return true
			}
			
			// Standalone function call statement
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
			
			// Check if comparison operator or suffix follows - if so, this is part of an expression
			if p.isComparisonOperator() || p.peek(lexapi.TOKEN_DOT) || p.peek(lexapi.TOKEN_LBRACK) || p.peek(lexapi.TOKEN_COLON) {
				var expr astapi.ExpNode = &funcCall{
					baseNode:     baseNode{line: tok.Line, column: tok.Column},
					func_:        &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name},
					args_:        []astapi.ExpNode{&stringExp{baseNode: baseNode{line: strTok.Line, column: strTok.Column}, value: strVal}},
					numResults:   1,
				}
				expr = p.handleSuffixLoop(expr)
				fullExpr := p.handleBinaryOps(expr, tok.Line, tok.Column)
				stat := &expressionStat{
					baseNode: baseNode{line: tok.Line, column: tok.Column},
					expr:     fullExpr,
				}
				p.block.stats = append(p.block.stats, stat)
				return true
			}
			
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

		// Handle assignment and expression statement with potential suffixes
		// Build left side expression
		var left astapi.ExpNode = &nameExp{baseNode: baseNode{line: tok.Line, column: tok.Column}, name: name}

		// Handle any suffixes (field access, indexing) first
		left = p.handleSuffixLoop(left)

		// Handle comma-separated list of variables for assignment
		var vars []astapi.ExpNode
		vars = append(vars, left)
		for p.peek(lexapi.TOKEN_COMMA) {
			p.next() // consume ','
			// Parse the next variable - must be a name or indexed expression
			var nextVar astapi.ExpNode
			switch p.current().Type {
			case lexapi.TOKEN_NAME:
				nextName := p.current().Value
				nextTok := p.current()
				p.next()
				nextVar = &nameExp{baseNode: baseNode{line: nextTok.Line, column: nextTok.Column}, name: nextName}
				nextVar = p.handleSuffixLoop(nextVar)
			default:
				// Not a valid assignment target
				return false
			}
			vars = append(vars, nextVar)
		}

		// Assignment: x, y = expr, expr
		if p.peek(lexapi.TOKEN_ASSIGN) {
			p.next() // consume '='
			// Parse comma-separated expression list
			var exprs []astapi.ExpNode
			for {
				expr, err := p.parseExpr()
				if err != nil {
					return false
				}
				exprs = append(exprs, expr)
				if !p.peek(lexapi.TOKEN_COMMA) {
					break
				}
				p.next() // consume ','
			}
			stat := &assignStat{
				baseNode: baseNode{line: tok.Line, column: tok.Column},
				vars:     vars,
				exprs:    exprs,
			}
			p.block.stats = append(p.block.stats, stat)
			return true
		}

		// Expression statement: a + b, a == b, etc.
		expr := p.handleBinaryOps(left, tok.Line, tok.Column)

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
			continue // End of method call - continue to check for more suffixes
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
			
			// Check if comparison operator follows - if so, this is NOT a function call
			// Let caller handle the comparison (e.g., "require"string" == ...")
			if p.isComparisonOperator() {
				return expr
			}
			
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
	
	// Check for empty return
	if p.peek(lexapi.TOKEN_SEMICOLON) || p.peek(lexapi.TOKEN_END) || p.peek(lexapi.TOKEN_EOS) {
		// Consume trailing semicolon if present (advances past it)
		if p.peek(lexapi.TOKEN_SEMICOLON) {
			p.next()
		}
		return exprs
	}
	
	// Parse comma-separated expressions
	for {
		expr, err := p.parseExpr()
		if err != nil {
			break
		}
		exprs = append(exprs, expr)
		
		if !p.peek(lexapi.TOKEN_COMMA) {
			break
		}
		p.next() // consume comma
	}
	
	// Consume optional trailing semicolon
	if p.peek(lexapi.TOKEN_SEMICOLON) {
		p.next()
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
	// Use explicit token checks instead of range — TOKEN_NE=230 overlaps with keywords like TOKEN_THEN=220, TOKEN_TRUE=221
	for p.current().Type == lexapi.TOKEN_LT || p.current().Type == lexapi.TOKEN_GT ||
		p.current().Type == lexapi.TOKEN_LE || p.current().Type == lexapi.TOKEN_GE ||
		p.current().Type == lexapi.TOKEN_EQ || p.current().Type == lexapi.TOKEN_NE {
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
	for p.current().Type == lexapi.TOKEN_PLUS || p.current().Type == lexapi.TOKEN_MINUS || p.current().Type == lexapi.TOKEN_CONCAT {
		var op astapi.BinopKind
		switch p.current().Type {
		case lexapi.TOKEN_PLUS:
			op = astapi.BINOP_ADD
		case lexapi.TOKEN_MINUS:
			op = astapi.BINOP_SUB
		case lexapi.TOKEN_CONCAT:
			op = astapi.BINOP_CONCAT
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

// parsePow handles the power operator '^' which has the highest precedence
// and is right-associative: a^b^c = a^(b^c)
// Left operand is parsePrimary (NOT parseUnary) because unary ops have
// lower precedence than ^: -2^2 = -(2^2), not (-2)^2
func (p *parser) parsePow() (astapi.ExpNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if p.current().Type == lexapi.TOKEN_POW {
		tok := p.current()
		p.next()
		// Right-associative, but allow unary on right: 2^-3 = 2^(-3)
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &binopExp{op: astapi.BINOP_POW, left: left, right: right, baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
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
	if p.current().Type == lexapi.TOKEN_TILDE {
		// Unary bitwise NOT: ~x
		tok := p.current()
		p.next()
		exp, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unopExp{op: astapi.UNOP_BNOT, exp: exp, baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
	}
	if p.current().Type == lexapi.TOKEN_HASH {
		// Length operator: #x
		tok := p.current()
		p.next()
		exp, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unopExp{op: astapi.UNOP_LEN, exp: exp, baseNode: baseNode{line: tok.Line, column: tok.Column}}, nil
	}
	return p.parsePow()
}

func (p *parser) parsePrimary() (astapi.ExpNode, error) {
	// Parse the first part (prefix)
	var expr astapi.ExpNode
	var err error

	switch p.current().Type {
	case lexapi.TOKEN_FUNCTION:
		// Anonymous function expression: function(args) body end
		expr = p.parseAnonFunction()

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
			// Parenthesized expression: the inner expr might be a literal,
			// but we need to allow suffixes on the whole thing like ("hello"):sub(1)
			// Skip literal early-returns, go directly to suffix handling
			expr = exp
			goto handleSuffixes
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
	// BUT: parenthesized expressions like ("hello") CAN have suffixes
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
	if _, ok := expr.(*tableConstructor); ok {
		// Check if it's the "()" empty case
		if tc, ok := expr.(*tableConstructor); ok && len(tc.arrayFields) == 0 && tc.recordFields == nil {
			// This is "()" - return immediately
			return expr, nil
		}
	}

handleSuffixes:
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
			// Method call: expr:method(args) or expr:method"string" or expr:method{...}
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
			// Parse arguments (explicit parens, implicit string, or implicit table)
			var args []astapi.ExpNode
			if p.peek(lexapi.TOKEN_LPAREN) {
				p.next() // consume '('
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
			} else if p.peek(lexapi.TOKEN_STRING) {
				// Implicit string arg: expr:method"string"
				args = append(args, &stringExp{baseNode: baseNode{line: p.current().Line, column: p.current().Column}, value: p.current().Value})
				p.next()
			} else if p.peek(lexapi.TOKEN_LBRACE) {
				// Implicit table arg: expr:method{...}
				table, err := p.parseTableConstructor()
				if err != nil {
					return nil, err
				}
				if table != nil {
					args = append(args, table)
				}
			} else {
				// No args after method name
				args = []astapi.ExpNode{}
			}
			// Prepend self (the table/object) as first argument for method call
			args = append([]astapi.ExpNode{expr}, args...)
			expr = &funcCall{
				baseNode:   baseNode{line: methodTok.Line, column: methodTok.Column},
				func_:      methodExpr,
				args_:      args,
				numResults:  1,
			}
			continue // End of method call - continue to check for more suffixes
		} else if p.peek(lexapi.TOKEN_STRING) {
			// Implicit function call with string arg: expr "string"
			strVal := p.current().Value
			strTok := p.current()
			p.next()
			expr = &funcCall{
				baseNode:   baseNode{line: strTok.Line, column: strTok.Column},
				func_:      expr,
				args_:      []astapi.ExpNode{&stringExp{baseNode: baseNode{line: strTok.Line, column: strTok.Column}, value: strVal}},
				numResults: 1,
			}
		} else if p.peek(lexapi.TOKEN_LBRACE) {
			// Implicit function call with table arg: expr {1, 2, 3}
			table, err := p.parseTableConstructor()
			if err != nil {
				return nil, err
			}
			expr = &funcCall{
				baseNode:   baseNode{line: p.current().Line, column: p.current().Column},
				func_:      expr,
				args_:      []astapi.ExpNode{table},
				numResults: 1,
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

// parseAnonFunction parses an anonymous function expression.
// function (params) body end
func (p *parser) parseAnonFunction() astapi.ExpNode {
	tok := p.current() // 'function' token
	p.next() // consume 'function'
	
	// Parse parameters
	var params []string
	isVarArg := false
	if p.peek(lexapi.TOKEN_LPAREN) {
		p.next() // consume '('
		params, isVarArg = p.parseParamList()
	}
	
	// Parse function body
	body, _ := p.parseBlock()
	
	// Consume 'end'
	if p.peek(lexapi.TOKEN_END) {
		p.next()
	}
	
	// Return a funcDefImpl as expression
	return &funcDefImpl{
		baseNode:  baseNode{line: tok.Line, column: tok.Column},
		isLocal:   true, // anonymous functions are local
		params:    params,
		varArg:    isVarArg,
		block:     body,
		lastLine:  tok.Line,
	}
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
	isMethodCall := false
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
		isMethodCall = true
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

	// For method calls (colon syntax), prepend self (the object) as first argument
	if isMethodCall {
		args = append([]astapi.ExpNode{prefix}, args...)
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

// isComparisonOperator returns true if current token is a comparison operator.
// Used to detect cases like "f() == g()" where the call is part of a comparison.
func (p *parser) isComparisonOperator() bool {
	switch p.current().Type {
	case lexapi.TOKEN_EQ, lexapi.TOKEN_NE, lexapi.TOKEN_LT, lexapi.TOKEN_GT, lexapi.TOKEN_LE, lexapi.TOKEN_GE:
		return true
	}
	return false
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
