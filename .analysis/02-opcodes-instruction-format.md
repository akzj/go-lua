# 02 — Lua 5.5.1 Opcodes & Instruction Format

> **Source**: `lua-master/lopcodes.h` (439 lines) + `lua-master/lopcodes.c` (140 lines) + `lua-master/lopnames.h` (105 lines)  
> **Reference C file lines are cited throughout as `lopcodes.h:N` or `lopcodes.c:N`.**  
> **Lua version**: 5.5.1 (not 5.4). No explicit version markers found in these files.

---

## 1. Instruction Encoding: The Six Formats

Every Lua VM instruction is an unsigned 32-bit integer (`uint32_t`). All instructions share the same opcode field: **7 bits at bit position 0** (`SIZE_OP = 7`, `POS_OP = 0`). This limits Lua to **128 opcodes maximum** (0–127). The remaining 25 bits encode operands in one of six formats (`enum OpMode` at `lopcodes.h:36`):

### 1.1 `iABC` — Standard three-register format

```
  3 3 2 2 2 2 2 2 2 2 2 2 1 1 1 1 1 1 1 1 1 1 0 0 0 0 0 0 0 0 0 0
  1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0
|       C(8)       |      B(8)     |k|     A(8)      |   Op(7)     |
  31            24 | 23          16 |15| 14           7 | 6          0|
```

- **Op** = 7 bits (bits 0–6): opcode index
- **A** = 8 bits (bits 7–14): register or result index
- **k** = 1 bit (bit 15): "extra argument" flag (see §6)
- **B** = 8 bits (bits 16–23): second source operand (or upvalue index, etc.)
- **C** = 8 bits (bits 24–31): third source operand (or table index, etc.)

Layout constants (`lopcodes.h:42–60`):
```c
#define SIZE_C   8
#define SIZE_B   8
#define SIZE_Bx  (SIZE_C + SIZE_B + 1)  // 17
#define SIZE_A   8
#define SIZE_Ax  (SIZE_Bx + SIZE_A)    // 25
#define SIZE_sJ  (SIZE_Bx + SIZE_A)    // 25

#define POS_OP   0
#define POS_A    (POS_OP + SIZE_OP)    // 7
#define POS_k    (POS_A + SIZE_A)       // 15
#define POS_B    (POS_k + 1)            // 16
#define POS_C    (POS_B + SIZE_B)       // 24
#define POS_Bx   POS_k                  // 15 (Bx, sBx share position)
```

**Arg ranges**: `MAXARG_A = 255`, `MAXARG_B = 255`, `MAXARG_C = 255`.

---

### 1.2 `ivABC` — Variant ABC (variable-width B and C)

```
|       vC(10)      |  vB(6)  |k|     A(8)      |   Op(7)     |
  31             22 |21     16|15| 14           7 | 6          0|
```

Used only by `OP_NEWTABLE` and `OP_SETLIST`. This squeezes more bits into C (10 bits, range 0–1023) at the cost of B (6 bits, range 0–63). The variant positions `POS_vB` and `POS_vC` are defined at `lopcodes.h:58,60`:
```c
#define POS_vB  (POS_k + 1)  // 16 (same as POS_B)
#define POS_vC  (POS_vB + SIZE_vB)  // 22
```

**Arg ranges**: `MAXARG_vB = 63`, `MAXARG_vC = 1023`.

---

### 1.3 `iABx` — Extended B (no k bit)

```
|              Bx(17)              |     A(8)      |   Op(7)     |
  31                             15 | 14           7 | 6          0|
```

No k bit. The 17 bits that normally hold `C|k|B` are consolidated into a single extended unsigned argument `Bx`. Used for constant-table indices (`OP_LOADK`, `OP_LOADKX`), prototype indices (`OP_CLOSURE`, `OP_FORLOOP`, `OP_FORPREP`, `OP_TFORPREP`, `OP_TFORLOOP`, `OP_ERRNNIL`).

**Arg range**: `MAXARG_Bx = (1<<17) - 1 = 131,071` (`lopcodes.h:83`).

---

### 1.4 `iAsBx` — Signed extended B

```
|              sBx(17 signed)       |     A(8)      |   Op(7)     |
  31                             15 | 14           7 | 6          0|
```

Same bit layout as `iABx`, but `sBx` is interpreted as a **signed** integer using "excess-K" encoding. Used for arithmetic immediate operands (`OP_LOADI`, `OP_LOADF`) and PC-relative jumps in for-loops.

**Signed encoding trick**: `OFFSET_sBx = MAXARG_Bx >> 1 = 65,535` (`lopcodes.h:88`). To encode: `stored_value = signed_value + OFFSET_sBx`. To decode: `signed_value = stored_value - OFFSET_sBx`. This maps the unsigned range [0, 131071] to signed range [-65535, +65536].

**⚠️ TRAP — off-by-one asymmetry**: The positive range has one extra value. `MAXARG_Bx = 131071` is odd, so `OFFSET_sBx = 65535`. The unsigned range 0–131071 maps to:
- 0 → -65535
- 65535 → 0
- 65536 → +1
- 131071 → +65536

The zero point is at unsigned 65535, NOT at 65536. There is no way to store `sBx = 0` as an immediate in a raw encoding — it always needs the offset applied.

---

### 1.5 `iAx` — Full-width A (25 bits of operand)

```
|                         Ax(25)                         |   Op(7)     |
  31                                                     7 | 6          0|
```

A takes up all remaining bits (25 bits). Only `OP_EXTRAARG` uses this format (`lopcodes.c:108`). It provides the maximum operand space: `MAXARG_Ax = 33,554,431`.

---

### 1.6 `isJ` — Signed J (jump offset)

```
|                       sJ(25 signed)                     |   Op(7)     |
  31                                                     7 | 6          0|
```

Only `OP_JMP` uses this format (`lopcodes.c:80`). Same encoding as `iAx` but `sJ` is signed using excess-K with `OFFSET_sJ = MAXARG_sJ >> 1 = 16,777,215`. Range: [-16777215, +16777216].

---

## 2. The k-bit (Extra Argument) Mechanism

The `k` bit (bit 15, `POS_k`) appears in `iABC` and `ivABC` formats. It signals that one of the other operands is an **index into the constant table** rather than a register number.

