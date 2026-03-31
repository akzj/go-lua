// Package parser implements the Lua syntax analyzer.
package parser

import (
	"github.com/akzj/go-lua/pkg/lexer"
)

// ============================================================================
// Expression Parsing
// ============================================================================

// precedenceLevel represents operator precedence levels.
// Higher numbers = higher precedence (bind tighter).
type precedenceLevel int

const (
	precNone precedenceLevel = iota
	precOr                  // or
	precAnd                 // and
	precComparison          // < > <= >= ~= ==
	precBitOr               // |
	precBitXor              // ~
	precBitAnd              // &
	precShift               // << >>
	precConcat              // ..
	precAdd                 // + -
	precMul                 // * / // %
	precUnary               // not - ~ #
	precPower               // ^
	precPrimary             // Primary expressions (literals, variables, etc.)
)

// parseExpr parses an expression with minimum precedence.
//
// This uses the precedence climbing algorithm for operator parsing.
func (p *Parser) parseExpr() Expr {
	return p.parseExprPrecedence(precNone)
}

// parseExprPrecedence parses an expression using precedence climbing.
func (p *Parser) parseExprPrecedence(minPrecedence precedenceLevel) Expr {
	// Parse left side (prefix/unary)
	left := p.parsePrefixExpr()
	if left == nil {
		return nil
	}

	// Continue with binary operators
	return p.parseBinaryExprFromLeft(left, minPrecedence)
}

// parseBinaryExprFromLeft continues parsing binary expressions from an existing left expression.
// This is used when we've already parsed a prefix expression (possibly with suffixes) and
// need to continue with binary operator parsing.
func (p *Parser) parseBinaryExprFromLeft(left Expr, minPrecedence precedenceLevel) Expr {
	// Parse infix operators
	for {
		// Get precedence of current operator
		precedence := p.getOperatorPrecedence()
		// Stop if not an operator or precedence is too low
		if precedence <= precNone || precedence < minPrecedence {
			break
		}

		// Get operator token
		op := p.Current
		p.advance()

		// Parse right side
		var right Expr
		if op.Type == lexer.TK_CONCAT || op.Type == lexer.TK_CARET {
			// Right-associative operators
			right = p.parseExprPrecedence(precedence)
		} else {
			// Left-associative operators
			right = p.parseExprPrecedence(precedence + 1)
		}

		if right == nil {
			return left
		}

		// Create binary expression
		left = p.createBinOpExpr(left, op, right)
	}

	return left
}

// parsePrefixExpr parses a prefix expression (literal, variable, unary, etc.).
func (p *Parser) parsePrefixExpr() Expr {
	switch p.Current.Type {
	case lexer.TK_NIL:
		p.advance()
		return &NilExpr{
			baseExpr: baseExpr{line: p.Current.Line},
		}

	case lexer.TK_TRUE:
		p.advance()
		return &BooleanExpr{
			baseExpr: baseExpr{line: p.Current.Line},
			Value:    true,
		}

	case lexer.TK_FALSE:
		p.advance()
		return &BooleanExpr{
			baseExpr: baseExpr{line: p.Current.Line},
			Value:    false,
		}

	case lexer.TK_INT:
		line := p.Current.Line
		val := p.Current.Value.(int64)
		p.advance()
		return &NumberExpr{
			baseExpr: baseExpr{line: line},
			Int:      val,
			Value:    float64(val),
			IsInt:    true,
		}

	case lexer.TK_FLOAT:
		line := p.Current.Line
		val := p.Current.Value.(float64)
		p.advance()
		return &NumberExpr{
			baseExpr: baseExpr{line: line},
			Value:    val,
			IsInt:    false,
		}

	case lexer.TK_STRING:
		line := p.Current.Line
		val := p.Current.Value.(string)
		p.advance()
		return &StringExpr{
			baseExpr: baseExpr{line: line},
			Value:    val,
		}

	case lexer.TK_DOTS:
		p.advance()
		return &DotsExpr{
			baseExpr: baseExpr{line: p.Current.Line},
		}

	case lexer.TK_FUNCTION:
		return p.parseAnonFunc()

	case lexer.TK_LBRACE:
		line := p.Current.Line
		expr := p.parseTableConstructor()
		return p.parseSuffixes(expr, line)

	case lexer.TK_NAME:
		return p.parseVarExpr()

	case lexer.TK_LPAREN:
		return p.parseParenExpr()

	case lexer.TK_MINUS, lexer.TK_NOT, lexer.TK_HASH, lexer.TK_CARET, lexer.TK_BXOR:
		return p.parseUnaryExpr()

	default:
		p.Error("unexpected token: %v", p.Current.Type)
		return nil
	}
}

