# Load & Upvalue Opcodes — C ↔ Go Mapping

> Covers: OP_MOVE, OP_LOADI, OP_LOADF, OP_LOADK, OP_LOADKX, OP_LOADFALSE,
> OP_LFALSESKIP, OP_LOADTRUE, OP_LOADNIL, OP_GETUPVAL, OP_SETUPVAL
>
> Source: `lua-master/lvm.c` (C), `internal/vm/api/vm.go` (Go)

---

## OP_MOVE (C:1233 → Go:1518)

**Encoding**: iABC — `R[A] := R[B]`

### C Implementation
```c
// lvm.c:1233
vmcase(OP_MOVE) {
  StkId ra = RA(i);
  setobjs2s(L, ra, RB(i));
  vmbreak;
}
```

### Why Each Line
- `RA(i)`: Decode A field → stack slot pointer (destination register).
- `RB(i)`: Decode B field → stack slot pointer (source register).
- `setobjs2s`: Copy the TValue from source to destination. The `s2s` suffix means
  stack-to-stack copy (both are stack slots, so no GC barrier needed).

### Go Implementation
```go
// vm.go:1518
case opcodeapi.OP_MOVE:
    rb := base + opcodeapi.GetArgB(inst)
    L.Stack[ra].Val = L.Stack[rb].Val
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Copy mechanism | `setobjs2s` macro (handles GC tags) | Direct `TValue` struct assignment |
| GC barrier | Not needed (stack-to-stack) | Not needed (GC traces stack directly) |
| Register decode | `RA(i)` returns `StkId` pointer | `ra` is pre-computed `int` index |

---

## OP_LOADI (C:1238 → Go:1522)

**Encoding**: iAsBx — `R[A] := sBx` (signed integer immediate)

### C Implementation
```c
// lvm.c:1238
vmcase(OP_LOADI) {
  StkId ra = RA(i);
  lua_Integer b = GETARG_sBx(i);
  setivalue(s2v(ra), b);
  vmbreak;
}
```

### Why Each Line
- `GETARG_sBx(i)`: Extract signed Bx field. Range: −2^24+1 to +2^24−1 (offset-encoded).
- `setivalue(s2v(ra), b)`: Set the stack slot's value to integer `b` with tag `LUA_VNUMINT`.
- WHY OP_LOADI exists: Avoids a constant-table lookup for small integers. The compiler
  emits LOADI when the integer fits in sBx; otherwise falls back to LOADK.

### Go Implementation
```go
// vm.go:1522
case opcodeapi.OP_LOADI:
    L.Stack[ra].Val = objectapi.MakeInteger(int64(opcodeapi.GetArgSBx(inst)))
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Value creation | `setivalue` sets tag + value in-place | `MakeInteger` returns a `TValue` struct |
| Range | sBx = 25-bit signed (±16M) | Same encoding, cast to `int64` |

---

## OP_LOADF (C:1244 → Go:1525)

**Encoding**: iAsBx — `R[A] := (float)sBx`

### C Implementation
```c
// lvm.c:1244
vmcase(OP_LOADF) {
  StkId ra = RA(i);
  int b = GETARG_sBx(i);
  setfltvalue(s2v(ra), cast_num(b));
  vmbreak;
}
```

### Why Each Line
- `GETARG_sBx(i)`: Same signed Bx extraction as LOADI.
- `cast_num(b)`: Cast integer to `lua_Number` (double). This is the ONLY difference from LOADI.
- WHY OP_LOADF exists: For float literals that happen to be integers (e.g., `3.0`).
  The compiler emits LOADF instead of LOADK to avoid a constant-table entry.