**Encoding RK(x)** (`lopcodes.h:222`):
```c
R[x] - register
K[x] - constant (in constant table)
RK(x) == if k(i) then K[x] else R[x]
```

In the C operand fields, when `k = 1`, the **B or C field encodes a constant-table index** (offset by `MAXARG_B + 1 = 256`). The threshold is:
- If operand value < 256 → it's a **register** index
- If operand value >= 256 → it's a **constant table index minus 256**

This encoding compresses two lookup paths into one operand field. The threshold value `256` equals `MAXARG_B + 1 = (1<<8)`, which is **intentionally one more than the maximum register index** (`MAXARG_A = 255`).

**The k-bit itself** (`lopcodes.h:157–159`):
- `TESTARG_k(i)` — tests if the k bit is set (non-zero = true)
- `GETARG_k(i)` — extracts the k bit value (0 or 1)
- `SETARG_k(i,v)` — sets the k bit

**Where k is used** (`lopcodes.h:375–388`):
- `OP_LOADKX` — next instruction is `OP_EXTRAARG` carrying the actual constant index
- `OP_NEWTABLE` — if k=1, array size encoded as `(EXTRAARG << 10) | vC` (concatenated)
- `OP_SETLIST` — if k=1, real C = `(EXTRAARG << 10) | vC`
- `OP_MMBINI`/`OP_MMBINK` — k signals argument order was flipped
- `OP_EQK` — B indexes the constant table directly (no +256 encoding needed)
- `OP_CALL`, `OP_TAILCALL`, `OP_RETURN`, `OP_VARARGPREP` — k signals upvalue handling

---

## 3. Every Opcode: Full Reference

**`NUM_OPCODES = 77`** (computed as `OP_EXTRAARG + 1 = 76 + 1 = 77` at `lopcodes.h:351`).  
**The Go implementation defines 85 opcodes (0–84)** including `OP_SETTABLEN` (85) and `OP_EXTRAARG` (84), which differs from the C source. The C source has 77 opcodes (0–76).

### 3.1 Format Summary Table

