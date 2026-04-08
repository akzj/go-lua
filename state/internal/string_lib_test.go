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

// Additional pattern matching tests from Lua test suite scenarios
func TestLuaPatternFindAdvanced(t *testing.T) {
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
		// Lazy quantifier
		{"lazy -", "aaab", "a-b", 0, false, 0, 4, true},
		{"lazy - minimal", "ab", "a-b", 0, false, 0, 2, true},
		// Optional ?
		{"optional ?", "ab", "a?b", 0, false, 0, 2, true},
		{"optional ? no match", "b", "a?b", 0, false, 0, 1, true},
		// Character classes
		{"word %w", "hello world", "%w+", 0, false, 0, 5, true},
		{"non-word %W", "hello world", "%W+", 0, false, 5, 6, true},
		{"lower %l", "Hello", "%l+", 0, false, 1, 5, true},
		{"upper %u", "Hello", "%u", 0, false, 0, 1, true},
		{"punct %p", "hello, world", "%p", 0, false, 5, 6, true},
		{"hex %x", "ff00", "%x+", 0, false, 0, 4, true},
		{"ctrl %c", "a\nb", "%c", 0, false, 1, 2, true},
		// Anchors
		{"anchor $", "hello", "lo$", 0, false, 3, 5, true},
		{"anchor $ fail", "hello!", "lo$", 0, false, 0, 0, false},
		// Escaped special chars
		{"escaped dot", "a.b", "a%.b", 0, false, 0, 3, true},
		{"escaped dot no match", "axb", "a%.b", 0, false, 0, 0, false},
		// Negated set
		{"negated set", "abc123", "[^%d]+", 0, false, 0, 3, true},
		// Init offset
		{"init offset", "abcabc", "abc", 3, false, 3, 6, true},
		// %g (printable non-space)
		{"printable %g", "  hello  ", "%g+", 0, false, 2, 7, true},
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

func TestLuaPatternMultiCapture(t *testing.T) {
	// Two captures
	s, e, caps, ncap, found := luaPatternFind("hello world", "(%w+)%s+(%w+)", 0, false)
	if !found {
		t.Fatal("expected match")
	}
	if s != 0 || e != 11 {
		t.Fatalf("expected (0,11), got (%d,%d)", s, e)
	}
	if ncap != 2 {
		t.Fatalf("expected 2 captures, got %d", ncap)
	}
	if caps[0].init != 0 || caps[0].len != 5 {
		t.Fatalf("cap1: expected (0,5), got (%d,%d)", caps[0].init, caps[0].len)
	}
	if caps[1].init != 6 || caps[1].len != 5 {
		t.Fatalf("cap2: expected (6,5), got (%d,%d)", caps[1].init, caps[1].len)
	}
}

func TestLuaPatternPositionCapture(t *testing.T) {
	_, _, caps, ncap, found := luaPatternFind("hello", "()()", 0, false)
	if !found {
		t.Fatal("expected match")
	}
	if ncap != 2 {
		t.Fatalf("expected 2 captures, got %d", ncap)
	}
	if caps[0].len != capPosition || caps[0].init != 0 {
		t.Fatalf("cap1: expected position capture at 0, got init=%d len=%d", caps[0].init, caps[0].len)
	}
}

func TestBstringGmatch(t *testing.T) {
	stack := makeStack(0,
		types.NewTValueString("hello world foo"),
		types.NewTValueString("%w+"),
	)
	nret := bstringGmatch(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return (iterator), got %d", nret)
	}
	// The result should be a function (goFuncWrapper)
	if !stack[0].IsFunction() {
		t.Fatalf("expected function, got tag %d", stack[0].GetTag())
	}

	// Call the iterator multiple times - use goFuncWrapper directly (internal package)
	gfw := stack[0].(*goFuncWrapper)
	iter := gfw.fn
	
	iterStack := make([]types.TValue, 5)
	for i := range iterStack {
		iterStack[i] = types.NewTValueNil()
	}

	// First call: "hello"
	n := iter(iterStack, 0)
	if n != 1 || iterStack[0].GetValue().(string) != "hello" {
		t.Fatalf("iter 1: expected 'hello', got %v (n=%d)", iterStack[0].GetValue(), n)
	}

	// Second call: "world"
	n = iter(iterStack, 0)
	if n != 1 || iterStack[0].GetValue().(string) != "world" {
		t.Fatalf("iter 2: expected 'world', got %v", iterStack[0].GetValue())
	}

	// Third call: "foo"
	n = iter(iterStack, 0)
	if n != 1 || iterStack[0].GetValue().(string) != "foo" {
		t.Fatalf("iter 3: expected 'foo', got %v", iterStack[0].GetValue())
	}

	// Fourth call: nil (exhausted)
	n = iter(iterStack, 0)
	if n != 1 || !iterStack[0].IsNil() {
		t.Fatalf("iter 4: expected nil, got %v", iterStack[0].GetValue())
	}
}

func TestBstringFormatUnsigned(t *testing.T) {
	// Lua's %x with negative should produce unsigned hex
	stack := makeStack(0,
		types.NewTValueString("%x"),
		types.NewTValueInteger(-1),
	)
	nret := bstringFormat(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "ffffffffffffffff" {
		t.Fatalf("expected 'ffffffffffffffff', got %q", got)
	}
}

func TestBstringGsubTable(t *testing.T) {
	// Create a table with key "hello" -> "world"
	tbl := createModuleTable()
	tbl.Set(types.NewTValueString("hello"), types.NewTValueString("world"))

	stack := makeStack(0,
		types.NewTValueString("hello"),
		types.NewTValueString("(%w+)"),
		&tableWrapper{tbl: tbl},
	)
	nret := bstringGsub(stack, 0)
	if nret != 2 {
		t.Fatalf("expected 2 returns, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "world" {
		t.Fatalf("expected 'world', got %q", got)
	}
}

func TestBstringFindInit(t *testing.T) {
	// string.find with init parameter
	stack := makeStack(0,
		types.NewTValueString("abcabc"),
		types.NewTValueString("abc"),
		types.NewTValueInteger(2),
	)
	nret := bstringFind(stack, 0)
	if nret != 2 {
		t.Fatalf("expected 2 returns, got %d", nret)
	}
	if stack[0].GetInteger() != 4 {
		t.Fatalf("expected start=4, got %d", stack[0].GetInteger())
	}
	if stack[1].GetInteger() != 6 {
		t.Fatalf("expected end=6, got %d", stack[1].GetInteger())
	}
}

func TestBstringMatchMultiCapture(t *testing.T) {
	stack := makeStackWithResults(0, 5,
		types.NewTValueString("2023-01-15"),
		types.NewTValueString("(%d+)-(%d+)-(%d+)"),
	)
	nret := bstringMatch(stack, 0)
	if nret != 3 {
		t.Fatalf("expected 3 returns, got %d", nret)
	}
	if stack[0].GetValue().(string) != "2023" {
		t.Fatalf("cap1: expected '2023', got %q", stack[0].GetValue())
	}
	if stack[1].GetValue().(string) != "01" {
		t.Fatalf("cap2: expected '01', got %q", stack[1].GetValue())
	}
	if stack[2].GetValue().(string) != "15" {
		t.Fatalf("cap3: expected '15', got %q", stack[2].GetValue())
	}
}

func TestBstringSubNegativeIndices(t *testing.T) {
	// string.sub("123456789", mini, -4) == "123456"
	stack := makeStack(0,
		types.NewTValueString("123456789"),
		types.NewTValueInteger(-1000000),
		types.NewTValueInteger(-4),
	)
	nret := bstringSub(stack, 0)
	if nret != 1 {
		t.Fatalf("expected 1 return, got %d", nret)
	}
	got := stack[0].GetValue().(string)
	if got != "123456" {
		t.Fatalf("expected '123456', got %q", got)
	}
}

func TestBstringFindNullByte(t *testing.T) {
	// Lua strings can contain null bytes
	stack := makeStack(0,
		types.NewTValueString("a\x00b"),
		types.NewTValueString("b"),
	)
	nret := bstringFind(stack, 0)
	if nret != 2 {
		t.Fatalf("expected 2 returns, got %d", nret)
	}
	if stack[0].GetInteger() != 3 {
		t.Fatalf("expected start=3, got %d", stack[0].GetInteger())
	}
}
