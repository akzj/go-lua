# Bitwise Opcodes (lvm.c → vm.go)

## Overview

Lua 5.5 bitwise operations are **integer-only**. Unlike arithmetic opcodes, they do NOT
coerce strings to numbers and do NOT have a float path. The conversion macro `tointegerns`
accepts only integers and floats-with-exact-integer-value; strings are rejected outright.

If conversion fails, the opcode falls through to `OP_MMBIN`/`OP_MMBINK` for metamethod dispatch.

---

## Key Definitions

### intop macro (lvm.h:73)

```c
#define intop(op,v1,v2) l_castU2S(l_castS2U(v1) op l_castS2U(v2))
```

**Why unsigned cast**: C signed integer overflow is undefined behavior. By casting to
unsigned, performing the operation, and casting back, Lua gets well-defined wrapping
semantics. Go's `int64` arithmetic wraps naturally, so no cast is needed.

### tointegerns macro (lvm.h:68)

```c
#define tointegerns(o,i) \
  (l_likely(ttisinteger(o)) ? (*(i) = ivalue(o), 1) \
                            : luaV_tointegerns(o,i,LUA_FLOORN2I))
```

Fast path: if already integer, extract directly. Slow path: `luaV_tointegerns` (lvm.c:142)
accepts floats with exact integer value (e.g., `3.0` → `3`), rejects strings.

**Go equivalent**: `toIntegerStrict` (vm.go:49):
```go
func toIntegerStrict(v objectapi.TValue) (int64, bool) {
  switch v.Tt {
  case objectapi.TagInteger: return v.Val.(int64), true
  case objectapi.TagFloat:   return FloatToInteger(v.Val.(float64))
  }
  return 0, false  // strings rejected
}
```

### luaV_shiftl (lvm.c:818)

```c
lua_Integer luaV_shiftl (lua_Integer x, lua_Integer y) {
  if (y < 0) {                    // negative y = shift right
    if (y <= -NBITS) return 0;    // shift >= 64 bits → zero
    else return intop(>>, x, -y);
  }
  else {                          // positive y = shift left
    if (y >= NBITS) return 0;     // shift >= 64 bits → zero
    else return intop(<<, x, y);
  }
}
```

**Go equivalent**: `ShiftL` (vm.go:326):
```go
func ShiftL(x, y int64) int64 {
  if y < 0 {
    if y <= -64 { return 0 }
    return int64(uint64(x) >> uint(-y))  // unsigned shift right
  }
  if y >= 64 { return 0 }
  return int64(uint64(x) << uint(y))     // unsigned shift left
}
```

| Aspect | C `luaV_shiftl` | Go `ShiftL` |
|--------|-----------------|-------------|
| Bits | `NBITS` (= 64 on 64-bit) | Hardcoded `64` |
| Unsigned shift | `intop(>>, ...)` | `uint64(x) >> uint(...)` |
| Right shift | Negate y | Same |

### luaV_shiftr (lvm.h:111)

```c
#define luaV_shiftr(x,y) luaV_shiftl(x, intop(-, 0, y))
```

Go: `ShiftL(ib, -ic)` — negate second arg directly.

---

## Core Macros Expanded

### op_bitwiseK (C:1022) — Constant operand

```c
#define op_bitwiseK(L,op) {
  TValue *v1 = vRB(i);
  TValue *v2 = KC(i);                       // constant from k[]
  lua_Integer i1;
  lua_Integer i2 = ivalue(v2);              // k[] value is always integer
  if (tointegerns(v1, &i1)) {               // try convert v1 to integer
    StkId ra = RA(i);
    pc++; setivalue(s2v(ra), op(i1, i2));   // pc++ skips MMBINK
  }}                                         // else: fall through to MMBINK
```

**Key insight**: Only `v1` (register) needs `tointegerns`; `v2` (constant) is known integer.

### op_bitwise (C:1036) — Register-register

```c
#define op_bitwise(L,op) {
  TValue *v1 = vRB(i);
  TValue *v2 = vRC(i);
  lua_Integer i1; lua_Integer i2;
  if (tointegerns(v1, &i1) && tointegerns(v2, &i2)) {  // both must be integer
    StkId ra = RA(i);
    pc++; setivalue(s2v(ra), op(i1, i2));
  }}                                                     // else: fall to MMBIN
```

---

## K-Variant Opcodes (Constant Operand)

### OP_BANDK (C:1474 → Go:1854)

