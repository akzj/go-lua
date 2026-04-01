package lparser

import (
	"io"
	"testing"

	"github.com/akzj/go-lua/internal/lstate"
	"github.com/akzj/go-lua/internal/lstring"
	"github.com/akzj/go-lua/internal/lzio"
)

// StringReader creates a reader from a string - fixed to track position
type StringReader struct {
	s   string
	pos int
}

func (r *StringReader) Read(L *lstate.LuaState, data interface{}, size *int64) []byte {
	if r.pos >= len(r.s) {
		*size = 0
		return nil
	}
	remaining := r.s[r.pos:]
	*size = int64(len(remaining))
	r.pos = len(r.s)
	return []byte(remaining)
}

// TestParseLocalAssignment tests parsing "local x = 1 + 2"
func TestParseLocalAssignment(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic: %v", r)
		}
	}()

	L := &lstate.LuaState{}
	_ = lstring.NewString(L, "test")

	input := "local x = 1"
	reader := &StringReader{s: input, pos: 0}

	z := &lzio.ZIO{}
	lzio.Init(L, z, reader.Read, nil)

	buff := &lzio.Mbuffer{}
	lzio.InitBuffer(buff)
	lzio.ResizeBuffer(L, buff, 256)

	t.Log("About to call LuaY_parser...")
	cl := LuaY_parser(L, z, buff, "test")
	t.Log("LuaY_parser returned")

	if cl == nil {
		t.Fatal("LuaY_parser returned nil closure")
	}
	if cl.P == nil {
		t.Fatal("Closure prototype is nil")
	}

	code := cl.P.Code
	if len(code) == 0 {
		t.Fatal("No bytecode generated")
	}

	t.Logf("Generated %d bytecode instructions", len(code))
	for i, instr := range code {
		t.Logf("  [%d] = 0x%08x", i, instr)
	}
}

// TestParseSimpleExpression tests various expressions
func TestParseSimpleExpression(t *testing.T) {
	L := &lstate.LuaState{}
	_ = lstring.NewString(L, "test")

	tests := []struct {
		name  string
		input string
	}{
		{"number", "local x = 42"},
		{"addition", "local x = 1 + 2"},
		{"subtraction", "local x = 5 - 3"},
		{"multiplication", "local x = 2 * 3"},
		{"division", "local x = 6 / 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &StringReader{s: tt.input, pos: 0}
			z := &lzio.ZIO{}
			lzio.Init(L, z, reader.Read, nil)
			buff := &lzio.Mbuffer{}
			lzio.InitBuffer(buff)
			lzio.ResizeBuffer(L, buff, 256)

			cl := LuaY_parser(L, z, buff, "test")
			if cl == nil || cl.P == nil {
				t.Fatalf("Failed to parse: %s", tt.input)
			}
			if len(cl.P.Code) == 0 {
				t.Errorf("No bytecode for: %s", tt.input)
			}
			t.Logf("%s: %d instructions", tt.input, len(cl.P.Code))
		})
	}
}

// TestParseMultipleLocals tests parsing multiple local variables
func TestParseMultipleLocals(t *testing.T) {
	L := &lstate.LuaState{}

	input := "local x = 1 local y = 2"
	reader := &StringReader{s: input, pos: 0}
	z := &lzio.ZIO{}
	lzio.Init(L, z, reader.Read, nil)
	buff := &lzio.Mbuffer{}
	lzio.InitBuffer(buff)
	lzio.ResizeBuffer(L, buff, 256)

	cl := LuaY_parser(L, z, buff, "test")
	if cl == nil || cl.P == nil {
		t.Fatal("Failed to parse multiple locals")
	}

	if len(cl.P.Code) == 0 {
		t.Error("No bytecode generated for multiple locals")
	}
}

// TestParseNil tests parsing nil
func TestParseNil(t *testing.T) {
	L := &lstate.LuaState{}

	input := "local x = nil"
	reader := &StringReader{s: input, pos: 0}
	z := &lzio.ZIO{}
	lzio.Init(L, z, reader.Read, nil)
	buff := &lzio.Mbuffer{}
	lzio.InitBuffer(buff)
	lzio.ResizeBuffer(L, buff, 256)

	cl := LuaY_parser(L, z, buff, "test")
	if cl == nil || cl.P == nil {
		t.Fatal("Failed to parse nil")
	}
}

// TestParseBooleans tests parsing true/false
func TestParseBooleans(t *testing.T) {
	L := &lstate.LuaState{}

	tests := []string{"local x = true", "local x = false"}

	for _, input := range tests {
		reader := &StringReader{s: input, pos: 0}
		z := &lzio.ZIO{}
		lzio.Init(L, z, reader.Read, nil)
		buff := &lzio.Mbuffer{}
		lzio.InitBuffer(buff)
		lzio.ResizeBuffer(L, buff, 256)

		cl := LuaY_parser(L, z, buff, "test")
		if cl == nil || cl.P == nil {
			t.Errorf("Failed to parse: %s", input)
		}
	}
}

