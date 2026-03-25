// Package parser implements the Lua syntax analyzer.
package parser

import (
	"github.com/akzj/go-lua/pkg/lexer"
)

// ============================================================================
// Statement Parsing
// ============================================================================

// parseIfStmt parses an if-then-elseif-else-end statement.
func (p *Parser) parseIfStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip 'if'

	// Parse condition
	cond := p.parseExpr()
	if cond == nil {
		p.sync()
		return nil
	}

	// Expect 'then'
	if !p.expect(lexer.TK_THEN, "'then'") {
		p.sync()
		return nil
	}

	// Parse then block
	thenBlock := p.parseBlock()

	// Parse elseif blocks
	elseIfBlocks := []ElseIfClause{}

	for p.Current.Type == lexer.TK_ELSEIF {
		p.advance() // Skip 'elseif'

		elseifCond := p.parseExpr()
		if elseifCond == nil {
			p.sync()
			return nil
		}

		if !p.expect(lexer.TK_THEN, "'then'") {
			p.sync()
			return nil
		}

		elseifBlock := p.parseBlock()

		elseIfBlocks = append(elseIfBlocks, ElseIfClause{
			Cond: elseifCond,
			Then: elseifBlock,
		})
	}

	// Parse else block
	var elseBlock *BlockStmt
	if p.match(lexer.TK_ELSE) {
		elseBlock = p.parseBlock()
	}

	// Expect 'end'
	if !p.expect(lexer.TK_END, "'end'") {
		p.sync()
		return nil
	}

	return &IfStmt{
		baseStmt: baseStmt{line: line},
		Cond:     cond,
		Then:     thenBlock,
		ElseIf:   elseIfBlocks,
		Else:     elseBlock,
	}
}

// parseWhileStmt parses a while-do-end loop.
func (p *Parser) parseWhileStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip 'while'

	// Parse condition
	cond := p.parseExpr()
	if cond == nil {
		p.sync()
		return nil
	}

	// Expect 'do'
	if !p.expect(lexer.TK_DO, "'do'") {
		p.sync()
		return nil
	}

	// Parse body
	body := p.parseBlock()

	// Expect 'end'
	if !p.expect(lexer.TK_END, "'end'") {
		p.sync()
		return nil
	}

	return &WhileStmt{
		baseStmt: baseStmt{line: line},
		Cond:     cond,
		Body:     body,
	}
}

// parseRepeatStmt parses a repeat-until loop.
func (p *Parser) parseRepeatStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip 'repeat'

	// Parse body
	body := p.parseBlock()

	// Expect 'until'
	if !p.expect(lexer.TK_UNTIL, "'until'") {
		p.sync()
		return nil
	}

	// Parse condition
	cond := p.parseExpr()
	if cond == nil {
		p.sync()
		return nil
	}

	return &RepeatStmt{
		baseStmt: baseStmt{line: line},
		Body:     body,
		Cond:     cond,
	}
}

// parseForStmt parses a for loop (numeric or generic).
func (p *Parser) parseForStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip 'for'

	// Parse first variable name
	if p.Current.Type != lexer.TK_NAME {
		p.Error("expected variable name after 'for'")
		p.sync()
		return nil
	}

	name := p.Current.Value.(string)
	nameLine := p.Current.Line
	p.advance()

	// Check if it's a numeric for (has '=') or generic for (has ',')
	if p.match(lexer.TK_ASSIGN) {
		// Numeric for loop
		start := p.parseExpr()
		if start == nil {
			p.sync()
			return nil
		}

		if !p.match(lexer.TK_COMMA) {
			p.Error("expected ',' after start value")
			p.sync()
			return nil
		}

		end := p.parseExpr()
		if end == nil {
			p.sync()
			return nil
		}

		// Optional step
		var step Expr
		if p.match(lexer.TK_COMMA) {
			step = p.parseExpr()
			if step == nil {
				p.sync()
				return nil
			}
		}

		// Expect 'do'
		if !p.expect(lexer.TK_DO, "'do'") {
			p.sync()
			return nil
		}

		// Parse body
		body := p.parseBlock()

		// Expect 'end'
		if !p.expect(lexer.TK_END, "'end'") {
			p.sync()
			return nil
		}

		return &ForNumericStmt{
			baseStmt: baseStmt{line: line},
			Var: &VarExpr{
				baseExpr: baseExpr{line: nameLine},
				Name:     name,
			},
			From: start,
			To:   end,
			Step: step,
			Body: body,
		}
	} else {
		// Generic for loop
		names := []*VarExpr{
			{
				baseExpr: baseExpr{line: nameLine},
				Name:     name,
			},
		}

		// Parse additional variable names
		for p.match(lexer.TK_COMMA) {
			if p.Current.Type != lexer.TK_NAME {
				p.Error("expected variable name")
				break
			}
			name := p.Current.Value.(string)
			line := p.Current.Line
			p.advance()
			names = append(names, &VarExpr{
				baseExpr: baseExpr{line: line},
				Name:     name,
			})
		}

		// Expect 'in'
		if !p.expect(lexer.TK_IN, "'in'") {
			p.sync()
			return nil
		}

		// Parse function expression
		funcExpr := p.parseExpr()
		if funcExpr == nil {
			p.sync()
			return nil
		}

		// Parse argument expressions
		args := []Expr{}
		for p.match(lexer.TK_COMMA) {
			arg := p.parseExpr()
			if arg != nil {
				args = append(args, arg)
			}
		}

		// Expect 'do'
		if !p.expect(lexer.TK_DO, "'do'") {
			p.sync()
			return nil
		}

		// Parse body
		body := p.parseBlock()

		// Expect 'end'
		if !p.expect(lexer.TK_END, "'end'") {
			p.sync()
			return nil
		}

		return &ForGenericStmt{
			baseStmt: baseStmt{line: line},
			Vars:    names,
			Exprs:   append([]Expr{funcExpr}, args...),
			Body:    body,
		}
	}
}

