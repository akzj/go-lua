// Package lexer implements the Lua lexical analyzer.
//
// This file provides helper functions for character classification
// and scanner operations used by the main lexer.

package lexer

// isAlpha reports whether c is an alphabetic character.
// Lua identifiers start with a letter or underscore.
func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isDigit reports whether c is a decimal digit.
func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// isHexDigit reports whether c is a hexadecimal digit.
func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// isAlphaNum reports whether c is alphanumeric or underscore.
// Used for continuing identifiers after the first character.
func isAlphaNum(c byte) bool {
	return isAlpha(c) || isDigit(c) || c == '_'
}

// isSpace reports whether c is a whitespace character.
// Lua considers space, tab, formfeed, and vertical tab as whitespace.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\f' || c == '\v'
}

// isNewline reports whether c is a newline character.
// Lua handles both Unix (\n) and Windows (\r\n) line endings.
func isNewline(c byte) bool {
	return c == '\n' || c == '\r'
}

// Scanner provides low-level character scanning utilities for the lexer.
// It wraps a byte slice source and provides methods for reading characters,
// peeking ahead, and managing position tracking.
type Scanner struct {
	source []byte  // Source code being scanned
	pos    int     // Current position in source
	line   int     // Current line number (1-based)
	column int     // Current column number (0-based)
	current byte  // Current character (0 if at end)
}

// NewScanner creates a new scanner for the given source code.
// The name parameter is used for error messages (typically the filename).
func NewScanner(source []byte) *Scanner {
	s := &Scanner{
		source: source,
		line:   1,
		column: 0,
	}
	if len(source) > 0 {
		s.current = source[0]
	}
	return s
}

// Current returns the current character being scanned.
// Returns 0 if at end of source.
func (s *Scanner) Current() byte {
	return s.current
}

// Pos returns the current position in the source.
func (s *Scanner) Pos() int {
	return s.pos
}

// Line returns the current line number (1-based).
func (s *Scanner) Line() int {
	return s.line
}

// Column returns the current column number (0-based).
func (s *Scanner) Column() int {
	return s.column
}

// Advance moves to the next character in the source.
// Updates line and column tracking appropriately.
func (s *Scanner) Advance() {
	if s.pos >= len(s.source) {
		s.current = 0
		return
	}
	s.pos++
	if s.current == '\n' {
		s.line++
		s.column = 0
	} else {
		s.column++
	}
	if s.pos < len(s.source) {
		s.current = s.source[s.pos]
	} else {
		s.current = 0
	}
}

// Peek returns the next character without advancing.
// Returns 0 if at end of source or if offset is out of bounds.
func (s *Scanner) Peek(offset int) byte {
	idx := s.pos + 1 + offset
	if idx < 0 || idx >= len(s.source) {
		return 0
	}
	return s.source[idx]
}

// Peek1 returns the immediate next character (shorthand for Peek(0)).
func (s *Scanner) Peek1() byte {
	return s.Peek(0)
}

// Match advances if the current character matches the expected character.
// Returns true if matched and advanced, false otherwise.
func (s *Scanner) Match(expected byte) bool {
	if s.current == expected {
		s.Advance()
		return true
	}
	return false
}

// MatchAny advances if the current character matches any of the given characters.
// Returns true if matched and advanced, false otherwise.
func (s *Scanner) MatchAny(chars ...byte) bool {
	for _, c := range chars {
		if s.current == c {
			s.Advance()
			return true
		}
	}
	return false
}

// SkipWhile advances while the predicate returns true.
// Returns the number of characters skipped.
func (s *Scanner) SkipWhile(pred func(byte) bool) int {
	count := 0
	for pred(s.current) {
		s.Advance()
		count++
	}
	return count
}

// AtEnd reports whether the scanner is at the end of the source.
func (s *Scanner) AtEnd() bool {
	return s.current == 0
}

// Substring returns a substring from the source.
// Start and end are byte offsets in the source.
func (s *Scanner) Substring(start, end int) []byte {
	if start < 0 || end > len(s.source) || start >= end {
		return []byte{}
	}
	return s.source[start:end]
}