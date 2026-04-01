// Package internal implements the Lua OS library.
// This package provides implementations for:
//   - os.clock(): CPU time in seconds
//   - os.date([format [, time]]): current date/time
//   - os.difftime(t1, t2): time difference in seconds
//   - os.execute([command]): execute shell command
//   - os.exit([code [, close]]): exit the program
//   - os.getenv(name): get environment variable
//   - os.remove(filename): delete file
//   - os.rename(old, new): rename file
//   - os.setlocale(locale [, category]): set locale
//   - os.time([table]): get timestamp
//   - os.tmpname(): get temporary file name
//
// Reference: lua-master/loslib.c
package internal

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	oslib "github.com/akzj/go-lua/lib/os/api"
)

// OSLib is the implementation of the Lua OS library.
type OSLib struct{}

// NewOSLib creates a new OSLib instance.
func NewOSLib() oslib.OSLib {
	return &OSLib{}
}

// Open implements oslib.OSLib.Open.
// Registers all OS library functions in the global table under "os".
func (o *OSLib) Open(L oslib.LuaAPI) int {
	// Create "os" table: 0 array elements, 11 predefined fields
	L.CreateTable(0, 11)

	// Register all OS functions using PushGoFunction + SetField
	register := func(name string, fn oslib.LuaFunc) {
		L.PushGoFunction(fn)
		L.SetField(-2, name)
	}

	register("clock", osClock)
	register("date", osDate)
	register("difftime", osDifftime)
	register("execute", osExecute)
	register("exit", osExit)
	register("getenv", osGetenv)
	register("remove", osRemove)
	register("rename", osRename)
	register("setlocale", osSetlocale)
	register("time", osTime)
	register("tmpname", osTmpname)

	// luaopen_os convention: return 1 (the module table stays on stack)
	return 1
}

// Ensure OSLib implements OSLib interface
var _ oslib.OSLib = (*OSLib)(nil)

// Ensure types implement LuaFunc (compile-time check)
var _ oslib.LuaFunc = osClock
var _ oslib.LuaFunc = osDate
var _ oslib.LuaFunc = osDifftime
var _ oslib.LuaFunc = osExecute
var _ oslib.LuaFunc = osExit
var _ oslib.LuaFunc = osGetenv
var _ oslib.LuaFunc = osRemove
var _ oslib.LuaFunc = osRename
var _ oslib.LuaFunc = osSetlocale
var _ oslib.LuaFunc = osTime
var _ oslib.LuaFunc = osTmpname

// =============================================================================
// OS Functions
// =============================================================================

// osClock returns CPU time used by the program.
// os.clock() -> number (seconds of CPU time)
func osClock(L oslib.LuaAPI) int {
	L.PushNumber(float64(getClock()))
	return 1
}

// osDate returns date/time information.
// os.date([format [, time]]) -> string or table
func osDate(L oslib.LuaAPI) int {
	format := optString(L, 1, "%c")
	var timestamp int64
	if !L.IsNoneOrNil(2) {
		timestamp, _ = L.ToInteger(2)
	} else {
		timestamp = time.Now().Unix()
	}

	var t time.Time
	if strings.HasPrefix(format, "!") {
		// UTC timezone
		t = time.Unix(timestamp, 0).UTC()
		format = format[1:]
	} else {
		// Local timezone
		t = time.Unix(timestamp, 0).Local()
	}

	// If format is "*t", return a table
	if format == "*t" {
		timeToTable(L, t)
		return 1
	}

	// Otherwise, format as string
	result := luaFormatDate(format, t)
	L.PushString(result)
	return 1
}

// osDifftime returns difference between two times.
// os.difftime(t1, t2) -> number (seconds)
func osDifftime(L oslib.LuaAPI) int {
	t1, _ := L.ToInteger(1)
	t2, _ := L.ToInteger(2)
	L.PushNumber(float64(t1 - t2))
	return 1
}

// osExecute executes a shell command.
// os.execute([command]) -> true, exitcode | nil, error message
func osExecute(L oslib.LuaAPI) int {
	var cmd string
	if L.IsNoneOrNil(1) {
		// No command: just check if shell is available
		L.PushBoolean(true)
		L.PushInteger(0)
		return 2
	}
	cmd, _ = L.ToString(1)
	if cmd == "" {
		L.PushBoolean(true)
		L.PushInteger(0)
		return 2
	}

	// Execute command through shell
	execCmd := exec.Command("sh", "-c", cmd)
	err := execCmd.Run()

	if err == nil {
		L.PushBoolean(true)
		L.PushInteger(0)
		return 2
	}

	// Check if it's an exit error with a code
	if exitErr, ok := err.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		if code == 0 {
			L.PushBoolean(true)
		} else {
			L.PushBoolean(false)
		}
		L.PushInteger(int64(code))
		return 2
	}

	// Other errors
	L.PushNil()
	L.PushString(err.Error())
	return 2
}

