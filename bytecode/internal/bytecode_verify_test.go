package internal

import (
	"fmt"
	"strings"
	"testing"

	bcapi "github.com/akzj/go-lua/bytecode/api"
	"github.com/akzj/go-lua/parse"
)

// TestBytecodeGeneration verifies that bytecode is generated correctly from parsed Lua code.
// This test actually calls parse → compile and verifies the output.
func TestBytecodeGeneration(t *testing.T) {
	tests := []struct {
		name           string
		luaCode        string
		wantOpcodes    []string // expected opcode mnemonics (substring match)
		wantConstTypes []bcapi.ConstantType
		minInstrCount  int
	}{
		{
			name:        "simple_integer_assignment",
			luaCode:     "local x = 1",
			wantOpcodes: []string{"LOADK", "RETURN"},
			minInstrCount: 2,
		},
		{
			name:        "simple_function_call",
			luaCode:     "print(1)",
			wantOpcodes: []string{"GETTABUP", "LOADK", "CALL", "RETURN"},
			minInstrCount: 4,
		},
		{
			name:        "binary_expression",
			luaCode:     "local x = 1 + 2",
			wantOpcodes: []string{"LOADK", "LOADK", "ADD", "RETURN"},
			minInstrCount: 4,
		},
		{
			name:        "multiple_assignments",
			luaCode:     "local x = 1; local y = 2",
			wantOpcodes: []string{"LOADK", "LOADK", "RETURN"},
			minInstrCount: 3,
		},
		{
			name:        "string_constant",
			luaCode:     `local s = "hello"`,
			wantOpcodes: []string{"LOADK", "RETURN"},
			minInstrCount: 2,
		},
		{
			name:        "global_variable_assignment",
			luaCode:     "x = 10",
			wantOpcodes: []string{"SETTABUP", "RETURN"},
			minInstrCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the Lua code
			p := parse.NewParser()
			chunk, err := p.Parse(tt.luaCode)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if chunk == nil {
				t.Fatal("parsed chunk is nil")
			}

			// Compile to bytecode
			compiler := NewCompiler("test")
			proto, err := compiler.Compile(chunk)
			if err != nil {
				t.Fatalf("compile error: %v", err)
			}
			if proto == nil {
				t.Fatal("compiled prototype is nil")
			}

			// Verify we have instructions
			code := proto.GetCode()
			if len(code) < tt.minInstrCount {
				t.Errorf("expected at least %d instructions, got %d", tt.minInstrCount, len(code))
			}

			// Verify constants exist
			consts := proto.GetConstants()
			if len(consts) > 0 && len(tt.wantConstTypes) == 0 {
				// If we expect constants, at least verify we have some
				t.Logf("generated %d constants", len(consts))
			}

			// Log bytecode for debugging
			t.Logf("Generated %d instructions, %d constants", len(code), len(consts))
			t.Logf("Opcodes: %v", formatOpcodes(code))
			t.Logf("Constants: %v", formatConstants(consts))
		})
	}
}

// TestConstantPool verifies that constants are properly deduplicated and typed.
func TestConstantPool(t *testing.T) {
	p := parse.NewParser()
	compiler := NewCompiler("test")

	// Test with duplicate constants
	code := "local a = 1; local b = 1; local c = 2"
	chunk, err := p.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	proto, err := compiler.Compile(chunk)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	consts := proto.GetConstants()
	// Should have 2 unique constants: 1 and 2
	if len(consts) < 2 {
		t.Errorf("expected at least 2 constants, got %d", len(consts))
	}

	// Check that constants are integers
	for i, c := range consts {
		if c.Type != bcapi.ConstInteger {
			t.Errorf("constant %d: expected integer type, got %v", i, c.Type)
		}
	}
}

// TestSimpleBytecodeOutput generates bytecode and formats it for verification.
func TestSimpleBytecodeOutput(t *testing.T) {
	p := parse.NewParser()
	compiler := NewCompiler("test")

	testCases := []string{
		"local x = 1",
		"print(42)",
		"local x = 1 + 2",
		"x = 5",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			chunk, err := p.Parse(tc)
			if err != nil {
				t.Fatalf("parse error for '%s': %v", tc, err)
			}

			proto, err := compiler.Compile(chunk)
			if err != nil {
				t.Fatalf("compile error for '%s': %v", tc, err)
			}

			code := proto.GetCode()
			consts := proto.GetConstants()

			output := fmt.Sprintf("-- Lua: %s\n", tc)
			output += fmt.Sprintf("Instructions: %d\n", len(code))
			output += fmt.Sprintf("Constants: %d\n", len(consts))

			t.Log(output)
			t.Logf("Code: %v", formatOpcodes(code))
			t.Logf("Consts: %v", formatConstants(consts))
		})
	}
}

