// Package parser implements the Lua syntax analyzer.
package parser

import (
	"testing"

	"github.com/akzj/go-lua/pkg/lexer"
)

// ============================================================================
// Helper Functions
// ============================================================================

func parseSource(t *testing.T, source string) *Parser {
	t.Helper()
	l := lexer.NewLexer([]byte(source), "test")
	p := NewParser(l)
	return p
}

func parseExpr(t *testing.T, source string) Expr {
	t.Helper()
	p := parseSource(t, source)
	expr := p.parseExpr()
	if expr == nil {
		if len(p.Errors) > 0 {
			t.Fatalf("Failed to parse expression: %v", p.Errors[0])
		}
		t.Fatal("Failed to parse expression")
	}
	return expr
}

func parseStmt(t *testing.T, source string) Stmt {
	t.Helper()
	p := parseSource(t, source)
	stmt := p.parseStmt()
	if stmt == nil {
		t.Fatal("Failed to parse statement")
	}
	return stmt
}

// ============================================================================
// Expression Parsing Tests
// ============================================================================

func TestParseNil(t *testing.T) {
	expr := parseExpr(t, "nil")
	if _, ok := expr.(*NilExpr); !ok {
		t.Errorf("Expected *NilExpr, got %T", expr)
	}
}

func TestParseTrue(t *testing.T) {
	expr := parseExpr(t, "true")
	if expr, ok := expr.(*BooleanExpr); !ok || !expr.Value {
		t.Errorf("Expected true boolean")
	}
}

func TestParseFalse(t *testing.T) {
	expr := parseExpr(t, "false")
	if expr, ok := expr.(*BooleanExpr); !ok || expr.Value {
		t.Errorf("Expected false boolean")
	}
}

func TestParseInteger(t *testing.T) {
	expr := parseExpr(t, "42")
	if expr, ok := expr.(*NumberExpr); !ok || !expr.IsInt || expr.Int != 42 {
		t.Errorf("Expected integer 42, got %v", expr)
	}
}

func TestParseFloat(t *testing.T) {
	expr := parseExpr(t, "3.14")
	if expr, ok := expr.(*NumberExpr); !ok || expr.IsInt || expr.Value != 3.14 {
		t.Errorf("Expected float 3.14, got %v", expr)
	}
}

func TestParseString(t *testing.T) {
	expr := parseExpr(t, `"hello"`)
	if expr, ok := expr.(*StringExpr); !ok || expr.Value != "hello" {
		t.Errorf("Expected string 'hello', got %v", expr)
	}
}

func TestParseVar(t *testing.T) {
	expr := parseExpr(t, "foo")
	if expr, ok := expr.(*VarExpr); !ok || expr.Name != "foo" {
		t.Errorf("Expected variable 'foo', got %v", expr)
	}
}

func TestParseFieldAccess(t *testing.T) {
	expr := parseExpr(t, "obj.field")
	if expr, ok := expr.(*FieldExpr); !ok || expr.Field != "field" {
		t.Errorf("Expected field access, got %v", expr)
	}
}

func TestParseIndexAccess(t *testing.T) {
	expr := parseExpr(t, "arr[1]")
	if expr, ok := expr.(*IndexExpr); !ok {
		t.Errorf("Expected index access, got %v", expr)
	}
}

func TestParseFunctionCall(t *testing.T) {
	expr := parseExpr(t, "foo(1, 2)")
	if expr, ok := expr.(*CallExpr); !ok || len(expr.Args) != 2 {
		t.Errorf("Expected function call with 2 args, got %v", expr)
	}
}

func TestParseMethodCall(t *testing.T) {
	expr := parseExpr(t, "obj:method(1)")
	if expr, ok := expr.(*MethodCallExpr); !ok || expr.Method != "method" {
		t.Errorf("Expected method call, got %v", expr)
	}
}

func TestParseUnaryMinus(t *testing.T) {
	expr := parseExpr(t, "-5")
	if expr, ok := expr.(*NumberExpr); !ok || expr.Value != -5 {
		t.Errorf("Expected -5, got %v", expr)
	}
}

func TestParseUnaryNot(t *testing.T) {
	expr := parseExpr(t, "not true")
	if expr, ok := expr.(*UnOpExpr); !ok || expr.Op != "not" {
		t.Errorf("Expected not expression, got %v", expr)
	}
}

func TestParseUnaryLen(t *testing.T) {
	expr := parseExpr(t, "#arr")
	if expr, ok := expr.(*UnOpExpr); !ok || expr.Op != "#" {
		t.Errorf("Expected len expression, got %v", expr)
	}
}

