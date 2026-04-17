# Deep Analysis: Yield-in-Metamethods for go-lua

> **Date**: 2025-07-14
> **Analyst**: analyze-yield-in-metamethods
> **Status**: Complete
> **Supersedes**: `docs/TODO-yield-in-metamethods.md` (which overestimated complexity at 500-1000 lines)

---

## Executive Summary

go-lua does **NOT** use goroutines for coroutines. It uses **panic/recover** — the
exact same mechanism as C Lua's setjmp/longjmp. When a metamethod yields,
`panic(LuaYield{})` destroys intermediate Go call frames (callTMRes, tryBinTM,
callOrderTM). The code that would place the metamethod result into the correct
register **never executes** on resume.

The fix is to expand `FinishOp()` (currently 15 lines, handles only OP_CLOSE and
OP_RETURN) to handle all metamethod opcodes (~60-80 lines), and change `unroll()`
to call `FinishOp` unconditionally for Lua CIs (not just when CISTClsRet is set).

**Estimated change**: ~100-150 lines across 2 files. No new files. No architectural changes.

---

## 1. Critical Architecture Insight: panic/recover, NOT goroutines

The task description assumed go-lua uses goroutines for coroutines. **This is wrong.**

### Evidence

**Yield** (`internal/vm/api/do.go:1138-1155`):
```go
func Yield(L *stateapi.LuaState, nResults int) {
    // ...
    L.Status = stateapi.StatusYield
    ci := L.CI
    ci.NYield = nResults
    if !ci.IsLua() {
        panic(stateapi.LuaYield{NResults: nResults})  // ← panic, not channel
    }
}
```

**Resume** (`internal/vm/api/do.go:975-1010`):
```go
func Resume(L, from, nArgs) {
    // ...
    status = RunProtected(L, func() {
        if L.Status == stateapi.StatusOK {
            Call(L, funcIdx, stateapi.MultiRet)  // first call
        } else {
            // Resuming from yield
            L.Status = stateapi.StatusOK
            ci := L.CI
            if !ci.IsLua() && ci.K != nil {
                n := ci.K(L, stateapi.StatusYield, ci.Ctx)
                PosCall(L, ci, n)
            }
            unroll(L)
        }
    })
}
```

**RunProtected** (`internal/vm/api/do.go:728-760`):
```go
func RunProtected(L, f func()) (status int) {
    defer func() {
        if r := recover(); r != nil {
            switch e := r.(type) {
            case stateapi.LuaYield:
                status = stateapi.StatusYield  // ← caught by recover
            // ...
            }
        }
    }()
    f()
    return status
}
```

### Implication

