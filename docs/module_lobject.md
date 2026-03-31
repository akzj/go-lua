# lobject 模块规格书

## 模块职责

定义所有 Lua 值的类型系统，包括 TValue tagged union、GCObject 基类、以及所有 Lua 内建类型的结构体定义。这是整个实现的基础。

## 依赖模块

无依赖（最底层模块）

## 核心数据结构

### 1. TValue (Tagged Union)

```c
// C 实现
typedef union Value {
  struct GCObject *gc;    // 可回收对象
  void *p;               // light userdata
  lua_CFunction f;       // light C function
  lua_Integer i;         // 整数
  lua_Number n;          // 浮点数
} Value;

typedef struct TValue {
  Value value_;
  lu_byte tt_;           // type tag
} TValue;
```

### 2. 类型标签

```c
// 基础类型
#define LUA_TNIL           0
#define LUA_TBOOLEAN       1
#define LUA_TLIGHTUSERDATA 2
#define LUA_TNUMBER        3
#define LUA_TSTRING        4
#define LUA_TTABLE         5
#define LUA_TFUNCTION      6
#define LUA_TUSERDATA      7
#define LUA_TTHREAD        8
#define LUA_NUMTYPES       9

// 扩展类型
#define LUA_TUPVAL         9   // upvalues
#define LUA_TPROTO         10  // 函数原型
#define LUA_TDEADKEY       11  // 表中的死键

// Variant tags（类型 + 变体）
// makevariant(t, v) = (t) | ((v) << 4)
#define LUA_VNIL           makevariant(LUA_TNIL, 0)
#define LUA_VFALSE         makevariant(LUA_TBOOLEAN, 0)
#define LUA_VTRUE          makevariant(LUA_TBOOLEAN, 1)
#define LUA_VNUMINT        makevariant(LUA_TNUMBER, 0)   // 整数
#define LUA_VNUMFLT        makevariant(LUA_TNUMBER, 1)   // 浮点
#define LUA_VSHRSTR        makevariant(LUA_TSTRING, 0)   // 短字符串
#define LUA_VLNGSTR        makevariant(LUA_TSTRING, 1)   // 长字符串
#define LUA_VLCL           makevariant(LUA_TFUNCTION, 0) // Lua 闭包
#define LUA_VLCF           makevariant(LUA_TFUNCTION, 1) // light C function
#define LUA_VCCL           makevariant(LUA_TFUNCTION, 2) // C 闭包
#define LUA_VTABLE         makevariant(LUA_TTABLE, 0)
#define LUA_VTHREAD        makevariant(LUA_TTHREAD, 0)
#define LUA_VUPVAL         makevariant(LUA_TUPVAL, 0)
#define LUA_VPROTO         makevariant(LUA_TPROTO, 0)
#define LUA_VUSERDATA      makevariant(LUA_TUSERDATA, 0)
```

## Go 重写规格

