# go-lua GC 完整性分析与修复方案

## 概述

本文档对 go-lua 项目的 GC（垃圾回收）实现进行了全面分析，对照 C Lua 5.4 的 `lgc.c` 参考实现，识别了所有已实现的功能、缺失的功能，以及导致 `lua-master/testes/` 中测试被跳过的具体原因。最后给出分层修复方案。

### 当前状态一览

| 维度 | 状态 |
|------|------|
| `__gc` 终结器（表） | ✅ 已实现（register → enqueue → drain 三阶段） |
| `__gc` 终结器（userdata） | ❌ **未注册**（SetMetatable 未调用 SetFinalizer） |
| 弱表 `__mode="k"/"v"/"kv"` | ✅ 基本实现（两阶段 sweep） |
| 弱表临时键（ephemeron） | ❌ 完全缺失 |
| `collectgarbage("collect")` | ✅ 工作正常 |
| `collectgarbage("stop"/"restart")` | ❌ 空操作（no-op） |
| `collectgarbage("step", n)` | ⚠️ 忽略 n，执行完整 GC |
| `collectgarbage("count")` | ⚠️ 单调递增，不反映实际内存 |
| `collectgarbage("isrunning")` | ⚠️ 始终返回 true |
| 增量/分代 GC 模式 | ❌ 仅存储字符串，无实际行为 |
| 关闭状态终结 | ❌ CloseState 不排空终结队列 |
| 终结器重入检测 | ❌ 缺失 |
| 对象复活（resurrection） | ❌ SetFinalizer 一次性触发 |
| 自动弱表清扫 | ❌ 仅在显式 collectgarbage() 时清扫 |

---

## 一、已实现的 GC 功能

### 1.1 `__gc` 终结器系统（三组件架构）

| 组件 | 代码位置 | 说明 |
|------|---------|------|
| 注册 | `impl.go:860-873` | SetMetatable 检测 `__gc` 元方法，调用 `runtime.SetFinalizer` |
| 入队 | `impl.go:864-871` | Go 终结器回调获取锁，追加到 `GCFinalizerQueue` |
| 排空 | `impl.go:2562-2608` | `DrainGCFinalizers()` 原子获取队列，逐个 PCall 执行 |

### 1.2 弱表（Weak Tables）

| 功能 | 代码位置 | 状态 |
|------|---------|------|
| `__mode="v"` | `table/api/weak.go:75-92` | ✅ 数组+哈希部分 |
| `__mode="k"` | `table/api/weak.go:95-143` | ✅ |
| `__mode="kv"` | 同上 | ✅ |
| 弱引用创建 | `api/api/weak.go:39-56` | ✅ 支持 Table/LClosure/CClosure/Userdata/Thread |
| 两阶段清扫 | `api/api/weak.go:96-119` | ✅ Phase1: 创建弱引用+清空强引用; Phase2: GC+检查/恢复 |
| 字符串键处理 | `api/api/weak.go:39-56` | ✅ 字符串被正确视为不可回收（因为被 strtab 强引用） |

### 1.3 周期性 GC

| 功能 | 代码位置 | 说明 |
|------|---------|------|
| 分配计数触发 | `impl.go:569-582` | 每 10 次分配排空终结器，每 100 次触发 runtime.GC() |
| GCStepFn | `impl.go:140-155` | 清理栈槽 + runtime.GC()×2 + Gosched + drain |
| GCHasFinalizers 门控 | `impl.go:571` | 无终结器时跳过周期性 GC |

### 1.4 collectgarbage() 选项

| 选项 | 代码位置 | 状态 |
|------|---------|------|
| `"collect"` | `baselib.go:651-658` | ✅ runtime.GC()×2 + DrainGCFinalizers + SweepWeakTables |
| `"count"` | `baselib.go:659-662` | ⚠️ 返回 GCTotalBytes/1024（单调递增） |
| `"step"` | `baselib.go:663-669` | ⚠️ 执行完整 GC，忽略 n 参数，始终返回 true |
| `"isrunning"` | `baselib.go:670-672` | ⚠️ 始终返回 true |
| `"generational"` | `baselib.go:673-676` | ⚠️ 仅存储模式字符串 |
| `"incremental"` | `baselib.go:677-680` | ⚠️ 仅存储模式字符串 |
| `"param"` | `baselib.go:681-693` | ✅ 存储/读取参数（但参数未被使用） |
| `"stop"` | `baselib.go:694-696` | ❌ 空操作（落入 default 分支） |
| `"restart"` | `baselib.go:694-696` | ❌ 空操作 |

