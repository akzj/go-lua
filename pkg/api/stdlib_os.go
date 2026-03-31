package api

import (
	"os"
	"time"
)

// openOSLib registers the os library.
func (s *State) openOSLib() {
	s.NewTable()
	osIdx := s.GetTop()

	s.PushFunction(osClock)
	s.SetField(osIdx, "clock")

	s.PushFunction(osTime)
	s.SetField(osIdx, "time")

	s.PushFunction(osDate)
	s.SetField(osIdx, "date")

	s.PushFunction(osExit)
	s.SetField(osIdx, "exit")

	s.PushFunction(osGetenv)
	s.SetField(osIdx, "getenv")

	s.PushFunction(osRemove)
	s.SetField(osIdx, "remove")

	s.PushFunction(osRename)
	s.SetField(osIdx, "rename")

	s.PushFunction(osTmpname)
	s.SetField(osIdx, "tmpname")

	s.PushFunction(osExecute)
	s.SetField(osIdx, "execute")

	s.PushFunction(osSetlocale)
	s.SetField(osIdx, "setlocale")

	s.SetGlobal("os")
}

func osClock(L *State) int {
	L.PushNumber(float64(time.Now().UnixNano()) / 1e9)
	return 1
}

func osTime(L *State) int {
	L.PushNumber(float64(time.Now().Unix()))
	return 1
}

func osDate(L *State) int {
	t := time.Now()
	L.PushString(t.Format("Mon Jan 2 15:04:05 2006"))
	return 1
}

func osExit(L *State) int {
	code := 0
	if L.GetTop() >= 1 {
		if n, ok := L.ToNumber(1); ok {
			code = int(n)
		}
	}
	os.Exit(code)
	return 0
}

func osGetenv(L *State) int {
	if L.GetTop() < 1 {
		L.PushNil()
		return 1
	}
	name, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		return 1
	}
	val := os.Getenv(name)
	if val == "" {
		L.PushNil()
	} else {
		L.PushString(val)
	}
	return 1
}

func osRemove(L *State) int {
	if L.GetTop() < 1 {
		L.PushNil()
		L.PushString("missing filename")
		return 2
	}
	name, ok := L.ToString(1)
	if !ok {
		L.PushNil()
		L.PushString("filename must be a string")
		return 2
	}
	err := os.Remove(name)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}
	L.PushBoolean(true)
	return 1
}

func osRename(L *State) int {
	if L.GetTop() < 2 {
		L.PushNil()
		L.PushString("missing arguments")
		return 2
	}
	oldname, _ := L.ToString(1)
	newname, _ := L.ToString(2)
	err := os.Rename(oldname, newname)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}
	L.PushBoolean(true)
	return 1
}

func osTmpname(L *State) int {
	f, err := os.CreateTemp("", "lua_")
	if err != nil {
		L.PushString("")
		return 1
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	L.PushString(name)
	return 1
}

func osExecute(L *State) int {
	L.PushBoolean(true)
	return 1
}

// osSetlocale implements os.setlocale([locale [, category]])
// Returns the current locale or false if locale cannot be set.
// Currently returns false as locale support is not implemented.
func osSetlocale(L *State) int {
	// os.setlocale returns false (locale not available in this implementation)
	L.PushBoolean(false)
	return 1
}
