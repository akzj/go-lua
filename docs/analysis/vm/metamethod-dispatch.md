# Metamethod Dispatch Mechanism

> C: `lua-master/lvm.c` + `lua-master/ltm.c` | Go: `internal/vm/vm.go` + `internal/metamethod/metamethod.go`

## Overview

Lua 5.5 uses a **two-instruction sequence** for arithmetic/bitwise metamethods. The first
instruction (e.g., OP_ADD) attempts the fast path (raw integer/float operation). If the
fast path fails (non-numeric operands), execution **falls through** to the next instruction
(OP_MMBIN/MMBINI/MMBINK), which dispatches the metamethod. This avoids bloating every
arithmetic opcode with metamethod logic.

---

## 1. The Two-Instruction Sequence Pattern

### How It Works in C

Every arithmetic/bitwise opcode (OP_ADD through OP_SHR) is compiled as a **pair**:

```
OP_ADD   rA, rB, rC      -- try fast path
OP_MMBIN rA, rB, TM_ADD  -- metamethod fallback (only reached if fast path fails)
```

The fast-path macro (e.g., `op_arith`) does `pc++` on success, **skipping** the MMBIN
instruction. On failure, `pc` is NOT advanced, so the next iteration naturally fetches
the MMBIN instruction.

```c
// lvm.c:1004 — op_arith macro (simplified)
#define op_arith(L,iop,fop) {
  TValue *v1 = vRB(i);
  TValue *v2 = vRC(i);
  if (ttisinteger(v1) && ttisinteger(v2)) {
    lua_Integer i1 = ivalue(v1); lua_Integer i2 = ivalue(v2);
    pc++; setivalue(s2v(ra), iop(L, i1, i2));   // SUCCESS → skip MMBIN
  }
  else {  // try float conversion
    lua_Number n1; lua_Number n2;
    if (tonumberns(v1, n1) && tonumberns(v2, n2)) {
      pc++; setfltvalue(s2v(ra), fop(L, n1, n2)); // SUCCESS → skip MMBIN
    }
    // FAILURE → pc NOT advanced → falls through to MMBIN
  }
}
```

### How It Works in Go

Same two-instruction pattern. The Go handler does `ci.SavedPC++` on success:

```go
// vm.go:1904 — OP_ADD
case opcodeapi.OP_ADD:
    rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
    rc := L.Stack[base+opcodeapi.GetArgC(inst)].Val
    if rb.IsInteger() && rc.IsInteger() {
        L.Stack[ra].Val = objectapi.MakeInteger(rb.Integer() + rc.Integer())
        ci.SavedPC++                            // skip MMBIN
    } else {
        nb, ok1 := toNumberTV(rb)
        nc, ok2 := toNumberTV(rc)
        if ok1 && ok2 {
            L.Stack[ra].Val = arithBinTV(nb, nc, ...)
            ci.SavedPC++                        // skip MMBIN
        }
        // failure: SavedPC NOT advanced → next iteration fetches MMBIN
    }
```

### Variant Patterns

| Arithmetic type | Fast-path macro (C) | Fallback instruction | Example |
|----------------|---------------------|---------------------|---------|
| Register-Register | `op_arith` (lvm.c:1004) | OP_MMBIN | OP_ADD rA,rB,rC |
| Immediate int | `op_arithI` (lvm.c:944) | OP_MMBINI | OP_ADDI rA,rB,imm |
| Constant | `op_arithK` (lvm.c:1013) | OP_MMBINK | OP_ADDK rA,rB,kC |
| Float-only reg | `op_arithf` (lvm.c:974) | OP_MMBIN | OP_POW rA,rB,rC |
| Float-only K | `op_arithfK` (lvm.c:983) | OP_MMBINK | OP_POWK rA,rB,kC |
| Bitwise reg | `op_bitwise` (lvm.c:1035) | OP_MMBIN | OP_BAND rA,rB,rC |
| Bitwise K | `op_bitwiseK` (lvm.c:1022) | OP_MMBINK | OP_BANDK rA,rB,kC |

