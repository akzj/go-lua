# ltable 模块规格书

## 模块职责

Lua 表（table）是语言的核心数据结构，实现数组+哈希的混合结构。这是 Lua 中唯一的复合数据类型，用于实现数组、字典、对象等所有复杂数据结构。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lobject | TValue 类型定义 |
| lgc | GC 相关操作 |
| lmem | 内存分配 |
| lstring | 字符串 key 的哈希 |

## 公开 API

```c
/* 查找操作 */
LUAI_FUNC lu_byte luaH_get (Table *t, const TValue *key, TValue *res);
LUAI_FUNC lu_byte luaH_getshortstr (Table *t, TString *key, TValue *res);
LUAI_FUNC lu_byte luaH_getstr (Table *t, TString *key, TValue *res);
LUAI_FUNC lu_byte luaH_getint (Table *t, lua_Integer key, TValue *res);

/* 设置操作 */
LUAI_FUNC int luaH_psetint (Table *t, lua_Integer key, TValue *val);
LUAI_FUNC int luaH_psetshortstr (Table *t, TString *key, TValue *val);
LUAI_FUNC int luaH_psetstr (Table *t, TString *key, TValue *val);
LUAI_FUNC int luaH_pset (Table *t, const TValue *key, TValue *val);
LUAI_FUNC void luaH_setint (lua_State *L, Table *t, lua_Integer key, TValue *value);
LUAI_FUNC void luaH_set (lua_State *L, Table *t, const TValue *key, TValue *value);
LUAI_FUNC void luaH_finishset (lua_State *L, Table *t, const TValue *key,
                                TValue *value, int hres);

/* 创建和销毁 */
LUAI_FUNC Table *luaH_new (lua_State *L);
LUAI_FUNC void luaH_free (lua_State *L, Table *t);

/* 调整大小 */
LUAI_FUNC void luaH_resize (lua_State *L, Table *t, unsigned nasize, unsigned nhsize);
LUAI_FUNC void luaH_resizearray (lua_State *L, Table *t, unsigned nasize);

/* 迭代 */
LUAI_FUNC int luaH_next (lua_State *L, Table *t, StkId key);
LUAI_FUNC lua_Unsigned luaH_getn (lua_State *L, Table *t);

/* 统计 */
LUAI_FUNC lu_mem luaH_size (Table *t);

/* 元方法专用 */
LUAI_FUNC const TValue *luaH_Hgetshortstr (Table *t, TString *key);
```

## 核心数据结构

### C 定义

```c
/* 哈希节点：key-value 对 + 链表指针 */
typedef union Node {
  struct NodeKey {
    TValuefields;        /* value + tt */
    lu_byte key_tt;      /* key type tag */
    int next;            /* 冲突链表索引 */
    Value key_val;       /* key value */
  } u;
  TValue i_val;          /* 直接访问 value */
} Node;

/* 表结构 */
typedef struct Table {
  CommonHeader;
  lu_byte flags;         /* 元方法缓存标记 */
  lu_byte lsizenode;     /* log2(哈希大小) */
  unsigned int asize;    /* 数组部分大小 */
  Value *array;          /* 数组部分指针 */
  Node *node;            /* 哈希部分指针 */
  struct Table *metatable;
  GCObject *gclist;
} Table;
```

### 关键宏

```c
#define gnode(t,i)       (&(t)->node[i])
#define gval(n)          (&(n)->i_val)
#define gnext(n)         ((n)->u.next)

#define sizenode(t)      (twoto((t)->lsizenode))

/* 获取数组部分的 tag */
#define getArrTag(t,k)    (cast(lu_byte*, (t)->array) + sizeof(unsigned) + (k))
/* 获取数组部分的值 */
#define getArrVal(t,k)    ((t)->array - 1 - (k))

/* 主位置计算 */
static Node *mainposition (const Table *t, const TValue *key);
```

## Go 重写规格

### 类型定义

