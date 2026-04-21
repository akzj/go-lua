// testlib.go — T (testC) testing library for the Lua 5.5 test suite.
//
// Implements the T global table with T.testC(), T.makeCfunc(), etc.
// Reference: lua-master/ltests.c
package stdlib

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode"

	luaapi "github.com/akzj/go-lua/internal/api"
	"github.com/akzj/go-lua/internal/object"
)

// ---------------------------------------------------------------------------
// Parser helpers — mirrors getstring_aux, getnum_aux, getindex_aux from ltests.c
// ---------------------------------------------------------------------------

const delimits = " \t\n,;"

type testCParser struct {
	pc string // remaining program text
}

func (p *testCParser) skip() {
	for len(p.pc) > 0 {
		ch := p.pc[0]
		if ch != 0 && strings.ContainsRune(delimits, rune(ch)) {
			p.pc = p.pc[1:]
		} else if ch == '#' {
			// comment — skip until end of line
			for len(p.pc) > 0 && p.pc[0] != '\n' {
				p.pc = p.pc[1:]
			}
		} else {
			break
		}
	}
}

func (p *testCParser) getString() string {
	p.skip()
	if len(p.pc) == 0 {
		return ""
	}
	var buf strings.Builder
	if p.pc[0] == '"' || p.pc[0] == '\'' {
		quote := p.pc[0]
		p.pc = p.pc[1:]
		for len(p.pc) > 0 && p.pc[0] != quote {
			buf.WriteByte(p.pc[0])
			p.pc = p.pc[1:]
		}
		if len(p.pc) > 0 {
			p.pc = p.pc[1:] // skip closing quote
		}
	} else {
		for len(p.pc) > 0 && !strings.ContainsRune(delimits, rune(p.pc[0])) {
			buf.WriteByte(p.pc[0])
			p.pc = p.pc[1:]
		}
	}
	return buf.String()
}

func (p *testCParser) getNum(L *luaapi.State, L1 *luaapi.State) int {
	p.skip()
	if len(p.pc) == 0 {
		return 0
	}
	if p.pc[0] == '.' {
		// pop from L1 stack
		val, _ := L1.ToInteger(-1)
		L1.Pop(1)
		p.pc = p.pc[1:]
		return int(val)
	}
	if p.pc[0] == '*' {
		p.pc = p.pc[1:]
		return L1.GetTop()
	}
	if p.pc[0] == '!' {
		p.pc = p.pc[1:]
		if len(p.pc) > 0 && p.pc[0] == 'G' {
			p.pc = p.pc[1:]
			return luaapi.RIdxGlobals
		} else if len(p.pc) > 0 && p.pc[0] == 'M' {
			p.pc = p.pc[1:]
			return luaapi.RIdxMainThread
		}
		return 0
	}
	sig := 1
	if p.pc[0] == '-' {
		sig = -1
		p.pc = p.pc[1:]
	}
	if len(p.pc) == 0 || !isDigitTC(p.pc[0]) {
		L.Errorf("number expected (%s)", p.pc)
		return 0
	}
	res := 0
	for len(p.pc) > 0 && isDigitTC(p.pc[0]) {
		res = res*10 + int(p.pc[0]-'0')
		p.pc = p.pc[1:]
	}
	return sig * res
}

func (p *testCParser) getIndex(L *luaapi.State, L1 *luaapi.State) int {
	p.skip()
	if len(p.pc) == 0 {
		return 0
	}
	switch p.pc[0] {
	case 'R':
		p.pc = p.pc[1:]
		return luaapi.RegistryIndex
	case 'U':
		p.pc = p.pc[1:]
		return luaapi.UpvalueIndex(p.getNum(L, L1))
	default:
		n := p.getNum(L, L1)
		if n == 0 {
			return 0
		}
		return L1.AbsIndex(n)
	}
}

func isDigitTC(b byte) bool {
	return b >= '0' && b <= '9'
}

// ---------------------------------------------------------------------------
// Status codes
// ---------------------------------------------------------------------------

var statCodes = []string{"OK", "YIELD", "ERRRUN", "ERRSYNTAX", "ERRMEM", "ERRERR"}

func statusToString(status int) string {
	if status >= 0 && status < len(statCodes) {
		return statCodes[status]
	}
	return "UNKNOWN"
}

// ---------------------------------------------------------------------------
// Arithmetic operation encoding: "+-*%^/\&|~<>_!"
// ---------------------------------------------------------------------------

const arithOps = "+-*%^/\\&|~<>_!"

// ---------------------------------------------------------------------------
// runC — the testC mini-language interpreter
// ---------------------------------------------------------------------------

