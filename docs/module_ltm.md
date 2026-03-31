# ltm 模块规格书

## 模块职责

管理 Lua 的元方法（metamethod）查找和调用。元方法是 Lua 面向对象和操作符重载的基础。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lobject | TValue, TString |
| ltable | Table, 元表查找 |
| lstate | 全局状态 |
| lgc | GC |

## 元方法类型

```c
/* 元方法枚举 */
typedef enum {
  TM_INDEX,
  TM_NEWINDEX,
  TM_GC,
  TM_MODE,
  TM_LEN,
  TM_EQ,
  TM_ADD,
  TM_SUB,
  TM_MUL,
  TM_MOD,
  TM_POW,
  TM_DIV,
  TM_IDIV,
  TM_BAND,
  TM_BOR,
  TM_BXOR,
  TM_SHL,
  TM_SHR,
  TM_UNM,
  TM_BNOT,
  TM_LT,
  TM_LE,
  TM_CONCAT,
  TM_CALL,
  TM_CLOSE,    /* Lua 5.4+ to-be-closed */
  TM_N         /* 元方法数量 */
} TMS;
```

## 公开 API

```c
/* 元方法查找 */
LUAI_FUNC const TValue *luaT_gettm (Table *events, TMS event, TString *ename);
LUAI_FUNC const TValue *luaT_gettmbyobj (lua_State *L, const TValue *o, TMS event);
LUAI_FUNC const char *luaT_eventname (int event);

/* 元表操作 */
LUAI_FUNC void luaT_init (lua_State *L);

/* 元方法缓存 */
LUAI_FUNC void luaT_cachealert (lua_State *L, Table *t, TMS event);
LUAI_FUNC void luaT_checkcache (lua_State *L, TMS event);
```

## C 实现分析

```c
// 全局状态中的元方法名
#define luaT_eventname(event)  (G(L)->tmname[event])

// 查找元方法（带缓存）
const TValue *luaT_gettm (Table *events, TMS event, TString *ename) {
  const TValue *tm = luaH_getshortstr(events, ename);
  return tm;
}

// 按对象类型查找元方法
const TValue *luaT_gettmbyobj (lua_State *L, const TValue *o, TMS event) {
  Table *mt;
  switch (ttype(o)) {
    case LUA_TTABLE:
      mt = hvalue(o)->metatable;
      break;
    case LUA_TUSERDATA:
      mt = uvalue(o)->metatable;
      break;
    default:
      mt = G(L)->mt[ttype(o)];  // 基本类型的全局元表
  }
  return (mt ? luaH_getshortstr(mt, G(L)->tmname[event]) : NULL);
}

// 调用元方法（ldo.c 中）
void luaV_callTM (lua_State *L, const TValue *f, int nParams, int nResults) {
  /* 压入函数和参数，调用 */
  luaD_call(L, L->top - nParams - 1, nResults);
}
```

## Go 重写规格

```go
package lua

// 元方法类型
type TMS int

const (
    TM_INDEX    TMS = iota
    TM_NEWINDEX
    TM_GC
    TM_MODE
    TM_LEN
    TM_EQ
    TM_ADD
    TM_SUB
    TM_MUL
    TM_MOD
    TM_POW
    TM_DIV
    TM_IDIV
    TM_BAND
    TM_BOR
    TM_BXOR
    TM_SHL
    TM_SHR
    TM_UNM
    TM_BNOT
    TM_LT
    TM_LE
    TM_CONCAT
    TM_CALL
    TM_CLOSE
    TM_N
)

// GlobalState 元方法相关字段
type GlobalState struct {
    // ...
    TmName [TM_N]*TString  // 元方法名
    Mt     [LUA_NUMTYPES]*Table  // 基本类型的元表
}

// 获取元方法
func (L *LuaState) GetTmByObj(o *TValue, event TMS) *TValue {
    var mt *Table
    
    switch o.Type() {
    case LUA_TTABLE:
        mt = o.AsTable().Metatable
    case LUA_TUSERDATA:
        mt = o.AsUserdata().Metatable
    case LUA_TSTRING:
        mt = L.G.Mt[LUA_TSTRING]
    case LUA_TNUMBER:
        mt = L.G.Mt[LUA_TNUMBER]
    case LUA_TFUNCTION:
        mt = L.G.Mt[LUA_TFUNCTION]
    case LUA_TTHREAD:
        mt = L.G.Mt[LUA_TTHREAD]
    default:
        return nil
    }
    
    if mt == nil {
        return nil
    }
    
    // 从元表获取
    return mt.Get(L.G.TmName[event])
}

// 获取二元元方法（两个操作数）
func (L *LuaState) GetBinaryTm(a, b *TValue, event TMS) *TValue {
    // 先查第一个操作数的元表
    tm := L.GetTmByObj(a, event)
    if tm != nil && !tm.IsNil() {
        return tm
    }
    
    // 再查第二个操作数的元表
    return L.GetTmByObj(b, event)
}

// 调用元方法
func (L *LuaState) CallTM(f *TValue, a, b *TValue, nResults int) {
    L.Push(f)
    L.Push(a)
    L.Push(b)
    L.Call(2, nResults)
}

// 调用二元元方法
func (L *LuaState) CallBinTM(event TMS, a, b *TValue, res *TValue) bool {
    tm := L.GetBinaryTm(a, b, event)
    if tm == nil || tm.IsNil() {
        return false
    }
    
    // 调用元方法
    L.Push(tm)
    L.Push(a)
    L.Push(b)
    L.Call(2, 1)
    
    // 获取结果
    *res = L.stack[L.top-1]
    L.Pop(1)
    
    return true
}
```