// osExit exits the program.
// os.exit([code [, close]]) - signals VM to exit (does not call os.Exit)
// Returns: exitCode, closeState
func osExit(L oslib.LuaAPI) int {
	var code int64 = 0
	closeState := false

	if !L.IsNoneOrNil(1) {
		if L.IsBoolean(1) {
			// true -> 0
			if L.ToBoolean(1) {
				code = 0
			} else {
				code = 1
			}
		} else if L.IsString(1) {
			// string -> 1
			code = 1
		} else if L.IsNumber(1) {
			// use the number
			code, _ = L.ToInteger(1)
		}
	}

	if !L.IsNoneOrNil(2) {
		closeState = L.ToBoolean(2)
	}

	// Push results: exit code and close state
	// The VM will handle the actual exit behavior
	L.PushInteger(code)
	L.PushBoolean(closeState)
	return 2
}

// osGetenv returns environment variable value.
// os.getenv(name) -> string or nil
func osGetenv(L oslib.LuaAPI) int {
	// Get argument
	name, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}

	// Get environment variable
	value := os.Getenv(name)
	if value == "" {
		L.PushNil()
	} else {
		L.PushString(value)
	}
	return 1
}

// osRemove deletes a file.
// os.remove(filename) -> true | nil, error message
func osRemove(L oslib.LuaAPI) int {
	filename, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		L.PushString("os.remove: invalid filename")
		return 2
	}

	err := os.Remove(filename)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	L.PushBoolean(true)
	return 1
}

// osRename renames a file.
// os.rename(oldname, newname) -> true | nil, error message
func osRename(L oslib.LuaAPI) int {
	oldname, ok1 := L.ToString(1)
	newname, ok2 := L.ToString(2)
	if !ok1 || !ok2 {
		L.PushNil()
		L.PushString("os.rename: invalid arguments")
		return 2
	}

	err := os.Rename(oldname, newname)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	L.PushBoolean(true)
	return 1
}

// osSetlocale sets the locale.
// os.setlocale(locale [, category]) -> newlocale | nil
func osSetlocale(L oslib.LuaAPI) int {
	locale := optString(L, 1, "")

	// We only support the "C" locale in this implementation
	// Go's standard library doesn't provide locale setting capabilities
	// without external dependencies like golang.org/x/text/language
	if locale == "" || locale == "C" {
		L.PushString("C")
		return 1
	}

	// For any other locale, return nil (not supported)
	L.PushNil()
	return 1
}

// osTime returns timestamp.
// os.time([table]) -> timestamp | nil
func osTime(L oslib.LuaAPI) int {
	if L.IsNoneOrNil(1) {
		// No argument: return current time
		now := time.Now()
		L.PushNumber(float64(now.Unix()))
		return 1
	}

	// Convert table to time.Time
	t, err := tableToTime(L, 1)
	if err != nil {
		L.PushNil()
		return 1
	}

	L.PushNumber(float64(t.Unix()))
	return 1
}

// osTmpname returns a temporary file name.
// os.tmpname() -> string
func osTmpname(L oslib.LuaAPI) int {
	// Create a temporary file and immediately close it
	// We only need the name, not the file handle
	f, err := os.CreateTemp("", "lua_")
	if err != nil {
		// Fallback: generate a name manually
		L.PushString("/tmp/lua_" + strconv.FormatInt(time.Now().UnixNano(), 10))
		return 1
	}
	f.Close()
	os.Remove(f.Name()) // Remove it immediately since we only need the name
	L.PushString(f.Name())
	return 1
}

// =============================================================================
// Platform-specific implementation
// =============================================================================

// getClock returns CPU time in seconds.
// Cross-platform: uses os.Stdout.Stat() for timing on Unix,
// falls back to wall clock on other platforms.
func getClock() float64 {
	return osClockImpl()
}

// =============================================================================
// Helper functions
// =============================================================================

