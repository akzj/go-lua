package stdlib

// Log library — provides log.info, log.warn, log.error, log.debug
// with file:line prefix and shallow table inspection.

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

// logTimers stores named timers for log.time/time_end.
// NOTE: This is a global map shared across all Lua states. This is acceptable
// because Lua states are single-threaded, but if multiple states use timers
// with the same label concurrently, they may interfere. A sync.Map is used
// for safety.
var logTimers sync.Map // map[string]time.Time

// OpenLog registers the "log" module.
// It is designed to be used as a preloaded module: require("log").
func OpenLog(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"info":      logInfo,
		"warn":      logWarn,
		"error":     logError,
		"debug":     logDebug,
		"time":      logTime,
		"time_end":  logTimeEnd,
		"set_depth": logSetDepth,
	})

	// Set default max depth (1) in registry.
	L.PushInteger(1)
	L.SetField(luaapi.RegistryIndex, "_log_max_depth")
	return 1
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func logGetWriter(L *luaapi.State) io.Writer {
	if L.Writer != nil {
		return L.Writer
	}
	return os.Stdout
}

// logGetCallerInfo returns "file:line" for the Lua caller.
// level 1 = direct caller of the C function (the Lua code that called log.info).
func logGetCallerInfo(L *luaapi.State) string {
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

// getLogMaxDepth reads the configured max table depth from the registry.
// Returns 1 (default) if not set.
func getLogMaxDepth(L *luaapi.State) int {
	tp := L.GetField(luaapi.RegistryIndex, "_log_max_depth")
	if tp == object.TypeNumber {
		n, _ := L.ToInteger(-1)
		L.Pop(1)
		return int(n)
	}
	L.Pop(1)
	return 1 // default
}

// logFormatArgs formats all Lua stack arguments as tab-separated strings.
func logFormatArgs(L *luaapi.State) string {
	maxDepth := getLogMaxDepth(L)
	n := L.GetTop()
	if n == 0 {
		return ""
	}
	parts := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		parts = append(parts, logFormatValue(L, i, 0, maxDepth))
	}
	return strings.Join(parts, "\t")
}

// logFormatValue formats a single Lua value with configurable table expansion depth.
func logFormatValue(L *luaapi.State, idx int, depth int, maxDepth int) string {
	tp := L.Type(idx)
	switch tp {
	case object.TypeTable:
		if depth >= maxDepth {
			return "{...}" // depth limit reached
		}
		return logFormatTable(L, idx, depth, maxDepth)
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

// logFormatTable formats a table with key=value expansion up to maxDepth.
func logFormatTable(L *luaapi.State, idx int, depth int, maxDepth int) string {
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
		keyStr := logFormatKey(L, -2)
		valStr := logFormatValue(L, -1, depth+1, maxDepth)

		parts = append(parts, keyStr+" = "+valStr)

		L.Pop(1) // pop value, keep key for next iteration
		count++
	}

	return "{" + strings.Join(parts, ", ") + "}"
}

// logFormatKey formats a table key for display.
func logFormatKey(L *luaapi.State, idx int) string {
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
// Log functions
// ---------------------------------------------------------------------------

func logPrint(L *luaapi.State, prefix string) int {
	w := logGetWriter(L)
	loc := logGetCallerInfo(L)
	args := logFormatArgs(L)

	if prefix == "" {
		fmt.Fprintf(w, "[%s] %s\n", loc, args)
	} else {
		fmt.Fprintf(w, "[%s %s] %s\n", prefix, loc, args)
	}
	return 0
}

func logInfo(L *luaapi.State) int  { return logPrint(L, "") }
func logWarn(L *luaapi.State) int  { return logPrint(L, "WARN") }
func logError(L *luaapi.State) int { return logPrint(L, "ERROR") }
func logDebug(L *luaapi.State) int { return logPrint(L, "DEBUG") }

// log.set_depth(n) — sets the maximum table recursion depth for formatting.
// n=0 means all tables show as {...}. n=1 (default) expands one level. etc.
func logSetDepth(L *luaapi.State) int {
	n := int(L.CheckInteger(1))
	if n < 0 {
		n = 0
	}
	L.PushInteger(int64(n))
	L.SetField(luaapi.RegistryIndex, "_log_max_depth")
	return 0
}

// log.time([label]) — starts a named timer.
func logTime(L *luaapi.State) int {
	label := "default"
	if L.GetTop() >= 1 {
		s, _ := L.ToString(1)
		if s != "" {
			label = s
		}
	}
	logTimers.Store(label, time.Now())
	return 0
}

// log.time_end([label]) — stops a named timer and prints elapsed time.
func logTimeEnd(L *luaapi.State) int {
	label := "default"
	if L.GetTop() >= 1 {
		s, _ := L.ToString(1)
		if s != "" {
			label = s
		}
	}
	w := logGetWriter(L)
	loc := logGetCallerInfo(L)

	if startVal, ok := logTimers.LoadAndDelete(label); ok {
		start := startVal.(time.Time)
		elapsed := time.Since(start)
		fmt.Fprintf(w, "[%s] %s: %s\n", loc, label, elapsed.Round(time.Microsecond))
	} else {
		fmt.Fprintf(w, "[%s] %s: timer not found\n", loc, label)
	}
	return 0
}