```go
package lua

// LuaType 是类型标签
type LuaType uint8

const (
    LUA_TNIL           LuaType = 0
    LUA_TBOOLEAN       LuaType = 1
    LUA_TLIGHTUSERDATA LuaType = 2
    LUA_TNUMBER        LuaType = 3
    LUA_TSTRING        LuaType = 4
    LUA_TTABLE         LuaType = 5
    LUA_TFUNCTION      LuaType = 6
    LUA_TUSERDATA      LuaType = 7
    LUA_TTHREAD        LuaType = 8
    LUA_NUMTYPES       LuaType = 9
    LUA_TUPVAL         LuaType = 9
    LUA_TPROTO         LuaType = 10
    LUA_TDEADKEY       LuaType = 11
)

// Variant 类型
const (
    LUA_VNIL    LuaType = 0x00
    LUA_VFALSE  LuaType = 0x11  // BOOLEAN | (0 << 4)
    LUA_VTRUE   LuaType = 0x12  // BOOLEAN | (1 << 4)
    LUA_VNUMINT LuaType = 0x30  // NUMBER  | (0 << 4)
    LUA_VNUMFLT LuaType = 0x31  // NUMBER  | (1 << 4)
    LUA_VSHRSTR LuaType = 0x44  // STRING  | (0 << 4)
    LUA_VLNGSTR LuaType = 0x45  // STRING  | (1 << 4)
    LUA_VLCL    LuaType = 0x60  // FUNCTION| (0 << 4)
    LUA_VLCF    LuaType = 0x61  // FUNCTION| (1 << 4)
    LUA_VCCL    LuaType = 0x62  // FUNCTION| (2 << 4)
)

// GC 可回收标记
const BIT_ISCOLLECTABLE = uint8(1 << 6)

// TValue Go 实现
// ⚠️ 最终推荐方案：数值类型直接存储，GC 对象用 interface{}
type TValue struct {
    tt  LuaType
    i   int64           // integer value
    n   float64         // float value
    obj interface{}     // GC objects: string, table, closure, thread, userdata, proto
}

// 这样设计的好处：
// 1. int64/float64 不需要堆分配
// 2. GC 对象用 interface{}，Go GC 自动管理
// 3. 避免 union 的复杂性

// 工厂函数
func (v *TValue) SetNil() {
    v.tt = LUA_VNIL
}

func (v *TValue) SetBoolean(b bool) {
    v.tt = LUA_VTRUE
    if !b {
        v.tt = LUA_VFALSE
    }
}

func (v *TValue) SetInteger(i int64) {
    v.tt = LUA_VNUMINT
    v.i = i
}

func (v *TValue) SetFloat(n float64) {
    v.tt = LUA_VNUMFLT
    v.n = n
}

func (v *TValue) SetString(s *TString) {
    v.tt = LUA_VSHRSTR | BIT_ISCOLLECTABLE
    v.gc = uintptr(unsafe.Pointer(s))
}

func (v *TValue) SetTable(t *Table) {
    v.tt = LUA_VTABLE | BIT_ISCOLLECTABLE
    v.gc = uintptr(unsafe.Pointer(t))
}

func (v *TValue) SetClosure(cl *LClosure) {
    v.tt = LUA_VLCL | BIT_ISCOLLECTABLE
    v.gc = uintptr(unsafe.Pointer(cl))
}

func (v *TValue) SetCClosure(cl *CClosure) {
    v.tt = LUA_VCCL | BIT_ISCOLLECTABLE
    v.gc = uintptr(unsafe.Pointer(cl))
}

func (v *TValue) SetLightFunction(f GoFunction) {
    v.tt = LUA_VLCF
    v.f = f
}

// 类型判断
func (v *TValue) IsNil() bool           { return v.tt == LUA_VNIL }
func (v *TValue) IsBoolean() bool       { return v.tt == LUA_VFALSE || v.tt == LUA_VTRUE }
func (v *TValue) IsInteger() bool       { return v.tt == LUA_VNUMINT }
func (v *TValue) IsFloat() bool         { return v.tt == LUA_VNUMFLT }
func (v *TValue) IsNumber() bool        { return v.IsInteger() || v.IsFloat() }
func (v *TValue) IsString() bool        { return v.tt == LUA_VSHRSTR || v.tt == LUA_VLNGSTR }
func (v *TValue) IsTable() bool         { return v.tt == LUA_VTABLE }
func (v *TValue) IsFunction() bool      { return v.IsLClosure() || v.IsCClosure() || v.IsLightFunction() }
func (v *TValue) IsLClosure() bool      { return v.tt == LUA_VLCL }
func (v *TValue) IsCClosure() bool      { return v.tt == LUA_VCCL }
func (v *TValue) IsLightFunction() bool { return v.tt == LUA_VLCF }
func (v *TValue) IsThread() bool        { return v.tt == LUA_VTHREAD }
func (v *TValue) IsUserdata() bool      { return v.tt == LUA_VUSERDATA || v.tt == LUA_VLIGHTUSERDATA }
func (v *TValue) IsCollectable() bool   { return v.tt&BIT_ISCOLLECTABLE != 0 }

// 值获取
func (v *TValue) ToInteger() int64      { return v.i }
func (v *TValue) ToFloat() float64      { return v.n }
func (v *TValue) ToNumber() float64 {
    if v.IsInteger() {
        return float64(v.i)
    }
    return v.n
}
```

## GCObject 基类

```c
// CommonHeader 宏
#define CommonHeader  struct GCObject *next; lu_byte tt; lu_byte marked

typedef struct GCObject {
  CommonHeader;
} GCObject;
```