```c
// lvm.c:1474
vmcase(OP_BANDK) { op_bitwiseK(L, l_band); vmbreak; }
// l_band(a,b) = intop(&, a, b)
```

```go
// vm.go:1854
case opcodeapi.OP_BANDK:
  rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
  kc := k[opcodeapi.GetArgC(inst)]
  ib, ok1 := toIntegerStrict(rb)
  ic, ok2 := toIntegerStrict(kc)
  if ok1 && ok2 {
    L.Stack[ra].Val = objectapi.MakeInteger(ib & ic)
    ci.SavedPC++
  }
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| K value check | `ivalue(v2)` — assumes integer | `toIntegerStrict(kc)` — checks both |
| Operation | `intop(&, i1, i2)` | `ib & ic` (native Go) |

### OP_BORK (C:1478 → Go:1864)

```c
vmcase(OP_BORK) { op_bitwiseK(L, l_bor); vmbreak; }
```
Same pattern as BANDK with `|` operator. Go: `ib | ic`.

### OP_BXORK (C:1482 → Go:1874)

```c
vmcase(OP_BXORK) { op_bitwiseK(L, l_bxor); vmbreak; }
```
Same pattern with `^` operator. Go: `ib ^ ic`.

---

## I-Variant Opcodes (Shift with Immediate)

Shifts are unique: they use immediate operands but are NOT symmetric — `SHLI` shifts
the immediate BY the register, while `SHRI` shifts the register BY the immediate.

### OP_SHLI (C:1486 → Go:1884) — Shift Left Immediate

```c
// lvm.c:1486
vmcase(OP_SHLI) {
  StkId ra = RA(i);
  TValue *rb = vRB(i);
  int ic = GETARG_sC(i);          // signed immediate = shift amount source
  lua_Integer ib;
  if (tointegerns(rb, &ib)) {     // rb = shift count
    pc++; setivalue(s2v(ra), luaV_shiftl(ic, ib));  // shift ic LEFT by ib
  }
  vmbreak;
}
```

**Operand order**: `luaV_shiftl(ic, ib)` — the immediate `ic` is the VALUE being shifted,
`ib` (register) is the shift COUNT. This is `ic << ib`.

```go
// vm.go:1884
case opcodeapi.OP_SHLI:
  rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
  ic := int64(opcodeapi.GetArgSC(inst))
  ib, ok := toIntegerStrict(rb)
  if ok {
    L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ic, ib))
    ci.SavedPC++
  }
```

### OP_SHRI (C:1496 → Go:1893) — Shift Right Immediate

```c
// lvm.c:1496
vmcase(OP_SHRI) {
  StkId ra = RA(i);
  TValue *rb = vRB(i);
  int ic = GETARG_sC(i);          // signed immediate = shift count
  lua_Integer ib;
  if (tointegerns(rb, &ib)) {     // rb = value being shifted
    pc++; setivalue(s2v(ra), luaV_shiftl(ib, -ic));  // shift ib RIGHT by ic
  }
  vmbreak;
}
```

**Operand order**: `luaV_shiftl(ib, -ic)` — the register `ib` is shifted RIGHT by negating `ic`.

```go
// vm.go:1893
case opcodeapi.OP_SHRI:
  rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
  ic := int64(opcodeapi.GetArgSC(inst))
  ib, ok := toIntegerStrict(rb)
  if ok {
    L.Stack[ra].Val = objectapi.MakeInteger(ShiftL(ib, -ic))
    ci.SavedPC++
  }
```

### Shift I-Variant Differences

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| SHLI order | `luaV_shiftl(ic, ib)` — imm << reg | `ShiftL(ic, ib)` — same |
| SHRI order | `luaV_shiftl(ib, -ic)` — reg >> imm | `ShiftL(ib, -ic)` — same |
| No string coercion | `tointegerns` rejects strings | `toIntegerStrict` rejects strings |
| Metamethod | Falls to MMBINI on failure | Same |

---

## Register-Register Opcodes

### OP_BAND (C:1536 → Go:2009)

```c
vmcase(OP_BAND) { op_bitwise(L, l_band); vmbreak; }
```

```go
case opcodeapi.OP_BAND:
  rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
  rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
  ib, ok1 := toIntegerStrict(rb)
  ic, ok2 := toIntegerStrict(rc)
  if ok1 && ok2 {
    L.Stack[ra].Val = objectapi.MakeInteger(ib & ic)
    ci.SavedPC++
  }