```go
package lua

// Table 是 Lua 的核心数据结构
type Table struct {
    Header     GCHeader
    Flags      uint8    // 元方法缓存
    LSizenode  uint8    // log2(哈希大小)
    Asize      uint32   // 数组大小
    Array      []TValue // 数组部分
    Node       []Node   // 哈希部分
    Metatable  *Table
    Gclist     *GCHeader
}

// Node 是哈希桶中的节点
type Node struct {
    Key  TValue  // key
    Val  TValue  // value
    Next int     // 冲突链表：下一个节点的索引，-1 表示结束
}

func (t *Table) Size() int {
    return 1 << t.LSizenode
}
```

### 数组+哈希混合结构详解

Lua 表使用两部分存储：

```
┌─────────────────────────────────────────────────────┐
│                     Table 结构                       │
├─────────────────────────────────────────────────────┤
│  Asize = 4, LSizenode = 2 (哈希大小 = 4)            │
├─────────────────────────────────────────────────────┤
│  Array 部分 (Value* 指向)          │  Node 部分     │
│  ┌─────┬─────┬─────┬─────┐        │  ┌─────┐      │
│  │ nil │  1  │  2  │ nil │        │  │ a:10 │ →    │
│  └─────┴─────┴─────┴─────┘        │  ├─────┤      │
│  索引:    1    2    3             │  │ b:20 │ →    │
│                                     │  ├─────┤      │
│  用于存储 1-based 正整数 key        │  │    │ nil │  │
│  访问 O(1)                          │  ├─────┤      │
│                                     │  │    │ nil │  │
│                                     │  └─────┘      │
└─────────────────────────────────────────────────────┘
```

### 关键实现细节

#### 1. 主位置计算

```go
func (t *Table) mainPosition(key *TValue) int {
    h := hashValue(key, t.G)
    size := t.Size()
    return int(h) & (size - 1)  // 2的幂次取模，等价于 h % size
}

// 哈希函数（来自 lvm.c）
func hashValue(key *TValue, g *globalState) uint {
    switch key.Type() {
    case LUA_TNUMBER:
        if key.IsInteger() {
            // 整数直接返回
            return uint(key.I)
        }
        // 浮点数用位表示做哈希
        return floatToBits(key.N)
    case LUA_TSTRING:
        return key.Str.Hash
    case LUA_TBOOLEAN:
        if key.B { return 1 }
        return 0
    case LUA_TNIL:
        return 0
    }
    // 其他类型用指针做哈希
    return uint(reflect.ValueOf(key.GC).Pointer())
}
```

#### 2. Get 操作

```go
// luaH_get: 通用 key 查找
func (t *Table) Get(key *TValue) (TValue, bool) {
    // 先检查数组部分（整数 key 1-based）
    if key.IsInteger() {
        idx := key.I - 1
        if idx >= 0 && idx < int(t.Asize) {
            if !t.Array[idx].IsNil() {
                return t.Array[idx], true
            }
        }
    }
    
    // 查哈希部分
    return t.getNode(key)
}

// getNode: 哈希查找，使用链式冲突解决
// ⚠️ 修正：Lua 5.5 使用链式冲突，不是线性探测
func (t *Table) getNode(key *TValue) (TValue, bool) {
    if t.Node == nil {
        return TValue{}, false
    }
    
    mp := t.mainPosition(key)
    n := &t.Node[mp]
    
    // 检查主位置
    if n.Key.IsNil() {
        return TValue{}, false
    }
    if equalValue(&n.Key, key) {
        return n.Val, true
    }
    
    // 沿冲突链遍历
    for n.Next != -1 {
        n = &t.Node[n.Next]
        if n.Key.IsNil() {
            return TValue{}, false
        }
        if equalValue(&n.Key, key) {
            return n.Val, true
        }
    }
    
    return TValue{}, false
}

// luaH_getint: 整数 key 专用
func (t *Table) GetInt(key int64) (TValue, bool) {
    idx := key - 1  // Lua 1-based → Go 0-based
    if idx >= 0 && idx < int64(t.Asize) {
        if !t.Array[idx].IsNil() {
            return t.Array[idx], true
        }
    }
    
    // 查哈希部分
    var keyTv TValue
    keyTv.SetInteger(key)
    return t.getNode(&keyTv)
}

// luaH_getstr: 字符串 key 专用（更快的路径）
// ⚠️ 修正：使用链式冲突解决
func (t *Table) GetStr(key *TString) (TValue, bool) {
    if t.Node == nil {
        return TValue{}, false
    }
    
    h := key.Hash
    size := t.Size()
    i := int(h) & (size - 1)
    n := &t.Node[i]
    
    // 检查主位置
    if n.Key.IsNil() {
        return TValue{}, false
    }
    if n.Key.Type() == LUA_TSTRING && n.Key.Str == key {
        return n.Val, true
    }
    
    // 沿冲突链遍历
    for n.Next != -1 {
        n = &t.Node[n.Next]
        if n.Key.IsNil() {
            return TValue{}, false
        }
        if n.Key.Type() == LUA_TSTRING && n.Key.Str == key {
            return n.Val, true
        }
    }
    
    return TValue{}, false
}
```