```go
// GCHeader 所有可回收对象的基类
type GCHeader struct {
    Next   *GCHeader  // GC 链表
    Tt     LuaType    // 类型
    Marked uint8      // 标记
}

// GCObject 接口
type GCObject interface {
    GetHeader() *GCHeader
    Mark()
}

// GC 颜色标记
const (
    WHITE0BIT uint8 = 1 << 0  // 白色0
    WHITE1BIT uint8 = 1 << 1  // 白色1
    GRAYBIT   uint8 = 1 << 2  // 灰色
    BLACKBIT  uint8 = 1 << 3  // 黑色
    FIXEDBIT  uint8 = 1 << 4  // 固定不回收
    FINALIZEDBIT uint8 = 1 << 5  // 已终结
    WHITEBITS = WHITE0BIT | WHITE1BIT
)

func (h *GCHeader) IsWhite() bool   { return h.Marked&WHITEBITS != 0 }
func (h *GCHeader) IsGray() bool   { return h.Marked&GRAYBIT != 0 }
func (h *GCHeader) IsBlack() bool  { return h.Marked&(WHITEBITS|GRAYBIT) == 0 }
func (h *GCHeader) SetWhite()     { h.Marked = (h.Marked & ^WHITEBITS) | WHITE0BIT }
func (h *GCHeader) SetGray()      { h.Marked |= GRAYBIT }
func (h *GCHeader) SetBlack()     { h.Marked &^= GRAYBIT }
```

## 公开 API (lobject.c)

```c
/* 运算 */
LUAI_FUNC int luaO_rawarith (lua_State *L, int op, const TValue *p1,
                             const TValue *p2, TValue *res);
LUAI_FUNC void luaO_arith (lua_State *L, int op, const TValue *p1,
                           const TValue *p2, StkId res);
LUAI_FUNC size_t luaO_str2num (const char *s, TValue *o);
LUAI_FUNC void luaO_tostring (lua_State *L, TValue *obj);

/* 字符串格式化 */
LUAI_FUNC const char *luaO_pushvfstring (lua_State *L, const char *fmt,
                                                       va_list argp);
LUAI_FUNC const char *luaO_pushfstring (lua_State *L, const char *fmt, ...);

/* 工具 */
LUAI_FUNC int luaO_utf8esc (char *buff, l_uint32 x);
LUAI_FUNC lu_byte luaO_ceillog2 (unsigned int x);
LUAI_FUNC void luaO_chunkid (char *out, const char *source, size_t srclen);
```

## 陷阱和注意事项

### 陷阱 1: Variant Tag 位运算

```c
// makevariant(t, v) = (t) | ((v) << 4)
// 提取变体：t & 0x0F
// 提取类型：(t >> 4) & 0x03
```

**Go 实现需要正确处理位运算**

### 陷阱 2: 浮点数 NaN 检查

```c
// 检查是否是 number（整数或浮点）
#define ttisnumber(o)    checktype(o, LUA_TNUMBER)

// 检查是否是浮点数
#define ttisfloat(o)    checktag(o, LUA_VNUMFLT)

// 检查是否是整数
#define ttisinteger(o)  checktag(o, LUA_VNUMINT)
```

### 陷阱 3: Collectable 对象

```c
// 可回收对象有额外的 GC 标记位
#define iscollectable(o)  (rawtt(o) & BIT_ISCOLLECTABLE)
```

**Go 中需要跟踪哪些对象是 GC 对象**

### 陷阱 4: nil 的多种变体

```c
#define LUA_VNIL       makevariant(LUA_TNIL, 0)    // 标准 nil
#define LUA_VEMPTY     makevariant(LUA_TNIL, 1)    // 空槽
#define LUA_VABSTKEY   makevariant(LUA_TNIL, 2)    // 绝对不存在
#define LUA_VNOTABLE   makevariant(LUA_TNIL, 3)    // 快速路径失败
```

## 验证测试

```go
func TestTValue(t *testing.T) {
    L := NewLuaState()
    
    // nil
    L.PushNil()
    assert.True(t, L.stack[0].IsNil())
    
    // boolean
    L.PushBoolean(true)
    assert.True(t, L.stack[1].IsBoolean())
    assert.True(t, L.stack[1].ToBoolean())
    
    // integer
    L.PushInteger(42)
    assert.True(t, L.stack[2].IsInteger())
    assert.Equal(t, int64(42), L.stack[2].ToInteger())
    
    // float
    L.PushNumber(3.14)
    assert.True(t, L.stack[3].IsFloat())
    assert.InDelta(t, 3.14, L.stack[3].ToFloat(), 0.001)
    
    // string
    L.PushString("hello")
    assert.True(t, L.stack[4].IsString())
}
```