---

## 二、缺失功能清单（对照 C Lua 5.4）

### 2.1 严重（CRITICAL）— 直接导致测试失败

| ID | 缺失功能 | C Lua 位置 | 影响 |
|----|---------|-----------|------|
| **M1** | Userdata `__gc` 未注册 | `lgc.c:1065-1090` | `impl.go:897-899` 的 SetMetatable 仅设置 `ud.MetaTable = mt`，**未调用 runtime.SetFinalizer**。Userdata 的 `__gc` 永远不会触发。 |
| **M2** | 临时键（Ephemeron）收敛 | `lgc.c:530-570, 750-770` | C Lua 有专门的 ephemeron 收敛循环。Go 将弱键表与弱值表同等对待，无迭代标记传播。**导致 gc.lua Patch 1 挂起。** |
| **M3** | 对象复活（Resurrection） | `lgc.c:1569-1575` | C Lua 在终结后重新标记对象。Go 的 SetFinalizer 是一次性的，复活后不会重新注册。 |

### 2.2 高（HIGH）— 影响多个测试

| ID | 缺失功能 | 影响 |
|----|---------|------|
| **M4** | stop/restart 为空操作 | `collectgarbage("stop")` 和 `"restart"` 无实际行为。**导致 gc.lua Patch 4。** |
| **M5** | step 参数被忽略 | `collectgarbage("step", n)` 忽略 n，执行完整 GC。**导致 gc.lua Patch 4。** |
| **M11** | CloseState 不排空终结队列 | `state.go:216-228` 设置 GCClosed=true 但不排空已入队的终结器。**导致 gc.lua Patch 5。** |
| **M-AUTO** | 无自动弱表清扫 | SweepWeakTables 仅在显式 collectgarbage() 时调用，不在分配压力下自动触发。**导致 closure.lua 挂起。** |

### 2.3 中（MEDIUM）— 行为差异

| ID | 缺失功能 | 影响 |
|----|---------|------|
| **M6** | 终结顺序不同 | C Lua 按创建逆序（LIFO）终结。Go 的 SetFinalizer 顺序不确定。 |
| **M8** | 无增量/分代 GC 模式 | gengc.lua 完全依赖分代 GC 行为，无法通过。 |
| **M9** | GCTotalBytes 只增不减 | collectgarbage("count") 返回递增值，不反映实际存活内存。 |
| **M12** | 无终结器重入检测 | 在 `__gc` 内调用 collectgarbage() 不返回 false。 |

---

## 三、被跳过的测试完整目录

### 3.1 gc.lua（8 个补丁）

| 补丁 | 行号 | 测试内容 | 根因 | 严重度 | 可修复? |
|------|------|---------|------|--------|---------|
| **Patch 0** | 286, 289 | 弱 kv 表条目计数 | Go GC 不保证单次回收所有弱引用 | (b) 时序 | ⚠️ 部分（多次 GC 可缓解） |
| **Patch 0b** | 307-325 | `__gc` + 弱表顺序（5.1 bug） | 弱值清除与终结器执行无确定顺序 | (a) 根本 | ❌ 否 |
| **Patch 1** | 327-360 | Ephemeron 链解析 | Go 无 ephemeron 支持 | (a) 根本 | ❌ 否 |
| **Patch 2** | 440-458 | `__gc` × 弱表（os.exit 测试） | 终结器在弱值清除前触发 | (a) 根本 | ❌ 否 |
| **Patch 2+** | 459-478 | 弱字符串键回收 + 内存 | 弱清扫边界情况 + count 不准 | (c) 缺失 | ✅ 是 |
| **Patch 3** | 522-546 | 协程自引用循环 + `__gc` | SetFinalizer 不处理循环 + stop 无效 | (a) 根本 | ❌ 否 |
| **Patch 4** | 549-561 | stop/step 语义 | stop/restart 为空操作 | (a) 根本 | ✅ 部分可修 |
| **Patch 5** | 653-706 | 关闭状态 + 重入 `__gc` | 无关闭状态终结 + 无重入检测 | (a)+(c) | ✅ 部分可修 |

### 3.2 closure.lua（1 个补丁）

| 补丁 | 行号 | 测试内容 | 根因 | 严重度 | 可修复? |
|------|------|---------|------|--------|---------|
| **C1** | 36-52 | 弱表作为 GC 检测器（循环） | 分配时不自动清扫弱表 | (a) 根本 | ✅ 是（需架构改动） |