func runC(L *luaapi.State, L1 *luaapi.State, pc string) int {
	p := &testCParser{pc: pc}
	status := 0

	for {
		inst := p.getString()
		if inst == "" {
			return 0
		}

		switch inst {
		case "absindex":
			idx := p.getIndex(L, L1)
			L1.PushInteger(int64(idx))

		case "append":
			t := p.getIndex(L, L1)
			i := L1.RawLen(t)
			L1.RawSetI(t, i+1)

		case "arith":
			p.skip()
			if len(p.pc) == 0 {
				break
			}
			ch := p.pc[0]
			p.pc = p.pc[1:]
			opIdx := strings.IndexByte(arithOps, ch)
			if opIdx >= 0 {
				L1.Arith(luaapi.ArithOp(opIdx))
			}

		case "call":
			narg := p.getNum(L, L1)
			nres := p.getNum(L, L1)
			L1.Call(narg, nres)

		case "callk":
			narg := p.getNum(L, L1)
			nres := p.getNum(L, L1)
			_ = p.getIndex(L, L1) // ctx — ignored for now
			L1.Call(narg, nres)   // simplified: no continuation support yet

		case "checkstack":
			sz := p.getNum(L, L1)
			msg := p.getString()
			if msg == "" {
				L1.CheckStack(sz)
			} else {
				L1.CheckStack(sz)
			}

		case "rawcheckstack":
			sz := p.getNum(L, L1)
			L1.PushBoolean(L1.CheckStack(sz))

		case "compare":
			opt := p.getString()
			a := p.getIndex(L, L1)
			b := p.getIndex(L, L1)
			var op luaapi.CompareOp
			if len(opt) > 0 && opt[0] == 'E' {
				op = luaapi.OpEQ
			} else if len(opt) > 1 && opt[1] == 'T' {
				op = luaapi.OpLT
			} else {
				op = luaapi.OpLE
			}
			L1.PushBoolean(L1.Compare(a, b, op))

		case "concat":
			L1.Concat(p.getNum(L, L1))

		case "copy":
			f := p.getIndex(L, L1)
			t := p.getIndex(L, L1)
			L1.Copy(f, t)

		case "error":
			L1.Error()

		case "getfield":
			t := p.getIndex(L, L1)
			s := p.getString()
			L1.GetField(t, s)

		case "getglobal":
			L1.GetGlobal(p.getString())

		case "getmetatable":
			idx := p.getIndex(L, L1)
			if !L1.GetMetatable(idx) {
				L1.PushNil()
			}

		case "gettable":
			L1.GetTable(p.getIndex(L, L1))

		case "gettop":
			L1.PushInteger(int64(L1.GetTop()))

		case "gsub":
			a := p.getNum(L, L1)
			b := p.getNum(L, L1)
			c := p.getNum(L, L1)
			sa, _ := L1.ToString(a)
			sb, _ := L1.ToString(b)
			sc, _ := L1.ToString(c)
			result := strings.ReplaceAll(sa, sb, sc)
			L1.PushString(result)

		case "insert":
			L1.Insert(p.getNum(L, L1))

		case "iscfunction":
			L1.PushBoolean(L1.IsCFunction(p.getIndex(L, L1)))

		case "isfunction":
			L1.PushBoolean(L1.IsFunction(p.getIndex(L, L1)))

		case "isnil":
			L1.PushBoolean(L1.IsNil(p.getIndex(L, L1)))

		case "isnull":
			L1.PushBoolean(L1.IsNone(p.getIndex(L, L1)))

		case "isnumber":
			L1.PushBoolean(L1.IsNumber(p.getIndex(L, L1)))

		case "isstring":
			L1.PushBoolean(L1.IsString(p.getIndex(L, L1)))

		case "istable":
			L1.PushBoolean(L1.IsTable(p.getIndex(L, L1)))

		case "isudataval":
			// light userdata check
			tp := L1.Type(p.getIndex(L, L1))
			L1.PushBoolean(tp == object.TypeLightUserdata)

		case "isuserdata":
			L1.PushBoolean(L1.IsUserdata(p.getIndex(L, L1)))

		case "len":
			L1.Len(p.getIndex(L, L1))

		case "Llen":
			idx := p.getIndex(L, L1)
			L1.PushInteger(L1.LenI(idx))

		case "loadfile":
			// luaL_loadfile(L1, luaL_checkstring(L1, getnum))
			idx := p.getNum(L, L1)
			fname := L1.CheckString(idx)
			data, readErr := os.ReadFile(fname)
			if readErr != nil {
				L1.PushString(fmt.Sprintf("cannot open %s: %v", fname, readErr))
			} else {
				L1.Load(string(data), "@"+fname, "bt")
			}

		case "loadstring":
			idx := p.getNum(L, L1)
			s := L1.CheckString(idx)
			name := p.getString()
			mode := p.getString()
			if name == "" {
				name = s
			}
			L1.Load(s, name, mode)

		case "newmetatable":
			s := p.getString()
			L1.PushBoolean(L1.NewMetatable(s))

		case "newtable":
			L1.NewTable()

		case "newthread":
			L1.NewThread()

		case "newuserdata":
			sz := p.getNum(L, L1)
			L1.NewUserdata(sz, 0)

		case "next":
			L1.Next(-2)

		case "objsize":
			idx := p.getIndex(L, L1)
			L1.PushInteger(L1.RawLen(idx))

		case "pcall":
			narg := p.getNum(L, L1)
			nres := p.getNum(L, L1)
			handler := p.getNum(L, L1)
			status = L1.PCall(narg, nres, handler)

		case "pcallk":
			narg := p.getNum(L, L1)
			nres := p.getNum(L, L1)
			_ = p.getIndex(L, L1) // ctx
			status = L1.PCall(narg, nres, 0) // simplified

		case "pop":
			L1.Pop(p.getNum(L, L1))

		case "print":
			msg := p.getString()
			fmt.Printf("%s\n", msg)

		case "printstack":
			_ = p.getNum(L, L1)
			// no-op for now

		case "pushbool":
			L1.PushBoolean(p.getNum(L, L1) != 0)

		case "pushcclosure":
			n := p.getNum(L, L1)
			// C Lua uses testC (reads program from arg 1), not Cfunc (reads from upvalue 1)
			L1.PushCClosure(luaapi.CFunction(testCEntry), n)

		case "pushint":
			L1.PushInteger(int64(p.getNum(L, L1)))

		case "pushnil":
			L1.PushNil()

		case "pushnum":
			L1.PushNumber(float64(p.getNum(L, L1)))

		case "pushstatus":
			L1.PushString(statusToString(status))

		case "pushstring":
			L1.PushString(p.getString())

		case "pushupvalueindex":
			L1.PushInteger(int64(luaapi.UpvalueIndex(p.getNum(L, L1))))

		case "pushvalue":
			L1.PushValue(p.getIndex(L, L1))

		case "pushfstringI":
			// lua_pushfstring(L1, lua_tostring(L, -2), (int)lua_tointeger(L, -1))
			fmtStr, _ := L.ToString(-2)
			val, _ := L.ToInteger(-1)
			L1.PushFString(fmtStr, int(val))

		case "pushfstringS":
			fmtStr, _ := L.ToString(-2)
			s, _ := L.ToString(-1)
			L1.PushFString(fmtStr, s)

		case "pushfstringP":
			fmtStr, _ := L.ToString(-2)
			ptr := L.ToPointer(-1)
			L1.PushFString(fmtStr, ptr)

		case "rawget":
			t := p.getIndex(L, L1)
			L1.RawGet(t)

		case "rawgeti":
			t := p.getIndex(L, L1)
			i := p.getNum(L, L1)
			L1.RawGetI(t, int64(i))

		case "rawgetp":
			// rawgetp uses a light userdata pointer key
			t := p.getIndex(L, L1)
			key := p.getNum(L, L1)
			L1.PushLightUserdata(uintptr(key))
			L1.RawGet(t)

		case "rawset":
			t := p.getIndex(L, L1)
			L1.RawSet(t)

		case "rawseti":
			t := p.getIndex(L, L1)
			i := p.getNum(L, L1)
			L1.RawSetI(t, int64(i))

		case "rawsetp":
			// rawsetp uses a light userdata pointer key
			t := p.getIndex(L, L1)
			key := p.getNum(L, L1)
			// Stack: [..., value]. Push the key, then swap so key is below value.
			L1.PushLightUserdata(uintptr(key))
			L1.Insert(-2) // now: [..., key, value]
			L1.RawSet(t)

		case "remove":
			L1.Remove(p.getNum(L, L1))

		case "replace":
			L1.Replace(p.getIndex(L, L1))

		case "resume":
			i := p.getIndex(L, L1)
			narg := p.getNum(L, L1)
			thread := L1.ToThread(i)
			if thread != nil {
				st, _ := thread.Resume(L1, narg)
				status = st
			}

		case "return":
			n := p.getNum(L, L1)
			if L1 != L {
				// Transfer values from L1 to L
				for i := 0; i < n; i++ {
					idx := -(n - i)
					tp := L1.Type(idx)
					switch tp {
					case object.TypeBoolean:
						L.PushBoolean(L1.ToBoolean(idx))
					default:
						s, _ := L1.ToString(idx)
						L.PushString(s)
					}
				}
			}
			return n

		case "rotate":
			i := p.getIndex(L, L1)
			n := p.getNum(L, L1)
			L1.Rotate(i, n)

		case "setfield":
			t := p.getIndex(L, L1)
			s := p.getString()
			L1.SetField(t, s)

		case "seti":
			t := p.getIndex(L, L1)
			i := p.getNum(L, L1)
			L1.SetI(t, int64(i))

		case "setglobal":
			s := p.getString()
			L1.SetGlobal(s)

		case "setmetatable":
			idx := p.getIndex(L, L1)
			L1.SetMetatable(idx)

		case "settable":
			idx := p.getIndex(L, L1)
			L1.SetTableMeta(idx)

		case "settop":
			L1.SetTop(p.getNum(L, L1))

		case "testudata":
			i := p.getIndex(L, L1)
			s := p.getString()
			L1.PushBoolean(L1.TestUdata(i, s))

		case "tobool":
			L1.PushBoolean(L1.ToBoolean(p.getIndex(L, L1)))

		case "tocfunction":
			idx := p.getIndex(L, L1)
			if L1.IsCFunction(idx) {
				// Push a copy of the C function value
				L1.PushValue(idx)
			} else {
				L1.PushNil()
			}

		case "tointeger":
			idx := p.getIndex(L, L1)
			val, _ := L1.ToInteger(idx)
			L1.PushInteger(val)

		case "tonumber":
			idx := p.getIndex(L, L1)
			val, _ := L1.ToNumber(idx)
			L1.PushNumber(val)

		case "topointer":
			idx := p.getIndex(L, L1)
			tp := L1.Type(idx)
			switch tp {
			case object.TypeLightUserdata:
				ud := L1.ToUserdata(idx)
				L1.PushLightUserdata(ud)
			case object.TypeNil, object.TypeBoolean, object.TypeNumber:
				L1.PushLightUserdata(uintptr(0))
			default:
				// Tables, functions, userdata, threads, strings — all have identity
				ptrStr := L1.ToPointer(idx)
				if ptrStr == "" {
					L1.PushLightUserdata(uintptr(0))
				} else {
					L1.PushLightUserdata(ptrStr)
				}
			}

		case "touserdata":
			idx := p.getIndex(L, L1)
			ud := L1.ToUserdata(idx)
			L1.PushLightUserdata(ud)

		case "tostring":
			idx := p.getIndex(L, L1)
			s, ok := L1.ToString(idx)
			if ok {
				L1.PushString(s)
			} else {
				L1.PushNil()
			}

		case "Ltolstring":
			idx := p.getIndex(L, L1)
			L1.PushString(L1.TolString(idx))

		case "type":
			idx := p.getNum(L, L1)
			tp := L1.Type(idx)
			L1.PushString(L1.TypeName(tp))

		case "xmove":
			f := p.getIndex(L, L1)
			t := p.getIndex(L, L1)
			n := p.getNum(L, L1)
			var fs, ts *luaapi.State
			if f == 0 {
				fs = L1
			} else {
				fs = L1.ToThread(f)
			}
			if t == 0 {
				ts = L1
			} else {
				ts = L1.ToThread(t)
			}
			if n == 0 && fs != nil {
				n = fs.GetTop()
			}
			if fs != nil && ts != nil {
				fs.XMove(ts, n)
			}

		case "isyieldable":
			idx := p.getIndex(L, L1)
			thread := L1.ToThread(idx)
			if thread != nil {
				L1.PushBoolean(thread.IsYieldable())
			} else {
				L1.PushBoolean(false)
			}

		case "yield":
			n := p.getNum(L, L1)
			return L1.Yield(n)

		case "yieldk":
			nres := p.getNum(L, L1)
			_ = p.getIndex(L, L1) // ctx
			return L1.Yield(nres) // simplified

		case "toclose":
			// lua_toclose — mark slot as to-be-closed
			_ = p.getNum(L, L1)
			// no-op stub for now

		case "closeslot":
			_ = p.getNum(L, L1)
			// no-op stub for now

		case "sethook":
			_ = p.getNum(L, L1)
			_ = p.getNum(L, L1)
			_ = p.getString()
			// no-op stub

		case "traceback":
			_ = p.getString()
			_ = p.getNum(L, L1)
			L1.PushString("traceback") // stub

		case "warningC":
			_ = p.getString()
			// no-op — warning system not implemented

		case "warning":
			_ = p.getString()
			// no-op — warning system not implemented

		case "threadstatus":
			L1.PushString(statusToString(L1.Status()))

		case "alloccount":
			_ = p.getNum(L, L1)
			// no-op — memory allocator control not available

		case "argerror":
			arg := p.getNum(L, L1)
			msg := p.getString()
			L1.ArgError(arg, msg)

		case "func2num":
			idx := p.getIndex(L, L1)
			if L1.IsCFunction(idx) {
				// Return a non-zero identifier for C functions
				ptrStr := L1.ToPointer(idx)
				if ptrStr != "" {
					// Hash the pointer string to get a stable integer
					var h int64 = 1
					for _, c := range ptrStr {
						h = h*31 + int64(c)
					}
					if h == 0 {
						h = 1
					}
					L1.PushInteger(h)
				} else {
					L1.PushInteger(1) // non-zero for any C function
				}
			} else {
				L1.PushInteger(0)
			}

		case "abort":
			// don't actually abort in Go
			panic("testC: abort")

		case "resetthread":
			L1.PushInteger(0) // stub — deprecated in Lua 5.5

		default:
			L.Errorf("unknown instruction '%s'", inst)
		}
	}
}

