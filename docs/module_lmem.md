# lmem 模块规格书

## 模块职责

Lua 的内存分配器封装。提供安全的内存分配/释放接口，带紧急 GC 恢复机制。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| lstate | 全局状态 |
| lgc | 紧急 GC 触发 |
| lua.h | lua_Alloc 类型 |

## 公开 API

```c
/* 通用分配 */
LUAI_FUNC void *luaM_realloc_ (lua_State *L, void *block, size_t osize, size_t nsize);
LUAI_FUNC void *luaM_saferealloc_ (lua_State *L, void *block, size_t osize, size_t nsize);
LUAI_FUNC void *luaM_malloc_ (lua_State *L, size_t size, int tag);

/* 数组分配 */
LUAI_FUNC void *luaM_reallocvector (lua_State *L, void *block, int oi, int ni, unsigned size_elems);
LUAI_FUNC void *luaM_newvector (lua_State *L, int n, unsigned size_elems);

/* 向量增长 */
LUAI_FUNC void *luaM_growaux_ (lua_State *L, void *block, int nelems, int *psize,
                               unsigned size_elems, int limit, const char *what);
LUAI_FUNC void *luaM_shrinkvector_ (lua_State *L, void *block, int *size,
                                   int final_n, unsigned size_elem);

/* 释放 */
LUAI_FUNC void luaM_free_ (lua_State *L, void *block, size_t osize);

/* 错误 */
LUAI_FUNC l_noret luaM_toobig (lua_State *L);
LUAI_FUNC l_noret luaM_error (lua_State *L);
```

## C 实现分析

```c
/* 内存分配器签名 */
typedef void *(*lua_Alloc) (void *ud, void *ptr, size_t osize, size_t nsize);

/* 分配策略 */
void *luaM_realloc_ (lua_State *L, void *block, size_t osize, size_t nsize) {
    global_State *g = G(L);
    
    // 先尝试直接分配
    newblock = firsttry(g, block, osize, nsize);
    
    // 失败时尝试紧急 GC
    if (newblock == NULL && nsize > 0) {
        luaC_fullgc(L, 1);  // 紧急 GC
        newblock = callfrealloc(g, block, osize, nsize);
    }
    
    // 更新 GC debt
    g->GCdebt -= (nsize - osize);
    
    return newblock;
}

/* 向量增长（自动倍增） */
void *luaM_growaux_ (...) {
    if (nelems + 1 <= size)
        return block;
    
    if (size >= limit / 2) {
        if (size >= limit)
            luaG_runerror(L, "too many %s", what);
        size = limit;
    } else {
        size *= 2;
        if (size < MINSIZEARRAY)
            size = MINSIZEARRAY;
    }
    
    return luaM_saferealloc_(L, block, old_size, new_size);
}
```

## Go 重写规格

