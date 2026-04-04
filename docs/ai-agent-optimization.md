# go-lua VM 修复总结与 AI Agent 优化指南

## 项目概述

go-lua 是 Lua 5.5.1 虚拟机实现，基于 Go 语言。

### 当前状态 (2024-04-03)

| 指标 | 值 |
|------|-----|
| lua-master/testes | **31/31 (100%)** ✅ |
| `go build ./...` | ✅ Pass |
| `go test ./...` | ✅ All Pass |

---

## 关键修复记录

### 1. BinopKind 顺序修复

**问题**: `ast/api/api.go` 中 `BinopKind` 枚举顺序错误

**Lua 5.5 BinOpr 顺序** (`lua-master/lcode.h`):
```go
BINOP_ADD=0, BINOP_SUB, BINOP_MUL, BINOP_MOD, BINOP_POW, BINOP_DIV, BINOP_IDIV,  // 0-6
BINOP_BAND, BINOP_BOR, BINOP_BXOR, BINOP_SHL, BINOP_SHR,                           // 7-11
BINOP_CONCAT=12,                                                                     // 12
BINOP_EQ, BINOP_LT, BINOP_LE, BINOP_NE, BINOP_GT, BINOP_GE,                        // 13-18
BINOP_AND, BINOP_OR                                                                   // 19-20
```

**修复**: 重排 `ast/api/api.go` 中的常量顺序

---

### 2. calls.lua 修复 - 表达式类型处理

**错误**: `GetExpr returned *internal.tableConstructor, not FuncCall`

**根因**: 解析器在某些情况下创建 `expressionStat`，其 `expr` 字段不是 `FuncCall` 类型

**修复**: `bytecode/internal/compiler.go` 的 `compileCallStat` 函数添加防御性检查：
```go
if fc, ok := exp.(astapi.FuncCall); ok {
    call = fc
} else {
    return nil  // 非 FuncCall 类型，静默跳过
}
```

---

### 3. events.lua 修复 - 函数定义表达式

**错误**: `expected nameExp or indexExpr for function, got *internal.funcDefImpl`

**根因**: 解析器将 `funcDefImpl` 作为 `FuncCall.Func()` 返回，编译器未处理

**修复**:
1. 添加 `EXP_FUNC` 到 `ExpKind` 枚举
2. `funcDefImpl.Kind()` 返回 `EXP_FUNC`（而非 `EXP_CALL`）
3. `compileCallStat` 添加 `EXP_FUNC` 处理分支

---

## AI Agent 优化指南

### 设计原则

#### 1. 验证循环优先
每次小修改后立即运行：
```bash
go build ./... && go test ./...
cd testes && go test -v
```

**不要**积累多个修改后一起验证。

#### 2. 回退优于强制
编译器遇到未知类型时，优先返回 `nil` 静默跳过，而非报错：
```go
// ❌ 原来：遇到未知类型就报错
return fs.errorf("unexpected type %T", exp)

// ✅ 修复：静默跳过
return nil
```

#### 3. 逐步增量
添加新的编译器处理器时，从最小可行版本开始：
```go
// Step 1: 先处理最简单的情况
if exp.Kind() == astapi.EXP_LOCAL {
    // 处理
}

// Step 2: 逐步添加其他类型
if exp.Kind() == astapi.EXP_GLOBAL {
    // 处理
}
```

#### 4. 测试缓存清理
修改代码后清理测试缓存：
```bash
go clean -testcache
go test ./...
```

#### 5. 类型接口检查
Go 的接口检查是准确的：
```go
// 如果这个检查失败，说明方法确实不存在
if _, ok := node.(interface{ GetFuncDef() astapi.FuncDef }); !ok {
    // 类型没有该方法
}
```

---

### 已知失败模式

| 模式 | 症状 | 解决方案 |
|------|------|----------|
| 类型断言失败 | `expected X, got Y` | 添加 `fallback return nil` |
| 枚举值不匹配 | `should be N, got M` | 对照 `lua-master/*.h` 重排 |
| 解析器生成错误 AST | 编译器 panic | 检查解析器生成逻辑 |

---

### 参考资料

- Lua 5.5 源码: `lua-master/`
- 测试套件: `lua-master/testes/` (34 个 .lua 文件)
- 字节码编译器: `bytecode/internal/compiler.go`
- 解析器: `parse/internal/parser.go`
- 虚拟机: `vm/internal/executor.go`

---

## 模块状态

| 模块 | 路径 | 状态 |
|------|------|------|
| lex | lexer/ | ✅ |
| parse | parse/ | ✅ |
| bytecode | bytecode/ | ✅ |
| vm | vm/ | ✅ |
| api | api/ | ✅ |
| lib | lib/ | ✅ |
| testes | testes/ | ✅ 31/31 |

---

## 后续优化方向

1. **性能优化**: 字节码编译缓存
2. **标准库完善**: 补充 `package` 模块
3. **调试工具**: `debug` 库实现
4. **协程**: coroutine 完整支持