| Opcode | Value | Format | Args | Semantic |
|---|---|---|---|---|
| OP_MOVE | 0 | iABC | A B | `R[A] := R[B]` |
| OP_LOADI | 1 | iAsBx | A sBx | `R[A] := sBx` (integer literal) |
| OP_LOADF | 2 | iAsBx | A sBx | `R[A] := (lua_Number)sBx` (float literal) |
| OP_LOADK | 3 | iABx | A Bx | `R[A] := K[Bx]` (constant table entry) |
| OP_LOADKX | 4 | iABx | A | `R[A] := K[extra arg]` (next instr is OP_EXTRAARG) |
| OP_LOADFALSE | 5 | iABC | A | `R[A] := false` |
| OP_LFALSESKIP | 6 | iABC | A | `R[A] := false; pc++` (skip next instr) |
| OP_LOADTRUE | 7 | iABC | A | `R[A] := true` |
| OP_LOADNIL | 8 | iABC | A B | `R[A]...R[A+B] := nil` |
| OP_GETUPVAL | 9 | iABC | A B | `R[A] := UpValue[B]` |
| OP_SETUPVAL | 10 | iABC | A B | `UpValue[B] := R[A]` |
| OP_GETTABUP | 11 | iABC | A B C | `R[A] := UpValue[B][K[C]]` |
| OP_GETTABLE | 12 | iABC | A B C | `R[A] := R[B][R[C]]` |
| OP_GETI | 13 | iABC | A B C | `R[A] := R[B][C]` (C is literal integer) |
| OP_GETFIELD | 14 | iABC | A B C | `R[A] := R[B][K[C]]` (C is short string constant) |
| OP_SETTABUP | 15 | iABC | A B C k | `UpValue[A][K[B]] := RK(C)` |
| OP_SETTABLE | 16 | iABC | A B C k | `R[A][R[B]] := RK(C)` |
| OP_SETI | 17 | iABC | A B C k | `R[A][B] := RK(C)` (B is literal integer) |
| OP_SETFIELD | 18 | iABC | A B C k | `R[A][K[B]] := RK(C)` (B is short string constant) |
| OP_NEWTABLE | 19 | ivABC | A vB vC k | `R[A] := {}` (array+hash init) |
| OP_SELF | 20 | iABC | A B C | `R[A+1] := R[B]; R[A] := R[B][K[C]]` |
| OP_ADDI | 21 | iABC | A B sC | `R[A] := R[B] + sC` (sC is signed 8-bit) |
| OP_ADDK | 22 | iABC | A B C k | `R[A] := R[B] + K[C]` |
| OP_SUBK | 23 | iABC | A B C k | `R[A] := R[B] - K[C]` |
| OP_MULK | 24 | iABC | A B C k | `R[A] := R[B] * K[C]` |
| OP_MODK | 25 | iABC | A B C k | `R[A] := R[B] % K[C]` |
| OP_POWK | 26 | iABC | A B C k | `R[A] := R[B] ^ K[C]` |
| OP_DIVK | 27 | iABC | A B C k | `R[A] := R[B] / K[C]` |
| OP_IDIVK | 28 | iABC | A B C k | `R[A] := R[B] // K[C]` (floor division) |
| OP_BANDK | 29 | iABC | A B C k | `R[A] := R[B] & K[C]` (bitwise) |
| OP_BORK | 30 | iABC | A B C k | `R[A] := R[B] \| K[C]` (bitwise or) |
| OP_BXORK | 31 | iABC | A B C k | `R[A] := R[B] ~ K[C]` (bitwise xor) |
| OP_SHLI | 32 | iABC | A B sC | `R[A] := sC << R[B]` |
| OP_SHRI | 33 | iABC | A B sC | `R[A] := R[B] >> sC` |
| OP_ADD | 34 | iABC | A B C | `R[A] := R[B] + R[C]` |
| OP_SUB | 35 | iABC | A B C | `R[A] := R[B] - R[C]` |
| OP_MUL | 36 | iABC | A B C | `R[A] := R[B] * R[C]` |
| OP_MOD | 37 | iABC | A B C | `R[A] := R[B] % R[C]` |
| OP_POW | 38 | iABC | A B C | `R[A] := R[B] ^ R[C]` |
| OP_DIV | 39 | iABC | A B C | `R[A] := R[B] / R[C]` |
| OP_IDIV | 40 | iABC | A B C | `R[A] := R[B] // R[C]` |
| OP_BAND | 41 | iABC | A B C | `R[A] := R[B] & R[C]` |
| OP_BOR | 42 | iABC | A B C | `R[A] := R[B] \| R[C]` |
| OP_BXOR | 43 | iABC | A B C | `R[A] := R[B] ~ R[C]` |
| OP_SHL | 44 | iABC | A B C | `R[A] := R[B] << R[C]` |
| OP_SHR | 45 | iABC | A B C | `R[A] := R[B] >> R[C]` |
| OP_MMBIN | 46 | iABC | A B C | metamethod call: `C` metamethod over `R[A]` and `R[B]` |
| OP_MMBINI | 47 | iABC | A sB C k | metamethod call: `C` over `R[A]` and signed `sB` |
| OP_MMBINK | 48 | iABC | A B C k | metamethod call: `C` over `R[A]` and constant `K[B]` |
| OP_UNM | 49 | iABC | A B | `R[A] := -R[B]` |
| OP_BNOT | 50 | iABC | A B | `R[A] := ~R[B]` |
| OP_NOT | 51 | iABC | A B | `R[A] := not R[B]` |
| OP_LEN | 52 | iABC | A B | `R[A] := #R[B]` (length operator) |
| OP_CONCAT | 53 | iABC | A B | `R[A] := R[A].. ... ..R[A+B-1]` |
| OP_CLOSE | 54 | iABC | A | `close all upvalues >= R[A]` |
| OP_TBC | 55 | iABC | A | `mark variable A "to be closed"` |
| OP_JMP | 56 | isJ | sJ | `pc += sJ` |
| OP_EQ | 57 | iABC | A B k | `if ((R[A] == R[B]) ~= k) then pc++` |
| OP_LT | 58 | iABC | A B k | `if ((R[A] < R[B]) ~= k) then pc++` |
| OP_LE | 59 | iABC | A B k | `if ((R[A] <= R[B]) ~= k) then pc++` |
| OP_EQK | 60 | iABC | A B k | `if ((R[A] == K[B]) ~= k) then pc++` |
| OP_EQI | 61 | iABC | A sB k | `if ((R[A] == sB) ~= k) then pc++` |
| OP_LTI | 62 | iABC | A sB k | `if ((R[A] < sB) ~= k) then pc++` |
| OP_LEI | 63 | iABC | A sB k | `if ((R[A] <= sB) ~= k) then pc++` |
| OP_GTI | 64 | iABC | A sB k | `if ((R[A] > sB) ~= k) then pc++` |
| OP_GEI | 65 | iABC | A sB k | `if ((R[A] >= sB) ~= k) then pc++` |
| OP_TEST | 66 | iABC | A k | `if (not R[A] == k) then pc++` |
| OP_TESTSET | 67 | iABC | A B k | `if (not R[B] == k) then pc++ else R[A] := R[B]` |
| OP_CALL | 68 | iABC | A B C k | `R[A]..R[A+C-2] := R[A](R[A+1]..R[A+B-1])` |
| OP_TAILCALL | 69 | iABC | A B C k | `return R[A](R[A+1]..R[A+B-1])` |
| OP_RETURN | 70 | iABC | A B C k | `return R[A]..R[A+B-2]` |
| OP_RETURN0 | 71 | iABC | — | `return` (no arguments) |
| OP_RETURN1 | 72 | iABC | A | `return R[A]` |
| OP_FORLOOP | 73 | iABx | A Bx | `update counters; if continues pc -= Bx` |
| OP_FORPREP | 74 | iABx | A Bx | `check and prepare counters; if skip pc += Bx+1` |
| OP_TFORPREP | 75 | iABx | A Bx | `create upvalue for R[A+3]; pc += Bx` |
| OP_TFORCALL | 76 | iABC | A C | `R[A+4]..R[A+3+C] := R[A](R[A+1], R[A+2])` |
| OP_TFORLOOP | 77 | iABx | A Bx | `if R[A+2] ~= nil then R[A]:=R[A+2]; pc -= Bx` |
| OP_SETLIST | 78 | ivABC | A vB vC k | `R[A][vC+i] := R[A+i], 1 <= i <= vB` |
| OP_CLOSURE | 79 | iABx | A Bx | `R[A] := closure(KPROTO[Bx])` |
| OP_VARARG | 80 | iABC | A B C k | `R[A]..R[A+C-2] = varargs` |
| OP_GETVARG | 81 | iABC | A B C | `R[A] := R[B][R[C]]` (B is vararg parameter) |
| OP_ERRNNIL | 82 | iABx | A Bx | `if R[A] ~= nil then error` (K[Bx-1] is global name) |
| OP_VARARGPREP | 83 | iABC | A k | `adjust varargs` |
| OP_EXTRAARG | 84 | iAx | Ax | `extra (larger) argument for previous opcode` |

> **Note on opcode count discrepancy**: The C source (`lopcodes.h:351`) defines `NUM_OPCODES = 77` (OP_EXTRAARG = 76). The Go implementation defines `NUM_OPCODES = 85` (OP_EXTRAARG = 84) including `OP_SETTABLEN` (85). The Go implementation appears to have added two opcodes beyond the reference C source. The analysis above covers the 77 C opcodes (0–76). The Go opcodes `OP_SETTABLEN` (85) is not in the C source.

---

## 4. Opcode Grouping by Category

### 4.1 Load & Constants (0–8, 84)
- **Register movement**: `OP_MOVE`
- **Immediate loads**: `OP_LOADI` (integer), `OP_LOADF` (float), `OP_LOADFALSE`, `OP_LOADTRUE`, `OP_LOADNIL`
- **Constant loads**: `OP_LOADK` (via Bx), `OP_LOADKX` (via OP_EXTRAARG)
- **Conditional false skip**: `OP_LFALSESKIP`
- **Extra argument carrier**: `OP_EXTRAARG`

### 4.2 Upvalue Access (9–10)
- `OP_GETUPVAL`, `OP_SETUPVAL`