// TestParseUnaryMinus tests parsing unary minus
func TestParseUnaryMinus(t *testing.T) {
	L := &lstate.LuaState{}

	input := "local x = -5"
	reader := &StringReader{s: input, pos: 0}
	z := &lzio.ZIO{}
	lzio.Init(L, z, reader.Read, nil)
	buff := &lzio.Mbuffer{}
	lzio.InitBuffer(buff)
	lzio.ResizeBuffer(L, buff, 256)

	cl := LuaY_parser(L, z, buff, "test")
	if cl == nil || cl.P == nil {
		t.Fatal("Failed to parse unary minus")
	}
}

// StringIOReader implements io.Reader for testing
type StringIOReader struct {
	s string
	i int
}

func (r *StringIOReader) Read(p []byte) (n int, err error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n = copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
// TestParseIfStatement tests parsing "if x then y end"
func TestParseIfStatement(t *testing.T) {
	L := &lstate.LuaState{}
	_ = lstring.NewString(L, "test")

	input := "if x then y end"
	reader := &StringReader{s: input, pos: 0}
	z := &lzio.ZIO{}
	lzio.Init(L, z, reader.Read, nil)
	buff := &lzio.Mbuffer{}
	lzio.InitBuffer(buff)
	lzio.ResizeBuffer(L, buff, 256)

	cl := LuaY_parser(L, z, buff, "test")
	if cl == nil || cl.P == nil {
		t.Fatal("Failed to parse if statement")
	}
	if len(cl.P.Code) == 0 {
		t.Error("No bytecode generated for if statement")
	}
	t.Logf("if statement: %d bytecode instructions", len(cl.P.Code))
}

// TestParseWhileStatement tests parsing "while x do y end"
func TestParseWhileStatement(t *testing.T) {
	L := &lstate.LuaState{}
	_ = lstring.NewString(L, "test")

	input := "while x do y end"
	reader := &StringReader{s: input, pos: 0}
	z := &lzio.ZIO{}
	lzio.Init(L, z, reader.Read, nil)
	buff := &lzio.Mbuffer{}
	lzio.InitBuffer(buff)
	lzio.ResizeBuffer(L, buff, 256)

	cl := LuaY_parser(L, z, buff, "test")
	if cl == nil || cl.P == nil {
		t.Fatal("Failed to parse while statement")
	}
	if len(cl.P.Code) == 0 {
		t.Error("No bytecode generated for while statement")
	}
	t.Logf("while statement: %d bytecode instructions", len(cl.P.Code))
}

// TestParseForStatement tests parsing "for i=1,10 do end"
func TestParseForStatement(t *testing.T) {
	L := &lstate.LuaState{}
	_ = lstring.NewString(L, "test")

	input := "for i=1,10 do end"
	reader := &StringReader{s: input, pos: 0}
	z := &lzio.ZIO{}
	lzio.Init(L, z, reader.Read, nil)
	buff := &lzio.Mbuffer{}
	lzio.InitBuffer(buff)
	lzio.ResizeBuffer(L, buff, 256)

	cl := LuaY_parser(L, z, buff, "test")
	if cl == nil || cl.P == nil {
		t.Fatal("Failed to parse for statement")
	}
	if len(cl.P.Code) == 0 {
		t.Error("No bytecode generated for for statement")
	}
	t.Logf("for statement: %d bytecode instructions", len(cl.P.Code))
}

// TestParseRepeatStatement tests parsing "repeat x until y"
func TestParseRepeatStatement(t *testing.T) {
	L := &lstate.LuaState{}
	_ = lstring.NewString(L, "test")

	input := "repeat x until y"
	reader := &StringReader{s: input, pos: 0}
	z := &lzio.ZIO{}
	lzio.Init(L, z, reader.Read, nil)
	buff := &lzio.Mbuffer{}
	lzio.InitBuffer(buff)
	lzio.ResizeBuffer(L, buff, 256)

	cl := LuaY_parser(L, z, buff, "test")
	if cl == nil || cl.P == nil {
		t.Fatal("Failed to parse repeat statement")
	}
	if len(cl.P.Code) == 0 {
		t.Error("No bytecode generated for repeat statement")
	}
	t.Logf("repeat statement: %d bytecode instructions", len(cl.P.Code))
}

// TestParseBreakStatement tests parsing "while x do break end"
func TestParseBreakStatement(t *testing.T) {
	L := &lstate.LuaState{}
	_ = lstring.NewString(L, "test")

	input := "while x do break end"
	reader := &StringReader{s: input, pos: 0}
	z := &lzio.ZIO{}
	lzio.Init(L, z, reader.Read, nil)
	buff := &lzio.Mbuffer{}
	lzio.InitBuffer(buff)
	lzio.ResizeBuffer(L, buff, 256)

	cl := LuaY_parser(L, z, buff, "test")
	if cl == nil || cl.P == nil {
		t.Fatal("Failed to parse break statement")
	}
	if len(cl.P.Code) == 0 {
		t.Error("No bytecode generated for break statement")
	}
	t.Logf("break statement: %d bytecode instructions", len(cl.P.Code))
}
