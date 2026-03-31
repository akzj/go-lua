# lopcodes 模块规格书

## 模块职责

定义 Lua 5.5 的字节码指令集。包括 OpCode 枚举、指令格式、参数编解码宏。

## 依赖模块

无依赖（最底层）

## 公开 API

```c
/* 指令格式定义 */
typedef enum OpMode { iABC, ivABC, iABx, iAsBx, iAx, isJ } OpMode;

/* 指令解码宏（在 lopcodes.h 中定义） */
#define GET_OPCODE(i) ...
#define SET_OPCODE(i,o) ...
#define GETARG_A(i) ...
#define GETARG_B(i) ...
#define GETARG_C(i) ...
#define GETARG_k(i) ...
#define GETARG_Bx(i) ...
#define GETARG_Ax(i) ...
#define GETARG_sBx(i) ...
#define GETARG_sJ(i) ...

/* 指令创建宏 */
#define CREATE_ABCk(o,a,b,c,k) ...
#define CREATE_ABx(o,a,bc) ...
#define CREATE_Ax(o,a) ...
```

## OpCode 枚举 (48 条指令)

```c
typedef enum {
/*----------------------------------------------------------------------
  name        args    description
------------------------------------------------------------------------*/
OP_MOVE,/*      A B     R[A] := R[B]                          */
OP_LOADI,/*     A sBx   R[A] := sBx                           */
OP_LOADF,/*     A sBx   R[A] := (lua_Number)sBx              */
OP_LOADK,/*     A Bx    R[A] := K[Bx]                        */
OP_LOADKX,/*    A       R[A] := K[extra arg]                  */
OP_LOADFALSE,/*  A       R[A] := false                        */
OP_LFALSESKIP,/*A       R[A] := false; pc++                   */
OP_LOADTRUE,/*  A       R[A] := true                          */
OP_LOADNIL,/*   A B     R[A], ..., R[A+B] := nil             */
OP_GETUPVAL,/*  A B     R[A] := UpValue[B]                   */
OP_SETUPVAL,/*  A B     UpValue[B] := R[A]                   */

OP_GETTABUP,/*  A B C   R[A] := UpValue[B][K[C]]            */
OP_GETTABLE,/*   A B C   R[A] := R[B][R[C]]                  */
OP_GETI,/*      A B C   R[A] := R[B][C]                     */
OP_GETFIELD,/*   A B C   R[A] := R[B][K[C]]                  */

OP_SETTABUP,/*   A B C   UpValue[A][K[B]] := RK(C)           */
OP_SETTABLE,/*   A B C   R[A][R[B]] := RK(C)                 */
OP_SETI,/*      A B C   R[A][B] := RK(C)                    */
OP_SETFIELD,/*   A B C   R[A][K[B]] := RK(C)                  */

OP_NEWTABLE,/*   A vB vC k  R[A] := {}                      */

OP_SELF,/*      A B C   R[A+1] := R[B]; R[A] := R[B][K[C]]  */

OP_ADDI,/*      A B sC   R[A] := R[B] + sC                  */

OP_ADDK,/*      A B C    R[A] := R[B] + K[C]                */
OP_SUBK,/*      A B C    R[A] := R[B] - K[C]                */
OP_MULK,/*      A B C    R[A] := R[B] * K[C]                */
OP_MODK,/*      A B C    R[A] := R[B] % K[C]                */
OP_POWK,/*      A B C    R[A] := R[B] ^ K[C]                */
OP_DIVK,/*      A B C    R[A] := R[B] / K[C]                */
OP_IDIVK,/*     A B C    R[A] := R[B] // K[C]               */

OP_BANDK,/*     A B C    R[A] := R[B] & K[C]                */
OP_BORK,/*      A B C    R[A] := R[B] | K[C]                */
OP_BXORK,/*     A B C    R[A] := R[B] ~ K[C]                */

OP_SHLI,/*      A B sC   R[A] := sC << R[B]                 */
OP_SHRI,/*      A B sC   R[A] := R[B] >> sC                 */

OP_ADD,/*       A B C    R[A] := R[B] + R[C]                */
OP_SUB,/*       A B C    R[A] := R[B] - R[C]                */
OP_MUL,/*       A B C    R[A] := R[B] * R[C]                */
OP_MOD,/*       A B C    R[A] := R[B] % R[C]                */
OP_POW,/*       A B C    R[A] := R[B] ^ R[C]                */
OP_DIV,/*       A B C    R[A] := R[B] / R[C]                */
OP_IDIV,/*      A B C    R[A] := R[B] // R[C]               */

OP_BAND,/*      A B C    R[A] := R[B] & R[C]                */
OP_BOR,/*       A B C    R[A] := R[B] | R[C]                */
OP_BXOR,/*      A B C    R[A] := R[B] ~ R[C]                */
OP_SHL,/*       A B C    R[A] := R[B] << R[C]               */
OP_SHR,/*       A B C    R[A] := R[B] >> R[C]               */

OP_MMBIN,/*     A B C    call C metamethod over R[A] and R[B] */
OP_MMBINI,/*    A sB C k  call C metamethod over R[A] and sB  */
OP_MMBINK,/*    A B C k  call C metamethod over R[A] and K[B] */

OP_UNM,/*       A B     R[A] := -R[B]                         */
OP_BNOT,/*     A B     R[A] := ~R[B]                         */
OP_NOT,/*       A B     R[A] := not R[B]                     */
OP_LEN,/*       A B     R[A] := #R[B]                        */

OP_CONCAT,/*    A B     R[A] := R[A].. ... ..R[A + B - 1]   */

OP_CLOSE,/*     A       close all upvalues >= R[A]             */
OP_TBC,/*       A       mark variable A "to be closed"       */
OP_JMP,/*       sJ      pc += sJ                             */
OP_EQ,/*        A B k   if ((R[A] == R[B]) ~= k) then pc++  */
OP_LT,/*        A B k   if ((R[A] <  R[B]) ~= k) then pc++  */
OP_LE,/*        A B k   if ((R[A] <= R[B]) ~= k) then pc++  */

OP_EQK,/*       A B k   if ((R[A] == K[B]) ~= k) then pc++  */
OP_EQI,/*       A sB k  if ((R[A] == sB) ~= k) then pc++   */
OP_LTI,/*       A sB k  if ((R[A] < sB) ~= k) then pc++    */
OP_LEI,/*       A sB k  if ((R[A] <= sB) ~= k) then pc++   */
OP_GTI,/*       A sB k  if ((R[A] > sB) ~= k) then pc++    */
OP_GEI,/*       A sB k  if ((R[A] >= sB) ~= k) then pc++   */

OP_TEST,/*      A k     if (not R[A] == k) then pc++        */
OP_TESTSET,/*   A B k   if (not R[B] == k) then pc++ else R[A] := R[B] */

OP_CALL,/*      A B C   R[A](R[A+1], ..., R[A+B-1])          */
OP_TAILCALL,/*  A B C k return R[A](R[A+1], ..., R[A+B-1])  */

OP_RETURN,/*     A B C k return R[A], ..., R[A+B-2]           */
OP_RETURN0,/*             return                              */
OP_RETURN1,/*   A       return R[A]                         */

OP_FORLOOP,/*   A Bx    update counters; if loop then pc-=Bx */
OP_FORPREP,/*   A Bx    check and prepare counters           */

OP_TFORPREP,/*  A Bx    create upvalue; pc+=Bx               */
OP_TFORCALL,/*  A C     R[A+4],... := R[A](R[A+1],R[A+2])  */
OP_TFORLOOP,/*  A Bx    if R[A+2] ~= nil then R[A]=R[A+2]; pc-=Bx */

OP_SETLIST,/*   A vB vC k R[A][vC+i] := R[A+i], 1<=i<=vB    */

OP_CLOSURE,/*   A Bx    R[A] := closure(KPROTO[Bx])        */

OP_VARARG,/*    A B C k R[A], ... = varargs                  */

OP_GETVARG,/*  A B C    R[A] := R[B][R[C]]                   */

OP_ERRNNIL,/*   A Bx     raise error if R[A] ~= nil          */

OP_VARARGPREP,/* (adjust varargs)                            */

OP_EXTRAARG/*   Ax       extra (larger) argument              */
} OpCode;
```

