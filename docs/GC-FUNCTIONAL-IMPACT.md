# GC 不完整对 Lua VM 功能完整性的影响分析

## 核心结论

> **GC 不完整不会破坏 Lua 语言的核心语义（计算、控制流、闭包、协程、元表），但会导致三类实际问题：资源泄漏、内存泄漏、行为不一致。** 好消息是 `<close>` 变量完全不依赖 GC，且标准库中只有 `io` 库受直接影响。

---

## 一、影响全景图

```
                    GC 缺失的影响范围
                    
  ┌─────────────────────────────────────────────────────┐
  │                  不受影响（核心语言）                  │
  │  ✅ 变量/作用域  ✅ 控制流  ✅ 函数/闭包              │
  │  ✅ 元表/元方法   ✅ 协程    ✅ 字符串操作             │
  │  ✅ 数学运算      ✅ 表操作  ✅ 模式匹配               │
  │  ✅ <close> 变量  ✅ pcall/xpcall  ✅ require/模块     │
  └─────────────────────────────────────────────────────┘
  
  ┌─────────────────────────────────────────────────────┐
  │              直接受影响（按严重度排序）                │
  │                                                     │
  │  🔴 CRITICAL                                        │
  │  ├─ io 库文件句柄泄漏（userdata __gc 未注册）        │
  │  └─ 用户自定义 userdata 资源泄漏                     │
  │                                                     │
  │  🟠 HIGH                                            │
  │  ├─ 弱表缓存静默内存泄漏（无自动清扫）               │
  │  ├─ 嵌入场景状态关闭时终结器丢失                     │
  │  └─ collectgarbage("stop"/"restart") 无效            │
  │                                                     │
  │  🟡 MEDIUM                                          │
  │  ├─ Ephemeron 模式数据泄漏（弱键表高级用法）         │
  │  ├─ collectgarbage("count") 不准确                   │
  │  └─ collectgarbage("step") 性能问题                  │
  │                                                     │
  │  🟢 LOW                                             │
  │  ├─ 对象复活（__gc 中保存 self）不支持               │
  │  ├─ 终结器重入检测缺失                               │
  │  └─ 分代/增量 GC 模式无实际效果                      │
  └─────────────────────────────────────────────────────┘
```

---

## 二、标准库影响详表

### 只有 `io` 库受直接影响

| 标准库 | 是否创建 userdata | 是否依赖 `__gc` | 影响 |
|--------|:-:|:-:|------|
| **io** | ✅ `iolib.go:47-52` | ✅ `__gc = fGC` (行177) | 🔴 **文件句柄泄漏** |
| string | ❌ | ❌ | ✅ 无影响 |
| math | ❌ | ❌ | ✅ 无影响 |
| table | ❌ | ❌ | ✅ 无影响 |
| os | ❌ | ❌ | ✅ 无影响 |
| coroutine | ❌ | ❌ (间接：协程持有的 userdata) | ⚠️ 间接影响 |
| debug | ❌ | ❌ | ✅ 无影响 |
| utf8 | ❌ | ❌ | ✅ 无影响 |
| package | ❌ | ❌ | ✅ 无影响 |

**关键发现**：整个标准库中，**只有 `iolib.go` 创建带 `__gc` 的 userdata**。修复 M1（userdata `__gc` 注册）这一个改动就能解决标准库层面的所有 GC 问题。

---

## 三、受影响的功能场景（附具体代码示例）

### 🔴 场景 1：io 库文件句柄泄漏 [CRITICAL]

**根因**：M1 — `impl.go:897-899` SetMetatable 对 userdata 未调用 `runtime.SetFinalizer`

```lua
-- ❌ 在 go-lua 中泄漏文件描述符
for i = 1, 10000 do
  local f = io.open("/tmp/test.txt", "r")
  -- 忘记调用 f:close()
end
-- C Lua: GC 最终调用 __gc 关闭文件
-- go-lua: 10000 个未关闭的文件描述符 → "too many open files" 错误

-- ✅ 使用 <close> 可以正常工作（不依赖 GC）
for i = 1, 10000 do
  local f <close> = io.open("/tmp/test.txt", "r")
  -- 作用域结束时自动调用 __close
end
```

**影响范围**：`io.open()`、`io.tmpfile()`、`io.lines(filename)` 的非 TBC 路径

**缓解因素**：`<close>` 变量完全正常工作（`do.go:1229-1400` 的 TBC 实现独立于 GC）。`io.lines(filename)` 的 for 循环通过 TBC 第 4 返回值正确关闭文件。

### 🔴 场景 2：用户自定义 userdata 资源泄漏 [CRITICAL]

**根因**：同 M1