### 3.3 cstack.lua（1 个补丁）

| 补丁 | 行号 | 测试内容 | 根因 | 严重度 | 可修复? |
|------|------|---------|------|--------|---------|
| **CS1** | 4 | tracegc 模块（stop/start 终结器） | 缺少 C 模块 | (c) 缺失 | ✅ 是 |

### 3.4 gengc.lua 状态

- 通过 `L.DoFile(path)` 运行，**无补丁**
- 第 12 行 `collectgarbage("generational")` 仅存储字符串，不切换模式
- 第 130 行 `if T == nil then return end` **提前退出**
- 第 68-79 行（`__gc` 终结器测试）和第 122 行（弱表断言）可能失败
- 第 132-196 行永远不执行（需要 T/testC API）

### 3.5 自动跳过的部分（无需补丁）

gc.lua 中还有 ~9 个部分被 `if T then` 或 `if not _port then` 守卫自动跳过，涉及：
- step 计数比较（行 70-74）
- GC 节奏测试（行 200-210）
- 弱表重访 bug（行 296-303）
- GC 期间错误处理（行 363-404）
- Userdata GC（行 409-437）
- 回收期间错误+警告（行 481-488）
- 奇特上值回收（行 563-600）
- testC 依赖的 GC 不变量测试（行 607-637）
- 关闭状态错误对象（行 670-706）

---

## 四、根因分类

| 根因 | 影响的补丁 | 能否在 Go GC 框架内修复 |
|------|-----------|----------------------|
| **Go GC 无法停止/启动** | Patch 3, 4, 自动跳过行 70-74, 200-210 | ⚠️ 可模拟（抑制 DrainGCFinalizers） |
| **弱值清除与终结器无确定顺序** | Patch 0b, 2 | ❌ 需要自定义 GC 阶段排序 |
| **无 Ephemeron 支持** | Patch 1 | ❌ 需要自定义标记阶段逻辑 |
| **弱引用回收时序不确定** | Patch 0, closure.lua, gengc.lua:122 | ⚠️ 可通过多次 GC + 自动清扫缓解 |
| **分配时不自动清扫弱表** | closure.lua（挂起） | ✅ 可修复（钩入周期性 GC） |
| **SetFinalizer 不处理循环** | Patch 3 | ❌ Go 文档明确不保证 |
| **无重入检测** | Patch 5 | ✅ 可修复（添加标志位） |
| **缺少 C 模块** | cstack tracegc | ✅ 可用 Go 实现 |

---

## 五、分层修复方案

### Tier 1：快速修复（1-2 天，高测试收益）

#### 1.1 Userdata `__gc` 注册 [M1]
**文件**: `internal/api/api/impl.go:897-899`
**改动**: 在 `TagUserdata` 的 SetMetatable 分支中，仿照 `TagTable`（行 860-873）添加 `runtime.SetFinalizer` 调用。

```go
case objectapi.TagUserdata:
    if ud, ok := v.Val.(*objectapi.Userdata); ok {
        ud.MetaTable = mt
        // 新增：注册 __gc 终结器
        if mt != nil {
            if tm := metamethodapi.GetTM(ls, mt, metamethodapi.TM_GC); tm.Tag() != objectapi.TagNil {
                ls.Global.GCHasFinalizers = true
                runtime.SetFinalizer(ud, func(obj *objectapi.Userdata) {
                    ls.Global.GCFinalizerMu.Lock()
                    defer ls.Global.GCFinalizerMu.Unlock()
                    if !ls.Global.GCClosed {
                        ls.Global.GCFinalizerQueue = append(ls.Global.GCFinalizerQueue, obj)
                    }
                })
            }
        }
    }
```

**影响**: 解锁所有 userdata `__gc` 场景。

#### 1.2 CloseState 排空终结队列 [M11]
**文件**: `internal/state/api/state.go:216-228`
**改动**: 在设置 GCClosed=true **之前**，先排空已入队的终结器。

```go
func CloseState(L *LuaState) {
    if L.Global != nil {
        // 新增：先排空已入队的终结器
        L.DrainGCFinalizers()  // 需要传入 LuaState
        
        L.Global.GCFinalizerMu.Lock()
        L.Global.GCClosed = true
        L.Global.GCFinalizerMu.Unlock()
    }
    // ... 原有清理代码
}
```

**影响**: 部分解锁 gc.lua Patch 5（关闭状态终结）。

