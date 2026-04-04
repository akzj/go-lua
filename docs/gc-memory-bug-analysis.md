# GC 内存管理 Bug 根本原因分析

## 问题概述

go-lua 的内存分配器存在严重缺陷，在长时间运行场景下会导致**内存泄漏**和**内存提前回收**问题。

---

## Bug 1: 内存提前回收 (Use-After-Free 风险)

### 位置
`mem/internal/mem.go` - `Alloc()` 函数

### 问题代码

```go
func (a *allocator) Alloc(size api.LuaMem) unsafe.Pointer {
    if size == 0 {
        return nil
    }
    // 致命错误：slice 分配后没有保存引用
    slice := make([]byte, size)
    
    // ... GC 检查 ...
    
    return unsafe.Pointer(&slice[0])  // ❌ slice 立即成为不可达对象
}
```

### 根本原因

**Go 编译器将 `slice` 变量视为函数内局部变量**。当函数返回时：

1. `slice` 变量成为不可达（unreachable）
2. `&slice[0]` 返回的指针指向的底层数组可能已被 GC 回收
3. **返回值是悬空指针（dangling pointer）**

### 等价的错误代码

```go
// 这个例子展示了同样的问题
func getPtr() []byte {
    s := make([]byte, 100)
    return s  // 返回 slice，引用有效
}

func getRawPtr() unsafe.Pointer {
    s := make([]byte, 100)
    return unsafe.Pointer(&s[0])  // 危险！s 在函数结束时被回收
}
```

### 影响

| 影响 | 描述 |
|------|------|
| 数据竞争 | 分配的内存可能被其他对象覆盖 |
| 内存泄漏 | Go 认为内存已释放，但 Lua VM 仍持有指针 |
| 崩溃 | 访问已回收内存导致 panic |
| 随机错误 | 数据损坏，难以复现 |

### 正确做法

```go
func (a *allocator) Alloc(size api.LuaMem) unsafe.Pointer {
    if size == 0 {
        return nil
    }
    
    // 方案 1: 使用 sync.Pool 保留已分配块
    if block := a.pool.Get(); block != nil {
        return block
    }
    
    // 方案 2: 分配在堆上的 slice
    slice := new([]byte)
    *slice = make([]byte, size)
    
    // 方案 3: 将指针存储到全局 map（需要手动管理生命周期）
    ptr := unsafe.SliceData(make([]byte, size))
    a.allocated[ptr] = size  // 保持引用
    return unsafe.Pointer(ptr)
}
```

---

## Bug 2: Free 是空操作

### 位置
`mem/internal/mem.go` - `Free()` 函数

### 问题代码

```go
func (a *allocator) Free(ptr unsafe.Pointer, size api.LuaMem) {
    if ptr == nil || size == 0 {
        return
    }
    // 更新 GC 会计
    if a.gc != nil {
        a.gc.AllocateBytes(^uint64(size) + 1) // 等价于 -size
    }
    // Go's GC handles deallocation; we just need to make the pointer unreachable.
    // ❌ 但这里没有做任何事情使指针不可达！
}
```

### 根本原因

1. **指针未被保存**：`Alloc()` 返回的指针从未被存储在任何地方
2. **无法释放**：即使用户想释放，也无法追踪哪些内存需要释放
3. **内存永远不回收**：即使 Go GC 运行，也无法回收这些"已分配"的内存

### 内存泄漏路径

```
Alloc() 调用
    ↓
make([]byte, size) 创建 slice
    ↓
函数返回，slice 变成不可达
    ↓
底层数组可能被 GC 回收（或不回收，行为未定义）
    ↓
Lua VM 认为内存仍有效，继续使用
    ↓
内存泄漏 / 悬空指针
```

### 影响

| 影响 | 描述 |
|------|------|
| 内存泄漏 | 分配的内存永远不会被 Go GC 回收 |
| OOM | 长期运行后内存耗尽 |
| accounting 失效 | `gc.AllocateBytes()` 调用没有实际效果 |

---

## Bug 3: Realloc 内存泄漏

### 位置
`mem/internal/mem.go` - `Realloc()` 函数

### 问题代码