```go
package lua

// Lua 内存分配器
type Allocator struct {
    alloc luaAlloc  // 用户提供的分配器
    ud    interface{}
    
    // GC 跟踪
    totalBytes int64
    debt       int64  // 工作预算
}

// 用户分配的签名
type luaAlloc func(ud interface{}, ptr unsafe.Pointer, osize, nsize uintptr) unsafe.Pointer

// 默认分配器
func DefaultAlloc(ud interface{}, ptr unsafe.Pointer, oldSize, newSize uintptr) unsafe.Pointer {
    if newSize == 0 {
        if oldSize != 0 {
            free(ptr)
        }
        return nil
    }
    
    if ptr == nil {
        return malloc(newSize)
    }
    
    return realloc(ptr, oldSize, newSize)
}

// 重新分配
func (a *Allocator) Realloc(L *LuaState, ptr unsafe.Pointer, oldSize, newSize uintptr) unsafe.Pointer {
    if newSize == 0 {
        if oldSize != 0 {
            a.totalBytes -= int64(oldSize)
            a.alloc(a.ud, ptr, oldSize, 0)
        }
        return nil
    }
    
    // 尝试分配
    newPtr := a.alloc(a.ud, ptr, oldSize, newSize)
    
    if newPtr == nil && newSize > 0 {
        // 分配失败，尝试紧急 GC
        if L.canEmergencyGC() {
            L.emergencyGC()
            newPtr = a.alloc(a.ud, ptr, oldSize, newSize)
        }
    }
    
    // 更新统计
    if ptr == nil {
        a.totalBytes += int64(newSize)
    } else if newPtr == nil {
        // 分配失败，不更新
    } else {
        a.totalBytes += int64(newSize) - int64(oldSize)
    }
    
    // 更新 GC debt
    a.debt -= int64(newSize) - int64(oldSize)
    
    return newPtr
}

// 安全重新分配（失败 panic）
func (a *Allocator) SafeRealloc(L *LuaState, ptr unsafe.Pointer, oldSize, newSize uintptr) unsafe.Pointer {
    newPtr := a.Realloc(L, ptr, oldSize, newSize)
    if newPtr == nil && newSize > 0 {
        panic(LuaError{Status: LUA_ERRMEM})
    }
    return newPtr
}

// 分配切片
func (a *Allocator) ReallocSlice(L *LuaState, slice interface{}, newLen int, elemSize uintptr) interface{} {
    // 通用实现，根据类型处理
    switch s := slice.(type) {
    case []byte:
        newSlice := make([]byte, newLen)
        copy(newSlice, s)
        return newSlice
    case []uintptr:
        newSlice := make([]uintptr, newLen)
        copy(newSlice, s)
        return newSlice
    // 其他类型...
    default:
        panic("unsupported slice type")
    }
}

// 向量增长
func (a *Allocator) GrowVector(L *LuaState, vec interface{}, nelems, psize *int, elemSize uintptr, limit int, name string) interface{} {
    if nelems + 1 <= *psize {
        return vec  // 不需要增长
    }
    
    newSize := *psize
    if newSize >= limit / 2 {
        if newSize >= limit {
            panic(LuaError{
                Status:  LUA_ERRRUN,
                Message: fmt.Sprintf("too many %s (limit is %d)", name, limit),
            })
        }
        newSize = limit
    } else {
        newSize *= 2
        if newSize < 4 {
            newSize = 4
        }
    }
    
    *psize = newSize
    return a.ReallocSlice(L, vec, newSize, elemSize)
}
```

## GC 预算管理

```go
// GC 通过 debt 控制执行时机
// debt > 0: 需要运行 GC
// debt < 0: 预算充裕

const (
    GCSTEPSIZE   = 1024  // 每次步进的最小单位
    PAUSEADJ     = 80    // GC 暂停时间调整
    STEPmul      = 100   // 步进乘数
    STEPSize     = (GCSTEPSIZE / STEPmul) * 100  // 步进大小
)

func (L *LuaState) stepGC() bool {
    g := L.G
    
    if g.GCThreshold > g.TotalBytes + g.Debt {
        // 运行一步
        L.GCStep()
        
        if g.GCThreshold > g.TotalBytes {
            return true  // 完成了一个周期
        }
    }
    return false
}

// 增加 GC 预算
func (g *GlobalState) IncDebt(inc int64) {
    g.GCDebt += inc
}
```

## 陷阱和注意事项

### 陷阱 1: 紧急 GC 递归

```c
// 紧急 GC 中不能再触发紧急 GC
static void *tryagain (...) {
    if (cantryagain(g)) {  // completestate(g) && !g->gcstopem
        luaC_fullgc(L, 1);
        return callfrealloc(...);
    }
    return NULL;
}
```

**Go 实现需要防止递归**
```go
func (a *Allocator) Realloc(...) {
    if inEmergencyGC {
        return nil  // 不能再尝试 GC
    }
    // ...
}
```

### 陷阱 2: 内存碎片

Lua 使用 `luaM_reallocvector` 进行数组增长，可能产生碎片。建议 Go 实现使用类似策略。

### 陷阱 3: size_t vs int

```c
// C 中使用 size_t
// Go 中使用 int 或 uintptr
```

## 验证测试

```go
func TestMemoryAllocation(t *testing.T) {
    L := NewLuaState()
    
    // 测试基本分配
    data := L.Alloc.Realloc(nil, 0, 100)
    assert.NotNil(t, data)
    
    // 测试增长
    data = L.Alloc.Realloc(data, 100, 200)
    
    // 测试释放
    data = L.Alloc.Realloc(data, 200, 0)
    assert.Nil(t, data)
    
    // 测试 GC 触发
    L.SetMemoryLimit(1024)  // 限制内存
    for i := 0; i < 1000; i++ {
        L.Alloc.Realloc(nil, 0, 1024)
    }
}
```