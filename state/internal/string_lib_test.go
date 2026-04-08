package internal

import (
	"testing"

	types "github.com/akzj/go-lua/types/api"
)

// Direct unit tests for string library functions.
// These bypass the VM entirely and test the Go functions directly.

func makeStack(base int, args ...types.TValue) []types.TValue {
	// Create a stack sized exactly: base+1 (func slot) + len(args)
	// The VM sizes the stack exactly, so nArgs = len(stack) - base - 1
	size := base + 1 + len(args)
	stack := make([]types.TValue, size)
	for i := range stack {
		stack[i] = types.NewTValueNil()
	}
	// Place args starting at base+1
	for i, arg := range args {
		stack[base+1+i] = arg
	}
	return stack
}

// makeStackWithResults creates a stack with extra space for return values
func makeStackWithResults(base int, nResults int, args ...types.TValue) []types.TValue {
	size := base + 1 + len(args)
	if nResults > len(args)+1 {
		size = base + nResults
	}
	stack := make([]types.TValue, size)
	for i := range stack {
		stack[i] = types.NewTValueNil()
	}
	for i, arg := range args {
		stack[base+1+i] = arg
	}
	return stack
}

func TestBstringLen(t *testing.T) {
	stack := makeStack(0, types.NewTValueString("hello"))
	nret := bstringLen(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	if !stack[0].IsInteger() {
		t.Fatalf("expected integer result, got tag %d", stack[0].GetTag())
	}
	if stack[0].GetInteger() != 5 {
		t.Fatalf("expected 5, got %d", stack[0].GetInteger())
	}
}

func TestBstringLenEmpty(t *testing.T) {
	stack := makeStack(0, types.NewTValueString(""))
	nret := bstringLen(stack, 0)
	if nret != 1 || stack[0].GetInteger() != 0 {
		t.Fatalf("expected len 0, got %d (nret=%d)", stack[0].GetInteger(), nret)
	}
}

func TestBstringSub(t *testing.T) {
	tests := []struct {
		name string
		s    string
		i, j int64
		want string
	}{
		{"basic", "hello", 2, 4, "ell"},
		{"full", "hello", 1, 5, "hello"},
		{"negative", "hello", -3, -1, "llo"},
		{"empty", "hello", 3, 2, ""},
		{"default j", "hello", 2, -1, "ello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := makeStack(0,
				types.NewTValueString(tt.s),
				types.NewTValueInteger(types.LuaInteger(tt.i)),
				types.NewTValueInteger(types.LuaInteger(tt.j)),
			)
			nret := bstringSub(stack, 0)
			if nret != 1 {
				t.Fatalf("expected 1 return, got %d", nret)
			}
			got, ok := stack[0].GetValue().(string)
			if !ok {
				t.Fatalf("expected string result")
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestBstringRep(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("ab"),
		types.NewTValueInteger(3),
	)
	nret := bstringRep(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "ababab" {
		t.Fatalf("expected 'ababab', got %q", got)
	}
}

func TestBstringRepSep(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("ab"),
		types.NewTValueInteger(3),
		types.NewTValueString(","),
	)
	nret := bstringRep(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "ab,ab,ab" {
		t.Fatalf("expected 'ab,ab,ab', got %q", got)
	}
}

func TestBstringUpper(t *testing.T) {
	stack := makeStack(0, types.NewTValueString("hello"))
	bstringUpper(stack, 0)
	got := stack[0].GetValue().(string)
	if got != "HELLO" {
		t.Fatalf("expected 'HELLO', got %q", got)
	}
}

func TestBstringLower(t *testing.T) {
	stack := makeStack(0, types.NewTValueString("HELLO"))
	bstringLower(stack, 0)
	got := stack[0].GetValue().(string)
	if got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}
}

func TestBstringReverse(t *testing.T) {
	stack := makeStack(0, types.NewTValueString("hello"))
	bstringReverse(stack, 0)
	got := stack[0].GetValue().(string)
	if got != "olleh" {
		t.Fatalf("expected 'olleh', got %q", got)
	}
}

func TestBstringByte(t *testing.T) {
	stack := makeStack(0, types.NewTValueString("ABC"))
	nret := bstringByte(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	if stack[0].GetInteger() != 65 {
		t.Fatalf("expected 65, got %d", stack[0].GetInteger())
	}
}

func TestBstringByteRange(t *testing.T) {
	stack := makeStackWithResults(0, 5,
		types.NewTValueString("ABC"),
		types.NewTValueInteger(1),
		types.NewTValueInteger(3),
	)
	nret := bstringByte(stack, 0)
	if nret != 3 {
		t.Fatalf("expected 3 returns, got %d", nret)
	}
	if stack[0].GetInteger() != 65 || stack[1].GetInteger() != 66 || stack[2].GetInteger() != 67 {
		t.Fatalf("expected 65,66,67, got %d,%d,%d",
			stack[0].GetInteger(), stack[1].GetInteger(), stack[2].GetInteger())
	}
}

func TestBstringChar(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueInteger(65),
		types.NewTValueInteger(66),
		types.NewTValueInteger(67),
	)
	nret := bstringChar(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "ABC" {
		t.Fatalf("expected 'ABC', got %q", got)
	}
}

func TestBstringFindPlain(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("hello world"),
		types.NewTValueString("world"),
		types.NewTValueInteger(1),
		types.NewTValueBoolean(true),
	)
	nret := bstringFind(stack, 0)
	if nret != 2 {
		t.Fatalf("expected 2 returns, got %d", nret)
	}
	if stack[0].GetInteger() != 7 {
		t.Fatalf("expected start=7, got %d", stack[0].GetInteger())
	}
	if stack[1].GetInteger() != 11 {
		t.Fatalf("expected end=11, got %d", stack[1].GetInteger())
	}
}

func TestBstringFindPattern(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("hello123world"),
		types.NewTValueString("%d+"),
	)
	nret := bstringFind(stack, 0)
	if nret != 2 {
		t.Fatalf("expected 2 returns, got %d", nret)
	}
	if stack[0].GetInteger() != 6 {
		t.Fatalf("expected start=6, got %d", stack[0].GetInteger())
	}
	if stack[1].GetInteger() != 8 {
		t.Fatalf("expected end=8, got %d", stack[1].GetInteger())
	}
}

func TestBstringFindCapture(t *testing.T) {
	stack := makeStackWithResults(0, 5,
		types.NewTValueString("hello123world"),
		types.NewTValueString("(%d+)"),
	)
	nret := bstringFind(stack, 0)
	if nret != 3 {
		t.Fatalf("expected 3 returns, got %d", nret)
	}
	if stack[0].GetInteger() != 6 {
		t.Fatalf("expected start=6, got %d", stack[0].GetInteger())
	}
	if stack[1].GetInteger() != 8 {
		t.Fatalf("expected end=8, got %d", stack[1].GetInteger())
	}
	cap := stack[2].GetValue().(string)
	if cap != "123" {
		t.Fatalf("expected capture '123', got %q", cap)
	}
}

func TestBstringFindEmpty(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString(""),
		types.NewTValueString(""),
	)
	nret := bstringFind(stack, 0)
	if nret != 2 {
		t.Fatalf("expected 2 returns, got %d", nret)
	}
	if stack[0].GetInteger() != 1 {
		t.Fatalf("expected start=1, got %d", stack[0].GetInteger())
	}
	if stack[1].GetInteger() != 0 {
		t.Fatalf("expected end=0, got %d", stack[1].GetInteger())
	}
}

func TestBstringFindNotFound(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("hello"),
		types.NewTValueString("xyz"),
	)
	nret := bstringFind(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return (nil), got %d", nret)
	}
	if !stack[0].IsNil() {
		t.Fatalf("expected nil, got tag %d", stack[0].GetTag())
	}
}