### 4.3 Table Access / GET (11–14)
- `OP_GETTABUP` (upvalue table), `OP_GETTABLE` (register table), `OP_GETI` (integer key), `OP_GETFIELD` (string constant key)

### 4.4 Table Store / SET (15–18)
- `OP_SETTABUP`, `OP_SETTABLE`, `OP_SETI`, `OP_SETFIELD`
- All use RK(C) encoding

### 4.5 Table Creation (19)
- `OP_NEWTABLE` — uses ivABC with vB (log2 hash size + 1), vC (array size), k (extended sizes via OP_EXTRAARG)

### 4.6 Object Method Access (20)
- `OP_SELF` — optimized method call setup: stores object in R[A+1], method in R[A]

### 4.7 Arithmetic with Constants — "K" variants (21–33)
- Binary op with constant: `OP_ADDK`, `OP_SUBK`, `OP_MULK`, `OP_MODK`, `OP_POWK`, `OP_DIVK`, `OP_IDIVK`, `OP_BANDK`, `OP_BORK`, `OP_BXORK`
- Binary op with immediate: `OP_ADDI` (add signed 8-bit), `OP_SHLI`, `OP_SHRI`

### 4.8 Arithmetic with Registers (34–45)
- `OP_ADD`, `OP_SUB`, `OP_MUL`, `OP_MOD`, `OP_POW`, `OP_DIV`, `OP_IDIV`, `OP_BAND`, `OP_BOR`, `OP_BXOR`, `OP_SHL`, `OP_SHR`

### 4.9 Metamethod Fallback (46–48)
- `OP_MMBIN` (both register), `OP_MMBINI` (immediate), `OP_MMBINK` (constant). These follow arithmetic/bitwise ops when the operation can't be resolved to primitive types.

### 4.10 Unary Operations (49–52)
- `OP_UNM`, `OP_BNOT`, `OP_NOT`, `OP_LEN`

### 4.11 String Concatenation (53)
- `OP_CONCAT` — concatenates B registers starting from R[A]: `R[A] = R[A] .. R[A+1] .. ... .. R[A+B-1]`

### 4.12 Upvalue Lifecycle (54–55)
- `OP_CLOSE` — close all upvalues >= R[A]
- `OP_TBC` — mark variable as "to be closed" (for `__close` metamethod)

### 4.13 Jump / Control Flow (56)
- `OP_JMP` — unconditional PC-relative jump with signed sJ offset

### 4.14 Comparison (57–65)
- Register-register: `OP_EQ`, `OP_LT`, `OP_LE`
- Constant comparison: `OP_EQK` (constant K[B])
- Immediate comparisons: `OP_EQI`, `OP_LTI`, `OP_LEI`, `OP_GTI`, `OP_GEI`
- All use k-bit to invert the condition. The next instruction after a comparison is always assumed to be a jump.

### 4.15 Test (66–67)
- `OP_TEST` — test register A, skip next if `(not R[A]) == k`
- `OP_TESTSET` — conditional assignment: if `(not R[B]) != k` then jump, else `R[A] := R[B]`. Used for short-circuit evaluation like `a = b or c`.

### 4.16 Function Call (68–72)
- `OP_CALL` — variable results, uses IT/OT modes
- `OP_TAILCALL` — tail-call optimization, always sets OT
- `OP_RETURN`, `OP_RETURN0`, `OP_RETURN1` — return with k-bit for upvalue handling
- C > 0 in RETURN/TAILCALL with k=1: C-1 = number of fixed parameters

### 4.17 For-loop (73–74)
- `OP_FORPREP` — prepare: check types, convert, subtract jump offset
- `OP_FORLOOP` — update counters, test, jump back

### 4.18 Generic For-loop (75–77)
- `OP_TFORPREP` — create upvalue, jump to body
- `OP_TFORCALL` — call iterator
- `OP_TFORLOOP` — test result, loop or exit

### 4.19 Table Initialization (78)
- `OP_SETLIST` — batch table field assignment. vB = count, vC = base index. If k=1, real C = (EXTRAARG << 10) | vC. If vB=0, real B = top.

### 4.20 Closure & Varargs (79–83, 81)
- `OP_CLOSURE` — create closure from prototype
- `OP_VARARG` — copy varargs to registers
- `OP_GETVARG` — access vararg parameter
- `OP_VARARGPREP` — adjust for vararg setup

### 4.21 Error Checking (82)
- `OP_ERRNNIL` — runtime check: raise error if R[A] ~= nil. Bx=0 means global name not available for error message.

---

## 5. Signed Encoding: The sBx Trick

Lua uses **excess-K (bias) encoding** for signed operands. The key insight: all instruction operands are stored as **unsigned** 32-bit integers in memory. The signed interpretation is applied only during decoding.

### 5.1 Excess-K Encoding

For a field of `N` bits, the maximum unsigned value is `2^N - 1`. For `sBx` (17 bits):
```
MAXARG_Bx = 2^17 - 1 = 131,071
OFFSET_sBx = MAXARG_Bx >> 1 = 65,535

stored_value = signed_value + OFFSET_sBx
signed_value = stored_value - OFFSET_sBx
```

This maps:
- unsigned `0` → signed `-65,535`
- unsigned `65,535` → signed `0`
- unsigned `131,071` → signed `+65,536`

### 5.2 Why Excess-K?

Two reasons for this encoding in Lua:

1. **Natural zero-point**: The midpoint of the unsigned range maps to zero. This makes `pc -= Bx` with `Bx = 65,535` a no-op (jump to self), which is useful for loop setup.
2. **No sign bit needed**: All storage is unsigned. Comparison and arithmetic on the stored values work without special-case handling for sign bits.

### 5.3 Signed C in iABC (sC)

The C field in `iABC` can also be signed when used by `OP_ADDI`, `OP_SHLI`, `OP_SHRI`, and comparison immediates. The same excess-K trick applies:
```c
#define OFFSET_sC (MAXARG_C >> 1)  // 127 (lopcodes.h:111)
#define int2sC(i) ((i) + OFFSET_sC)   // encode
#define sC2int(i) ((i) - OFFSET_sC)   // decode
```
For 8-bit sC: range [-127, +128].

### 5.4 sJ for Jumps

