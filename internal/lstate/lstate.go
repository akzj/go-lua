package lstate

/*
** $Id: lstate.go $
** Global State - Lua State, Global State, CallInfo
** Ported from lstate.h and lstate.c
*/

import (
	"unsafe"

	"github.com/akzj/go-lua/internal/lobject"
)

// Constants
const (
	EXTRA_STACK         = 5
	BASIC_STACK_SIZE    = 2 * lobject.LUA_MINSTACK
	MAX_CCMT           = 0xf << CIST_CCMT
)

// CallInfo status bits
const (
	CIST_NRESULTS = 0xff
	CIST_CCMT     = 8
	CIST_RECST    = 12
	CIST_C        = 1 << (CIST_RECST + 3)
	CIST_FRESH    = CIST_C << 1
	CIST_CLSRET   = CIST_FRESH << 1
	CIST_TBC      = CIST_CLSRET << 1
	CIST_OAH      = CIST_TBC << 1
	CIST_HOOKED   = CIST_OAH << 1
	CIST_YPCALL   = CIST_HOOKED << 1
	CIST_TAIL     = CIST_YPCALL << 1
	CIST_HOOKYIELD = CIST_TAIL << 1
	CIST_FIN      = CIST_HOOKYIELD << 1
)

// Status codes for CallInfo recover
const (
	RECST_OK    = 0
	RECST_YIELD = 1
	RECST_ERR   = 2
)

// StkIdRel - relative stack index
type StkIdRel struct {
	P      *lobject.TValue
	Offset int
}

// CallInfo - information about a function call
type CallInfo struct {
	F        StkIdRel  // function index in the stack
	Top      StkIdRel  // top for this function
	Previous *CallInfo // dynamic call link
	Next     *CallInfo // dynamic call link
	// Lua function fields
	L struct {
		SavedPC    *uint32 // saved program counter
		Trap       bool    // function is tracing lines/counts
		NExtraArgs int     // # of extra arguments in vararg functions
	}
	// C function fields
	C struct {
		K         lobject.LuaKFunction // continuation in case of yields
		OldErrFunc int                 // old error function index
		Ctx       lobject.LuaInteger   // context info in case of yields
	}
	// Status fields
	U2 struct {
		FuncIdx int // called-function index
		NYield  int // number of values yielded
		NRes    int // number of values returned
	}
	CallStatus uint32
}

// IsLua returns true if this call is running a Lua function
func (ci *CallInfo) IsLua() bool {
	return (ci.CallStatus & CIST_C) == 0
}

// IsLuacode returns true if call is running Lua code (not a hook)
func (ci *CallInfo) IsLuacode() bool {
	return (ci.CallStatus & (CIST_C | CIST_HOOKED)) == 0
}

// GlobalState - global state shared by all threads
type GlobalState struct {
	Frealloc     lobject.LuaAlloc       // memory allocator function
	Ud           interface{}            // auxiliary data for allocator
	GCtotal      lobject.LMem           // total bytes allocated
	GCdebt       lobject.LMem           // allocation debt
	GCmarked     lobject.LMem           // objects marked in current GC cycle
	Strt         lobject.StringTable    // string table
	LRegistry    lobject.TValue        // registry table
	NilValue     lobject.TValue         // a nil value
	Seed         uint32                // randomized seed for hashes
	GCParams     [6]uint8               // GC parameters
	CurrentWhite uint8                  // current white bit
	GCState      uint8                  // GC state
	GCKind       uint8                  // kind of GC running
	GCStopEm     uint8                  // stops emergency collections
	GCStp        uint8                  // control whether GC is running
	GCEmergency  uint8                  // true if emergency collection
	Allgc        *lobject.GCObject     // list of all collectable objects
	Sweepgc      *lobject.GCObject     // current position in sweep list
	Finobj       *lobject.GCObject     // list of objects with finalizers
	Gray         *lobject.GCObject     // list of gray objects
	GrayAgain    *lobject.GCObject     // objects to be traversed atomically
	Weak         *lobject.GCObject     // tables with weak values
	Ephemeron    *lobject.GCObject     // ephemeron tables
	Allweak      *lobject.GCObject     // all-weak tables
	TobefnZ      *lobject.GCObject     // userdata to be finalized
	Fixedgc      *lobject.GCObject    // objects not to be collected
	// generational collector fields
	Survival     *lobject.GCObject     // start of surviving objects
	Old1         *lobject.GCObject     // start of old1 objects
	ReallyOld    *lobject.GCObject     // objects old for more than one cycle
	FirstOld1    *lobject.GCObject     // first OLD1 object in list
	FinobjSur    *lobject.GCObject     // survival objects with finalizers
	FinobjOld1   *lobject.GCObject     // old1 objects with finalizers
	FinobjROld   *lobject.GCObject     // really old objects with finalizers
	Twups        *LuaState              // list of threads with open upvalues
	Panic        lobject.LuaCFunction   // panic function
	Memerrmsg    *lobject.TString       // message for memory errors
	Tmname       [lobject.TM_N]*lobject.TString // tag method names
	Mt           [lobject.LUA_NUMTYPES]*lobject.Table // metatables
	MainTh       MainThread             // main thread of this state
}

