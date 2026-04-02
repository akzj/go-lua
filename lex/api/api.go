// Package api defines the Lua lexical analyzer interface.
// NO dependencies - pure interface definitions.
package api

// =============================================================================
// Token Types
// =============================================================================

// TokenType represents the type of a lexical token.
// Order matters: FIRST_RESERVED must match luaX_tokens array order.
// Why not int constants? Type safety - callers cannot accidentally mix int and TokenType.
type TokenType int

// Reserved keywords and operators (FIRST_RESERVED = 0)
const (
	// Keywords
	TOKEN_AND TokenType = iota
	TOKEN_BREAK
	TOKEN_DO
	TOKEN_ELSE
	TOKEN_ELSEIF
	TOKEN_END
	TOKEN_FALSE
	TOKEN_FOR
	TOKEN_FUNCTION
	TOKEN_GLOBAL // Lua 5.4 compatibility
	TOKEN_GOTO
	TOKEN_CONST // Lua 5.4 compatibility - for "global const"
	TOKEN_IF
	TOKEN_IN
	TOKEN_LOCAL
	TOKEN_NIL
	TOKEN_NOT
	TOKEN_OR
	TOKEN_REPEAT
	TOKEN_RETURN
	TOKEN_THEN
	TOKEN_TRUE
	TOKEN_UNTIL
	TOKEN_WHILE

	// Multi-char operators (iota from TOKEN_IDIV=27)
	TOKEN_IDIV   // //
	TOKEN_CONCAT // ..
	TOKEN_DOTS   // ...
	TOKEN_EQ     // ==
	TOKEN_GE     // >=
	TOKEN_LE     // <=
	TOKEN_NE     // ~=
	TOKEN_SHL    // <<
	TOKEN_SHR    // >>
	TOKEN_DBCOLON // ::

	// Pseudo-tokens (not in source, returned by lexer)
	TOKEN_EOS // end of source

	// Token types with semantic values
	TOKEN_NUMBER  // numeric literal (float or int as string)
	TOKEN_INTEGER // integer literal (kept separate for parser optimization)
	TOKEN_NAME    // identifier
	TOKEN_STRING  // string literal
)

// Single-character tokens returned as int values
const (
	TOKEN_SINGLE_CHAR_MIN TokenType = 256 // Above ASCII range

	TOKEN_PLUS   TokenType = '+'
	TOKEN_MINUS  TokenType = '-'
	TOKEN_MUL    TokenType = '*'
	TOKEN_DIV    TokenType = '/'
	TOKEN_MOD    TokenType = '%'
	TOKEN_POW    TokenType = '^'
	TOKEN_HASH   TokenType = '#'
	TOKEN_AMP    TokenType = '&'
	TOKEN_PIPE   TokenType = '|'
	TOKEN_LT     TokenType = '<'
	TOKEN_GT     TokenType = '>'
	TOKEN_ASSIGN TokenType = '='

	TOKEN_LPAREN  TokenType = '('
	TOKEN_RPAREN  TokenType = ')'
	TOKEN_LBRACK  TokenType = '['
	TOKEN_RBRACK  TokenType = ']'
	TOKEN_LBRACE  TokenType = '{'
	TOKEN_RBRACE  TokenType = '}'

	TOKEN_SEMICOLON TokenType = ';'
	TOKEN_COLON     TokenType = ':'
	TOKEN_COMMA     TokenType = ','
	TOKEN_DOT       TokenType = '.'
	TOKEN_TILDE     TokenType = '~'
)

// IsKeyword returns true if the token type is a Lua keyword.
func (tt TokenType) IsKeyword() bool {
	return tt >= TOKEN_AND && tt <= TOKEN_WHILE
}

// IsOperator returns true if the token type is a multi-char operator.
func (tt TokenType) IsOperator() bool {
	return tt >= TOKEN_IDIV && tt <= TOKEN_DBCOLON
}

// IsLiteral returns true if the token type carries semantic value.
func (tt TokenType) IsLiteral() bool {
	return tt >= TOKEN_NUMBER && tt <= TOKEN_STRING
}

// IsSingleChar returns true for single-character tokens.
func (tt TokenType) IsSingleChar() bool {
	return tt >= TOKEN_SINGLE_CHAR_MIN
}

// TokenTypeName returns the token name for debugging.
func TokenTypeName(tt TokenType) string {
	names := []string{
		"and", "break", "do", "else", "elseif",
		"end", "false", "for", "function", "global", "goto", "if",
		"in", "local", "nil", "not", "or", "repeat",
		"return", "then", "true", "until", "while",
		"//", "..", "...", "==", ">=", "<=", "~=",
		"<<", ">>", "::", "<eof>", "<number>", "<integer>", "<name>", "<string>",
	}
	if int(tt) < len(names) {
		return names[tt]
	}
	return "<unknown>"
}

// =============================================================================
// Token
// =============================================================================

// Token represents a lexical token with its position and value.
// Value type struct - immutable after creation.
type Token struct {
	Type   TokenType // Token type
	Value  string    // String value (for names, numbers, strings)
	Line   int       // Line number (1-based)
	Column int       // Column number (1-based, byte offset in line)
}

// =============================================================================
// Lexer Interface
// =============================================================================

// Lexer is the interface for Lua lexical analysis.
// All operations needed for parsing are here.
type Lexer interface {
	// NextToken returns the next token from the input.
	// Advances the lexer's position by one token.
	NextToken() Token

	// Lookahead returns the next token without advancing.
	// Used for two-token lookahead (e.g., '..' vs '...').
	Lookahead() Token

	// CurrentLine returns the current line number (1-based).
	// The line of the most recently returned token.
	CurrentLine() int

	// CurrentColumn returns the current column (1-based byte offset).
	CurrentColumn() int

	// Error reports a lexical error.
	// Implementation should include source position in error message.
	Error(msg string)

	// SourceName returns the name of the source (e.g., filename or =stdin).
	SourceName() string
}

// =============================================================================
// Lexer Factory
// =============================================================================



// NewLexer is in lex/lex.go (avoids circular dependency)
