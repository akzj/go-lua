package lobject

import (
	"math"
	"unsafe"
)

/*
** Basic types
 */
type LuByte = uint8
type LsByte = int8
type LUInt32 = uint32
type LuaInteger = int64
type LuaNumber = float64
type LuaUnsigned = uint64

/*
** Function types
 */
type LuaCFunction func(*LuaState) int
type LuaKFunction func(*LuaState, int, LuaInteger) int
type LuaAlloc func(ud interface{}, ptr interface{}, osize uint64, nsize uint64) interface{}
type LuaHook func(*LuaState, *Debug)

/*
** Memory types
 */
type LMem = int64
type LUMem = uint64

/*
** Thread status
 */
type TStatus = uint8

/*
** TMS - Tag Method enum
 */
type TMS int

/*
** Debug info
 */
type Debug struct {
	Event      int
	Name       string
	NameWhat   string
	What       string
	Source     string
	SrcLen     uint64
	Line       int
	LineDef    int
	LastLine   int
	Nups       uint8
	Nparams    uint8
	Isvararg   bool
	ExtraArgs  uint8
	IsTailCall bool
	FTransfer  int
	NTransfer  int
}

/*
** Common header for GC objects
 */
type GCObject struct {
	Next   *GCObject
	Tt     LuByte
	Marked LuByte
}

/*
** Value union - all possible Lua values
 */
type Value struct {
	Gc *GCObject   // collectable objects
	P  interface{}  // light userdata
	F  LuaCFunction // light C functions
	I  LuaInteger   // integer numbers
	N  LuaNumber    // float numbers
}

/*
** Tagged Value - core value type with explicit type tag
 */
type TValue struct {
	Value_ Value
	Tt_    LuByte
}

/*
** GC macros
 */
const BIT_ISCOLLECTABLE = 1 << 6

func IsCollectable(o *TValue) bool { return (o.Tt_ & BIT_ISCOLLECTABLE) != 0 }
func GcValue(o *TValue) *GCObject { return o.Value_.Gc }
func CTb(t int) int { return t | BIT_ISCOLLECTABLE }

/*
** Type tag accessors
 */
func RawTT(o *TValue) int  { return int(o.Tt_ & 0x0F) }
func TypeTag(o *TValue) int { return int(o.Tt_ & 0x3F) }
func TType(o *TValue) int { return int(o.Tt_ & 0x0F) }
func CheckTag(o *TValue, t int) bool { return int(o.Tt_) == t }
func CheckType(o *TValue, t int) bool { return TType(o) == t }

/*
** Type tests
 */
func TtIsNil(o *TValue) bool           { return CheckType(o, LUA_TNIL) }
func TtIsBoolean(o *TValue) bool      { return CheckType(o, LUA_TBOOLEAN) }
func TtIsNumber(o *TValue) bool       { return CheckType(o, LUA_TNUMBER) }
func TtIsTable(o *TValue) bool        { return CheckTag(o, CTb(LUA_TTABLE)) }
func TtIsFunction(o *TValue) bool     { return CheckType(o, LUA_TFUNCTION) }
func TtIsUserdata(o *TValue) bool     { return CheckType(o, LUA_TUSERDATA) }
func TtIsThread(o *TValue) bool       { return CheckTag(o, CTb(LUA_VTHREAD)) }
func TtIsLightUserdata(o *TValue) bool { return CheckTag(o, LUA_VLIGHTUSERDATA) }
func TtIsInteger(o *TValue) bool      { return CheckTag(o, LUA_VNUMINT) }
func TtIsFloat(o *TValue) bool        { return CheckTag(o, LUA_VNUMFLT) }
func TtIsFalse(o *TValue) bool        { return CheckTag(o, LUA_VFALSE) }
func TtIsTrue(o *TValue) bool         { return CheckTag(o, LUA_VTRUE) }
func TtIsShrString(o *TValue) bool    { return CheckTag(o, CTb(LUA_VSHRSTR)) }
func TtIsLngString(o *TValue) bool    { return CheckTag(o, CTb(LUA_VLNGSTR)) }
func TtIsLClosure(o *TValue) bool     { return CheckTag(o, CTb(LUA_VLCL)) }
func TtIsCClosure(o *TValue) bool     { return CheckTag(o, CTb(LUA_VCCL)) }
func TtIsLcf(o *TValue) bool          { return CheckTag(o, LUA_VLCF) }
func TtIsUpval(o *TValue) bool        { return CheckTag(o, CTb(LUA_VUPVAL)) }
func TtIsFullUserdata(o *TValue) bool { return CheckTag(o, CTb(LUA_VUSERDATA)) }
func TtIsClosure(o *TValue) bool      { return TtIsLClosure(o) || TtIsCClosure(o) }
func TtIsString(o *TValue) bool       { return TtIsShrString(o) || TtIsLngString(o) }

