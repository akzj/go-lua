# Error Messages — Name Resolution and Type Error Analysis

## Overview

Lua's error message system produces rich diagnostic messages like `"attempt to index a nil value (local 'x')"`. This requires **reverse engineering** the bytecode to find variable names. The system had **8 bugs** in go-lua — the highest bug density of any subsystem.

**C Source:** `lua-master/ldebug.c` — lines 440–815
**Go Source:** `internal/vm/callerror.go`, `internal/vm/debug.go:271–400`, `internal/vm/vm.go:700–740`

---

## 1. Architecture: The Name Resolution Pipeline

```
Error occurs at instruction N
  │
  ├─ varinfo(L, o)              → " (local 'x')" / " (upvalue 'y')" / ""
  │   ├─ getupvalname(ci, o)    → check if value is an upvalue
  │   ├─ instack(ci, o)         → find register index of value
  │   └─ getobjname(p, pc, reg) → trace bytecode for name
  │       ├─ basicgetobjname()  → local/upvalue/constant/move
  │       │   └─ findsetreg()   → symbolic execution to find setter
  │       └─ table access cases → GETTABUP/GETTABLE/GETFIELD/SELF
  │           └─ isEnv()        → is table _ENV? → "global" vs "field"
  │
  └─ funcnamefromcode(L, p, pc) → "(method 'foo')" / "(metamethod 'add')"
      ├─ CALL/TAILCALL          → getobjname for function register
      ├─ TFORCALL               → "for iterator"
      └─ metamethod opcodes     → TM name lookup
```

---

## 2. `findsetreg` — Symbolic Execution Engine

### C Source (ldebug.c:443–490)

```c
static int findsetreg (const Proto *p, int lastpc, int reg) {
  int setreg = -1;        // last instruction that changed 'reg'
  int jmptarget = 0;      // code before this is conditional
  if (testMMMode(GET_OPCODE(p->code[lastpc])))
    lastpc--;              // [1] MM instruction wasn't really executed
  for (pc = 0; pc < lastpc; pc++) {
    // ... check each instruction for register modification
    if (change)
      setreg = filterpc(pc, jmptarget);  // [2] Skip conditional sets
  }
  return setreg;
}
```

**WHY each key decision:**
- **[1] testMMMode skip**: Metamethod instructions (OP_MMBIN etc.) are preceded by the actual operation. The MM instruction itself wasn't executed — the error happened at the preceding instruction.
- **[2] filterpc**: If `pc < jmptarget`, the instruction is inside a conditional branch. We can't be sure it executed, so return -1. This prevents reporting wrong names from untaken branches.

**Special cases tracked:**
- `OP_LOADNIL`: Sets registers `a` through `a+b`
- `OP_CALL/OP_TAILCALL`: Clobbers all registers `>= a`
- `OP_TFORCALL`: Clobbers registers `>= a+2`
- `OP_JMP`: Updates `jmptarget` but doesn't change registers

### Go Mapping: `findSetRegForward` (debug.go — not shown in callerror.go)

Go uses a function called `findSetRegForward` in `internal/vm/debug.go` that performs the same symbolic walk. Key difference: Go scans **forward** from instruction 0 to `lastpc` (same as C), tracking the last instruction that set the target register.

---

## 3. `basicgetobjname` — Core Name Resolver

### C Source (ldebug.c:504–533)

```c
static const char *basicgetobjname (const Proto *p, int *ppc, int reg,
                                    const char **name) {
  int pc = *ppc;
  *name = luaF_getlocalname(p, reg + 1, pc);  // [1] Check LocVars
  if (*name) return strlocal;                   // Found a local
  *ppc = pc = findsetreg(p, pc, reg);           // [2] Symbolic execution
  if (pc != -1) {
    Instruction i = p->code[pc];
    switch (GET_OPCODE(i)) {
      case OP_MOVE:                              // [3] Trace through moves
        if (b < GETARG_A(i))
          return basicgetobjname(p, ppc, b, name);
      case OP_GETUPVAL:                          // [4] Upvalue name
        *name = upvalname(p, GETARG_B(i));
        return strupval;
      case OP_LOADK:                             // [5] Constant name
        return kname(p, GETARG_Bx(i), name);
      case OP_LOADKX:
        return kname(p, GETARG_Ax(p->code[pc + 1]), name);
    }
  }
  return NULL;
}
```