// parseReturnStmt parses a return statement.
func (p *Parser) parseReturnStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip 'return'

	values := []Expr{}

	// Check for return with values
	if !p.check(lexer.TK_SEMICOLON, lexer.TK_END, lexer.TK_ELSE, lexer.TK_ELSEIF, lexer.TK_UNTIL) && !p.isAtEnd() {
		// Parse return values
		for {
			value := p.parseExpr()
			if value != nil {
				values = append(values, value)
			}
			if !p.match(lexer.TK_COMMA) {
				break
			}
		}
	}

	return &ReturnStmt{
		baseStmt: baseStmt{line: line},
		Values:   values,
	}
}

// parseBreakStmt parses a break statement.
func (p *Parser) parseBreakStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip 'break'

	return &BreakStmt{
		baseStmt: baseStmt{line: line},
	}
}

// parseLocalStmt parses a local variable declaration or local function.
func (p *Parser) parseLocalStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip 'local'

	// Check for 'local function name() ... end'
	if p.Current.Type == lexer.TK_FUNCTION {
		return p.parseLocalFunction(line)
	}

	names := []*VarExpr{}
	attrs := []string{}

	// Parse variable names
	for {
		if p.Current.Type != lexer.TK_NAME {
			p.Error("expected variable name")
			break
		}

		name := p.Current.Value.(string)
		nameLine := p.Current.Line
		p.advance()

		names = append(names, &VarExpr{
			baseExpr: baseExpr{line: nameLine},
			Name:     name,
		})

		// Check for attribute (simplified - not fully implemented)
		// Lua 5.4 supports <const> attribute

		if !p.match(lexer.TK_COMMA) {
			break
		}
	}

	// Check for initialization
	values := []Expr{}
	if p.match(lexer.TK_ASSIGN) {
		for {
			value := p.parseExpr()
			if value != nil {
				values = append(values, value)
			}
			if !p.match(lexer.TK_COMMA) {
				break
			}
		}
	}

	return &LocalStmt{
		baseStmt: baseStmt{line: line},
		Names:    names,
		Attrs:    attrs,
		Values:   values,
	}
}

// parseLocalFunction parses 'local function name() ... end'.
// This is syntactic sugar for: local name; name = function() ... end
func (p *Parser) parseLocalFunction(line int) Stmt {
	p.advance() // Skip 'function'

	// Parse function name
	if p.Current.Type != lexer.TK_NAME {
		p.Error("expected function name after 'local function'")
		p.sync()
		return nil
	}

	name := p.Current.Value.(string)
	nameLine := p.Current.Line
	p.advance()

	// Parse parameter list
	params, isVarArg := p.parseParamList()

	// Parse body
	body := p.parseBlock()

	// Expect 'end'
	if !p.expect(lexer.TK_END, "'end'") {
		p.sync()
		return nil
	}

	return &FuncDefStmt{
		baseStmt: baseStmt{line: line},
		Name: []*VarExpr{
			{
				baseExpr: baseExpr{line: nameLine},
				Name:     name,
			},
		},
		Params:   params,
		Body:     body,
		IsVarArg: isVarArg,
		IsLocal:  true,
	}
}

