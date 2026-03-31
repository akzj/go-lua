# Lua 5.5 虚拟机 (lvm) 模块分析

## 1. VM 执行循环结构

### 1.1 luaV_execute 函数概述

```c
// lvm.c 第 1198 行
void luaV_execute (lua_State *L, CallInfo *ci) {
  LClosure *cl;
  TValue *k;           // 常量表指针
  StkId base;         // 当前函数的栈基址
  const Instruction *pc;  // 程序计数器
  int trap;
#if LUA_USE_JUMPTABLE
#include "ljumptab.h"  // GCC 可用的跳转表
#endif
  
 startfunc:
  trap = L->hookmask;
 returning:
  cl = ci_func(ci);
  k = cl->p->k;          // 从闭包获取常量表
  pc = ci->u.l.savedpc;  // 恢复 PC
  if (l_unlikely(trap))
    trap = luaG_tracecall(L);
  base = ci->func.p + 1;  // 计算栈基址 = 函数指针 + 1
```

### 1.2 主循环 (第 1209-1231 行)

```c
  /* main loop of interpreter */
  for (;;) {
    Instruction i;  /* 当前执行的指令 */
    vmfetch();      // 取指
    
    lua_assert(base == ci->func.p + 1);
    lua_assert(base <= L->top.p && L->top.p <= L->stack_last.p);
    
    vmdispatch (GET_OPCODE(i)) {  // 指令分派
      vmcase(OP_MOVE) {
        StkId ra = RA(i);
        setobjs2s(L, ra, RB(i));
        vmbreak;
      }
      // ... 48 条指令的 case
    }
  }
}
```

### 1.3 vmfetch 宏 (第 1185 行)

```c
#define vmfetch()  { \
  if (l_unlikely(trap)) {                    /* 栈重分配或 hook? */ \
    trap = luaG_traceexec(L, pc);             /* 处理 hook */ \
    updatebase(ci);                           /* 修正栈基址 */ \
  } \
  i = *(pc++);                                /* 取指并递增 PC */ \
}
```

### 1.4 vmdispatch 宏

```c
#define vmdispatch(o)  switch(o)
#define vmcase(l)      case l:
#define vmbreak        break
```

**GCC 跳转表优化** (ljumptab.h):
```c
// GCC 编译器下，使用跳转表代替 switch，生成更高效的机器码
// 表驱动 dispatch
static const void* const jumptable[] = {
    &&OP_MOVE, &&OP_LOADI, &&OP_LOADK, ...
};
goto jumptable[GET_OPCODE(i)];
```

---

## 2. 指令格式分析

### 2.1 五种指令格式 (lopcodes.h)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 格式      │ 31-25            │ 24-17       │ 16    │ 15-8        │ 7-0   │
├─────────────────────────────────────────────────────────────────────────────┤
│ iABC      │ C(8)             │ B(8)        │ k(1)  │ A(8)        │ Op(7) │
├─────────────────────────────────────────────────────────────────────────────┤
│ ivABC     │ vC(10)           │ vB(6)       │ k(1)  │ A(8)        │ Op(7) │
├─────────────────────────────────────────────────────────────────────────────┤
│ iABx      │ Bx(17)           │             │ A(8)        │ Op(7) │
├─────────────────────────────────────────────────────────────────────────────┤
│ iAsBx     │ sBx(signed)(17)  │             │ A(8)        │ Op(7) │
├─────────────────────────────────────────────────────────────────────────────┤
│ iAx       │ Ax(25)           │                      │ Op(7) │
├─────────────────────────────────────────────────────────────────────────────┤
│ isJ       │ sJ(signed)(25)   │                      │ Op(7) │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 参数解码宏 (lopcodes.h 第 60-100 行)

