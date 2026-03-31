# lstring 模块规格书

## 模块职责

管理 Lua 字符串，包括字符串 interning（短字符串全局唯一）和长字符串管理。这是 Lua 性能的关键优化点。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lobject | TString, GCObject 定义 |
| lgc | GC 相关操作 |
| lmem | 内存分配 |
| lstate | 全局状态、seed |

## 公开 API

```c
/* 创建字符串 */
LUAI_FUNC TString *luaS_newlstr (lua_State *L, const char *str, size_t l);
LUAI_FUNC TString *luaS_new (lua_State *L, const char *str);
LUAI_FUNC TString *luaS_createlngstrobj (lua_State *L, size_t l);
LUAI_FUNC TString *luaS_newextlstr (lua_State *L, const char *s, size_t len,
                                    lua_Alloc falloc, void *ud);
LUAI_FUNC TString *luaS_normstr (lua_State *L, TString *ts);

/* 字符串操作 */
LUAI_FUNC unsigned luaS_hash (const char *str, size_t l, unsigned seed);
LUAI_FUNC unsigned luaS_hashlongstr (TString *ts);
LUAI_FUNC int luaS_eqstr (TString *a, TString *b);

/* 表管理 */
LUAI_FUNC void luaS_resize (lua_State *L, int nsize);
LUAI_FUNC void luaS_remove (lua_State *L, TString *ts);
LUAI_FUNC void luaS_init (lua_State *L);
LUAI_FUNC void luaS_clearcache (global_State *g);

/* Userdata */
LUAI_FUNC Udata *luaS_newudata (lua_State *L, size_t s, unsigned short nuvalue);
```

## 核心数据结构

### C 定义

```c
/* 字符串表 */
typedef struct stringtable {
  TString **hash;    /* 哈希桶数组 */
  int nuse;          /* 已用数量 */
  int size;          /* 桶数量 */
} stringtable;

/* 字符串结构 */
typedef struct TString {
  CommonHeader;
  lu_byte extra;    /* 短字符串：hash 标记；长字符串：是否有 hash */
  ls_byte shrlen;   /* 短字符串：长度(>=0)；长字符串：类型(<0) */
  unsigned int hash;
  union {
    size_t lnglen;   /* 长字符串长度 */
    struct TString *hnext;  /* 哈希链表 */
  } u;
  char *contents;    /* 长字符串内容指针 */
  lua_Alloc falloc; /* 外部字符串释放函数 */
  void *ud;         /* 用户数据 */
} TString;

/* 短字符串长度限制 */
#define LUAI_MAXSHORTLEN   40

/* 长字符串类型 */
#define LSTRREG    -1  /* 常规长字符串 */
#define LSTRFIX    -2  /* 固定外部字符串 */
#define LSTRMEM    -3  /* 需要释放的外部字符串 */
```

### 关键宏

```c
#define strisshr(ts)    ((ts)->shrlen >= 0)
#define isextstr(ts)    (ttislngstring(ts) && tsvalue(ts)->shrlen != LSTRREG)

/* 获取字符串内容 */
#define rawgetshrstr(ts)  (cast_charp(&(ts)->contents))
#define getshrstr(ts)     check_exp(strisshr(ts), rawgetshrstr(ts))
#define getlngstr(ts)     check_exp(!strisshr(ts), (ts)->contents)
#define getstr(ts)        (strisshr(ts) ? rawgetshrstr(ts) : (ts)->contents)

/* 长度 */
#define tsslen(ts)     (strisshr(ts) ? cast_sizet((ts)->shrlen) : (ts)->u.lnglen)
#define getlstr(ts,len) \
    (strisshr(ts) \
        ? (cast_void((len) = cast_sizet((ts)->shrlen)), rawgetshrstr(ts)) \
        : (cast_void((len) = (ts)->u.lnglen), (ts)->contents))
```

## Go 重写规格

### 类型定义

```go
package lua

// StringTable 字符串 interning 表
type StringTable struct {
    Hash [][]*TString  // 桶数组（链表解决冲突）
    Nuse int           // 已用数量
    Size int           // 桶数量
}

// TString Lua 字符串
type TString struct {
    GCHeader
    Extra   uint8      // 短字符串：hash 标记
    Shrlen  int8       // >=0: 短字符串长度; <0: 长字符串类型
    Hash    uint32     // hash 值
    
    // 长字符串专用
    Contents string    // Go string 存储
    
    // 外部字符串
    Falloc luaGoAlloc  // 释放函数
    UserData unsafe.Pointer
}

// API
func (ts *TString) IsShort() bool { return ts.Shrlen >= 0 }
func (ts *TString) Len() int {
    if ts.IsShort() {
        return int(ts.Shrlen)
    }
    return len(ts.Contents)
}

// API
func (ts *TString) String() string {
    if ts.IsShort() {
        // 短字符串存储在 contents[0] 开始的位置
        // 需要特殊处理
        return ts.getShortStr()
    }
    return ts.Contents
}
```

