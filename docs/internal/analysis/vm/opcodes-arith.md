# Arithmetic Opcodes (lvm.c → vm.go)

## Overview

Lua 5.5 arithmetic uses a **two-instruction pattern**: the arith opcode attempts a fast path
(int/float), and if both operands fail numeric conversion, it falls through to the next
instruction (`OP_MMBIN`/`OP_MMBINI`/`OP_MMBINK`) which dispatches the metamethod.

**Type coercion rules** (C `tonumberns` / Go `toNumberTV`):
- `int op int → int` (wrapping unsigned arithmetic via `intop`)
- `int op float → float` (int promoted)
- `float op float → float`
- `string → number` coercion attempted (preserves int/float type)
- Non-numeric → fall through to metamethod

**Key C macros** (lvm.c:927–1042, lvm.h:73):

```c
// lvm.h:73 — wrapping integer arithmetic via unsigned cast
#define intop(op,v1,v2) l_castU2S(l_castS2U(v1) op l_castS2U(v2))

// lvm.c:927-932 — integer operation wrappers
#define l_addi(L,a,b)  intop(+, a, b)
#define l_subi(L,a,b)  intop(-, a, b)
#define l_muli(L,a,b)  intop(*, a, b)
```

**Go equivalents** (vm.go:34–145):
- `toNumberTV(v)` → returns TValue preserving int/float type (= C `tonumberns`)
- `toIntegerStrict(v)` → int64 without string coercion (= C `tointegerns`)
- `arithBinTV(a, b, intOp, floatOp)` → dispatches int vs float path
- `ToNumber(v)` → float64 with string coercion (= C `tonumber`)
- Go uses native `int64` wrapping (same as C unsigned cast for +, -, *)

---

## Core Macros Expanded

### op_arithI (C:944) — Immediate operand (only OP_ADDI uses this)

```c
#define op_arithI(L,iop,fop) {
  TValue *ra = vRA(i);  TValue *v1 = vRB(i);
  int imm = GETARG_sC(i);                    // signed immediate from C field
  if (ttisinteger(v1)) {                      // fast: int + imm → int
    lua_Integer iv1 = ivalue(v1);
    pc++; setivalue(ra, iop(L, iv1, imm));    // pc++ skips following MMBINI
  }
  else if (ttisfloat(v1)) {                   // float + imm → float
    lua_Number nb = fltvalue(v1);
    lua_Number fimm = cast_num(imm);
    pc++; setfltvalue(ra, fop(L, nb, fimm));
  }}                                          // else: fall through to MMBINI
```

### op_arithK (C:1013) — Constant operand

```c
#define op_arithK(L,iop,fop) {
  TValue *v1 = vRB(i);
  TValue *v2 = KC(i);  // constant from k[] array
  op_arith_aux(L, v1, v2, iop, fop); }
```

### op_arith_aux (C:992) — Core int/float dispatch

```c
#define op_arith_aux(L,v1,v2,iop,fop) {
  if (ttisinteger(v1) && ttisinteger(v2)) {   // both int → int
    StkId ra = RA(i);
    lua_Integer i1 = ivalue(v1); lua_Integer i2 = ivalue(v2);
    pc++; setivalue(s2v(ra), iop(L, i1, i2));
  }
  else op_arithf_aux(L, v1, v2, fop); }       // fallback to float path
```

### op_arithf_aux (C:963) — Float fallback (includes string→number coercion)

```c
#define op_arithf_aux(L,v1,v2,fop) {
  lua_Number n1; lua_Number n2;
  if (tonumberns(v1, n1) && tonumberns(v2, n2)) {  // coerce to float
    StkId ra = RA(i);
    pc++; setfltvalue(s2v(ra), fop(L, n1, n2));
  }}                                                // else: fall to MMBIN
```

### op_arithf / op_arithfK (C:974/983) — Float-only ops (POW, DIV)

```c
#define op_arithf(L,fop) {          // register operands
  TValue *v1 = vRB(i);  TValue *v2 = vRC(i);
  op_arithf_aux(L, v1, v2, fop); }

#define op_arithfK(L,fop) {         // constant operand
  TValue *v1 = vRB(i);  TValue *v2 = KC(i);
  op_arithf_aux(L, v1, v2, fop); }
```

### op_arith (C:1004) — Register-register

```c
#define op_arith(L,iop,fop) {
  TValue *v1 = vRB(i);  TValue *v2 = vRC(i);
  op_arith_aux(L, v1, v2, iop, fop); }
```