// parseSuffixes parses suffixes (field access, index, call) on an expression.
// This is used after parsing primary expressions like variables, table constructors,
// and parenthesized expressions.
func (p *Parser) parseSuffixes(expr Expr, line int) Expr {
	for {
		switch p.Current.Type {
		case lexer.TK_DOT:
			p.advance()
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

		case lexer.TK_LBRACK:
			p.advance()
			index := p.parseExpr()
			if !p.match(lexer.TK_RBRACK) {
				p.Error("expected ']' after index expression")
				return expr
			}
			expr = &IndexExpr{
				baseExpr: baseExpr{line: line},
				Table:    expr,
				Index:    index,
			}

		case lexer.TK_COLON:
			// Method call
			p.advance()
			if p.Current.Type != lexer.TK_NAME {
				p.Error("expected method name after ':'")
				return expr
			}
			method := p.Current.Value.(string)
			p.advance()
			args := p.parseArgs()
			expr = &MethodCallExpr{
				baseExpr: baseExpr{line: line},
				Object:   expr,
				Method:   method,
				Args:     args,
			}

		case lexer.TK_LPAREN, lexer.TK_LBRACE, lexer.TK_STRING:
			// Function call
			args := p.parseArgs()
			expr = &CallExpr{
				baseExpr: baseExpr{line: line},
				Func:     expr,
				Args:     args,
			}

		default:
			return expr
		}
	}
}

// parseVarExpr parses a variable expression (name, field access, index).
func (p *Parser) parseVarExpr() Expr {
	line := p.Current.Line
	name := p.Current.Value.(string)
	p.advance()

	expr := &VarExpr{
		baseExpr: baseExpr{line: line},
		Name:     name,
	}

	// Parse suffixes (field access, index, call)
	return p.parseSuffixes(expr, line)
}

// parseParenExpr parses a parenthesized expression.
func (p *Parser) parseParenExpr() Expr {
	line := p.Current.Line
	p.advance() // Skip '('

	expr := p.parseExpr()

	if !p.match(lexer.TK_RPAREN) {
		p.Error("expected ')' after expression")
		return expr
	}

	// Parenthesized expressions can have suffixes like indexing and field access
	// e.g., (expr)[1], (expr).field
	return p.parseSuffixes(&ParenExpr{
		baseExpr: baseExpr{line: line},
		Expr:     expr,
	}, line)
}

// parseUnaryExpr parses a unary expression.
func (p *Parser) parseUnaryExpr() Expr {
	line := p.Current.Line
	op := p.Current.Type
	p.advance()

	// Parse operand at precPower level so that ^ binds tighter than unary operators.
	// For example, -2^2 should parse as -(2^2) = -4, not (-2)^2 = 4.
	operand := p.parseExprPrecedence(precPower)
	if operand == nil {
		return nil
	}

	var opStr string
	switch op {
	case lexer.TK_MINUS:
		opStr = "-"
	case lexer.TK_NOT:
		opStr = "not"
	case lexer.TK_HASH:
		opStr = "#"
	case lexer.TK_CARET:
		// This shouldn't happen in unary context, but handle it
		opStr = "^"
	case lexer.TK_BXOR:
		// Unary ~ for bitwise NOT
		opStr = "~"
	}

	return &UnOpExpr{
		baseExpr: baseExpr{line: line},
		Op:       opStr,
		Expr:     operand,
	}
}

// parseAnonFunc parses an anonymous function expression.
func (p *Parser) parseAnonFunc() Expr {
	line := p.Current.Line
	p.advance() // Skip 'function'

	// Parse parameter list
	params, isVarArg, varargName := p.parseParamList()

	// Parse body
	body := p.parseBlock()

	// Expect 'end'
	if !p.match(lexer.TK_END) {
		p.Error("expected 'end' after function body")
	}

	return &FuncExpr{
		baseExpr: baseExpr{line: line},
		Params:   params,
		Body:     body,
		IsVarArg: isVarArg,
		VarargName: varargName,
	}
}