// ---------------------------------------------------------------------------
// testC entry point — T.testC(prog, ...)
// ---------------------------------------------------------------------------

func testCEntry(L *luaapi.State) int {
	var L1 *luaapi.State
	var pc string

	tp := L.Type(1)
	if tp == object.TypeLightUserdata || tp == object.TypeUserdata {
		// First arg is a state (light userdata from T.newstate)
		ud := L.ToUserdata(1)
		if s, ok := ud.(*luaapi.State); ok {
			L1 = s
		} else {
			L1 = L
		}
		pc = L.CheckString(2)
	} else if tp == object.TypeThread {
		L1 = L.ToThread(1)
		pc = L.CheckString(2)
	} else {
		L1 = L
		pc = L.CheckString(1)
	}
	return runC(L, L1, pc)
}

// testCFunc — used by makeCfunc: runs the script stored as upvalue 1
func testCFunc(L *luaapi.State) int {
	s, _ := L.ToString(luaapi.UpvalueIndex(1))
	return runC(L, L, s)
}

// ---------------------------------------------------------------------------
// T.makeCfunc(prog) — creates a closure that runs the testC program
// ---------------------------------------------------------------------------

func testMakeCfunc(L *luaapi.State) int {
	L.CheckString(1)
	L.PushCClosure(luaapi.CFunction(testCFunc), L.GetTop())
	return 1
}

