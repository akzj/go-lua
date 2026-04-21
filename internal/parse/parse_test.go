package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akzj/go-lua/internal/lex"
	"github.com/akzj/go-lua/internal/object"
	"github.com/akzj/go-lua/internal/opcode"
)

// ---------------------------------------------------------------------------
// Test helper: StringReader for parsing from strings
// ---------------------------------------------------------------------------

type stringReader struct {
	data string
	pos  int
}

func newStringReader(s string) *stringReader {
	return &stringReader{data: s}
}

func (r *stringReader) NextByte() int {
	if r.pos >= len(r.data) {
		return -1
	}
	b := r.data[r.pos]
	r.pos++
	return int(b)
}

// parseString compiles a Lua source string and returns the Proto.
func parseString(src string) *object.Proto {
	return Parse("test", newStringReader(src))
}

// expectParse verifies that source compiles without panic.
func expectParse(t *testing.T, src string) *object.Proto {
	t.Helper()
	p := parseString(src)
	if p == nil {
		t.Fatal("Parse returned nil")
	}
	return p
}

// expectParseError verifies that source causes a syntax error.
func expectParseError(t *testing.T, src string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected parse error, got none")
		}
		if _, ok := r.(*lex.SyntaxError); !ok {
			t.Fatalf("expected *SyntaxError, got %T: %v", r, r)
		}
	}()
	parseString(src)
}

// ===========================================================================
// Smoke tests — verify Parse doesn't panic on valid input
// ===========================================================================

func TestParseEmpty(t *testing.T) {
	p := expectParse(t, "")
	if !p.IsVararg() {
		t.Error("main function should be vararg")
	}
	if len(p.Upvalues) != 1 {
		t.Errorf("expected 1 upvalue (_ENV), got %d", len(p.Upvalues))
	}
	if p.Upvalues[0].Name == nil || p.Upvalues[0].Name.Data != "_ENV" {
		t.Error("first upvalue should be _ENV")
	}
}

func TestParseSemicolons(t *testing.T) {
	expectParse(t, ";;;")
}

func TestParseReturn(t *testing.T) {
	p := expectParse(t, "return 42")
	if len(p.Code) == 0 {
		t.Fatal("expected bytecode instructions")
	}
}

func TestParseReturnNil(t *testing.T) {
	expectParse(t, "return nil")
}

func TestParseReturnTrue(t *testing.T) {
	expectParse(t, "return true")
}

func TestParseReturnFalse(t *testing.T) {
	expectParse(t, "return false")
}

func TestParseReturnString(t *testing.T) {
	p := expectParse(t, "return 'hello'")
	// Should have "hello" in constant pool
	found := false
	for _, k := range p.Constants {
		if sv := k.StringVal(); sv != nil && sv.Data == "hello" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'hello' in constant pool")
	}
}

func TestParseReturnFloat(t *testing.T) {
	expectParse(t, "return 3.14")
}

func TestParseReturnMultiple(t *testing.T) {
	expectParse(t, "return 1, 2, 3")
}

// ===========================================================================
// Local variables
// ===========================================================================

func TestParseLocal(t *testing.T) {
	p := expectParse(t, "local x = 10")
	if len(p.LocVars) < 1 {
		t.Fatal("expected at least 1 LocVar")
	}
	if p.LocVars[0].Name == nil || p.LocVars[0].Name.Data != "x" {
		t.Errorf("expected LocVar 'x', got %v", p.LocVars[0].Name)
	}
}

func TestParseLocalMulti(t *testing.T) {
	p := expectParse(t, "local a, b, c = 1, 2, 3")
	names := make([]string, len(p.LocVars))
	for i, lv := range p.LocVars {
		if lv.Name != nil {
			names[i] = lv.Name.Data
		}
	}
	if len(p.LocVars) < 3 {
		t.Fatalf("expected at least 3 LocVars, got %d: %v", len(p.LocVars), names)
	}
}

func TestParseLocalNil(t *testing.T) {
	p := expectParse(t, "local x")
	if len(p.LocVars) < 1 {
		t.Fatal("expected at least 1 LocVar")
	}
}

func TestParseLocalConst(t *testing.T) {
	expectParse(t, "local x <const> = 42")
}

// ===========================================================================
// Arithmetic expressions
// ===========================================================================

func TestParseArith(t *testing.T) {
	expectParse(t, "local x = 1 + 2 * 3")
}

func TestParseArithComplex(t *testing.T) {
	expectParse(t, "local x = (1 + 2) * 3 - 4 / 2")
}

func TestParseUnary(t *testing.T) {
	expectParse(t, "local x = -1")
}

func TestParseConcat(t *testing.T) {
	expectParse(t, "local x = 'a' .. 'b' .. 'c'")
}

