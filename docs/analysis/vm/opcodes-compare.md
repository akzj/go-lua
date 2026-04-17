# VM Comparison & Test Opcodes

> C source: `lua-master/lvm.c` (1972 lines) | Go source: `internal/vm/api/vm.go` (2591 lines)
> Covers: OP_NOT, OP_EQ, OP_LT, OP_LE, OP_EQK, OP_EQI, OP_LTI, OP_LEI, OP_GTI, OP_GEI, OP_TEST, OP_TESTSET, OP_JMP

---

## Key Concepts

### The Conditional Jump Pattern (C: lvm.c:1136)

Every comparison opcode in Lua 5.5 follows a two-instruction sequence:
1. **Comparison instruction** — computes a boolean `cond`
2. **Jump instruction** — immediately follows; executed only if condition matches

The `k` field in the comparison instruction acts as a **complement flag**:
- `k=0`: jump when condition is TRUE (normal `if a < b then`)
- `k=1`: jump when condition is FALSE (negated `if not (a < b) then`)

This lets the compiler encode `if not (a < b)` without a separate NOT opcode.

### C Macros (lvm.c:1127–1138)

```c
// lvm.c:1127 — Execute jump, advance PC by signed offset + extra
#define dojump(ci,i,e)  { pc += GETARG_sJ(i) + e; updatetrap(ci); }

// lvm.c:1130 — Fetch NEXT instruction (must be a JMP) and execute it
#define donextjump(ci)   { Instruction ni = *pc; dojump(ci, ni, 1); }

// lvm.c:1136 — Core conditional: skip next instr if cond≠k, else take the jump
#define docondjump()  if (cond != GETARG_k(i)) pc++; else donextjump(ci);
```

**Why `+1` in donextjump?** The `dojump` adds `e=1` because after reading `*pc` (the JMP),
we need to advance past it. The signed offset in JMP is relative to the instruction *after* the JMP.

### Go Equivalent (vm.go:2133+)

Go inlines the pattern directly — no macros:
```go
if cond != (opcodeapi.GetArgK(inst) != 0) {
    ci.SavedPC++                                          // skip jump
} else {
    ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1  // take jump
}
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| PC model | Local `pc` pointer, macros | `ci.SavedPC` index into `code[]` |
| Jump decode | `donextjump` reads `*pc` | `code[ci.SavedPC]` reads next instr |
| Trap update | `updatetrap(ci)` in dojump | Not needed (no local trap copy) |

---

## OP_NOT (C:1612 → Go:2112)

### C Implementation
```c
// lvm.c:1612
vmcase(OP_NOT) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    if (l_isfalse(rb))
        setbtvalue(s2v(ra));    // R[A] = true
    else
        setbfvalue(s2v(ra));    // R[A] = false
    vmbreak;
}
```

**Why:** Lua truthiness — only `nil` and `false` are falsy. Everything else (including 0) is truthy.
`l_isfalse` checks tag: `ttisnil(v) || ttisfalse(v)`.

### Go Implementation
```go
// vm.go:2112
case opcodeapi.OP_NOT:
    rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
    if rb.IsFalsy() {
        L.Stack[ra].Val = objectapi.True
    } else {
        L.Stack[ra].Val = objectapi.False
    }
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Falsy check | `l_isfalse` (tag-based macro) | `IsFalsy()` method |
| Result | `setbtvalue`/`setbfvalue` sets tag | Assigns pre-allocated `True`/`False` singletons |

---

## OP_EQ (C:1650 → Go:2133)

### C Implementation
```c
// lvm.c:1650
vmcase(OP_EQ) {
    StkId ra = RA(i);
    int cond;
    TValue *rb = vRB(i);
    Protect(cond = luaV_equalobj(L, s2v(ra), rb));  // may call __eq metamethod
    docondjump();
    vmbreak;
}
```

**Why Protect:** `luaV_equalobj` may invoke `__eq` metamethod, which can error/realloc stack.
`Protect` saves PC+top before, updates trap after.

### Go Implementation
```go
// vm.go:2133
case opcodeapi.OP_EQ:
    rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
    L.Top = ci.Top                          // ensure scratch space for metamethod
    cond := EqualObj(L, L.Stack[ra].Val, rb)
    if cond != (opcodeapi.GetArgK(inst) != 0) {
        ci.SavedPC++
    } else {
        ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
    }
```

**EqualObj** (vm.go:520): Handles int/float cross-comparison, string comparison,
table/userdata identity + `__eq` metamethod. NaN ≠ NaN by IEEE 754.

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Core function | `luaV_equalobj` (lvm.c:389) | `EqualObj` (vm.go:520) |
| Stack protection | `Protect` macro | Manual `L.Top = ci.Top` |
| Raw equality | `luaV_rawequalobj` = `luaV_equalobj(NULL,...)` | `RawEqualObj` (vm.go:597) — separate function |

---

## OP_LT / OP_LE (C:1658,1662 → Go:2145,2155)

