# lparser + lcode 模块规格书

## 模块职责

lparser.c 是 Lua 的**语法分析器**（2202行），lcode.c 是**代码生成器**（1972行）。两者紧密耦合，lparser 负责将词符流解析为 AST，lcode 负责将 AST 转换为字节码。

## 依赖模块

| 模块 | 依赖关系 |
|------|----------|
| llex | 词法分析 |
| lopcodes | 指令生成 |
| lobject | 类型 |
| lfunc | Proto |

## lparser 公开 API

```c
/* 解析入口 */
LUAI_FUNC LClosure *luaY_parser (lua_State *L, ZIO *z, Mbuffer *buff,
                                  Dyndata *dyd, const char *name, int firstchar);

/* FuncState 管理 */
struct FuncState {
  Proto *p;              /* 当前函数原型 */
  struct FuncState *prev; /* 外层函数 */
  struct LexState *ls;   /* 词法分析器状态 */
  struct lua_State *L;
  int freereg;           /* 下一个可用寄存器 */
  int maxstacksize;      /* 最大栈需求 */
  int nk;               /* 常量数量 */
  int np;               /* 子函数数量 */
  int firstline;        /* 首行 */
  int linedefined;      /* 定义行 */
  int lastlinedefined;  /* 结束行 */
  unsigned short nactvar; /* 活动局部变量 */
  lu_byte flag;
  unsigned short numparams;
  unsigned short is_vararg;
  unsigned short needed_specs;  /* 需要的信息 */
  unsigned char specs;
  struct tablearr {
    int *arr;
    int size;
    int n;
  } actvar, gt, label;
};
```

## lcode 公开 API

```c
/* 代码生成 */
LUAI_FUNC void luaK_codeABCk (FuncState *fs, OpCode o, int a, int b, int c, int k);
LUAI_FUNC void luaK_codeABx (FuncState *fs, OpCode o, int a, unsigned int bc);
LUAI_FUNC void luaK_codeAsBx (FuncState *fs, OpCode o, int a, int bc);
LUAI_FUNC void luaK_code (FuncState *fs, Instruction i);
LUAI_FUNC int luaK_codeAsBx (FuncState *fs, OpCode o, int a, int bc);
LUAI_FUNC int luaK_freereg (FuncState *fs, int reg);
LUAI_FUNC void luaK_reserveregs (FuncState *fs, int n);
LUAI_FUNC void luaK_checkstack (FuncState *fs, int n);
LUAI_FUNC void luaK_infix (FuncState *fs, OpCode op, expdesc *v);
LUAI_FUNC void luaK_posfix (FuncState *fs, OpCode op, expdesc *v1, expdesc *v2);
LUAI_FUNC void luaK_dischargevars (FuncState *fs, expdesc *v);
LUAI_FUNC int luaK_exp2anyreg (FuncState *fs, expdesc *v);
LUAI_FUNC int luaK_exp2anyregup (FuncState *fs, expdesc *v);
LUAI_FUNC int luaK_exp2reg (FuncState *fs, expdesc *v, int reg);
LUAI_FUNC void luaK_exp2nextreg (FuncState *fs, expdesc *v);
LUAI_FUNC void luaK_setreturns (FuncState *fs, expdesc *e, int nres);
LUAI_FUNC void luaK_setoneret (FuncState *fs, expdesc *e);
LUAI_FUNC void luaK_jump (FuncState *fs);
LUAI_FUNC int luaK_booljump (FuncState *fs, int cond, int target);
LUAI_FUNC void luaK_concat (FuncState *fs, int *l1, int l2);
LUAI_FUNC int luaK_getlabel (FuncState *fs);
LUAI_FUNC void luaK_prefix (FuncState *fs, UnOpr op, expdesc *v, int line);
LUAI_FUNC void luaK_storevar (FuncState *fs, expdesc *var, expdesc *ex);
LUAI_FUNC void luaK_self (FuncState *fs, expdesc *e, expdesc *key);
LUAI_FUNC void luaK_nil (FuncState *fs, int from, int n);
LUAI_FUNC void luaK_loadconst (FuncState *fs, expdesc *k);
LUAI_FUNC void luaK_gotonvm (FuncState *fs, Labeldesc *lb);
LUAI_FUNC void luaK_addspec (FuncState *fs, expdesc *e);
```