## 指令格式

```
        3 3 2 2 2 2 2 2 2 2 2 2 1 1 1 1 1 1 1 1 1 1 0 0 0 0 0 0 0 0 0 0
        1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0
iABC          C(8)     |      B(8)     |k|     A(8)      |   Op(7)     |
ivABC         vC(10)     |     vB(6)   |k|     A(8)      |   Op(7)     |
iABx                Bx(17)               |     A(8)      |   Op(7)     |
iAsBx              sBx (signed)(17)     |     A(8)      |   Op(7)     |
iAx                           Ax(25)                     |   Op(7)     |
isJ                           sJ (signed)(25)            |   Op(7)     |
```

## Go 重写规格

```go
package lua

// OpCode 指令码
type OpCode int

const (
    OP_MOVE OpCode = iota
    OP_LOADI
    OP_LOADF
    OP_LOADK
    OP_LOADKX
    OP_LOADFALSE
    OP_LFALSESKIP
    OP_LOADTRUE
    OP_LOADNIL
    OP_GETUPVAL
    OP_SETUPVAL
    OP_GETTABUP
    OP_GETTABLE
    OP_GETI
    OP_GETFIELD
    OP_SETTABUP
    OP_SETTABLE
    OP_SETI
    OP_SETFIELD
    OP_NEWTABLE
    OP_SELF
    OP_ADDI
    OP_ADDK
    OP_SUBK
    OP_MULK
    OP_MODK
    OP_POWK
    OP_DIVK
    OP_IDIVK
    OP_BANDK
    OP_BORK
    OP_BXORK
    OP_SHLI
    OP_SHRI
    OP_ADD
    OP_SUB
    OP_MUL
    OP_MOD
    OP_POW
    OP_DIV
    OP_IDIV
    OP_BAND
    OP_BOR
    OP_BXOR
    OP_SHL
    OP_SHR
    OP_MMBIN
    OP_MMBINI
    OP_MMBINK
    OP_UNM
    OP_BNOT
    OP_NOT
    OP_LEN
    OP_CONCAT
    OP_CLOSE
    OP_TBC
    OP_JMP
    OP_EQ
    OP_LT
    OP_LE
    OP_EQK
    OP_EQI
    OP_LTI
    OP_LEI
    OP_GTI
    OP_GEI
    OP_TEST
    OP_TESTSET
    OP_CALL
    OP_TAILCALL
    OP_RETURN
    OP_RETURN0
    OP_RETURN1
    OP_FORLOOP
    OP_FORPREP
    OP_TFORPREP
    OP_TFORCALL
    OP_TFORLOOP
    OP_SETLIST
    OP_CLOSURE
    OP_VARARG
    OP_GETVARG
    OP_ERRNNIL
    OP_VARARGPREP
    OP_EXTRAARG
    
    NUM_OPCODES
)

// Instruction 字节码 (32-bit unsigned)
type Instruction uint32

// OpMode 指令格式
type OpMode int

const (
    IABC OpMode = iota
    IVABC
    IABx
    IASBx
    IAx
    ISJ
)

// 指令编解码
const (
    SIZE_OP  = 7
    POS_OP   = 0
    SIZE_A   = 8
    POS_A    = POS_OP + SIZE_OP  // 7
    POS_k    = POS_A + SIZE_A    // 15
    SIZE_B   = 8
    POS_B    = POS_k + 1         // 16
    SIZE_C   = 8
    POS_C    = POS_B + SIZE_B   // 24
    SIZE_Bx  = SIZE_C + SIZE_B + 1  // 17
    SIZE_Ax  = SIZE_Bx + SIZE_A  // 25
)

// 获取 opcode
func GetOpCode(i Instruction) OpCode {
    return OpCode((i >> POS_OP) & 0x7F)
}

// 设置 opcode
func SetOpCode(i *Instruction, op OpCode) {
    *i = (*i & 0xFFFFFF80) | (Instruction(op) << POS_OP)
}

// 获取参数 A
func GetArgA(i Instruction) int {
    return int((i >> POS_A) & 0xFF)
}

// 获取参数 B
func GetArgB(i Instruction) int {
    return int((i >> POS_B) & 0xFF)
}

// 获取参数 C
func GetArgC(i Instruction) int {
    return int((i >> POS_C) & 0xFF)
}

// 获取 Bx
func GetArgBx(i Instruction) int {
    return int((i >> POS_k) & 0x1FFFF)
}

// 获取 sBx (signed)
func GetArgsBx(i Instruction) int {
    return int(i>>POS_k) - 0x1FFFF>>1
}

// 获取 k 位
func GetArgk(i Instruction) bool {
    return (i & (1 << POS_k)) != 0
}

// 创建指令
func CreateABCk(op OpCode, a, b, c int, k bool) Instruction {
    i := Instruction(op)
    i |= Instruction(a) << POS_A
    i |= Instruction(b) << POS_B
    i |= Instruction(c) << POS_C
    if k {
        i |= 1 << POS_k
    }
    return i
}

func CreateABx(op OpCode, a, bc int) Instruction {
    i := Instruction(op)
    i |= Instruction(a) << POS_A
    i |= Instruction(bc) << POS_k
    return i
}
```

