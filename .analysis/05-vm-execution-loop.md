# 05 — VM Execution Loop (`lvm.c`, `lvm.h`)

> **Source**: `lua-master/lvm.c` (1972 lines), `lua-master/lvm.h` (136 lines), `lua-master/ljumptab.h` (114 lines)
> **Lua Version**: 5.5.1 (development)
> **Purpose**: The HEART of Lua — the main execution loop that fetches, decodes, and executes every instruction.

---

## Table of Contents

1. [luaV_execute Overview](#1-luav_execute-overview)
2. [Register-Based VM Model](#2-register-based-vm-model)
3. [Instruction Encoding](#3-instruction-encoding)
4. [Opcode Handlers by Category](#4-opcode-handlers-by-category)
   - 4a. Loading
   - 4b. Upvalues
   - 4c. Table Access
   - 4d. Arithmetic
   - 4e. Bitwise
   - 4f. Comparison
   - 4g. Unary
   - 4h. Concat/Close
   - 4i. Jumps & Tests
   - 4j. Calls & Returns
   - 4k. Loops
   - 4l. Vararg
   - 4m. Closure
   - 4n. Other / Metamethod Dispatch
5. [Metamethod Dispatch Deep-Dive](#5-metamethod-dispatch-deep-dive)
6. [Fast Path / Slow Path Pattern](#6-fast-path--slow-path-pattern)
7. [VM ↔ ldo.c Interaction](#7-vm--ldoc-interaction)
8. [If I Were Building This in Go](#8-if-i-were-building-this-in-go)
9. [Edge Cases](#9-edge-cases)
10. [Bug Pattern Guide](#10-bug-pattern-guide)

---

## 1. luaV_execute Overview

### Function Signature (line 1198)

```c
void luaV_execute (lua_State *L, CallInfo *ci);
```

### Main Loop Structure

```
luaV_execute(L, ci)
│
├── startfunc:                    // label for Lua→Lua calls
│   trap = L->hookmask
│
├── returning:                    // label for returns that continue in same C frame
│   cl = ci_func(ci)             // get LClosure
│   k = cl->p->k                // constants array
│   pc = ci->u.l.savedpc        // instruction pointer
│   if (trap) trap = luaG_tracecall(L)
│   base = ci->func.p + 1       // register base
│
└── for (;;) {                   // THE MAIN LOOP
        vmfetch()                // fetch + hook check
        vmdispatch(GET_OPCODE(i)) {
            vmcase(OP_XXX) { ... vmbreak; }
            ...
        }
    }
```

### Key Local Variables (lines 1199-1203)

| Variable | Type | Purpose |
|----------|------|---------|
| `cl` | `LClosure*` | Current closure being executed |
| `k` | `TValue*` | Pointer to constants array (`cl->p->k`) |
| `base` | `StkId` | Register base (`ci->func.p + 1`) |
| `pc` | `const Instruction*` | Program counter |
| `trap` | `int` | Hook/signal flag — when set, `vmfetch` calls `luaG_traceexec` |

### vmfetch Macro (lines 1185-1191)

```c
#define vmfetch() { \
  if (l_unlikely(trap)) {              /* stack reallocation or hooks? */ \
    trap = luaG_traceexec(L, pc);      /* handle hooks */ \
    updatebase(ci);                     /* correct stack (may have moved) */ \
  } \
  i = *(pc++);                          /* fetch and advance */ \
}
```

**Go equivalent:**
```go
func vmfetch() {
    if trap {
        trap = luaG_traceexec(L, pc)
        base = ci.Func + 1  // stack may have moved
    }
    i = code[pc]
    pc++
}
```

### Dispatch Strategy (lines 1193-1195 vs ljumptab.h)

Two dispatch modes:
1. **Switch dispatch** (default, non-GCC): `vmdispatch(o)` → `switch(o)`, `vmcase(l)` → `case l:`, `vmbreak` → `break`
2. **Computed goto** (GCC): `vmdispatch(x)` → `goto *disptab[x]`, `vmcase(l)` → `L_##l:`, `vmbreak` → `vmfetch(); vmdispatch(GET_OPCODE(i));`

The jump table in `ljumptab.h` (lines 18-113) contains `&&L_OP_XXX` labels for all opcodes.

### Protection Macros (lines 1144-1167)

| Macro | What it saves | Use case |
|-------|--------------|----------|
| `savepc(ci)` | `ci->u.l.savedpc = pc` | Before any error-raising code |
| `savestate(L,ci)` | `savepc + L->top.p = ci->top.p` | Before code that may reallocate stack |
| `Protect(exp)` | `savestate`, run exp, `updatetrap` | General protection — may reallocate stack, change hooks |
| `ProtectNT(exp)` | `savepc`, run exp, `updatetrap` | Protection without changing top |
| `halfProtect(exp)` | `savestate`, run exp (no updatetrap) | Code that raises errors but doesn't change hooks |

### Key Helper Macros (lines 1102-1110)

```c
#define RA(i)   (base+GETARG_A(i))       // register A (StkId)
#define vRA(i)  s2v(RA(i))               // TValue at register A
#define RB(i)   (base+GETARG_B(i))       // register B (StkId)
#define vRB(i)  s2v(RB(i))               // TValue at register B
#define KB(i)   (k+GETARG_B(i))          // constant B
#define RC(i)   (base+GETARG_C(i))       // register C (StkId)
#define vRC(i)  s2v(RC(i))               // TValue at register C
#define KC(i)   (k+GETARG_C(i))          // constant C
#define RKC(i)  ((TESTARG_k(i)) ? k + GETARG_C(i) : s2v(base + GETARG_C(i)))  // register or constant C
```

---

## 2. Register-Based VM Model

### Stack = Register File

Lua's VM is **register-based** (not stack-based like JVM). The Lua stack doubles as the register file for each function call.

```
Stack layout during a call:
┌──────────┬──────────┬──────────┬──────────┬──────────┬──────────┐
│ func     │ R[0]     │ R[1]     │ R[2]     │ ...      │ R[n]     │
│ ci->func │ base     │ base+1   │ base+2   │          │          │
│          │ (params) │ (params) │ (locals) │          │ (temps)  │
└──────────┴──────────┴──────────┴──────────┴──────────┴──────────┘
             ↑
             base = ci->func.p + 1
```

### Register Addressing

- `R[A]` = `base + GETARG_A(i)` — the A field of the instruction indexes into the register window
- `R[B]` = `base + GETARG_B(i)` — source operand
- `R[C]` = `base + GETARG_C(i)` — source operand or immediate
- `K[x]` = `cl->p->k[x]` — constant from the prototype's constant table

### The `s2v` Macro

`s2v(o)` converts a `StkId` (stack slot pointer) to a `TValue*` (value pointer). In Lua 5.5, stack slots contain `StackValue` which wraps `TValue` plus a `delta` field. `s2v` extracts the `TValue` part.

### Base Pointer Stability

**Critical**: `base` can become invalid after any operation that may reallocate the stack (calls, GC, errors). The `updatebase(ci)` macro refreshes it:

```c
#define updatebase(ci)  (base = ci->func.p + 1)
```

The `updatestack(ci)` macro conditionally refreshes both base and ra:
```c
#define updatestack(ci) { if (l_unlikely(trap)) { updatebase(ci); ra = RA(i); } }
```

---

## 3. Instruction Encoding

(From `lopcodes.h`, lines 14-32)

All instructions are **unsigned 32-bit integers** with opcode in the first 7 bits.

```
        3 3 2 2 2 2 2 2 2 2 2 2 1 1 1 1 1 1 1 1 1 1 0 0 0 0 0 0 0 0 0 0
        1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0
iABC          C(8)     |      B(8)     |k|     A(8)      |   Op(7)     |
ivABC         vC(10)     |     vB(6)   |k|     A(8)      |   Op(7)     |
iABx                Bx(17)               |     A(8)      |   Op(7)     |
iAsBx              sBx (signed)(17)      |     A(8)      |   Op(7)     |
iAx                           Ax(25)                     |   Op(7)     |
isJ                           sJ (signed)(25)            |   Op(7)     |
```

### Field Sizes

| Field | Bits | Range |
|-------|------|-------|
| Op | 7 | 0-127 |
| A | 8 | 0-255 |
| B | 8 | 0-255 |
| C | 8 | 0-255 |
| k | 1 | 0-1 |
| Bx | 17 | 0-131071 |
| sBx | 17 (signed) | -65536 to 65535 |
| Ax | 25 | 0-33554431 |
| sJ | 25 (signed) | -16777216 to 16777215 |
| vB | 6 | 0-63 |
| vC | 10 | 0-1023 |

### Signed Encoding

Signed fields use **excess-K** encoding: `value = unsigned_value - K`, where `K = max/2`.
- `sC2int(i)` = `i - OFFSET_sC` where `OFFSET_sC = 127`
- `GETARG_sBx(i)` = `GETARG_Bx(i) - OFFSET_sBx` where `OFFSET_sBx = 65535`
- `GETARG_sJ(i)` = `GETARG_sJ(i) - OFFSET_sJ` where `OFFSET_sJ = 16777215`


---

## 4. Opcode Handlers by Category

### 4a. Loading Opcodes

#### OP_MOVE (line 1233)
**Format**: `iABC` — `R[A] := R[B]`

```c
vmcase(OP_MOVE) {
    StkId ra = RA(i);
    setobjs2s(L, ra, RB(i));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_MOVE:
    ra := base + A(i)
    stack[ra] = stack[base + B(i)]  // full TValue copy
```

**⚠️ Trap**: `setobjs2s` does a full value copy including type tag. In Go, ensure you copy the entire Value struct, not just a pointer.

---

#### OP_LOADI (line 1238)
**Format**: `iAsBx` — `R[A] := sBx`

```c
vmcase(OP_LOADI) {
    StkId ra = RA(i);
    lua_Integer b = GETARG_sBx(i);
    setivalue(s2v(ra), b);
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_LOADI:
    ra := base + A(i)
    stack[ra].SetInteger(int64(sBx(i)))
```

**Note**: sBx is signed, range -65536 to 65535. For larger integers, the compiler uses OP_LOADK.

---

#### OP_LOADF (line 1244)
**Format**: `iAsBx` — `R[A] := (lua_Number)sBx`

```c
vmcase(OP_LOADF) {
    StkId ra = RA(i);
    int b = GETARG_sBx(i);
    setfltvalue(s2v(ra), cast_num(b));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_LOADF:
    ra := base + A(i)
    stack[ra].SetFloat(float64(sBx(i)))
```

**Note**: Only used for small integer-valued floats. Larger/fractional floats use OP_LOADK.

---

#### OP_LOADK (line 1250)
**Format**: `iABx` — `R[A] := K[Bx]`

```c
vmcase(OP_LOADK) {
    StkId ra = RA(i);
    TValue *rb = k + GETARG_Bx(i);
    setobj2s(L, ra, rb);
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_LOADK:
    ra := base + A(i)
    stack[ra] = proto.Constants[Bx(i)]
```

---

#### OP_LOADKX (line 1256)
**Format**: `iABx` + `EXTRAARG` — `R[A] := K[extra arg]`

```c
vmcase(OP_LOADKX) {
    StkId ra = RA(i);
    TValue *rb;
    rb = k + GETARG_Ax(*pc); pc++;   // next instruction is OP_EXTRAARG
    setobj2s(L, ra, rb);
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_LOADKX:
    ra := base + A(i)
    extraArg := code[pc]; pc++  // fetch OP_EXTRAARG
    stack[ra] = proto.Constants[Ax(extraArg)]
```

**Note**: Used when the constant index exceeds Bx's 17-bit range (>131071 constants). The next instruction must be OP_EXTRAARG carrying a 25-bit Ax field.

---

#### OP_LOADFALSE (line 1263)
**Format**: `iABC` — `R[A] := false`

```c
vmcase(OP_LOADFALSE) {
    StkId ra = RA(i);
    setbfvalue(s2v(ra));
    vmbreak;
}
```

---

#### OP_LFALSESKIP (line 1268)
**Format**: `iABC` — `R[A] := false; pc++`

```c
vmcase(OP_LFALSESKIP) {
    StkId ra = RA(i);
    setbfvalue(s2v(ra));
    pc++;  /* skip next instruction */
    vmbreak;
}
```

**Note**: Used to convert a condition to a boolean value. The skipped instruction is typically OP_LOADTRUE. Pattern: `LFALSESKIP; LOADTRUE` implements `not cond`.

---

#### OP_LOADTRUE (line 1274)
**Format**: `iABC` — `R[A] := true`

```c
vmcase(OP_LOADTRUE) {
    StkId ra = RA(i);
    setbtvalue(s2v(ra));
    vmbreak;
}
```

---

#### OP_LOADNIL (line 1279)
**Format**: `iABC` — `R[A], R[A+1], ..., R[A+B] := nil`

```c
vmcase(OP_LOADNIL) {
    StkId ra = RA(i);
    int b = GETARG_B(i);
    do {
        setnilvalue(s2v(ra++));
    } while (b--);
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_LOADNIL:
    ra := base + A(i)
    b := B(i)
    for j := 0; j <= b; j++ {
        stack[ra+j].SetNil()
    }
```

**⚠️ Trap**: Sets `B+1` registers to nil (inclusive range `[A, A+B]`), not `B` registers! The loop uses post-decrement `b--` after the `do`.

---

### 4b. Upvalue Opcodes

#### OP_GETUPVAL (line 1287)
**Format**: `iABC` — `R[A] := UpValue[B]`

```c
vmcase(OP_GETUPVAL) {
    StkId ra = RA(i);
    int b = GETARG_B(i);
    setobj2s(L, ra, cl->upvals[b]->v.p);
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_GETUPVAL:
    ra := base + A(i)
    stack[ra] = *cl.Upvals[B(i)].Value()  // dereference upvalue pointer
```

**Key detail**: `cl->upvals[b]->v.p` — the upvalue's `v.p` points to either:
- A stack slot (if the upvalue is still "open" — the variable is still alive in an enclosing frame)
- The upvalue's own `u.value` field (if the upvalue is "closed" — the variable's frame has returned)

---

#### OP_SETUPVAL (line 1293)
**Format**: `iABC` — `UpValue[B] := R[A]`

```c
vmcase(OP_SETUPVAL) {
    StkId ra = RA(i);
    UpVal *uv = cl->upvals[GETARG_B(i)];
    setobj(L, uv->v.p, s2v(ra));
    luaC_barrier(L, uv, s2v(ra));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_SETUPVAL:
    ra := base + A(i)
    uv := cl.Upvals[B(i)]
    *uv.Value() = stack[ra]
    // GC write barrier: uv now references the new value
    luaC_barrier(L, uv, stack[ra])
```

**⚠️ GC Trap**: Must call write barrier after setting an upvalue! If the upvalue (old generation) now points to a new-generation object, the GC needs to know.

---

#### OP_GETTABUP (line 1300)
**Format**: `iABC` — `R[A] := UpValue[B][K[C]:shortstring]`

```c
vmcase(OP_GETTABUP) {
    StkId ra = RA(i);
    TValue *upval = cl->upvals[GETARG_B(i)]->v.p;
    TValue *rc = KC(i);
    TString *key = tsvalue(rc);  /* key must be a short string */
    lu_byte tag;
    luaV_fastget(upval, key, s2v(ra), luaH_getshortstr, tag);
    if (tagisempty(tag))
        Protect(luaV_finishget(L, upval, rc, ra, tag));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_GETTABUP:
    ra := base + A(i)
    upval := cl.Upvals[B(i)].Value()
    key := proto.Constants[C(i)]  // must be short string
    // Fast path: direct table lookup
    if tbl, ok := upval.AsTable(); ok {
        if val, found := tbl.GetShortStr(key.AsString()); found {
            stack[ra] = val
            break
        }
    }
    // Slow path: metamethod chain
    stack[ra] = finishGet(L, upval, key, ra)
```

**This is the most common opcode** — every global variable access (`print`, `table`, etc.) goes through `OP_GETTABUP` on `_ENV`.

---

#### OP_SETTABUP (line 1349)
**Format**: `iABC` — `UpValue[A][K[B]:shortstring] := RK(C)`

```c
vmcase(OP_SETTABUP) {
    int hres;
    TValue *upval = cl->upvals[GETARG_A(i)]->v.p;
    TValue *rb = KB(i);
    TValue *rc = RKC(i);
    TString *key = tsvalue(rb);  /* key must be a short string */
    luaV_fastset(upval, key, rc, hres, luaH_psetshortstr);
    if (hres == HOK)
        luaV_finishfastset(L, upval, rc);
    else
        Protect(luaV_finishset(L, upval, rb, rc, hres));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_SETTABUP:
    upval := cl.Upvals[A(i)].Value()
    key := proto.Constants[B(i)]  // short string
    val := RKC(i)                 // register or constant based on k bit
    if tbl, ok := upval.AsTable(); ok {
        if tbl.SetShortStr(key.AsString(), val) {
            gcBarrierBack(L, tbl, val)
            break
        }
    }
    finishSet(L, upval, key, val)
```

**Note**: Uses `GETARG_A(i)` for the upvalue index (not B!). The table is in the upvalue, key is `K[B]`, value is `RK(C)`.


---

### 4c. Table Access Opcodes

#### OP_GETTABLE (line 1311)
**Format**: `iABC` — `R[A] := R[B][R[C]]`

```c
vmcase(OP_GETTABLE) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    TValue *rc = vRC(i);
    lu_byte tag;
    if (ttisinteger(rc)) {  /* fast track for integers? */
        luaV_fastgeti(rb, ivalue(rc), s2v(ra), tag);
    }
    else
        luaV_fastget(rb, rc, s2v(ra), luaH_get, tag);
    if (tagisempty(tag))
        Protect(luaV_finishget(L, rb, rc, ra, tag));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_GETTABLE:
    ra := base + A(i)
    rb := stack[base + B(i)]  // table
    rc := stack[base + C(i)]  // key
    if rc.IsInteger() {
        // Fast path for integer keys (array access)
        val, found := rb.AsTable().FastGetInt(rc.IntegerValue())
        if found { stack[ra] = val; break }
    } else {
        val, found := rb.AsTable().Get(rc)
        if found { stack[ra] = val; break }
    }
    // Slow path: __index metamethod
    stack[ra] = finishGet(L, rb, rc, ra)
```

**Key insight**: Integer keys get a separate fast path (`luaV_fastgeti` → `luaH_fastgeti`) that directly indexes the array part. This is the hot path for array access.

---

#### OP_GETI (line 1325)
**Format**: `iABC` — `R[A] := R[B][C]`

```c
vmcase(OP_GETI) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    int c = GETARG_C(i);
    lu_byte tag;
    luaV_fastgeti(rb, c, s2v(ra), tag);
    if (tagisempty(tag)) {
        TValue key;
        setivalue(&key, c);
        Protect(luaV_finishget(L, rb, &key, ra, tag));
    }
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_GETI:
    ra := base + A(i)
    rb := stack[base + B(i)]  // table
    c := C(i)                 // immediate integer key (0-255)
    val, found := rb.AsTable().FastGetInt(c)
    if found { stack[ra] = val; break }
    key := IntegerValue(c)
    stack[ra] = finishGet(L, rb, key, ra)
```

**Note**: C is an unsigned 8-bit immediate (0-255). For array indices 1-255, this avoids loading the index into a register.

---

#### OP_GETFIELD (line 1338)
**Format**: `iABC` — `R[A] := R[B][K[C]:shortstring]`

```c
vmcase(OP_GETFIELD) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    TValue *rc = KC(i);
    TString *key = tsvalue(rc);  /* key must be a short string */
    lu_byte tag;
    luaV_fastget(rb, key, s2v(ra), luaH_getshortstr, tag);
    if (tagisempty(tag))
        Protect(luaV_finishget(L, rb, rc, ra, tag));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_GETFIELD:
    ra := base + A(i)
    rb := stack[base + B(i)]  // table
    key := proto.Constants[C(i)]  // short string constant
    val, found := rb.AsTable().GetShortStr(key.AsString())
    if found { stack[ra] = val; break }
    stack[ra] = finishGet(L, rb, key, ra)
```

---

#### OP_SETTABLE (line 1362)
**Format**: `iABC` — `R[A][R[B]] := RK(C)`

```c
vmcase(OP_SETTABLE) {
    StkId ra = RA(i);
    int hres;
    TValue *rb = vRB(i);  /* key (table is in 'ra') */
    TValue *rc = RKC(i);  /* value */
    if (ttisinteger(rb)) {  /* fast track for integers? */
        luaV_fastseti(s2v(ra), ivalue(rb), rc, hres);
    }
    else {
        luaV_fastset(s2v(ra), rb, rc, hres, luaH_pset);
    }
    if (hres == HOK)
        luaV_finishfastset(L, s2v(ra), rc);
    else
        Protect(luaV_finishset(L, s2v(ra), rb, rc, hres));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_SETTABLE:
    ra := base + A(i)
    key := stack[base + B(i)]
    val := RKC(i)
    tbl := stack[ra].AsTable()
    if key.IsInteger() {
        if tbl.FastSetInt(key.IntegerValue(), val) {
            gcBarrierBack(L, tbl, val); break
        }
    } else {
        if tbl.Set(key, val) {
            gcBarrierBack(L, tbl, val); break
        }
    }
    finishSet(L, stack[ra], key, val)
```

---

#### OP_SETI (line 1379)
**Format**: `iABC` — `R[A][B] := RK(C)`

```c
vmcase(OP_SETI) {
    StkId ra = RA(i);
    int hres;
    int b = GETARG_B(i);
    TValue *rc = RKC(i);
    luaV_fastseti(s2v(ra), b, rc, hres);
    if (hres == HOK)
        luaV_finishfastset(L, s2v(ra), rc);
    else {
        TValue key;
        setivalue(&key, b);
        Protect(luaV_finishset(L, s2v(ra), &key, rc, hres));
    }
    vmbreak;
}
```

**Note**: B is an immediate integer key (0-255). Used for array-style assignment like `t[1] = x`.

---

#### OP_SETFIELD (line 1394)
**Format**: `iABC` — `R[A][K[B]:shortstring] := RK(C)`

```c
vmcase(OP_SETFIELD) {
    StkId ra = RA(i);
    int hres;
    TValue *rb = KB(i);
    TValue *rc = RKC(i);
    TString *key = tsvalue(rb);  /* key must be a short string */
    luaV_fastset(s2v(ra), key, rc, hres, luaH_psetshortstr);
    if (hres == HOK)
        luaV_finishfastset(L, s2v(ra), rc);
    else
        Protect(luaV_finishset(L, s2v(ra), rb, rc, hres));
    vmbreak;
}
```

---

#### OP_NEWTABLE (line 1407)
**Format**: `ivABC` + `EXTRAARG` — `R[A] := {}`

```c
vmcase(OP_NEWTABLE) {
    StkId ra = RA(i);
    unsigned b = cast_uint(GETARG_vB(i));  /* log2(hash size) + 1 */
    unsigned c = cast_uint(GETARG_vC(i));  /* array size */
    Table *t;
    if (b > 0)
        b = 1u << (b - 1);  /* hash size is 2^(b - 1) */
    if (TESTARG_k(i)) {  /* non-zero extra argument? */
        lua_assert(GETARG_Ax(*pc) != 0);
        c += cast_uint(GETARG_Ax(*pc)) * (MAXARG_vC + 1);
    }
    pc++;  /* skip extra argument */
    L->top.p = ra + 1;  /* correct top in case of emergency GC */
    t = luaH_new(L);  /* memory allocation */
    sethvalue2s(L, ra, t);
    if (b != 0 || c != 0)
        luaH_resize(L, t, c, b);  /* idem */
    checkGC(L, ra + 1);
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_NEWTABLE:
    ra := base + A(i)
    hashLog := vB(i)  // log2(hash_size) + 1, using variant B (6 bits)
    arraySize := vC(i)  // using variant C (10 bits)
    hashSize := 0
    if hashLog > 0 {
        hashSize = 1 << (hashLog - 1)
    }
    if testK(i) {  // extra argument present
        extra := Ax(code[pc]); pc++
        arraySize += extra * (MAXARG_vC + 1)  // extend array size
    } else {
        pc++  // always skip the EXTRAARG slot
    }
    t := newTable(L)
    stack[ra].SetTable(t)
    if hashSize > 0 || arraySize > 0 {
        t.Resize(arraySize, hashSize)
    }
```

**⚠️ Critical details**:
1. Uses **variant** format (`ivABC`): vB is 6 bits, vC is 10 bits
2. `vB` encodes `log2(hash_size) + 1`, not the hash size directly
3. `vC` is the array size, but can be extended via EXTRAARG: `total_array = vC + Ax * (MAXARG_vC + 1)`
4. **Always** consumes the next instruction slot (EXTRAARG), even if k=0
5. Sets `L->top.p = ra + 1` before allocation for emergency GC safety

---

#### OP_SELF (line 1428)
**Format**: `iABC` — `R[A+1] := R[B]; R[A] := R[B][K[C]:shortstring]`

```c
vmcase(OP_SELF) {
    StkId ra = RA(i);
    lu_byte tag;
    TValue *rb = vRB(i);
    TValue *rc = KC(i);
    TString *key = tsvalue(rc);  /* key must be a short string */
    setobj2s(L, ra + 1, rb);    /* copy 'self' to ra+1 FIRST */
    luaV_fastget(rb, key, s2v(ra), luaH_getshortstr, tag);
    if (tagisempty(tag))
        Protect(luaV_finishget(L, rb, rc, ra, tag));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_SELF:
    ra := base + A(i)
    rb := stack[base + B(i)]
    key := proto.Constants[C(i)]  // method name
    stack[ra+1] = rb  // save 'self' BEFORE the lookup (rb may == ra)
    // Look up method in rb's table
    val, found := rb.AsTable().GetShortStr(key.AsString())
    if found { stack[ra] = val; break }
    stack[ra] = finishGet(L, rb, key, ra)
```

**⚠️ Trap**: `ra+1` is set BEFORE the table lookup. This is critical because `A` and `B` might overlap (e.g., `A == B`). The self-reference must be saved before `ra` is overwritten with the method.

---

#### OP_SETLIST (line 1901)
**Format**: `ivABC` + optional `EXTRAARG` — `R[A][vC+i] := R[A+i], 1 <= i <= vB`

```c
vmcase(OP_SETLIST) {
    StkId ra = RA(i);
    unsigned n = cast_uint(GETARG_vB(i));
    unsigned last = cast_uint(GETARG_vC(i));
    Table *h = hvalue(s2v(ra));
    if (n == 0)
        n = cast_uint(L->top.p - ra) - 1;  /* get up to the top */
    else
        L->top.p = ci->top.p;  /* correct top in case of emergency GC */
    last += n;
    if (TESTARG_k(i)) {
        last += cast_uint(GETARG_Ax(*pc)) * (MAXARG_vC + 1);
        pc++;
    }
    if (last > h->asize) {  /* needs more space? */
        lua_assert(GETARG_vB(i) == 0);
        luaH_resizearray(L, h, last);  /* preallocate it at once */
    }
    for (; n > 0; n--) {
        TValue *val = s2v(ra + n);
        obj2arr(h, last - 1, val);
        last--;
        luaC_barrierback(L, obj2gco(h), val);
    }
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_SETLIST:
    ra := base + A(i)
    n := vB(i)      // number of elements to set
    last := vC(i)   // starting index (0-based internally, but 1-based in Lua)
    tbl := stack[ra].AsTable()
    if n == 0 {
        n = L.Top - ra - 1  // variable number (from previous CALL)
    }
    last += n
    if testK(i) {
        last += Ax(code[pc]) * (MAXARG_vC + 1)
        pc++
    }
    if last > tbl.ArraySize() {
        tbl.ResizeArray(last)
    }
    for j := n; j > 0; j-- {
        tbl.SetArrayIndex(last-1, stack[ra+j])  // 0-based array
        last--
    }
```

**⚠️ Trap**: When `vB == 0`, the count comes from `L->top - ra - 1` (set by a preceding OP_CALL with C=0). The `last` index may be extended via EXTRAARG for tables with >1023 elements in the constructor.


---

### 4d. Arithmetic Opcodes

The arithmetic opcodes use a layered macro system (lines 927-1016) that generates the fast path inline. The pattern is always:

1. **Fast path**: Try integer operation, then float operation
2. **Slow path**: If neither works, fall through to the **next instruction** which is always `OP_MMBIN`, `OP_MMBINI`, or `OP_MMBINK`

#### The Macro System

```c
// Integer operations (lines 927-929) — use unsigned cast for wrapping
#define l_addi(L,a,b)  intop(+, a, b)   // → l_castU2S(l_castS2U(a) + l_castS2U(b))
#define l_subi(L,a,b)  intop(-, a, b)
#define l_muli(L,a,b)  intop(*, a, b)

// op_arithI — arithmetic with immediate operand (line 944)
// Used by: OP_ADDI
// Pattern: R[A] = R[B] op sC
#define op_arithI(L,iop,fop) {
  TValue *v1 = vRB(i);
  int imm = GETARG_sC(i);
  if (ttisinteger(v1)) {
    pc++; setivalue(ra, iop(L, ivalue(v1), imm));  // skip OP_MMBINI
  }
  else if (ttisfloat(v1)) {
    pc++; setfltvalue(ra, fop(L, fltvalue(v1), cast_num(imm)));
  }
  // else: fall through to OP_MMBINI (pc NOT incremented)
}

// op_arithK — arithmetic with constant operand (line 1013)
// Used by: OP_ADDK, OP_SUBK, OP_MULK, etc.
// Pattern: R[A] = R[B] op K[C]
#define op_arithK(L,iop,fop) {
  TValue *v1 = vRB(i);
  TValue *v2 = KC(i);
  op_arith_aux(L, v1, v2, iop, fop);
}

// op_arith — arithmetic with register operands (line 1004)
// Used by: OP_ADD, OP_SUB, OP_MUL, etc.
// Pattern: R[A] = R[B] op R[C]
#define op_arith(L,iop,fop) {
  TValue *v1 = vRB(i);
  TValue *v2 = vRC(i);
  op_arith_aux(L, v1, v2, iop, fop);
}

// op_arith_aux — the core: try int, then float (line 992)
#define op_arith_aux(L,v1,v2,iop,fop) {
  if (ttisinteger(v1) && ttisinteger(v2)) {
    pc++; setivalue(s2v(ra), iop(L, ivalue(v1), ivalue(v2)));
  }
  else op_arithf_aux(L, v1, v2, fop);
}

// op_arithf_aux — float-only path (line 963)
#define op_arithf_aux(L,v1,v2,fop) {
  lua_Number n1; lua_Number n2;
  if (tonumberns(v1, n1) && tonumberns(v2, n2)) {
    pc++; setfltvalue(s2v(ra), fop(L, n1, n2));
  }
  // else: fall through to OP_MMBIN/MMBINK
}

// op_arithf — float-only with register operands (line 974)
// Used by: OP_POW, OP_DIV (always produce floats)
#define op_arithf(L,fop) {
  TValue *v1 = vRB(i);
  TValue *v2 = vRC(i);
  op_arithf_aux(L, v1, v2, fop);
}
```

**⚠️ CRITICAL: The `pc++` Pattern**

When the fast path succeeds, `pc` is incremented to **skip the following OP_MMBIN/MMBINI/MMBINK instruction**. When it fails (operands aren't numbers), `pc` is NOT incremented, so execution falls through to the metamethod dispatch instruction.

This means **every arithmetic opcode is always followed by an OP_MMBIN variant** in the bytecode.

---

#### OP_ADDI (line 1440)
**Format**: `iABC` — `R[A] := R[B] + sC`

```c
vmcase(OP_ADDI) {
    op_arithI(L, l_addi, luai_numadd);
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_ADDI:
    ra := base + A(i)
    rb := stack[base + B(i)]
    imm := sC(i)  // signed immediate (-127 to 127)
    if rb.IsInteger() {
        pc++  // skip OP_MMBINI
        stack[ra].SetInteger(rb.IntegerValue() + int64(imm))  // wrapping add
    } else if rb.IsFloat() {
        pc++
        stack[ra].SetFloat(rb.FloatValue() + float64(imm))
    }
    // else: fall through to OP_MMBINI
```

---

#### OP_ADDK (line 1444)
**Format**: `iABC` — `R[A] := R[B] + K[C]:number`

```c
vmcase(OP_ADDK) {
    op_arithK(L, l_addi, luai_numadd);
    vmbreak;
}
```

---

#### OP_SUBK (line 1448)
**Format**: `iABC` — `R[A] := R[B] - K[C]:number`

```c
vmcase(OP_SUBK) {
    op_arithK(L, l_subi, luai_numsub);
    vmbreak;
}
```

---

#### OP_MULK (line 1452)
**Format**: `iABC` — `R[A] := R[B] * K[C]:number`

```c
vmcase(OP_MULK) {
    op_arithK(L, l_muli, luai_nummul);
    vmbreak;
}
```

---

#### OP_MODK (line 1456)
**Format**: `iABC` — `R[A] := R[B] % K[C]:number`

```c
vmcase(OP_MODK) {
    savestate(L, ci);  /* in case of division by 0 */
    op_arithK(L, luaV_mod, luaV_modf);
    vmbreak;
}
```

**Note**: `savestate` is called BEFORE the operation because `luaV_mod` can raise "attempt to perform 'n%%0'" error.

---

#### OP_POWK (line 1461)
**Format**: `iABC` — `R[A] := R[B] ^ K[C]:number`

```c
vmcase(OP_POWK) {
    op_arithfK(L, luai_numpow);
    vmbreak;
}
```

**Note**: Power is **float-only** (`op_arithfK`). Even `2^3` produces `8.0`, not `8`.

---

#### OP_DIVK (line 1465)
**Format**: `iABC` — `R[A] := R[B] / K[C]:number`

```c
vmcase(OP_DIVK) {
    op_arithfK(L, luai_numdiv);
    vmbreak;
}
```

**Note**: Float division is **always float** (`op_arithfK`). Even `6/3` produces `2.0`.

---

#### OP_IDIVK (line 1469)
**Format**: `iABC` — `R[A] := R[B] // K[C]:number`

```c
vmcase(OP_IDIVK) {
    savestate(L, ci);  /* in case of division by 0 */
    op_arithK(L, luaV_idiv, luai_numidiv);
    vmbreak;
}
```

**Note**: Integer floor division. `luaV_idiv` handles the special case of `MININT // -1` (uses unsigned negation to avoid overflow).

---

#### OP_ADD (line 1506)
**Format**: `iABC` — `R[A] := R[B] + R[C]`

```c
vmcase(OP_ADD) {
    op_arith(L, l_addi, luai_numadd);
    vmbreak;
}
```

---

#### OP_SUB (line 1510)
**Format**: `iABC` — `R[A] := R[B] - R[C]`

```c
vmcase(OP_SUB) {
    op_arith(L, l_subi, luai_numsub);
    vmbreak;
}
```

---

#### OP_MUL (line 1514)
**Format**: `iABC` — `R[A] := R[B] * R[C]`

```c
vmcase(OP_MUL) {
    op_arith(L, l_muli, luai_nummul);
    vmbreak;
}
```

---

#### OP_MOD (line 1518)
**Format**: `iABC` — `R[A] := R[B] % R[C]`

```c
vmcase(OP_MOD) {
    savestate(L, ci);  /* in case of division by 0 */
    op_arith(L, luaV_mod, luaV_modf);
    vmbreak;
}
```

---

#### OP_POW (line 1523)
**Format**: `iABC` — `R[A] := R[B] ^ R[C]`

```c
vmcase(OP_POW) {
    op_arithf(L, luai_numpow);
    vmbreak;
}
```

---

#### OP_DIV (line 1527)
**Format**: `iABC` — `R[A] := R[B] / R[C]`

```c
vmcase(OP_DIV) {  /* float division (always with floats) */
    op_arithf(L, luai_numdiv);
    vmbreak;
}
```

---

#### OP_IDIV (line 1531)
**Format**: `iABC` — `R[A] := R[B] // R[C]`

```c
vmcase(OP_IDIV) {  /* floor division */
    savestate(L, ci);  /* in case of division by 0 */
    op_arith(L, luaV_idiv, luai_numidiv);
    vmbreak;
}
```

---

#### luaV_idiv — Integer Floor Division (line 766)

```c
lua_Integer luaV_idiv (lua_State *L, lua_Integer m, lua_Integer n) {
    if (l_unlikely(l_castS2U(n) + 1u <= 1u)) {  /* special cases: -1 or 0 */
        if (n == 0)
            luaG_runerror(L, "attempt to divide by zero");
        return intop(-, 0, m);   /* n==-1; avoid overflow with 0x80000...//-1 */
    }
    else {
        lua_Integer q = m / n;  /* perform C division */
        if ((m ^ n) < 0 && m % n != 0)  /* 'm/n' would be negative non-integer? */
            q -= 1;  /* correct result for different rounding */
        return q;
    }
}
```

**⚠️ Key edge cases**:
- `n == 0`: runtime error
- `n == -1`: uses `intop(-, 0, m)` = unsigned negation to avoid `MININT / -1` overflow
- Negative results: C truncates toward zero, but Lua floors toward negative infinity. Correction: `q -= 1` when signs differ and there's a remainder.

---

#### luaV_mod — Integer Modulus (line 786)

```c
lua_Integer luaV_mod (lua_State *L, lua_Integer m, lua_Integer n) {
    if (l_unlikely(l_castS2U(n) + 1u <= 1u)) {
        if (n == 0)
            luaG_runerror(L, "attempt to perform 'n%%0'");
        return 0;   /* m % -1 == 0 */
    }
    else {
        lua_Integer r = m % n;
        if (r != 0 && (r ^ n) < 0)  /* different signs? */
            r += n;  /* correct result for different rounding */
        return r;
    }
}
```

**⚠️ Key edge cases**:
- `n == 0`: runtime error
- `n == -1`: always returns 0 (avoids overflow)
- Lua modulus always has the same sign as the divisor (unlike C's `%`)

---

#### luaV_modf — Float Modulus (line 804)

```c
lua_Number luaV_modf (lua_State *L, lua_Number m, lua_Number n) {
    lua_Number r;
    luai_nummod(L, m, n, r);  // r = m - floor(m/n)*n
    return r;
}
```


---

### 4e. Bitwise Opcodes

Bitwise operations require **integer** operands (floats that are exact integers are coerced via `tointegerns`). If coercion fails, fall through to OP_MMBIN/MMBINK.

#### The Macro System

```c
// Bitwise primitives (lines 930-932)
#define l_band(a,b)  intop(&, a, b)
#define l_bor(a,b)   intop(|, a, b)
#define l_bxor(a,b)  intop(^, a, b)

// op_bitwiseK — with constant operand (line 1022)
#define op_bitwiseK(L,op) {
  TValue *v1 = vRB(i);
  TValue *v2 = KC(i);
  lua_Integer i1;
  lua_Integer i2 = ivalue(v2);  // constant is always integer
  if (tointegerns(v1, &i1)) {
    pc++; setivalue(s2v(ra), op(i1, i2));
  }
}

// op_bitwise — with register operands (line 1036)
#define op_bitwise(L,op) {
  TValue *v1 = vRB(i);
  TValue *v2 = vRC(i);
  lua_Integer i1; lua_Integer i2;
  if (tointegerns(v1, &i1) && tointegerns(v2, &i2)) {
    pc++; setivalue(s2v(ra), op(i1, i2));
  }
}
```

**Note**: `tointegerns` (no string coercion) converts floats to integers if they are exact integers. E.g., `3.0 & 5` works, but `3.5 & 5` falls through to metamethod.

---

#### OP_BANDK (line 1474)
**Format**: `iABC` — `R[A] := R[B] & K[C]:integer`

```c
vmcase(OP_BANDK) { op_bitwiseK(L, l_band); vmbreak; }
```

#### OP_BORK (line 1478)
**Format**: `iABC` — `R[A] := R[B] | K[C]:integer`

```c
vmcase(OP_BORK) { op_bitwiseK(L, l_bor); vmbreak; }
```

#### OP_BXORK (line 1482)
**Format**: `iABC` — `R[A] := R[B] ~ K[C]:integer`

```c
vmcase(OP_BXORK) { op_bitwiseK(L, l_bxor); vmbreak; }
```

---

#### OP_SHLI (line 1486)
**Format**: `iABC` — `R[A] := sC << R[B]`

```c
vmcase(OP_SHLI) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    int ic = GETARG_sC(i);
    lua_Integer ib;
    if (tointegerns(rb, &ib)) {
        pc++; setivalue(s2v(ra), luaV_shiftl(ic, ib));
    }
    vmbreak;
}
```

**⚠️ CRITICAL**: Note the operand order! `R[A] = sC << R[B]`, NOT `R[B] << sC`. The immediate `sC` is the value being shifted, and `R[B]` is the shift amount. This is `luaV_shiftl(ic, ib)` = shift `ic` left by `ib`.

---

#### OP_SHRI (line 1496)
**Format**: `iABC` — `R[A] := R[B] >> sC`

```c
vmcase(OP_SHRI) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    int ic = GETARG_sC(i);
    lua_Integer ib;
    if (tointegerns(rb, &ib)) {
        pc++; setivalue(s2v(ra), luaV_shiftl(ib, -ic));
    }
    vmbreak;
}
```

**Note**: Right shift is implemented as left shift with negated amount: `luaV_shiftl(ib, -ic)`.

---

#### OP_BAND (line 1536)
**Format**: `iABC` — `R[A] := R[B] & R[C]`

```c
vmcase(OP_BAND) { op_bitwise(L, l_band); vmbreak; }
```

#### OP_BOR (line 1540)
**Format**: `iABC` — `R[A] := R[B] | R[C]`

```c
vmcase(OP_BOR) { op_bitwise(L, l_bor); vmbreak; }
```

#### OP_BXOR (line 1544)
**Format**: `iABC` — `R[A] := R[B] ~ R[C]`

```c
vmcase(OP_BXOR) { op_bitwise(L, l_bxor); vmbreak; }
```

#### OP_SHL (line 1548)
**Format**: `iABC` — `R[A] := R[B] << R[C]`

```c
vmcase(OP_SHL) { op_bitwise(L, luaV_shiftl); vmbreak; }
```

#### OP_SHR (line 1552)
**Format**: `iABC` — `R[A] := R[B] >> R[C]`

```c
vmcase(OP_SHR) { op_bitwise(L, luaV_shiftr); vmbreak; }
```

**Note**: `luaV_shiftr(x,y)` is defined as `luaV_shiftl(x, intop(-, 0, y))` — negate the shift amount.

---

#### luaV_shiftl — Shift Left (line 818)

```c
lua_Integer luaV_shiftl (lua_Integer x, lua_Integer y) {
    if (y < 0) {  /* shift right? */
        if (y <= -NBITS) return 0;
        else return intop(>>, x, -y);
    }
    else {  /* shift left */
        if (y >= NBITS) return 0;
        else return intop(<<, x, y);
    }
}
```

**Go pseudocode:**
```go
func luaV_shiftl(x, y int64) int64 {
    if y < 0 {  // shift right
        if y <= -64 { return 0 }
        return int64(uint64(x) >> uint(-y))
    } else {  // shift left
        if y >= 64 { return 0 }
        return int64(uint64(x) << uint(y))
    }
}
```

**⚠️ Key edge cases**:
- Shift amounts >= 64 (NBITS) return 0 (not undefined behavior like in C)
- Shift amounts <= -64 return 0
- Shifts are done in unsigned to avoid undefined behavior in C (and to get logical right shift, not arithmetic)
- **Known test failure**: `bitwise.lua` fails on negative shift — ensure the sign handling matches exactly


---

### 4f. Comparison Opcodes

All comparison opcodes follow the **conditional jump** pattern:
1. Compare two values → get boolean `cond`
2. If `cond != k` (the k bit in the instruction), skip the next instruction
3. If `cond == k`, execute the next instruction (which must be OP_JMP)

The `docondjump` macro (line 1138):
```c
#define docondjump()  if (cond != GETARG_k(i)) pc++; else donextjump(ci);
```

And `donextjump` (line 1131):
```c
#define donextjump(ci)  { Instruction ni = *pc; dojump(ci, ni, 1); }
```

The `+1` in `dojump(ci, ni, 1)` accounts for the fact that `pc` already points past the JMP instruction.

#### The op_order Macro (line 1051)

```c
#define op_order(L,opi,opn,other) {
  TValue *ra = vRA(i);
  int cond;
  TValue *rb = vRB(i);
  if (ttisinteger(ra) && ttisinteger(rb)) {      // fast: both integers
    lua_Integer ia = ivalue(ra);
    lua_Integer ib = ivalue(rb);
    cond = opi(ia, ib);
  }
  else if (ttisnumber(ra) && ttisnumber(rb))      // medium: both numbers
    cond = opn(ra, rb);                           // handles int/float mix
  else
    Protect(cond = other(L, ra, rb));             // slow: metamethod
  docondjump();
}
```

#### The op_orderI Macro (line 1071)

```c
#define op_orderI(L,opi,opf,inv,tm) {
  TValue *ra = vRA(i);
  int cond;
  int im = GETARG_sB(i);                          // signed immediate
  if (ttisinteger(ra))
    cond = opi(ivalue(ra), im);                    // integer fast path
  else if (ttisfloat(ra)) {
    lua_Number fa = fltvalue(ra);
    lua_Number fim = cast_num(im);
    cond = opf(fa, fim);                           // float comparison
  }
  else {
    int isf = GETARG_C(i);
    Protect(cond = luaT_callorderiTM(L, ra, im, inv, isf, tm));  // metamethod
  }
  docondjump();
}
```

---

#### OP_EQ (line 1650)
**Format**: `iABC` — `if ((R[A] == R[B]) ~= k) then pc++`

```c
vmcase(OP_EQ) {
    StkId ra = RA(i);
    int cond;
    TValue *rb = vRB(i);
    Protect(cond = luaV_equalobj(L, s2v(ra), rb));
    docondjump();
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_EQ:
    ra := stack[base + A(i)]
    rb := stack[base + B(i)]
    cond := luaV_equalobj(L, ra, rb)  // may call __eq metamethod
    if cond != k(i) { pc++ } else { doNextJump() }
```

**Note**: `luaV_equalobj` (line 582) handles cross-type equality: integer==float comparison converts float to integer if exact. Also handles __eq metamethod for tables and userdata.

---

#### OP_LT (line 1658)
**Format**: `iABC` — `if ((R[A] < R[B]) ~= k) then pc++`

```c
vmcase(OP_LT) {
    op_order(L, l_lti, LTnum, lessthanothers);
    vmbreak;
}
```

---

#### OP_LE (line 1662)
**Format**: `iABC` — `if ((R[A] <= R[B]) ~= k) then pc++`

```c
vmcase(OP_LE) {
    op_order(L, l_lei, LEnum, lessequalothers);
    vmbreak;
}
```

---

#### OP_EQK (line 1666)
**Format**: `iABC` — `if ((R[A] == K[B]) ~= k) then pc++`

```c
vmcase(OP_EQK) {
    StkId ra = RA(i);
    TValue *rb = KB(i);
    /* basic types do not use '__eq'; we can use raw equality */
    int cond = luaV_rawequalobj(s2v(ra), rb);
    docondjump();
    vmbreak;
}
```

**Note**: Uses **raw equality** (no metamethods) because constants are basic types (nil, boolean, number, string). `luaV_rawequalobj` = `luaV_equalobj(NULL, t1, t2)` — passing NULL for L disables metamethods.

---

#### OP_EQI (line 1674)
**Format**: `iABC` — `if ((R[A] == sB) ~= k) then pc++`

```c
vmcase(OP_EQI) {
    StkId ra = RA(i);
    int cond;
    int im = GETARG_sB(i);
    if (ttisinteger(s2v(ra)))
        cond = (ivalue(s2v(ra)) == im);
    else if (ttisfloat(s2v(ra)))
        cond = luai_numeq(fltvalue(s2v(ra)), cast_num(im));
    else
        cond = 0;  /* other types cannot be equal to a number */
    docondjump();
    vmbreak;
}
```

**Note**: No metamethod call — if R[A] is not a number, it simply can't equal an integer immediate.

---

#### OP_LTI (line 1687)
**Format**: `iABC` — `if ((R[A] < sB) ~= k) then pc++`

```c
vmcase(OP_LTI) {
    op_orderI(L, l_lti, luai_numlt, 0, TM_LT);
    vmbreak;
}
```

#### OP_LEI (line 1691)
**Format**: `iABC` — `if ((R[A] <= sB) ~= k) then pc++`

```c
vmcase(OP_LEI) {
    op_orderI(L, l_lei, luai_numle, 0, TM_LE);
    vmbreak;
}
```

#### OP_GTI (line 1695)
**Format**: `iABC` — `if ((R[A] > sB) ~= k) then pc++`

```c
vmcase(OP_GTI) {
    op_orderI(L, l_gti, luai_numgt, 1, TM_LT);
    vmbreak;
}
```

**⚠️ Key detail**: `OP_GTI` uses `TM_LT` with `inv=1`. The metamethod call becomes `__lt(sB, R[A])` — operands are **inverted**! `a > b` is implemented as `b < a`.

#### OP_GEI (line 1699)
**Format**: `iABC` — `if ((R[A] >= sB) ~= k) then pc++`

```c
vmcase(OP_GEI) {
    op_orderI(L, l_gei, luai_numge, 1, TM_LE);
    vmbreak;
}
```

**⚠️ Same pattern**: `OP_GEI` uses `TM_LE` with `inv=1`. `a >= b` → `b <= a`.

---

### Mixed Integer/Float Comparison Functions (lines 426-531)

These are critical for correctness and handle the tricky case where one operand is integer and the other is float.

```c
// LTintfloat: is integer i < float f? (line 426)
l_sinline int LTintfloat (lua_Integer i, lua_Number f) {
    if (l_intfitsf(i))
        return luai_numlt(cast_num(i), f);  // safe to compare as floats
    else {
        lua_Integer fi;
        if (luaV_flttointeger(f, &fi, F2Iceil))  // fi = ceil(f)
            return i < fi;   // compare as integers
        else
            return f > 0;  // f is out of integer range
    }
}
```

**Go pseudocode:**
```go
func LTintfloat(i int64, f float64) bool {
    if intFitsFloat(i) {
        return float64(i) < f
    }
    // i might lose precision as float; use integer comparison
    fi, ok := floatToInteger(f, F2Iceil)
    if ok {
        return i < fi
    }
    return f > 0  // f is beyond integer range
}
```

**⚠️ NaN handling**: When `f` is NaN, `luai_numlt` returns false, `f > 0` returns false. All comparisons with NaN correctly return false.


---

### 4g. Unary Opcodes

#### OP_UNM (line 1586)
**Format**: `iABC` — `R[A] := -R[B]`

```c
vmcase(OP_UNM) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    lua_Number nb;
    if (ttisinteger(rb)) {
        lua_Integer ib = ivalue(rb);
        setivalue(s2v(ra), intop(-, 0, ib));  // unsigned negation
    }
    else if (tonumberns(rb, nb)) {
        setfltvalue(s2v(ra), luai_numunm(L, nb));
    }
    else
        Protect(luaT_trybinTM(L, rb, rb, ra, TM_UNM));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_UNM:
    ra := base + A(i)
    rb := stack[base + B(i)]
    if rb.IsInteger() {
        // unsigned negation: handles MININT correctly (MININT stays MININT)
        stack[ra].SetInteger(int64(-uint64(rb.IntegerValue())))
    } else if n, ok := toNumberNS(rb); ok {
        stack[ra].SetFloat(-n)
    } else {
        luaT_trybinTM(L, rb, rb, ra, TM_UNM)  // __unm metamethod
    }
```

**⚠️ Trap**: Integer negation uses `intop(-, 0, ib)` = unsigned subtraction. This means `-MININT == MININT` (wraps around). This is correct Lua behavior.

**Note**: `luaT_trybinTM` is called with `rb, rb` (same operand twice) for unary operations.

---

#### OP_BNOT (line 1601)
**Format**: `iABC` — `R[A] := ~R[B]`

```c
vmcase(OP_BNOT) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    lua_Integer ib;
    if (tointegerns(rb, &ib)) {
        setivalue(s2v(ra), intop(^, ~l_castS2U(0), ib));
    }
    else
        Protect(luaT_trybinTM(L, rb, rb, ra, TM_BNOT));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_BNOT:
    ra := base + A(i)
    rb := stack[base + B(i)]
    if ib, ok := toIntegerNS(rb); ok {
        stack[ra].SetInteger(^ib)  // bitwise NOT
    } else {
        luaT_trybinTM(L, rb, rb, ra, TM_BNOT)
    }
```

**Note**: `intop(^, ~l_castS2U(0), ib)` = XOR with all-ones = bitwise NOT. In Go, just use `^ib`.

---

#### OP_NOT (line 1612)
**Format**: `iABC` — `R[A] := not R[B]`

```c
vmcase(OP_NOT) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    if (l_isfalse(rb))
        setbtvalue(s2v(ra));
    else
        setbfvalue(s2v(ra));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_NOT:
    ra := base + A(i)
    rb := stack[base + B(i)]
    if rb.IsNil() || (rb.IsBoolean() && !rb.BooleanValue()) {
        stack[ra].SetBoolean(true)
    } else {
        stack[ra].SetBoolean(false)
    }
```

**Note**: No metamethod! `not` always returns a boolean. Only `nil` and `false` are falsy.

---

#### OP_LEN (line 1621)
**Format**: `iABC` — `R[A] := #R[B]`

```c
vmcase(OP_LEN) {
    StkId ra = RA(i);
    Protect(luaV_objlen(L, ra, vRB(i)));
    vmbreak;
}
```

**luaV_objlen** (line 731):
```c
void luaV_objlen (lua_State *L, StkId ra, const TValue *rb) {
    const TValue *tm;
    switch (ttypetag(rb)) {
        case LUA_VTABLE: {
            Table *h = hvalue(rb);
            tm = fasttm(L, h->metatable, TM_LEN);
            if (tm) break;  /* metamethod? break to call it */
            setivalue(s2v(ra), l_castU2S(luaH_getn(L, h)));  /* primitive len */
            return;
        }
        case LUA_VSHRSTR:
            setivalue(s2v(ra), tsvalue(rb)->shrlen);
            return;
        case LUA_VLNGSTR:
            setivalue(s2v(ra), cast_st2S(tsvalue(rb)->u.lnglen));
            return;
        default:
            tm = luaT_gettmbyobj(L, rb, TM_LEN);
            if (l_unlikely(notm(tm)))
                luaG_typeerror(L, rb, "get length of");
            break;
    }
    luaT_callTMres(L, tm, rb, rb, ra);  // call __len metamethod
}
```

**Go pseudocode:**
```go
case OP_LEN:
    ra := base + A(i)
    rb := stack[base + B(i)]
    switch rb.Type() {
    case LUA_TTABLE:
        tbl := rb.AsTable()
        if tm := tbl.Metatable().FastTM(TM_LEN); tm != nil {
            callTMres(L, tm, rb, rb, ra)
        } else {
            stack[ra].SetInteger(int64(tbl.Length()))
        }
    case LUA_TSTRING:
        stack[ra].SetInteger(int64(rb.StringLen()))
    default:
        tm := getTMbyObj(L, rb, TM_LEN)
        if tm == nil { typeError(L, rb, "get length of") }
        callTMres(L, tm, rb, rb, ra)
    }
```

---

### 4h. Concat/Close Opcodes

#### OP_CONCAT (line 1626)
**Format**: `iABC` — `R[A] := R[A].. ... ..R[A + B - 1]`

```c
vmcase(OP_CONCAT) {
    StkId ra = RA(i);
    int n = GETARG_B(i);  /* number of elements to concatenate */
    L->top.p = ra + n;  /* mark the end of concat operands */
    ProtectNT(luaV_concat(L, n));
    checkGC(L, L->top.p);
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_CONCAT:
    ra := base + A(i)
    n := B(i)  // number of values to concatenate
    L.Top = ra + n
    luaV_concat(L, n)  // concatenate in-place, result at stack[ra]
    checkGC(L)
```

**luaV_concat** (line 684) concatenates `n` values from `top-n` to `top-1`:
- Handles `__concat` metamethod for non-string/non-number values
- Optimizes empty string concatenation
- Batches multiple string concatenations (collects as many as possible)
- Short results use stack buffer; long results allocate directly

**⚠️ Trap**: The B field is the **count** of values, starting from R[A]. After concatenation, the result is at `stack[ra]`.

---

#### OP_CLOSE (line 1634)
**Format**: `iABC` — `close all upvalues >= R[A]`

```c
vmcase(OP_CLOSE) {
    StkId ra = RA(i);
    lua_assert(!GETARG_B(i));  /* 'close must be alive */
    Protect(luaF_close(L, ra, LUA_OK, 1));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_CLOSE:
    ra := base + A(i)
    luaF_close(L, ra, LUA_OK, true)  // close upvalues, call __close methods
```

**Note**: Closes all open upvalues at or above register A. Also calls `__close` metamethods on to-be-closed variables. The `1` parameter means "call close methods".

---

#### OP_TBC (line 1640)
**Format**: `iABC` — `mark variable A "to be closed"`

```c
vmcase(OP_TBC) {
    StkId ra = RA(i);
    halfProtect(luaF_newtbcupval(L, ra));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_TBC:
    ra := base + A(i)
    luaF_newtbcupval(L, ra)  // create to-be-closed upvalue
```

**Note**: Creates a special upvalue that will have its `__close` method called when it goes out of scope. This implements the `<close>` variable annotation.


---

### 4i. Jump & Test Opcodes

#### OP_JMP (line 1646)
**Format**: `isJ` — `pc += sJ`

```c
vmcase(OP_JMP) {
    dojump(ci, i, 0);
    vmbreak;
}
```

Where `dojump` (line 1127):
```c
#define dojump(ci,i,e)  { pc += GETARG_sJ(i) + e; updatetrap(ci); }
```

**Go pseudocode:**
```go
case OP_JMP:
    pc += sJ(i)  // signed 25-bit offset
    trap = ci.Trap  // allow signals to break loops
```

**Note**: `updatetrap(ci)` refreshes the local `trap` variable from `ci->u.l.trap`. This is essential for tight loops — without it, a debug hook or signal could never interrupt a loop that only contains JMP instructions.

---

#### OP_TEST (line 1703)
**Format**: `iABC` — `if (not R[A] == k) then pc++`

```c
vmcase(OP_TEST) {
    StkId ra = RA(i);
    int cond = !l_isfalse(s2v(ra));
    docondjump();
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_TEST:
    ra := stack[base + A(i)]
    cond := !isFalse(ra)  // true if ra is truthy
    if cond != k(i) { pc++ } else { doNextJump() }
```

**Note**: Used for `if x then` / `if not x then`. Tests truthiness of R[A]. The `k` bit inverts the sense: `k=0` means "jump if truthy", `k=1` means "jump if falsy".

---

#### OP_TESTSET (line 1709)
**Format**: `iABC` — `if (not R[B] == k) then pc++ else R[A] := R[B]`

```c
vmcase(OP_TESTSET) {
    StkId ra = RA(i);
    TValue *rb = vRB(i);
    if (l_isfalse(rb) == GETARG_k(i))
        pc++;
    else {
        setobj2s(L, ra, rb);
        donextjump(ci);
    }
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_TESTSET:
    ra := base + A(i)
    rb := stack[base + B(i)]
    if isFalse(rb) == (k(i) != 0) {
        pc++  // condition failed, skip jump
    } else {
        stack[ra] = rb  // copy value AND jump
        doNextJump()
    }
```

**Note**: Used for short-circuit evaluation (`and`/`or`). Unlike OP_TEST, this also copies the tested value to R[A] before jumping. This implements:
- `x = a or b` → TESTSET copies `a` to result if truthy, otherwise falls through to evaluate `b`
- `x = a and b` → TESTSET copies `a` to result if falsy, otherwise falls through to evaluate `b`


---

### 4j. Call & Return Opcodes

These are the most complex opcodes and the primary interface between the VM and `ldo.c`.

#### OP_CALL (line 1720)
**Format**: `iABC` — `R[A], ... ,R[A+C-2] := R[A](R[A+1], ... ,R[A+B-1])`

```c
vmcase(OP_CALL) {
    StkId ra = RA(i);
    CallInfo *newci;
    int b = GETARG_B(i);
    int nresults = GETARG_C(i) - 1;
    if (b != 0)  /* fixed number of arguments? */
        L->top.p = ra + b;  /* top signals number of arguments */
    /* else previous instruction set top */
    savepc(ci);  /* in case of errors */
    if ((newci = luaD_precall(L, ra, nresults)) == NULL)
        updatetrap(ci);  /* C call; nothing else to be done */
    else {  /* Lua call: run function in this same C frame */
        ci = newci;
        goto startfunc;
    }
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_CALL:
    ra := base + A(i)
    b := B(i)         // number of args + 1 (0 = use top)
    nresults := C(i) - 1  // expected results (-1 = LUA_MULTRET)
    if b != 0 {
        L.Top = ra + b  // fixed args: func + b-1 args
    }
    // else: variable args, top already set by previous instruction
    
    newci := luaD_precall(L, ra, nresults)
    if newci == nil {
        // C function call completed synchronously
        updateTrap(ci)
    } else {
        // Lua function: continue in same Go frame
        ci = newci
        goto startfunc  // re-enter the main loop with new ci
    }
```

**Key semantics**:
- `B == 0`: variable number of arguments (top was set by previous OP_CALL/OP_VARARG)
- `B != 0`: `B-1` arguments (func is at ra, args at ra+1..ra+B-1)
- `C == 0`: variable number of results (caller will use top), next instruction is "open" (OP_CALL, OP_RETURN, OP_SETLIST)
- `C != 0`: `C-1` expected results
- `luaD_precall` returns NULL for C functions (already executed), non-NULL CallInfo for Lua functions

**⚠️ Critical for Go**: The `goto startfunc` pattern means Lua→Lua calls DON'T create new Go stack frames. This is how C Lua avoids C stack overflow for deep Lua call chains. In Go, you'll need to either:
1. Use a loop (iterative dispatch) — recommended
2. Use goroutines (expensive)
3. Accept Go stack growth (Go stacks are growable, so this actually works)

---

#### OP_TAILCALL (line 1737)
**Format**: `iABC` — `return R[A](R[A+1], ... ,R[A+B-1])`

```c
vmcase(OP_TAILCALL) {
    StkId ra = RA(i);
    int b = GETARG_B(i);  /* number of arguments + 1 (function) */
    int n;  /* number of results when calling a C function */
    int nparams1 = GETARG_C(i);
    /* delta is virtual 'func' - real 'func' (vararg functions) */
    int delta = (nparams1) ? ci->u.l.nextraargs + nparams1 : 0;
    if (b != 0)
        L->top.p = ra + b;
    else  /* previous instruction set top */
        b = cast_int(L->top.p - ra);
    savepc(ci);  /* several calls here can raise errors */
    if (TESTARG_k(i)) {
        luaF_closeupval(L, base);  /* close upvalues from current call */
        lua_assert(L->tbclist.p < base);  /* no pending tbc variables */
        lua_assert(base == ci->func.p + 1);
    }
    if ((n = luaD_pretailcall(L, ci, ra, b, delta)) < 0)  /* Lua function? */
        goto startfunc;  /* execute the callee */
    else {  /* C function? */
        ci->func.p -= delta;  /* restore 'func' (if vararg) */
        luaD_poscall(L, ci, n);  /* finish caller */
        updatetrap(ci);  /* 'luaD_poscall' can change hooks */
        goto ret;  /* caller returns after the tail call */
    }
}
```

**Go pseudocode:**
```go
case OP_TAILCALL:
    ra := base + A(i)
    b := B(i)
    nparams1 := C(i)
    delta := 0
    if nparams1 != 0 {
        delta = ci.ExtraArgs + nparams1  // vararg adjustment
    }
    if b != 0 {
        L.Top = ra + b
    } else {
        b = L.Top - ra
    }
    if testK(i) {
        luaF_closeupval(L, base)  // close upvalues before reusing frame
    }
    n := luaD_pretailcall(L, ci, ra, b, delta)
    if n < 0 {
        // Lua function: frame was reused, re-enter loop
        goto startfunc
    } else {
        // C function: finish and return
        ci.Func -= delta
        luaD_poscall(L, ci, n)
        goto ret
    }
```

**Key semantics**:
- `C` field stores `nparams1` (number of fixed parameters + 1, or 0 if not vararg)
- `k` bit: if set, there are upvalues to close before the tail call
- `delta`: adjustment for vararg functions where the virtual func position differs from the real one
- `luaD_pretailcall` reuses the current CallInfo frame (moves args down to current func position)
- Returns < 0 for Lua calls (continue in same C frame), >= 0 for C calls (number of results)

**⚠️ Known trap**: `calls.lua` fails with "attempt to call nil value" — the handoff between OP_TAILCALL and luaD_pretailcall is a known pain point. Ensure `b` is computed correctly when `B == 0`.

---

#### OP_RETURN (line 1763)
**Format**: `iABC` — `return R[A], ... ,R[A+B-2]`

```c
vmcase(OP_RETURN) {
    StkId ra = RA(i);
    int n = GETARG_B(i) - 1;  /* number of results */
    int nparams1 = GETARG_C(i);
    if (n < 0)  /* not fixed? */
        n = cast_int(L->top.p - ra);  /* get what is available */
    savepc(ci);
    if (TESTARG_k(i)) {  /* may there be open upvalues? */
        ci->u2.nres = n;  /* save number of returns */
        if (L->top.p < ci->top.p)
            L->top.p = ci->top.p;
        luaF_close(L, base, CLOSEKTOP, 1);
        updatetrap(ci);
        updatestack(ci);
    }
    if (nparams1)  /* vararg function? */
        ci->func.p -= ci->u.l.nextraargs + nparams1;
    L->top.p = ra + n;  /* set call for 'luaD_poscall' */
    luaD_poscall(L, ci, n);
    updatetrap(ci);  /* 'luaD_poscall' can change hooks */
    goto ret;
}
```

**Go pseudocode:**
```go
case OP_RETURN:
    ra := base + A(i)
    n := B(i) - 1  // number of return values (-1 = variable)
    nparams1 := C(i)
    if n < 0 {
        n = L.Top - ra  // variable returns: use everything above ra
    }
    if testK(i) {
        // Close upvalues and to-be-closed variables
        ci.NRes = n
        if L.Top < ci.Top { L.Top = ci.Top }
        luaF_close(L, base, CLOSEKTOP, true)
        updateTrap(ci)
        updateStack(ci)  // base/ra may have changed
    }
    if nparams1 != 0 {
        // Vararg function: restore real func position
        ci.Func -= ci.ExtraArgs + nparams1
    }
    L.Top = ra + n
    luaD_poscall(L, ci, n)
    goto ret
```

**Key semantics**:
- `B == 0`: variable number of returns (use `top - ra`)
- `B != 0`: `B-1` fixed returns
- `k` bit: upvalues/tbc variables need closing
- `C` (nparams1): if non-zero, this is a vararg function needing func pointer adjustment

---

#### OP_RETURN0 (line 1785)
**Format**: `iABC` — `return` (no values)

```c
vmcase(OP_RETURN0) {
    if (l_unlikely(L->hookmask)) {
        StkId ra = RA(i);
        L->top.p = ra;
        savepc(ci);
        luaD_poscall(L, ci, 0);  /* no hurry... */
        trap = 1;
    }
    else {  /* do the 'poscall' here */
        int nres = get_nresults(ci->callstatus);
        L->ci = ci->previous;  /* back to caller */
        L->top.p = base - 1;
        for (; l_unlikely(nres > 0); nres--)
            setnilvalue(s2v(L->top.p++));  /* all results are nil */
    }
    goto ret;
}
```

**Go pseudocode:**
```go
case OP_RETURN0:
    if L.HookMask != 0 {
        // Slow path with hooks
        luaD_poscall(L, ci, 0)
        trap = true
    } else {
        // Fast path: inline poscall
        nres := getNResults(ci.CallStatus)
        L.CI = ci.Previous
        L.Top = base - 1  // pop the function frame
        for ; nres > 0; nres-- {
            stack[L.Top].SetNil()  // fill expected results with nil
            L.Top++
        }
    }
    goto ret
```

**Note**: This is an optimized version of OP_RETURN for the common case of `return` with no values. When there are no hooks, it inlines the `luaD_poscall` logic to avoid the function call overhead.

---

#### OP_RETURN1 (line 1802)
**Format**: `iABC` — `return R[A]`

```c
vmcase(OP_RETURN1) {
    if (l_unlikely(L->hookmask)) {
        StkId ra = RA(i);
        L->top.p = ra + 1;
        savepc(ci);
        luaD_poscall(L, ci, 1);
        trap = 1;
    }
    else {  /* do the 'poscall' here */
        int nres = get_nresults(ci->callstatus);
        L->ci = ci->previous;  /* back to caller */
        if (nres == 0)
            L->top.p = base - 1;  /* asked for no results */
        else {
            StkId ra = RA(i);
            setobjs2s(L, base - 1, ra);  /* at least this result */
            L->top.p = base;
            for (; l_unlikely(nres > 1); nres--)
                setnilvalue(s2v(L->top.p++));  /* complete missing results */
        }
    }
   ret:  /* return from a Lua function */
    if (ci->callstatus & CIST_FRESH)
        return;  /* end this frame */
    else {
        ci = ci->previous;
        goto returning;  /* continue running caller in this frame */
    }
}
```

**Go pseudocode:**
```go
case OP_RETURN1:
    if L.HookMask != 0 {
        ra := base + A(i)
        L.Top = ra + 1
        luaD_poscall(L, ci, 1)
        trap = true
    } else {
        nres := getNResults(ci.CallStatus)
        L.CI = ci.Previous
        if nres == 0 {
            L.Top = base - 1
        } else {
            ra := base + A(i)
            stack[base-1] = stack[ra]  // copy return value to caller's expected slot
            L.Top = base
            for ; nres > 1; nres-- {
                stack[L.Top].SetNil()
                L.Top++
            }
        }
    }
    // ret: label — return from Lua function
    if ci.CallStatus & CIST_FRESH != 0 {
        return  // end this luaV_execute invocation
    }
    ci = ci.Previous
    goto returning  // continue running caller in same C frame
```

**⚠️ CRITICAL: The `ret` label**

The `ret:` label (line 1823) is shared by ALL return opcodes. It decides whether to:
1. **Return from luaV_execute** (`CIST_FRESH` flag set): This happens when the call was initiated from C code (e.g., `lua_call`). The `CIST_FRESH` flag marks the boundary between C and Lua frames.
2. **Continue in the same C frame** (`goto returning`): This happens for Lua→Lua calls. Instead of returning from luaV_execute and re-entering it, the code jumps to `returning:` which reinitializes `cl`, `k`, `pc`, `base` for the caller's frame.

This is the **trampoline pattern** that avoids C stack growth for Lua→Lua call chains.


---

### 4k. Loop Opcodes

#### OP_FORPREP (line 1849)
**Format**: `iAsBx` — prepare numerical for loop; if skip, `pc += Bx + 1`

```c
vmcase(OP_FORPREP) {
    StkId ra = RA(i);
    savestate(L, ci);  /* in case of errors */
    if (forprep(L, ra))
        pc += GETARG_Bx(i) + 1;  /* skip the loop */
    vmbreak;
}
```

**The `forprep` function** (line 214) is critical. Before execution:
```
ra     : initial value
ra + 1 : limit
ra + 2 : step
```

After preparation (integer loop):
```
ra     : loop counter (number of iterations remaining)
ra + 1 : step
ra + 2 : control variable (current value, starts at init)
```

After preparation (float loop):
```
ra     : limit
ra + 1 : step
ra + 2 : control variable (starts at init)
```

**Go pseudocode:**
```go
case OP_FORPREP:
    ra := base + A(i)
    init := stack[ra]
    limit := stack[ra+1]
    step := stack[ra+2]
    
    if init.IsInteger() && step.IsInteger() {
        // Integer loop path
        iInit := init.IntegerValue()
        iStep := step.IntegerValue()
        if iStep == 0 { error("'for' step is zero") }
        iLimit := forlimit(L, iInit, limit, iStep)
        if shouldSkip {
            pc += Bx(i) + 1; break
        }
        // Convert to counter-based loop
        var count uint64
        if iStep > 0 {
            count = uint64(iLimit) - uint64(iInit)
            if iStep != 1 { count /= uint64(iStep) }
        } else {
            count = uint64(iInit) - uint64(iLimit)
            count /= uint64(-(iStep+1)) + 1
        }
        stack[ra].SetInteger(int64(count))    // counter
        stack[ra+1].SetInteger(iStep)         // step (unchanged)
        stack[ra+2].SetInteger(iInit)         // control var = init
    } else {
        // Float loop path
        fInit := toFloat(init)
        fLimit := toFloat(limit)
        fStep := toFloat(step)
        if fStep == 0 { error("'for' step is zero") }
        if (fStep > 0 && fLimit < fInit) || (fStep < 0 && fInit < fLimit) {
            pc += Bx(i) + 1; break  // skip
        }
        stack[ra].SetFloat(fLimit)    // limit
        stack[ra+1].SetFloat(fStep)   // step
        stack[ra+2].SetFloat(fInit)   // control var = init
    }
```

**⚠️ CRITICAL: Counter-based integer loops (Lua 5.5 change)**

In Lua 5.5, integer for-loops use a **counter** instead of comparing against the limit each iteration. The counter is computed as:
- `count = (limit - init) / step` (for positive step)
- `count = (init - limit) / (-(step+1) + 1)` (for negative step)

All arithmetic is done in **unsigned** to avoid overflow. The `-(step+1) + 1` trick avoids negating `MININT`.

This is a significant change from Lua 5.4 where the loop compared `idx <= limit` (or `idx >= limit`) each iteration. The counter approach:
1. Eliminates the comparison on each iteration
2. Handles overflow correctly (unsigned arithmetic)
3. Makes the loop body simpler (just decrement counter)

---

#### OP_FORLOOP (line 1831)
**Format**: `iABx` — `update counters; if loop continues then pc -= Bx`

```c
vmcase(OP_FORLOOP) {
    StkId ra = RA(i);
    if (ttisinteger(s2v(ra + 1))) {  /* integer loop? */
        lua_Unsigned count = l_castS2U(ivalue(s2v(ra)));
        if (count > 0) {  /* still more iterations? */
            lua_Integer step = ivalue(s2v(ra + 1));
            lua_Integer idx = ivalue(s2v(ra + 2));  /* control variable */
            chgivalue(s2v(ra), l_castU2S(count - 1));  /* update counter */
            idx = intop(+, idx, step);  /* add step to index */
            chgivalue(s2v(ra + 2), idx);  /* update control variable */
            pc -= GETARG_Bx(i);  /* jump back */
        }
    }
    else if (floatforloop(L, ra))  /* float loop */
        pc -= GETARG_Bx(i);  /* jump back */
    updatetrap(ci);  /* allows a signal to break the loop */
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_FORLOOP:
    ra := base + A(i)
    if stack[ra+1].IsInteger() {
        // Integer loop: counter-based
        count := uint64(stack[ra].IntegerValue())
        if count > 0 {
            step := stack[ra+1].IntegerValue()
            idx := stack[ra+2].IntegerValue()
            stack[ra].SetInteger(int64(count - 1))       // decrement counter
            idx = int64(uint64(idx) + uint64(step))      // wrapping add
            stack[ra+2].SetInteger(idx)                   // update control var
            pc -= Bx(i)                                   // jump back to loop body
        }
        // else: counter exhausted, fall through (loop ends)
    } else {
        // Float loop: comparison-based
        if floatForLoop(L, ra) {
            pc -= Bx(i)
        }
    }
    trap = ci.Trap  // allow signals to break the loop
```

**⚠️ Key details**:
1. Integer loop checks `count > 0`, not a comparison against limit
2. Counter decrement and index update use **unsigned** arithmetic (wrapping)
3. `updatetrap(ci)` at the end allows debug hooks to interrupt tight loops
4. **Known trap**: `gc.lua` times out — likely infinite loop. If the counter is wrong (e.g., signed vs unsigned mismatch), the loop may never terminate.

---

#### OP_TFORPREP (line 1856)
**Format**: `iABx` — prepare generic for loop

Before: `ra` = iterator, `ra+1` = state, `ra+2` = initial control, `ra+3` = closing variable

```c
vmcase(OP_TFORPREP) {
    StkId ra = RA(i);
    TValue temp;
    setobj(L, &temp, s2v(ra + 3));       // save closing var
    setobjs2s(L, ra + 3, ra + 2);        // move control to ra+3
    setobj2s(L, ra + 2, &temp);           // move closing to ra+2
    halfProtect(luaF_newtbcupval(L, ra + 2));  // mark closing as tbc
    pc += GETARG_Bx(i);                   // jump to end (OP_TFORCALL)
    i = *(pc++);                          // fetch OP_TFORCALL
    lua_assert(GET_OPCODE(i) == OP_TFORCALL && ra == RA(i));
    goto l_tforcall;
}
```

**Go pseudocode:**
```go
case OP_TFORPREP:
    ra := base + A(i)
    // Swap control and closing variables
    // Before: ra=iter, ra+1=state, ra+2=control, ra+3=closing
    // After:  ra=iter, ra+1=state, ra+2=closing(tbc), ra+3=control
    temp := stack[ra+3]
    stack[ra+3] = stack[ra+2]
    stack[ra+2] = temp
    luaF_newtbcupval(L, ra+2)  // mark closing var as to-be-closed
    pc += Bx(i)                // jump to loop end
    i = code[pc]; pc++         // fetch OP_TFORCALL
    goto l_tforcall            // execute it immediately
```

**Note**: The swap puts the closing variable at `ra+2` so it can be marked as to-be-closed. The control variable moves to `ra+3` where the iterator results will go.

---

#### OP_TFORCALL (line 1875)
**Format**: `iABC` — call iterator function

Layout: `ra` = iterator, `ra+1` = state, `ra+2` = closing, `ra+3` = control

```c
vmcase(OP_TFORCALL) {
   l_tforcall: {
    StkId ra = RA(i);
    setobjs2s(L, ra + 5, ra + 3);  /* copy the control variable */
    setobjs2s(L, ra + 4, ra + 1);  /* copy state */
    setobjs2s(L, ra + 3, ra);      /* copy function */
    L->top.p = ra + 3 + 3;
    ProtectNT(luaD_call(L, ra + 3, GETARG_C(i)));  /* do the call */
    updatestack(ci);  /* stack may have changed */
    i = *(pc++);  /* go to next instruction */
    lua_assert(GET_OPCODE(i) == OP_TFORLOOP && ra == RA(i));
    goto l_tforloop;
  }}
```

**Go pseudocode:**
```go
case OP_TFORCALL:
    ra := base + A(i)
    // Set up call: iter(state, control)
    stack[ra+5] = stack[ra+3]  // copy control variable
    stack[ra+4] = stack[ra+1]  // copy state
    stack[ra+3] = stack[ra]    // copy iterator function
    L.Top = ra + 6             // 3 args (func + 2 args)
    luaD_call(L, ra+3, C(i))  // call with C expected results
    updateStack(ci)
    i = code[pc]; pc++         // fetch OP_TFORLOOP
    goto l_tforloop
```

---

#### OP_TFORLOOP (line 1894)
**Format**: `iABx` — check iterator result and loop

```c
vmcase(OP_TFORLOOP) {
   l_tforloop: {
    StkId ra = RA(i);
    if (!ttisnil(s2v(ra + 3)))  /* continue loop? */
        pc -= GETARG_Bx(i);  /* jump back */
    vmbreak;
  }}
```

**Go pseudocode:**
```go
case OP_TFORLOOP:
    ra := base + A(i)
    if !stack[ra+3].IsNil() {
        pc -= Bx(i)  // jump back to loop body
    }
    // else: iterator returned nil, loop ends
```

**Note**: The first return value of the iterator (at `ra+3`) becomes the new control variable. If it's nil, the loop terminates. The closing variable at `ra+2` will have its `__close` method called when the loop exits.


---

### 4l. Vararg Opcodes

#### OP_VARARG (line 1936)
**Format**: `iABC` — `R[A], ..., R[A+C-2] = varargs`

```c
vmcase(OP_VARARG) {
    StkId ra = RA(i);
    int n = GETARG_C(i) - 1;  /* required results (-1 means all) */
    int vatab = GETARG_k(i) ? GETARG_B(i) : -1;
    Protect(luaT_getvarargs(L, ci, ra, n, vatab));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_VARARG:
    ra := base + A(i)
    n := C(i) - 1   // number of varargs to copy (-1 = all)
    vatab := -1
    if testK(i) {
        vatab = B(i)  // vararg table index (Lua 5.5 feature)
    }
    luaT_getvarargs(L, ci, ra, n, vatab)
```

**Key semantics**:
- `C == 0`: copy ALL varargs, set top accordingly (variable results)
- `C != 0`: copy exactly `C-1` varargs (pad with nil if fewer available)
- `k` bit + `B`: Lua 5.5 feature — vararg table support
- `luaT_getvarargs` copies the extra arguments from below the function's fixed parameters

---

#### OP_GETVARG (line 1943) — **NEW in Lua 5.5**
**Format**: `iABC` — `R[A] := R[B][R[C]], R[B] is vararg parameter`

```c
vmcase(OP_GETVARG) {
    StkId ra = RA(i);
    TValue *rc = vRC(i);
    luaT_getvararg(ci, ra, rc);
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_GETVARG:
    ra := base + A(i)
    rc := stack[base + C(i)]  // index into varargs
    luaT_getvararg(ci, ra, rc)  // get single vararg by index
```

**Note**: This is a Lua 5.5 addition for efficient single-vararg access without copying all varargs. Used for `select(n, ...)` style patterns.

---

#### OP_VARARGPREP (line 1955)
**Format**: `iABC` — adjust varargs

```c
vmcase(OP_VARARGPREP) {
    ProtectNT(luaT_adjustvarargs(L, ci, cl->p));
    if (l_unlikely(trap)) {  /* previous "Protect" updated trap */
        luaD_hookcall(L, ci);
        L->oldpc = 1;  /* next opcode will be seen as a "new" line */
    }
    updatebase(ci);  /* function has new base after adjustment */
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_VARARGPREP:
    luaT_adjustvarargs(L, ci, cl.Proto)
    if trap {
        luaD_hookcall(L, ci)
        L.OldPC = 1
    }
    base = ci.Func + 1  // MUST update base — it changed!
```

**⚠️ CRITICAL**: This is always the **first instruction** of a vararg function. `luaT_adjustvarargs` moves the fixed parameters up and stores the extra arguments below them. After this, `base` has changed, so `updatebase(ci)` is mandatory.

---

#### OP_ERRNNIL (line 1949) — **NEW in Lua 5.5**
**Format**: `iABx` — `raise error if R[A] ~= nil`

```c
vmcase(OP_ERRNNIL) {
    TValue *ra = vRA(i);
    if (!ttisnil(ra))
        halfProtect(luaG_errnnil(L, cl, GETARG_Bx(i)));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_ERRNNIL:
    ra := stack[base + A(i)]
    if !ra.IsNil() {
        luaG_errnnil(L, cl, Bx(i))  // raise error
    }
```

**Note**: Lua 5.5 addition. `Bx` is an index into the constants for the global name (or 0 if not available). Used for detecting errors in global variable access patterns.

---

### 4m. Closure Opcode

#### OP_CLOSURE (line 1929)
**Format**: `iABx` — `R[A] := closure(KPROTO[Bx])`

```c
vmcase(OP_CLOSURE) {
    StkId ra = RA(i);
    Proto *p = cl->p->p[GETARG_Bx(i)];
    halfProtect(pushclosure(L, p, cl->upvals, base, ra));
    checkGC(L, ra + 1);
    vmbreak;
}
```

**The `pushclosure` function** (line 834):
```c
static void pushclosure (lua_State *L, Proto *p, UpVal **encup, StkId base,
                         StkId ra) {
    int nup = p->sizeupvalues;
    Upvaldesc *uv = p->upvalues;
    int i;
    LClosure *ncl = luaF_newLclosure(L, nup);
    ncl->p = p;
    setclLvalue2s(L, ra, ncl);  /* anchor new closure in stack */
    for (i = 0; i < nup; i++) {  /* fill in its upvalues */
        if (uv[i].instack)  /* upvalue refers to local variable? */
            ncl->upvals[i] = luaF_findupval(L, base + uv[i].idx);
        else  /* get upvalue from enclosing function */
            ncl->upvals[i] = encup[uv[i].idx];
        luaC_objbarrier(L, ncl, ncl->upvals[i]);
    }
}
```

**Go pseudocode:**
```go
case OP_CLOSURE:
    ra := base + A(i)
    proto := cl.Proto.InnerProtos[Bx(i)]
    ncl := newLuaClosure(L, proto.NumUpvalues)
    ncl.Proto = proto
    stack[ra].SetClosure(ncl)  // anchor in stack BEFORE filling upvalues
    
    for j := 0; j < proto.NumUpvalues; j++ {
        uvDesc := proto.Upvalues[j]
        if uvDesc.InStack {
            // Upvalue refers to a local in the current frame
            ncl.Upvals[j] = findUpval(L, base + uvDesc.Index)
        } else {
            // Upvalue inherited from enclosing closure
            ncl.Upvals[j] = cl.Upvals[uvDesc.Index]
        }
        gcObjBarrier(L, ncl, ncl.Upvals[j])
    }
    checkGC(L)
```

**⚠️ Key details**:
1. The closure is anchored in the stack (`setclLvalue2s`) BEFORE filling upvalues — this prevents the GC from collecting it during `luaF_findupval`
2. `uv[i].instack`: the upvalue refers to a local variable in the current (enclosing) function's stack frame. `luaF_findupval` either finds an existing open upvalue for that slot or creates a new one.
3. `!uv[i].instack`: the upvalue is inherited from the enclosing closure's upvalue array (transitive closure capture)
4. **Known pain point**: Upvalue handling was a multi-commit struggle in the Go implementation. The `instack` vs inherited distinction and the `luaF_findupval` linked-list management are error-prone.

---

### 4n. Metamethod Dispatch Opcodes

These opcodes are **never executed directly by user code**. They are always placed immediately after an arithmetic/bitwise opcode and are only reached when the fast path fails (operands aren't numbers).

#### OP_MMBIN (line 1556)
**Format**: `iABC` — `call C metamethod over R[A] and R[B]`

```c
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

**Go pseudocode:**
```go
case OP_MMBIN:
    ra := stack[base + A(i)]
    prevInstr := code[pc - 2]  // the arithmetic instruction that failed
    rb := stack[base + B(i)]
    tm := TMS(C(i))            // which metamethod (__add, __sub, etc.)
    result := base + A(prevInstr)  // where to put the result
    luaT_trybinTM(L, ra, rb, result, tm)
```

**⚠️ Key detail**: The result goes to `RA(pi)` — the A register of the **previous** arithmetic instruction (at `pc-2`), NOT the A register of OP_MMBIN itself. This is because the arithmetic instruction's A field is where the result should go.

---

#### OP_MMBINI (line 1566)
**Format**: `iABC` — `call C metamethod over R[A] and sB` (immediate operand)

```c
vmcase(OP_MMBINI) {
    StkId ra = RA(i);
    Instruction pi = *(pc - 2);  /* original arith. expression */
    int imm = GETARG_sB(i);
    TMS tm = (TMS)GETARG_C(i);
    int flip = GETARG_k(i);
    StkId result = RA(pi);
    Protect(luaT_trybiniTM(L, s2v(ra), imm, flip, result, tm));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_MMBINI:
    ra := stack[base + A(i)]
    prevInstr := code[pc - 2]
    imm := sB(i)              // signed immediate
    tm := TMS(C(i))
    flip := k(i)              // if 1, operands were flipped
    result := base + A(prevInstr)
    luaT_trybiniTM(L, ra, imm, flip, result, tm)
```

**Note**: `flip` indicates whether the original expression had the immediate on the left side. E.g., `5 + x` has `flip=1` because the immediate `5` is the first operand. The metamethod needs to know the correct operand order.

---

#### OP_MMBINK (line 1576)
**Format**: `iABC` — `call C metamethod over R[A] and K[B]` (constant operand)

```c
vmcase(OP_MMBINK) {
    StkId ra = RA(i);
    Instruction pi = *(pc - 2);  /* original arith. expression */
    TValue *imm = KB(i);
    TMS tm = (TMS)GETARG_C(i);
    int flip = GETARG_k(i);
    StkId result = RA(pi);
    Protect(luaT_trybinassocTM(L, s2v(ra), imm, flip, result, tm));
    vmbreak;
}
```

**Go pseudocode:**
```go
case OP_MMBINK:
    ra := stack[base + A(i)]
    prevInstr := code[pc - 2]
    imm := proto.Constants[B(i)]
    tm := TMS(C(i))
    flip := k(i)
    result := base + A(prevInstr)
    luaT_trybinassocTM(L, ra, imm, flip, result, tm)
```

**Note**: `luaT_trybinassocTM` handles the flip: if `flip=1`, it calls `luaT_trybinTM(L, imm, ra, ...)` (swapped order); if `flip=0`, it calls `luaT_trybinTM(L, ra, imm, ...)`.

---

### 4o. Extra Opcode

#### OP_EXTRAARG (line 1964)
**Format**: `iAx` — extra argument for previous opcode

```c
vmcase(OP_EXTRAARG) {
    lua_assert(0);
    vmbreak;
}
```

**Note**: This opcode should **never** be dispatched directly. It's always consumed by the preceding instruction (OP_LOADKX, OP_NEWTABLE, OP_SETLIST). If execution reaches it, that's a bug — hence the `lua_assert(0)`.


---

## 5. Metamethod Dispatch Deep-Dive

### The Metamethod Chain

Lua's metamethod system is a layered dispatch mechanism. The VM always tries the fast path first, then falls back to metamethods.

### Key Functions (from `ltm.c`)

#### luaT_trybinTM (line 150)
Called by OP_MMBIN when a binary arithmetic/bitwise operation fails.

```
luaT_trybinTM(L, p1, p2, res, event)
│
├── Look for metamethod on p1: tm = luaT_gettmbyobj(L, p1, event)
│   └── If p1 is a table/userdata, check its metatable for the event name
│
├── If not found on p1, look on p2: tm = luaT_gettmbyobj(L, p2, event)
│
├── If found: luaT_callTMres(L, tm, p1, p2, res)
│   └── Push tm, p1, p2 on stack, call, copy result to res
│
└── If not found: luaG_opinterror(L, p1, p2, "perform arithmetic on")
```

#### luaT_trybiniTM (line 189)
Called by OP_MMBINI. Converts the immediate integer to a TValue, then calls `luaT_trybinassocTM`.

```c
void luaT_trybiniTM (lua_State *L, const TValue *p1, lua_Integer i2,
                      int flip, StkId res, TMS event) {
    TValue aux;
    setivalue(&aux, i2);  // convert immediate to TValue
    luaT_trybinassocTM(L, p1, &aux, flip, res, event);
}
```

#### luaT_trybinassocTM (line 180)
Handles the `flip` flag for correct operand ordering:

```c
void luaT_trybinassocTM (lua_State *L, const TValue *p1, const TValue *p2,
                          int flip, StkId res, TMS event) {
    if (flip)
        luaT_trybinTM(L, p2, p1, res, event);  // swapped
    else
        luaT_trybinTM(L, p1, p2, res, event);  // normal
}
```

#### luaT_callTMres (line 119)
Calls a metamethod and returns the result's type tag:

```
luaT_callTMres(L, f, p1, p2, res)
│
├── Push f (metamethod function) on stack
├── Push p1 (first operand) on stack
├── Push p2 (second operand) on stack
├── Call with 2 args, 1 result
├── Copy result to res
└── Return type tag of result
```

#### luaT_callorderTM (line 200)
Called for comparison metamethods (__lt, __le):

```c
int luaT_callorderTM (lua_State *L, const TValue *p1, const TValue *p2,
                       TMS event) {
    // Try p1's metamethod first, then p2's
    if (callbinTM(L, p1, p2, L->top.p, event))
        return !tagisfalse(s2v(L->top.p));
    luaG_ordererror(L, p1, p2);  // no metamethod found
}
```

#### luaT_callorderiTM (line 210)
Called by OP_LTI/OP_LEI/OP_GTI/OP_GEI for order comparisons with immediate:

```c
int luaT_callorderiTM (lua_State *L, const TValue *p1, int v2,
                        int inv, int isfloat, TMS event) {
    TValue aux; const TValue *p2;
    if (isfloat) { setfltvalue(&aux, cast_num(v2)); }
    else { setivalue(&aux, v2); }
    p2 = &aux;
    if (inv)  // inverted: GT uses LT with swapped args
        return luaT_callorderTM(L, p2, p1, event);
    else
        return luaT_callorderTM(L, p1, p2, event);
}
```

### luaV_finishget — The __index Chain (line 291)

```
luaV_finishget(L, t, key, val, tag)
│
└── for loop (max MAXTAGLOOP = 2000 iterations):
    │
    ├── If tag == LUA_VNOTABLE (t is not a table):
    │   └── tm = luaT_gettmbyobj(L, t, TM_INDEX)
    │       └── If no metamethod: type error "index"
    │
    ├── If t IS a table:
    │   └── tm = fasttm(L, t->metatable, TM_INDEX)
    │       └── If no metamethod: result is nil, return
    │
    ├── If tm is a function:
    │   └── Call tm(t, key) → result in val, return
    │
    └── If tm is NOT a function (it's a table or other value):
        └── t = tm; retry lookup on tm[key]
            └── luaV_fastget(t, key, ...) → if found, return
            └── else: loop again (chain continues)
```

**⚠️ Key insight**: `__index` can be either a function OR a table. If it's a table, the lookup chains to that table. This chain can go up to 2000 levels deep before erroring.

### luaV_finishset — The __newindex Chain (line 334)

Same pattern as finishget but for assignment. `__newindex` can also be a function or table.

```
luaV_finishset(L, t, key, val, hres)
│
└── for loop (max MAXTAGLOOP):
    │
    ├── If t IS a table:
    │   └── tm = fasttm(L, t->metatable, TM_NEWINDEX)
    │       └── If no metamethod: do the actual set (luaH_finishset)
    │
    ├── If t is NOT a table:
    │   └── tm = luaT_gettmbyobj(L, t, TM_NEWINDEX)
    │       └── If no metamethod: type error "index"
    │
    ├── If tm is a function:
    │   └── Call tm(t, key, val) → return
    │
    └── If tm is NOT a function:
        └── t = tm; retry set on tm[key] = val
```


---

## 6. Fast Path / Slow Path Pattern

This is the single most important architectural pattern in the Lua VM. Understanding it is essential for a correct Go reimplementation.

### The Pattern

```
┌─────────────────────────────────────────────────────────┐
│ FAST PATH (inline in opcode handler)                     │
│                                                          │
│ 1. Check types directly (ttisinteger, ttisfloat, etc.)   │
│ 2. Perform operation directly                            │
│ 3. pc++ to skip metamethod instruction                   │
│ 4. Store result                                          │
│                                                          │
│ Cost: ~3-5 instructions, no function calls               │
├─────────────────────────────────────────────────────────┤
│ SLOW PATH (fall through to next opcode or call function) │
│                                                          │
│ For arithmetic: fall through to OP_MMBIN/MMBINI/MMBINK   │
│ For table ops: call luaV_finishget / luaV_finishset      │
│                                                          │
│ 1. Look up metamethod in metatable                       │
│ 2. If found, call it (full function call overhead)       │
│ 3. If not found, raise error                             │
│                                                          │
│ Cost: ~100-1000x more expensive                          │
└─────────────────────────────────────────────────────────┘
```

### Pattern Variations

#### Arithmetic: Skip-Next-Instruction Pattern

```
OP_ADD R[A], R[B], R[C]     ← fast path tries int/float arithmetic
OP_MMBIN R[A], R[B], TM_ADD ← only reached if fast path fails
```

The fast path does `pc++` to skip OP_MMBIN. If it fails, execution naturally falls to OP_MMBIN.

#### Table Get: Call-If-Empty Pattern

```c
luaV_fastget(rb, key, s2v(ra), luaH_getshortstr, tag);
if (tagisempty(tag))
    Protect(luaV_finishget(L, rb, rc, ra, tag));
```

The fast path returns a tag. If the tag indicates "empty" (key not found in table), the slow path is invoked.

#### Table Set: HOK Pattern

```c
luaV_fastset(s2v(ra), key, rc, hres, luaH_psetshortstr);
if (hres == HOK)
    luaV_finishfastset(L, s2v(ra), rc);  // just GC barrier
else
    Protect(luaV_finishset(L, s2v(ra), rb, rc, hres));
```

The fast path returns `HOK` if the key already exists in the table. If not (new key, or not a table), the slow path handles metamethods and table resizing.

### Go Implementation Strategy

```go
// Pattern for arithmetic opcodes:
case OP_ADD:
    rb := stack[base + B(i)]
    rc := stack[base + C(i)]
    if rb.IsInteger() && rc.IsInteger() {
        pc++  // skip MMBIN
        stack[base + A(i)].SetInteger(rb.IntegerValue() + rc.IntegerValue())
    } else if rb.IsNumber() && rc.IsNumber() {
        pc++  // skip MMBIN
        stack[base + A(i)].SetFloat(rb.NumberValue() + rc.NumberValue())
    }
    // else: DON'T increment pc, let next iteration dispatch MMBIN

// Pattern for table get:
case OP_GETTABLE:
    rb := stack[base + B(i)]
    rc := stack[base + C(i)]
    if tbl, ok := rb.AsTable(); ok {
        if val, tag := tbl.Get(rc); !tagIsEmpty(tag) {
            stack[base + A(i)] = val
            break
        }
    }
    finishGet(L, rb, rc, base + A(i))
```

---

## 7. VM ↔ ldo.c Interaction

### Call Handoff

```
VM (lvm.c)                          ldo.c
────────────                        ──────
OP_CALL:
  savepc(ci)
  newci = luaD_precall(L, ra, nres) ──→ luaD_precall:
                                         │ Check what ra points to
                                         ├─ C function:
                                         │   Create CallInfo
                                         │   Call the C function
                                         │   luaD_poscall() 
                                         │   Return NULL
                                         │
                                         └─ Lua function:
                                             Create CallInfo
                                             Set up ci->u.l.savedpc
                                             Return new CallInfo
  if newci == NULL:
    updatetrap(ci)  ← C call done
  else:
    ci = newci
    goto startfunc  ← Lua call, re-enter loop
```

### Return Handoff

```
VM (lvm.c)                          ldo.c
────────────                        ──────
OP_RETURN:
  L->top.p = ra + n
  luaD_poscall(L, ci, n) ──────────→ luaD_poscall:
                                       │ Move results to caller's expected slots
                                       │ Adjust top
                                       │ L->ci = ci->previous
                                       │ Call return hooks if needed
                                       └─ Return
  goto ret:
    if CIST_FRESH: return  ← back to C caller
    else: ci = previous; goto returning  ← back to Lua caller
```

### Tail Call Handoff

```
VM (lvm.c)                          ldo.c
────────────                        ──────
OP_TAILCALL:
  Close upvalues if needed
  n = luaD_pretailcall(L, ci, ra, b, delta) ──→ luaD_pretailcall:
                                                  │ Move args to current func slot
                                                  │ Reuse CallInfo
                                                  ├─ Lua: return -1
                                                  └─ C: call it, return nresults
  if n < 0: goto startfunc  ← Lua tail call
  else: luaD_poscall; goto ret  ← C tail call
```

### Key Constants

```c
#define CIST_FRESH  (CIST_C << 1)  // CallInfo was created by a C-to-Lua transition
```

`CIST_FRESH` marks the boundary: when `ret:` sees this flag, it returns from `luaV_execute` instead of continuing the trampoline.

```c
#define get_nresults(cs)  (cast_int((cs) & CIST_NRESULTS) - 1)
```

The number of expected results is encoded in the CallInfo's callstatus field.

---

## 8. If I Were Building This in Go

### Dispatch Strategy

**Recommended: `switch` statement**

Go's `switch` on integer types compiles to a jump table (when dense enough). This is equivalent to C's computed goto performance.

```go
func (vm *VM) Execute(L *LuaState, ci *CallInfo) {
    cl := ci.LuaClosure()
    k := cl.Proto.Constants
    code := cl.Proto.Code
    base := ci.Base()
    pc := ci.SavedPC
    
    for {
        i := code[pc]
        pc++
        
        switch OpCode(i & 0x7F) {
        case OP_MOVE:
            // ...
        case OP_LOADI:
            // ...
        // ... all opcodes
        }
    }
}
```

**Alternative: Closure table** (function pointer dispatch)

```go
type OpHandler func(vm *VM, i Instruction)

var handlers [NUM_OPCODES]OpHandler

func init() {
    handlers[OP_MOVE] = (*VM).opMove
    handlers[OP_LOADI] = (*VM).opLoadI
    // ...
}

// In the loop:
handlers[GET_OPCODE(i)](vm, i)
```

**Trade-offs**:
| Approach | Pros | Cons |
|----------|------|------|
| `switch` | Simple, compiler optimizes, inline-friendly | Large function |
| Closure table | Modular, easy to extend | Function call overhead, no inlining |
| Interface dispatch | Most Go-idiomatic | Slowest (interface method call) |

**Recommendation**: Use `switch`. Go's compiler generates efficient jump tables for dense integer switches. The entire VM loop should be one function for maximum inlining of fast paths.

### The Trampoline Problem

C Lua uses `goto startfunc` / `goto returning` to avoid growing the C stack for Lua→Lua calls. In Go:

**Option 1: Loop-based trampoline (recommended)**
```go
for {
    result := vm.executeFrame(L, ci)
    switch result {
    case CALL_LUA:
        ci = L.CI  // new frame was pushed
        continue
    case RETURN_LUA:
        ci = L.CI  // frame was popped
        if ci.IsFresh() { return }
        continue
    case RETURN_C:
        return
    }
}
```

**Option 2: Rely on Go's growable stacks**
Go stacks grow automatically. Recursive calls to `Execute` won't overflow (unless you hit the goroutine stack limit, which is 1GB by default). This is simpler but uses more memory for deep call chains.

### Value Representation

```go
// Option A: Tagged union (most faithful to C)
type Value struct {
    tt  uint8    // type tag
    val uint64   // raw bits (integer, float bits, or pointer)
}

// Option B: Go interface (simpler but slower)
type Value interface{}  // int64, float64, string, *Table, etc.

// Option C: NaN-boxing (advanced, matches C performance)
// Pack type info into NaN float64 bits
```

**Recommendation**: Tagged union (Option A). It matches C's TValue exactly and avoids interface overhead on the hot path.

---

## 9. Edge Cases

### Integer Overflow Handling

All integer arithmetic uses **unsigned wrapping** via the `intop` macro:
```c
#define intop(op,v1,v2) l_castU2S(l_castS2U(v1) op l_castS2U(v2))
```

In Go:
```go
func intop_add(a, b int64) int64 {
    return int64(uint64(a) + uint64(b))
}
```

**Key cases**:
- `MININT + (-1)` wraps to `MAXINT`
- `MAXINT + 1` wraps to `MININT`
- `-MININT` = `MININT` (wraps around)
- `MININT // -1` = `MININT` (uses unsigned negation, not division)
- `MININT % -1` = 0

### Division by Zero

- Integer: `luaV_idiv` and `luaV_mod` raise runtime errors
- Float division: produces `inf` or `-inf` (IEEE 754)
- Float modulus: `luai_nummod` may produce NaN for `0 % 0`
- `savestate(L, ci)` is called BEFORE division operations to ensure correct error reporting

### NaN Comparisons

All comparisons with NaN return false:
- `NaN == NaN` → false
- `NaN < 1` → false
- `NaN > 1` → false
- `NaN <= 1` → false

This is handled naturally by IEEE 754 float comparison in C. In Go, `math.NaN()` comparisons work the same way.

**⚠️ Trap**: `NaN == NaN` is false, but `rawequal(NaN, NaN)` should also be false (C's `==` on NaN returns false). Ensure your equality function doesn't short-circuit on identical pointers for float values.

### String-to-Number Coercion

The `tonumber`/`tointeger` macros handle string coercion:
- `tonumber(o, n)`: tries float first, then string→number
- `tointeger(o, i)`: tries integer first, then string→integer
- `tonumberns(o, n)`: number conversion WITHOUT string coercion
- `tointegerns(o, i)`: integer conversion WITHOUT string coercion

**When string coercion applies**:
- Arithmetic operations: YES (controlled by `LUA_NOCVTS2N` macro)
- Bitwise operations: NO (uses `tointegerns`)
- Comparisons: NO (different types are not equal/ordered)
- Concatenation: numbers are converted to strings (controlled by `LUA_NOCVTN2S`)

### Float-to-Integer Conversion (F2Imod)

```c
typedef enum {
    F2Ieq,    // no rounding; accepts only integral values
    F2Ifloor, // takes the floor
    F2Iceil   // takes the ceiling
} F2Imod;
```

Used in:
- `forlimit`: `F2Ifloor` for positive step, `F2Iceil` for negative step
- `tointeger`: `F2Ieq` by default (configurable via `LUA_FLOORN2I`)
- Mixed comparisons: `F2Iceil` for `i < f`, `F2Ifloor` for `f < i`

### The `l_intfitsf` Check (line 75)

```c
#define l_intfitsf(i)  ((MAXINTFITSF + l_castS2U(i)) <= (2 * MAXINTFITSF))
```

Checks if an integer can be exactly represented as a float. Used in mixed int/float comparisons. If the integer doesn't fit, the comparison converts the float to integer instead (using floor/ceil).

---

## 10. Bug Pattern Guide

### Per-Category Common Mistakes

#### Loading Opcodes
| Bug | Description |
|-----|-------------|
| OP_LOADNIL off-by-one | Sets `B+1` nils, not `B`. The `do { } while (b--)` loop is inclusive. |
| OP_LOADKX missing pc++ | Must consume the EXTRAARG instruction after fetching Ax. |
| OP_LFALSESKIP missing pc++ | Must skip the next instruction (usually LOADTRUE). |

#### Upvalue Opcodes
| Bug | Description |
|-----|-------------|
| Missing GC barrier on SETUPVAL | After writing to an upvalue, must call write barrier. |
| Stale upvalue pointer | After stack reallocation, upvalue `v.p` may point to moved memory. Open upvalues track stack slots, not absolute addresses. |
| Wrong upvalue dereference | `cl->upvals[b]->v.p` is a pointer-to-pointer pattern. Must dereference twice. |

#### Table Opcodes
| Bug | Description |
|-----|-------------|
| OP_SELF ordering | Must save `self` to `ra+1` BEFORE looking up the method in `ra`. |
| OP_NEWTABLE always skips EXTRAARG | Even when k=0, the next instruction slot is EXTRAARG and must be skipped. |
| OP_SETLIST vB=0 count | When vB=0, count comes from `top - ra - 1`, not `top - ra`. |
| Missing GC barrier on table set | After `luaV_finishfastset`, must call `luaC_barrierback`. |
| OP_SETTABUP uses A for upvalue | Unlike other opcodes, the upvalue index is in A, not B. |

#### Arithmetic Opcodes
| Bug | Description |
|-----|-------------|
| Missing pc++ skip | When fast path succeeds, MUST increment pc to skip MMBIN. |
| Signed integer overflow | Must use unsigned wrapping arithmetic (cast to uint64, operate, cast back). |
| Wrong modulus sign | Lua's `%` always has the sign of the divisor, unlike Go's `%`. |
| MININT // -1 | Must use unsigned negation, not `MININT / -1` (which overflows). |
| DIV/POW always float | OP_DIV and OP_POW always produce floats, even for integer operands. |
| Missing savestate before MOD/IDIV | These can raise division-by-zero errors; state must be saved first. |

#### Bitwise Opcodes
| Bug | Description |
|-----|-------------|
| OP_SHLI operand order | `R[A] = sC << R[B]`, NOT `R[B] << sC`. The immediate is the value, register is the amount. |
| OP_SHRI sign | Uses `luaV_shiftl(ib, -ic)`, not `luaV_shiftr`. |
| Shift >= 64 | Must return 0, not undefined behavior. |
| Arithmetic vs logical right shift | Must use unsigned right shift (logical), not signed (arithmetic). |

#### Comparison Opcodes
| Bug | Description |
|-----|-------------|
| docondjump polarity | `if cond != k then pc++ else doNextJump`. Getting the polarity wrong inverts all conditions. |
| GTI/GEI inversion | These use `inv=1` with TM_LT/TM_LE. The metamethod args are swapped. |
| Mixed int/float comparison | Must handle the case where an integer doesn't fit exactly in a float. |
| NaN comparison | All comparisons with NaN must return false. |

#### Call/Return Opcodes
| Bug | Description |
|-----|-------------|
| B=0 argument count | When B=0, args come from top, not from a fixed count. |
| C=0 result count | When C=0, results are variable (LUA_MULTRET). |
| Missing base update after call | After any call that may reallocate the stack, base must be refreshed. |
| CIST_FRESH check | Must check this flag at `ret:` to know whether to return or trampoline. |
| Vararg delta in TAILCALL | The `delta` calculation for vararg functions is tricky. |
| RETURN0/RETURN1 inline poscall | These inline the post-call logic for performance; must match luaD_poscall exactly. |

#### Loop Opcodes
| Bug | Description |
|-----|-------------|
| Counter-based integer for loop | Lua 5.5 uses a counter, not limit comparison. Must use unsigned arithmetic. |
| FORPREP slot rearrangement | After prep, slots are: counter/limit, step, control (NOT init, limit, step). |
| FORLOOP counter as unsigned | The counter at `ra` must be treated as unsigned when checking `> 0`. |
| TFORPREP swap | Control and closing variables are swapped. |
| TFORCALL copies | Must copy func, state, control to ra+3..ra+5 before calling. |
| Missing updatetrap in FORLOOP | Without this, tight loops can't be interrupted by signals/hooks. |

#### Closure/Upvalue Opcodes
| Bug | Description |
|-----|-------------|
| Anchor before filling | Closure must be set in stack BEFORE filling upvalues (GC safety). |
| instack vs inherited | `instack=true` → find/create open upvalue; `instack=false` → copy from parent closure. |
| luaF_findupval linked list | Must maintain the sorted linked list of open upvalues correctly. |

#### Metamethod Opcodes
| Bug | Description |
|-----|-------------|
| MMBIN result destination | Result goes to `RA(*(pc-2))`, the A field of the PREVIOUS instruction. |
| MMBINI/MMBINK flip | The `k` bit indicates operand flip. Must pass correct order to metamethod. |
| luaV_finishOp | After a yield during a metamethod, `luaV_finishOp` must correctly resume. |

---

## Appendix: Complete Opcode Quick Reference

| # | Opcode | Format | Line | Category |
|---|--------|--------|------|----------|
| 0 | OP_MOVE | iABC | 1233 | Loading |
| 1 | OP_LOADI | iAsBx | 1238 | Loading |
| 2 | OP_LOADF | iAsBx | 1244 | Loading |
| 3 | OP_LOADK | iABx | 1250 | Loading |
| 4 | OP_LOADKX | iABx | 1256 | Loading |
| 5 | OP_LOADFALSE | iABC | 1263 | Loading |
| 6 | OP_LFALSESKIP | iABC | 1268 | Loading |
| 7 | OP_LOADTRUE | iABC | 1274 | Loading |
| 8 | OP_LOADNIL | iABC | 1279 | Loading |
| 9 | OP_GETUPVAL | iABC | 1287 | Upvalue |
| 10 | OP_SETUPVAL | iABC | 1293 | Upvalue |
| 11 | OP_GETTABUP | iABC | 1300 | Table |
| 12 | OP_GETTABLE | iABC | 1311 | Table |
| 13 | OP_GETI | iABC | 1325 | Table |
| 14 | OP_GETFIELD | iABC | 1338 | Table |
| 15 | OP_SETTABUP | iABC | 1349 | Table |
| 16 | OP_SETTABLE | iABC | 1362 | Table |
| 17 | OP_SETI | iABC | 1379 | Table |
| 18 | OP_SETFIELD | iABC | 1394 | Table |
| 19 | OP_NEWTABLE | ivABC | 1407 | Table |
| 20 | OP_SELF | iABC | 1428 | Table |
| 21 | OP_ADDI | iABC | 1440 | Arithmetic |
| 22 | OP_ADDK | iABC | 1444 | Arithmetic |
| 23 | OP_SUBK | iABC | 1448 | Arithmetic |
| 24 | OP_MULK | iABC | 1452 | Arithmetic |
| 25 | OP_MODK | iABC | 1456 | Arithmetic |
| 26 | OP_POWK | iABC | 1461 | Arithmetic |
| 27 | OP_DIVK | iABC | 1465 | Arithmetic |
| 28 | OP_IDIVK | iABC | 1469 | Arithmetic |
| 29 | OP_BANDK | iABC | 1474 | Bitwise |
| 30 | OP_BORK | iABC | 1478 | Bitwise |
| 31 | OP_BXORK | iABC | 1482 | Bitwise |
| 32 | OP_SHLI | iABC | 1486 | Bitwise |
| 33 | OP_SHRI | iABC | 1496 | Bitwise |
| 34 | OP_ADD | iABC | 1506 | Arithmetic |
| 35 | OP_SUB | iABC | 1510 | Arithmetic |
| 36 | OP_MUL | iABC | 1514 | Arithmetic |
| 37 | OP_MOD | iABC | 1518 | Arithmetic |
| 38 | OP_POW | iABC | 1523 | Arithmetic |
| 39 | OP_DIV | iABC | 1527 | Arithmetic |
| 40 | OP_IDIV | iABC | 1531 | Arithmetic |
| 41 | OP_BAND | iABC | 1536 | Bitwise |
| 42 | OP_BOR | iABC | 1540 | Bitwise |
| 43 | OP_BXOR | iABC | 1544 | Bitwise |
| 44 | OP_SHL | iABC | 1548 | Bitwise |
| 45 | OP_SHR | iABC | 1552 | Bitwise |
| 46 | OP_MMBIN | iABC | 1556 | Metamethod |
| 47 | OP_MMBINI | iABC | 1566 | Metamethod |
| 48 | OP_MMBINK | iABC | 1576 | Metamethod |
| 49 | OP_UNM | iABC | 1586 | Unary |
| 50 | OP_BNOT | iABC | 1601 | Unary |
| 51 | OP_NOT | iABC | 1612 | Unary |
| 52 | OP_LEN | iABC | 1621 | Unary |
| 53 | OP_CONCAT | iABC | 1626 | String |
| 54 | OP_CLOSE | iABC | 1634 | Upvalue |
| 55 | OP_TBC | iABC | 1640 | Upvalue |
| 56 | OP_JMP | isJ | 1646 | Jump |
| 57 | OP_EQ | iABC | 1650 | Comparison |
| 58 | OP_LT | iABC | 1658 | Comparison |
| 59 | OP_LE | iABC | 1662 | Comparison |
| 60 | OP_EQK | iABC | 1666 | Comparison |
| 61 | OP_EQI | iABC | 1674 | Comparison |
| 62 | OP_LTI | iABC | 1687 | Comparison |
| 63 | OP_LEI | iABC | 1691 | Comparison |
| 64 | OP_GTI | iABC | 1695 | Comparison |
| 65 | OP_GEI | iABC | 1699 | Comparison |
| 66 | OP_TEST | iABC | 1703 | Jump |
| 67 | OP_TESTSET | iABC | 1709 | Jump |
| 68 | OP_CALL | iABC | 1720 | Call |
| 69 | OP_TAILCALL | iABC | 1737 | Call |
| 70 | OP_RETURN | iABC | 1763 | Call |
| 71 | OP_RETURN0 | iABC | 1785 | Call |
| 72 | OP_RETURN1 | iABC | 1802 | Call |
| 73 | OP_FORLOOP | iABx | 1831 | Loop |
| 74 | OP_FORPREP | iAsBx | 1849 | Loop |
| 75 | OP_TFORPREP | iABx | 1856 | Loop |
| 76 | OP_TFORCALL | iABC | 1875 | Loop |
| 77 | OP_TFORLOOP | iABx | 1894 | Loop |
| 78 | OP_SETLIST | ivABC | 1901 | Table |
| 79 | OP_CLOSURE | iABx | 1929 | Closure |
| 80 | OP_VARARG | iABC | 1936 | Vararg |
| 81 | OP_GETVARG | iABC | 1943 | Vararg (5.5) |
| 82 | OP_ERRNNIL | iABx | 1949 | Error (5.5) |
| 83 | OP_VARARGPREP | iABC | 1955 | Vararg |
| 84 | OP_EXTRAARG | iAx | 1964 | Meta |

---

## Appendix: luaV_finishOp — Resuming After Yield (line 855)

When a Lua function yields (via coroutine), the VM must be able to resume the interrupted instruction. `luaV_finishOp` handles this:

```c
void luaV_finishOp (lua_State *L) {
    CallInfo *ci = L->ci;
    StkId base = ci->func.p + 1;
    Instruction inst = *(ci->u.l.savedpc - 1);  /* interrupted instruction */
    OpCode op = GET_OPCODE(inst);
    switch (op) {
        case OP_MMBIN: case OP_MMBINI: case OP_MMBINK:
            // Result is at top of stack; move to correct register
            setobjs2s(L, base + GETARG_A(*(ci->u.l.savedpc - 2)), --L->top.p);
            break;
        case OP_UNM: case OP_BNOT: case OP_LEN:
        case OP_GETTABUP: case OP_GETTABLE: case OP_GETI:
        case OP_GETFIELD: case OP_SELF:
            setobjs2s(L, base + GETARG_A(inst), --L->top.p);
            break;
        case OP_LT: case OP_LE: case OP_LTI: case OP_LEI:
        case OP_GTI: case OP_GEI: case OP_EQ:
            // Comparison result: adjust conditional jump
            int res = !l_isfalse(s2v(L->top.p - 1));
            L->top.p--;
            if (res != GETARG_k(inst))
                ci->u.l.savedpc++;  /* skip jump */
            break;
        case OP_CONCAT:
            // Continue concatenation
            // ...
        case OP_CLOSE:
            ci->u.l.savedpc--;  /* repeat to close remaining vars */
            break;
        case OP_RETURN:
            ci->u.l.savedpc--;  /* repeat to complete return */
            break;
        // OP_TFORCALL, OP_CALL, OP_TAILCALL, OP_SETTABUP,
        // OP_SETTABLE, OP_SETI, OP_SETFIELD: no special handling
    }
}
```

**Note for Go**: If implementing coroutines, `finishOp` is essential. The key insight is that metamethod calls can yield, and when resumed, the VM needs to know which instruction was interrupted and where to put the result.