---

## OP_ADDI (C:1440 → Go:1727) — Immediate addition

```c
// lvm.c:1440
vmcase(OP_ADDI) {
  op_arithI(L, l_addi, luai_numadd);  // l_addi = intop(+, a, b)
  vmbreak;
}
```

```go
// vm.go:1727
case opcodeapi.OP_ADDI:
  rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
  ic := int64(opcodeapi.GetArgSC(inst))
  if rb.IsInteger() {
    L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() + ic)
    ci.SavedPC++ // skip MMBINI
  } else if rb.IsFloat() {
    L.Stack[ra].Val = objectapi.MakeFloat(rb.Float() + float64(ic))
    ci.SavedPC++
  } else if nv, ok := toNumberTV(rb); ok { // string→number coercion
    // ...preserves int/float type
    ci.SavedPC++
  }
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| String coercion | `tonumberns` rejects strings (float-only) | `toNumberTV` accepts strings (preserves type) |
| PC advance | `pc++` (local pointer) | `ci.SavedPC++` (field on CallInfo) |
| Wrapping | `intop(+,...)` unsigned cast | Native Go int64 `+` (same wrapping) |

> **Note**: C `op_arithI` does NOT attempt string coercion (uses `ttisinteger`/`ttisfloat`
> direct checks). Go adds a third branch with `toNumberTV` for string→number. This is a
> **semantic difference**: C defers string coercion to the metamethod path.

---

## K-Variant Opcodes (Constant Operand)

All K-variants use `op_arithK` (int+float path) or `op_arithfK` (float-only path).
The Go pattern is identical across all: fast int path, then `toNumberTV` fallback.

### OP_ADDK (C:1444 → Go:1749)
`op_arithK(L, l_addi, luai_numadd)` — both int→int, else float.

### OP_SUBK (C:1448 → Go:1764)
`op_arithK(L, l_subi, luai_numsub)` — same pattern with subtraction.

### OP_MULK (C:1452 → Go:1779)
`op_arithK(L, l_muli, luai_nummul)` — same pattern with multiplication.

### OP_MODK (C:1456 → Go:1794)
```c
vmcase(OP_MODK) {
  savestate(L, ci);  /* in case of division by 0 */
  op_arithK(L, luaV_mod, luaV_modf);
  vmbreak;
}
```
**Why `savestate`**: Mod by zero raises an error; `savestate` ensures PC/top are correct
for the error message. Go equivalent: `IMod(L, ...)` calls `RunError` which reads `SavedPC`.

### OP_POWK (C:1461 → Go:1813)
```c
vmcase(OP_POWK) {
  op_arithfK(L, luai_numpow);  // float-only: no int fast path
  vmbreak;
}
```
Uses `op_arithfK` — always produces float. Go: `ToNumber()` → `math.Pow()`.

### OP_DIVK (C:1465 → Go:1823)
`op_arithfK(L, luai_numdiv)` — float division always produces float. Go: `ToNumber()` → `/`.

### OP_IDIVK (C:1469 → Go:1833)
```c
vmcase(OP_IDIVK) {
  savestate(L, ci);  /* in case of division by 0 */
  op_arithK(L, luaV_idiv, luai_numidiv);
  vmbreak;
}
```
Floor division: int//int→int, else float. `savestate` for div-by-zero.
Go: `IDiv(L, ...)` for int path, `math.Floor(a/b)` for float path.

### K-Variant Differences Table

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Macro dispatch | `op_arithK`/`op_arithfK` macros | Inline code per case |
| Int fast path | `ttisinteger(v1) && ttisinteger(v2)` | `rb.IsInteger() && kc.IsInteger()` |
| Float fallback | `tonumberns` (no string coercion) | `toNumberTV` (includes string coercion) |
| String coercion | Deferred to MMBINK metamethod | Done in fast path |
| Div-by-zero | `savestate` before macro | `IMod`/`IDiv` call `RunError` directly |

---

## Register-Register Opcodes

All use `op_arith(L, iop, fop)` macro which expands to `op_arith_aux`.

### OP_ADD (C:1506 → Go:1904)
`op_arith(L, l_addi, luai_numadd)` — int+int→int, else float+float→float.

### OP_SUB (C:1510 → Go:1919)
`op_arith(L, l_subi, luai_numsub)` — same pattern.

### OP_MUL (C:1514 → Go:1934)
`op_arith(L, l_muli, luai_nummul)` — same pattern.

### OP_MOD (C:1518 → Go:1949)
```c
vmcase(OP_MOD) {
  savestate(L, ci);  /* in case of division by 0 */
  op_arith(L, luaV_mod, luaV_modf);
  vmbreak;
}
```
`savestate` for div-by-zero. Go uses `IMod`/`FMod` helpers.

### OP_POW (C:1523 → Go:1968)
`op_arithf(L, luai_numpow)` — float-only, no int path. Go: `math.Pow`.

### OP_DIV (C:1527 → Go:1978)
`op_arithf(L, luai_numdiv)` — float division always. Go: `ToNumber()` → `/`.

### OP_IDIV (C:1531 → Go:1988)
```c
vmcase(OP_IDIV) {
  savestate(L, ci);  /* in case of division by 0 */
  op_arith(L, luaV_idiv, luai_numidiv);
  vmbreak;
}
```
Floor division: int//int→int, else float. Go: `IDiv`/`math.Floor(a/b)`.

### Register-Register Differences Table

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Dispatch | Single `op_arith` macro | Inline per case |
| Int wrapping | `intop` (unsigned cast) | Native Go int64 (identical semantics) |
| Float coercion | `tonumberns` (no strings) | `toNumberTV` (coerces strings) |
| POW/DIV | Always float via `op_arithf` | Always float via `ToNumber` |
| Metamethod | Falls through to next OP_MMBIN | Falls through to next OP_MMBIN |

---

## OP_UNM (C:1586 → Go:2086) — Unary Minus

```c
// lvm.c:1586
vmcase(OP_UNM) {
  StkId ra = RA(i);  TValue *rb = vRB(i);
  lua_Number nb;
  if (ttisinteger(rb)) {
    lua_Integer ib = ivalue(rb);
    setivalue(s2v(ra), intop(-, 0, ib));     // unsigned negate (handles MIN_INT)
  }
  else if (tonumberns(rb, nb)) {
    setfltvalue(s2v(ra), luai_numunm(L, nb));
  }
  else
    Protect(luaT_trybinTM(L, rb, rb, ra, TM_UNM));  // metamethod
  vmbreak;
}
```

```go
// vm.go:2086
case opcodeapi.OP_UNM:
  rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
  if rb.IsInteger() {
    L.Stack[ra].Val = objectapi.MakeInteger(-rb.Integer())
  } else if rb.IsFloat() {
    L.Stack[ra].Val = objectapi.MakeFloat(-rb.Float())
  } else if nb, ok := toNumberTV(rb); ok {
    // string→number coercion preserving type
    if nb.IsInteger() { ... } else { ... }
  } else {
    tryBinTM(L, rb, rb, ra, mmapi.TM_UNM, ...)
  }
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Int negate | `intop(-, 0, ib)` unsigned | `-rb.Integer()` (same: Go wraps) |
| No pc++ | UNM has no following MMBIN to skip | Same — no SavedPC++ |
| String coercion | `tonumberns` (no strings) | `toNumberTV` (coerces strings) |
| Metamethod | `luaT_trybinTM(rb, rb, ...)` | `tryBinTM(rb, rb, ...)` |

---

## Metamethod Fallthrough Pattern

Every arithmetic opcode is followed by `OP_MMBIN` (register), `OP_MMBINI` (immediate),
or `OP_MMBINK` (constant). The pattern:

```
OP_ADD  R(A), R(B), R(C)    -- tries fast path; if ok, pc++ skips next
OP_MMBIN R(A), R(B), TM_ADD -- only reached if fast path failed
```

**C**: The `pc++` inside the macro skips the MMBIN instruction on success.
On failure (neither operand is numeric), the macro body ends without `pc++`,
so execution falls through to the next `vmfetch()` → `OP_MMBIN`.

**Go**: Identical pattern — `ci.SavedPC++` on success skips the MMBIN case.

---

## Verification

```
grep -c 'OP_ADD\|OP_SUB\|OP_MUL\|OP_MOD\|OP_POW\|OP_DIV\|OP_IDIV\|OP_UNM\|OP_ADDI\|OP_ADDK\|OP_SUBK\|OP_MULK\|OP_MODK\|OP_POWK\|OP_DIVK\|OP_IDIVK' lvm.c
# 16 opcodes covered: ADDI, ADDK-IDIVK (7), ADD-IDIV (7), UNM
```