```go
func (a *allocator) Realloc(ptr unsafe.Pointer, oldSize, newSize api.LuaMem) unsafe.Pointer {
    if newSize == 0 {
        a.Free(ptr, oldSize)
        return nil
    }
    if ptr == nil {
        return a.Alloc(newSize)
    }

    // 分配新块
    slice := make([]byte, newSize)  // ❌ 新 slice
    newPtr := unsafe.Pointer(&slice[0])

    // 复制数据
    if oldSize > 0 && newSize > 0 {
        copyBytes(newPtr, ptr, min(oldSize, newSize))
    }

    // ❌ 旧内存从未被释放！
    // 旧 slice 变成不可达，但内存永远不会被回收
    // 因为它从未被追踪
    
    return newPtr
}
```

### 根本原因

1. 每次 `Realloc()` 都会分配新内存
2. 旧内存从未被记录或释放
3. 每次 Realloc 都泄漏 `oldSize` 字节

### 泄漏率

| 操作 | 泄漏量 |
|------|--------|
| Realloc(100MB → 200MB) | 100MB |
| Realloc(1KB → 2KB) 1000次 | 1KB × 1000 = 1MB |
| 长期运行 | 线性增长 |

---

## Bug 4: GCCollector 从未被触发

### 位置
`gc/api/api.go` - `NewCollector()` 函数

### 问题代码

```go
func NewCollector(alloc memapi.Allocator) GCCollector {
    if DefaultGCCollector != nil {
        return DefaultGCCollector
    }
    return nil  // ❌ 当 DefaultGCCollector 为 nil 时返回 nil！
}
```

### 位置
`gc/init.go`

```go
func init() {
    gcapi.DefaultGCCollector = gcinternal.NewCollector(memapi.DefaultAllocator)
}
```

### 初始化顺序问题

1. `gc/init.go` 的 `init()` 依赖于 `memapi.DefaultAllocator` 已初始化
2. 但初始化顺序未明确定义
3. 如果 `memapi.DefaultAllocator` 为 nil，`NewCollector()` 返回 nil

### 实际影响

```go
// 在 mem/internal/mem.go
func newAllocator(config *api.AllocatorConfig) api.Allocator {
    // ...
    return &allocator{onOOM: onOOM, gc: config.GCCollector}  // gc 可能是 nil！
}
```

### GC 触发链断裂

```
mem.Alloc()
    ↓
检查 a.gc != nil  // ❌ 可能是 nil
    ↓
不调用 GC 触发
    ↓
内存持续增长
    ↓
触发 Go OOM
```

---

## Bug 5: GCCollector 与 VM 对象系统脱节

### 问题描述

`gc/internal/collector.go` 定义了完整的 GC 实现：
- `LinkObject()` 添加对象到 GC 列表
- `sweepOneList()` 遍历并回收对象
- `freeObject()` 释放对象

**但这些从未被调用！**

### 需要集成的地方（缺失）

| 模块 | 缺失的集成点 |
|------|-------------|
| `types/` | 创建 GC 对象时调用 `LinkObject()` |
| `vm/` | 销毁对象时调用 `freeObject()` |
| `state/` | 维护 `allgc` 列表头 |

### 证据

```go
// gc/internal/collector.go
func (c *Collector) LinkObject(obj *GCObject, list **GCObject) {
    // 这个函数存在，但从未被 types 或 vm 模块调用
}
```

---

## 综合分析：内存管理架构缺陷

### 理想架构

```
┌─────────────────────────────────────────────────────────┐
│                     Lua VM                              │
├─────────────────────────────────────────────────────────┤
│  types/     │ 创建对象时 → LinkObject() → allgc         │
│  vm/        │ 销毁对象时 → freeObject()                  │
│  state/     │ 遍历 GC 列表，调用 Step()                  │
└─────────────┴──────────────────────────────────────────┘
                           ↑
                           │
┌─────────────────────────────────────────────────────────┐
│                    mem/allocator                         │
├─────────────────────────────────────────────────────────┤
│  Alloc()   → gc.AllocateBytes() → 更新 totalbytes       │
│  Free()    → 通知 GC 对象已释放                         │
│  Realloc() → 更新 GC accounting                         │
└─────────────────────────────────────────────────────────┘
                           ↑
                           │
┌─────────────────────────────────────────────────────────┐
│                      gc/                                 │
├─────────────────────────────────────────────────────────┤
│  Collector.Step() → 当 totalbytes >= gcthreshold         │
│  遍历 allgc 列表 → 标记 → 回收                           │
└─────────────────────────────────────────────────────────┘
```

