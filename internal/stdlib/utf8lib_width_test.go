package stdlib

import (
	"testing"

	luaapi "github.com/akzj/go-lua/internal/api"
)

func TestUTF8Width(t *testing.T) {
	L := luaapi.NewState()
	defer L.Close()
	OpenAll(L)

	tests := []struct {
		name   string
		code   string
		expect int64
	}{
		{"ascii", `return utf8.width("hello")`, 5},
		{"chinese", `return utf8.width("你好")`, 4},
		{"mixed", `return utf8.width("hi你好")`, 6},
		{"empty", `return utf8.width("")`, 0},
		{"emoji", `return utf8.width("🎉")`, 2},
		{"fullwidth", `return utf8.width("\xEF\xBC\xA1")`, 2}, // U+FF21 fullwidth A
		{"hangul", `return utf8.width("한글")`, 4},
		// Substring by byte positions (1-based)
		{"sub_range", `return utf8.width("hello", 2, 4)`, 3}, // "ell"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := L.DoString(tt.code); err != nil {
				t.Fatalf("DoString error: %v", err)
			}
			got, _ := L.ToInteger(-1)
			L.Pop(1)
			if got != tt.expect {
				t.Errorf("got %d, want %d", got, tt.expect)
			}
		})
	}
}

func TestUTF8Sub(t *testing.T) {
	L := luaapi.NewState()
	defer L.Close()
	OpenAll(L)

	tests := []struct {
		name   string
		code   string
		expect string
	}{
		{"ascii_full", `return utf8.sub("hello", 1, 5)`, "hello"},
		{"ascii_mid", `return utf8.sub("hello", 2, 4)`, "ell"},
		{"chinese_full", `return utf8.sub("你好世界", 1, 4)`, "你好世界"},
		{"chinese_mid", `return utf8.sub("你好世界", 2, 3)`, "好世"},
		{"negative_end", `return utf8.sub("hello", 1, -1)`, "hello"},
		{"negative_start", `return utf8.sub("hello", -3, -1)`, "llo"},
		{"single_char", `return utf8.sub("你好", 1, 1)`, "你"},
		{"out_of_range", `return utf8.sub("hi", 1, 10)`, "hi"},
		{"empty_range", `return utf8.sub("hello", 3, 2)`, ""},
		{"mixed", `return utf8.sub("a你b好c", 2, 4)`, "你b好"},
		{"emoji", `return utf8.sub("🎉hello", 1, 1)`, "🎉"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := L.DoString(tt.code); err != nil {
				t.Fatalf("DoString error: %v", err)
			}
			got, _ := L.ToString(-1)
			L.Pop(1)
			if got != tt.expect {
				t.Errorf("got %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestRuneDisplayWidth(t *testing.T) {
	tests := []struct {
		name  string
		r     rune
		width int
	}{
		{"ascii_a", 'a', 1},
		{"space", ' ', 1},
		{"chinese", '你', 2},
		{"hangul", '한', 2},
		{"fullwidth_A", 'Ａ', 2}, // U+FF21
		{"hiragana", 'あ', 2},
		{"latin", 'é', 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runeDisplayWidth(tt.r)
			if got != tt.width {
				t.Errorf("runeDisplayWidth(%q) = %d, want %d", tt.r, got, tt.width)
			}
		})
	}
}