// MainThread - container for main thread
type MainThread struct {
	L LuaState
}

// LuaState - thread state
type LuaState struct {
	lobject.CommonHeader
	AllowHook     bool
	Status        lobject.TStatus
	Top           StkIdRel
	BaseCcalls    int
	G             *GlobalState
	Ci            *CallInfo
	StackLast     StkIdRel
	Stack         []lobject.StackValue
	OpenUpval     *lobject.UpVal
	TbcList       StkIdRel         // list of to-be-closed variables
	Gclist        *lobject.GCObject
	Twups         *LuaState
	ErrorJmp      *LuaLongjmp
	BaseCi        CallInfo
	Hook          lobject.LuaHook
	Errfunc       int
	NCcalls       uint32           // number of C calls
	Oldpc         int
	Nci           int              // number of items in 'ci' list
	BaseHookCount int
	HookCount     int
	HookMask      uint8
}

// Lua - thread type alias
type Lua = LuaState

// LuaLongjmp - error recovery structure
type LuaLongjmp struct {
	Previous *LuaLongjmp
	Status   lobject.TStatus
}

// LX - thread state with extra space
type LX struct {
	Extra [lobject.LUA_EXTRASPACE]byte
	L     LuaState
}

// Yieldable returns true if thread can yield
func Yieldable(L *LuaState) bool {
	return (L.NCcalls & 0xffff0000) == 0
}

// GetCcalls returns the number of recursive C calls
func GetCcalls(L *LuaState) int {
	return int(L.NCcalls & 0xffff)
}

// Incnny increments non-yieldable call counter
func Incnny(L *LuaState) {
	L.NCcalls += 0x10000
}

// Decnny decrements non-yieldable call counter
func Decnny(L *LuaState) {
	L.NCcalls -= 0x10000
}

// G returns the global state pointer
func G(L *LuaState) *GlobalState {
	return L.G
}

// StackSize returns remaining stack space
func StackSize(L *LuaState) int {
	return int(uintptr(unsafe.Pointer(L.StackLast.P)) - uintptr(unsafe.Pointer(&L.Stack[0].Val))) / int(unsafe.Sizeof(lobject.TValue{}))
}

// FromState extracts LX from LuaState
func FromState(L *LuaState) *LX {
	ptr := unsafe.Pointer(L)
	offset := unsafe.Offsetof(LX{}.L)
	return (*LX)(unsafe.Pointer(uintptr(ptr) - offset))
}

// MainThread returns the main thread from global state
func MainThreadPtr(g *GlobalState) *LuaState {
	return &g.MainTh.L
}

// Completestate checks if state is complete
func Completestate(g *GlobalState) bool {
	return !lobject.TtIsNil(&g.NilValue)
}

// Savestack saves stack position as offset
func Savestack(L *LuaState, pt *lobject.TValue) int {
	return int(uintptr(unsafe.Pointer(pt)) - uintptr(unsafe.Pointer(&L.Stack[0].Val)))
}

// Restorestack restores stack position from offset
func Restorestack(L *LuaState, n int) *lobject.TValue {
	return (*lobject.TValue)(unsafe.Pointer(uintptr(unsafe.Pointer(&L.Stack[0].Val)) + uintptr(n)))
}

// Getnresults extracts number of expected results from call status
func Getnresults(cs uint32) int {
	return int(cs&CIST_NRESULTS) - 1
}

// Setcistrecst sets the recover status in CallInfo
func Setcistrecst(ci *CallInfo, st int) {
	ci.CallStatus = (ci.CallStatus & ^(uint32(7) << CIST_RECST)) | (uint32(st) << CIST_RECST)
}

// Getcistrecst gets the recover status from CallInfo
func Getcistrecst(ci *CallInfo) int {
	return int((ci.CallStatus >> CIST_RECST) & 7)
}

// Getoah gets the original allowhook value
func Getoah(ci *CallInfo) int {
	if ci.CallStatus&CIST_OAH != 0 {
		return 1
	}
	return 0
}

// Setoah sets the allowhook value
func Setoah(ci *CallInfo, v bool) {
	if v {
		ci.CallStatus |= CIST_OAH
	} else {
		ci.CallStatus &^= CIST_OAH
	}
}

// IsInTwups checks if thread is in twups list
func IsInTwups(L *LuaState) bool {
	return L.Twups != L
}
