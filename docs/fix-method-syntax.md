# 修复总结: obj:method() 方法调用语法

## 问题描述

**症状**: lua-master/testes 测试失败，报错 "expected NameExp for function, got *internal.indexExpr"

**根因**: Bytecode 编译器 `compileCallStat` 函数只处理了 `nameExp` (全局函数调用如 `foo()`)，无法处理 `indexExpr` (方法调用如 `obj:method()`)

## 错误分析流程

### 1. 定位错误来源

```bash
# 搜索错误消息
grep -rn "expected NameExp" bytecode/ parse/ vm/ --include="*.go"
# 结果: bytecode/internal/compiler.go:112

# 查看具体代码
sed -n '80,140p' bytecode/internal/compiler.go
```

### 2. 理解 AST 结构

```bash
# 查看 funcCall 结构
grep -n "type funcCall" parse/internal/parser.go
# 结果: 第475行

# 查看 funcCall 实现
sed -n '475,495p' parse/internal/parser.go
```

关键发现:
- `funcCall` 有一个 `func_` 字段是 `astapi.ExpNode`
- 方法调用时 `func_` 是 `indexExpr` 类型 (表.方法)
- 全局调用时 `func_` 是 `nameExp` 类型 (函数名)

### 3. 检查 indexExpr 结构

```bash
grep -n "type indexExpr" parse/internal/parser.go
sed -n '367,380p' parse/internal/parser.go
```

发现问题: `indexExpr` 缺少 accessor 方法 (`GetTable()`, `GetKey()`)

## 修复方案

### 修复1: 添加 indexExpr accessor 方法

**文件**: `parse/internal/parser.go`

```go
// 原始代码 (第367-374行)
type indexExpr struct {
    baseNode
    table astapi.ExpNode
    key   astapi.ExpNode
}

func (e *indexExpr) IsConstant() bool { return false }
func (e *indexExpr) Kind() astapi.ExpKind { return astapi.EXP_INDEXED }

// 修复后
type indexExpr struct {
    baseNode
    table astapi.ExpNode
    key   astapi.ExpNode
}

func (e *indexExpr) IsConstant() bool { return false }
func (e *indexExpr) Kind() astapi.ExpKind { return astapi.EXP_INDEXED }
func (e *indexExpr) GetTable() astapi.ExpNode { return e.table }  // 新增
func (e *indexExpr) GetKey() astapi.ExpNode { return e.key }       // 新增
```

### 修复2: 添加编译器接口和辅助函数

**文件**: `bytecode/internal/compiler.go`

```go
// 添加接口定义
type indexAccess interface {
    GetTable() astapi.ExpNode
    GetKey() astapi.ExpNode
}

type nameAccess interface {
    Name() string
}

// 添加表达式编译到寄存器的辅助函数
func (fs *FuncState) expToReg(exp astapi.ExpNode, destReg int) int {
    switch e := exp.(type) {
    case interface{ GetValue() string }:
        // 处理字符串常量
        idx := fs.addConstant(&Constant{Type: ConstString, Str: e.GetValue()})
        fs.emitABx(int(opcodes.OP_LOADK), destReg, idx)
    case interface{ GetValue() int64 }:
        // 处理整数常量
        idx := fs.addConstant(&Constant{Type: ConstInteger, Int: e.GetValue()})
        fs.emitABx(int(opcodes.OP_LOADK), destReg, idx)
    case interface{ GetValue() float64 }:
        // 处理浮点数常量
        idx := fs.addConstant(&Constant{Type: ConstFloat, Float: e.GetValue()})
        fs.emitABx(int(opcodes.OP_LOADK), destReg, idx)
    case binopAccess:
        // 处理二元表达式
        fs.compileBinop(e, destReg)
    case indexAccess:
        // 处理索引表达式 (obj.key)
        fs.compileIndexExpr(e, destReg)
    case interface{ Name() string }:
        // 处理全局变量
        nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: e.Name()})
        fs.emitABC(int(opcodes.OP_GETTABUP), destReg, 0, nameIdx+256)
    default:
        fs.emitABC(int(opcodes.OP_LOADNIL), destReg, 0, 0)
    }
    return destReg
}
```