func TestParseBitwise(t *testing.T) {
	expectParse(t, "local x = 1 & 2 | 3 ~ 4")
}

func TestParseComparison(t *testing.T) {
	expectParse(t, "local x = 1 < 2")
}

func TestParseLogical(t *testing.T) {
	expectParse(t, "local x = true and false or nil")
}

func TestParsePower(t *testing.T) {
	// Power is right-associative: 2^3^4 = 2^(3^4)
	expectParse(t, "local x = 2^3^4")
}

func TestParseLen(t *testing.T) {
	expectParse(t, "local t = {}; local x = #t")
}

func TestParseNot(t *testing.T) {
	expectParse(t, "local x = not true")
}

func TestParseBnot(t *testing.T) {
	expectParse(t, "local x = ~0")
}

// ===========================================================================
// If statements
// ===========================================================================

func TestParseIf(t *testing.T) {
	expectParse(t, "if true then local x = 1 end")
}

func TestParseIfElse(t *testing.T) {
	expectParse(t, "if true then return 1 else return 2 end")
}

func TestParseIfElseif(t *testing.T) {
	expectParse(t, "if true then return 1 elseif false then return 2 else return 3 end")
}

// ===========================================================================
// Loops
// ===========================================================================

func TestParseWhile(t *testing.T) {
	expectParse(t, "while true do break end")
}

func TestParseRepeat(t *testing.T) {
	expectParse(t, "repeat local x = 1 until x > 0")
}

func TestParseForNumeric(t *testing.T) {
	p := expectParse(t, "for i = 1, 10 do end")
	// Should have for-loop opcodes
	hasForPrep := false
	hasForLoop := false
	for _, inst := range p.Code {
		op := opcode.OpCode(inst & 0x7F)
		if op == opcode.OP_FORPREP {
			hasForPrep = true
		}
		if op == opcode.OP_FORLOOP {
			hasForLoop = true
		}
	}
	if !hasForPrep {
		t.Error("expected OP_FORPREP")
	}
	if !hasForLoop {
		t.Error("expected OP_FORLOOP")
	}
}

func TestParseForNumericStep(t *testing.T) {
	expectParse(t, "for i = 1, 10, 2 do end")
}

func TestParseForGeneric(t *testing.T) {
	p := expectParse(t, "for k, v in next, t do end")
	hasTForPrep := false
	for _, inst := range p.Code {
		op := opcode.OpCode(inst & 0x7F)
		if op == opcode.OP_TFORPREP {
			hasTForPrep = true
		}
	}
	if !hasTForPrep {
		t.Error("expected OP_TFORPREP")
	}
}

func TestParseDo(t *testing.T) {
	expectParse(t, "do local x = 1 end")
}

// ===========================================================================
// Functions
// ===========================================================================

func TestParseFunction(t *testing.T) {
	p := expectParse(t, "local function f(a, b) return a + b end")
	if len(p.Protos) < 1 {
		t.Fatal("expected at least 1 nested proto")
	}
	inner := p.Protos[0]
	if inner.NumParams != 2 {
		t.Errorf("expected 2 params, got %d", inner.NumParams)
	}
}

func TestParseFunctionNoParams(t *testing.T) {
	p := expectParse(t, "local function f() end")
	if len(p.Protos) < 1 {
		t.Fatal("expected nested proto")
	}
	if p.Protos[0].NumParams != 0 {
		t.Errorf("expected 0 params, got %d", p.Protos[0].NumParams)
	}
}

func TestParseVararg(t *testing.T) {
	p := expectParse(t, "local function f(...) return ... end")
	if len(p.Protos) < 1 {
		t.Fatal("expected nested proto")
	}
	if !p.Protos[0].IsVararg() {
		t.Error("expected vararg function")
	}
}

func TestParseAnonymousFunction(t *testing.T) {
	expectParse(t, "local f = function(x) return x end")
}

func TestParseMethodCall(t *testing.T) {
	expectParse(t, "local t = {}; t:foo()")
}

func TestParseFunctionStatement(t *testing.T) {
	expectParse(t, "function f() end")
}

// ===========================================================================
// Table constructors
// ===========================================================================

func TestParseTableEmpty(t *testing.T) {
	expectParse(t, "local t = {}")
}

func TestParseTableList(t *testing.T) {
	expectParse(t, "local t = {1, 2, 3}")
}

func TestParseTableRecord(t *testing.T) {
	expectParse(t, "local t = {x=1, y=2}")
}

func TestParseTableMixed(t *testing.T) {
	expectParse(t, "local t = {1, x=2, [3]=4}")
}

func TestParseTableNested(t *testing.T) {
	expectParse(t, "local t = {{1}, {2}}")
}