// optString returns string at idx, or def if nil/absent.
func optString(L oslib.LuaAPI, idx int, def string) string {
	if L.IsNoneOrNil(idx) {
		return def
	}
	s, _ := L.ToString(idx)
	return s
}

// luaFormatDate formats a time.Time according to Lua date format specifiers.
// Lua 5.5.1 format specifiers (similar to strftime with some differences):
//   %a - abbreviated weekday name
//   %A - full weekday name
//   %b - abbreviated month name
//   %B - full month name
//   %c - date and time (locale-specific)
//   %d - day of month [01-31]
//   %H - hour [00-23]
//   %I - hour [01-12]
//   %j - day of year [001-366]
//   %m - month [01-12]
//   %M - minute [00-59]
//   %p - AM/PM
//   %S - second [00-59]
//   %U - week number [00-53] (Sunday as first day)
//   %w - weekday [0-6] (Sunday=0)
//   %W - week number [00-53] (Monday as first day)
//   %x - date (locale-specific)
//   %X - time (locale-specific)
//   %y - year [00-99]
//   %Y - year (full)
//   %G - ISO 8601 year
//   %g - ISO 8601 year (2 digits)
//   %u - ISO 8601 weekday (1-7, Monday=1)
//   %V - ISO 8601 week number
//   %% - literal '%'
func luaFormatDate(format string, t time.Time) string {
	var result strings.Builder
	weekday := t.Weekday()

	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			if format[i] == '%' && i+1 >= len(format) {
				result.WriteByte('%')
			} else {
				result.WriteByte(format[i])
			}
			continue
		}

		i++
		switch format[i] {
		case 'a':
			result.WriteString(weekdayShort(weekday))
		case 'A':
			result.WriteString(weekday.String())
		case 'b', 'h':
			result.WriteString(monthShort(t.Month()))
		case 'B':
			result.WriteString(t.Month().String())
		case 'c':
			result.WriteString(t.Format("Mon Jan 02 15:04:05 2006"))
		case 'd':
			result.WriteString(fmtZero(t.Day(), 2))
		case 'H':
			result.WriteString(fmtZero(t.Hour(), 2))
		case 'I':
			hour := t.Hour() % 12
			if hour == 0 {
				hour = 12
			}
			result.WriteString(fmtZero(hour, 2))
		case 'j':
			result.WriteString(fmtZero(t.YearDay(), 3))
		case 'm':
			result.WriteString(fmtZero(int(t.Month()), 2))
		case 'M':
			result.WriteString(fmtZero(t.Minute(), 2))
		case 'p':
			if t.Hour() < 12 {
				result.WriteString("PM")
			} else {
				result.WriteString("AM")
			}
		case 'S':
			result.WriteString(fmtZero(t.Second(), 2))
		case 'U':
			weekNum := isoWeekNumber(t, 0) // Sunday as first day
			result.WriteString(fmtZero(weekNum, 2))
		case 'w':
			result.WriteString(strconv.Itoa(int(weekday)))
		case 'W':
			weekNum := isoWeekNumber(t, 1) // Monday as first day
			result.WriteString(fmtZero(weekNum, 2))
		case 'x':
			result.WriteString(t.Format("01/02/2006"))
		case 'X':
			result.WriteString(t.Format("15:04:05"))
		case 'y':
			result.WriteString(fmtZero(t.Year()%100, 2))
		case 'Y':
			result.WriteString(strconv.Itoa(t.Year()))
		case 'z':
			_, offset := t.Zone()
			sign := "+"
			if offset < 0 {
				sign = "-"
				offset = -offset
			}
			result.WriteString(sign + fmtZero(offset/3600*100+offset%3600/60, 4))
		case 'G':
			year, _ := t.ISOWeek()
			result.WriteString(strconv.Itoa(year))
		case 'g':
			year, _ := t.ISOWeek()
			result.WriteString(fmtZero(year%100, 2))
		case 'u':
			wd := int(weekday)
			if wd == 0 {
				wd = 7
			}
			result.WriteString(strconv.Itoa(wd))
		case 'V':
			_, week := t.ISOWeek()
			result.WriteString(fmtZero(week, 2))
		case '%':
			result.WriteByte('%')
		default:
			result.WriteByte('%')
			result.WriteByte(format[i])
		}
	}

	return result.String()
}