---

## 2. OP_MMBIN (C:1556 → Go:2061)

### C Implementation

```c
// lvm.c:1556
vmcase(OP_MMBIN) {
    StkId ra = RA(i);
    Instruction pi = *(pc - 2);  /* original arith. expression */
    TValue *rb = vRB(i);
    TMS tm = (TMS)GETARG_C(i);
    StkId result = RA(pi);
    lua_assert(OP_ADD <= GET_OPCODE(pi) && GET_OPCODE(pi) <= OP_SHR);
    Protect(luaT_trybinTM(L, s2v(ra), rb, result, tm));
    vmbreak;
}
```

### Why Each Line

| Line | Purpose |
|------|---------|
| `pi = *(pc - 2)` | Fetch the ORIGINAL arithmetic instruction (2 back: one for MMBIN itself, one for the pc++ after fetch). The result register is encoded in the original instruction, not in MMBIN. |
| `rb = vRB(i)` | Second operand from MMBIN's B field. |
| `tm = (TMS)GETARG_C(i)` | Metamethod tag (TM_ADD, TM_SUB, etc.) encoded in C field. |
| `result = RA(pi)` | Destination register from the ORIGINAL instruction. MMBIN's own A field holds the first operand, NOT the destination. |
| `Protect(luaT_trybinTM(...))` | Save state, call metamethod, update trap. Will error if no metamethod found. |

### Go Implementation

```go
// vm.go:2061
case opcodeapi.OP_MMBIN:
    rb := L.Stack[base+opcodeapi.GetArgB(inst)].Val
    tm := mmapi.TMS(opcodeapi.GetArgC(inst))
    prevInst := code[ci.SavedPC-2]
    result := base + opcodeapi.GetArgA(prevInst)
    tryBinTM(L, L.Stack[ra].Val, rb, result, tm, ra-base, opcodeapi.GetArgB(inst))
```

### Differences

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Previous instruction | `*(pc - 2)` pointer arithmetic | `code[ci.SavedPC-2]` index arithmetic |
| Result location | `StkId` (pointer to stack slot) | `int` (stack index) |
| Error reporting | No register info passed to trybinTM | Passes `ra-base` and `GetArgB(inst)` as register hints for error messages |
| Protection | `Protect()` macro wraps call | Direct call (no savepc needed — PC always in ci) |

---

## 3. OP_MMBINI (C:1566 → Go:2068)

### C Implementation

```c
// lvm.c:1566
vmcase(OP_MMBINI) {
    StkId ra = RA(i);
    Instruction pi = *(pc - 2);
    int imm = GETARG_sB(i);
    TMS tm = (TMS)GETARG_C(i);
    int flip = GETARG_k(i);
    StkId result = RA(pi);
    Protect(luaT_trybiniTM(L, s2v(ra), imm, flip, result, tm));
    vmbreak;
}
```

### Why Each Line

