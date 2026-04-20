// Package api defines the Lua lexer (scanner) types and interface.
//
// The lexer converts Lua source text into a stream of tokens.
// It handles all escape sequences, long strings, comments, number parsing,
// and reserved word detection.
//
// Reference: .analysis/06-compiler-pipeline.md §2
package api

// ---------------------------------------------------------------------------
// Token types
//
// Single-char tokens use their ASCII value (<FirstReserved).
// Reserved words and multi-char tokens use the constants below.
// ---------------------------------------------------------------------------

// FirstReserved is the boundary between single-char tokens and reserved words.
const FirstReserved = 256

// TokenType is the type of a lexical token.
type TokenType int

// Reserved words (FirstReserved + index)
const (
	TK_AND    TokenType = iota + FirstReserved
	TK_BREAK
	TK_DO
	TK_ELSE
	TK_ELSEIF
	TK_END
	TK_FALSE
	TK_FOR
	TK_FUNCTION
	TK_GLOBAL // Lua 5.5
	TK_GOTO
	TK_IF
	TK_IN
	TK_LOCAL
	TK_NIL
	TK_NOT
	TK_OR
	TK_REPEAT
	TK_RETURN
	TK_THEN
	TK_TRUE
	TK_UNTIL
	TK_WHILE
	NumReserved // count of reserved words = 23
)

// NumReservedCount is the actual count of reserved words (23).
// NumReserved above is a token value, not a count.
const NumReservedCount = NumReserved - FirstReserved

// Multi-char operators and value tokens
const (
	TK_IDIV   TokenType = iota + NumReserved // //
	TK_CONCAT                                                // ..
	TK_DOTS                                                  // ...
	TK_EQ                                                    // ==
	TK_GE                                                    // >=
	TK_LE                                                    // <=
	TK_NE                                                    // ~=
	TK_SHL                                                   // <<
	TK_SHR                                                   // >>
	TK_DBCOLON                                               // ::
	TK_EOS                                                   // <eof>
	TK_FLT                                                   // <number> (float)
	TK_INT                                                   // <integer>
	TK_NAME                                                  // <name>
	TK_STRING                                                // <string>
)

// ---------------------------------------------------------------------------
// Token carries the token type and its semantic value.
// ---------------------------------------------------------------------------
type Token struct {
	Type   TokenType
	IntVal int64   // for TK_INT
	FltVal float64 // for TK_FLT
	StrVal string  // for TK_NAME, TK_STRING
}

// ---------------------------------------------------------------------------
// ReservedWords maps reserved word strings to their token types.
// Used during identifier scanning to detect keywords.
// ---------------------------------------------------------------------------
var ReservedWords = map[string]TokenType{
	"and": TK_AND, "break": TK_BREAK, "do": TK_DO,
	"else": TK_ELSE, "elseif": TK_ELSEIF, "end": TK_END,
	"false": TK_FALSE, "for": TK_FOR, "function": TK_FUNCTION,
	"global": TK_GLOBAL, "goto": TK_GOTO, "if": TK_IF,
	"in": TK_IN, "local": TK_LOCAL, "nil": TK_NIL,
	"not": TK_NOT, "or": TK_OR, "repeat": TK_REPEAT,
	"return": TK_RETURN, "then": TK_THEN, "true": TK_TRUE,
	"until": TK_UNTIL, "while": TK_WHILE,
}

// ---------------------------------------------------------------------------
// LexState is the lexer state. It holds the input stream, current token,
// lookahead token, and line tracking.
//
// This is a concrete struct (not an interface) because the parser directly
// accesses its fields for performance and simplicity.
// ---------------------------------------------------------------------------
type LexState struct {
	Current  int    // current character (rune or -1 for EOF)
	Line     int    // current input line number
	LastLine int    // line of last consumed token
	Token    Token  // current token
	Lookahead Token // lookahead token (Type == TK_EOS if empty)
	HasAhead bool   // whether lookahead is populated

	Source    string // source name (for error messages)
	EnvName   string // "_ENV" string
	BreakName string // "break" string (used as label name)

	// Input reading (set during initialization)
	Reader LexReader // character source
	Buf    []byte    // token buffer

	// Back-reference to parser state (set by parser)
	// Uses any to avoid circular import with parse module.
	FuncState any // current FuncState (set by parser)
	DynData   any // shared dynamic data (set by parser)

	// Parser nesting depth counter (mirrors C Lua's L->nCcalls)
	NestLevel    int // current nesting depth
	MaxNestLevel int // maximum allowed nesting (default 200)
}

// LexReader is the interface for reading source characters.
type LexReader interface {
	// ReadByte returns the next byte, or -1 on EOF.
	ReadByte() int
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// SyntaxError is raised (via panic) for lexical/syntax errors.
type SyntaxError struct {
	Source  string
	Line    int
	Token   string // token that caused the error (or "")
	Message string
}

func (e *SyntaxError) Error() string {
	return e.Message // detailed formatting includes source:line
}