## 表达式描述

```go
package lua

// 表达式类型
type ExpKind int

const (
    VVOID ExpKind = iota  /* 空表达式 */
    VNIL                  /* nil */
    VTRUE                 /* true */
    VFALSE                /* false */
    VK                    /* 常量 */
    VKINT                 /* 整数常量 */
    VKFLT                 /* 浮点常量 */
    VKNUM                 /* 数值常量 */
    VNONRELOC             /* 非可重定位表达式 */
    VLOCAL                /* 局部变量 */
    VUPVAL                /* upvalue */
    VGLOBAL               /* 全局变量 */
    VINDEXED              /* 表索引 */
    VJMP                  /* 跳转指令位置 */
    VRELOC                /* 可重定位表达式 */
    VCALL                 /* 函数调用 */
    VVARARG               /* 可变参数 */
)

// expdesc 表达式描述
type expdesc struct {
    Kind   ExpKind
    Info   int      /* 寄存器、常量索引等 */
    Aux    int      /* 辅助信息 */
    Ind    struct { /* 索引信息 */
        T  int    /* 表寄存器 */
        Ty int    /* 索引类型 */
        K int     /* 常量索引 */
    }
}
```

## Go 重写规格

### 语法分析器

```go
// Parser 语法分析器
type Parser struct {
    L     *LuaState
    Lex   *LexState
    FS    *FuncState  // 当前函数状态
    dynd  *Dyndata     // 动态数据
    
    // 错误恢复
    errfunc int
    nests  []nest // 嵌套层级
}

// luaY_parser
func (L *LuaState) Parse(z *ZIO, name string) *LClosure {
    p := &Parser{
        L:   L,
        Lex: &LexState{},
    }
    
    // 初始化词法分析器
    p.Lex.SetInput(L, z, name)
    
    // 创建主函数
    mainFs := p.openFunc(nil)
    p.FS = mainFs
    
    // 解析函数体
    p.parseBlock()
    p.check(TK_EOS)
    
    // 关闭函数
    p.closeFunc()
    
    return L.NewLClosure(mainFs.p)
}

// openFunc: 打开新函数作用域
func (p *Parser) openFunc(parent *FuncState) *FuncState {
    fs := &FuncState{
        prev:     p.FS,
        L:        p.L,
        ls:       p.Lex,
        linedefined: -1,
    }
    
    // 创建 Proto
    fs.p = p.L.NewProto()
    
    if parent != nil {
        fs.maxstacksize = parent.maxstacksize
    }
    
    p.FS = fs
    return fs
}

// closeFunc: 关闭函数
func (p *Parser) closeFunc() {
    fs := p.FS
    
    // 释放未使用的寄存器
    p.freeRegs(fs, fs.freereg)
    
    // 生成 RETURN 指令
    p.codeAsBx(OP_RETURN0, 0, 0)
    
    // 链接到父函数
    if fs.prev != nil {
        fs.prev.p.P = append(fs.prev.p.P, fs.p)
    }
    
    p.FS = fs.prev
}
```

### 语句解析

