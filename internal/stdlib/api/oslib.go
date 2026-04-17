package api

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	luaapi "github.com/akzj/go-lua/internal/api/api"
)

// ---------------------------------------------------------------------------
// OS Library — mirrors loslib.c
// ---------------------------------------------------------------------------

// processStartTime records when the process started, for os.clock().
var processStartTime = time.Now()

func OpenOS(L *luaapi.State) int {
	L.NewLib(map[string]luaapi.CFunction{
		"clock":     osClock,
		"date":      osDate,
		"difftime":  osDifftime,
		"execute":   osExecute,
		"exit":      osExit,
		"getenv":    osGetenv,
		"remove":    osRemove,
		"rename":    osRename,
		"setlocale": osSetlocale,
		"time":      osTime,
		"tmpname":   osTmpname,
	})
	return 1
}

// os.getenv(varname) → string or nil
func osGetenv(L *luaapi.State) int {
	name := L.CheckString(1)
	val, ok := os.LookupEnv(name)
	if ok {
		L.PushString(val)
	} else {
		L.PushNil()
	}
	return 1
}

// os.clock() → number (CPU time in seconds)
func osClock(L *luaapi.State) int {
	// Go doesn't expose CPU clock; approximate with wall time since start
	elapsed := time.Since(processStartTime).Seconds()
	L.PushNumber(elapsed)
	return 1
}

// os.time([table]) → integer
func osTime(L *luaapi.State) int {
	if L.IsNoneOrNil(1) {
		L.PushInteger(time.Now().Unix())
		return 1
	}
	// Table argument — build time from fields
	L.CheckType(1, 5) // TypeTable = 5
	L.SetTop(1)

	year := getTimeField(L, "year", -1, 1900)
	month := getTimeField(L, "month", -1, 1)
	day := getTimeField(L, "day", -1, 0)
	hour := getTimeField(L, "hour", 12, 0)
	min := getTimeField(L, "min", 0, 0)
	sec := getTimeField(L, "sec", 0, 0)
	isdst := getBoolField(L, "isdst")

	// Build a time.Time in local timezone
	loc := time.Local
	t := time.Date(year, time.Month(month), day, hour, min, sec, 0, loc)

	// Handle DST: if isdst is explicitly false but the time is in DST,
	// or vice versa, we may need to adjust. For simplicity, we just use
	// Go's default behavior which handles DST automatically.
	_ = isdst

	// Normalize: update the table fields with the normalized values
	setTimeFields(L, t)

	L.PushInteger(t.Unix())
	return 1
}

// getTimeField reads an integer field from the table at stack top.
// If the field is nil and d >= 0, returns d+delta. If nil and d < 0, errors.
// Otherwise returns the field value - delta.
func getTimeField(L *luaapi.State, key string, d int, delta int) int {
	tp := L.GetField(-1, key) // pushes field value
	if tp == 0 {              // LUA_TNIL
		L.Pop(1)
		if d < 0 {
			L.Errorf("field '%s' missing in date table", key)
			return 0
		}
		return d + delta
	}
	res, ok := L.ToInteger(-1)
	if !ok {
		// Check if it's a non-integer number or a float
		if L.IsNumber(-1) {
			L.Pop(1)
			L.Errorf("field '%s' is not an integer", key)
			return 0
		}
		L.Pop(1)
		L.Errorf("field '%s' is not an integer", key)
		return 0
	}
	L.Pop(1)
	// Check overflow: the result after subtracting delta must fit in int32
	r := int(res) - delta
	if int64(r)+int64(delta) != res {
		L.Errorf("field '%s' is out-of-bound", key)
		return 0
	}
	return r
}

// getBoolField reads a boolean field from the table at stack top.
// Returns -1 if nil (undefined), 0 if false, 1 if true.
func getBoolField(L *luaapi.State, key string) int {
	tp := L.GetField(-1, key)
	if tp == 0 { // nil
		L.Pop(1)
		return -1
	}
	res := L.ToBoolean(-1)
	L.Pop(1)
	if res {
		return 1
	}
	return 0
}