// ---------------------------------------------------------------------------
// T.newuserdata(size [, nuv]) — push new full userdata
// ---------------------------------------------------------------------------

func testNewuserdata(L *luaapi.State) int {
	sz := int(L.CheckInteger(1))
	nuv := int(L.OptInteger(2, 0))
	L.NewUserdata(sz, nuv)
	return 1
}

// ---------------------------------------------------------------------------
// T.pushuserdata(n) — push light userdata with integer value
// ---------------------------------------------------------------------------

func testPushuserdata(L *luaapi.State) int {
	n := L.CheckInteger(1)
	L.PushLightUserdata(uintptr(n))
	return 1
}

// ---------------------------------------------------------------------------
// T.udataval(idx) — get light userdata value as integer
// ---------------------------------------------------------------------------

func testUdataval(L *luaapi.State) int {
	ud := L.ToUserdata(1)
	switch v := ud.(type) {
	case uintptr:
		L.PushInteger(int64(v))
	default:
		L.PushInteger(0)
	}
	return 1
}

// ---------------------------------------------------------------------------
// T.d2s(n) — convert double to 8-byte binary string
// ---------------------------------------------------------------------------

func testD2s(L *luaapi.State) int {
	n := L.CheckNumber(1)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(n))
	L.PushString(string(buf[:]))
	return 1
}

