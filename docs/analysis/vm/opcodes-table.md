# VM Table, Length & Concat Opcodes

> C: `lua-master/lvm.c` | Go: `internal/vm/vm.go`
> Covers: GETTABUP, GETTABLE, GETI, GETFIELD, SETTABUP, SETTABLE, SETI, SETFIELD,
> NEWTABLE, SELF, SETLIST, LEN, CONCAT

---

## Key Concepts

### Fast Path vs Slow Path

Every table GET/SET follows a two-tier pattern:
1. **Fast path:** Object is a table → raw hash lookup. Key exists & value not nil → return. No metamethods.
2. **Slow path:** Not a table, or key absent → `luaV_finishget`/`luaV_finishset` walks `__index`/`__newindex` chain.

### C Fast-Path Macros (lvm.h:84–100)

```c
#define luaV_fastget(t,k,res,f,tag) \
    (tag = (!ttistable(t) ? LUA_VNOTABLE : f(hvalue(t), k, res)))
#define luaV_fastgeti(t,k,res,tag) \
    if (!ttistable(t)) tag = LUA_VNOTABLE; \
    else { luaH_fastgeti(hvalue(t), k, res, tag); }
#define luaV_fastset(t,k,val,hres,f) \
    (hres = (!ttistable(t) ? HNOTATABLE : f(hvalue(t), k, val)))
#define luaV_fastseti(t,k,val,hres) \
    if (!ttistable(t)) hres = HNOTATABLE; \
    else { luaH_fastseti(hvalue(t), k, val, hres); }
```

### Go Equivalent

Go inlines the check per opcode (no macros):
```go
if rb.IsTable() {
    val, found := rb.Val.(*tableapi.Table).Get(rc)  // or GetInt(c)
    if found && !val.IsNil() { L.Stack[ra].Val = val }
    else { FinishGet(L, rb, rc, ra) }
} else { FinishGet(L, rb, rc, ra) }
```

### The Tag Loop

Both `luaV_finishget` (C) and `FinishGet` (Go, vm.go:1025) loop up to `maxTagLoop` times.
If `__index` is a table → repeat lookup. If `__index` is a function → `callTMRes`. Limit prevents loops.

---

## OP_GETTABUP (C:1300 → Go:1572)

```c
// lvm.c:1300 — Global variable access: _ENV.name
vmcase(OP_GETTABUP) {
    TValue *upval = cl->upvals[GETARG_B(i)]->v.p;  // environment table
    TValue *rc = KC(i);                              // constant string key
    TString *key = tsvalue(rc);
    luaV_fastget(upval, key, s2v(ra), luaH_getshortstr, tag);
    if (tagisempty(tag))
        Protect(luaV_finishget(L, upval, rc, ra, tag));
}
```
```go
// vm.go:1572
upval := cl.UpVals[b].Get(L.Stack)
rc := k[opcodeapi.GetArgC(inst)]
// ... fast path: h.Get(rc), slow path: FinishGet(L, upval, rc, ra)
```

**Why:** Primary opcode for global access. `_ENV` is upvalue[0] of every closure.

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Fast lookup | `luaH_getshortstr` (short-string hash) | `h.Get(rc)` (generic) |
| Upvalue deref | `cl->upvals[B]->v.p` (pointer chain) | `cl.UpVals[b].Get(L.Stack)` |
| Tag system | `lu_byte tag` + `tagisempty` | `(val, found)` bool pair |

---

## OP_GETTABLE (C:1311 → Go:1588)