#### 1.3 实现 stop/restart [M4]
**文件**: `internal/stdlib/api/baselib.go` + `internal/state/api/api.go`
**改动**:
1. 在 GlobalState 中添加 `GCStopped bool` 字段
2. `"stop"` → 设置 `GCStopped = true`
3. `"restart"` → 设置 `GCStopped = false`
4. `"isrunning"` → 返回 `!GCStopped`
5. 在 `DrainGCFinalizers` 和周期性 GC 中检查 `GCStopped`

**影响**: 解锁 gc.lua Patch 4 的部分测试、cstack.lua tracegc 功能。

#### 1.4 终结器重入检测 [M12]
**文件**: `internal/api/api/impl.go` (DrainGCFinalizers)
**改动**: 添加 `InFinalizer bool` 标志，在 `__gc` 内调用 collectgarbage() 时返回 false。

```go
func (ls *State) DrainGCFinalizers() {
    if ls.Global.InFinalizer {
        return  // 防止重入
    }
    ls.Global.InFinalizer = true
    defer func() { ls.Global.InFinalizer = false }()
    // ... 原有排空逻辑
}
```

在 collectgarbage("collect") 中：
```go
if ls.Global.InFinalizer {
    L.PushBoolean(false)
    return 1
}
```

**影响**: 解锁 gc.lua Patch 5 的重入检测测试。

#### 1.5 tracegc Go 模块 [CS1]
**文件**: 新建 `internal/stdlib/api/tracegc.go`
**改动**: 实现 `tracegc.stop()` / `tracegc.start()` 作为 Go 库，控制 `DrainGCFinalizers` 的执行。

**影响**: 解锁 cstack.lua 原生 tracegc 使用。

---

### Tier 2：中等改动（3-5 天，解锁更多测试）

#### 2.1 自动弱表清扫 [M-AUTO]
**文件**: `internal/api/api/impl.go` (周期性 GC 逻辑)
**改动**: 在现有的分配计数 GC 触发中（行 569-582），添加 `SweepWeakTables()` 调用：

```go
if allocCount%100 == 0 {
    runtime.GC()
    ls.DrainGCFinalizers()
    ls.SweepWeakTables()  // 新增
}
```

或者更优雅的方案：利用 Go 的 `runtime.SetFinalizer` 在 GC 后触发一个回调，自动调用 `SweepWeakTables()`。

**影响**: **最高收益修复** — 解锁 closure.lua Patch C1（挂起问题），改善 gc.lua Patch 0 的时序问题。

#### 2.2 collectgarbage("count") 使用 runtime.MemStats [M9]
**文件**: `internal/stdlib/api/baselib.go:659-662`
**改动**: 使用 `runtime.ReadMemStats()` 获取实际堆内存，而非单调递增的 GCTotalBytes。

```go
case 3: // count
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    kb := float64(m.HeapAlloc) / 1024.0
    L.PushNumber(kb)
    return 1
```

**影响**: 解锁依赖内存计数的测试（gc.lua Patch 2+ 的 `assert(collectgarbage("count") <= m + 1)`）。

#### 2.3 弱字符串键+可回收值的正确处理 [Patch 2+]
**文件**: `internal/api/api/weak.go` + `internal/table/api/weak.go`
**改动**: 确保弱 kv 表中，字符串键+表值的条目在值被回收后正确清除，而字符串键+数字值的条目保留。

**影响**: 解锁 gc.lua Patch 2+ 的字符串键测试。

---

### Tier 3：根本性改动（长期，需要架构决策）

#### 3.1 Ephemeron 支持 [M2]
**难度**: 极高
**说明**: 需要在弱键表清扫中实现迭代收敛：如果一个键只能通过同一表中另一个条目的值到达，则该键应被视为不可达。这本质上需要一个自定义的标记阶段。

**可能的方案**:
- 在 `SweepWeakTables` 中添加多轮扫描：每轮检查哪些键仍然可达，标记其值为"暂时可达"，重复直到收敛
- 使用 Go 的 `weak.Pointer` 检测键的可达性，但这无法区分"通过值间接可达"和"直接可达"

**评估**: 不建议在当前架构下实现。标记为"已知限制"。

#### 3.2 弱值清除-终结器顺序 [Patch 0b, 2]
**难度**: 极高
**说明**: C Lua 保证弱值在终结器执行前被清除。Go 的 `runtime.SetFinalizer` 和 `weak.Pointer` 之间没有确定的顺序。

**可能的方案**:
- 在 `DrainGCFinalizers` 中，执行每个 `__gc` 前先手动清扫该对象相关的弱表条目
- 这需要维护"对象 → 弱表引用"的反向映射，复杂度高