```go
// parseBlock: 解析语句块
func (p *Parser) parseBlock() {
    for {
        switch p.Lex.T.Type {
        case TK_LOCAL:
            p.parseLocal()
        case TK_FUNCTION:
            p.parseFunction()
        case TK_RETURN:
            p.parseReturn()
        case TK_IF:
            p.parseIf()
        case TK_WHILE:
            p.parseWhile()
        case TK_FOR:
            p.parseFor()
        case TK_DO:
            p.next()
            p.parseBlock()
            p.check(TK_END)
        case TK_BREAK, TK_GOTO:
            p.parseGoto()
        default:
            p.parseStatement()
        }
        
        if p.Lex.Lookahead().Type == TK_EOS {
            break
        }
    }
}

// parseIf: if 语句
func (p *Parser) parseIf() {
    // if cond then block {elseif cond then block} [else block] end
    var flist []int  // else if 跳转表
    
    for i := 0; ; i++ {
        p.next()  // 跳过 if/elseif/else
        
        if i == 0 {
            // if 条件
            cond := p.parseCondition()
            thenJmp := p.codeAsBx(OP_JMP, 0, 0)
            p.enterLevel()
            p.parseBlock()
            p.leaveLevel()
            
            flist = append(flist, p.codeAsBx(OP_JMP, 0, 0))
            p.patchHere(thenJmp)
            
        } else if p.prevToken.Type == TK_ELSEIF {
            cond := p.parseCondition()
            thenJmp := p.codeAsBx(OP_JMP, 0, 0)
            p.enterLevel()
            p.parseBlock()
            p.leaveLevel()
            flist = append(flist, p.codeAsBx(OP_JMP, 0, 0))
            p.patchHere(thenJmp)
            
        } else if p.prevToken.Type == TK_ELSE {
            p.enterLevel()
            p.parseBlock()
            p.leaveLevel()
            break
        }
        
        if p.Lex.T.Type != TK_ELSEIF && p.Lex.T.Type != TK_ELSE {
            break
        }
    }
    
    p.check(TK_END)
    
    // 补丁跳转表
    for _, jmp := range flist {
        p.patchHere(jmp)
    }
}

// parseCondition: 解析条件
func (p *Parser) parseCondition() *expdesc {
    e := p.parseExpr()
    p.dischargeIfNotBool(&e)
    return &e
}
```

### 表达式解析

```go
// parseExpr: 表达式
func (p *Parser) parseExpr() *expdesc {
    return p.parseSubExpr(0)
}

// parseSubExpr: 二元表达式（优先级解析）
func (p *Parser) parseSubExpr(level int) *expdesc {
    // 处理前缀
    if p.isPrefix() {
        v := p.parsePrefix()
        return p.parseSufFix(v, level)
    }
    
    // 处理一元
    if p.isUnaryOp() {
        line := p.Lex.T.Line
        op := p.getUnaryOp()
        p.next()
        e := p.parseSubExpr(level)
        p.prefixNot(op, &e, line)
        return &e
    }
    
    // 处理字面量
    return p.parsePrimary()
}

// parseSufFix: 后缀表达式
func (p *Parser) parseSufFix(v *expdesc, level int) *expdesc {
    for {
        switch p.Lex.T.Type {
        case '.':
            // 表字段
            p.next()
            key := p.parseName()
            p.indexed(&v, key)
            
        case '[':
            // 表索引
            p.next()
            idx := p.parseExpr()
            p.checkNil()
            e := p.mkobj()
            p.discharge2anyreg(&idx)
            v.Kind = VINDEXED
            v.Ind.T = idx.Info
            v.Ind.Idx = idx.Info
            
        case ':':
            // 方法调用
            p.next()
            key := p.parseName()
            self := p.mkobj()
            p.self(&self, key)
            p.parseCall(v, args, level)
            
        case '(':
            // 函数调用
            args := p.parseCall()
            p.parseCall(v, args, level)
            
        default:
            return v
        }
    }
}
```

### 代码生成

