package lobject

/*
** Common header for all GC objects
** Embedded in each GC type
 */
type CommonHeader struct {
	Next  *GCObject
	Tt    LuByte
	Marked LuByte
}

/*
** Node for hash tables: key-value pair plus next pointer for chaining
 */
type Node struct {
	IVal TValue  // value as a proper TValue (direct access)
	Key  NodeKey // key info (fields for value, key type, next, key value)
}

type NodeKey struct {
	Value Value // key value
	Tt    LuByte // key type
	Next  int    // next node index in chain (0 = end of chain)
}

/*
** Table type
 */
type Table struct {
	CommonHeader
	Flags     LuByte  // 1<<p means tagmethod(p) is not present
	Lsizenode LuByte  // log2 of number of slots of node array
	Asize     uint32  // number of slots in array part
	Array     []TValue // array part - TValue with type tags for proper Get/Set
	Node      []Node  // hash part (slice of nodes)
	Metatable *Table  // metatable
	Gclist    *GCObject // GC list
}

/*
** UpVal type
 */
type UpVal struct {
	CommonHeader
	V *Value  // points to stack or to its own value
	U UpValU // union for open/closed state
}

type UpValU struct {
	Open  UpValOpen // when open
	Value TValue    // when closed
}

type UpValOpen struct {
	Next *UpVal  // linked list
	Prev **UpVal // pointer to Next pointer
}

/*
** Lua Closure
 */
type LClosure struct {
	CommonHeader
	Nupvalues LuByte
	Gclist    *GCObject
	P         *Proto
	Upvals    []*UpVal // list of upvalues
}

/*
** C Closure
 */
type CClosure struct {
	CommonHeader
	Nupvalues LuByte
	Gclist    *GCObject
	F         LuaCFunction
	Upvalue   [1]TValue // list of upvalues (flexible array)
}

/*
** Closure union
 */
type Closure struct {
	C CClosure // C closure
	L LClosure // Lua closure
}

/*
** Prototype (function definition)
 */
type Proto struct {
	CommonHeader
	Numparams       LuByte
	Flag            LuByte
	Maxstacksize    LuByte
	Sizeupvalues    int
	Sizek           int
	Sizecode        int
	Sizelineinfo    int
	Sizep           int
	Sizelocvars     int
	Sizeabslineinfo int
	Linedefined     int
	Lastlinedefined int
	K               []TValue      // constants used by the function
	Code            []uint32      // opcodes
	P               []*Proto      // functions defined inside the function
	Upvalues        []Upvaldesc   // upvalue information
	Lineinfo        []LsByte      // information about source lines
	Abslineinfo     []AbsLineInfo // absolute line information
	Locvars         []LocVar      // information about local variables
	Source          *TString      // used for debug information
	Gclist          *GCObject     // GC list
}

/*
** Upvaldesc
 */
type Upvaldesc struct {
	Name    *TString
	Instack LuByte
	Idx     LuByte
	Kind    LuByte
}

/*
** LocVar
 */
type LocVar struct {
	Varname   *TString
	Startpc   int
	Endpc     int
}

/*
** AbsLineInfo
 */
type AbsLineInfo struct {
	Pc   int
	Line int
}

/*
** Userdata
 */
type Udata struct {
	CommonHeader
	Nuvalue   uint16
	Len       uint64
	Metatable *Table
	Gclist    *GCObject
	Uv        []TValue // user values
}

/*
** Table macros
 */
func GNode(t *Table, i int) *Node {
	return &t.Node[i]
}

func GVal(n *Node) *TValue {
	return &n.IVal
}

func GNext(n *Node) int {
	return n.Key.Next
}

func SetGNext(n *Node, next int) {
	n.Key.Next = next
}

func KeyTT(n *Node) int {
	return int(n.Key.Tt)
}

func KeyVal(n *Node) *Value {
	return &n.Key.Value
}

func IsKeyNil(n *Node) bool {
	return KeyTT(n) == LUA_TNIL
}

func IsKeyInteger(n *Node) bool {
	return KeyTT(n) == LUA_VNUMINT
}

func KeyInt(n *Node) LuaInteger {
	return KeyVal(n).I
}

func IsKeyShrStr(n *Node) bool {
	return KeyTT(n) == CTb(LUA_VSHRSTR)
}

func KeyStrVal(n *Node) *TString {
	return Gco2Ts(KeyVal(n).Gc)
}

func IsKeyDead(n *Node) bool {
	return KeyTT(n) == CTb(LUA_TDEADKEY)
}

func SetDeadKey(n *Node) {
	n.Key.Tt = LuByte(CTb(LUA_TDEADKEY))
}

/*
** Size of hash table (power of 2)
 */
func SizeNode(t *Table) int {
	return 1 << int(t.Lsizenode)
}

/*
** Module operation for hashing (size is always power of 2)
 */
func LMod(s uint64, size int) int {
	return int(s & uint64(size-1))
}

/*
** Power of 2
 */
func TwoTo(x int) int {
	return 1 << x
}