```c
// 从指令中提取参数
#define GET_OPCODE(i)   (cast(OpCode, ((i)>>POS_OP) & MASK1(SIZE_OP,0)))
#define GETARG_A(i)     getarg(i, POS_A, SIZE_A)
#define GETARG_B(i)     check_exp(checkopm(i, iABC), getarg(i, POS_B, SIZE_B))
#define GETARG_C(i)     check_exp(checkopm(i, iABC), getarg(i, POS_C, SIZE_C))
#define GETARG_Bx(i)    check_exp(checkopm(i, iABx), getarg(i, POS_Bx, SIZE_Bx))
#define GETARG_sBx(i)   (getarg(i, POS_Bx, SIZE_Bx) - OFFSET_sBx)  // 有符号
#define GETARG_Ax(i)    check_exp(checkopm(i, iAx), getarg(i, POS_Ax, SIZE_Ax))
#define GETARG_sJ(i)    (getarg(i, POS_sJ, SIZE_sJ) - OFFSET_sJ)   // 有符号跳转
#define TESTARG_k(i)    (cast_int(((i) & (1u << POS_k))))         // k 位测试
```

### 2.3 各指令格式使用场景

| 格式 | 指令示例 | 用途 |
|------|----------|------|
| **iABC** | OP_ADD, OP_SUB, OP_CALL | 标准三元操作 |
| **ivABC** | OP_NEWTABLE | 变长参数 |
| **iABx** | OP_LOADK, OP_CLOSURE | 常量索引 |
| **iAsBx** | OP_JMP, OP_FORLOOP | 有符号跳转偏移 |
| **iAx** | OP_EXTRAARG | 扩展参数 |
| **isJ** | OP_JMP | 长跳转 (sJ) |

---

## 3. 寄存器访问模式

### 3.1 寄存器计算宏 (lvm.c 第 1102-1110 行)

```c
#define RA(i)    (base + GETARG_A(i))       // R[A] = base[A]
#define vRA(i)   s2v(RA(i))                 // R[A] 的 TValue 指针

#define RB(i)    (base + GETARG_B(i))       // R[B] = base[B]
#define vRB(i)   s2v(RB(i))

#define RC(i)    (base + GETARG_C(i))       // R[C] = base[C]
#define vRC(i)   s2v(RC(i))

#define KB(i)    (k + GETARG_B(i))          // K[B] = 常量表[B]
#define KC(i)    (k + GETARG_C(i))          // K[C] = 常量表[C]

// RKC: 如果 k=1 则使用常量，否则使用寄存器
#define RKC(i)   ((TESTARG_k(i)) ? k + GETARG_C(i) : s2v(base + GETARG_C(i)))
```

### 3.2 栈布局示意图

```
┌─────────────────────────────────────────────────────────────┐
│ 调用帧布局                                                   │
├─────────────────────────────────────────────────────────────┤
│ func    │ arg1 │ arg2 │ ... │ local1 │ ... │ temp │ top  │
├─────────────────────────────────────────────────────────────┤
│   ▲                                              ▲          │
│   │                                              │          │
│ base                                          L->top       │
│ (= func + 1)                                                    │
└─────────────────────────────────────────────────────────────┘

寄存器索引：
  R[0] = *(base + 0)  即 *(func + 1)
  R[1] = *(base + 1)  即 *(func + 2) = arg1
  R[A] = *(base + A)
```

### 3.3 Go 实现建议

```go
type LuaState struct {
    stack     []TValue   // 栈
    top       int        // 栈顶索引
    ci        *CallInfo  // 当前调用信息
}

func (L *LuaState) execute() {
    for {
        // 取指
        i := L.fetch()
        
        // 分派
        switch i.Opcode() {
        case OP_MOVE:
            ra := L.reg(i.A())
            rb := L.reg(i.B())
            ra.Copy(rb)
        // ...
        }
    }
}

// 寄存器访问
func (L *LuaState) reg(idx int) *TValue {
    base := L.ci.Base()
    return &L.stack[base+idx]
}

// 常量访问
func (L *LClosure) const(idx int) *TValue {
    return &L.Proto.K[idx]
}
```