/*
** Value setters
 */
func SetNilValue(o *TValue)              { o.Tt_ = LuByte(LUA_VNIL) }
func SetFltValue(o *TValue, n LuaNumber) { o.Value_.N = n; o.Tt_ = LuByte(LUA_VNUMFLT) }
func SetIntValue(o *TValue, i LuaInteger) { o.Value_.I = i; o.Tt_ = LuByte(LUA_VNUMINT) }
func SetPValue(o *TValue, p interface{})  { o.Value_.P = p; o.Tt_ = LuByte(LUA_VLIGHTUSERDATA) }
func SetFValue(o *TValue, f LuaCFunction) { o.Value_.F = f; o.Tt_ = LuByte(LUA_VLCF) }
func SetBtValue(o *TValue, b bool) {
	if b { o.Tt_ = LuByte(LUA_VTRUE) } else { o.Tt_ = LuByte(LUA_VFALSE) }
}

/*
** Value getters
 */
func FltValue(o *TValue) LuaNumber    { return o.Value_.N }
func IntValue(o *TValue) LuaInteger   { return o.Value_.I }
func PValue(o *TValue) interface{}   { return o.Value_.P }
func FValue(o *TValue) LuaCFunction  { return o.Value_.F }
func GcValue2(o *TValue) *GCObject   { return o.Value_.Gc }

/*
** Utility functions
 */
func IsFalse(o *TValue) bool   { return TtIsFalse(o) || TtIsNil(o) }
func NumEq(a, b LuaNumber) bool { return a == b || (math.IsNaN(a) && math.IsNaN(b)) }
func NumIsNaN(n LuaNumber) bool  { return math.IsNaN(n) }

/*
** Copy value from one TValue to another
 */
func SetObj(obj1, obj2 *TValue) {
	obj1.Value_ = obj2.Value_
	obj1.Tt_ = obj2.Tt_
}

/*
** GCObject to typed object conversions
 */
func Gco2Ts(o *GCObject) *TString  { return (*TString)(unsafe.Pointer(o)) }
func Gco2U(o *GCObject) *Udata     { return (*Udata)(unsafe.Pointer(o)) }
func Gco2Lcl(o *GCObject) *LClosure { return (*LClosure)(unsafe.Pointer(o)) }
func Gco2Ccl(o *GCObject) *CClosure { return (*CClosure)(unsafe.Pointer(o)) }
func Gco2T(o *GCObject) *Table     { return (*Table)(unsafe.Pointer(o)) }
func Gco2P(o *GCObject) *Proto     { return (*Proto)(unsafe.Pointer(o)) }
func Gco2Th(o *GCObject) *LuaState { return (*LuaState)(unsafe.Pointer(o)) }
func Gco2Upv(o *GCObject) *UpVal   { return (*UpVal)(unsafe.Pointer(o)) }

/*
** Stack Value - for stack slots
 */
type StackValue struct {
	Val TValue
}

/*
** StkId - index to stack element
 */
type StkId *TValue

/*
** Convert StackValue to TValue pointer
 */
func S2V(o *StackValue) *TValue { return &o.Val }

/*
** Extra space per thread
 */
const LUA_EXTRASPACE = 0

/*
** Forward declaration for LuaState - defined in lstate package
 */
type LuaState struct{}

/*
** Type name lookup
 */
func TTypeName(t int) string {
	if t < len(typeNames) {
		return typeNames[t+1]
	}
	return "?"
}

var typeNames = [...]string{
	"no value",
	"nil",
	"boolean",
	"userdata",
	"number",
	"string",
	"table",
	"function",
	"userdata",
	"thread",
}