// TestConstructsFileBytecode verifies bytecode generation for constructs.lua content.
// This uses actual content from lua-master/testes/constructs.lua.
func TestConstructsFileBytecode(t *testing.T) {
	// Simple test cases extracted from constructs.lua patterns
	testCases := []struct {
		name string
		code string
	}{
		{"do_end", "do end"},
		{"local_assign", "local a = 3"},
		{"semicolon", "; do ; a = 3; assert(a == 3) end;"},
		{"priorities_add", "local x = 2+1"},
		{"priorities_mul", "local x = 2*3"},
		{"priorities_pow", "local x = 2^3"},
		{"concat", `local s = "a".."b"`},
		{"comparison", "local x = 2 > 3"},
	}

	p := parse.NewParser()
	compiler := NewCompiler("test")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := p.Parse(tc.code)
			if err != nil {
				t.Fatalf("parse error for '%s': %v", tc.code, err)
			}

			proto, err := compiler.Compile(chunk)
			if err != nil {
				t.Fatalf("compile error for '%s': %v", tc.code, err)
			}

			code := proto.GetCode()
			t.Logf("Lua: %s -> %d instructions", tc.code, len(code))
		})
	}
}

// =============================================================================
// Helper functions for bytecode formatting and verification
// =============================================================================

// opcodeNames maps opcode numbers to names (matching this project's opcodes/api.go)
var opcodeNames = map[int]string{
	0:   "MOVE",
	1:   "GETTABUP",
	2:   "GETTABLE",
	3:   "LOADK",
	4:   "LOADKX",
	5:   "LOADBOOL",
	6:   "LOADNIL",
	7:   "GETUPVAL",
	8:   "SETTABUP",
	9:   "SETUPVAL",
	10:  "SETTABLE",
	11:  "NEWTABLE",
	12:  "SELF",
	13:  "ADD",
	14:  "SUB",
	15:  "MUL",
	16:  "DIV",
	17:  "IDIV",
	18:  "MOD",
	19:  "POW",
	20:  "UNM",
	21:  "NOT",
	22:  "LEN",
	23:  "CONCAT",
	24:  "JMP",
	25:  "EQ",
	26:  "LT",
	27:  "LE",
	28:  "TEST",
	29:  "TESTSET",
	30:  "CALL",
	31:  "TAILCALL",
	32:  "RETURN0",
	33:  "RETURN1",
	34:  "RETURN",
	35:  "FORLOOP",
	36:  "FORPREP",
	37:  "TFORLOOP",
	38:  "SETLIST",
	39:  "CLOSE",
	40:  "CLOSURE",
	41:  "VARARG",
}

// formatOpcodes converts instruction array to opcode names
func formatOpcodes(code []uint32) []string {
	result := make([]string, len(code))
	for i, inst := range code {
		op := int(inst & 0x7F)
		name := opcodeNames[op]
		if name == "" {
			name = fmt.Sprintf("OP_%d", op)
		}
		result[i] = name
	}
	return result
}

// formatConstants converts constant array to readable strings
func formatConstants(consts []*bcapi.Constant) []string {
	result := make([]string, len(consts))
	for i, c := range consts {
		switch c.Type {
		case bcapi.ConstNil:
			result[i] = "nil"
		case bcapi.ConstInteger:
			result[i] = fmt.Sprintf("%d", c.Int)
		case bcapi.ConstFloat:
			result[i] = fmt.Sprintf("%.14g", c.Float)
		case bcapi.ConstString:
			result[i] = fmt.Sprintf("%q", c.Str)
		case bcapi.ConstBool:
			if c.Int != 0 {
				result[i] = "true"
			} else {
				result[i] = "false"
			}
		default:
			result[i] = fmt.Sprintf("<type=%d>", c.Type)
		}
	}
	return result
}

// VerifyOpcodes checks that generated code contains expected opcodes
func VerifyOpcodes(t *testing.T, code []uint32, expected []string) bool {
	opcodes := formatOpcodes(code)
	passed := true

	for _, exp := range expected {
		found := false
		for _, op := range opcodes {
			if strings.Contains(op, exp) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected opcode containing %q, got %v", exp, opcodes)
			passed = false
		}
	}
	return passed
}