## RK 常量 (Register or Constant)

```go
// RK(x): 如果 k=1 是常量表索引，否则是寄存器索引
func (L *LuaState) RKValue(inst Instruction, kBit bool) *TValue {
    if kBit {
        // K[x] - 从常量表获取
        idx := GetArgC(inst) // 或 GetArgBx 等
        return &L.Cl().Proto.K[idx]
    } else {
        // R[x] - 从栈获取
        idx := GetArgC(inst)
        base := L.Base()
        return &L.Stack[base+idx]
    }
}
```

## 指令模式属性

```go
// luaP_opmodes 数组定义每个指令的属性
// bit 0-2: op mode
// bit 3:   set A register
// bit 4:   test (next instruction must be jump)
// bit 5:   uses L->top set by previous instruction
// bit 6:   sets L->top for next instruction
// bit 7:   metamethod instruction

var OpModes = [NUM_OPCODES]uint8{
    // 每个 opcode 的属性
    // ...
}

func getOpMode(m OpCode) OpMode {
    return OpMode(OpModes[m] & 7)
}

func testAMode(m OpCode) bool {
    return (OpModes[m] & (1 << 3)) != 0
}

func testTMode(m OpCode) bool {
    return (OpModes[m] & (1 << 4)) != 0
}
```

## 验证测试

```go
func TestOpCodes(t *testing.T) {
    // 测试指令编解码
    i := CreateABCk(OP_ADD, 0, 1, 2, false)
    assert(GetOpCode(i) == OP_ADD)
    assert(GetArgA(i) == 0)
    assert(GetArgB(i) == 1)
    assert(GetArgC(i) == 2)
    assert(!GetArgk(i))
    
    // 测试带常量的指令
    i = CreateABCk(OP_ADDK, 0, 1, 42, true)
    assert(GetArgk(i) == true)
}
```