## 元表缓存

```c
// Table 的 flags 字段缓存元方法存在性
// 每个 bit 表示对应的 TM 是否存在

#define TMMODE(table, event)    (maskflags & (table)->flags)
```

**Go 实现：**
```go
// Table 缓存元方法
type Table struct {
    // ...
    Flags uint8  // 元方法缓存
}

const (
    TM_INDEX_BIT    = 1 << 0
    TM_NEWINDEX_BIT = 1 << 1
    TM_LEN_BIT     = 1 << 2
    // ...
)

// 缓存检查
func (t *Table) HasTM(event TMS) bool {
    if t.Metatable == nil {
        return false
    }
    return t.Metatable.Get(G().TmName[event]) != nil
}

// 调用元方法前的快速检查
func (L *LuaState) TryFastIndex(obj *TValue, key *TValue) (*TValue, bool) {
    if !obj.IsTable() {
        return nil, false
    }
    
    t := obj.AsTable()
    
    // 检查是否有自定义 __index
    if t.Flags&TM_INDEX_BIT != 0 {
        return nil, false  // 需要调用 __index
    }
    
    // 快速路径：直接查表
    if v, ok := t.Get(key); ok {
        return &v, true
    }
    
    return nil, false
}
```

## 算术元方法

```go
// 算术元方法调度
func (L *LuaState) Arith(op int) {
    var a, b *TValue
    b = &L.stack[L.top-1]
    a = &L.stack[L.top-2]
    
    var event TMS
    switch op {
    case LUA_OPADD:  event = TM_ADD
    case LUA_OPSUB:  event = TM_SUB
    case LUA_OPMUL:  event = TM_MUL
    case LUA_OPDIV:  event = TM_DIV
    case LUA_OPIDIV: event = TM_IDIV
    case LUA_OPMOD:  event = TM_MOD
    case LUA_OPPOW:  event = TM_POW
    case LUA_OPBAND: event = TM_BAND
    case LUA_OPBOR:  event = TM_BOR
    case LUA_OPBXOR: event = TM_BXOR
    case LUA_OPSHL:  event = TM_SHL
    case LUA_OPSHR:  event = TM_SHR
    case LUA_OPUNM:  event = TM_UNM
    case LUA_OPBNOT: event = TM_BNOT
    }
    
    if tm := L.GetBinaryTm(a, b, event); tm != nil {
        // 调用元方法
        L.CallTM(tm, a, b, 1)
    } else {
        // 基础运算
        // ...
    }
}
```

## 陷阱和注意事项

### 陷阱 1: 元表继承链

```lua
-- 元表可以有自己的元表（继承）
local mt = {__add = function(a, b) return a + b end}
setmetatable(mt, {__add = function(a, b) return a * b end})
-- mt + mt 会调用哪个？
```

**Lua 5.3+ 行为：只查找直接元表，不递归**

### 陷阱 2: nil 和 boolean 的元表

```lua
-- nil 可以有元表
getmetatable(nil)  -- 返回 nil 或默认元表

-- boolean 的元表在 Lua 5.3+ 可以修改
setmetatable(true, {__add = function() return 42 end})
```

### 陷阱 3: 元方法缓存失效

```go
// 修改元表后需要清除缓存
func (t *Table) SetMetatable(mt *Table) {
    t.Metatable = mt
    t.Flags = 0  // 清除缓存
}
```

## 验证测试

```lua
-- 元方法测试
local mt = {
    __add = function(a, b)
        if type(a) == "table" and type(b) == "table" then
            local result = {}
            for k, v in pairs(a) do result[k] = v end
            for k, v in pairs(b) do result[k] = v end
            return result
        end
        return a + b
    end,
    __index = function(t, k)
        return "default " .. k
    end,
    __newindex = function(t, k, v)
        rawset(t, k, v)
    end
}

local t1 = setmetatable({a = 1}, mt)
local t2 = setmetatable({b = 2}, mt)
local t3 = t1 + t2
assert(t3.a == 1 and t3.b == 2)

-- 访问不存在的键
assert(t1.missing == "default missing")
```