---

## 4. 关键指令实现分析

### 4.1 OP_CALL: 函数调用 (第 1629-1646 行)

```c
vmcase(OP_CALL) {
  StkId ra = RA(i);
  CallInfo *newci;
  int b = GETARG_B(i);         // 参数个数 (+1 表示包含函数自己)
  int nresults = GETARG_C(i) - 1;  // 期望返回值个数
  
  if (b != 0)  /* 固定参数? */
    L->top.p = ra + b;        // 设置 top 指示参数个数
  /* 否则前一条指令已设置 top */
  
  savepc(ci);  /* 保存 PC 以便错误处理 */
  
  if ((newci = luaD_precall(L, ra, nresults)) == NULL)
    updatetrap(ci);  /* C 调用，无需额外处理 */
  else {  /* Lua 调用: 在同一 C 帧中运行 */
    ci = newci;
    goto startfunc;  /* 跳转到新函数执行 */
  }
}
```

**语义**：
- B=0 表示参数个数由前一条指令决定
- B≠0 表示固定 B 个参数（含函数自己）
- C=0 表示可变返回值
- 调用 luaD_precall 后，若返回 NULL 表示 C 函数，否则返回新的 CallInfo

### 4.2 OP_RETURN: 函数返回 (第 1766-1797 行)

```c
vmcase(OP_RETURN) {
  StkId ra = RA(i);
  int n = GETARG_B(i) - 1;       // 返回值个数
  int nparams1 = GETARG_C(i);     // 是否有固定参数
  
  if (n < 0)  /* 非固定? */
    n = cast_int(L->top.p - ra);  // 取可用值
  
  savepc(ci);
  
  if (TESTARG_k(i)) {  /* 有待关闭的 upvalue? */
    ci->u2.nres = n;
    if (L->top.p < ci->top.p)
      L->top.p = ci->top.p;
    luaF_close(L, base, CLOSEKTOP, 1);  /* 关闭 upvalue */
    updatetrap(ci);
    updatestack(ci);
  }
  
  if (nparams1)  /* 变参函数? */
    ci->func.p -= ci->u.l.nextraargs + nparams1;
  
  L->top.p = ra + n;  /* 设置调用者栈顶 */
  luaD_poscall(L, ci, n);  /* 返回到调用者 */
  updatetrap(ci);
  
  goto ret;  /* 检查是否真正返回还是尾调用 */
}

ret:
  if (ci->callstatus & CIST_FRESH)
    return;  /* 结束此帧 */
  else {
    ci = ci->previous;
    goto returning;  /* 继续运行调用者 */
  }
}
```

**OP_RETURN0/OP_RETURN1 优化**：无返回值或单返回值时的快速路径

### 4.3 OP_JMP: 跳转 (第 1595 行)

```c
#define dojump(ci,i,e)  { pc += GETARG_sJ(i) + e; updatetrap(ci); }

vmcase(OP_JMP) {
  dojump(ci, i, 0);
  vmbreak;
}
```

**关键点**：跳转是相对于下一条指令的位移

### 4.4 OP_GETTABLE: 表访问 (第 1302-1316 行)

```c
vmcase(OP_GETTABLE) {
  StkId ra = RA(i);
  TValue *rb = vRB(i);       // 表
  TValue *rc = vRC(i);       // 键
  lu_byte tag;
  
  if (ttisinteger(rc)) {    /* 整数键的快速路径 */
    luaV_fastgeti(rb, ivalue(rc), s2v(ra), tag);
  }
  else
    luaV_fastget(rb, rc, s2v(ra), luaH_get, tag);
  
  if (tagisempty(tag))
    Protect(luaV_finishget(L, rb, rc, ra, tag));  /* 处理元方法 */
  vmbreak;
}
```