**评估**: 不建议。标记为"已知限制"。

#### 3.3 循环引用终结 [Patch 3]
**难度**: 不可能（Go 语言限制）
**说明**: Go 文档明确声明："如果循环结构包含带终结器的块，不保证该循环被垃圾回收。"

**评估**: 无法修复。标记为"Go 语言限制"。

#### 3.4 分代 GC [M8]
**难度**: 极高（等同于重写 GC）
**说明**: 需要实现写屏障、年龄位、小/大回收周期区分。这本质上是在 Go 之上重新实现一个完整的 Lua GC。

**评估**: 不建议。gengc.lua 的大部分测试（130 行后）已被 `T==nil` 守卫跳过。

---

## 六、修复优先级与预期收益

```
修复项                          难度    预计可解锁的测试
─────────────────────────────────────────────────────────
Tier 1.1 Userdata __gc 注册     低      userdata 相关 __gc 场景
Tier 1.2 CloseState 排空        低      gc.lua Patch 5 部分
Tier 1.3 stop/restart 实现      低      gc.lua Patch 4 部分
Tier 1.4 重入检测               低      gc.lua Patch 5 重入测试
Tier 1.5 tracegc 模块           低      cstack.lua 原生支持
Tier 2.1 自动弱表清扫           中      closure.lua C1 + gc.lua Patch 0
Tier 2.2 count 使用 MemStats    中      内存计数相关断言
Tier 2.3 弱字符串键处理         中      gc.lua Patch 2+
─────────────────────────────────────────────────────────
合计可解锁：~4-5 个补丁（10 个中）
永久跳过：~5 个补丁（Go GC 根本限制）
```

### 不可修复的限制（建议永久保留 `_port` 守卫）

| 补丁 | 原因 | 分类 |
|------|------|------|
| Patch 0b | 弱值/终结器顺序 | Go GC 架构限制 |
| Patch 1 | Ephemeron | Go GC 架构限制 |
| Patch 2 | `__gc` × 弱表 | Go GC 架构限制 |
| Patch 3 | 循环引用终结 | Go 语言限制 |
| Patch 4（部分） | 真正的增量 step | Go GC 架构限制 |

---

## 七、建议实施顺序

1. **第一阶段**（Tier 1，1-2 天）：实现 1.1-1.5 的快速修复，运行测试验证
2. **第二阶段**（Tier 2，3-5 天）：实现自动弱表清扫（最高收益），改进 count 和弱字符串键处理
3. **第三阶段**：更新 `testes_wide_test.go`，移除已修复的补丁，验证测试通过
4. **文档化**：更新 `docs/TODO-gc-finalizer.md`，将 Tier 3 项标记为"已知限制（Go GC 架构决定）"

---

## 附录：C Lua vs go-lua GC 架构对比

```
C Lua 5.4 GC 架构:
┌──────────────────────────────────┐
│ 自主 GC 引擎 (lgc.c)             │
│ ├─ 三色标记（白/灰/黑）          │
│ ├─ 写屏障（barrier/barrierback） │
│ ├─ 增量步进（debt-based）        │
│ ├─ 分代模式（minor/major）       │
│ ├─ Ephemeron 收敛循环            │
│ ├─ 弱表清扫（确定性顺序）        │
│ ├─ 终结器队列（tobefnz）         │
│ └─ 对象复活 + 重新标记            │
└──────────────────────────────────┘

go-lua GC 架构:
┌──────────────────────────────────┐
│ Go runtime.GC() 桥接             │
│ ├─ runtime.SetFinalizer → 入队   │
│ ├─ DrainGCFinalizers → PCall     │
│ ├─ weak.Pointer → 弱引用         │
│ ├─ SweepWeakTables → 两阶段清扫  │
│ └─ 周期性 GC（分配计数触发）      │
│                                  │
│ ❌ 无三色标记                     │
│ ❌ 无写屏障                       │
│ ❌ 无增量/分代模式                │
│ ❌ 无 Ephemeron                   │
│ ❌ 无确定性顺序                   │
│ ❌ 无对象复活                     │
└──────────────────────────────────┘
```

**核心设计取舍**: go-lua 选择利用 Go 的 GC 而非重新实现一个完整的 Lua GC。这大幅降低了实现复杂度，但代价是无法完全兼容 C Lua 的 GC 语义。Tier 1 和 Tier 2 的修复可以在不改变这一根本决策的前提下，显著提高兼容性。