// parseParamList parses a function parameter list.
func (p *Parser) parseParamList() ([]*VarExpr, bool, string) {
	params := []*VarExpr{}
	isVarArg := false
	varargName := ""

	if !p.match(lexer.TK_LPAREN) {
		p.Error("expected '(' after 'function'")
		return params, isVarArg, varargName
	}

	// Check for empty parameter list
	if p.match(lexer.TK_RPAREN) {
		return params, isVarArg, varargName
	}

	// Parse parameters
	for {
		if p.Current.Type == lexer.TK_DOTS {
			isVarArg = true
			p.advance()
			// Lua 5.4: support named vararg (...t) where t is the vararg table name
			if p.Current.Type == lexer.TK_NAME {
				varargName = p.Current.Value.(string)
				p.advance()
			}
			break
		}

		if p.Current.Type != lexer.TK_NAME {
			p.Error("expected parameter name")
			break
		}

		name := p.Current.Value.(string)
		line := p.Current.Line
		p.advance()

		params = append(params, &VarExpr{
			baseExpr: baseExpr{line: line},
			Name:     name,
		})

		if !p.match(lexer.TK_COMMA) {
			break
		}
	}

	// Expect closing paren
	if !p.match(lexer.TK_RPAREN) {
		p.Error("expected ')' after parameter list")
	}

	return params, isVarArg, varargName
}

// parseTableConstructor parses a table constructor {...}.
func (p *Parser) parseTableConstructor() Expr {
	line := p.Current.Line
	p.advance() // Skip '{'

	entries := []TableEntry{}

	// Check for empty table
	if p.match(lexer.TK_RBRACE) {
		return &TableExpr{
			baseExpr: baseExpr{line: line},
			Entries:  entries,
		}
	}

	// Parse entries
	for {
		entry := p.parseTableEntry()
		if entry != nil {
			entries = append(entries, *entry)
		}

		// Check for comma or semicolon separator
		if !p.match(lexer.TK_COMMA, lexer.TK_SEMICOLON) {
			break
		}

		// Check for end of table
		if p.Current.Type == lexer.TK_RBRACE {
			break
		}
	}

	// Expect closing brace
	if !p.match(lexer.TK_RBRACE) {
		p.Error("expected '}' after table constructor")
	}

	return &TableExpr{
		baseExpr: baseExpr{line: line},
		Entries:  entries,
	}
}

// parseTableEntry parses a single table entry.
func (p *Parser) parseTableEntry() *TableEntry {
	entry := &TableEntry{}

	// Check for [key] = value form
	if p.Current.Type == lexer.TK_LBRACK {
		p.advance()
		key := p.parseExpr()
		if !p.match(lexer.TK_RBRACK) {
			p.Error("expected ']' after table index")
			return nil
		}
		if !p.match(lexer.TK_ASSIGN) {
			p.Error("expected '=' after table index")
			return nil
		}
		value := p.parseExpr()
		entry.Kind = TableEntryIndex
		entry.Key = key
		entry.Value = value
		return entry
	}

	// Check for name = value form
	if p.Current.Type == lexer.TK_NAME {
		name := p.Current.Value.(string)
		line := p.Current.Line
		p.advance()

		if p.Current.Type == lexer.TK_ASSIGN {
			// It's a field assignment
			p.advance() // consume '='
			value := p.parseExpr()
			entry.Kind = TableEntryField
			entry.Key = &VarExpr{
				baseExpr: baseExpr{line: line},
				Name:     name,
			}
			entry.Value = value
			return entry
		} else {
			// It's a value-only entry - could be a simple name or a function call
			var expr Expr = &VarExpr{
				baseExpr: baseExpr{line: line},
				Name:     name,
			}

			// Check for function call suffixes
		suffixLoop:
			for {
				switch p.Current.Type {
				case lexer.TK_COLON:
					// Method call
					p.advance()
					if p.Current.Type != lexer.TK_NAME {
						p.Error("expected method name after ':'")
						entry.Kind = TableEntryValue
						entry.Value = expr
						return entry
					}
					method := p.Current.Value.(string)
					p.advance()
					args := p.parseArgs()
					expr = &MethodCallExpr{
						baseExpr: baseExpr{line: line},
						Object:   expr,
						Method:   method,
						Args:     args,
					}

				case lexer.TK_LPAREN, lexer.TK_LBRACE, lexer.TK_STRING:
					// Function call
					args := p.parseArgs()
					expr = &CallExpr{
						baseExpr: baseExpr{line: line},
						Func:     expr,
						Args:     args,
					}

				case lexer.TK_DOT:
					// Field access - continue parsing
					p.advance()
					if p.Current.Type != lexer.TK_NAME {
						p.Error("expected field name after '.'")
						entry.Kind = TableEntryValue
						entry.Value = expr
						return entry
					}
					field := p.Current.Value.(string)
					p.advance()
					expr = &FieldExpr{
						baseExpr: baseExpr{line: line},
						Table:    expr,
						Field:    field,
					}

				case lexer.TK_LBRACK:
					// Index access - continue parsing
					p.advance()
					index := p.parseExpr()
					if !p.match(lexer.TK_RBRACK) {
						p.Error("expected ']' after index expression")
						entry.Kind = TableEntryValue
						entry.Value = expr
						return entry
					}
					expr = &IndexExpr{
						baseExpr: baseExpr{line: line},
						Table:    expr,
						Index:    index,
					}

				default:
					// No more suffixes - break out to check for binary operators
					break suffixLoop
				}
			}
			// After parsing suffixes, continue with binary expression parsing
			entry.Kind = TableEntryValue
			entry.Value = p.parseBinaryExprFromLeft(expr, precNone)
			return entry
		}
	}

	// Value-only entry
	value := p.parseExpr()
	entry.Kind = TableEntryValue
	entry.Value = value
	return entry
}

