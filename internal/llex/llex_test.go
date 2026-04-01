package llex

import (
	"testing"
)

func TestTokenConstants(t *testing.T) {
	// Verify FirstReserved is 257
	if FirstReserved != 257 {
		t.Errorf("FirstReserved should be 257, got %d", FirstReserved)
	}

	// Verify NUM_RESERVED
	if NUM_RESERVED <= 0 {
		t.Error("NUM_RESERVED should be positive")
	}
}

func TestReservedWords(t *testing.T) {
	// Test some reserved word tokens
	if TK_AND != FirstReserved {
		t.Errorf("TK_AND should be %d, got %d", FirstReserved, TK_AND)
	}
	if TK_END != FirstReserved+5 {
		t.Errorf("TK_END should be %d, got %d", FirstReserved+5, TK_END)
	}
}

func TestTokenStrings(t *testing.T) {
	// Verify token strings array has correct length
	expectedLen := NUM_RESERVED + 7 // reserved words + other tokens (//, .., ..., ==, >=, <=, ~=, <<, >>, ::, <eof>, <number>, <integer>, <name>, <string>)
	if len(luaX_tokens) < expectedLen {
		t.Errorf("luaX_tokens array too short: expected at least %d, got %d", expectedLen, len(luaX_tokens))
	}
}

func TestLuaEnv(t *testing.T) {
	if LUA_ENV != "_ENV" {
		t.Errorf("LUA_ENV should be \"_ENV\", got %q", LUA_ENV)
	}
}

func TestIsAlpha(t *testing.T) {
	if !isalpha('a') || !isalpha('z') || !isalpha('A') || !isalpha('Z') || !isalpha('_') {
		t.Error("isalpha should return true for letters and underscore")
	}
	if isalpha('0') || isalpha('9') || isalpha(' ') {
		t.Error("isalpha should return false for digits and space")
	}
}

func TestIsDigit(t *testing.T) {
	if !isdigit('0') || !isdigit('9') {
		t.Error("isdigit should return true for digits")
	}
	if isdigit('a') || isdigit(' ') {
		t.Error("isdigit should return false for non-digits")
	}
}

func TestIsAlNum(t *testing.T) {
	if !isalnum('a') || !isalnum('1') || !isalnum('_') {
		t.Error("isalnum should return true for alphanumeric chars")
	}
}

func TestIsSpace(t *testing.T) {
	if !isspace(' ') || !isspace('\t') || !isspace('\n') {
		t.Error("isspace should return true for whitespace")
	}
	if isspace('a') || isspace('1') {
		t.Error("isspace should return false for non-whitespace")
	}
}

func TestIsXdigit(t *testing.T) {
	if !isxdigit('0') || !isxdigit('9') || !isxdigit('a') || !isxdigit('f') || !isxdigit('A') || !isxdigit('F') {
		t.Error("isxdigit should return true for hex digits")
	}
	if isxdigit('g') || isxdigit('z') {
		t.Error("isxdigit should return false for non-hex digits")
	}
}

func TestHexAValue(t *testing.T) {
	if hexavalue('0') != 0 || hexavalue('9') != 9 {
		t.Error("hexavalue for '0'-'9' incorrect")
	}
	if hexavalue('a') != 10 || hexavalue('f') != 15 {
		t.Error("hexavalue for 'a'-'f' incorrect")
	}
	if hexavalue('A') != 10 || hexavalue('F') != 15 {
		t.Error("hexavalue for 'A'-'F' incorrect")
	}
}

func TestCurrIsNewline(t *testing.T) {
	ls := &LexState{Current: '\n'}
	if !currIsNewline(ls) {
		t.Error("currIsNewline should return true for '\\n'")
	}
	ls.Current = '\r'
	if !currIsNewline(ls) {
		t.Error("currIsNewline should return true for '\\r'")
	}
	ls.Current = 'a'
	if currIsNewline(ls) {
		t.Error("currIsNewline should return false for 'a'")
	}
}