### 4.5 OP_SETTABLE: 表赋值 (第 1380-1397 行)

```c
vmcase(OP_SETTABLE) {
  StkId ra = RA(i);          // 表
  int hres;
  TValue *rb = vRB(i);       // 键
  TValue *rc = RKC(i);        // 值
  
  if (ttisinteger(rb)) {     /* 整数键的快速路径 */
    luaV_fastseti(s2v(ra), ivalue(rb), rc, hres);
  }
  else {
    luaV_fastset(s2v(ra), rb, rc, hres, luaH_pset);
  }
  
  if (hres == HOK)
    luaV_finishfastset(L, s2v(ra), rc);  /* 写屏障 */
  else
    Protect(luaV_finishset(L, s2v(ra), rb, rc, hres));  /* 处理元方法 */
  vmbreak;
}
```

### 4.6 OP_FORLOOP: 数值循环 (第 1844-1868 行)

```c
vmcase(OP_FORLOOP) {
  StkId ra = RA(i);
  
  if (ttisinteger(s2v(ra + 1))) {  /* 整数循环? */
    lua_Unsigned count = l_castS2U(ivalue(s2v(ra)));  // 剩余次数
    if (count > 0) {  /* 还有迭代? */
      lua_Integer step = ivalue(s2v(ra + 1));
      lua_Integer idx = ivalue(s2v(ra + 2));  // 控制变量
      chgivalue(s2v(ra), l_castU2S(count - 1));  // 更新计数器
      idx = intop(+, idx, step);  // 加步长
      chgivalue(s2v(ra + 2), idx);  // 更新控制变量
      pc -= GETARG_Bx(i);  /* 跳转回去 */
    }
  }
  else if (floatforloop(L, ra))  /* 浮点循环 */
    pc -= GETARG_Bx(i);  /* 跳转回去 */
  
  updatetrap(ci);  /* 允许信号中断循环 */
  vmbreak;
}

vmcase(OP_FORPREP) {
  StkId ra = RA(i);
  savestate(L, ci);
  if (forprep(L, ra))
    pc += GETARG_Bx(i) + 1;  /* 跳过循环体 */
  vmbreak;
}
```

**for 循环栈布局**：
```
ra[0]     = count/limit (初始化时的 limit，转为迭代次数)
ra[1]     = step
ra[2]     = 控制变量 (idx)
ra[3...]  = 循环体局部变量
```

---

## 5. 元方法调用

### 5.1 OP_MMBIN: 二元运算元方法 (第 1554-1567 行)

```c
vmcase(OP_MMBIN) {
  StkId ra = RA(i);
  Instruction pi = *(pc - 2);   /* 原始算术指令 */
  TValue *rb = vRB(i);
  TMS tm = (TMS)GETARG_C(i);   /* 元方法类型 */
  StkId result = RA(pi);       /* 结果位置 */
  lua_assert(OP_ADD <= GET_OPCODE(pi) && GET_OPCODE(pi) <= OP_SHR);
  Protect(luaT_trybinTM(L, s2v(ra), rb, result, tm));
  vmbreak;
}
```

**执行流程**：
1. 算术指令先尝试直接计算
2. 若失败，生成 OP_MMBIN 指令
3. OP_MMBIN 调用对应元方法

### 5.2 luaV_finishget: GET 元方法处理 (lvm.c 第 859-870 行)

```c
static void unroll (lua_State *L, void *ud) {
  CallInfo *ci;
  UNUSED(ud);
  while ((ci = L->ci) != &L->base_ci) {  /* 栈上有内容? */
    if (!isLua(ci))  /* C 函数? */
      finishCcall(L, ci);
    else {  /* Lua 函数 */
      luaV_finishOp(L);  /* 完成被中断的指令 */
      luaV_execute(L, ci);  /* 继续执行到更高 C 边界 */
    }
  }
}

lu_byte luaV_finishget (lua_State *L, const TValue *t, TValue *key,
                        StkId val, lu_byte tag) {
  // 处理 __index 元方法
  // 返回最终 tag
}
```