| Line | Purpose |
|------|---------|
| `imm = GETARG_sB(i)` | Signed immediate integer from B field (the constant that was in OP_ADDI's C field). |
| `flip = GETARG_k(i)` | If 1, the immediate was the LEFT operand (e.g., `5 + x` compiled as ADDI with flip). Metamethod must swap operand order. |
| `luaT_trybiniTM` | Converts `imm` to TValue, then calls `trybinassocTM` with flip logic. |

### Go Implementation

```go
// vm.go:2068
case opcodeapi.OP_MMBINI:
    imm := opcodeapi.GetArgSB(inst)
    tm := mmapi.TMS(opcodeapi.GetArgC(inst))
    flip := opcodeapi.GetArgK(inst) != 0
    prevInst := code[ci.SavedPC-2]
    result := base + opcodeapi.GetArgA(prevInst)
    tryBiniTM(L, L.Stack[ra].Val, imm, flip, result, tm, ra-base)
```

### Differences

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Flip type | `int` (0 or 1) | `bool` |
| Dispatch | `luaT_trybiniTM` → `luaT_trybinassocTM` → `luaT_trybinTM` | `tryBiniTM` → `tryBinTM` (flip handled inline) |

---

## 4. OP_MMBINK (C:1576 → Go:2076)

### C Implementation

```c
// lvm.c:1576
vmcase(OP_MMBINK) {
    StkId ra = RA(i);
    Instruction pi = *(pc - 2);
    TValue *imm = KB(i);
    TMS tm = (TMS)GETARG_C(i);
    int flip = GETARG_k(i);
    StkId result = RA(pi);
    Protect(luaT_trybinassocTM(L, s2v(ra), imm, flip, result, tm));
    vmbreak;
}
```

### Why Each Line

| Line | Purpose |
|------|---------|
| `imm = KB(i)` | Constant from B field (index into constant table). Already a TValue. |
| `luaT_trybinassocTM` | Handles flip: if flip, swaps p1/p2 before calling `luaT_trybinTM`. |

### Go Implementation

```go
// vm.go:2076
case opcodeapi.OP_MMBINK:
    imm := k[opcodeapi.GetArgB(inst)]
    tm := mmapi.TMS(opcodeapi.GetArgC(inst))
    flip := opcodeapi.GetArgK(inst) != 0
    prevInst := code[ci.SavedPC-2]
    result := base + opcodeapi.GetArgA(prevInst)
    tryBinKTM(L, L.Stack[ra].Val, imm, flip, result, tm, ra-base)
```

### Differences

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Function name | `luaT_trybinassocTM` | `tryBinKTM` |
| Constant access | `KB(i)` = `k + GETARG_B(i)` (pointer) | `k[opcodeapi.GetArgB(inst)]` (slice index) |

---

## 5. Metamethod Lookup Chain

### C Call Chain (ltm.c)

```
callbinTM (ltm.c:138)
  ├─ luaT_gettmbyobj(L, p1, event)  → try first operand's metatable
  ├─ luaT_gettmbyobj(L, p2, event)  → try second operand's metatable
  ├─ if neither has TM → return -1 (not found)
  └─ luaT_callTMres(L, tm, p1, p2, res) → call metamethod, store result

luaT_trybinTM (ltm.c:150)
  ├─ callbinTM(L, p1, p2, res, event)
  └─ if not found → error:
       ├─ bitwise ops → "perform bitwise operation on" (or tointerror)
       └─ arithmetic → "perform arithmetic on"

luaT_trybinassocTM (ltm.c:180)
  ├─ if flip → luaT_trybinTM(L, p2, p1, res, event)
  └─ else   → luaT_trybinTM(L, p1, p2, res, event)

luaT_trybiniTM (ltm.c:189)
  ├─ setivalue(&aux, i2)        → convert int immediate to TValue
  └─ luaT_trybinassocTM(L, p1, &aux, flip, res, event)
```

### Go Call Chain (vm.go)

```
tryBinTM (vm.go:717)
  ├─ mmapi.GetTMByObj(L.Global, p1, event)  → try first operand
  ├─ mmapi.GetTMByObj(L.Global, p2, event)  → try second operand
  ├─ if neither has TM → RunError with type-specific message
  ├─ if res >= L.Top → L.Top = res + 1      → ensure result slot is valid
  └─ callTMRes(L, tm, p1, p2) → call metamethod, store in L.Stack[res]

tryBiniTM (vm.go:751)
  ├─ p2 := objectapi.MakeInteger(int64(imm))  → convert int to TValue
  ├─ if flip → tryBinTM(L, p2, p1, ...)
  └─ else   → tryBinTM(L, p1, p2, ...)

tryBinKTM (vm.go:761)
  ├─ if flip → tryBinTM(L, p2, p1, ...)
  └─ else   → tryBinTM(L, p1, p2, ...)
```

---

## 6. callTMres / callTM — The Actual Metamethod Call

### C Implementation (ltm.c:119)

```c
// ltm.c:119
lu_byte luaT_callTMres (lua_State *L, const TValue *f, const TValue *p1,
                        const TValue *p2, StkId res) {
  ptrdiff_t result = savestack(L, res);       // save res as offset (stack may move)
  StkId func = L->top.p;
  setobj2s(L, func, f);                       // push metamethod function
  setobj2s(L, func + 1, p1);                  // push operand 1
  setobj2s(L, func + 2, p2);                  // push operand 2
  L->top.p += 3;
  if (isLuacode(L->ci))
    luaD_call(L, func, 1);                    // call with 1 result (can yield)
  else
    luaD_callnoyield(L, func, 1);             // call with 1 result (no yield)
  res = restorestack(L, result);              // restore res (stack may have moved)
  setobjs2s(L, res, --L->top.p);             // pop result into destination
  return ttypetag(s2v(res));                  // return type tag of result
}
```

### C Implementation — callTM (ltm.c:103)

```c
// ltm.c:103
void luaT_callTM (lua_State *L, const TValue *f, const TValue *p1,
                  const TValue *p2, const TValue *p3) {
  StkId func = L->top.p;
  setobj2s(L, func, f);       // push metamethod
  setobj2s(L, func + 1, p1);  // arg 1
  setobj2s(L, func + 2, p2);  // arg 2
  setobj2s(L, func + 3, p3);  // arg 3
  L->top.p = func + 4;
  if (isLuacode(L->ci))
    luaD_call(L, func, 0);    // 0 results (for __newindex etc.)
  else
    luaD_callnoyield(L, func, 0);
}
```

### Go Implementation (vm.go:674)

```go
// vm.go:674
func callTMRes(L *stateapi.LuaState, tm, p1, p2 objectapi.TValue) objectapi.TValue {
    top := L.Top
    L.Stack[top].Val = tm        // push metamethod
    L.Stack[top+1].Val = p1      // push operand 1
    L.Stack[top+2].Val = p2      // push operand 2
    L.Top = top + 3
    Call(L, top, 1)              // call with 1 result
    result := L.Stack[top].Val   // PosCall moves result to func slot
    L.Top = top                  // restore top
    return result                // return result value
}

// vm.go:688
func callTM(L *stateapi.LuaState, tm, p1, p2, p3 objectapi.TValue) {
    top := L.Top
    L.Stack[top].Val = tm
    L.Stack[top+1].Val = p1
    L.Stack[top+2].Val = p2
    L.Stack[top+3].Val = p3
    L.Top = top + 4
    Call(L, top, 0)              // 0 results
}
```

### Differences — callTMres

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Stack safety | `savestack`/`restorestack` — saves res as byte offset because GC can move stack | Not needed — Go GC doesn't move stack; indices are stable |
| Yield support | `isLuacode` check: `luaD_call` (yieldable) vs `luaD_callnoyield` | Always calls `Call()` — yield semantics handled differently |
| Result placement | Pops from top into `res` slot via `setobjs2s` | Returns value directly; caller stores it |
| Return value | Returns type tag (`lu_byte`) | Returns `objectapi.TValue` |

---

## 7. Metamethod Lookup — GetTMByObj

### C Implementation (ltm.c, via ltm.h)

`luaT_gettmbyobj` looks up the metamethod by checking the metatable of the value:
- **Table**: uses `table->metatable`
- **Userdata**: uses `udata->metatable`
- **Other types**: uses `G(L)->mt[type]` (global per-type metatable)

### Go Implementation (metamethod.go:83)

```go
// metamethod.go:83
func GetTMByObj(g *stateapi.GlobalState, obj objectapi.TValue, event TMS) objectapi.TValue {
    var mt *tableapi.Table
    switch obj.Type() {
    case objectapi.TypeTable:
        mt = tbl.GetMetatable()
    case objectapi.TypeUserdata:
        mt = ud.MetaTable.(*tableapi.Table)
    default:
        mt = g.MT[obj.Type()]          // global per-type metatable
    }
    if mt == nil { return objectapi.Nil }
    tmName := g.TMNames[event]         // cached __add, __sub, etc. strings
    // ... look up tmName in mt ...
}
```

### Key Design: TM Name Caching

Both C and Go cache metamethod name strings (e.g., `"__add"`) in the global state to
avoid creating new strings for every metamethod lookup. C uses `G(L)->tmname[event]`;
Go uses `g.TMNames[event]`.

---

## 8. Error Reporting

### C: Type-Specific Errors (ltm.c:150)

```c
void luaT_trybinTM (...) {
  if (callbinTM(L, p1, p2, res, event) < 0) {
    switch (event) {
      case TM_BAND: case TM_BOR: case TM_BXOR:
      case TM_SHL: case TM_SHR: case TM_BNOT:
        if (ttisnumber(p1) && ttisnumber(p2))
          luaG_tointerror(L, p1, p2);     // "number has no integer representation"
        else
          luaG_opinterror(L, p1, p2, "perform bitwise operation on");
      default:
        luaG_opinterror(L, p1, p2, "perform arithmetic on");
    }
  }
}
```

### Go: Equivalent Error Logic (vm.go:717)

```go
func tryBinTM(...) {
    // ... lookup tm ...
    if tm.IsNil() {
        if event >= mmapi.TM_BAND && event <= mmapi.TM_SHR || event == mmapi.TM_BNOT {
            if p1.IsNumber() && p2.IsNumber() {
                RunError(L, "number"+VarInfo(L, badReg)+" has no integer representation")
            }
            RunError(L, opErrorMsg(L, p1, p2, "perform bitwise operation on", reg1, reg2))
        }
        RunError(L, opErrorMsg(L, p1, p2, "perform arithmetic on", reg1, reg2))
    }
}
```

### Difference: Register Info in Errors

Go's `tryBinTM` takes `reg1, reg2` parameters to provide better error messages with
variable names (via `VarInfo`). C Lua's `luaG_opinterror` derives this from the debug
info and stack position.

---

## 9. Summary: Full Metamethod Dispatch Flow

```
1. OP_ADD executes:
   - int+int fast path → success → pc++ (skip MMBIN) → done
   - float conversion → success → pc++ (skip MMBIN) → done
   - both fail → pc NOT advanced → falls through

2. OP_MMBIN executes:
   - Read original instruction at pc-2 to get result register
   - Read metamethod tag from C field
   - Call trybinTM:
     a. Look up __add in p1's metatable
     b. Look up __add in p2's metatable
     c. If found → callTMres → push tm,p1,p2 → luaD_call → pop result → done
     d. If not found → error("attempt to perform arithmetic on a TYPE value")
```

## 10. Master Differences Table

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Skip pattern | `pc++` in macro on success | `ci.SavedPC++` on success |
| Fallthrough | Natural: pc not advanced | Same: SavedPC not advanced |
| MMBIN result reg | `RA(*(pc-2))` pointer deref | `base + GetArgA(code[ci.SavedPC-2])` |
| Protect wrapper | `Protect(luaT_trybinTM(...))` | Direct `tryBinTM(...)` call |
| Flip handling | Separate `trybinassocTM` function | Inline `if flip` in `tryBiniTM`/`tryBinKTM` |
| Error info | Derived from debug info | Explicit `reg1, reg2` parameters |
| Stack safety | `savestack`/`restorestack` in callTMres | Not needed (Go GC doesn't move stack) |
| TM lookup | `luaT_gettmbyobj` (C function) | `mmapi.GetTMByObj` (Go function) |
| Yield awareness | `isLuacode` → call vs callnoyield | Single `Call()` path |
| callTM layers | callbinTM → trybinTM → trybinassocTM → trybiniTM | tryBinTM → tryBiniTM → tryBinKTM (flatter) |
