# TODO: Yield in Metamethods & For-Iterators

## 跳过位置
- `coroutine.lua:856-:1052` — "yields inside metamethods" + "yields inside for iterators"

## 现状
go-lua **不支持在 metamethod 执行期间 yield**。当 VM 执行 `__lt`、`__le`、`__eq`、`__add` 等 metamethod 时，如果 metamethod 内部调用了 `coroutine.yield()`，会导致错误而非正确挂起/恢复。

同样，`for ... in` 循环的迭代器函数中 yield 也不支持。

## 影响范围
| 文件 | 行号 | 影响 |
|------|------|------|
| coroutine.lua | :856-:1035 | yield-in-metamethods 测试（已跳过） |
| coroutine.lua | :1038-:1052 | yield-in-for-iterators 测试（已跳过） |
| 用户代码 | — | 在协程中使用 metamethod 时不能 yield |

## 技术原理

### C Lua 的实现方式
C Lua 使用 **continuation** 机制（`lua_callk`、`lua_pcallk`）来支持 yield-in-metamethods：

1. VM Execute 循环中每个调用 metamethod 的 opcode（OP_LT、OP_LE、OP_EQ、OP_ADD 等）都注册一个 **continuation function**
2. 当 metamethod 内部 yield 时，VM 保存当前 PC 和 continuation 到 CallInfo
3. resume 时，不是重新进入 Execute 循环，而是调用 continuation function
4. continuation function 从 yield 点之后继续执行逻辑

### 关键 C Lua 代码
```c
// lvm.c — OP_LT 示例
case OP_LT: {
  // ...
  if (luaV_lessthan(L, s2v(ra), s2v(prc)))  // 可能触发 __lt
    pc++;  // 跳过下一条指令
  // ...
}

// lvm.c:291 — luaV_lessthan 注册 continuation
int luaV_lessthan (lua_State *L, const TValue *l, const TValue *r) {
  // ... 如果需要调用 metamethod:
  luaT_callTMres(L, tm, l, r, L->top.p);  // 调用 __lt
  return !l_isfalse(s2v(L->top.p));
}

// ldo.c:512 — callk 支持
void luaD_callnoyield (lua_State *L, StkId func, int nResults) {
  // ...
}
```

### go-lua 需要的改动

#### 1. CallInfo 添加 continuation 支持
```go
type CallInfo struct {
    // ... existing fields ...
    Continuation func(L *LuaState, status int, ctx int) int  // continuation function
    Ctx          int                                           // continuation context
    SavedPC      int                                           // PC at yield point
}
```

#### 2. VM Execute 中每个 metamethod 调用点注册 continuation
需要修改的 opcode（至少）：
| Opcode | Metamethod | 文件 |
|--------|-----------|------|
| OP_ADD, OP_SUB, OP_MUL, ... | `__add`, `__sub`, `__mul`, ... | vm.go |
| OP_LT, OP_LE, OP_EQ | `__lt`, `__le`, `__eq` | vm.go |
| OP_CONCAT | `__concat` | vm.go |
| OP_LEN | `__len` | vm.go |
| OP_GETTABLE, OP_SETTABLE | `__index`, `__newindex` | vm.go |
| OP_CALL, OP_TAILCALL | `__call` | vm.go |
| OP_FORLOOP (generic) | iterator | vm.go |

#### 3. Resume 路径调用 continuation
```go
func (L *LuaState) Resume() {
    // ... 
    if ci.Continuation != nil {
        // 不重新进入 Execute，直接调用 continuation
        ci.Continuation(L, status, ci.Ctx)
    }
}
```

### 预估工作量
- **大量工作**：需要修改 vm.go 中 ~20 个 opcode 的 metamethod 调用点
- 每个调用点需要：保存状态 → 注册 continuation → continuation 恢复逻辑
- 预估 500-1000 行代码修改
- **5-10 天工作量**
- 高风险：可能引入 Execute 循环的微妙 bug

### 需要修改的文件
| 文件 | 修改内容 |
|------|----------|
| `internal/vm/api/vm.go` | 每个 metamethod 调用点添加 continuation |
| `internal/vm/api/do.go` | Resume 路径支持 continuation 调用 |
| `internal/state/api/state.go` | CallInfo 结构添加 continuation 字段 |

## C Lua 参考
- `lvm.c:291` — `luaV_lessthan` 调用 metamethod
- `lvm.c:1782` — OP_LT 实现
- `ldo.c:512` — `luaD_callnoyield` / `luaD_call` continuation 支持
- `ldo.c:636` — `resume` 中检查 continuation 并调用
- `ldo.c:489` — `finishCcall` 调用 continuation function

## 优先级
**高（但复杂）** — 这是 Lua 5.2+ 的核心特性，影响所有在协程中使用 metamethod 的代码。
建议在所有简单 testes 修复完成后，作为专项任务实施。