// weekdayShort returns abbreviated weekday name.
func weekdayShort(w time.Weekday) string {
	switch w {
	case time.Sunday:
		return "Sun"
	case time.Monday:
		return "Mon"
	case time.Tuesday:
		return "Tue"
	case time.Wednesday:
		return "Wed"
	case time.Thursday:
		return "Thu"
	case time.Friday:
		return "Fri"
	case time.Saturday:
		return "Sat"
	}
	return ""
}

// monthShort returns abbreviated month name.
func monthShort(m time.Month) string {
	switch m {
	case time.January:
		return "Jan"
	case time.February:
		return "Feb"
	case time.March:
		return "Mar"
	case time.April:
		return "Apr"
	case time.May:
		return "May"
	case time.June:
		return "Jun"
	case time.July:
		return "Jul"
	case time.August:
		return "Aug"
	case time.September:
		return "Sep"
	case time.October:
		return "Oct"
	case time.November:
		return "Nov"
	case time.December:
		return "Dec"
	}
	return ""
}

// fmtZero formats n with leading zeros to width.
func fmtZero(n, width int) string {
	s := strconv.Itoa(n)
	if len(s) < width {
		return strings.Repeat("0", width-len(s)) + s
	}
	return s
}

// isoWeekNumber calculates week number.
// If firstDay is 0, Sunday is first day of week.
// If firstDay is 1, Monday is first day of week.
// Returns week number starting from 0.
func isoWeekNumber(t time.Time, firstDay int) int {
	// Get the first day of the year
	yearStart := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
	yearDay := t.YearDay() - 1 // 0-indexed day of year
	yearStartWd := int(yearStart.Weekday()) - firstDay
	if yearStartWd < 0 {
		yearStartWd += 7
	}
	return (yearDay - yearStartWd + 7) / 7
}

// timeError represents an error from tableToTime.
type timeError struct {
	Field string
}

func (e *timeError) Error() string {
	return "os.time: invalid " + e.Field
}

// tableToTime converts a Lua table to time.Time.
// Table fields: year, month, day, hour, min, sec, isdst
func tableToTime(L oslib.LuaAPI, idx int) (time.Time, error) {
	// Get table from stack
	L.PushValue(idx)

	// year (required)
	L.GetField(-1, "year")
	year, ok := L.ToInteger(-1)
	if !ok {
		L.Pop()
		return time.Time{}, &timeError{Field: "year"}
	}
	L.Pop()
	L.GetField(-1, "month")
	month, ok := L.ToInteger(-1)
	if !ok {
		month = 1
	}
	L.Pop()

	// day (default to 1)
	L.GetField(-1, "day")
	day, ok := L.ToInteger(-1)
	if !ok {
		day = 1
	}
	L.Pop()

	// hour (default to 0)
	L.GetField(-1, "hour")
	hour, _ := L.ToInteger(-1)
	L.Pop()

	// min (default to 0)
	L.GetField(-1, "min")
	min, _ := L.ToInteger(-1)
	L.Pop()

	// sec (default to 0)
	L.GetField(-1, "sec")
	sec, _ := L.ToInteger(-1)
	L.Pop()

	// Pop the table
	L.Pop()

	return time.Date(int(year), time.Month(month), int(day), int(hour), int(min), int(sec), 0, time.UTC), nil
}

// timeToTable converts time.Time to a Lua table.
// Returns table with: year, month, day, hour, min, sec, wday, yday, isdst
func timeToTable(L oslib.LuaAPI, t time.Time) {
	L.CreateTable(0, 9)

	// year
	L.PushInteger(int64(t.Year()))
	L.SetField(-2, "year")

	// month
	L.PushInteger(int64(t.Month()))
	L.SetField(-2, "month")

	// day
	L.PushInteger(int64(t.Day()))
	L.SetField(-2, "day")

	// hour
	L.PushInteger(int64(t.Hour()))
	L.SetField(-2, "hour")

	// min
	L.PushInteger(int64(t.Minute()))
	L.SetField(-2, "min")

	// sec
	L.PushInteger(int64(t.Second()))
	L.SetField(-2, "sec")

	// wday: weekday (Sunday = 1, Lua convention)
	L.PushInteger(int64(t.Weekday()) + 1)
	L.SetField(-2, "wday")

	// yday: day of year (1-366)
	L.PushInteger(int64(t.YearDay()))
	L.SetField(-2, "yday")

	// isdst: daylight saving time flag
	_, isDstOffset := t.Zone()
	L.PushBoolean(isDstOffset != 0)
	L.SetField(-2, "isdst")
}