// setTimeFields updates the date table at stack index 1 with normalized values.
func setTimeFields(L *luaapi.State, t time.Time) {
	L.PushInteger(int64(t.Year()))
	L.SetField(1, "year")
	L.PushInteger(int64(t.Month()))
	L.SetField(1, "month")
	L.PushInteger(int64(t.Day()))
	L.SetField(1, "day")
	L.PushInteger(int64(t.Hour()))
	L.SetField(1, "hour")
	L.PushInteger(int64(t.Minute()))
	L.SetField(1, "min")
	L.PushInteger(int64(t.Second()))
	L.SetField(1, "sec")
	L.PushInteger(int64(t.YearDay()))
	L.SetField(1, "yday")
	L.PushInteger(int64(t.Weekday()))
	L.SetField(1, "wday")

	// DST: Go's time package handles this via the zone
	_, offset := t.Zone()
	_, stdOffset := time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, t.Location()).Zone()
	isDST := offset != stdOffset
	L.PushBoolean(isDST)
	L.SetField(1, "isdst")
}

// os.date([format [, time]]) → string or table
func osDate(L *luaapi.State) int {
	format := "%c"
	if !L.IsNoneOrNil(1) {
		format = L.CheckString(1)
	}

	var t time.Time
	if L.IsNoneOrNil(2) {
		t = time.Now()
	} else {
		ts := L.CheckInteger(2)
		t = time.Unix(ts, 0)
	}

	// Check for UTC prefix
	utc := false
	s := format
	if len(s) > 0 && s[0] == '!' {
		utc = true
		s = s[1:]
		t = t.UTC()
	} else {
		t = t.In(time.Local)
	}
	_ = utc

	// Special case: "*t" returns a table
	if s == "*t" {
		L.CreateTable(0, 9)
		L.PushInteger(int64(t.Year()))
		L.SetField(-2, "year")
		L.PushInteger(int64(t.Month()))
		L.SetField(-2, "month")
		L.PushInteger(int64(t.Day()))
		L.SetField(-2, "day")
		L.PushInteger(int64(t.Hour()))
		L.SetField(-2, "hour")
		L.PushInteger(int64(t.Minute()))
		L.SetField(-2, "min")
		L.PushInteger(int64(t.Second()))
		L.SetField(-2, "sec")
		L.PushInteger(int64(t.YearDay()))
		L.SetField(-2, "yday")
		L.PushInteger(int64(t.Weekday()))
		L.SetField(-2, "wday")

		_, offset := t.Zone()
		_, stdOffset := time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, t.Location()).Zone()
		isDST := offset != stdOffset
		L.PushBoolean(isDST)
		L.SetField(-2, "isdst")
		return 1
	}

	// Convert strftime format to Go time format, character by character
	result := strftimeToGo(L, s, t)
	L.PushString(result)
	return 1
}

// Valid strftime conversion specifiers (C99)
var validStrftimeSpecs = "aAbBcCdDeFgGhHIjmMnprRStTuUVwWxXyYzZ%"