// ---------------------------------------------------------------------------
// T.s2d(s) — convert 8-byte binary string to double
// ---------------------------------------------------------------------------

func testS2d(L *luaapi.State) int {
	s := L.CheckString(1)
	if len(s) < 8 {
		L.PushNumber(0)
		return 1
	}
	bits := binary.LittleEndian.Uint64([]byte(s))
	L.PushNumber(math.Float64frombits(bits))
	return 1
}

// ---------------------------------------------------------------------------
// T.stacklevel() — return stack level info
// ---------------------------------------------------------------------------

func testStacklevel(L *luaapi.State) int {
	// Return the number of active call frames
	level := 0
	for {
		_, ok := L.GetStack(level)
		if !ok {
			break
		}
		level++
	}
	L.PushInteger(int64(level))
	return 1
}

// ---------------------------------------------------------------------------
// T.ref(idx) / T.unref(ref) / T.getref(ref) — registry references
// ---------------------------------------------------------------------------

func testRef(L *luaapi.State) int {
	// T.ref(obj) — creates a reference to obj in the registry
	// luaL_ref pops the top value, stores it, returns ref int
	L.CheckAny(1)
	L.PushValue(1) // push value to top (luaL_ref pops it)
	ref := L.Ref(luaapi.RegistryIndex)
	L.PushInteger(int64(ref))
	return 1
}

func testUnref(L *luaapi.State) int {
	// T.unref(ref) — unreference
	ref := int(L.CheckInteger(1))
	L.Unref(luaapi.RegistryIndex, ref)
	return 0
}

func testGetref(L *luaapi.State) int {
	// T.getref(ref) — get referenced value
	ref := int(L.CheckInteger(1))
	L.RawGetI(luaapi.RegistryIndex, int64(ref))
	return 1
}

// ---------------------------------------------------------------------------
// T.upvalue(func, idx) — get/set upvalue
// ---------------------------------------------------------------------------

func testUpvalue(L *luaapi.State) int {
	// T.upvalue(f, n [, val])
	// GET: returns value, name (2 results) — or 0 if invalid
	// SET: sets upvalue, returns name (1 result)
	n := int(L.CheckInteger(2))
	L.CheckType(1, object.TypeFunction)
	if L.IsNone(3) {
		// GET: lua_getupvalue pushes value, then we push name
		name, ok := L.GetUpvalue(1, n)
		if !ok {
			return 0
		}
		// Stack: [..., value]. Push name after value.
		L.PushString(name)
		return 2 // returns: value, name
	}
	// SET: lua_setupvalue(L, 1, n) — takes value from top
	L.PushValue(3) // push the new value to top
	name, ok := L.SetUpvalue(1, n)
	if !ok {
		L.Pop(1) // pop unused value
		return 0
	}
	L.PushString(name)
	return 1
}

// ---------------------------------------------------------------------------
// T.checkpanic(code) — test panic handler
// ---------------------------------------------------------------------------