```lua
-- ❌ C 库绑定模式 — go-lua 中资源泄漏
local DB = {}
DB.__index = DB
function DB.connect(dsn)
  local conn = newuserdata(...)  -- 假设通过 Go 扩展创建
  setmetatable(conn, DB)
  conn._handle = raw_connect(dsn)
  return conn
end
function DB:__gc()
  if self._handle then self:close() end  -- 安全网
end
function DB:close()
  raw_close(self._handle); self._handle = nil
end

-- 使用方忘记 close → C Lua 的 GC 会兜底，go-lua 不会
```

**影响范围**：所有使用 userdata + `__gc` 作为资源管理安全网的 Go 扩展库

### 🟠 场景 3：弱表缓存内存泄漏 [HIGH]

**根因**：M-AUTO — `SweepWeakTables()` 仅在显式 `collectgarbage()` 时调用

```lua
-- ❌ 在 go-lua 中静默内存泄漏
local cache = setmetatable({}, {__mode = "v"})

function expensive_compute(key)
  if cache[key] then return cache[key] end
  local result = {heavy_computation(key)}
  cache[key] = result
  return result
end

-- C Lua: 当 result 不可达时，弱值被 GC 自动清除
-- go-lua: cache 条目永远不会被清除（除非显式调用 collectgarbage()）
--         程序看起来正确运行，但内存无限增长
```

**影响范围**：所有使用弱值表做缓存/备忘录的程序。这是 Lua 中**非常常见**的模式。

**重要细节**：弱引用本身（`weak.Pointer`）能正确检测对象被回收，但**表槽不会被自动清理**。这意味着：
- 查找行为正确（键查找返回 nil，因为弱引用已失效）
- 但表的内存不断增长（死条目占用空间）
- 只有显式 `collectgarbage()` 才会触发 `SweepWeakTables()` 清理

### 🟠 场景 4：嵌入场景状态关闭 [HIGH]

**根因**：M11 — `CloseState` 不排空终结队列

```go
// Go 嵌入代码
func runLuaScript(code string) {
    L := luaapi.NewState()
    OpenAll(L)
    L.DoString(code)  // Lua 代码创建了带 __gc 的表
    CloseState(L)     // ❌ 已入队的终结器被丢弃
}
```

**影响范围**：Go 程序频繁创建/销毁 Lua 状态的场景（如请求级别的 Lua 沙箱）

### 🟠 场景 5：GC 控制无效 [HIGH]

**根因**：M4 — stop/restart 为空操作

```lua
-- ❌ 在 go-lua 中无效
collectgarbage("stop")   -- 期望停止 GC
-- 性能关键段...
for i = 1, 1000000 do
  process(data[i])       -- Go GC 仍在后台运行
end
collectgarbage("restart") -- 期望恢复 GC
collectgarbage("collect") -- 手动触发一次完整回收
```

**影响范围**：需要精确控制 GC 时机的实时应用、游戏循环、低延迟服务

### 🟡 场景 6：Ephemeron 模式 [MEDIUM]

**根因**：M2 — 无 ephemeron 收敛

```lua
-- ⚠️ 在 go-lua 中可能泄漏（取决于值是否引用键）
local private_data = setmetatable({}, {__mode = "k"})

function attach_data(obj, data)
  private_data[obj] = data  -- data 仅通过 obj 可达
end

-- 当 obj 被回收时：
-- C Lua: ephemeron 收敛确保 data 也被回收，条目被清除
-- go-lua: 如果 data 不引用 obj → 正常工作 ✅
--         如果 data 引用 obj（循环）→ 条目可能不被清除 ⚠️
```

**实际影响**：大多数实际使用中，值不会引用键，所以弱键表在 go-lua 中**通常能正常工作**。只有值通过键间接可达的复杂场景才会出问题。

### 🟡 场景 7：内存监控不准 [MEDIUM]

**根因**：M9 — GCTotalBytes 只增不减

```lua
-- ❌ 在 go-lua 中返回不准确的值
local before = collectgarbage("count")
-- 创建大量临时对象...
local big = {}
for i = 1, 100000 do big[i] = {i} end
big = nil
collectgarbage()
local after = collectgarbage("count")
print(after - before)  -- C Lua: 接近 0; go-lua: 大正数（只增不减）
```

### 🟢 场景 8：对象复活 [LOW]

**根因**：M3 — SetFinalizer 一次性触发

```lua
-- ❌ 在 go-lua 中不支持
local resurrect_target = nil
local t = setmetatable({}, {__gc = function(self)
  resurrect_target = self  -- 复活对象
end})
t = nil
collectgarbage()
-- C Lua: t 被终结但复活，下次回收时再次调用 __gc
-- go-lua: t 被终结，复活后不会再次调用 __gc（一次性）
```

**实际影响**：极少有程序依赖对象复活。这是 Lua 规范中的边缘特性。

---

## 四、`<close>` 变量：不受 GC 影响的好消息 ✅

