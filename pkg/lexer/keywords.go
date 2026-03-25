// Package lexer implements the Lua lexical analyzer.
//
// This file provides the keyword mapping for Lua reserved words,
// enabling O(1) lookup to determine if an identifier is a reserved word.

package lexer

// Keywords maps Lua reserved words to their corresponding token types.
// This map provides O(1) lookup during lexical analysis to determine
// if an identifier string is a reserved word.
//
// The map contains all 23 Lua 5.x reserved words in lowercase.
// Lua is case-sensitive, so keywords must be exactly as specified.
var Keywords map[string]TokenType

// initKeywords initializes the Keywords map with all Lua reserved words.
// This function populates the map with the 23 reserved words defined
// in the Lua language specification, mapping each keyword string to
// its corresponding TK_* token type constant.
//
// Keyword categories:
//   - Logical operators: and, or, not
//   - Control flow: if, then, else, elseif, end, while, do, repeat, until, for, break, return
//   - Declarations: function, local, global, goto
//   - Literals: true, false, nil
//   - Other: in
func initKeywords() {
	Keywords = make(map[string]TokenType, 24)

	// Logical operators
	Keywords["and"] = TK_AND
	Keywords["or"] = TK_OR
	Keywords["not"] = TK_NOT

	// Control flow statements
	Keywords["if"] = TK_IF
	Keywords["then"] = TK_THEN
	Keywords["else"] = TK_ELSE
	Keywords["elseif"] = TK_ELSEIF
	Keywords["end"] = TK_END
	Keywords["while"] = TK_WHILE
	Keywords["do"] = TK_DO
	Keywords["repeat"] = TK_REPEAT
	Keywords["until"] = TK_UNTIL
	Keywords["for"] = TK_FOR
	Keywords["break"] = TK_BREAK
	Keywords["return"] = TK_RETURN

	// Declarations and scope
	Keywords["function"] = TK_FUNCTION
	Keywords["local"] = TK_LOCAL
	Keywords["global"] = TK_GLOBAL
	Keywords["goto"] = TK_GOTO

	// Literals
	Keywords["true"] = TK_TRUE
	Keywords["false"] = TK_FALSE
	Keywords["nil"] = TK_NIL

	// Other
	Keywords["in"] = TK_IN
}

// init initializes the Keywords map when the package is loaded.
// Using init() ensures thread-safe initialization without sync.Once.
func init() {
	initKeywords()
}