```c
// lvm.c:1311 — R[A] = R[B][R[C]]
vmcase(OP_GETTABLE) {
    TValue *rb = vRB(i);  TValue *rc = vRC(i);
    if (ttisinteger(rc))                    // integer key → array fast path
        luaV_fastgeti(rb, ivalue(rc), s2v(ra), tag);
    else
        luaV_fastget(rb, rc, s2v(ra), luaH_get, tag);
    if (tagisempty(tag))
        Protect(luaV_finishget(L, rb, rc, ra, tag));
}
```
```go
// vm.go:1588 — Uses h.Get(rc) for all key types
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Int key opt | Explicit `ttisinteger` → `luaV_fastgeti` | Handled inside `h.Get()` |

---

## OP_GETI (C:1325 → Go:1603)

```c
// lvm.c:1325 — R[A] = R[B][C] where C is immediate integer
vmcase(OP_GETI) {
    TValue *rb = vRB(i);
    int c = GETARG_C(i);
    luaV_fastgeti(rb, c, s2v(ra), tag);
    if (tagisempty(tag)) {
        TValue key; setivalue(&key, c);     // create TValue only on slow path
        Protect(luaV_finishget(L, rb, &key, ra, tag));
    }
}
```
```go
// vm.go:1603 — Uses h.GetInt(c), creates MakeInteger(c) on slow path
```

**Why:** Optimized for `t[1]`, `t[2]`. Integer key encoded directly in instruction.

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Array lookup | `luaH_fastgeti` (inlined) | `h.GetInt(c)` |
| Key creation | Lazy — only on slow path | Also lazy (slow path only) |

---

## OP_GETFIELD (C:1338 → Go:1618)

```c
// lvm.c:1338 — R[A] = R[B].K[C] where K[C] is constant short string
vmcase(OP_GETFIELD) {
    TValue *rb = vRB(i);  TValue *rc = KC(i);
    TString *key = tsvalue(rc);
    luaV_fastget(rb, key, s2v(ra), luaH_getshortstr, tag);
    if (tagisempty(tag))
        Protect(luaV_finishget(L, rb, rc, ra, tag));
}
```
```go
// vm.go:1618 — Uses h.Get(rc) with rc from k[]
```

**Why:** Optimized for `t.field`. Key is constant string with pre-computed hash.

---

## OP_SETTABUP (C:1349 → Go:1633)

```c
// lvm.c:1349 — Sets global: UpValue[A][K[B]] = RKC
vmcase(OP_SETTABUP) {
    TValue *upval = cl->upvals[GETARG_A(i)]->v.p;  // NOTE: A = upvalue index
    TValue *rb = KB(i);                              // constant string key
    TValue *rc = RKC(i);                             // value (register or constant)
    luaV_fastset(upval, tsvalue(rb), rc, hres, luaH_psetshortstr);
    if (hres == HOK) luaV_finishfastset(L, upval, rc);  // GC barrier
    else Protect(luaV_finishset(L, upval, rb, rc, hres));
}
```
```go
// vm.go:1633
upval := cl.UpVals[opcodeapi.GetArgA(inst)].Get(L.Stack)
rb := k[b]                           // key
// rc from k[] or register based on k-bit
if upval.IsTable() { tableSetWithMeta(L, upval, rb, rc) }
else { FinishSet(L, upval, rb, rc) }
```

**`RKC` macro:** If k-bit set → value from K[], else from register. Go decodes explicitly.
**`luaV_finishfastset`:** Just a GC write barrier. Go handles this inside `table.Set`.

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Fast set | `luaH_psetshortstr` + `luaV_finishfastset` | `tableSetWithMeta` (vm.go:1069) |
| GC barrier | Explicit per-set | Inside table implementation |

---

## OP_SETTABLE / OP_SETI / OP_SETFIELD (C:1362,1379,1394 → Go:1650,1665,1680)

All three follow the same C pattern: fast set → HOK check → GC barrier or finishset.

```c
// lvm.c:1362 — R[A][R[B]] = RKC (generic key from register)
vmcase(OP_SETTABLE) {
    TValue *rb = vRB(i);  TValue *rc = RKC(i);
    if (ttisinteger(rb)) luaV_fastseti(s2v(ra), ivalue(rb), rc, hres);
    else luaV_fastset(s2v(ra), rb, rc, hres, luaH_pset);
    if (hres == HOK) luaV_finishfastset(L, s2v(ra), rc);
    else Protect(luaV_finishset(L, s2v(ra), rb, rc, hres));
}

// lvm.c:1379 — R[A][B] = RKC (immediate integer key)
vmcase(OP_SETI) {
    int b = GETARG_B(i);  TValue *rc = RKC(i);
    luaV_fastseti(s2v(ra), b, rc, hres);
    // ... same HOK/finishset pattern, creates TValue key on slow path
}