// strftimeToGo converts a C strftime format string to a formatted result using Go's time package.
func strftimeToGo(L *luaapi.State, format string, t time.Time) string {
	var buf strings.Builder
	i := 0
	for i < len(format) {
		if format[i] != '%' {
			buf.WriteByte(format[i])
			i++
			continue
		}
		i++ // skip '%'
		if i >= len(format) {
			// Trailing '%' — invalid
			L.Errorf("invalid conversion specifier '%%%s'", string(format[len(format)-1:]))
			return ""
		}
		spec := format[i]

		// Check for invalid specifiers
		if !strings.ContainsRune(validStrftimeSpecs, rune(spec)) {
			L.Errorf("invalid conversion specifier '%%%s'", string(spec))
			return ""
		}

		switch spec {
		case 'a':
			buf.WriteString(t.Format("Mon"))
		case 'A':
			buf.WriteString(t.Format("Monday"))
		case 'b', 'h':
			buf.WriteString(t.Format("Jan"))
		case 'B':
			buf.WriteString(t.Format("January"))
		case 'c':
			buf.WriteString(t.Format("Mon Jan  2 15:04:05 2006"))
		case 'C':
			buf.WriteString(fmt.Sprintf("%02d", t.Year()/100))
		case 'd':
			buf.WriteString(fmt.Sprintf("%02d", t.Day()))
		case 'D':
			buf.WriteString(fmt.Sprintf("%02d/%02d/%02d", t.Month(), t.Day(), t.Year()%100))
		case 'e':
			buf.WriteString(fmt.Sprintf("%2d", t.Day()))
		case 'F':
			buf.WriteString(t.Format("2006-01-02"))
		case 'g':
			y, _ := t.ISOWeek()
			buf.WriteString(fmt.Sprintf("%02d", y%100))
		case 'G':
			y, _ := t.ISOWeek()
			buf.WriteString(fmt.Sprintf("%04d", y))
		case 'H':
			buf.WriteString(fmt.Sprintf("%02d", t.Hour()))
		case 'I':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			buf.WriteString(fmt.Sprintf("%02d", h))
		case 'j':
			buf.WriteString(fmt.Sprintf("%03d", t.YearDay()))
		case 'm':
			buf.WriteString(fmt.Sprintf("%02d", t.Month()))
		case 'M':
			buf.WriteString(fmt.Sprintf("%02d", t.Minute()))
		case 'n':
			buf.WriteByte('\n')
		case 'p':
			if t.Hour() < 12 {
				buf.WriteString("AM")
			} else {
				buf.WriteString("PM")
			}
		case 'r':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			ampm := "AM"
			if t.Hour() >= 12 {
				ampm = "PM"
			}
			buf.WriteString(fmt.Sprintf("%02d:%02d:%02d %s", h, t.Minute(), t.Second(), ampm))
		case 'R':
			buf.WriteString(fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute()))
		case 'S':
			buf.WriteString(fmt.Sprintf("%02d", t.Second()))
		case 't':
			buf.WriteByte('\t')
		case 'T':
			buf.WriteString(fmt.Sprintf("%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second()))
		case 'u':
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			buf.WriteString(fmt.Sprintf("%d", wd))
		case 'U':
			// Week number (Sunday as first day)
			yday := t.YearDay()
			wday := int(t.Weekday())
			wk := (yday + 6 - wday) / 7
			buf.WriteString(fmt.Sprintf("%02d", wk))
		case 'V':
			_, wk := t.ISOWeek()
			buf.WriteString(fmt.Sprintf("%02d", wk))
		case 'w':
			buf.WriteString(fmt.Sprintf("%d", t.Weekday()))
		case 'W':
			// Week number (Monday as first day)
			yday := t.YearDay()
			wday := int(t.Weekday())
			if wday == 0 {
				wday = 7
			}
			wk := (yday + 6 - wday + 1) / 7
			buf.WriteString(fmt.Sprintf("%02d", wk))
		case 'x':
			buf.WriteString(fmt.Sprintf("%02d/%02d/%02d", t.Month(), t.Day(), t.Year()%100))
		case 'X':
			buf.WriteString(fmt.Sprintf("%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second()))
		case 'y':
			buf.WriteString(fmt.Sprintf("%02d", t.Year()%100))
		case 'Y':
			buf.WriteString(fmt.Sprintf("%04d", t.Year()))
		case 'z':
			buf.WriteString(t.Format("-0700"))
		case 'Z':
			name, _ := t.Zone()
			buf.WriteString(name)
		case '%':
			buf.WriteByte('%')
		default:
			// Should not reach here due to validation above
			buf.WriteByte('%')
			buf.WriteByte(spec)
		}
		i++
	}
	return buf.String()
}

// os.difftime(t2, t1) → number
func osDifftime(L *luaapi.State) int {
	t1 := L.CheckInteger(1)
	t2 := L.CheckInteger(2)
	L.PushNumber(float64(t1 - t2))
	return 1
}

// os.execute([command]) → true/nil, "exit"/"signal", code
func osExecute(L *luaapi.State) int {
	if L.IsNoneOrNil(1) {
		// Check if shell is available
		L.PushBoolean(true)
		return 1
	}
	cmd := L.CheckString(1)
	c := exec.Command("sh", "-c", cmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	return pushExecResult(L, err)
}

// pushExecResult pushes the result of os.execute/io.popen:close
// Returns: true/nil, "exit"/"signal", code
func pushExecResult(L *luaapi.State, err error) int {
	if err == nil {
		L.PushBoolean(true)
		L.PushString("exit")
		L.PushInteger(0)
		return 3
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		status := exitErr.Sys().(syscall.WaitStatus)
		if status.Signaled() {
			L.PushNil()
			L.PushString("signal")
			L.PushInteger(int64(status.Signal()))
			return 3
		}
		L.PushNil()
		L.PushString("exit")
		L.PushInteger(int64(status.ExitStatus()))
		return 3
	}
	// Other error
	L.PushNil()
	L.PushString("exit")
	L.PushInteger(1)
	return 3
}

// os.exit([code [, close]])
func osExit(L *luaapi.State) int {
	var status int
	if L.IsBoolean(1) {
		if L.ToBoolean(1) {
			status = 0
		} else {
			status = 1
		}
	} else {
		status = int(L.OptInteger(1, 0))
	}
	// We ignore the 'close' argument (arg 2) since we can't close the Lua state
	os.Exit(status)
	return 0 // unreachable
}

// os.remove(filename) → true or nil, errmsg, errno
func osRemove(L *luaapi.State) int {
	filename := L.CheckString(1)
	err := os.Remove(filename)
	return pushFileResult(L, err == nil, filename, err)
}

// os.rename(oldname, newname) → true or nil, errmsg, errno
func osRename(L *luaapi.State) int {
	from := L.CheckString(1)
	to := L.CheckString(2)
	err := os.Rename(from, to)
	return pushFileResult(L, err == nil, "", err)
}

// os.tmpname() → string
func osTmpname(L *luaapi.State) int {
	f, err := os.CreateTemp("", "lua_")
	if err != nil {
		L.Errorf("unable to generate a unique filename")
		return 0
	}
	name := f.Name()
	f.Close()
	// Remove the file — tmpname just returns the name
	// (C Lua's mkstemp creates then closes the file)
	L.PushString(name)
	return 1
}

// os.setlocale([locale [, category]]) → string or nil
// Go doesn't have locale switching, so this is a minimal implementation.
func osSetlocale(L *luaapi.State) int {
	locale := ""
	if !L.IsNoneOrNil(1) {
		locale = L.CheckString(1)
	}
	// Only "C" and "" are supported
	if locale == "" || locale == "C" || locale == "POSIX" {
		L.PushString("C")
	} else {
		// Unknown locale — return nil (not available)
		L.PushNil()
	}
	return 1
}

// pushFileResult pushes the result of a file operation.
// Mirrors: luaL_fileresult in lauxlib.c
// On success: pushes true, returns 1
// On failure: pushes nil, error message, errno; returns 3
func pushFileResult(L *luaapi.State, ok bool, filename string, err error) int {
	if ok {
		L.PushBoolean(true)
		return 1
	}
	L.PushNil()
	if err != nil {
		msg := err.Error()
		if filename != "" {
			msg = filename + ": " + getErrnoMsg(err)
		} else {
			msg = getErrnoMsg(err)
		}
		L.PushString(msg)
		L.PushInteger(int64(getErrno(err)))
	} else {
		L.PushString("unknown error")
		L.PushInteger(0)
	}
	return 3
}

// getErrnoMsg extracts the error message string from a Go error.
func getErrnoMsg(err error) string {
	if pathErr, ok := err.(*os.PathError); ok {
		return pathErr.Err.Error()
	}
	if linkErr, ok := err.(*os.LinkError); ok {
		return linkErr.Err.Error()
	}
	return err.Error()
}

// getErrno extracts the errno from a Go error.
func getErrno(err error) int {
	if pathErr, ok := err.(*os.PathError); ok {
		if errno, ok := pathErr.Err.(syscall.Errno); ok {
			return int(errno)
		}
	}
	if linkErr, ok := err.(*os.LinkError); ok {
		if errno, ok := linkErr.Err.(syscall.Errno); ok {
			return int(errno)
		}
	}
	if errno, ok := err.(syscall.Errno); ok {
		return int(errno)
	}
	return 0
}