### 实际架构

```
┌─────────────────────────────────────────────────────────┐
│                     Lua VM                              │
├─────────────────────────────────────────────────────────┤
│  ❌ 不调用 LinkObject()                                  │
│  ❌ 不维护 allgc 列表                                    │
│  ✅ 使用 mem.Alloc() 但返回悬空指针                       │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼ （断裂）
┌─────────────────────────────────────────────────────────┐
│                    mem/allocator                         │
├─────────────────────────────────────────────────────────┤
│  Alloc()   → gc 可能为 nil → 不触发 GC                  │
│  Free()    → 空操作                                      │
│  Realloc() → 泄漏旧内存                                  │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼ （断裂）
┌─────────────────────────────────────────────────────────┐
│                      gc/                                 │
├─────────────────────────────────────────────────────────┤
│  ✅ 代码完整                                             │
│  ❌ 从未被触发                                           │
│  ❌ 没有对象在 allgc 中                                  │
└─────────────────────────────────────────────────────────┘
```

---

## 修复优先级

| 优先级 | Bug | 修复难度 | 影响 |
|--------|-----|----------|------|
| P0 | Bug 1: 内存提前回收 | 中 | 崩溃、数据损坏 |
| P0 | Bug 4: GC 从未被触发 | 低 | 内存泄漏 |
| P1 | Bug 2: Free 空操作 | 高 | 内存泄漏 |
| P1 | Bug 3: Realloc 泄漏 | 中 | 内存泄漏 |
| P2 | Bug 5: GC 集成缺失 | 高 | GC 失效 |

---

## 关键代码位置总结

| 文件 | 行号 | 问题 |
|------|------|------|
| `mem/internal/mem.go` | 49-56 | `Alloc()` 返回悬空指针 |
| `mem/internal/mem.go` | 64-68 | `Free()` 空操作 |
| `mem/internal/mem.go` | 74-94 | `Realloc()` 泄漏旧内存 |
| `gc/api/api.go` | 289-294 | `NewCollector()` 返回 nil |
| `gc/init.go` | 11 | 初始化顺序依赖 |
| `gc/internal/` | 全文 | `LinkObject()` 从未被调用 |

---

## 验证方法

### 测试 1: 内存提前回收
```go
func TestAllocReturnsValidPointer(t *testing.T) {
    alloc := mem.NewAllocator()
    
    // 分配
    ptr := alloc.Alloc(1024)
    
    // 写入数据
    slice := unsafe.Slice((*byte)(ptr), 1024)
    for i := range slice {
        slice[i] = byte(i % 256)
    }
    
    // 再次分配，触发 GC（如果 Go GC 运行）
    runtime.GC()
    runtime.GC()
    runtime.GC()
    
    // 验证数据未被破坏
    for i := range slice {
        if slice[i] != byte(i%256) {
            t.Fatal("Data corruption detected!")
        }
    }
}
```

### 测试 2: 内存泄漏
```go
func TestNoMemoryLeak(t *testing.T) {
    memBefore := runtime.MemStats{}
    runtime.ReadMemStats(&memBefore)
    
    alloc := mem.NewAllocator()
    
    for i := 0; i < 100000; i++ {
        ptr := alloc.Alloc(1024)
        alloc.Realloc(ptr, 1024, 2048)  // 泄漏 1024
    }
    
    runtime.GC()
    
    memAfter := runtime.MemStats{}
    runtime.ReadMemStats(&memAfter)
    
    // 如果泄漏，会看到显著增长
    leaked := int64(memAfter.Alloc) - int64(memBefore.Alloc)
    if leaked > 1024*100 {  // 允许一些正常增长
        t.Fatalf("Memory leak detected: %d bytes leaked", leaked)
    }
}
```