**WHY each case:**
- **[1]**: Local variable debug info (`LocVars`) gives the most readable names. Check first.
- **[2]**: If no local name, find which instruction last set this register.
- **[3]**: `OP_MOVE` copies register B to A. Recurse on B to find the original name. Guard `b < a` prevents infinite recursion on self-moves.
- **[4]**: `OP_GETUPVAL` loads an upvalue — get its name from the proto's upvalue table.
- **[5]**: `OP_LOADK` loads a constant — if it's a string, that's the name.

### Go Mapping: `BasicGetObjName` (debug.go:274–356)

```go
func BasicGetObjName(p *objectapi.Proto, pc int, reg int) (kind string, name string) {
    setpc := findSetRegForward(p, pc, reg)        // [2]
    if setpc < 0 {
        if name := locVarName(p, pc, reg); name != "" {
            return "local", name                   // [1] — checked AFTER findsetreg
        }
        return "", ""
    }
    // ... switch on opcode, same cases as C
}
```

**Critical difference**: Go checks `locVarName` **after** `findSetRegForward` fails (when `setpc < 0`), while C checks `luaF_getlocalname` **first**. This means Go may miss local names when `findsetreg` returns a valid PC but the instruction doesn't match any case.

Go also handles additional opcodes that C handles in `getobjname`:
- `OP_GETTABUP`, `OP_GETFIELD`, `OP_GETTABLE`, `OP_SELF` — merged into `BasicGetObjName`

---

## 4. `getobjname` — Extended Name Resolution with Table Access

### C Source (ldebug.c:572–604)

```c
static const char *getobjname (const Proto *p, int lastpc, int reg,
                               const char **name) {
  const char *kind = basicgetobjname(p, &lastpc, reg, name);
  if (kind != NULL) return kind;
  else if (lastpc != -1) {  // findsetreg found an instruction
    Instruction i = p->code[lastpc];
    switch (GET_OPCODE(i)) {
      case OP_GETTABUP:   // upvalue[B][K[C]] — check if _ENV
        kname(p, GETARG_C(i), name);
        return isEnv(p, lastpc, i, 1);     // "global" if _ENV, else "field"
      case OP_GETTABLE:   // reg[B][reg[C]]
        rname(p, lastpc, GETARG_C(i), name);
        return isEnv(p, lastpc, i, 0);
      case OP_GETI:       // reg[B][integer]
        *name = "integer index";
        return "field";
      case OP_GETFIELD:   // reg[B][K[C]]
        kname(p, GETARG_C(i), name);
        return isEnv(p, lastpc, i, 0);
      case OP_SELF:       // method call
        kname(p, GETARG_C(i), name);
        return "method";
    }
  }
  return NULL;
}
```