func TestParseBinaryArithmetic(t *testing.T) {
	expr := parseExpr(t, "1 + 2 * 3")
	if expr, ok := expr.(*BinOpExpr); !ok {
		t.Errorf("Expected binary expression, got %v", expr)
		return
	}
	binExpr := expr.(*BinOpExpr)
	// Check precedence: 2 * 3 should be grouped
	if binExpr.Op != "+" {
		t.Errorf("Expected + at top level, got %s", binExpr.Op)
	}
	if right, ok := binExpr.Right.(*BinOpExpr); !ok || right.Op != "*" {
		t.Errorf("Expected * on right side, got %v", binExpr.Right)
	}
}

func TestParseBinaryComparison(t *testing.T) {
	expr := parseExpr(t, "a == b")
	if expr, ok := expr.(*BinOpExpr); !ok || expr.Op != "==" {
		t.Errorf("Expected == expression, got %v", expr)
	}
}

func TestParseBinaryLogical(t *testing.T) {
	expr := parseExpr(t, "a and b or c")
	if expr, ok := expr.(*BinOpExpr); !ok || expr.Op != "or" {
		t.Errorf("Expected or at top level, got %v", expr)
	}
}

func TestParseConcat(t *testing.T) {
	expr := parseExpr(t, "a .. b .. c")
	if expr, ok := expr.(*BinOpExpr); !ok || expr.Op != ".." {
		t.Errorf("Expected concat expression, got %v", expr)
	}
}

func TestParsePower(t *testing.T) {
	expr := parseExpr(t, "2 ^ 3")
	if expr, ok := expr.(*BinOpExpr); !ok || expr.Op != "^" {
		t.Errorf("Expected power expression, got %v", expr)
	}
}

func TestParseParen(t *testing.T) {
	expr := parseExpr(t, "(1 + 2)")
	if expr, ok := expr.(*ParenExpr); !ok {
		t.Errorf("Expected parenthesized expression, got %v", expr)
	}
}

func TestParseTableConstructor(t *testing.T) {
	expr := parseExpr(t, "{1, 2, 3}")
	if expr, ok := expr.(*TableExpr); !ok || len(expr.Entries) != 3 {
		t.Errorf("Expected table with 3 entries, got %v", expr)
	}
}

func TestParseTableWithFields(t *testing.T) {
	expr := parseExpr(t, "{x = 1, y = 2}")
	if expr, ok := expr.(*TableExpr); !ok || len(expr.Entries) != 2 {
		t.Errorf("Expected table with 2 fields, got %v", expr)
	}
}

func TestParseTableWithIndex(t *testing.T) {
	expr := parseExpr(t, "{[1] = 'a', [2] = 'b'}")
	if expr, ok := expr.(*TableExpr); !ok || len(expr.Entries) != 2 {
		t.Errorf("Expected table with 2 indexed entries, got %v", expr)
	}
}

func TestParseEmptyTable(t *testing.T) {
	expr := parseExpr(t, "{}")
	if expr, ok := expr.(*TableExpr); !ok || len(expr.Entries) != 0 {
		t.Errorf("Expected empty table, got %v", expr)
	}
}

func TestParseAnonymousFunction(t *testing.T) {
	expr := parseExpr(t, "function(x) return x end")
	if expr, ok := expr.(*FuncExpr); !ok || len(expr.Params) != 1 {
		t.Errorf("Expected function with 1 param, got %v", expr)
	}
}

func TestParseVararg(t *testing.T) {
	expr := parseExpr(t, "...")
	if _, ok := expr.(*DotsExpr); !ok {
		t.Errorf("Expected vararg expression, got %v", expr)
	}
}

func TestParseChainedCall(t *testing.T) {
	expr := parseExpr(t, "foo.bar(1)(2)")
	// First call
	call1, ok := expr.(*CallExpr)
	if !ok {
		t.Errorf("Expected call expression, got %v", expr)
		return
	}
	// Function should be another call
	if _, ok := call1.Func.(*CallExpr); !ok {
		t.Errorf("Expected chained call, got %v", call1.Func)
	}
}

// ============================================================================
// Statement Parsing Tests
// ============================================================================

func TestParseAssign(t *testing.T) {
	stmt := parseStmt(t, "x = 1")
	if stmt, ok := stmt.(*AssignStmt); !ok || len(stmt.Left) != 1 || len(stmt.Right) != 1 {
		t.Errorf("Expected simple assignment, got %v", stmt)
	}
}

