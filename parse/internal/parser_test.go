package internal

import (
	"testing"

	astapi "github.com/akzj/go-lua/ast/api"
)

func TestParserBasic(t *testing.T) {
	p := NewParser()

	// Test empty chunk
	chunk, err := p.Parse("")
	if err != nil {
		t.Fatalf("Parse empty string failed: %v", err)
	}
	if chunk == nil {
		t.Fatal("Parse returned nil chunk")
	}
	if chunk.Block() == nil {
		t.Fatal("Parse returned nil block")
	}
}

func TestParserSemicolon(t *testing.T) {
	p := NewParser()

	// Test semicolon statement
	_, err := p.Parse(";")
	if err != nil {
		t.Fatalf("Parse semicolon failed: %v", err)
	}
}

func TestParserMultipleSemicolons(t *testing.T) {
	p := NewParser()

	// Multiple semicolons
	_, err := p.Parse(";;")
	if err != nil {
		t.Fatalf("Parse multiple semicolons failed: %v", err)
	}
}

func TestChunkSourceName(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if chunk.SourceName() == "" {
		t.Fatal("Expected non-empty source name")
	}
}

func TestParserAssignment(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("x = 1")
	if err != nil {
		t.Fatalf("Parse assignment failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserLocalVar(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("local x = 1")
	if err != nil {
		t.Fatalf("Parse local var failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserIf(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("if true then end")
	if err != nil {
		t.Fatalf("Parse if failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserWhile(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("while true do end")
	if err != nil {
		t.Fatalf("Parse while failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserDo(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("do end")
	if err != nil {
		t.Fatalf("Parse do failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserRepeat(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("repeat until true")
	if err != nil {
		t.Fatalf("Parse repeat failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserForNumeric(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("for i = 1, 10 do end")
	if err != nil {
		t.Fatalf("Parse for numeric failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserForGeneric(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("for k, v in pairs({}) do end")
	if err != nil {
		t.Fatalf("Parse for generic failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserFunction(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("function f() end")
	if err != nil {
		t.Fatalf("Parse function failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserLocalFunction(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("local function f() end")
	if err != nil {
		t.Fatalf("Parse local function failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserReturn(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("return 1, 2, 3")
	if err != nil {
		t.Fatalf("Parse return failed: %v", err)
	}
	if len(chunk.Block().ReturnExp()) != 3 {
		t.Fatalf("Expected 3 return values, got %d", len(chunk.Block().ReturnExp()))
	}
}

func TestParserExpression(t *testing.T) {
	p := NewParser()

	exprs := []string{
		"1 + 2",
		"a and b",
		"not true",
		"{}",
		"(1 + 2) * 3",
		"a[1]",
		"a.b",
	}

	for _, expr := range exprs {
		_, err := p.ParseExpression(expr)
		if err != nil {
			t.Errorf("ParseExpression(%q) failed: %v", expr, err)
		}
	}
}

func TestParserIfElseIfElse(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("if a then b elseif c then d else e end")
	if err != nil {
		t.Fatalf("Parse if elseif else failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(chunk.Block().Stats()))
	}
}

func TestParserMultipleStatements(t *testing.T) {
	p := NewParser()

	chunk, err := p.Parse("x = 1; y = 2; z = 3")
	if err != nil {
		t.Fatalf("Parse multiple statements failed: %v", err)
	}
	if len(chunk.Block().Stats()) != 3 {
		t.Fatalf("Expected 3 statements, got %d", len(chunk.Block().Stats()))
	}
}



// Verify interface satisfaction
var _ astapi.Chunk = (*chunkImpl)(nil)
var _ astapi.Block = (*blockImpl)(nil)
var _ astapi.StatNode = (*emptyStat)(nil)