### Go Implementation
```go
// vm.go:1525
case opcodeapi.OP_LOADF:
    L.Stack[ra].Val = objectapi.MakeFloat(float64(opcodeapi.GetArgSBx(inst)))
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Conversion | `cast_num()` macro → double | `float64()` cast |
| Tag | `LUA_VNUMFLT` via `setfltvalue` | `TagFloat` via `MakeFloat` |

---

## OP_LOADK (C:1250 → Go:1528)

**Encoding**: iABx — `R[A] := K[Bx]`

### C Implementation
```c
// lvm.c:1250
vmcase(OP_LOADK) {
  StkId ra = RA(i);
  TValue *rb = k + GETARG_Bx(i);
  setobj2s(L, ra, rb);
  vmbreak;
}
```

### Why Each Line
- `k + GETARG_Bx(i)`: Index into the function's constant table (`k` is `Proto->k`).
  Bx is unsigned 17-bit, supporting up to 131071 constants.
- `setobj2s`: Copy constant to stack. The `2s` suffix = "to stack" (no barrier needed
  because constants are older than the stack in GC terms).

### Go Implementation
```go
// vm.go:1528
case opcodeapi.OP_LOADK:
    L.Stack[ra].Val = k[opcodeapi.GetArgBx(inst)]
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Constant access | Pointer arithmetic `k + Bx` | Slice index `k[Bx]` |
| Copy | `setobj2s` macro | Direct struct assignment |

---

## OP_LOADKX (C:1256 → Go:1531)

**Encoding**: iABx + EXTRAARG — `R[A] := K[extra arg]`

### C Implementation
```c
// lvm.c:1256
vmcase(OP_LOADKX) {
  StkId ra = RA(i);
  TValue *rb;
  rb = k + GETARG_Ax(*pc); pc++;
  setobj2s(L, ra, rb);
  vmbreak;
}
```

### Why Each Line
- `GETARG_Ax(*pc)`: Read the NEXT instruction (OP_EXTRAARG) and extract its Ax field.
  Ax is 25-bit unsigned, supporting up to 33 million constants.
- `pc++`: Skip past the EXTRAARG instruction.
- WHY LOADKX exists: When a function has >131071 constants, Bx (17-bit) isn't enough.
  LOADKX uses a two-instruction sequence: LOADKX + EXTRAARG.

### Go Implementation
```go
// vm.go:1531
case opcodeapi.OP_LOADKX:
    ax := opcodeapi.GetArgAx(code[ci.SavedPC])
    ci.SavedPC++
    L.Stack[ra].Val = k[ax]
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| PC model | `*pc` dereference then `pc++` | `code[ci.SavedPC]` then `ci.SavedPC++` |
| PC storage | Local `pc` variable (register-optimized) | `ci.SavedPC` field (always in struct) |

---

## OP_LOADFALSE (C:1263 → Go:1536)

**Encoding**: iABC — `R[A] := false`

### C Implementation
```c
// lvm.c:1263
vmcase(OP_LOADFALSE) {
  StkId ra = RA(i);
  setbfvalue(s2v(ra));
  vmbreak;
}
```

### Why Each Line
- `setbfvalue`: Set to boolean false (`LUA_VFALSE` tag, no value payload needed).
  The `bf` = "boolean false". Lua 5.5 uses separate tags for true/false (no payload).

### Go Implementation
```go
// vm.go:1536
case opcodeapi.OP_LOADFALSE:
    L.Stack[ra].Val = objectapi.False
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Representation | `setbfvalue` sets tag in-place | Pre-allocated `objectapi.False` singleton |

---

## OP_LFALSESKIP (C:1268 → Go:1539)

**Encoding**: iABC — `R[A] := false; pc++`

### C Implementation
```c
// lvm.c:1268
vmcase(OP_LFALSESKIP) {
  StkId ra = RA(i);
  setbfvalue(s2v(ra));
  pc++;  /* skip next instruction */
  vmbreak;
}
```