func TestParseMultiAssign(t *testing.T) {
	stmt := parseStmt(t, "x, y = 1, 2")
	if stmt, ok := stmt.(*AssignStmt); !ok || len(stmt.Left) != 2 || len(stmt.Right) != 2 {
		t.Errorf("Expected multi-assignment, got %v", stmt)
	}
}

func TestParseLocal(t *testing.T) {
	stmt := parseStmt(t, "local x = 1")
	if stmt, ok := stmt.(*LocalStmt); !ok || len(stmt.Names) != 1 {
		t.Errorf("Expected local declaration, got %v", stmt)
	}
}

func TestParseLocalMulti(t *testing.T) {
	stmt := parseStmt(t, "local x, y = 1, 2")
	if stmt, ok := stmt.(*LocalStmt); !ok || len(stmt.Names) != 2 {
		t.Errorf("Expected multi local declaration, got %v", stmt)
	}
}

func TestParseIf(t *testing.T) {
	stmt := parseStmt(t, "if x then return end")
	if stmt, ok := stmt.(*IfStmt); !ok {
		t.Errorf("Expected if statement, got %v", stmt)
	}
}

func TestParseIfElse(t *testing.T) {
	stmt := parseStmt(t, "if x then return 1 else return 2 end")
	if stmt, ok := stmt.(*IfStmt); !ok || stmt.Else == nil {
		t.Errorf("Expected if-else statement, got %v", stmt)
	}
}

func TestParseIfElseIf(t *testing.T) {
	stmt := parseStmt(t, "if x then return 1 elseif y then return 2 else return 3 end")
	if stmt, ok := stmt.(*IfStmt); !ok || len(stmt.ElseIf) != 1 {
		t.Errorf("Expected if-elseif-else statement, got %v", stmt)
	}
}

func TestParseWhile(t *testing.T) {
	stmt := parseStmt(t, "while x do x = x - 1 end")
	if stmt, ok := stmt.(*WhileStmt); !ok {
		t.Errorf("Expected while statement, got %v", stmt)
	}
}

func TestParseRepeat(t *testing.T) {
	stmt := parseStmt(t, "repeat x = x - 1 until x == 0")
	if stmt, ok := stmt.(*RepeatStmt); !ok {
		t.Errorf("Expected repeat statement, got %v", stmt)
	}
}

func TestParseForNumeric(t *testing.T) {
	stmt := parseStmt(t, "for i = 1, 10 do print(i) end")
	if stmt, ok := stmt.(*ForNumericStmt); !ok {
		t.Errorf("Expected numeric for statement, got %v", stmt)
	}
}

func TestParseForNumericWithStep(t *testing.T) {
	stmt := parseStmt(t, "for i = 1, 10, 2 do print(i) end")
	if stmt, ok := stmt.(*ForNumericStmt); !ok || stmt.Step == nil {
		t.Errorf("Expected numeric for with step, got %v", stmt)
	}
}

func TestParseForGeneric(t *testing.T) {
	stmt := parseStmt(t, "for k, v in pairs(t) do print(k, v) end")
	if stmt, ok := stmt.(*ForGenericStmt); !ok || len(stmt.Vars) != 2 {
		t.Errorf("Expected generic for statement, got %v", stmt)
	}
}

func TestParseBreak(t *testing.T) {
	stmt := parseStmt(t, "break")
	if _, ok := stmt.(*BreakStmt); !ok {
		t.Errorf("Expected break statement, got %v", stmt)
	}
}

func TestParseReturn(t *testing.T) {
	stmt := parseStmt(t, "return 1, 2")
	if stmt, ok := stmt.(*ReturnStmt); !ok || len(stmt.Values) != 2 {
		t.Errorf("Expected return with 2 values, got %v", stmt)
	}
}

func TestParseReturnEmpty(t *testing.T) {
	stmt := parseStmt(t, "return")
	if stmt, ok := stmt.(*ReturnStmt); !ok || len(stmt.Values) != 0 {
		t.Errorf("Expected empty return, got %v", stmt)
	}
}

func TestParseFunctionDef(t *testing.T) {
	stmt := parseStmt(t, "function foo(x) return x end")
	if stmt, ok := stmt.(*FuncDefStmt); !ok || stmt.Name == nil {
		t.Errorf("Expected function definition, got %v", stmt)
	}
}