func TestBstringMatch(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("hello123"),
		types.NewTValueString("(%d+)"),
	)
	nret := bstringMatch(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "123" {
		t.Fatalf("expected '123', got %q", got)
	}
}

func TestBstringMatchNoCapture(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("hello"),
		types.NewTValueString("hel"),
	)
	nret := bstringMatch(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "hel" {
		t.Fatalf("expected 'hel', got %q", got)
	}
}

func TestBstringFormat(t *testing.T) {
	tests := []struct {
		name string
		fmt_ string
		args []types.TValue
		want string
	}{
		{"int", "%d", []types.TValue{types.NewTValueInteger(42)}, "42"},
		{"string", "%s", []types.TValue{types.NewTValueString("hello")}, "hello"},
		{"padded int", "%05d", []types.TValue{types.NewTValueInteger(42)}, "00042"},
		{"float", "%.2f", []types.TValue{types.NewTValueFloat(3.14159)}, "3.14"},
		{"hex", "%x", []types.TValue{types.NewTValueInteger(255)}, "ff"},
		{"HEX", "%X", []types.TValue{types.NewTValueInteger(255)}, "FF"},
		{"octal", "%o", []types.TValue{types.NewTValueInteger(8)}, "10"},
		{"percent", "%%", nil, "%"},
		{"mixed", "%d + %d = %d", []types.TValue{
			types.NewTValueInteger(1),
			types.NewTValueInteger(2),
			types.NewTValueInteger(3),
		}, "1 + 2 = 3"},
		{"g", "%g", []types.TValue{types.NewTValueFloat(3.14)}, "3.14"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allArgs := []types.TValue{types.NewTValueString(tt.fmt_)}
			allArgs = append(allArgs, tt.args...)
			stack := makeStack(0, allArgs...)
			nret := bstringFormat(stack, 0)
			if nret != 1 {
				t.Fatalf("expected 1 return, got %d", nret)
			}
			got := stack[0].GetValue().(string)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestBstringGsub(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("hello world"),
		types.NewTValueString("(%w+)"),
		types.NewTValueString("%1-%1"),
	)
	nret := bstringGsub(stack, 0)
	if nret != 2 {
		t.Fatalf("expected 2 returns, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "hello-hello world-world" {
		t.Fatalf("expected 'hello-hello world-world', got %q", got)
	}
	count := stack[1].GetInteger()
	if count != 2 {
		t.Fatalf("expected count=2, got %d", count)
	}
}

func TestBstringGsubLimit(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("aaa"),
		types.NewTValueString("a"),
		types.NewTValueString("b"),
		types.NewTValueInteger(2),
	)
	nret := bstringGsub(stack, 0)
	if nret != 2 {
		t.Fatalf("expected 2 returns, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "bba" {
		t.Fatalf("expected 'bba', got %q", got)
	}
}

func TestBstringFindAnchor(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("hello"),
		types.NewTValueString("^hel"),
	)
	nret := bstringFind(stack, 0)
	if nret != 2 {
		t.Fatalf("expected 2 returns, got %d", nret)
	}
	if stack[0].GetInteger() != 1 {
		t.Fatalf("expected start=1, got %d", stack[0].GetInteger())
	}
	if stack[1].GetInteger() != 3 {
		t.Fatalf("expected end=3, got %d", stack[1].GetInteger())
	}
}

func TestBstringFindAnchorFail(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("hello"),
		types.NewTValueString("^ell"),
	)
	nret := bstringFind(stack, 0)
	if nret != 1 || !stack[0].IsNil() {
		t.Fatalf("expected nil result for failed anchor match")
	}
}

// Test pattern matching engine directly
func TestLuaPatternFind(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		pat    string
		init   int
		anchor bool
		wantS  int
		wantE  int
		found  bool
	}{
		{"simple", "hello", "ell", 0, false, 1, 4, true},
		{"digit+", "abc123def", "%d+", 0, false, 3, 6, true},
		{"alpha+", "123abc", "%a+", 0, false, 3, 6, true},
		{"space", "hello world", "%s", 0, false, 5, 6, true},
		{"not found", "hello", "xyz", 0, false, 0, 0, false},
		{"empty", "", "", 0, false, 0, 0, true},
		{"empty in string", "hello", "", 0, false, 0, 0, true},
		{"dot star", "hello", ".*", 0, false, 0, 5, true},
		{"dot plus", "hello", ".+", 0, false, 0, 5, true},
		{"greedy", "aaab", "a*", 0, false, 0, 3, true},
		{"class set", "hello", "[helo]+", 0, false, 0, 5, true},
		{"class neg", "hello", "[^helo]+", 0, false, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, e, _, _, found := luaPatternFind(tt.src, tt.pat, tt.init, tt.anchor)
			if found != tt.found {
				t.Fatalf("found=%v, want %v", found, tt.found)
			}
			if found {
				if s != tt.wantS || e != tt.wantE {
					t.Fatalf("got (%d,%d), want (%d,%d)", s, e, tt.wantS, tt.wantE)
				}
			}
		})
	}
}

func TestLuaPatternCaptures(t *testing.T) {
	s, e, caps, ncap, found := luaPatternFind("hello123world", "(%d+)", 0, false)
	if !found {
		t.Fatal("expected match")
	}
	if s != 5 || e != 8 {
		t.Fatalf("expected (5,8), got (%d,%d)", s, e)
	}
	if ncap != 1 {
		t.Fatalf("expected 1 capture, got %d", ncap)
	}
	if caps[0].init != 5 || caps[0].len != 3 {
		t.Fatalf("expected capture (5,3), got (%d,%d)", caps[0].init, caps[0].len)
	}
}

func TestBstringFormatQuote(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("%q"),
		types.NewTValueString(`hello "world"`),
	)
	nret := bstringFormat(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	want := `"hello \"world\""`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestBstringPacksize(t *testing.T) {
	stack := makeStack(0, types.NewTValueString("bhi4"))
	nret := bstringPacksize(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	// b=1, h=2, i4=4 = 7
	if stack[0].GetInteger() != 7 {
		t.Fatalf("expected 7, got %d", stack[0].GetInteger())
	}
}