`sJ` uses 25-bit excess-K:
```c
#define MAXARG_sJ ((1 << SIZE_sJ) - 1)  // 33,554,431 (lopcodes.h:98)
#define OFFSET_sJ (MAXARG_sJ >> 1)      // 16,777,215 (lopcodes.h:103)
```
Range: [-16,777,215, +16,777,216]. This is the range for `OP_JMP`.

---

## 6. Instruction Decode/Encode Pseudocode (Go-flavored)

### 6.1 Bit Masks

```go
// From lopcodes.h:117-121
const (
    SIZE_OP = 7
    SIZE_A  = 8
    SIZE_B  = 8
    SIZE_C  = 8
    SIZE_Bx = SIZE_C + SIZE_B + 1  // 17
    SIZE_Ax = SIZE_Bx + SIZE_A     // 25
    SIZE_sJ = SIZE_Bx + SIZE_A     // 25
    SIZE_vB = 6
    SIZE_vC = 10

    POS_OP  = 0
    POS_A   = POS_OP + SIZE_OP      // 7
    POS_k   = POS_A + SIZE_A        // 15
    POS_B   = POS_k + 1             // 16
    POS_C   = POS_B + SIZE_B        // 24
    POS_Bx  = POS_k                 // 15
    POS_Ax  = POS_A                 // 7
    POS_sJ  = POS_A                 // 7
    POS_vB  = POS_k + 1              // 16
    POS_vC  = POS_vB + SIZE_vB      // 22
)

// Limits
const (
    MAXARG_A   = (1 << SIZE_A) - 1   // 255
    MAXARG_B   = (1 << SIZE_B) - 1   // 255
    MAXARG_C   = (1 << SIZE_C) - 1   // 255
    MAXARG_Bx  = (1 << SIZE_Bx) - 1  // 131071
    MAXARG_Ax  = (1 << SIZE_Ax) - 1  // 33554431
    MAXARG_sJ  = (1 << SIZE_sJ) - 1  // 33554431
    MAXARG_vB  = (1 << SIZE_vB) - 1  // 63
    MAXARG_vC  = (1 << SIZE_vC) - 1  // 1023

    OFFSET_sBx = MAXARG_Bx >> 1     // 65535
    OFFSET_sJ  = MAXARG_sJ >> 1     // 16777215
    OFFSET_sC  = MAXARG_C >> 1      // 127

    NO_REG     = MAXARG_A            // 255 (invalid register sentinel)
    MAX_FSTACK = MAXARG_A            // 255 (max stack depth)
)

// MASK1: create mask with n 1-bits at position p
func MASK1(n, p uint) uint32 {
    return ^((^(uint32(0))) << n) << p
}

// MASK0: create mask with n 0-bits at position p
func MASK0(n, p uint) uint32 {
    return ^MASK1(n, p)
}
```

### 6.2 Extraction (GETARG_*)

```go
// getarg extracts n bits starting at position p from instruction i
// Equivalent to: ((i) >> p) & MASK1(n, 0)
// From lopcodes.h:134
func getarg(i Instruction, pos, size uint) int {
    return int(uint32(i)>>pos) & int((uint32(1)<<size)-1)
}

// GET_OPCODE extracts the opcode (7 bits at position 0)
// From lopcodes.h:127
func GET_OPCODE(i Instruction) OpCode {
    return OpCode(uint32(i) >> POS_OP & ((1 << SIZE_OP) - 1))
}

// GETARG_A: extract A (8 bits, position 7)
// From lopcodes.h:138
func GETARG_A(i Instruction) int {
    return getarg(i, POS_A, SIZE_A)
}

// GETARG_B: extract B for iABC format only
// Note: uses check_exp — validates instruction format at runtime
// From lopcodes.h:141-142
func GETARG_B(i Instruction) int {
    if getOpMode(GET_OPCODE(i)) != OpModeABC {
        panic("GETARG_B: not an ABC instruction")
    }
    return getarg(i, POS_B, SIZE_B)
}

// GETARG_sB: extract signed B (applied to unsigned B)
// From lopcodes.h:145
func GETARG_sB(i Instruction) int {
    return sC2int(GETARG_B(i))
}

// GETARG_C: extract C for iABC format only
// From lopcodes.h:149-150
func GETARG_C(i Instruction) int {
    if getOpMode(GET_OPCODE(i)) != OpModeABC {
        panic("GETARG_C: not an ABC instruction")
    }
    return getarg(i, POS_C, SIZE_C)
}

// GETARG_sC: extract signed C
// From lopcodes.h:153
func GETARG_sC(i Instruction) int {
    return sC2int(GETARG_C(i))
}

// GETARG_k: extract the k bit (1 bit at position 15)
// From lopcodes.h:157-158
func GETARG_k(i Instruction) int {
    return int(uint32(i) & (uint32(1) << POS_k))
}

// GETARG_Bx: extract Bx for iABx format
// From lopcodes.h:161
func GETARG_Bx(i Instruction) int {
    if getOpMode(GET_OPCODE(i)) != OpModeABx {
        panic("GETARG_Bx: not an ABx instruction")
    }
    return getarg(i, POS_Bx, SIZE_Bx)
}

// GETARG_sBx: extract signed sBx for iAsBx format
// From lopcodes.h:167-168
func GETARG_sBx(i Instruction) int {
    if getOpMode(GET_OPCODE(i)) != OpModeAsBx {
        panic("GETARG_sBx: not an AsBx instruction")
    }
    return getarg(i, POS_Bx, SIZE_Bx) - OFFSET_sBx
}

// GETARG_Ax: extract Ax for iAx format
// From lopcodes.h:164
func GETARG_Ax(i Instruction) int {
    if getOpMode(GET_OPCODE(i)) != OpModeAx {
        panic("GETARG_Ax: not an Ax instruction")
    }
    return getarg(i, POS_Ax, SIZE_Ax)
}

// GETARG_sJ: extract signed sJ for isJ format
// From lopcodes.h:171-172
func GETARG_sJ(i Instruction) int {
    if getOpMode(GET_OPCODE(i)) != OpModeSJ {
        panic("GETARG_sJ: not an SJ instruction")
    }
    return getarg(i, POS_sJ, SIZE_sJ) - OFFSET_sJ
}

// Variant extraction for ivABC format
func GETARG_vB(i Instruction) int {
    if getOpMode(GET_OPCODE(i)) != OpModeVABC {
        panic("GETARG_vB: not a VABC instruction")
    }
    return getarg(i, POS_vB, SIZE_vB)
}

func GETARG_vC(i Instruction) int {
    if getOpMode(GET_OPCODE(i)) != OpModeVABC {
        panic("GETARG_vC: not a VABC instruction")
    }
    return getarg(i, POS_vC, SIZE_vC)
}

// Signed conversion helpers
// From lopcodes.h:111-114
func int2sC(i int) int { return i + OFFSET_sC }
func sC2int(i int) int { return i - OFFSET_sC }
```