### 修复3: 重写 compileCallStat 函数

```go
func (fs *FuncState) compileCallStat(stat astapi.StatNode) error {
    // ... 获取 call 表达式 ...
    
    funcExp := call.Func()
    args := call.Args()
    
    if idx, ok := funcExp.(indexAccess); ok {
        // 方法调用: obj:method(args)
        // 编译流程:
        // 1. 加载 obj 到寄存器
        // 2. GETTABLE 获取 obj.method
        // 3. MOVE obj 到下一个寄存器 (self)
        // 4. 加载参数
        // 5. CALL
        
        table := idx.GetTable()
        key := idx.GetKey()
        
        funcReg := fs.allocReg()
        tableReg := fs.allocReg()
        fs.expToReg(table, tableReg)
        
        // GETTABLE R(funcReg), R(tableReg), K(methodName)
        if s, ok := key.(interface{ GetValue() string }); ok {
            methodIdx := fs.addConstant(&Constant{Type: ConstString, Str: s.GetValue()})
            fs.emitABC(int(opcodes.OP_GETTABLE), funcReg, tableReg, methodIdx+256)
        }
        
        // SELF: MOVE obj 到 R(funcReg+1)
        fs.emitABC(int(opcodes.OP_MOVE), funcReg+1, tableReg, 0)
        
        // 加载参数
        for i, arg := range args {
            argReg := funcReg + 2 + i
            fs.expToReg(arg, argReg)
        }
        
        // CALL R(funcReg), nArgs+2, 1
        fs.emitABC(int(opcodes.OP_CALL), funcReg, len(args)+2, 1)
        
    } else if name, ok := funcExp.(nameAccess); ok {
        // 全局函数调用: foo(args)
        funcName := name.Name()
        funcReg := 0
        
        nameIdx := fs.addConstant(&Constant{Type: ConstString, Str: funcName})
        fs.emitABC(int(opcodes.OP_GETTABUP), funcReg, 0, nameIdx+256)
        
        for i, arg := range args {
            argReg := 1 + i
            fs.expToReg(arg, argReg)
        }
        
        fs.emitABC(int(opcodes.OP_CALL), funcReg, len(args)+1, 1)
    }
    
    return nil
}
```

## 验证步骤

```bash
# 1. 编译检查
go build ./...

# 2. 运行测试
go test ./...

# 3. 检查 testes 通过数量
go test ./testes/... -v 2>&1 | grep -E "Passed:|Failed:"
```

## 关键教训

### 1. 接口是 Go 中处理多态的标准方式

当 AST 节点类型不同时，使用接口来统一访问:

```go
// 不用 if-else 判断具体类型
// 用接口抽象公共行为
type tableGetter interface {
    GetTable() astapi.ExpNode
}
```

### 2. 错误消息包含足够信息

编译器报错 "expected NameExp for function, got *internal.indexExpr" 清楚地指出了:
- 期望的类型: `NameExp`
- 实际收到的类型: `indexExpr`
- 上下文: "for function"

### 3. 增量修复比重写更安全

不要一次性修改整个函数，先:
1. 添加必要的基础设施 (接口、辅助函数)
2. 测试通过后，再修改核心逻辑

### 4. 理解 Lua 方法调用语义

`obj:method(a, b)` 等价于:
```lua
obj.method(obj, a, b)  -- obj 作为第一个参数 (self)
```

## 后续优化建议

1. **支持 SELF 指令**: 当前使用 MOVE + GETTABLE，可以用更高效的 OP_SELF
2. **寄存器分配优化**: 当前手动管理寄存器，可以考虑线性扫描算法
3. **错误信息改进**: 提供更具体的修复建议

## 相关文件

- `parse/internal/parser.go` - AST 节点定义和访问器
- `bytecode/internal/compiler.go` - Bytecode 编译逻辑
- `lua-master/testes/` - Lua 标准测试套件