func TestParseTableTrailingComma(t *testing.T) {
	expectParse(t, "local t = {1, 2, 3,}")
}

func TestParseTableTrailingSemicolon(t *testing.T) {
	expectParse(t, "local t = {1; 2; 3;}")
}

// ===========================================================================
// Assignment
// ===========================================================================

func TestParseAssignment(t *testing.T) {
	expectParse(t, "local x; x = 1")
}

func TestParseMultiAssign(t *testing.T) {
	expectParse(t, "local a, b; a, b = 1, 2")
}

func TestParseGlobalAssign(t *testing.T) {
	expectParse(t, "x = 1")
}

func TestParseTableAssign(t *testing.T) {
	expectParse(t, "local t = {}; t.x = 1")
}

func TestParseIndexAssign(t *testing.T) {
	expectParse(t, "local t = {}; t[1] = 'a'")
}

// ===========================================================================
// Goto and labels
// ===========================================================================

func TestParseGoto(t *testing.T) {
	expectParse(t, "goto done; ::done::")
}

func TestParseGotoForward(t *testing.T) {
	expectParse(t, "goto skip; local x = 1; ::skip::")
}

// ===========================================================================
// Nested functions and upvalues
// ===========================================================================

func TestParseUpvalue(t *testing.T) {
	p := expectParse(t, "local x = 1; local function f() return x end")
	if len(p.Protos) < 1 {
		t.Fatal("expected nested proto")
	}
	inner := p.Protos[0]
	if len(inner.Upvalues) < 1 {
		t.Fatal("expected at least 1 upvalue in inner function")
	}
	// First upvalue should capture x from enclosing scope
	uv := inner.Upvalues[0]
	if !uv.InStack {
		t.Error("upvalue should be InStack (captured from enclosing function's registers)")
	}
}

func TestParseNestedUpvalue(t *testing.T) {
	// x captured through two levels
	p := expectParse(t, `
local x = 1
local function f()
    local function g()
        return x
    end
    return g
end
`)
	if len(p.Protos) < 1 {
		t.Fatal("expected nested proto")
	}
	f := p.Protos[0]
	if len(f.Protos) < 1 {
		t.Fatal("expected nested proto in f")
	}
	g := f.Protos[0]
	if len(g.Upvalues) < 1 {
		t.Fatal("expected upvalue in g")
	}
	// g's upvalue should NOT be InStack (it comes from f's upvalue, not f's stack)
	if g.Upvalues[0].InStack {
		t.Error("g's upvalue should NOT be InStack (captured from f's upvalue)")
	}
}

// ===========================================================================
// Complex programs (smoke tests)
// ===========================================================================

func TestParseFibonacci(t *testing.T) {
	p := expectParse(t, `
local function fib(n)
    if n < 2 then return n end
    return fib(n-1) + fib(n-2)
end
return fib(10)
`)
	if p == nil {
		t.Fatal("Parse returned nil")
	}
	if len(p.Protos) != 1 {
		t.Errorf("expected 1 nested proto (fib), got %d", len(p.Protos))
	}
}

func TestParseCounter(t *testing.T) {
	expectParse(t, `
local function counter()
    local n = 0
    return function()
        n = n + 1
        return n
    end
end
local c = counter()
return c(), c(), c()
`)
}

func TestParseSorter(t *testing.T) {
	expectParse(t, `
local function sort(t)
    for i = 2, #t do
        local key = t[i]
        local j = i - 1
        while j > 0 and t[j] > key do
            t[j+1] = t[j]
            j = j - 1
        end
        t[j+1] = key
    end
    return t
end
`)
}

func TestParseRepeatUntil(t *testing.T) {
	expectParse(t, `
local x = 0
repeat
    x = x + 1
until x >= 10
return x
`)
}

func TestParseMultiReturn(t *testing.T) {
	expectParse(t, `
local function swap(a, b)
    return b, a
end
local x, y = swap(1, 2)
return x, y
`)
}

func TestParseCallChain(t *testing.T) {
	expectParse(t, `
local t = {}
t.a = {}
t.a.b = function() return 42 end
return t.a.b()
`)
}

func TestParseVarargForward(t *testing.T) {
	expectParse(t, `
local function f(...)
    local function g(...)
        return ...
    end
    return g(...)
end
`)
}

func TestParseBreakInNestedLoop(t *testing.T) {
	expectParse(t, `
for i = 1, 10 do
    for j = 1, 10 do
        if i == j then break end
    end
end
`)
}

func TestParseLocalFunction(t *testing.T) {
	// local function can reference itself (it's in scope during body)
	expectParse(t, `
local function fact(n)
    if n <= 1 then return 1 end
    return n * fact(n-1)
end
`)
}