### 5.3 Protect 宏 (第 1144-1147 行)

```c
#define Protect(exp)  (savestate(L,ci), (exp), updatetrap(ci))
```

**作用**：在可能出错、可能重新分配栈、可能改变 hook 的代码周围使用，确保错误时 PC 和 top 正确。

---

## 6. Go 重写要点

### 6.1 整体架构

```go
// VM 执行器核心
func (L *LuaState) Execute() {
    for {
        // 取指
        i := L.fetch()
        
        // 调度
        switch i.Opcode() {
        case OP_MOVE:
            L.opMove(i)
        case OP_LOADK:
            L.opLoadK(i)
        case OP_GETTABLE:
            L.opGetTable(i)
        case OP_CALL:
            L.opCall(i)
        // ... 其他 45 条指令
        }
    }
}

// 取指
func (L *LuaState) fetch() Instruction {
    if L.hookmask != 0 {
        L.traceExec()
        L.updateBase()
    }
    ins := L.pc[0]
    L.pc = L.pc[1:]
    return ins
}
```

### 6.2 指令解码

```go
type Instruction uint32

const (
    POS_OP  = 0
    SIZE_OP = 7
    POS_A   = 7
    SIZE_A  = 8
    POS_k   = 15
    POS_B   = 16
    SIZE_B  = 8
    POS_C   = 24
    SIZE_C  = 8
)

func (i Instruction) Opcode() OpCode {
    return OpCode((i >> POS_OP) & 0x7F)
}

func (i Instruction) A() int {
    return int((i >> POS_A) & 0xFF)
}

func (i Instruction) B() int {
    return int((i >> POS_B) & 0xFF)
}

func (i Instruction) C() int {
    return int((i >> POS_C) & 0xFF)
}

func (i Instruction) sBx() int {
    bx := int((i >> POS_B) & 0x3FFFF)  // 17 bits
    return bx - 131071                 // OFFSET_sBx
}

func (i Instruction) k() bool {
    return (i & (1 << POS_k)) != 0
}
```

### 6.3 寄存器访问

```go
func (L *LuaState) RA(i Instruction) int {
    return L.ci.Base() + i.A()
}

func (L *LuaState) RB(i Instruction) int {
    return L.ci.Base() + i.B()
}

func (L *LuaState) RC(i Instruction) int {
    return L.ci.Base() + i.C()
}

// 常量
func (cl *LClosure) K(i int) *TValue {
    return &cl.p.k[i]
}

// RKC: 常量或寄存器
func (L *LuaState) RKC(i Instruction) *TValue {
    if i.k() {
        return L.ci.Closure().K(i.C())
    }
    return &L.stack[L.RC(i)]
}
```

### 6.4 关键指令实现

**OP_CALL**:
```go
func (L *LuaState) opCall(i Instruction) {
    funcIdx := L.RA(i)
    nResults := i.C() - 1
    
    // 设置参数
    if i.B() != 0 {
        L.top = funcIdx + i.B()
    }
    
    // 调用
    newCI := L.preCall(funcIdx, nResults)
    if newCI != nil {
        L.ci = newCI
        continue  // 跳转到新函数
    }
}
```

**OP_RETURN**:
```go
func (L *LuaState) opReturn(i Instruction) {
    ra := L.RA(i)
    n := i.B() - 1
    if n < 0 {
        n = L.top - ra
    }
    
    // 关闭 upvalue
    if i.k() {
        L.closeUpvals(L.ci.Base())
    }
    
    // 设置返回
    L.top = ra + n
    L.posCall(n)
    
    // 返回或继续
    if L.ci.Status()&CIST_FRESH != 0 {
        return
    }
    L.ci = L.ci.Previous
}
```