// parseFuncDefStmt parses a function definition statement.
func (p *Parser) parseFuncDefStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip 'function'

	isLocal := false
	var nameExpr Expr

	// Check for local function
	if p.Current.Type == lexer.TK_NAME && p.Current.Value.(string) == "function" {
		// This shouldn't happen, 'local function' is parsed in parseLocalStmt
	}

	// Parse function name (can be dotted or with colon)
	nameExpr = p.parseFuncName()
	if nameExpr == nil {
		p.sync()
		return nil
	}

	// Parse parameter list
	params, isVarArg := p.parseParamList()

	// Parse body
	body := p.parseBlock()

	// Expect 'end'
	if !p.expect(lexer.TK_END, "'end'") {
		p.sync()
		return nil
	}

	// Convert nameExpr to []*VarExpr for simple function definitions
	// For complex names like table.field, we use the base variable
	var names []*VarExpr
	switch n := nameExpr.(type) {
	case *VarExpr:
		names = []*VarExpr{n}
	case *FieldExpr:
		// For table.field, use the table part as the base var
		if table, ok := n.Table.(*VarExpr); ok {
			names = []*VarExpr{table}
		} else {
			names = []*VarExpr{}
		}
	default:
		names = []*VarExpr{}
	}

	return &FuncDefStmt{
		baseStmt: baseStmt{line: line},
		Name:     names,
		Params:   params,
		Body:     body,
		IsVarArg: isVarArg,
		IsLocal:  isLocal,
	}
}

// parseFuncName parses a function name (can include dots and colons).
func (p *Parser) parseFuncName() Expr {
	if p.Current.Type != lexer.TK_NAME {
		p.Error("expected function name")
		return nil
	}

	line := p.Current.Line
	name := p.Current.Value.(string)
	p.advance()

	var expr Expr = &VarExpr{
		baseExpr: baseExpr{line: line},
		Name:     name,
	}

	// Parse dots (table.field)
	for p.match(lexer.TK_DOT) {
		if p.Current.Type != lexer.TK_NAME {
			p.Error("expected field name after '.'")
			return expr
		}
		field := p.Current.Value.(string)
		p.advance()
		expr = &FieldExpr{
			baseExpr: baseExpr{line: line},
			Table:    expr,
			Field:    field,
		}
	}

	// Check for colon (method)
	if p.match(lexer.TK_COLON) {
		if p.Current.Type != lexer.TK_NAME {
			p.Error("expected method name after ':'")
			return expr
		}
		method := p.Current.Value.(string)
		p.advance()
		expr = &FieldExpr{
			baseExpr: baseExpr{line: line},
			Table:    expr,
			Field:    method,
		}
	}

	return expr
}

// parseGotoStmt parses a goto statement.
func (p *Parser) parseGotoStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip 'goto'

	if p.Current.Type != lexer.TK_NAME {
		p.Error("expected label name after 'goto'")
		p.sync()
		return nil
	}

	label := p.Current.Value.(string)
	p.advance()

	return &GotoStmt{
		baseStmt: baseStmt{line: line},
		Label:    label,
	}
}

// parseLabelStmt parses a label ::name::.
func (p *Parser) parseLabelStmt() Stmt {
	line := p.Current.Line
	p.advance() // Skip first ':'

	if p.Current.Type != lexer.TK_NAME {
		p.Error("expected label name")
		p.sync()
		return nil
	}

	name := p.Current.Value.(string)
	p.advance()

	if !p.expect(lexer.TK_DBCOLON, "'::'") {
		p.sync()
		return nil
	}

	return &LabelStmt{
		baseStmt: baseStmt{line: line},
		Name:     name,
	}
}

// parseAssignOrExprStmt parses an assignment or expression statement.
func (p *Parser) parseAssignOrExprStmt() Stmt {
	line := p.Current.Line

	// Parse left-hand side (variables)
	left := []Expr{}

	// First variable
	expr := p.parsePrefixExpr()
	if expr == nil {
		p.sync()
		return nil
	}
	left = append(left, expr)

	// Additional variables
	for p.match(lexer.TK_COMMA) {
		nextExpr := p.parsePrefixExpr()
		if nextExpr == nil {
			break
		}
		left = append(left, nextExpr)
	}

	// Check if it's an assignment
	if p.match(lexer.TK_ASSIGN) {
		// Parse right-hand side
		right := []Expr{}
		for {
			value := p.parseExpr()
			if value != nil {
				right = append(right, value)
			}
			if !p.match(lexer.TK_COMMA) {
				break
			}
		}

		return &AssignStmt{
			baseStmt: baseStmt{line: line},
			Left:     left,
			Right:    right,
		}
	}

	// It's an expression statement (typically a function call)
	return &ExprStmt{
		baseStmt: baseStmt{line: line},
		Expr:     left[0],
	}
}