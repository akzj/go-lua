# TODO: Weak Table GC Support

## 跳过位置
- `coroutine.lua:478` — `assert(C[1] == undef)` — 弱引用 collectgarbage 后应被回收

## 现状
go-lua **完全不支持** weak table。`__mode` metafield 被忽略，不会影响 table 的 GC 行为。

## 影响范围
| 文件 | 行号 | 影响 |
|------|------|------|
| coroutine.lua | :478 | 弱表断言（已跳过） |
| gc.lua | 多处 | 弱表测试（gc.lua 本身未通过） |
| closure.lua | 多处 | upvalue 弱引用测试 |
| 用户代码 | — | `setmetatable(t, {__mode="kv"})` 静默无效 |

## 实现方案

### 核心思路：利用 Go `runtime` 的 GC
Go 1.24+ 提供 `weak.Pointer[T]`，可以直接利用 Go 的 GC 来实现 Lua 弱引用语义。

### 步骤
1. **Table 结构添加 `WeakMode` 字段**（byte: bit 0 = weak values, bit 1 = weak keys）
2. **`setmetatable` 钩子**：设置 metatable 时解析 `__mode` 字段（"k"/"v"/"kv"），设置 `WeakMode`
3. **弱值存储**：weak value 条目用 `weak.Pointer` 包装
4. **`collectgarbage("collect")`**：调用 `runtime.GC()` 后扫描所有注册的弱表，清除已回收的条目
5. **弱表注册表**：全局维护一个弱表列表（或在 GC 时扫描所有 table）

### 预估工作量
- 200-400 行代码
- 2-4 天工作量

### 需要修改的文件
| 文件 | 修改内容 |
|------|----------|
| `internal/table/api/table.go` | WeakMode 字段，Get/Set/Next 检查弱模式 |
| `internal/state/api/state.go` | 弱表注册表 |
| `internal/stdlib/api/baselib.go` | collectgarbage 扫描弱表 |
| `internal/vm/api/vm.go` | setmetatable 钩子解析 __mode |

## C Lua 参考
- `lgc.c:596` — `getmode()` 读取 `__mode`
- `lgc.c:808` — `clearbyvalues()` 清除未标记的弱值条目
- `lgc.c:789` — `clearbykeys()` 清除未标记的弱键条目
- `lgc.c:1542` — `atomic()` 阶段调用清除函数

## 优先级
**中等** — gc.lua 推进需要此功能，但不阻塞其他 testes。