### C Implementation — `op_order` macro (lvm.c:1051)
```c
// lvm.c:1658-1663
vmcase(OP_LT) { op_order(L, l_lti, LTnum, lessthanothers); vmbreak; }
vmcase(OP_LE) { op_order(L, l_lei, LEnum, lessequalothers); vmbreak; }

// The op_order macro (lvm.c:1051) expands to:
//   if both int → opi(ia, ib)          // fast path
//   else if both number → opn(ra, rb)  // float compare
//   else → Protect(cond = other(L, ra, rb))  // string/metamethod
//   docondjump();
```

### Go Implementation
```go
// vm.go:2145 (LT), vm.go:2155 (LE)
cond := LessThan(L, L.Stack[ra].Val, rb)   // or LessEqual
```

**LessThan** (vm.go:448): number→`ltNum`, string→lexicographic, else→`callOrderTM(__lt)`.
**LessEqual** (vm.go:460): Same, but falls back to `!(b < a)` via `__lt` if no `__le`.

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Dispatch | `op_order` macro, 3 function pointers | `LessThan`/`LessEqual` with type switches |
| Int fast path | Inline `l_lti`/`l_lei` macro | Inside `ltNum`/`leNum` helper |
| LE fallback | `lessequalothers` → `__le` then `!__lt(b,a)` | `callOrderTM` → same logic |
| NaN | C `<`/`<=` (false for NaN) | Go `<`/`<=` — identical |

---

## OP_EQK (C:1666 → Go:2165)

### C Implementation
```c
// lvm.c:1666
vmcase(OP_EQK) {
    StkId ra = RA(i);
    TValue *rb = KB(i);                          // constant from K[]
    int cond = luaV_rawequalobj(s2v(ra), rb);    // NO metamethods
    docondjump();
    vmbreak;
}
```

**Why raw equality?** Constants are basic types (nil, bool, number, string).
Basic types never have `__eq` metamethods, so raw comparison is safe and faster.

### Go Implementation
```go
// vm.go:2165
case opcodeapi.OP_EQK:
    rb := k[opcodeapi.GetArgB(inst)]
    cond := RawEqualObj(L.Stack[ra].Val, rb)
    // ... docondjump pattern ...
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Equality | `luaV_rawequalobj` (= `luaV_equalobj(NULL,...)`) | `RawEqualObj` (vm.go:597) — dedicated function |
| Metamethods | Never called | Never called |

---

## OP_EQI (C:1674 → Go:2174)

### C Implementation
```c
// lvm.c:1674
vmcase(OP_EQI) {
    StkId ra = RA(i);
    int cond;
    int im = GETARG_sB(i);           // signed immediate from B field
    if (ttisinteger(s2v(ra)))
        cond = (ivalue(s2v(ra)) == im);
    else if (ttisfloat(s2v(ra)))
        cond = luai_numeq(fltvalue(s2v(ra)), cast_num(im));
    else
        cond = 0;                    // non-number ≠ number
    docondjump();
    vmbreak;
}
```

**Why no metamethod?** Immediate values are always small integers. Comparing a non-number
to a number is always false — no metamethod can make `"hello" == 5` true.

### Go Implementation
```go
// vm.go:2174
case opcodeapi.OP_EQI:
    im := int64(opcodeapi.GetArgSB(inst))
    v := L.Stack[ra].Val
    var cond bool
    if v.IsInteger() {
        cond = v.Integer() == im
    } else if v.IsFloat() {
        cond = v.Float() == float64(im)
    }
    // ... docondjump pattern ...
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Immediate type | `int` (C int, ≥16 bits) | `int64` (always 64-bit) |
| Default cond | Explicit `cond = 0` | Go zero-value `false` |

---

## OP_LTI / OP_LEI (C:1687,1691 → Go:2189,2209)

### C Implementation — `op_orderI` macro (lvm.c:1070)
```c
// lvm.c:1687,1691
vmcase(OP_LTI) { op_orderI(L, l_lti, luai_numlt, 0, TM_LT); vmbreak; }
vmcase(OP_LEI) { op_orderI(L, l_lei, luai_numle, 0, TM_LE); vmbreak; }
// op_orderI: int→opi(val,im), float→opf(val,im), else→luaT_callorderiTM
// inv=0 means no flip. C field carries isf (is-float) flag for metamethod.
```