### Why Each Line
- Same as LOADFALSE, plus `pc++` to skip the next instruction.
- WHY this exists: Used in `and`/`or` compilation. Pattern:
  `TEST → JMP → LFALSESKIP → LOADTRUE`. If the test fails, LFALSESKIP loads false
  AND skips the LOADTRUE that follows. This saves one JMP instruction vs the
  alternative `LOADFALSE → JMP` sequence.

### Go Implementation
```go
// vm.go:1539
case opcodeapi.OP_LFALSESKIP:
    L.Stack[ra].Val = objectapi.False
    ci.SavedPC++ // skip next instruction
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| PC advance | `pc++` (local var) | `ci.SavedPC++` (struct field) |

---

## OP_LOADTRUE (C:1274 → Go:1543)

**Encoding**: iABC — `R[A] := true`

### C Implementation
```c
// lvm.c:1274
vmcase(OP_LOADTRUE) {
  StkId ra = RA(i);
  setbtvalue(s2v(ra));
  vmbreak;
}
```

### Why Each Line
- `setbtvalue`: Set to boolean true (`LUA_VTRUE` tag). Mirror of `setbfvalue`.

### Go Implementation
```go
// vm.go:1543
case opcodeapi.OP_LOADTRUE:
    L.Stack[ra].Val = objectapi.True
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Representation | `setbtvalue` tag-only | `objectapi.True` singleton |

---

## OP_LOADNIL (C:1279 → Go:1546)

**Encoding**: iABC — `R[A], R[A+1], ..., R[A+B] := nil`

### C Implementation
```c
// lvm.c:1279
vmcase(OP_LOADNIL) {
  StkId ra = RA(i);
  int b = GETARG_B(i);
  do {
    setnilvalue(s2v(ra++));
  } while (b--);
  vmbreak;
}
```

### Why Each Line
- `GETARG_B(i)`: Number of EXTRA registers to nil (B=0 means nil 1 register).
- `do { ... } while (b--)`: Post-decrement loop nils B+1 registers total (A through A+B).
  Uses do-while because we always nil at least one register (ra itself).
- WHY B+1: The compiler optimizes `local a, b, c` into a single LOADNIL with B=2
  (nil 3 registers starting at A).

### Go Implementation
```go
// vm.go:1546
case opcodeapi.OP_LOADNIL:
    b := opcodeapi.GetArgB(inst)
    for i := 0; i <= b; i++ {
        L.Stack[ra+i].Val = objectapi.Nil
    }
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Loop style | `do-while` with pointer increment `ra++` | `for` loop with index arithmetic `ra+i` |
| Nil value | `setnilvalue` sets tag in-place | `objectapi.Nil` singleton assignment |
| Count | B+1 via post-decrement | B+1 via `i <= b` |

---

## OP_GETUPVAL (C:1287 → Go:1554)

**Encoding**: iABC — `R[A] := UpValue[B]`

### C Implementation
```c
// lvm.c:1287
vmcase(OP_GETUPVAL) {
  StkId ra = RA(i);
  int b = GETARG_B(i);
  setobj2s(L, ra, cl->upvals[b]->v.p);
  vmbreak;
}
```

### Why Each Line
- `cl->upvals[b]`: Access the b-th upvalue of the current closure.
- `->v.p`: The `v` union in `UpVal` contains a pointer `p` to the actual value.
  When the upvalue is "open" (variable still on stack), `p` points into the stack.
  When "closed" (variable went out of scope), `p` points to `UpVal.u.value` (internal copy).
- WHY this indirection: Upvalues can be shared between closures. The pointer lets
  multiple closures see the same variable. When the variable's scope ends,
  `luaF_closeupval` copies the value and redirects `p` — all sharing closures
  automatically see the closed copy.

### Go Implementation
```go
// vm.go:1554
case opcodeapi.OP_GETUPVAL:
    b := opcodeapi.GetArgB(inst)
    L.Stack[ra].Val = cl.UpVals[b].Get(L.Stack)
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Access | Direct pointer deref `->v.p` | Method call `UpVal.Get(L.Stack)` |
| Open/closed | Pointer redirection (same code path) | `Get()` checks `Level >= 0` for open vs closed |
| GC barrier | Not needed (stack destination) | Not needed (same reasoning) |