### 短字符串 Interning

**核心算法：**

```go
// internshrstr: 短字符串 interning
func (L *LuaState) InternShortString(str string) *TString {
    g := L.G
    h := HashString(str, g.Seed)
    size := g.Strt.Size
    
    // 查找桶
    bucket := int(h) % size
    for ts := g.Strt.Hash[bucket]; ts != nil; ts = ts.Next {
        if ts.IsShort() && ts.Len() == len(str) {
            if ts.String() == str {
                // 找到
                if ts.IsDead() {
                    ts.Resurrect()  // 复活
                }
                return ts
            }
        }
    }
    
    // 没找到，创建新的
    if g.Strt.Nuse >= size {
        L.GrowStringTable()
        bucket = int(h) % g.Strt.Size
    }
    
    ts := L.NewShortString(str, h)
    // 添加到桶
    ts.Next = g.Strt.Hash[bucket]
    g.Strt.Hash[bucket] = ts
    g.Strt.Nuse++
    
    return ts
}

// luaS_new: 创建或复用字符串
func (L *LuaState) NewString(str string) *TString {
    if len(str) <= LUAI_MAXSHORTLEN {
        return L.InternShortString(str)
    }
    return L.NewLongString(str)
}
```

### 长字符串

长字符串不 intern，每次都创建新对象：

```go
// luaS_newlstr: 创建任意长度字符串
func (L *LuaState) NewLongString(str string) *TString {
    ts := &TString{
        GCHeader: L.GC.NewGCObject(LUA_VLNGSTR),
        Shrlen:   LSTRREG,  // 标记为常规长字符串
        Hash:     L.G.Seed,
        Contents: str,
    }
    return ts
}

// 外部字符串
func (L *LuaState) NewExternalString(data []byte, alloc func(unsafe.Pointer, int) unsafe.Pointer) *TString {
    ts := &TString{
        GCHeader: L.GC.NewGCObject(LUA_VLNGSTR),
        Shrlen:   LSTRMEM,
        Falloc:   alloc,
        Contents: string(data),
    }
    return ts
}
```

### 哈希算法

```go
// luaS_hash
func HashString(str string, seed uint32) uint32 {
    h := seed ^ uint32(len(str))
    
    // Lua 5.5 使用的哈希算法
    // 一个变形的 FNV-1a
    for i := 0; i < len(str); i++ {
        c := uint32(str[i])
        h ^= c
        h += (h << 1) + (h << 4) + (h << 5) + (h << 8) + (h << 13)
        h ^= (h >> 6)
    }
    h ^= (h << 17)
    h ^= (h >> 5)
    h ^= (h >> 3)
    
    // 确保非零
    if h == 0 {
        h = 1
    }
    return h
}
```

### 表重哈希

```go
// luaS_resize: 调整字符串表大小
func (L *LuaState) ResizeStringTable(newSize int) {
    g := L.G
    oldTable := g.Strt
    
    // 创建新表
    newTable := &StringTable{
        Hash: make([][]*TString, newSize),
        Size: newSize,
    }
    
    // 重新分配已有字符串
    for _, bucket := range oldTable.Hash {
        for ts := bucket; ts != nil; ts = next := ts.Next {
            next = ts.Next
            // 重新计算桶位置
            newBucket := int(ts.Hash) % newSize
            ts.Next = newTable.Hash[newBucket]
            newTable.Hash[newBucket] = ts
        }
    }
    
    g.Strt = *newTable
}

// 负载因子：nuse > size 时扩容
func (L *LuaState) GrowStringTable() {
    if L.G.Strt.Size <= MAX_STRTB_SIZE / 2 {
        L.ResizeStringTable(L.G.Strt.Size * 2)
    }
}
```

### 字符串比较

```go
// luaS_eqstr: 字符串相等比较
// 关键：短字符串可以直接比较指针（因为 interned）
func (L *LuaState) EqString(a, b *TString) bool {
    if a == b {
        return true  // 同一对象，肯定相等
    }
    if a.IsShort() && b.IsShort() {
        return false  // interned 后不同指针肯定不等
    }
    // 长字符串需要逐字节比较
    if a.Len() != b.Len() {
        return false
    }
    return a.String() == b.String()
}
```