**关键发现**：`<close>` 变量（to-be-closed）的实现**完全独立于 GC**。

| 特性 | 实现位置 | 依赖 GC? |
|------|---------|:--------:|
| `MarkTBC()` 标记栈槽 | `do.go:1280` | ❌ |
| `CloseTBC()` 作用域退出调用 `__close` | `do.go:1345-1380` | ❌ |
| `OP_CLOSE` 指令 | `vm.go:1656-1657` | ❌ |
| `OP_RETURN` 关闭 TBC | `vm.go:2476-2477` | ❌ |
| 错误路径关闭 | `do.go:791` | ❌ |
| `io.lines(filename)` TBC 返回值 | `iolib.go:969-973` | ❌ |

这意味着：
```lua
-- ✅ 以下代码在 go-lua 中完全正确
do
  local f <close> = io.open("file.txt")
  -- 即使发生错误，f 也会被正确关闭
end

-- ✅ io.lines 的 for 循环也正确
for line in io.lines("file.txt") do
  if condition then break end  -- 提前退出也能正确关闭文件
end
```

**建议**：在 go-lua 的文档中推荐用户优先使用 `<close>` 而非依赖 `__gc` 进行资源管理。

---

## 五、功能完整性评分

### 按使用场景评分

| 使用场景 | 完整性 | 说明 |
|---------|:------:|------|
| **脚本执行**（计算、字符串处理、数据转换） | 98% | 核心语言完全正常 |
| **配置文件**（读取 Lua 配置） | 100% | 无 GC 依赖 |
| **简单嵌入**（Go 调用 Lua 函数） | 95% | 注意显式关闭资源 |
| **文件处理**（io 库密集使用） | 80% | 必须显式 close 或用 `<close>` |
| **长期运行服务**（缓存、连接池） | 60% | 弱表缓存泄漏是主要风险 |
| **C 库绑定**（userdata + __gc） | 40% | userdata __gc 完全不工作 |
| **GC 调优**（stop/step/count） | 20% | 几乎所有控制选项无效 |
| **高级 GC 模式**（分代、增量） | 0% | 完全未实现 |

### 按 Lua 5.4 特性覆盖率

| 特性类别 | 覆盖率 | 缺失项 |
|---------|:------:|--------|
| 核心语言语义 | 100% | — |
| 元表/元方法 | 95% | `__gc` 对 userdata 不工作 |
| `<close>` 变量 | 100% | — |
| 弱表基本功能 | 85% | 无自动清扫、无 ephemeron |
| collectgarbage() | 40% | stop/restart/step/count/generational/incremental |
| 标准库 | 95% | io 库 `__gc` 路径 |
| GC 行为兼容性 | 30% | 时序、顺序、模式全部不同 |

---

## 六、修复优先级（按功能影响排序）

| 优先级 | 修复项 | 难度 | 解锁的功能场景 |
|:------:|-------|:----:|---------------|
| **P0** | Userdata `__gc` 注册 (M1) | 低 | io 库安全网、所有 userdata 资源管理 |
| **P1** | 自动弱表清扫 (M-AUTO) | 中 | 弱表缓存、备忘录、对象池 |
| **P2** | CloseState 排空 (M11) | 低 | 嵌入场景状态关闭 |
| **P3** | stop/restart 实现 (M4) | 低 | GC 控制、性能调优 |
| **P4** | count 使用 MemStats (M9) | 低 | 内存监控 |
| **P5** | 重入检测 (M12) | 低 | __gc 内调用 collectgarbage 安全性 |
| **P6** | step 参数 (M5) | 中 | 增量 GC 延迟控制 |
| **P7** | Ephemeron (M2) | 极高 | 高级弱键表模式 |
| **P8** | 分代 GC (M8) | 不建议 | 性能优化（非功能性） |

**P0+P1+P2 三项修复后，功能完整性评分可从当前的 ~70% 提升到 ~90%。**

---

## 七、总结：什么不实现也没关系

| 不实现的功能 | 为什么可以接受 |
|-------------|---------------|
| 分代 GC | 性能优化，不影响正确性。Go 自身的 GC 已经很好。 |
| 增量 step | 可以用全量 GC 替代，只是延迟更高。 |
| Ephemeron 收敛 | 大多数弱键表使用场景不涉及值→键循环引用。 |
| 对象复活 | 极少有程序依赖此特性。 |
| 终结顺序 | C Lua 保证 LIFO，但很少有程序依赖具体顺序。 |
| `__gc` 在 `__gc` 内递归 | 边缘情况，可以通过重入检测安全拒绝。 |

**真正必须修复的只有两项：M1（userdata __gc）和 M-AUTO（自动弱表清扫）。这两项修复后，go-lua 对绝大多数 Lua 程序都是功能完整的。**