Since go-lua uses panic/recover (identical to C Lua's setjmp/longjmp), it faces
the **exact same problem** as C Lua: when yield panics, the Go call stack is
destroyed. Intermediate Go frames that would place metamethod results into
registers are lost. Therefore go-lua **needs** `luaV_finishOp` — the same
mechanism C Lua uses.

---

## 2. Test Results — What Actually Fails

Four targeted tests were run (metamethod functions that call `coroutine.yield()`
inside a `coroutine.wrap`):

| Test | Metamethod | Result | Failure Mode |
|------|-----------|--------|-------------|
| `__add` yield | OP_MMBIN | **FAIL** | Returns table (raw operand) instead of 22 |
| `__lt` yield | OP_LT | **FAIL** | Comparison evaluates wrong ("not less" instead of "less") |
| `__index` yield | OP_GETFIELD | **FAIL** | Returns nil instead of 42 |
| `for` iterator yield | OP_TFORCALL | **PASS** | Already works |

### Test Code (for reproduction)

```lua
-- __add test (FAILS)
local mt = { __add = function(a,b) coroutine.yield(nil,"add"); return a.x+b.x end }
local a = setmetatable({x=10}, mt)
local b = setmetatable({x=12}, mt)
local co = coroutine.wrap(function() return a + b end)
local res, stat = co()        -- yields with nil, "add"
assert(stat == "add")          -- OK
local result = co()            -- resumes, should return 22
assert(result == 22)           -- FAILS: got table instead of 22

-- __lt test (FAILS)
local mt = { __lt = function(a,b) coroutine.yield(nil,"lt"); return a.x < b.x end }
local a, b = setmetatable({x=10}, mt), setmetatable({x=12}, mt)
local co = coroutine.wrap(function()
    if a < b then return "less" else return "not less" end
end)
co()                           -- yields with nil, "lt"
assert(co() == "less")         -- FAILS: got "not less"

-- __index test (FAILS)
local mt = { __index = function(t,k) coroutine.yield(nil,"idx"); return t.data[k] end }
local obj = setmetatable({data={x=42}}, mt)
local co = coroutine.wrap(function() return obj.x end)
co()                           -- yields with nil, "idx"
assert(co() == 42)             -- FAILS: got nil

-- for iterator test (PASSES)
local f = function(s,i)
    if i%2==0 then coroutine.yield(nil,"for") end
    if i < s then return i+1 end
end
local co = coroutine.wrap(function()
    local s=0; for i in f,4,0 do s=s+i end; return s
end)
co(); co(); co()               -- three yields
assert(co() == 10)             -- PASSES
```

---

## 3. Root Cause Analysis

### The Call Chain That Breaks

For `a + b` where `__add` yields:

```
Resume() → RunProtected() → Call() → Execute(main_func)
  → OP_ADD fails fast path → falls through to OP_MMBIN
  → tryBinTM() → callTMRes() → Call() → Execute(__add_metamethod)
    → ... → Yield() → panic(LuaYield{})
```

The panic unwinds through:
1. `Execute(__add)` — no recover, propagates
2. `Call()` inside `callTMRes` — no recover, propagates
3. `callTMRes()` — no recover, propagates
4. `tryBinTM()` — no recover, propagates
5. `Execute(main)` — no recover, propagates
6. `Call()` in Resume — no recover, propagates
7. `RunProtected()` — **has recover**, catches `LuaYield` → `StatusYield`

### What `callTMRes` Would Have Done (But Doesn't)

```go
// vm.go:675-685
func callTMRes(L, tm, p1, p2) TValue {
    top := L.Top
    L.Stack[top] = tm
    L.Stack[top+1] = p1
    L.Stack[top+2] = p2
    L.Top = top + 3
    Call(L, top, 1)          // ← YIELD HAPPENS HERE
    result := L.Stack[top]   // ← NEVER REACHED
    L.Top = top              // ← NEVER REACHED
    return result             // ← NEVER REACHED
}
```

After `Call(L, top, 1)`, PosCall moves the metamethod's return value to
`L.Stack[top]` (the func slot). Lines after `Call()` would read it and
return it to `tryBinTM`, which would store it in the destination register.
**None of this happens when yield panics.**

### What Happens on Resume

1. `Resume()` → `RunProtected()` → resume function
2. `L.CI` points to `__add` metamethod's CI (it was the active CI when yield happened)
3. `unroll()` loops: sees `__add` CI is Lua → calls `Execute(L, ci)`
4. `Execute` resumes `__add` from its savedPC, `__add` returns 22 via OP_RETURN
5. `PosCall` moves 22 to `__add`'s func slot, sets `L.CI = ci.Prev` (main func)
6. `Execute` returns (CISTFresh) back to `unroll()`
7. `unroll()` loops: sees main func CI is Lua → calls `Execute(L, ci)`
8. `Execute` resumes main func from savedPC (instruction AFTER OP_MMBIN)
9. **Problem**: The metamethod result (22) is sitting at `L.Top-1` but nobody
   moved it into the destination register (RA of the original OP_ADD).

### Why For-Iterator Already Works

`OP_TFORCALL` in go-lua (`vm.go:2594-2604`) calls `Call(L, ra+3, nr)` directly.
The iterator function's return values go through normal `PosCall` into the correct
stack positions (`ra+3`, `ra+4`, etc.). No intermediate Go frame needs to place
results — `PosCall` does it. When resumed after yield, `unroll()` → `Execute()`
picks up at `OP_TFORLOOP`, and results are already in place.

---

## 4. C Lua's `luaV_finishOp` — Complete Reference

Source: `lua-master/lvm.c:568-618`

C Lua calls `luaV_finishOp(L)` **unconditionally** for every Lua CI in `unroll()`:

```c
// ldo.c — unroll
while ((ci = L->ci) != &L->base_ci) {
    if (!isLua(ci))
        finishCcall(L, ci);
    else {
        luaV_finishOp(L);      // ← UNCONDITIONAL for Lua CIs
        luaV_execute(L, ci);
    }
}
```

### FinishOp Cases

```c
void luaV_finishOp (lua_State *L) {
    CallInfo *ci = L->ci;
    StkId base = ci->func.p + 1;
    Instruction inst = *(ci->u.l.savedpc - 1);  // interrupted instruction
    OpCode op = GET_OPCODE(inst);
    switch (op) {
```

#### Case 1: Binary Arithmetic Metamethods
**Opcodes**: `OP_MMBIN`, `OP_MMBINI`, `OP_MMBINK`

```c
case OP_MMBIN: case OP_MMBINI: case OP_MMBINK: {
    setobjs2s(L, base + GETARG_A(*(ci->u.l.savedpc - 2)), --L->top.p);
    break;
}
```

- `savedpc-1` = OP_MMBIN (the interrupted instruction)
- `savedpc-2` = OP_ADD/SUB/MUL/etc. (the original arithmetic instruction)
- Action: pop TM result from stack top → store in RA of original arith op

#### Case 2: Unary + Table Get
**Opcodes**: `OP_UNM`, `OP_BNOT`, `OP_LEN`, `OP_GETTABUP`, `OP_GETTABLE`, `OP_GETI`, `OP_GETFIELD`, `OP_SELF`

```c
case OP_UNM: case OP_BNOT: case OP_LEN:
case OP_GETTABUP: case OP_GETTABLE: case OP_GETI:
case OP_GETFIELD: case OP_SELF: {
    setobjs2s(L, base + GETARG_A(inst), --L->top.p);
    break;
}
```

- `inst` = the interrupted instruction itself
- Action: pop TM result from stack top → store in RA of this instruction

#### Case 3: Comparisons
**Opcodes**: `OP_LT`, `OP_LE`, `OP_LTI`, `OP_LEI`, `OP_GTI`, `OP_GEI`, `OP_EQ`

```c
case OP_LT: case OP_LE:
case OP_LTI: case OP_LEI:
case OP_GTI: case OP_GEI:
case OP_EQ: {
    int res = !l_isfalse(s2v(L->top.p - 1));
    L->top.p--;
    lua_assert(GET_OPCODE(*ci->u.l.savedpc) == OP_JMP);
    if (res != GETARG_k(inst))  // condition failed?
        ci->u.l.savedpc++;      // skip jump instruction
    break;
}
```

Note: `OP_EQI` and `OP_EQK` **cannot yield** (no metamethod for basic types).

- Action: read TM result as boolean, pop it, then do conditional jump logic

#### Case 4: Concat
**Opcode**: `OP_CONCAT`

```c
case OP_CONCAT: {
    StkId top = L->top.p - 1;
    int a = GETARG_A(inst);
    int total = cast_int(top - 1 - (base + a));
    setobjs2s(L, top - 2, top);    // put TM result in proper position
    L->top.p = top - 1;
    luaV_concat(L, total);          // concat remaining (may yield again)
    break;
}
```

- Action: reposition TM result, adjust top, continue concat loop

#### Case 5: Close/Return (already implemented in go-lua)
**Opcodes**: `OP_CLOSE`, `OP_RETURN`

```c
case OP_CLOSE: {
    ci->u.l.savedpc--;  // repeat instruction to close other vars
    break;
}
case OP_RETURN: {
    StkId ra = base + GETARG_A(inst);
    L->top.p = ra + ci->u2.nres;
    ci->u.l.savedpc--;
    break;
}
```

#### Case 6: No-op (just continue)
**Opcodes**: `OP_TFORCALL`, `OP_CALL`, `OP_TAILCALL`, `OP_SETTABUP`, `OP_SETTABLE`, `OP_SETI`, `OP_SETFIELD`

```c
default: {
    lua_assert(op == OP_TFORCALL || op == OP_CALL ||
         op == OP_TAILCALL || op == OP_SETTABUP || op == OP_SETTABLE ||
         op == OP_SETI || op == OP_SETFIELD);
    break;
}
```

- Action: nothing — results already in correct place or no result needed

---

## 5. go-lua's Current FinishOp — Gap Analysis

### Current Implementation (`internal/vm/api/vm.go:2477-2502`)

```go
func FinishOp(L *stateapi.LuaState, ci *stateapi.CallInfo) {
    cl := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
    code := cl.Proto.Code
    inst := code[ci.SavedPC-1]
    op := opcodeapi.GetOpCode(inst)
    base := ci.Func + 1
    switch op {
    case opcodeapi.OP_CLOSE:
        ci.SavedPC--
    case opcodeapi.OP_RETURN:
        ra := base + opcodeapi.GetArgA(inst)
        L.Top = ra + ci.NRes
        ci.SavedPC--
    default:
        // no adjustment needed
    }
}
```

### Missing Cases

| C Lua FinishOp Case | go-lua Status | Impact |
|---------------------|---------------|--------|
| OP_MMBIN/MMBINI/MMBINK | **MISSING** | __add, __sub, __mul, etc. yield broken |
| OP_UNM, OP_BNOT | **MISSING** | Unary metamethod yield broken |
| OP_LEN | **MISSING** | __len yield broken |
| OP_GETTABUP/GETTABLE/GETI/GETFIELD/SELF | **MISSING** | __index yield broken |
| OP_LT/LE/LTI/LEI/GTI/GEI/EQ | **MISSING** | Comparison metamethod yield broken |
| OP_CONCAT | **MISSING** | __concat yield broken |
| OP_SETTABUP/SETTABLE/SETI/SETFIELD | **MISSING** (but no-op) | __newindex yield — works by accident |
| OP_TFORCALL | **MISSING** (but no-op) | For iterator yield — works by accident |
| OP_CALL/OP_TAILCALL | **MISSING** (but no-op) | Function call yield — works by accident |
| OP_CLOSE | ✅ Implemented | |
| OP_RETURN | ✅ Implemented | |

### `unroll()` Bug

**Current** (`do.go:1126-1133`):
```go
} else {
    if ci.CallStatus&stateapi.CISTClsRet != 0 {  // ← ONLY when closing
        FinishOp(L, ci)
    }
    Execute(L, ci)
}
```

**C Lua** (`ldo.c:869-872`):
```c
} else {
    luaV_finishOp(L);      // ← UNCONDITIONAL
    luaV_execute(L, ci);
}
```

**Fix**: Remove the `CISTClsRet` guard. Call `FinishOp` unconditionally for Lua CIs.

---

## 6. Implementation Plan

### Change 1: Expand `FinishOp` in `internal/vm/api/vm.go`

Add cases for all metamethod opcodes. The TM result is at `L.Stack[L.Top-1]`
(placed there by PosCall when the metamethod returned).

```go
func FinishOp(L *stateapi.LuaState, ci *stateapi.CallInfo) {
    cl := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
    code := cl.Proto.Code
    inst := code[ci.SavedPC-1]  // interrupted instruction
    op := opcodeapi.GetOpCode(inst)
    base := ci.Func + 1

    switch op {
    case opcodeapi.OP_MMBIN, opcodeapi.OP_MMBINI, opcodeapi.OP_MMBINK:
        // Pop TM result → RA of the original arithmetic instruction (2 back)
        L.Top--
        prevInst := code[ci.SavedPC-2]
        dest := base + opcodeapi.GetArgA(prevInst)
        L.Stack[dest].Val = L.Stack[L.Top].Val

    case opcodeapi.OP_UNM, opcodeapi.OP_BNOT, opcodeapi.OP_LEN,
         opcodeapi.OP_GETTABUP, opcodeapi.OP_GETTABLE, opcodeapi.OP_GETI,
         opcodeapi.OP_GETFIELD, opcodeapi.OP_SELF:
        // Pop TM result → RA of this instruction
        L.Top--
        dest := base + opcodeapi.GetArgA(inst)
        L.Stack[dest].Val = L.Stack[L.Top].Val

    case opcodeapi.OP_LT, opcodeapi.OP_LE,
         opcodeapi.OP_LTI, opcodeapi.OP_LEI,
         opcodeapi.OP_GTI, opcodeapi.OP_GEI,
         opcodeapi.OP_EQ:
        // Evaluate TM result as boolean, then conditional jump
        res := !L.Stack[L.Top-1].Val.IsFalsy()
        L.Top--
        // Next instruction must be OP_JMP
        if res != (opcodeapi.GetArgK(inst) != 0) {
            ci.SavedPC++  // skip jump
        }

    case opcodeapi.OP_CONCAT:
        top := L.Top - 1
        a := opcodeapi.GetArgA(inst)
        total := (top - 1) - (base + a)
        L.Stack[top-2].Val = L.Stack[top].Val  // reposition TM result
        L.Top = top - 1
        Concat(L, total)  // continue concat (may yield again)

    case opcodeapi.OP_CLOSE:
        ci.SavedPC--

    case opcodeapi.OP_RETURN:
        ra := base + opcodeapi.GetArgA(inst)
        L.Top = ra + ci.NRes
        ci.SavedPC--

    default:
        // OP_TFORCALL, OP_CALL, OP_TAILCALL,
        // OP_SETTABUP, OP_SETTABLE, OP_SETI, OP_SETFIELD
        // No action needed
    }
}
```

### Change 2: Fix `unroll()` in `internal/vm/api/do.go`

```go
// BEFORE (line 1126-1133):
} else {
    if ci.CallStatus&stateapi.CISTClsRet != 0 {
        FinishOp(L, ci)
    }
    Execute(L, ci)
}

// AFTER:
} else {
    FinishOp(L, ci)   // unconditional — matches C Lua
    Execute(L, ci)
}
```

### Change 3: Enable skipped tests

Remove `if false` wrapper in `lua-master/testes/coroutine.lua:858` and
closing `end` at line 1055.

### Change 4 (optional): Enable nextvar.lua yield test

`lua-master/testes/nextvar.lua:938-957` — yield inside `__pairs`. This test
uses `_port` guard (not `if false`), so it may already be skipped by the
test framework. Verify after main fix.

---

## 7. What Was NOT Checked

1. **OP_CONCAT yield**: Not tested empirically. The FinishOp logic for concat
   is the most complex (repositions result, then re-calls Concat which may
   yield again). Needs careful testing.

2. **__newindex yield**: Not tested. Should work because OP_SETTABLE etc. are
   in the no-op case of FinishOp, but the `callTM` helper (which doesn't
   return a result) may have different stack state. Needs verification.

3. **Nested yields**: Multiple yields within a single metamethod chain
   (e.g., `__le` calls `__sub` which yields). Should work because each
   yield/resume goes through the same FinishOp path, but untested.

4. **Stack state correctness**: After FinishOp places the result, L.Top
   must be correct for Execute to resume. The `callTMRes` pattern leaves
   the result at `L.Stack[top]` where `top` was the original L.Top. After
   PosCall, the result is at the func slot. Need to verify that `L.Top`
   after FinishOp's `L.Top--` is correct for each case.

5. **NCCalls counter**: When panic unwinds through `Call()`, the `L.NCCalls--`
   at the end of `Call()` never executes. RunProtected restores NCCalls from
   `oldNCCalls`, but there may be an off-by-one. C Lua handles this in
   `luaE_incCstack` / the restore in `rawrunprotected`. Needs verification.

6. **Hook interaction**: Line/count hooks during metamethod execution that
   yields. The hook dispatch uses `Call()` which is yieldable, but hooks
   set `AllowHook = false` during dispatch. Should be fine but untested.

---

## 8. Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| FinishOp stack state wrong | High | Test each opcode category individually |
| L.Top incorrect after FinishOp | High | Compare with C Lua's exact stack state |
| NCCalls leak | Medium | Add assertion: NCCalls at resume start == expected |
| Concat re-yield loop | Medium | Test multi-element concat with yields |
| Regression in existing tests | Low | 21/21 testes gate |

---

## 9. Summary of Changes

| File | Change | Lines |
|------|--------|-------|
| `internal/vm/api/vm.go` | Expand `FinishOp()` with all metamethod cases | +50-60 |
| `internal/vm/api/do.go` | Remove `CISTClsRet` guard in `unroll()` | ~2 |
| `lua-master/testes/coroutine.lua` | Remove `if false` at :858 and `end` at :1055 | -2 |
| **Total** | | **~60 lines changed** |

The old design doc estimated 500-1000 lines and 5-10 days. The actual fix is
~60 lines and should take a few hours, because:

1. go-lua already has the continuation infrastructure (CallInfo.K, unroll, etc.)
2. The metamethod callers already use `Call()` (yieldable), not `CallNoYield()`
3. PosCall already places the TM result at the correct stack position
4. Only `FinishOp` needs to move it from stack top to the destination register