#### 3. Set 操作

```go
// luaH_set: 通用设置
func (L *LuaState) Set(t *Table, key, val *TValue) {
    // 先尝试快速路径
    if t.FastSet(key, val) {
        return
    }
    // 慢速路径：处理元表
    t.SlowSet(L, key, val)
}

func (t *Table) FastSet(key *TValue, val *TValue) bool {
    // 整数 key 在数组部分
    if key.IsInteger() {
        idx := key.I - 1
        if idx >= 0 && idx < int64(t.Asize) {
            if !t.Array[idx].IsNil() || !hasNewIndex(t) {
                t.Array[idx] = *val
                return true
            }
        }
    }
    
    // 哈希部分查找或创建
    // ...
    return false
}

// luaH_psetint: 预检查的整数设置
// 返回 HOK (0) = 成功, HNOTFOUND (1) = 需要新建, 其他 = 元方法
func (t *Table) PSetInt(key int64, val *TValue) int {
    idx := key - 1
    if idx >= 0 && idx < int64(t.Asize) {
        t.Array[idx] = *val
        return HOK
    }
    
    // 需要查哈希
    var k TValue
    k.SetInteger(key)
    return t.setNode(&k, val)
}
```

#### 4. 调整大小

```go
// luaH_resize: 重新分配数组和哈希部分
func (L *LuaState) Resize(t *Table, nasize, nhsize uint) {
    // 保存旧的数组
    oldArray := t.Array
    oldSize := t.Asize
    oldNode := t.Node
    oldNodeSize := 0
    if oldNode != nil {
        oldNodeSize = 1 << t.LSizenode
    }
    
    // 分配新数组
    t.Asize = nasize
    t.Array = make([]TValue, nasize)
    for i := range t.Array {
        t.Array[i].SetNil()
    }
    
    // 重新分配哈希
    t.LSizenode = 0
    if nhsize > 0 {
        for nhsize & (nhsize - 1) != 0 {
            nhsize &= nhsize - 1
        }
        t.LSizenode = log2(uint8(nhsize))
        t.Node = make([]Node, nhsize)
    } else {
        t.Node = nil
    }
    
    // 重建哈希
    for i := uint(0); i < oldNodeSize; i++ {
        old := &oldNode[i]
        if !old.Key.IsNil() && !old.Key.IsDead() {
            t.Set(&old.Key, &old.Val)  // 重新插入
        }
    }
    
    // 迁移数组部分的整数 key
    for i := uint(0); i < oldSize && i < nasize; i++ {
        if !oldArray[i].IsNil() {
            t.Array[i] = oldArray[i]
        }
    }
}

// luaH_resizearray: 只调整数组部分
func (L *LuaState) ResizeArray(t *Table, nasize uint) {
    // ... 类似上面的数组迁移逻辑
}
```

#### 5. 迭代