func TestParseFunctionMethod(t *testing.T) {
	stmt := parseStmt(t, "function obj:method(x) return x end")
	if stmt, ok := stmt.(*FuncDefStmt); !ok {
		t.Errorf("Expected method definition, got %v", stmt)
	}
}

func TestParseGoto(t *testing.T) {
	stmt := parseStmt(t, "goto label")
	if stmt, ok := stmt.(*GotoStmt); !ok || stmt.Label != "label" {
		t.Errorf("Expected goto statement, got %v", stmt)
	}
}

func TestParseLabel(t *testing.T) {
	stmt := parseStmt(t, "::label::")
	if stmt, ok := stmt.(*LabelStmt); !ok || stmt.Name != "label" {
		t.Errorf("Expected label statement, got %v", stmt)
	}
}

func TestParseExprStmt(t *testing.T) {
	stmt := parseStmt(t, "print('hello')")
	if stmt, ok := stmt.(*ExprStmt); !ok {
		t.Errorf("Expected expression statement, got %v", stmt)
	}
}

func TestParseBlock(t *testing.T) {
	source := `
		local x = 1
		local y = 2
		return x + y
	`
	p := parseSource(t, source)
	block := p.parseChunk()
	if len(block.Stmts) != 3 {
		t.Errorf("Expected 3 statements, got %d", len(block.Stmts))
	}
}

// ============================================================================
// Error Handling Tests
// ============================================================================

func TestParseError(t *testing.T) {
	p := parseSource(t, "if x then")
	p.parseChunk()
	if len(p.Errors) == 0 {
		t.Error("Expected error for incomplete if statement")
	}
}

func TestParseErrorUnexpectedToken(t *testing.T) {
	p := parseSource(t, "@#$")
	p.parseChunk()
	if len(p.Errors) == 0 {
		t.Error("Expected error for unexpected token")
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestParseSimpleProgram(t *testing.T) {
	source := `
		local x = 10
		local y = 20
		function add(a, b)
			return a + b
		end
		print(add(x, y))
	`
	p := parseSource(t, source)
	proto, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if proto == nil {
		t.Error("Expected prototype")
	}
}

func TestParseComplexExpression(t *testing.T) {
	source := `
		local result = (a + b) * c - d / e ^ 2
	`
	p := parseSource(t, source)
	proto, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if proto == nil {
		t.Error("Expected prototype")
	}
}

func TestParseTableConstructorComplex(t *testing.T) {
	source := `
		local t = {
			x = 1,
			y = 2,
			[1] = "one",
			[2] = "two",
			"three",
			"four"
		}
	`
	p := parseSource(t, source)
	proto, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if proto == nil {
		t.Error("Expected prototype")
	}
}

func TestParseNestedFunctions(t *testing.T) {
	source := `
		function outer(x)
			function inner(y)
				return x + y
			end
			return inner
		end
	`
	p := parseSource(t, source)
	proto, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if proto == nil {
		t.Error("Expected prototype")
	}
}

func TestParseChunk(t *testing.T) {
	source := `
		-- Comment
		local x = 1;
		local y = 2
		return x + y
	`
	p := parseSource(t, source)
	block := p.parseChunk()
	if len(block.Stmts) < 2 {
		t.Errorf("Expected at least 2 statements, got %d", len(block.Stmts))
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestParseEmptyChunk(t *testing.T) {
	p := parseSource(t, "")
	block := p.parseChunk()
	if len(block.Stmts) != 0 {
		t.Errorf("Expected empty block, got %d statements", len(block.Stmts))
	}
}

func TestParseWhitespaceOnly(t *testing.T) {
	p := parseSource(t, "   \n\t  \n  ")
	block := p.parseChunk()
	if len(block.Stmts) != 0 {
		t.Errorf("Expected empty block for whitespace only, got %d statements", len(block.Stmts))
	}
}

func TestParseSemicolons(t *testing.T) {
	source := "x = 1; y = 2; return x + y"
	p := parseSource(t, source)
	block := p.parseChunk()
	if len(block.Stmts) != 3 {
		t.Errorf("Expected 3 statements, got %d", len(block.Stmts))
	}
}

func TestParseLongChain(t *testing.T) {
	source := "a.b.c.d.e.f.g"
	expr := parseExpr(t, source)
	// Should parse as nested field accesses
	if expr == nil {
		t.Error("Expected to parse long chain")
	}
}

func TestParseMultipleCalls(t *testing.T) {
	source := "f()()()"
	expr := parseExpr(t, source)
	// Should parse as nested calls
	if expr == nil {
		t.Error("Expected to parse multiple calls")
	}
}