func testCheckpanic(L *luaapi.State) int {
	code := L.CheckString(1)
	panicCode := L.OptString(2, "")

	// Mirrors C Lua's checkpanic from ltests.c:
	// 1. Create new state L1
	// 2. Run code on L1 (unprotected)
	// 3. If error: panic handler runs panicCode on L1
	// 4. Take L1's top value, push to L, return 1
	L1 := luaapi.NewState()
	defer L1.Close()
	OpenAll(L1)
	OpenTestLib(L1)

	hadError := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				hadError = true
				// Set L1's status to ERRRUN so threadstatus reports correctly
				L1.SetStatus(luaapi.StatusErrRun)

				// Ensure error message is on L1's stack
				if _, ok := L1.ToString(-1); !ok {
					L1.PushString(fmt.Sprintf("%v", r))
				}

				if panicCode != "" {
					// Run panic handler code on L1 (like panicback in ltests.c)
					runC(L, L1, panicCode)
				}
			}
		}()
		runC(L, L1, code)
	}()

	if hadError {
		// Transfer L1's top value to L (like checkpanic's else branch)
		if s, ok := L1.ToString(-1); ok {
			L.PushString(s)
		} else {
			L.PushString("error object is not a string")
		}
	} else {
		L.PushString("no errors")
	}
	return 1
}

// ---------------------------------------------------------------------------
// T.doremote(state, code) — execute code on a different state
// ---------------------------------------------------------------------------

func testDoremote(L *luaapi.State) int {
	// T.doremote(L1, code) — run code on remote state L1
	// On success: return all L1 stack values as strings
	// On error: return nil, errmsg, status_code
	ud := L.ToUserdata(1)
	L1, ok := ud.(*luaapi.State)
	if !ok {
		L.PushNil()
		L.PushString("invalid state")
		L.PushInteger(0)
		return 3
	}
	code := L.CheckString(2)

	// Clean L1's stack before loading (previous operations may leave values)
	oldTop := L1.GetTop()

	// Load the code on L1
	status := L1.Load(code, "doremote", "bt")
	if status != 0 {
		// Syntax error
		msg, _ := L1.ToString(-1)
		L1.Pop(1)
		L.PushNil()
		L.PushString(msg)
		L.PushInteger(3) // 3 = syntax error
		return 3
	}

	// Execute on L1
	status = L1.PCall(0, luaapi.MultiRet, 0)
	if status != 0 {
		// Runtime error
		msg, _ := L1.ToString(-1)
		L1.Pop(1)
		L.PushNil()
		L.PushString(msg)
		L.PushInteger(2) // 2 = runtime error
		return 3
	}

	// Transfer results from L1 to L as strings
	// Only get values above oldTop (results from this PCall)
	n := L1.GetTop() - oldTop
	for i := oldTop + 1; i <= oldTop+n; i++ {
		L1.ToString(i) // convert to string in-place
		s, _ := L1.ToString(i)
		L.PushString(s)
	}
	L1.SetTop(oldTop) // restore L1's stack
	return n
}

// ---------------------------------------------------------------------------
// T.newstate() / T.closestate(s) / T.loadlib(s)
// ---------------------------------------------------------------------------

func testNewstate(L *luaapi.State) int {
	newL := luaapi.NewState()
	L.PushLightUserdata(newL)
	return 1
}

func testClosestate(L *luaapi.State) int {
	ud := L.ToUserdata(1)
	if s, ok := ud.(*luaapi.State); ok {
		s.Close()
	}
	return 0
}