**OP_GETTABLE**:
```go
func (L *LuaState) opGetTable(i Instruction) {
    ra := L.RA(i)
    table := L.stack[ra].(*Table)
    key := L.stack[L.RB(i)]
    
    var val TValue
    var tag lu_byte
    
    // 快速路径
    if key.IsInteger() {
        val, tag = table.FastGetInt(key.Int())
    } else {
        val, tag = table.Get(key)
    }
    
    // 处理缺失
    if tag == LUA_TNIL {
        val, tag = L.finishGet(table, key)
    }
    
    L.stack[ra] = val
    
    // 写屏障
    if tag.IsCollectable() {
        L.writeBarrier(table, val)
    }
}
```

### 6.5 性能优化

1. **跳转表**：GCC 下使用 `&&case_label` 跳转表
2. **内联快速路径**：整数表访问直接内联
3. **分派缓存**：高频指令序列优化
4. **Proto 内联**：小函数无函数调用开销

### 6.6 常见陷阱

| 陷阱 | 后果 | 解决方案 |
|------|------|----------|
| PC 更新错误 | 指令执行顺序错乱 | 使用 `pc += offset` 后立即取指 |
| base 计算错误 | 寄存器索引错误 | `base = func + 1`，注意变参修正 |
| 栈边界检查 | 栈溢出或下溢 | 每个指令前 assert |
| 元方法递归 | 无限循环 | `MAXTAGLOOP` 限制 |
| 写屏障遗漏 | GC 不正确 | 所有写操作后调用 |

---

## 7. 完整指令清单

| 类别 | 指令 | 格式 | 说明 |
|------|------|------|------|
| **常量** | OP_LOADNIL, OP_LOADFALSE, OP_LOADTRUE | iABC | 加载常量 nil/true/false |
| | OP_LOADI, OP_LOADF | iAsBx | 加载有符号整数/浮点 |
| | OP_LOADK | iABx | 加载常量表项 |
| | OP_LOADKX | iAx | 加载扩展常量 |
| **表** | OP_GETTABLE, OP_SETTABLE | iABC | 表访问 |
| | OP_GETI, OP_SETI | iABC | 整数键表访问 |
| | OP_GETFIELD, OP_SETFIELD | iABC | 字符串键表访问 |
| | OP_NEWTABLE | ivABC | 创建表 |
| | OP_SELF | iABC | obj.method 语法糖 |
| **算术** | OP_ADD, OP_SUB, OP_MUL, OP_DIV | iABC | 二元运算 |
| | OP_IDIV, OP_MOD, OP_POW | iABC | 整除/取模/幂 |
| | OP_ADDI, OP_SUBI... | iAsBx | 带立即数运算 |
| | OP_UNM, OP_LEN | iABC | 一元运算 |
| **位运算** | OP_BAND, OP_BOR, OP_BXOR | iABC | 按位与/或/异或 |
| | OP_SHL, OP_SHR | iABC | 移位 |
| **控制流** | OP_JMP | isJ | 跳转 |
| | OP_CALL, OP_TAILCALL | iABC | 调用/尾调用 |
| | OP_RETURN, OP_RETURN0, OP_RETURN1 | iABC | 返回 |
| | OP_EQ, OP_LT, OP_LE | iABC | 比较 |
| **循环** | OP_FORPREP, OP_FORLOOP | iABx | 数值 for 循环 |
| | OP_TFORPREP, OP_TFORCALL, OP_TFORLOOP | iABx | 泛型 for 循环 |
| **闭包** | OP_GETUPVAL, OP_SETUPVAL | iABC | upvalue 访问 |
| | OP_CLOSURE | iABx | 创建闭包 |
| | OP_VARARG | iABC | 变参处理 |
| **其他** | OP_CONCAT | iABC | 字符串拼接 |
| | OP_CLOSE | iABC | 关闭 upvalue |
| | OP_TBC | iABC | to-be-closed 标记 |