### Go Implementation
```go
// vm.go:2189 (LTI), vm.go:2209 (LEI)
if v.IsInteger() { cond = v.Integer() < im }      // int fast
else if v.IsFloat() { cond = v.Float() < float64(im) }  // float fast
else { cond = callOrderITM(L, v, im, false, isf, mmapi.TM_LT) }
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Macro | `op_orderI` expands inline | Inlined in case body |
| Metamethod | `luaT_callorderiTM` (ltm.c) | `callOrderITM` (vm.go:504) |
| Flip arg | `inv=0` | `flip=false` |

---

## OP_GTI / OP_GEI (C:1695,1699 → Go:2228,2248)

```c
// lvm.c:1695
vmcase(OP_GTI) { op_orderI(L, l_gti, luai_numgt, 1, TM_LT); vmbreak; }
```

**Key insight:** `a > im` is encoded as `im < a` with `inv=1` (flip) and `TM_LT`.
There is no `__gt` metamethod — GT is always reduced to LT with swapped operands.

```go
// vm.go:2228 — flip=true, TM_LT
cond = callOrderITM(L, v, im, true, isf, mmapi.TM_LT)
```

---

## OP_GEI (C:1699 → Go:2248)

```c
// lvm.c:1699
vmcase(OP_GEI) { op_orderI(L, l_gei, luai_numge, 1, TM_LE); vmbreak; }
```

**Key insight:** `a >= im` is encoded as `im <= a` with `inv=1` (flip) and `TM_LE`.

```go
// vm.go:2248 — flip=true, TM_LE
cond = callOrderITM(L, v, im, true, isf, mmapi.TM_LE)
```

### GTI/GEI Difference Table

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| GT encoding | `op_orderI(..., 1, TM_LT)` — flip + LT | `callOrderITM(..., true, TM_LT)` |
| GE encoding | `op_orderI(..., 1, TM_LE)` — flip + LE | `callOrderITM(..., true, TM_LE)` |
| No __gt/__ge | Correct — reduced to __lt/__le | Correct — same reduction |

---

## OP_TEST (C:1703 → Go:2268)

### C Implementation
```c
// lvm.c:1703
vmcase(OP_TEST) {
    StkId ra = RA(i);
    int cond = !l_isfalse(s2v(ra));    // truthy?
    docondjump();
    vmbreak;
}
```

**Why:** Used for `if x then` / `if not x then`. Tests truthiness of R[A].
The `k` flag handles negation: `k=0` → jump if truthy, `k=1` → jump if falsy.

### Go Implementation
```go
// vm.go:2268
case opcodeapi.OP_TEST:
    cond := !L.Stack[ra].Val.IsFalsy()
    // ... docondjump pattern ...
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Truthiness | `!l_isfalse(s2v(ra))` | `!IsFalsy()` |
| No metamethods | Correct — TEST never calls metamethods | Same |

---

## OP_TESTSET (C:1709 → Go:2276)

### C Implementation
```c
// lvm.c:1709
vmcase(OP_TESTSET) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    if (l_isfalse(rb) == GETARG_k(i))
        pc++;                          // condition failed → skip jump
    else {
        setobj2s(L, ra, rb);          // R[A] = R[B] (copy value)
        donextjump(ci);               // take the jump
    }
    vmbreak;
}
```

**Why:** Used for `and`/`or` short-circuit: `x = a or b` compiles to TESTSET.
If the test passes, it copies R[B]→R[A] (preserving the value) AND jumps.
This is the key difference from TEST: TESTSET also assigns.

### Go Implementation
```go
// vm.go:2276
case opcodeapi.OP_TESTSET:
    rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
    if rb.IsFalsy() == (opcodeapi.GetArgK(inst) != 0) {
        ci.SavedPC++                    // skip jump
    } else {
        L.Stack[ra].Val = rb            // copy value
        ci.SavedPC += opcodeapi.GetArgSJ(code[ci.SavedPC]) + 1
    }
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Value copy | `setobj2s(L, ra, rb)` (with GC barrier) | Direct assignment (GC is tracing) |
| Condition check | `l_isfalse(rb) == GETARG_k(i)` | `IsFalsy() == (GetArgK != 0)` |

---

## OP_JMP (C:1646 → Go:2287)

```c
// lvm.c:1646 — Unconditional jump
vmcase(OP_JMP) { dojump(ci, i, 0); vmbreak; }  // PC += GETARG_sJ(i)
```
```go
// vm.go:2287
case opcodeapi.OP_JMP:
    ci.SavedPC += opcodeapi.GetArgSJ(inst)
```

`e=0` because JMP uses its own offset directly (unlike `donextjump` which reads the *next* instruction and adds +1). C also calls `updatetrap(ci)` inside `dojump`; Go doesn't need this.

---

## NaN Handling

Both C and Go use native `<`/`<=`/`==` which follow IEEE 754: NaN comparisons always return false.
The `k` complement flag ensures `if not (NaN < 5)` correctly skips the "true" branch.

---

## Metamethod Dispatch Summary

| Opcode | Metamethod | Notes |
|--------|-----------|-------|
| OP_EQ | `__eq` | Only for tables/userdata with same type |
| OP_LT | `__lt` | Tries both operands |
| OP_LE | `__le` → `__lt` | Falls back to `!(b < a)` |
| OP_EQK | none | Raw equality, constants only |
| OP_EQI | none | Immediate integer, no metamethod possible |
| OP_LTI/LEI | `__lt`/`__le` | Via `luaT_callorderiTM` / `callOrderITM` |
| OP_GTI/GEI | `__lt`/`__le` | Flipped: `a > im` → `im < a` |
| OP_TEST | none | Pure truthiness test |
| OP_TESTSET | none | Truthiness + assignment |
| OP_NOT | none | Pure boolean negation |
| OP_JMP | none | Unconditional jump |
