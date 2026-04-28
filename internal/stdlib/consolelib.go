package stdlib

// Console library — provides console.log, console.warn, console.error, console.debug
// with file:line prefix and shallow table inspection. Similar to JavaScript's console API.

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	luaapi "github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/object"
)

// consoleTimers stores named timers for console.time/time_end.
// NOTE: This is a global map shared across all Lua states. This is acceptable
// because Lua states are single-threaded, but if multiple states use timers
// with the same label concurrently, they may interfere. A sync.Map is used
// for safety.
var consoleTimers sync.Map // map[string]time.Time

// OpenConsole registers the "console" module.
// It is designed to be used as a preloaded module: require("console").
func OpenConsole(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"log":      consoleLog,
		"warn":     consoleWarn,
		"error":    consoleError,
		"debug":    consoleDebug,
		"time":     consoleTime,
		"time_end": consoleTimeEnd,
	})
	return 1
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func consoleGetWriter(L *luaapi.State) io.Writer {
	if L.Writer != nil {
		return L.Writer
	}
	return os.Stdout
}

// consoleGetCallerInfo returns "file:line" for the Lua caller.
// level 1 = direct caller of the C function (the Lua code that called console.log).
func consoleGetCallerInfo(L *luaapi.State) string {
	ar, ok := L.GetStack(1)
	if !ok {
		return "?:?"
	}
	L.GetInfo("Sl", ar)
	src := ar.ShortSrc
	if src == "" {
		src = "?"
	}
	line := ar.CurrentLine
	if line < 0 {
		return src + ":?"
	}
	return fmt.Sprintf("%s:%d", src, line)
}

// ---------------------------------------------------------------------------
// Value formatting
// ---------------------------------------------------------------------------

// consoleFormatArgs formats all Lua stack arguments as tab-separated strings.
func consoleFormatArgs(L *luaapi.State) string {
	n := L.GetTop()
	if n == 0 {
		return ""
	}
	parts := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		parts = append(parts, consoleFormatValue(L, i, 0))
	}
	return strings.Join(parts, "\t")
}

// consoleFormatValue formats a single Lua value with shallow table expansion.
func consoleFormatValue(L *luaapi.State, idx int, depth int) string {
	tp := L.Type(idx)
	switch tp {
	case object.TypeTable:
		if depth > 0 {
			return "{...}" // prevent infinite recursion
		}
		return consoleFormatTable(L, idx)
	case object.TypeString:
		s, _ := L.ToString(idx)
		return s
	case object.TypeNumber:
		if L.IsInteger(idx) {
			v, _ := L.ToInteger(idx)
			return fmt.Sprintf("%d", v)
		}
		v, _ := L.ToNumber(idx)
		return fmt.Sprintf("%g", v)
	case object.TypeBoolean:
		if L.ToBoolean(idx) {
			return "true"
		}
		return "false"
	case object.TypeNil:
		return "nil"
	default:
		// function, userdata, thread — use TolString (pushes to stack)
		s := L.TolString(idx)
		L.Pop(1) // pop the string pushed by TolString
		return s
	}
}

// consoleFormatTable formats a table with shallow key=value expansion.
func consoleFormatTable(L *luaapi.State, idx int) string {
	// Normalize to absolute index so it stays valid as we push/pop
	if idx < 0 {
		idx = L.GetTop() + idx + 1
	}

	var parts []string
	const maxItems = 20
	count := 0

	L.PushNil() // first key for iteration
	for L.Next(idx) {
		if count >= maxItems {
			parts = append(parts, "...")
			L.Pop(2) // pop key + value, stop iteration
			break
		}

		// Stack: ... key(top-1) value(top)
		keyStr := consoleFormatKey(L, -2)
		valStr := consoleFormatValue(L, -1, 1) // depth=1 prevents recursion

		parts = append(parts, keyStr+" = "+valStr)

		L.Pop(1) // pop value, keep key for next iteration
		count++
	}

	return "{" + strings.Join(parts, ", ") + "}"
}

// consoleFormatKey formats a table key for display.
func consoleFormatKey(L *luaapi.State, idx int) string {
	tp := L.Type(idx)
	switch tp {
	case object.TypeString:
		s, _ := L.ToString(idx)
		return s
	case object.TypeNumber:
		if L.IsInteger(idx) {
			v, _ := L.ToInteger(idx)
			return fmt.Sprintf("[%d]", v)
		}
		v, _ := L.ToNumber(idx)
		return fmt.Sprintf("[%g]", v)
	default:
		return "[?]"
	}
}

// ---------------------------------------------------------------------------
// Console functions
// ---------------------------------------------------------------------------

func consolePrint(L *luaapi.State, prefix string) int {
	w := consoleGetWriter(L)
	loc := consoleGetCallerInfo(L)
	args := consoleFormatArgs(L)

	if prefix == "" {
		fmt.Fprintf(w, "[%s] %s\n", loc, args)
	} else {
		fmt.Fprintf(w, "[%s %s] %s\n", prefix, loc, args)
	}
	return 0
}

func consoleLog(L *luaapi.State) int   { return consolePrint(L, "") }
func consoleWarn(L *luaapi.State) int  { return consolePrint(L, "WARN") }
func consoleError(L *luaapi.State) int { return consolePrint(L, "ERROR") }
func consoleDebug(L *luaapi.State) int { return consolePrint(L, "DEBUG") }

// console.time([label]) — starts a named timer.
func consoleTime(L *luaapi.State) int {
	label := "default"
	if L.GetTop() >= 1 {
		s, _ := L.ToString(1)
		if s != "" {
			label = s
		}
	}
	consoleTimers.Store(label, time.Now())
	return 0
}

// console.time_end([label]) — stops a named timer and prints elapsed time.
func consoleTimeEnd(L *luaapi.State) int {
	label := "default"
	if L.GetTop() >= 1 {
		s, _ := L.ToString(1)
		if s != "" {
			label = s
		}
	}
	w := consoleGetWriter(L)
	loc := consoleGetCallerInfo(L)

	if startVal, ok := consoleTimers.LoadAndDelete(label); ok {
		start := startVal.(time.Time)
		elapsed := time.Since(start)
		fmt.Fprintf(w, "[%s] %s: %s\n", loc, label, elapsed.Round(time.Microsecond))
	} else {
		fmt.Fprintf(w, "[%s] %s: timer not found\n", loc, label)
	}
	return 0
}