**WHY `isEnv`**: When indexing `_ENV`, the access is a **global variable** reference. `isEnv` checks if the table being indexed is `_ENV` (by tracing the table register/upvalue back to see if it's named `"_ENV"`). This produces `"global 'foo'"` instead of `"field 'foo'"`.

### Go Mapping

Go merges `getobjname` into `BasicGetObjName` (debug.go:274–356). The table access cases are handled in the same switch statement. `isEnvReg` (debug.go:359–365) mirrors C's `isEnv`.

---

## 5. `varinfo` — Building the Description String

### C Source (ldebug.c:727–738)

```c
static const char *varinfo (lua_State *L, const TValue *o) {
  CallInfo *ci = L->ci;
  const char *name = NULL;
  const char *kind = NULL;
  if (isLua(ci)) {
    kind = getupvalname(ci, o, &name);  // [1] Check upvalues first
    if (!kind) {
      int reg = instack(ci, o);          // [2] Find register by pointer
      if (reg >= 0)
        kind = getobjname(ci_func(ci)->p, currentpc(ci), reg, &name);
    }
  }
  return formatvarinfo(L, kind, name);   // [3] " (kind 'name')" or ""
}
```

**WHY this order:**
- **[1]**: Upvalue check first — upvalues are accessed by pointer, not register index. `getupvalname` compares the value pointer `o` against each upvalue's `v.p`.
- **[2]**: `instack` finds which register holds the value by scanning the activation frame. This is a **pointer comparison** in C.
- **[3]**: `formatvarinfo` produces `" (local 'x')"` or `""` if no info found.

### Go Mapping: `VarInfo` (debug.go:382–400)

```go
func VarInfo(L *stateapi.LuaState, reg int) string {
    ci := L.CI
    if ci == nil || !ci.IsLua() { return "" }
    cl, ok := L.Stack[ci.Func].Val.Val.(*closureapi.LClosure)
    if !ok || cl.Proto == nil { return "" }
    pc := ci.SavedPC - 1
    kind, name := BasicGetObjName(cl.Proto, pc, reg)
    if kind == "" { return "" }
    return fmt.Sprintf(" (%s '%s')", kind, name)
}
```

**Critical difference**: Go takes a **register index** directly, not a value pointer. C's `varinfo` takes a `TValue*` and must find the register via `instack()` (pointer scan). Go skips the upvalue check entirely — it goes straight to `BasicGetObjName` with the register.

This means Go's `VarInfo` **cannot identify upvalues** — only locals, constants, globals, and fields. C's `varinfo` can identify upvalues because `getupvalname` checks upvalue pointers directly.

---

## 6. `funcnamefromcode` — Function Name from Calling Instruction

### C Source (ldebug.c:612–654)

```c
static const char *funcnamefromcode (lua_State *L, const Proto *p,
                                     int pc, const char **name) {
  Instruction i = p->code[pc];
  switch (GET_OPCODE(i)) {
    case OP_CALL: case OP_TAILCALL:
      return getobjname(p, pc, GETARG_A(i), name);  // function register
    case OP_TFORCALL:
      *name = "for iterator"; return "for iterator";
    // Metamethod opcodes:
    case OP_SELF: ... case OP_GETFIELD: tm = TM_INDEX; break;
    case OP_SETTABUP: ... case OP_SETFIELD: tm = TM_NEWINDEX; break;
    case OP_MMBIN: ... tm = GETARG_C(i); break;
    case OP_UNM: tm = TM_UNM; break;
    // ... more metamethods
    case OP_CLOSE: case OP_RETURN: tm = TM_CLOSE; break;
    default: return NULL;
  }
  *name = getshrstr(G(L)->tmname[tm]) + 2;  // strip "__" prefix
  return "metamethod";
}
```

**WHY `+ 2`**: Metamethod names are stored as `"__add"`, `"__index"`, etc. The `+ 2` skips the `"__"` prefix to produce cleaner error messages like `"metamethod 'add'"`.

### Go Mapping: `funcNameFromCode` (callerror.go:18–72)

```go
func funcNameFromCode(L *stateapi.LuaState, p *objectapi.Proto, pc int) (string, string) {
    // ... same switch structure
    case opcodeapi.OP_MMBIN, opcodeapi.OP_MMBINI, opcodeapi.OP_MMBINK:
        tm := opcodeapi.GetArgC(i)
        if tm < len(mmapi.TMNames) {
            name := mmapi.TMNames[tm]
            if len(name) > 2 { name = name[2:] }  // strip "__"
            return "metamethod", name
        }
    // ... hardcoded metamethod names for other opcodes
}
```

**Difference**: C looks up `G(L)->tmname[tm]` (global string table); Go uses `mmapi.TMNames[tm]` (a Go slice). Go also hardcodes metamethod names for non-MMBIN opcodes (e.g., `"index"`, `"newindex"`) instead of looking them up.

---

## 7. `funcnamefromcall` — Name from Calling Context

### C Source (ldebug.c:658–671)

```c
static const char *funcnamefromcall (lua_State *L, CallInfo *ci,
                                                   const char **name) {
  if (ci->callstatus & CIST_HOOKED) {      // [1] Inside a hook?
    *name = "?"; return "hook";
  }
  else if (ci->callstatus & CIST_FIN) {     // [2] Finalizer?
    *name = "__gc"; return "metamethod";
  }
  else if (isLua(ci))                       // [3] Lua caller?
    return funcnamefromcode(L, ci_func(ci)->p, currentpc(ci), name);
  else
    return NULL;                            // [4] C caller — no info
}
```

**WHY each case:**
- **[1]**: During hook execution, we can't determine the function name from bytecode.
- **[2]**: Finalizers are called by the GC, not by bytecode. Report as `"metamethod '__gc'"`.
- **[3]**: Only Lua callers have bytecode to analyze.
- **[4]**: C callers don't have proto/bytecode — no name available.

### Go Mapping

Go doesn't have a separate `funcnamefromcall`. Instead, `callErrorExtra` (callerror.go:74–88) checks `L.CI.IsLua()` and calls `funcNameFromCode` directly. The `CIST_HOOKED` and `CIST_FIN` special cases are **not implemented** in Go.

---

## 8. Error Functions

### `luaG_typeerror` (ldebug.c:753–755)

```c
l_noret luaG_typeerror (lua_State *L, const TValue *o, const char *op) {
  typeerror(L, o, op, varinfo(L, o));
}
// Produces: "attempt to <op> a <type> value <varinfo>"
```

**Go**: `RunTypeError` (callerror.go:95–101) and `RunTypeErrorByVal` (callerror.go:107–162).

`RunTypeError` takes an explicit register index. `RunTypeErrorByVal` examines the current instruction to determine which register holds the value — it inlines the C logic of `varinfo` + instruction analysis.

### `luaG_callerror` (ldebug.c:764–772)

```c
l_noret luaG_callerror (lua_State *L, const TValue *o) {
  CallInfo *ci = L->ci;
  const char *name = NULL;
  const char *kind = funcnamefromcall(L, ci, &name);
  const char *extra = kind ? formatvarinfo(L, kind, name) : varinfo(L, o);
  typeerror(L, o, "call", extra);
}
// Produces: "attempt to call a <type> value (<kind> '<name>')"
```

**WHY two paths**: First tries `funcnamefromcall` (examines the CALLING instruction). If that fails, falls back to `varinfo` (examines the VALUE's origin). The calling instruction often gives better context (e.g., `"global 'foo'"` vs just the type).

**Go**: `callErrorExtra` (callerror.go:74–88) + `TryFuncTM` (do.go:293–299). Go calls `callErrorExtra` which tries `funcNameFromCode`, then falls back to `VarInfo`.

### `luaG_forerror` (ldebug.c:776–779)

```c
l_noret luaG_forerror (lua_State *L, const TValue *o, const char *what) {
  luaG_runerror(L, "bad 'for' %s (number expected, got %s)",
                   what, luaT_objtypename(L, o));
}
// Produces: "bad 'for' limit (number expected, got string)"
```

**Go**: Inlined in `ForPrep` (vm.go:1145–1255) as direct `RunError` calls:
```go
RunError(L, "'for' limit must be a number")
RunError(L, "'for' step must be a number")
```

**Difference**: C includes the actual type name (`"got string"`); Go uses fixed messages without the type.

### `luaG_concaterror` (ldebug.c:781–784)

```c
l_noret luaG_concaterror (lua_State *L, const TValue *p1, const TValue *p2) {
  if (ttisstring(p1) || cvt2str(p1)) p1 = p2;  // pick the bad operand
  luaG_typeerror(L, p1, "concatenate");
}
```

**Go**: Inlined in `concatTM` (vm.go:778–785):
```go
errType := p1
if p1.IsString() || p1.IsNumber() { errType = p2 }
RunError(L, "attempt to concatenate a "+TypeNames[errType.Type()]+" value")
```

**Difference**: Go doesn't include `varinfo` in the concat error — no variable name context.

### `luaG_tointerror` (ldebug.c:798–803)

```c
l_noret luaG_tointerror (lua_State *L, const TValue *p1, const TValue *p2) {
  lua_Integer temp;
  if (!luaV_tointegerns(p1, &temp, LUA_FLOORN2I))
    p2 = p1;  // pick the non-convertible one
  luaG_runerror(L, "number%s has no integer representation", varinfo(L, p2));
}
```

**Go**: Inlined in `tryBinTM` (vm.go:728–735):
```go
RunError(L, "number"+VarInfo(L, badReg)+" has no integer representation")
```

**Match**: Both produce `"number (local 'x') has no integer representation"`. Go uses `VarInfo` with register index.

### `luaG_ordererror` (ldebug.c:806–813)

```c
l_noret luaG_ordererror (lua_State *L, const TValue *p1, const TValue *p2) {
  const char *t1 = luaT_objtypename(L, p1);
  const char *t2 = luaT_objtypename(L, p2);
  if (strcmp(t1, t2) == 0)
    luaG_runerror(L, "attempt to compare two %s values", t1);
  else
    luaG_runerror(L, "attempt to compare %s with %s", t1, t2);
}
```

**Go**: Inlined in VM comparison ops (vm.go:490–492):
```go
RunError(L, "attempt to compare two "+TypeNames[l.Type()]+" values")
RunError(L, "attempt to compare "+TypeNames[l.Type()]+" with "+TypeNames[r.Type()]+" values")
```

**Match**: Structurally identical messages.

---

## Difference Table

| Area | C Lua | go-lua | Severity | Notes |
|------|-------|--------|----------|-------|
| `varinfo` input | Takes `TValue*`, finds register via `instack()` | Takes register index directly | **MEDIUM** | Go can't detect upvalues in `VarInfo` |
| Upvalue detection | `getupvalname` checks upvalue pointers | Not implemented in `VarInfo` | **HIGH** | Go never reports `"upvalue 'x'"` from `VarInfo` |
| `funcnamefromcall` | Checks `CIST_HOOKED`, `CIST_FIN` | Neither checked | **MEDIUM** | Wrong name in hook/finalizer error contexts |
| `isEnv` | Checks local/upvalue named `_ENV` | `isEnvReg` checks via `BasicGetObjName` | **LOW** | Functionally equivalent |
| `locVarName` order | Checked FIRST in `basicgetobjname` | Checked as FALLBACK in `BasicGetObjName` | **MEDIUM** | May miss local names when findsetreg succeeds |
| `forerror` message | `"bad 'for' %s (number expected, got %s)"` | `"'for' limit must be a number"` | **LOW** | Less informative but functional |
| `concaterror` varinfo | Includes variable name context | No variable name | **MEDIUM** | Less helpful error messages |
| `getobjname` structure | Separate from `basicgetobjname` | Merged into `BasicGetObjName` | **LOW** | Same coverage, different organization |
| `rname` for table keys | Falls back to `"?"` for non-constants | Same logic in `BasicGetObjName` | **LOW** | Equivalent behavior |
| `funcnamefromcode` TM lookup | `G(L)->tmname[tm]` (global strings) | `mmapi.TMNames[tm]` (Go slice) | **NONE** | Same names, different storage |
| `RunTypeErrorByVal` | N/A (C uses `varinfo` with pointer) | Inlines instruction analysis | **MEDIUM** | Go-specific; may diverge from C logic |
| `filterpc` conditional skip | Skips sets inside conditional jumps | Same logic in `findSetRegForward` | **LOW** | Equivalent |

---

## Verification Methods

### 1. Error Message Comparison Tests
```lua
-- Type error with local name
local x = nil; x()
-- Expected: "attempt to call a nil value (local 'x')"

-- Type error with global name
foo()
-- Expected: "attempt to call a nil value (global 'foo')"

-- Arithmetic error with variable info
local y = "hello"; local z = y + 1
-- Expected: "attempt to perform arithmetic on a string value (local 'y')"
```

### 2. For Loop Error Tests
```lua
for i = "a", 10 do end
-- C: "bad 'for' initial value (number expected, got string)"
-- Go: "'for' initial value must be a number"
```

### 3. Source Verification Commands
```bash
# Compare error function signatures
grep -n 'luaG_typeerror\|luaG_callerror\|luaG_forerror\|luaG_tointerror' lua-master/ldebug.c
grep -n 'RunTypeError\|RunTypeErrorByVal\|callErrorExtra' internal/vm/callerror.go

# Verify BasicGetObjName covers all C cases
grep -c 'case OP_' internal/vm/debug.go   # Go opcode cases
grep -c 'case OP_' lua-master/ldebug.c         # C opcode cases (in getobjname+basicgetobjname)

# Check upvalue detection gap
grep -n 'getupvalname\|upvalname' lua-master/ldebug.c
grep -n 'upval' internal/vm/debug.go
```

### 4. Regression Test
```bash
cd /home/ubuntu/workspace/go-lua && go test ./... 2>&1 | grep -i "error\|fail" | head -20
```