### 6.3 Encoding (SETARG_* / CREATE_*)

```go
// setarg: set n bits at position p to value v in instruction i
// From lopcodes.h:135-136
func setarg(i Instruction, v int, pos, size uint) Instruction {
    mask := ^((^(uint32(0))) << size) << pos
    return Instruction(uint32(i)&^mask | (uint32(v)<<pos) & mask)
}

// SET_OPCODE: replace the opcode field
// From lopcodes.h:128-129
func SET_OPCODE(i *Instruction, op OpCode) {
    mask := ^((^(uint32(0))) << SIZE_OP) << POS_OP
    *i = Instruction(uint32(*i)&^mask | (uint32(op)<<POS_OP)&mask)
}

func SETARG_A(i *Instruction, v int)   { setarg(i, v, POS_A, SIZE_A) }
func SETARG_B(i *Instruction, v int)   { setarg(i, v, POS_B, SIZE_B) }
func SETARG_C(i *Instruction, v int)   { setarg(i, v, POS_C, SIZE_C) }
func SETARG_k(i *Instruction, v int)  { setarg(i, v, POS_k, 1) }
func SETARG_Bx(i *Instruction, v int)  { setarg(i, v, POS_Bx, SIZE_Bx) }
func SETARG_Ax(i *Instruction, v int)  { setarg(i, v, POS_Ax, SIZE_Ax) }
func SETARG_vB(i *Instruction, v int)  { setarg(i, v, POS_vB, SIZE_vB) }
func SETARG_vC(i *Instruction, v int)  { setarg(i, v, POS_vC, SIZE_vC) }

// SETARG_sBx: encode signed sBx using excess-K
// From lopcodes.h:169
func SETARG_sBx(i *Instruction, b int) {
    SETARG_Bx(i, b + OFFSET_sBx)
}

// SETARG_sJ: encode signed sJ using excess-K
// From lopcodes.h:173-174
func SETARG_sJ(i *Instruction, j int) {
    setarg(i, j+OFFSET_sJ, POS_sJ, SIZE_sJ)
}

// CREATE_ABCk: create an iABC instruction with k bit
// From lopcodes.h:177-181
func CREATE_ABCk(op OpCode, a, b, c, k int) Instruction {
    return Instruction(uint32(op)<<POS_OP |
        uint32(a)<<POS_A |
        uint32(b)<<POS_B |
        uint32(c)<<POS_C |
        uint32(k)<<POS_k)
}

// CREATE_vABCk: create an ivABC instruction
// From lopcodes.h:183-187
func CREATE_vABCk(op OpCode, a, vb, vc, k int) Instruction {
    return Instruction(uint32(op)<<POS_OP |
        uint32(a)<<POS_A |
        uint32(vb)<<POS_vB |
        uint32(vc)<<POS_vC |
        uint32(k)<<POS_k)
}

// CREATE_ABx: create an iABx instruction
// From lopcodes.h:189-191
func CREATE_ABx(op OpCode, a, bx int) Instruction {
    return Instruction(uint32(op)<<POS_OP |
        uint32(a)<<POS_A |
        uint32(bx)<<POS_Bx)
}

// CREATE_Ax: create an iAx instruction
// From lopcodes.h:193-194
func CREATE_Ax(op OpCode, ax int) Instruction {
    return Instruction(uint32(op)<<POS_OP |
        uint32(ax)<<POS_Ax)
}

// CREATE_sJ: create an isJ instruction
// From lopcodes.h:196-198
func CREATE_sJ(op OpCode, sj int) Instruction {
    return Instruction(uint32(op)<<POS_OP |
        uint32(sj+OFFSET_sJ)<<POS_sJ)
}
```

### 6.4 Opcode Mode Metadata

```go
// Instruction property encoding (from lopcodes.h:416-432):
// bits 0-2: op mode (enum OpMode)
// bit 3: A mode (sets register A)
// bit 4: T mode (is a test, next instruction is a jump)
// bit 5: IT mode (uses L->top from previous instruction when B==0)
// bit 6: OT mode (sets L->top for next instruction when C==0)
// bit 7: MM mode (is a metamethod instruction)

type OpProperty uint8

const (
    OpPropertyMM  OpProperty = 1 << 7  // metamethod instruction
    OpPropertyOT  OpProperty = 1 << 6  // sets top for next instruction
    OpPropertyIT  OpProperty = 1 << 5  // uses top from previous instruction
    OpPropertyT   OpProperty = 1 << 4  // test (next instruction is jump)
    OpPropertyA   OpProperty = 1 << 3  // sets register A
)

// luaP_opmodes array (from lopcodes.c:22-109)
// Each byte: MM(1) | OT(1) | IT(1) | T(1) | A(1) | mode(3)
// Index = OpCode value

// luaP_isOT: checks whether instruction sets top for next instruction
// From lopcodes.c:117-124
func luaP_isOT(i Instruction) bool {
    op := GET_OPCODE(i)
    switch op {
    case OP_TAILCALL:
        return true
    default:
        return testOTMode(op) && GETARG_C(i) == 0
    }
}

// luaP_isIT: checks whether instruction uses top from previous instruction
// From lopcodes.c:131-139
func luaP_isIT(i Instruction) bool {
    op := GET_OPCODE(i)
    switch op {
    case OP_SETLIST:
        return testITMode(op) && GETARG_vB(i) == 0
    default:
        return testITMode(op) && GETARG_B(i) == 0
    }
}
```

---

## 7. If I Were Building This in Go

### 7.1 Type Definitions