// ===========================================================================
// Error cases
// ===========================================================================

func TestParseErrorMissingEnd(t *testing.T) {
	expectParseError(t, "if true then")
}

func TestParseErrorMissingThen(t *testing.T) {
	expectParseError(t, "if true end")
}

func TestParseErrorMissingDo(t *testing.T) {
	expectParseError(t, "while true end")
}

func TestParseErrorBreakOutsideLoop(t *testing.T) {
	expectParseError(t, "break")
}

func TestParseErrorUnexpectedSymbol(t *testing.T) {
	expectParseError(t, "return +")
}

func TestParseErrorUnclosedParen(t *testing.T) {
	expectParseError(t, "return (1")
}

func TestParseErrorBadFor(t *testing.T) {
	expectParseError(t, "for x do end")
}

func TestParseErrorUndefinedGoto(t *testing.T) {
	expectParseError(t, "goto nowhere")
}

func TestParseErrorDuplicateLabel(t *testing.T) {
	expectParseError(t, "::dup:: ::dup::")
}

func TestParseErrorVarargOutsideVararg(t *testing.T) {
	expectParseError(t, "local function f() return ... end")
}

// ===========================================================================
// Proto structure verification
// ===========================================================================

func TestProtoMainIsVararg(t *testing.T) {
	p := expectParse(t, "return 1")
	if !p.IsVararg() {
		t.Error("main proto should be vararg")
	}
}

func TestProtoHasENV(t *testing.T) {
	p := expectParse(t, "")
	if len(p.Upvalues) != 1 {
		t.Fatalf("expected 1 upvalue, got %d", len(p.Upvalues))
	}
	if p.Upvalues[0].Name == nil || p.Upvalues[0].Name.Data != "_ENV" {
		t.Error("upvalue[0] should be _ENV")
	}
	if !p.Upvalues[0].InStack {
		t.Error("_ENV should be InStack")
	}
	if p.Upvalues[0].Idx != 0 {
		t.Errorf("_ENV idx should be 0, got %d", p.Upvalues[0].Idx)
	}
}

func TestProtoMaxStackSize(t *testing.T) {
	p := expectParse(t, "local a, b, c, d, e = 1, 2, 3, 4, 5")
	if p.MaxStackSize < 5 {
		t.Errorf("MaxStackSize should be >= 5, got %d", p.MaxStackSize)
	}
}

func TestProtoNestedCount(t *testing.T) {
	p := expectParse(t, `
local function a() end
local function b() end
local function c() end
`)
	if len(p.Protos) != 3 {
		t.Errorf("expected 3 nested protos, got %d", len(p.Protos))
	}
}

func TestProtoReturnInstruction(t *testing.T) {
	p := expectParse(t, "")
	// Last instruction should be a RETURN variant
	if len(p.Code) == 0 {
		t.Fatal("expected at least one instruction")
	}
	lastOp := opcode.OpCode(p.Code[len(p.Code)-1] & 0x7F)
	if lastOp != opcode.OP_RETURN && lastOp != opcode.OP_RETURN0 && lastOp != opcode.OP_RETURN1 {
		t.Errorf("last instruction should be RETURN variant, got opcode %d", lastOp)
	}
}

// ---------------------------------------------------------------------------
// TestParseAllTestes — parse ALL Lua 5.5.1 testes files (34/34 expected)
// ---------------------------------------------------------------------------

func TestParseAllTestes(t *testing.T) {
	testesDir := filepath.Join("..", "..", "..", "lua-master", "testes")
	entries, err := os.ReadDir(testesDir)
	if err != nil {
		t.Skipf("testes directory not found: %v", err)
	}

	// main.lua and all.lua are interpreter harness files that start with
	// shebang (#!) lines. Shebang stripping is handled by the file loader
	// (DoFile), not the parser, so these files cannot be parsed standalone.
	skipFiles := map[string]bool{"main.lua": true, "all.lua": true}

	var passed, failed int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lua") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			if skipFiles[e.Name()] {
				t.Skipf("%s is an interpreter harness with shebang line — not a standalone parse target", e.Name())
				return
			}
			data, err := os.ReadFile(filepath.Join(testesDir, e.Name()))
			if err != nil {
				t.Fatalf("read error: %v", err)
			}

			defer func() {
				if r := recover(); r != nil {
					failed++
					t.Fatalf("PANIC: %v", r)
				}
			}()

			reader := newStringReader(string(data))
			p := Parse("@"+e.Name(), reader)
			if p == nil {
				failed++
				t.Fatalf("Parse returned nil")
			}
			passed++
			t.Logf("PASS (%d instructions)", len(p.Code))
		})
	}
	t.Logf("Results: %d passed, %d failed", passed, failed)
}