---

## OP_SETUPVAL (C:1293 → Go:1558)

**Encoding**: iABC — `UpValue[B] := R[A]`

### C Implementation
```c
// lvm.c:1293
vmcase(OP_SETUPVAL) {
  StkId ra = RA(i);
  UpVal *uv = cl->upvals[GETARG_B(i)];
  setobj(L, uv->v.p, s2v(ra));
  luaC_barrier(L, uv, s2v(ra));
  vmbreak;
}
```

### Why Each Line
- `setobj(L, uv->v.p, s2v(ra))`: Copy stack value into the upvalue's target location.
- `luaC_barrier(L, uv, s2v(ra))`: **GC write barrier**. When we store a white (new) object
  into a black (already-scanned) upvalue, we must notify the GC. Without this barrier,
  the GC could miss the new reference and collect a live object.
- WHY barrier here but not in GETUPVAL: GETUPVAL copies TO the stack (which the GC
  always scans). SETUPVAL copies FROM the stack into a heap object (the UpVal), which
  the GC may have already scanned — hence the barrier.

### Go Implementation
```go
// vm.go:1558
case opcodeapi.OP_SETUPVAL:
    b := opcodeapi.GetArgB(inst)
    cl.UpVals[b].Set(L.Stack, L.Stack[ra].Val)
```

### Differences
| Aspect | C Lua | go-lua |
|--------|-------|--------|
| Write | Direct pointer write `uv->v.p` | Method call `UpVal.Set()` |
| GC barrier | Explicit `luaC_barrier` macro | Handled by Go's GC (no manual barrier needed) |
| Barrier cost | Conditional check + possible GC work | Zero (Go GC is concurrent + precise) |

---

## Summary Table

| Opcode | Format | C Line | Go Line | Semantics |
|--------|--------|--------|---------|-----------|
| OP_MOVE | iABC | 1233 | 1518 | R[A] := R[B] |
| OP_LOADI | iAsBx | 1238 | 1522 | R[A] := sBx (integer) |
| OP_LOADF | iAsBx | 1244 | 1525 | R[A] := (float)sBx |
| OP_LOADK | iABx | 1250 | 1528 | R[A] := K[Bx] |
| OP_LOADKX | iABx+Ax | 1256 | 1531 | R[A] := K[extra] (>131K constants) |
| OP_LOADFALSE | iABC | 1263 | 1536 | R[A] := false |
| OP_LFALSESKIP | iABC | 1268 | 1539 | R[A] := false; pc++ |
| OP_LOADTRUE | iABC | 1274 | 1543 | R[A] := true |
| OP_LOADNIL | iABC | 1279 | 1546 | R[A..A+B] := nil |
| OP_GETUPVAL | iABC | 1287 | 1554 | R[A] := UpValue[B] |
| OP_SETUPVAL | iABC | 1293 | 1558 | UpValue[B] := R[A] |

## Key Architectural Differences

1. **GC Barriers**: C Lua requires explicit `luaC_barrier` calls for heap writes.
   Go-lua relies on Go's concurrent GC — no manual barriers needed.

2. **PC Model**: C uses a local `pc` variable (optimized into a CPU register by the compiler).
   Go uses `ci.SavedPC` (a struct field), which means every PC read/write goes through memory.

3. **Value Representation**: C uses `setobj`/`setivalue`/`setfltvalue` macros that set
   tag + value in-place. Go uses `MakeInteger`/`MakeFloat` constructors that return
   `TValue` structs, or pre-allocated singletons (`objectapi.Nil`, `True`, `False`).

4. **Register Decode**: C computes `ra` per-opcode via `RA(i)` macro (returns pointer).
   Go pre-computes `ra` as an integer index before the switch statement.