```go
package opcodes

// Instruction is a 32-bit unsigned integer encoding a Lua VM instruction.
type Instruction uint32

// OpCode identifies a Lua VM opcode.
type OpCode uint8

// OpMode defines how an instruction encodes its operands.
type OpMode uint8

const (
    OpModeABC  OpMode = 0  // A(8) | k(1) | B(8) | C(8)
    OpModeVABC OpMode = 1  // A(8) | k(1) | vB(6) | vC(10)
    OpModeABx  OpMode = 2  // A(8) | Bx(17)
    OpModeAsBx OpMode = 3  // A(8) | sBx(17 signed)
    OpModeAx   OpMode = 4  // Ax(25)
    OpModeSJ   OpMode = 5  // sJ(25 signed)
)
```

### 7.2 Instruction as a Struct (Alternative Approach)

While the raw `uint32` approach matches the C source, a structured approach is more idiomatic in Go:

```go
// Instruction represents a decoded Lua VM instruction.
// Use Encode() to convert back to uint32 for storage.
type Instruction struct {
    Op   OpCode
    A    int
    B    int  // or vB for variant
    C    int  // or vC for variant
    Bx   int  // for ABx/AsBx
    Ax   int  // for Ax
    sJ   int  // for isJ
    K    int  // extra bit
    Mode OpMode
}

// Encode packs the instruction back into uint32.
func (i Instruction) Encode() Instruction { /* ... */ }

// Decode unpacks a uint32 into an Instruction struct.
func Decode(raw Instruction) Instruction { /* ... */ }
```

However, the raw `uint32` approach used in the existing Go implementation (`opcodes/api/api.go`) is more efficient (no allocation, cache-friendly) and matches the C source directly. The structured approach is better for debugging and disassembly.

### 7.3 Operand Classification: RK(x)

The RK(x) convention needs a helper function:

```go
// RK returns whether operand x is a constant (true) or register (false),
// and the decoded index. If isK is true, the actual index is (x - 256).
// Threshold: values 0-255 are registers, 256+ are (constant_index - 256).
// From lopcodes.h:222 and the k-bit explanation.
func RK(x int) (index int, isK bool) {
    if x > MAXARG_B {  // x >= 256
        return x - (MAXARG_B + 1), true
    }
    return x, false
}
```

### 7.4 OpMode Lookup Table

```go
// luaP_opmodes encodes instruction properties.
// Order must match OpCode enum order exactly.
// From lopcodes.c:22-109
var luaP_opmodes = [NUM_OPCODES]uint8{
    /*       MM OT IT T  A  mode       opcode  */
    0x08, /* opmode(0, 0, 0, 0, 1, iABC)     OP_MOVE = 0     */
    0x0B, /* opmode(0, 0, 0, 0, 1, iAsBx)   OP_LOADI = 1    */
    // ... 77 entries total (0-76), one per opcode
}

func getOpMode(op OpCode) OpMode {
    return OpMode(luaP_opmodes[op] & 7)
}

func testAMode(op OpCode) bool { return (luaP_opmodes[op] & 0x08) != 0 }
func testTMode(op OpCode) bool { return (luaP_opmodes[op] & 0x10) != 0 }
func testITMode(op OpCode) bool { return (luaP_opmodes[op] & 0x20) != 0 }
func testOTMode(op OpCode) bool { return (luaP_opmodes[op] & 0x40) != 0 }
func testMMMode(op OpCode) bool { return (luaP_opmodes[op] & 0x80) != 0 }
```

### 7.5 Current Go Implementation Assessment

The existing Go implementation in `/home/ubuntu/workspace/go-lua/opcodes/` is **well-structured and mostly correct**:

- **`opcodes/api/api.go`**: Correct constants and type definitions matching `lopcodes.h` lines 30–66.
- **`opcodes/internal/internal.go`**: Correct instruction creation and extraction functions matching `lopcodes.c` lines 22–139.
- **`opcodes/internal/internal_test.go`**: Tests for all major functions.

**One discrepancy to note**: The Go code defines `NUM_OPCODES = 85` (including `OP_SETTABLEN = 85`) while the C source has `NUM_OPCODES = 77` (OP_EXTRAARG = 76). The Go implementation appears to have added 8 extra opcodes beyond the Lua 5.5.1 reference. This should be verified.

---

## 8. Edge Cases & Traps for Reimplementers

### 8.1 Off-by-One in sBx Range
The sBx field has an **asymmetric range**: [-65535, +65536]. There are 131,072 possible unsigned values (0–131071) but only 131,072 signed values. The midpoint is at unsigned 65535, not 65536. `sBx = 0` is stored as unsigned 65535.

**Trap**: `OFFSET_sBx = MAXARG_Bx >> 1 = 65535`. If you compute `stored = signed + 65536` instead of `signed + 65535`, you'll be off by one for every positive value.

### 8.2 Unsigned vs Signed Types
All operands are stored as `unsigned int` in C. When extracting with `getarg`, the result is cast to `int` (`cast_int`). In Go, `uint32` shifting is fine, but when converting to `int` for signed arithmetic, be careful:
```go
// WRONG: this truncates on 32-bit overflow
sBx := int(uint32(i>>POS_Bx)) - OFFSET_sBx

// CORRECT: mask first, then convert
sBx := int(uint32(i>>POS_Bx)&MAXARG_Bx) - OFFSET_sBx
```

### 8.3 Format Validation with `check_exp`
`GETARG_B`, `GETARG_C`, `GETARG_Bx`, `GETARG_Ax`, `GETARG_sBx`, `GETARG_sJ` all use `check_exp` to verify the instruction format matches the requested accessor. Calling `GETARG_B` on an `iABx` instruction is a **programming error** — implementors should panic or log in debug builds.

### 8.4 Comparison Opcodes Always Have a Jump Next
The comments state: "All comparison and test instructions assume that the instruction being skipped (pc++) is a jump." (`lopcodes.h:399-400`). If you don't put a jump after a comparison, the VM will skip an arbitrary instruction — a subtle source of bugs.

### 8.5 B=0 Means "Top of Stack" in Some Opcodes
In `OP_CALL`, `OP_RETURN`, `OP_VARARG`, and `OP_SETLIST`:
- `B == 0` means "use `L->top`" (the current top of the stack set by the previous instruction)
- `C == 0` in `OP_CALL` means "return all results up to top"
- `vB == 0` in `OP_SETLIST` means "use top for the count"