```

### OP_BOR (C:1540 → Go:2019)
`op_bitwise(L, l_bor)` → Go: `ib | ic`. Same pattern.

### OP_BXOR (C:1544 → Go:2029)
`op_bitwise(L, l_bxor)` → Go: `ib ^ ic`. Same pattern.

### OP_SHL (C:1548 → Go:2039)

```c
vmcase(OP_SHL) { op_bitwise(L, luaV_shiftl); vmbreak; }
```

Go: `ShiftL(ib, ic)`. Note: `op_bitwise` passes both values to `luaV_shiftl(i1, i2)`,
so the first register is shifted left by the second register's value.

### OP_SHR (C:1552 → Go:2049)

```c
vmcase(OP_SHR) { op_bitwise(L, luaV_shiftr); vmbreak; }
// luaV_shiftr(x,y) = luaV_shiftl(x, intop(-, 0, y))
```

Go: `ShiftL(ib, -ic)`. Right shift = left shift with negated count.

### Register-Register Differences

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Macro | `op_bitwise(L, op)` | Inline per case |
| Conversion | `tointegerns` (no strings) | `toIntegerStrict` (no strings) |
| SHR impl | `luaV_shiftr` → `luaV_shiftl(x, -y)` | `ShiftL(ib, -ic)` direct |
| Overflow | `intop` unsigned wrapping | Go native wrapping |

---

## OP_BNOT (C:1601 → Go:2103) — Bitwise NOT

```c
// lvm.c:1601
vmcase(OP_BNOT) {
  StkId ra = RA(i);
  TValue *rb = vRB(i);
  lua_Integer ib;
  if (tointegerns(rb, &ib)) {
    setivalue(s2v(ra), intop(^, ~l_castS2U(0), ib));  // ~ib via XOR with all-ones
  }
  else
    Protect(luaT_trybinTM(L, rb, rb, ra, TM_BNOT));
  vmbreak;
}
```

**Why `intop(^, ~l_castS2U(0), ib)`**: This computes `~ib` by XORing with all-ones
(`~0ULL`). The `intop` macro ensures unsigned semantics. Equivalent to `~ib`.

```go
// vm.go:2103
case opcodeapi.OP_BNOT:
  rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
  ib, ok := toIntegerStrict(rb)
  if ok {
    L.Stack[ra].Val = objectapi.MakeInteger(^ib)  // Go bitwise complement
  } else {
    tryBinTM(L, rb, rb, ra, mmapi.TM_BNOT, ...)
  }
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| NOT impl | `intop(^, ~0, ib)` | `^ib` (Go complement) |
| No pc++ | BNOT has no MMBIN to skip | Same — no SavedPC++ |
| Metamethod | `luaT_trybinTM(rb, rb, ...)` | `tryBinTM(rb, rb, ...)` |
| String coercion | None (`tointegerns`) | None (`toIntegerStrict`) |

---

## Metamethod Fallthrough Pattern (Bitwise)

Same two-instruction pattern as arithmetic:

```
OP_BAND  R(A), R(B), R(C)     -- tries integer fast path
OP_MMBIN R(A), R(B), TM_BAND  -- only reached if conversion failed
```

For K-variants: followed by `OP_MMBINK`. For I-variants (shifts): followed by `OP_MMBINI`.

**Critical difference from arithmetic**: Bitwise metamethods are the ONLY path for
non-integer operands. There is no float fallback — it's integer or metamethod.

---

## Arithmetic vs Bitwise: Key Differences Summary

| Aspect | Arithmetic | Bitwise |
|--------|-----------|---------|
| Type paths | int → int, float → float | int only |
| String coercion (C) | `tonumberns` in float path | None |
| String coercion (Go) | `toNumberTV` | None (`toIntegerStrict`) |
| Float operands | Promoted to float path | Rejected (→ metamethod) |
| Float with int value | Used as float | Accepted as int (3.0→3) |
| Macro | `op_arith`/`op_arithK` | `op_bitwise`/`op_bitwiseK` |

---

## Verification

```bash
# All 12 bitwise opcodes covered:
# K-variants: BANDK, BORK, BXORK (3)
# I-variants: SHLI, SHRI (2)
# Register: BAND, BOR, BXOR, SHL, SHR (5)
# Unary: BNOT (1)
# Helpers: luaV_shiftl, intop, tointegerns
```