```go
// luaK_code: 生成指令
func (fs *FuncState) code(i Instruction) int {
    fs.p.Code = append(fs.p.Code, i)
    return len(fs.p.Code) - 1
}

func (fs *FuncState) codeAsBx(op OpCode, a, bc int) int {
    i := CreateAsBx(op, a, bc)
    return fs.code(i)
}

func (fs *FuncState) codeABCk(op OpCode, a, b, c int, k bool) int {
    i := CreateABCk(op, a, b, c, k)
    return fs.code(i)
}

// luaK_exp2reg: 表达式到寄存器
func (fs *FuncState) exp2reg(e *expdesc, reg int) {
    fs.discharge(e)
    
    switch e.Kind {
    case VNONRELOC:
        if reg != e.Info {
            fs.codeAsBx(OP_MOVE, reg, e.Info)
        }
        e.Kind = VNONRELOC
        e.Info = reg
        
    case VLOCAL:
        if reg != e.Info {
            fs.codeAsBx(OP_MOVE, reg, e.Info)
        }
        e.Kind = VNONRELOC
        e.Info = reg
        
    case VUPVAL:
        fs.codeABCk(OP_GETUPVAL, reg, e.Info, 0, false)
        e.Kind = VNONRELOC
        e.Info = reg
        
    // ...
    }
}

// luaK_jump: 生成跳转
func (fs *FuncState) jump() int {
    j := fs.codeAsBx(OP_JMP, 0, 0)
    return j
}
```

## 关键算法

### 1. 寄存器分配

```go
// 寄存器从 0 开始
// 参数占用最低的寄存器

// luaK_reserveregs: 预留寄存器
func (fs *FuncState) reserveRegs(n int) {
    fs.freereg += n
    if fs.freereg > fs.maxstacksize {
        fs.maxstacksize = fs.freereg
    }
}

// luaK_freeReg: 释放寄存器
func (fs *FuncState) freeReg() {
    fs.freereg--
}
```

### 2. 跳转表管理

```go
// patch: 补丁指令
func (fs *FuncState) patchInstruction(pc, target int) {
    fs.p.Code[pc] = CreateAsBx(GetOpCode(fs.p.Code[pc]), 
                                GetArgA(fs.p.Code[pc]), 
                                target-pc)
}

func (fs *FuncState) patchHere(j int) {
    fs.patchInstruction(j, fs.pc())
}

func (fs *FuncState) concat(list *int, j int) {
    if *list == 0 {
        *list = j
    } else {
        i := *list
        for {
            next := GetArgsBx(fs.p.Code[i])
            if next >= 0 {
                fs.patchInstruction(i, j)
                break
            }
            i = -next - 1
        }
    }
}
```

## 陷阱和注意事项

### 陷阱 1: 左值 vs 右值

```go
// 表达式需要区分左值和右值
// 表赋值需要特殊处理

func (p *Parser) parseTableAssign(list *explist, v *expdesc) {
    switch v.Kind {
    case VLOCAL, VUPVAL, VGLOBAL:
        // 简单变量
        p.storeVar(v, e)
    case VINDEXED:
        // 表索引
        p.codeABCk(OP_SETTABLE, v.Ind.T, v.Ind.Idx, e.Info, false)
    }
}
```

### 陷阱 2: for 循环变量

```go
// for i = 1, 10 do ... end
// i 在循环体内是局部的，循环外不可见
```

### 陷阱 3: UpValue 闭合

```go
// 嵌套函数中的 upvalue 需要正确处理
func (p *Parser) parseClosure() {
    p.next()  // 跳过 OP_CLOSURE
    
    p.openFunc()
    p.parseBody(false, true)
    p.closeFunc()
    
    proto := p.FS.prev.p.P[len(p.FS.prev.p.P)-1]
    
    // 设置 upvalue
    for i := 0; i < fs.nups; i++ {
        if p.upIsLocal(i) {
            // 从外层函数获取
        } else {
            // 从外层函数复制
        }
    }
}
```

## 验证测试

```lua
-- 基本语句
local x = 1
if x > 0 then
    print(x)
else
    print(-x)
end

-- 表达式
local a = x + y * z
local b = {1, 2, 3}
local c = b[1]
local d = b.foo

-- 函数
local function foo(a, b)
    return a + b
end

-- 闭包
local function outer()
    local x = 10
    return function() return x end
end
```