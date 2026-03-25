// Package lexer implements the Lua lexical analyzer.
//
// This file provides the complete set of token types used by the Lua
// lexical analyzer, including literals, reserved words, and operators.
// Token definitions follow the semantics from lua-master/llex.h.

package lexer

// TokenType represents a lexical token type.
type TokenType int

// Literal tokens represent basic lexical elements.
const (
	TK_EOF TokenType = iota // End of file
	TK_NAME                // Identifier (variable name, function name, etc.)
	TK_STRING              // String literal
	TK_INT                 // Integer literal
	TK_FLOAT               // Floating-point literal
)

// Reserved word tokens represent Lua keywords.
// These 24 tokens correspond to Lua's reserved words.
const (
	TK_AND TokenType = iota + 256 // 'and'
	TK_BREAK                       // 'break'
	TK_DO                          // 'do'
	TK_ELSE                        // 'else'
	TK_ELSEIF                      // 'elseif'
	TK_END                         // 'end'
	TK_FALSE                       // 'false'
	TK_FOR                         // 'for'
	TK_FUNCTION                    // 'function'
	TK_GLOBAL                      // 'global'
	TK_GOTO                        // 'goto'
	TK_IF                          // 'if'
	TK_IN                          // 'in'
	TK_LOCAL                       // 'local'
	TK_NIL                         // 'nil'
	TK_NOT                         // 'not'
	TK_OR                          // 'or'
	TK_REPEAT                      // 'repeat'
	TK_RETURN                      // 'return'
	TK_THEN                        // 'then'
	TK_TRUE                        // 'true'
	TK_UNTIL                       // 'until'
	TK_WHILE                       // 'while'
)

// Multi-character operator tokens.
const (
	TK_IDIV TokenType = iota + 512   // '//' Integer division
	TK_CONCAT                          // '..' Concatenation
	TK_DOTS                            // '...' Vararg
	TK_EQ                              // '==' Equality
	TK_GE                              // '>=' Greater than or equal
	TK_LE                              // '<=' Less than or equal
	TK_NE                              // '~=' Not equal
	TK_SHL                             // '<<' Left shift
	TK_SHR                             // '>>' Right shift
	TK_DBCOLON                         // '::' Label delimiter
)

// End of stream marker.
const (
	TK_EOS TokenType = iota + 640 // End of stream (internal use)
)

// Single-character operator tokens.
const (
	TK_PLUS TokenType = iota + 768    // '+' Addition
	TK_MINUS                            // '-' Subtraction/negation
	TK_STAR                             // '*' Multiplication
	TK_SLASH                            // '/' Division
	TK_PERCENT                          // '%' Modulo
	TK_CARET                            // '^' Exponentiation
	TK_HASH                             // '#' Length operator
	TK_LT                               // '<' Less than
	TK_GT                               // '>' Greater than
	TK_LPAREN                           // '(' Left parenthesis
	TK_RPAREN                           // ')' Right parenthesis
	TK_LBRACE                           // '{' Left brace
	TK_RBRACE                           // '}' Right brace
	TK_LBRACK                           // '[' Left bracket
	TK_RBRACK                           // ']' Right bracket
	TK_SEMICOLON                        // ';' Semicolon
	TK_COLON                            // ':' Colon
	TK_COMMA                            // ',' Comma
	TK_DOT                              // '.' Dot/period
	TK_ASSIGN                           // '=' Assignment
)