```go
// luaH_next: 返回下一个 key-value 对
// key 参数是当前 key，返回下一个
// 返回值：1 = 有下一个, 0 = 迭代结束
func (L *LuaState) Next(t *Table, key *TValue) int {
    // 查找当前 key 的位置
    var i int
    
    if key.IsNil() {
        i = 0  // 从头开始
    } else if key.IsInteger() {
        idx := key.I
        if idx >= 1 && idx <= int64(t.Asize) {
            i = int(idx)  // 数组部分索引
        } else {
            // 在哈希中找
            // ...
        }
    } else {
        // 在哈希中找 key
        // ...
    }
    
    // 返回下一个
    // ...
    return 0
}

// luaH_getn: 返回表的"长度" (#t)
func (L *LuaState) GetN(t *Table) uint {
    // #t 是数组部分最大连续长度
    n := t.Asize
    for n > 0 && t.Array[n-1].IsNil() {
        n--
    }
    return uint(n)
}
```

### 陷阱和注意事项

#### 陷阱 1: 数组/哈希边界

```c
// C 中的边界检查
unsigned u = l_castS2U(k) - 1u;  // Lua 1-based → C 0-based
if (u < h->asize) {  // 在数组范围
    // 访问数组
} else {
    // 查哈希
}
```

**Go 中容易犯的错误：**
```go
// ❌ 错误：没有正确处理 1-based 索引
if key.I < t.Asize {  // 错误！

// ✅ 正确：
idx := key.I - 1
if idx >= 0 && idx < int64(t.Asize) {
```

#### 陷阱 2: 主位置探测

```c
// 主位置冲突时使用线性探测
int mp = mainposition(t, key);
for (;;) {
    if (keyisnil(node) || keyisdead(node)) {
        // 可以使用这个空位
        return node;
    }
    if (equalkey(key, node)) {
        // 找到了
        return node;
    }
    node = gnode(t, node->u.next);  // 探测下一个
}
```

**Go 实现：**
```go
// 使用 2 的幂次大小，探测可以用 & (size-1) 优化
for i := mp; ; i = (i + 1) & (size - 1) {
    n := &t.Node[i]
    if n.Key.IsNil() {
        return n, true  // 空闲槽
    }
    if equalValue(&n.Key, key) {
        return n, false  // 找到
    }
    if i == mp {
        panic("hash table full")  // 理论不可能发生
    }
}
```

#### 陷阱 3: 元表触发

```c
// 设置时检查 __newindex 元方法
#define luaH_fastseti(t,k,val,hres) \
  { Table *h = t; \
    lua_Unsigned u = l_castS2U(k) - 1u; \
    if ((u < h->asize)) { \
      lu_byte *tag = getArrTag(h, u); \
      if (checknoTM(h->metatable, TM_NEWINDEX) || !tagisempty(*tag)) \
        { fval2arr(h, u, tag, val); hres = HOK; } \
      else hres = ~cast_int(u); } \  // ~u = -u-1 表示需要触发元方法
    else { hres = luaH_psetint(h, k, val); }}
```

**Go 实现需要考虑：**
```go
func (t *Table) SetFast(key *TValue, val *TValue) (Result, bool) {
    // 检查元表
    if t.Metatable != nil {
        // 有 __newindex 但数组位置有值 → 正常设置
        // 有 __newindex 且数组位置为空 → 返回需要触发
    }
    // 无元表 → 直接设置
}
```

#### 陷阱 4: 表在迭代中被修改

Lua 允许在迭代中修改表：
```lua
for k, v in pairs(t) do
    t.newkey = v  -- 允许
end
```

这要求实现使用安全复制或快照。

### 性能优化点

1. **字符串 key 专用路径**：短字符串（interned）可以直接比较指针
2. **整数 key 优先**：使用整数作为 key 的情况最常见
3. **小表优化**：避免为小表分配哈希部分
4. **元方法缓存**：flags 字段缓存已知不存在的元方法

### 验证测试

```lua
-- tables.lua 中的关键测试
local t = {1, 2, 3, a = "alpha"}

-- 基本访问
assert(t[1] == 1)
assert(t.a == "alpha")

-- 长度
assert(#t == 3)

-- 动态访问
t.x = 10
assert(t.x == 10)

-- 整数溢出到哈希
t[100] = "big"
assert(t[100] == "big")

-- 哈希碰撞
for i = 1, 100 do
    t["key" .. i] = i
end
-- 所有 key 都应该能正确访问
```