// lvm.c:1394 — R[A][K[B]] = RKC (constant string key)
vmcase(OP_SETFIELD) {
    TValue *rb = KB(i);  TValue *rc = RKC(i);
    luaV_fastset(s2v(ra), tsvalue(rb), rc, hres, luaH_psetshortstr);
    // ... same HOK/finishset pattern
}
```

Go uses `tableSetWithMeta` (vm.go:1069) for all three, which checks `__newindex`:
```go
// vm.go:1650 (SETTABLE), 1665 (SETI), 1680 (SETFIELD)
tval := L.Stack[ra].Val
if tval.IsTable() { tableSetWithMeta(L, tval, key, rc) }
else { FinishSet(L, tval, key, rc) }
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| SETTABLE int opt | `ttisinteger(rb)` → `luaV_fastseti` | Inside `tableSetWithMeta` |
| SETI key | `int b` from instruction | `int64(GetArgB)` → `MakeInteger(b)` |
| SETFIELD key | `KB(i)` → `tsvalue` | `k[GetArgB]` |

---

## OP_NEWTABLE (C:1407 → Go:1695)

```c
// lvm.c:1407
vmcase(OP_NEWTABLE) {
    unsigned b = cast_uint(GETARG_vB(i));   // log2(hash size) + 1
    unsigned c = cast_uint(GETARG_vC(i));   // array size
    if (b > 0) b = 1u << (b - 1);          // decode hash size
    if (TESTARG_k(i))                       // EXTRAARG for large arrays
        c += cast_uint(GETARG_Ax(*pc)) * (MAXARG_vC + 1);
    pc++;                                    // always skip extra arg
    t = luaH_new(L);
    if (b != 0 || c != 0) luaH_resize(L, t, c, b);
}
```
```go
// vm.go:1695
b := opcodeapi.GetArgVB(inst);  c := opcodeapi.GetArgVC(inst)
if b > 0 { b = 1 << (b - 1) }
if opcodeapi.GetArgK(inst) != 0 {
    c += opcodeapi.GetArgAx(code[ci.SavedPC]) * (opcodeapi.MaxArgVC + 1)
}
ci.SavedPC++
t := tableapi.New(c, b)
```

**Why always `pc++`?** NEWTABLE always followed by EXTRAARG (even if unused).

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Allocation | `luaH_new` + `luaH_resize` (two steps) | `tableapi.New(c, b)` (one step) |
| GC | `checkGC` + top protection | Adds to `GCTotalBytes` |

---

## OP_SELF (C:1428 → Go:1709)

```c
// lvm.c:1428 — R[A+1] = R[B]; R[A] = R[B][K[C]]
vmcase(OP_SELF) {
    TValue *rb = vRB(i);  TValue *rc = KC(i);
    setobj2s(L, ra + 1, rb);                    // save table as self
    luaV_fastget(rb, tsvalue(rc), s2v(ra), luaH_getshortstr, tag);
    if (tagisempty(tag))
        Protect(luaV_finishget(L, rb, rc, ra, tag));
}
```
```go
// vm.go:1709
L.Stack[ra+1].Val = rb   // save table as self
// ... then standard GETFIELD-like lookup into R[A]
```

**Why:** Implements `obj:method()`. Saves table in R[A+1] (becomes `self` argument),
looks up method in R[A]. The following CALL uses R[A] as function, R[A+1] as first arg.

---

## OP_SETLIST (C:1901 → Go:2467)

```c
// lvm.c:1901 — Batch-set array elements for table constructors
vmcase(OP_SETLIST) {
    unsigned n = cast_uint(GETARG_vB(i));        // element count
    unsigned last = cast_uint(GETARG_vC(i));     // base offset
    Table *h = hvalue(s2v(ra));
    if (n == 0)
        n = cast_uint(L->top.p - ra) - 1;       // B==0: use all up to top
    last += n;
    if (TESTARG_k(i)) {                          // EXTRAARG for large offsets
        last += cast_uint(GETARG_Ax(*pc)) * (MAXARG_vC + 1);
        pc++;
    }
    if (last > h->asize) luaH_resizearray(L, h, last);
    for (; n > 0; n--) {
        obj2arr(h, last - 1, s2v(ra + n));       // copy backward
        last--;
        luaC_barrierback(L, obj2gco(h), val);
    }
}
```
```go
// vm.go:2467
n := opcodeapi.GetArgVB(inst);  last := opcodeapi.GetArgVC(inst)
h := L.Stack[ra].Val.Val.(*tableapi.Table)
if n == 0 { n = L.Top - ra - 1 }
last += n
// ... EXTRAARG handling ...
for i := n; i > 0; i-- {
    h.SetInt(int64(last), L.Stack[ra+i].Val)
    last--
}
```