This IT/OT mechanism (`testITMode`, `testOTMode`) chains instructions: one instruction sets the stack top, the next uses it.

### 8.6 NEWTABLE Encoding of Sizes
`OP_NEWTABLE` uses ivABC format (`lopcodes.h:43`):
- `vB` encodes the hash size: `actual_size = 2^(vB-1)` for vB > 0, or 0 for vB == 0
- `vC` encodes the array size: if k=0, `array_size = vC`; if k=1, `array_size = (EXTRAARG << 10) | vC`
- Note: vB=0 means hash size of 0. The formula `2^(vB-1)` means vB=1 → 1, vB=2 → 2, vB=3 → 4, etc.

### 8.7 SETLIST Multi-Assignment
`OP_SETLIST` with `vB > 0` and `k == 0`:
- Fields `R[A][vC+i]` through `R[A][vC+vB]` are set from registers `R[A+i]`
- `i` ranges from 1 to vB (1-based, not 0-based)
- The `k` bit extends C for large table initializations: if k=1, `real_C = (EXTRAARG << 10) | vC`

### 8.8 OP_LOADKX Always Followed by OP_EXTRAARG
`OP_LOADKX` never stores its constant index in the instruction itself. It always requires the **next instruction** to be `OP_EXTRAARG`. The VM fetches the constant from `K[EXTRAARG_Ax]`. Skipping the EXTRAARG would cause an immediate crash or undefined behavior.

### 8.9 TAILCALL and Upvalues
`OP_TAILCALL` with k=1 signals that the function builds upvalues that need to be closed on return. The `(C - 1)` fixed parameter count is stored in C when C > 0 and k = 1.

### 8.10 ERRNNIL with Bx=0
When `OP_ERRNNIL` has `Bx == 0`, the global name is not available for the error message. This is used when the global name index doesn't fit in Bx.

---

## 9. Comparison with Lua 5.4

The C source files (`lopcodes.h`, `lopcodes.c`) contain **no explicit version markers** referencing 5.4 or 5.5. No comments mention "new in 5.5" or "removed in 5.4". The `$Id$` tags do not contain version information.

Based on visible differences from standard Lua 5.4 knowledge:

### 9.1 New Opcodes in Lua 5.5.1 (not in 5.4)
The following opcodes appear to be **new in Lua 5.5** based on their absence in Lua 5.4:
- `OP_LOADI`, `OP_LOADF` — immediate integer/float loading (5.4 used OP_LOADK with constants)
- `OP_LOADKX` + `OP_EXTRAARG` — extended constant index mechanism
- `OP_LFALSESKIP` — optimized false generation
- `OP_GETI`, `OP_SETI` — integer key table access
- `OP_GETFIELD`, `OP_SETFIELD` — short string key access
- `OP_GETTABUP`, `OP_SETTABUP` — upvalue table access
- `OP_SELF` — optimized method access
- `OP_ADDI`, `OP_SHLI`, `OP_SHRI` — immediate arithmetic
- `OP_BANDK`, `OP_BORK`, `OP_BXORK`, `OP_BAND`, `OP_BOR`, `OP_BXOR`, `OP_BNOT` — bitwise operations
- `OP_MMBINI`, `OP_MMBINK`, `OP_MMBIN` — metamethod fallback
- `OP_TBC` — to-be-closed variables (previously `OP_CLOSE` handled this)
- `OP_EQK`, `OP_EQI`, `OP_LTI`, `OP_LEI`, `OP_GTI`, `OP_GEI` — immediate comparisons
- `OP_TESTSET` — optimized short-circuit
- `OP_TFORPREP`, `OP_TFORCALL`, `OP_TFORLOOP` — generic for-loop
- `OP_VARARGPREP` — vararg preparation
- `OP_ERRNNIL` — error-on-nil check
- `OP_GETVARG` — vararg access
- `OP_RETURN0`, `OP_RETURN1` — optimized returns

### 9.2 The `isJ` Format
The `isJ` format (signed 25-bit jump, `OP_JMP`) and `iAx` format (25-bit extended operand, `OP_EXTRAARG`) appear to be **new in Lua 5.5**, replacing older mechanisms. Lua 5.4 used `iAsBx` for jumps, limiting them to ±131071 instructions.

### 9.3 The `ivABC` Variant Format
The variant `ivABC` format with vB=6 bits and vC=10 bits (used by `OP_NEWTABLE` and `OP_SETLIST`) allows larger immediate values than the standard ABC format. This is a 5.5 addition.

### 9.4 Instruction Property Flags
Lua 5.5 adds the `IT` (Instruction-Top) and `OT` (Operator-Top) mode flags (`lopcodes.h:420-421`) that weren't present in Lua 5.4. These handle the "use top of stack from previous instruction" and "set top for next instruction" semantics needed by varargs and variadic calls.

---

## 10. Key Source Line Reference

| Topic | File | Lines |
|---|---|---|
| Instruction format diagram | `lopcodes.h` | 19–26 |
| `OpMode` enum | `lopcodes.h` | 36 |
| SIZE/POS constants | `lopcodes.h` | 42–66 |
| MAXARG/OFFSET limits | `lopcodes.h` | 75–114 |
| Signed sC helpers | `lopcodes.h` | 111–114 |
| MASK1/MASK0 macros | `lopcodes.h` | 117–121 |
| GETARG_*/SETARG_* macros | `lopcodes.h` | 127–174 |
| Signed sBx GETARG_sBx | `lopcodes.h` | 167–169 |
| Signed sJ GETARG_sJ | `lopcodes.h` | 171–174 |
| CREATE_* macros | `lopcodes.h` | 177–198 |
| MAX_FSTACK / NO_REG | `lopcodes.h` | 206–215 |
| OpCode enum | `lopcodes.h` | 231–348 |
| Notes on special opcodes | `lopcodes.h` | 355–411 |
| luaP_opmodes array | `lopcodes.c` | 22–109 |
| luaP_isOT | `lopcodes.c` | 117–124 |
| luaP_isIT | `lopcodes.c` | 131–139 |
| Opcode names | `lopnames.h` | 15–102 |