### API 缓存

API 中频繁使用的字符串被缓存：

```go
// 全局状态中的字符串缓存
type globalState struct {
    // ...
    StrCache [STRCACHE_N][STRCACHE_M] *TString
}

// luaS_new 带缓存（用于 C API）
func (L *LuaState) NewStringCached(str string) *TString {
    g := L.G
    i := int(uintptr(unsafe.Pointer(stringData(str)))) % STRCACHE_N
    
    // 先查缓存
    for j := 0; j < STRCACHE_M; j++ {
        if g.StrCache[i][j].String() == str {
            return g.StrCache[i][j]
        }
    }
    
    // 缓存未命中，创建新字符串
    ts := L.NewString(str)
    
    // 移动缓存
    for j := STRCACHE_M - 1; j > 0; j-- {
        g.StrCache[i][j] = g.StrCache[i][j-1]
    }
    g.StrCache[i][0] = ts
    
    return ts
}
```

## 陷阱和注意事项

### 陷阱 1: 短字符串 vs 长字符串边界

```c
// C 中边界是 LUAI_MAXSHORTLEN = 40
if (l <= LUAI_MAXSHORTLEN)
    return internshrstr(L, str, l);  // intern
else
    return luaS_createlngstrobj(L, l);  // 不 intern
```

**Go 实现：**
```go
const MaxShortString = 40

func (L *LuaState) NewString(str string) *TString {
    if len(str) <= MaxShortString {
        return L.InternShortString(str)
    }
    return L.NewLongString(str)
}
```

### 陷阱 2: GC 时的字符串处理

```go
// GC 不会回收 interned 短字符串（除非明确删除）
// 但会收集长字符串

func (g *GCState) SweepString() {
    for i := 0; i < g.Strt.Size; i++ {
        prev := &g.Strt.Hash[i]
        for ts := *prev; ts != nil; {
            if ts.IsDead() && !ts.IsFixed() {
                // 回收
                *prev = ts.Next
                g.Strt.Nuse--
                g.FreeObject(ts)
            } else {
                prev = &ts.Next
            }
        }
    }
}
```

### 陷阱 3: 字符串内容存储

短字符串和长字符串存储方式不同：

```c
// 短字符串：存储在 TString 结构体末尾
typedef struct TString {
    // ...
    char contents[1];  // 变长
} TString;

// 长字符串：contents 指向外部
typedef struct TString {
    // ...
    char *contents;  // 指向外部
} TString;
```

**Go 实现：**
```go
// 短字符串
type ShortString struct {
    TString
    Len int8
    // Go string 存储在 Contents 中（自动 interned）
    // 但为了兼容 Lua 语义，可能需要特殊处理
}

// 长字符串
type LongString struct {
    TString
    // Contents 直接用 Go string
}

// 或者统一用 Go string，短字符串intern
type TString struct {
    GCHeader
    Extra   uint8
    Shrlen  int8   // >=0: 短字符串
    Hash    uint32
    // 长字符串长度（如果是长字符串）
    // 短字符串用 Shrlen 存储
    Contents string  // Go string
}
```

## 验证测试

```lua
-- strings.lua 关键测试

-- 短字符串 interning
local s1 = "hello"
local s2 = "hello"
assert(s1 == s2)  -- 同一对象
assert(string.len(s1) == 5)

-- 长字符串（不 intern）
local long1 = string.rep("a", 100)
local long2 = string.rep("a", 100)
assert(long1 == long2)  -- 值相等
-- 可能不是同一对象

-- 字符串操作
assert("hello" .. " world" == "hello world")
assert(#"hello" == 5)
assert(string.sub("hello", 1, 3) == "hel")
assert(string.rep("ab", 3) == "ababab")

-- 字符串比较
assert("abc" < "abd")
assert("abc" == "abc")
assert("abc" ~= "abd")

-- 模式匹配
assert(string.match("hello", "%a+") == "hello")
assert(string.gsub("test", "e", "a") == "tast")
```

### 性能测试

```lua
-- 字符串性能测试
local function bench_intern()
    local start = os.clock()
    for i = 1, 100000 do
        local s = "hello world test string"
        if s ~= "hello world test string" then error("fail") end
    end
    return os.clock() - start
end

print("intern string:", bench_intern(), "seconds")

-- 模式匹配性能
local function bench_pattern()
    local s = "hello 123 world 456 test"
    local start = os.clock()
    for i = 1, 10000 do
        string.gsub(s, "%d+", function(n) return tonumber(n) * 2 end)
    end
    return os.clock() - start
end

print("pattern match:", bench_pattern(), "seconds")
```