**B==0:** Last expression is function call or vararg — count unknown at compile time.
**EXTRAARG:** vC field (8 bits) can't hold offsets >255; high bits in next instruction.

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Array resize | Explicit `luaH_resizearray` | Auto-grow inside `SetInt` |
| GC barrier | Per-element `luaC_barrierback` | Not needed (tracing GC) |
| Top restore | Before loop if n≠0 | After loop: `L.Top = ci.Top` |

---

## OP_LEN (C:1621 → Go:2120)

```c
// lvm.c:1621
vmcase(OP_LEN) { Protect(luaV_objlen(L, ra, vRB(i))); }
```
```go
// vm.go:2120
ObjLen(L, ra, rb)   // vm.go:985
```

`luaV_objlen`/`ObjLen`: Table → `__len` metamethod first, else raw length.
String → byte length. Other types → require `__len` or error.

---

## OP_CONCAT (C:1626 → Go:2124)

```c
// lvm.c:1626
vmcase(OP_CONCAT) {
    int n = GETARG_B(i);
    L->top.p = ra + n;                   // mark end of operands
    ProtectNT(luaV_concat(L, n));        // NT = no top save (only saves PC)
    checkGC(L, L->top.p);
}
```
```go
// vm.go:2124
L.Top = ra + n
Concat(L, n)                             // vm.go:927
L.Stack[ra].Val = L.Stack[L.Top-1].Val   // copy result to R[A]
L.Top = ci.Top
```

| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Result | `luaV_concat` leaves at top-1 (= ra) | Explicit copy `Stack[Top-1] → Stack[ra]` |
| Top | `ProtectNT` + `checkGC` | Manual `L.Top = ci.Top` |

---

## Metamethod Dispatch Summary

### GET: `__index` chain (FinishGet, vm.go:1025)
Fast path succeeds → no metamethod. On miss: if `__index` is function → `callTMRes(tm, t, key)`;
if `__index` is table → repeat lookup. Loop limit: `maxTagLoop` (C: MAXTAGLOOP=2000).

### SET: `__newindex` chain (FinishSet, vm.go:1096)
Key exists → overwrite (no metamethod). Key absent: if `__newindex` is function → `callTM(tm, t, key, val)`;
if `__newindex` is table → repeat. Same loop limit.

**Key difference:** GET checks metamethod on key **absent/nil**; SET checks on key **absent**.

| Opcode | C Fast Path | Go Fast Path | Metamethod |
|--------|-------------|--------------|------------|
| GETTABUP | `luaH_getshortstr` | `h.Get(rc)` | `__index` |
| GETTABLE | `luaH_get`/`fastgeti` | `h.Get(rc)` | `__index` |
| GETI | `luaH_fastgeti` | `h.GetInt(c)` | `__index` |
| GETFIELD | `luaH_getshortstr` | `h.Get(rc)` | `__index` |
| SETTABUP | `luaH_psetshortstr` | `tableSetWithMeta` | `__newindex` |
| SETTABLE | `luaH_pset`/`fastseti` | `tableSetWithMeta` | `__newindex` |
| SETI | `luaH_fastseti` | `tableSetWithMeta` | `__newindex` |
| SETFIELD | `luaH_psetshortstr` | `tableSetWithMeta` | `__newindex` |
| SELF | `luaH_getshortstr` | `h.Get(rc)` | `__index` |
| LEN | raw length | `h.RawLen()` | `__len` |
| CONCAT | string concat | `Concat` | `__concat` |
| NEWTABLE | — | — | none |
| SETLIST | raw array set | `h.SetInt` | none |