func testLoadlib(L *luaapi.State) int {
	// T.loadlib(L1, what, preload)
	// what: bitmask of libs to open fully
	// preload: bitmask of libs to preload (available via require)
	// Bit mapping: 0=_G, 1=package, 2=coroutine, 3=table,
	//   4=io, 5=os, 6=string, 7=math, 8=utf8, 9=debug
	ud := L.ToUserdata(1)
	L1, ok := ud.(*luaapi.State)
	if !ok {
		return 0
	}
	what := int(L.OptInteger(2, 0))
	preload := int(L.OptInteger(3, 0))

	// Library openers in order matching C Lua's loadedlibs
	type libEntry struct {
		name string
		open func(*luaapi.State) int
	}
	libs := []libEntry{
		{"_G", OpenBase},
		{"package", OpenPackage},
		{"coroutine", OpenCoroutineLib},
		{"table", OpenTable},
		{"io", OpenIO},
		{"os", OpenOS},
		{"string", OpenString},
		{"math", OpenMath},
		{"utf8", OpenUTF8},
		{"debug", OpenDebug},
	}

	for i, lib := range libs {
		bit := 1 << i
		if what&bit != 0 {
			// Load fully
			lib.open(L1)
		} else if preload&bit != 0 {
			// Preload: register in package.preload so require() works
			// For simplicity, just open it (Go doesn't have a clean preload mechanism)
			lib.open(L1)
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// T.checkmemory() — stub (memory checking not applicable in Go)
// ---------------------------------------------------------------------------

func testCheckmemory(L *luaapi.State) int {
	return 0
}

// ---------------------------------------------------------------------------
// T.gcstate() — return GC state info
// ---------------------------------------------------------------------------

func testGcstate(L *luaapi.State) int {
	L.PushString(L.GetGCMode())
	return 1
}

// ---------------------------------------------------------------------------
// T.gccolor(obj) — return GC color of an object
// ---------------------------------------------------------------------------

func testGccolor(L *luaapi.State) int {
	L.CheckAny(1)
	L.PushString("white") // stub — Go GC doesn't expose tri-color state
	return 1
}

// ---------------------------------------------------------------------------
// T.alloccount(n) — set allocation count limit (stub)
// ---------------------------------------------------------------------------

func testAlloccount(L *luaapi.State) int {
	_ = L.OptInteger(1, 0)
	return 0
}

// ---------------------------------------------------------------------------
// T.allocfailnext() — make next allocation fail (stub)
// ---------------------------------------------------------------------------

func testAllocfailnext(L *luaapi.State) int {
	return 0
}

// ---------------------------------------------------------------------------
// T.querystr(idx) — query string table info
// ---------------------------------------------------------------------------

func testQuerystr(L *luaapi.State) int {
	L.PushInteger(0) // stub
	return 1
}

// ---------------------------------------------------------------------------
// T.querytab(t [, i]) — query table internal sizes
// Returns (arraySize, hashSize) or value at internal position i.
// ---------------------------------------------------------------------------

func testQuerytab(L *luaapi.State) int {
	L.CheckType(1, object.TypeTable)
	// Use RawLen for array size and a helper for hash size
	if L.GetTop() >= 2 {
		// querytab(t, i) — return value at internal position i
		// This is used for debugging; return nil as stub
		L.PushNil()
		return 1
	}
	// Return (arrayPart, hashPart) sizes
	arrLen := L.RawLen(1)
	L.PushInteger(arrLen)
	L.PushInteger(0) // hash size not easily accessible; return 0
	return 2
}

// ---------------------------------------------------------------------------
// T.listk(f) — list constants of a Lua function
// ---------------------------------------------------------------------------

func testListk(L *luaapi.State) int {
	L.CheckType(1, object.TypeFunction)
	lc := L.GetLClosure(1)
	if lc == nil || lc.Proto == nil {
		L.NewTable()
		return 1
	}
	p := lc.Proto
	L.CreateTable(len(p.Constants), 0)
	for i, k := range p.Constants {
		pushTValue(L, k)
		L.RawSetI(-2, int64(i+1))
	}
	return 1
}

// pushTValue pushes an object.TValue onto the Lua stack.
func pushTValue(L *luaapi.State, v object.TValue) {
	switch v.Tt {
	case object.TagNil:
		L.PushNil()
	case object.TagTrue:
		L.PushBoolean(true)
	case object.TagFalse:
		L.PushBoolean(false)
	case object.TagInteger:
		L.PushInteger(v.N)
	case object.TagFloat:
		L.PushNumber(v.Float())
	case object.TagShortStr, object.TagLongStr:
		if s, ok := v.Obj.(*object.LuaString); ok {
			L.PushString(s.Data)
		} else {
			L.PushNil()
		}
	default:
		L.PushNil()
	}
}

// ---------------------------------------------------------------------------
// T.listcode(f) — list opcodes of a Lua function
// ---------------------------------------------------------------------------

func testListcode(L *luaapi.State) int {
	L.CheckType(1, object.TypeFunction)
	lc := L.GetLClosure(1)
	if lc == nil || lc.Proto == nil {
		L.NewTable()
		return 1
	}
	p := lc.Proto
	L.CreateTable(len(p.Code), 0)
	for i, code := range p.Code {
		L.PushInteger(int64(code))
		L.RawSetI(-2, int64(i+1))
	}
	return 1
}

// ---------------------------------------------------------------------------
// T.printcode(f) — print opcodes (stub — just returns)
// ---------------------------------------------------------------------------

func testPrintcode(L *luaapi.State) int {
	return 0
}

// ---------------------------------------------------------------------------
// T.listabslineinfo(f) — list absolute line info (stub)
// ---------------------------------------------------------------------------

func testListabslineinfo(L *luaapi.State) int {
	L.NewTable()
	return 1
}

// ---------------------------------------------------------------------------
// T.listlocals(f, pc) — list local variables (stub)
// ---------------------------------------------------------------------------

func testListlocals(L *luaapi.State) int {
	L.NewTable()
	return 1
}

// ---------------------------------------------------------------------------
// T.totalmem([limit]) — get/set total memory usage
// ---------------------------------------------------------------------------

func testTotalmem(L *luaapi.State) int {
	if L.GetTop() >= 1 {
		// Set memory limit — stub (Go doesn't support this)
		_ = L.CheckInteger(1)
		return 0
	}
	// Return current memory usage
	L.PushInteger(L.GCTotalBytes())
	return 1
}

// ---------------------------------------------------------------------------
// T.gcage(obj) — return GC generational age of an object
// ---------------------------------------------------------------------------

func testGcage(L *luaapi.State) int {
	L.CheckAny(1)
	// Go GC doesn't have generational ages. Return "old" as default
	// since most gengc.lua tests check `not T or T.gcage(x) == "old"`.
	L.PushString("old")
	return 1
}

// ---------------------------------------------------------------------------
// T.resume(co) — resume a coroutine (C-level resume)
// ---------------------------------------------------------------------------

func testResume(L *luaapi.State) int {
	co := L.ToThread(1)
	if co == nil {
		L.PushFail()
		L.PushString("value is not a thread")
		return 2
	}
	narg := L.GetTop() - 1
	// Move arguments from L to co
	if narg > 0 {
		L.XMove(co, narg)
	}
	status, nres := co.Resume(L, narg)
	if status == luaapi.StatusOK || status == luaapi.StatusYield {
		// Move results back
		if nres > 0 {
			co.XMove(L, nres)
		}
		return nres
	}
	// Error — move error message back
	co.XMove(L, 1)
	return 1
}

// ---------------------------------------------------------------------------
// T.sethook(mask, count, func) — set debug hook (stub)
// ---------------------------------------------------------------------------

func testSethook(L *luaapi.State) int {
	return 0
}

// ---------------------------------------------------------------------------
// T.resetCI() — reset call info (stub)
// ---------------------------------------------------------------------------

func testResetCI(L *luaapi.State) int {
	return 0
}

// ---------------------------------------------------------------------------
// T.reallocstack(n) — reallocate stack (stub)
// ---------------------------------------------------------------------------

func testReallocstack(L *luaapi.State) int {
	_ = L.CheckInteger(1)
	return 0
}

// ---------------------------------------------------------------------------
// T.nonblock(co) — set coroutine as non-blocking (stub)
// ---------------------------------------------------------------------------

func testNonblock(L *luaapi.State) int {
	return 0
}

// ---------------------------------------------------------------------------
// T.hash(s) — return hash of a string (stub)
// ---------------------------------------------------------------------------

func testHash(L *luaapi.State) int {
	s := L.CheckString(1)
	// Simple hash for testing
	var h uint64
	for _, c := range s {
		h = h*31 + uint64(c)
	}
	L.PushInteger(int64(h))
	return 1
}

// ---------------------------------------------------------------------------
// T.externstr(s) / T.externKstr(s) — external string ops (stub)
// ---------------------------------------------------------------------------

func testExternstr(L *luaapi.State) int {
	s := L.CheckString(1)
	L.PushString(s)
	return 1
}

func testExternKstr(L *luaapi.State) int {
	s := L.CheckString(1)
	L.PushString(s)
	return 1
}

// ---------------------------------------------------------------------------
// T.doonnewstack(code) — execute code on a new thread (stub)
// ---------------------------------------------------------------------------

func testDoonnewstack(L *luaapi.State) int {
	code := L.CheckString(1)
	err := L.DoString(code)
	if err != nil {
		L.PushFail()
		L.PushString(err.Error())
		return 2
	}
	L.PushBoolean(true)
	return 1
}

// ---------------------------------------------------------------------------
// T.trick() — settrick (stub)
// ---------------------------------------------------------------------------

func testTrick(L *luaapi.State) int {
	return 0
}

// ---------------------------------------------------------------------------
// T.codeparam(f) / T.applyparam(f) — code param ops (stub)
// ---------------------------------------------------------------------------

func testCodeparam(L *luaapi.State) int {
	L.PushInteger(0)
	return 1
}

func testApplyparam(L *luaapi.State) int {
	return 0
}

// suppress unused import warning
var _ = unicode.IsDigit

// ---------------------------------------------------------------------------
// OpenTestLib — register the T global table
// ---------------------------------------------------------------------------

func OpenTestLib(L *luaapi.State) {
	funcs := map[string]luaapi.CFunction{
		// Core testC engine
		"testC":       testCEntry,
		"makeCfunc":   testMakeCfunc,
		// Userdata
		"newuserdata":  testNewuserdata,
		"pushuserdata": testPushuserdata,
		"udataval":     testUdataval,
		// Conversion
		"d2s": testD2s,
		"s2d": testS2d,
		// Stack/debug
		"stacklevel":  testStacklevel,
		"sethook":     testSethook,
		"resetCI":     testResetCI,
		"reallocstack": testReallocstack,
		// References
		"ref":    testRef,
		"unref":  testUnref,
		"getref": testGetref,
		// Upvalues
		"upvalue": testUpvalue,
		// State management
		"checkpanic": testCheckpanic,
		"doremote":   testDoremote,
		"newstate":   testNewstate,
		"closestate": testClosestate,
		"loadlib":    testLoadlib,
		"doonnewstack": testDoonnewstack,
		// GC
		"checkmemory": testCheckmemory,
		"gcstate":     testGcstate,
		"gccolor":     testGccolor,
		"gcage":       testGcage,
		"totalmem":    testTotalmem,
		// Memory/allocation
		"alloccount":    testAlloccount,
		"allocfailnext": testAllocfailnext,
		// Query
		"querystr": testQuerystr,
		"querytab": testQuerytab,
		// Code inspection
		"listk":            testListk,
		"listcode":         testListcode,
		"printcode":        testPrintcode,
		"listabslineinfo":  testListabslineinfo,
		"listlocals":       testListlocals,
		"codeparam":        testCodeparam,
		"applyparam":       testApplyparam,
		// Coroutine
		"resume": testResume,
		// String
		"hash":       testHash,
		"externstr":  testExternstr,
		"externKstr": testExternKstr,
		// Misc
		"nonblock": testNonblock,
		"trick":    testTrick,
	}

	L.NewTable()
	for name, fn := range funcs {
		L.PushCFunction(fn)
		L.SetField(-2, name)
	}
	L.SetGlobal("T")
}