// parseArgs parses function call arguments.
func (p *Parser) parseArgs() []Expr {
	args := []Expr{}

	switch p.Current.Type {
	case lexer.TK_LPAREN:
		p.advance()
		// Check for empty argument list
		if p.match(lexer.TK_RPAREN) {
			return args
		}
		// Parse arguments
		for {
			arg := p.parseExpr()
			if arg != nil {
				args = append(args, arg)
			}
			if !p.match(lexer.TK_COMMA) {
				break
			}
		}
		if !p.match(lexer.TK_RPAREN) {
			p.Error("expected ')' after arguments")
		}

	case lexer.TK_LBRACE:
		// Table constructor as single argument
		table := p.parseTableConstructor()
		args = append(args, table)

	case lexer.TK_STRING:
		// String literal as single argument
		line := p.Current.Line
		val := p.Current.Value.(string)
		p.advance()
		args = append(args, &StringExpr{
			baseExpr: baseExpr{line: line},
			Value:    val,
		})
	}

	return args
}

// createBinOpExpr creates a binary operation expression.
func (p *Parser) createBinOpExpr(left Expr, op lexer.Token, right Expr) Expr {
	var opStr string
	switch op.Type {
	case lexer.TK_PLUS:
		opStr = "+"
	case lexer.TK_MINUS:
		opStr = "-"
	case lexer.TK_STAR:
		opStr = "*"
	case lexer.TK_SLASH:
		opStr = "/"
	case lexer.TK_IDIV:
		opStr = "//"
	case lexer.TK_PERCENT:
		opStr = "%"
	case lexer.TK_CARET:
		opStr = "^"
	case lexer.TK_CONCAT:
		opStr = ".."
	case lexer.TK_LT:
		opStr = "<"
	case lexer.TK_GT:
		opStr = ">"
	case lexer.TK_LE:
		opStr = "<="
	case lexer.TK_GE:
		opStr = ">="
	case lexer.TK_EQ:
		opStr = "=="
	case lexer.TK_NE:
		opStr = "~="
	case lexer.TK_AND:
		opStr = "and"
	case lexer.TK_OR:
		opStr = "or"
	case lexer.TK_SHL:
		opStr = "<<"
	case lexer.TK_SHR:
		opStr = ">>"
	case lexer.TK_BAND:
		opStr = "&"
	case lexer.TK_BOR:
		opStr = "|"
	case lexer.TK_BXOR:
		opStr = "~"
	}

	return &BinOpExpr{
		baseExpr: baseExpr{line: op.Line},
		Left:     left,
		Op:       opStr,
		Right:    right,
	}
}

// getOperatorPrecedence returns the precedence level of the current operator.
func (p *Parser) getOperatorPrecedence() precedenceLevel {
	switch p.Current.Type {
	case lexer.TK_OR:
		return precOr
	case lexer.TK_AND:
		return precAnd
	case lexer.TK_LT, lexer.TK_GT, lexer.TK_LE, lexer.TK_GE, lexer.TK_EQ, lexer.TK_NE:
		return precComparison
	case lexer.TK_BOR:
		return precBitOr
	case lexer.TK_BXOR:
		return precBitXor
	case lexer.TK_BAND:
		return precBitAnd
	case lexer.TK_SHL, lexer.TK_SHR:
		return precShift
	case lexer.TK_CONCAT:
		return precConcat
	case lexer.TK_PLUS, lexer.TK_MINUS:
		return precAdd
	case lexer.TK_STAR, lexer.TK_SLASH, lexer.TK_IDIV, lexer.TK_PERCENT:
		return precMul
	case lexer.TK_CARET:
		return precPower
	default:
		return precNone
	}
}