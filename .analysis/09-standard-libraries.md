# Lua 5.5.1 Standard Libraries — Deep Analysis

> Distilled from `lapi.c` (1478 lines), `lauxlib.c` (1202 lines), `lbaselib.c` (559 lines),
> `lstrlib.c` (1894 lines), `ltablib.c` (429 lines), `ldblib.c` (477 lines).
> Total: 6,039 lines of C source. Reference: `/home/ubuntu/workspace/go-lua/lua-master/`

---

## Table of Contents

1. [lapi.c — The C API Layer](#1-lapic--the-c-api-layer)
2. [lauxlib.c — Auxiliary Library](#2-lauxlibc--auxiliary-library)
3. [lbaselib.c — Base Library](#3-lbaselibc--base-library)
4. [lstrlib.c — String Library](#4-lstrlibc--string-library)
5. [ltablib.c — Table Library](#5-ltablibc--table-library)
6. [ldblib.c — Debug Library](#6-ldblibcdblib--debug-library)
7. [Key Patterns — Library Registration & Argument Validation](#7-key-patterns)
8. [Go Implementation Guide](#8-go-implementation-guide)
9. [Edge Cases & Traps](#9-edge-cases--traps)
10. [Bug Pattern Guide](#10-bug-pattern-guide)

---

## 1. lapi.c — The C API Layer

The C API (`lapi.c`, 1478 lines) is the bridge between C library code and the Lua VM. Every standard library function is a C function that manipulates the Lua stack through this API. Understanding lapi.c is prerequisite to understanding any stdlib implementation.

### 1.1 Index Resolution — The Foundation

Every API function begins by resolving a stack index to a value pointer. Two helpers handle this:

#### `index2value(L, idx)` (line 58–86) — Returns `TValue*`

```
idx > 0:      ci->func.p + idx  (1-based from frame base)
              If o >= L->top.p → returns &G(L)->nilvalue (valid but "empty" slot)
idx < 0 && !ispseudo:  L->top.p + idx  (relative to top, -1 = top element)
idx == LUA_REGISTRYINDEX:  &G(L)->l_registry
idx < LUA_REGISTRYINDEX:   upvalue access
```

**Critical for Go**: When a positive index exceeds the current top but is within the frame, it returns the global nil singleton (`G(L)->nilvalue`). This is NOT a stack slot — it's a special sentinel. The `isvalid(L, o)` macro (line 44) tests `o != &G(L)->nilvalue`.

#### `index2stack(L, idx)` (line 93–106) — Returns `StkId`

Only for actual stack indices (NOT pseudo-indices). Used when modifying the stack slot itself:

```
idx > 0:  ci->func.p + idx  (asserts o < L->top.p — stricter than index2value)
idx < 0:  L->top.p + idx    (asserts !ispseudo)
```

**Key difference**: `index2value` tolerates above-top positive indices (returns nil), `index2stack` does NOT (asserts failure).

#### Pseudo-Index Macros (lines 47–51)

```c
#define ispseudo(i)    ((i) <= LUA_REGISTRYINDEX)
#define isupvalue(i)   ((i) < LUA_REGISTRYINDEX)
```

`LUA_REGISTRYINDEX` = `-(INT_MAX/2 + 1000)` (lua.h:43). Any index below that is an upvalue index.

#### The Index Space

```
Positive:  1, 2, 3, ...                    → stack slots from frame base
Negative:  -1, -2, -3, ...                 → stack slots from top
           LUA_REGISTRYINDEX                → global registry table
           LUA_REGISTRYINDEX - 1            → upvalue 1
           LUA_REGISTRYINDEX - 2            → upvalue 2
           ...
           LUA_REGISTRYINDEX - MAXUPVAL     → upvalue 255
```

Registry access (line 71–72): returns `&G(L)->l_registry`. Contains:
- `LUA_RIDX_MAINTHREAD` (=3): the main thread
- `LUA_RIDX_GLOBALS` (=2): the global table `_G`

Upvalue access (lines 73–85): Converts index to 1-based upvalue number, checks if current function is a C closure. Light C functions return nil (no upvalues). `lua_upvalueindex(i)` = `LUA_REGISTRYINDEX - (i)`.

### 1.2 Stack Manipulation Functions

| Function | Line | Signature | Notes |
|----------|------|-----------|-------|
| `lua_absindex` | 167 | `int (L, idx)` | Converts negative to absolute. Pseudo-indices returned as-is. |
| `lua_gettop` | 174 | `int (L)` | `top - (func + 1)` — number of values in current frame |
| `lua_settop` | 179 | `void (L, idx)` | If `idx >= 0`: sets top, fills with nil. If `idx < 0`: relative shrink. **⚠️ Can trigger `__close` metamethods** via `luaF_close` (line 199) when shrinking past TBC slots |
| `lua_pushvalue` | 268 | `void (L, idx)` | Pushes copy of value at `idx` onto top |
| `lua_rotate` | 238 | `void (L, idx, n)` | Three-reverse algorithm. Foundation for insert/remove/replace |
| `lua_copy` | 253 | `void (L, from, to)` | Copies value. Triggers GC barrier for upvalue targets |
| `lua_checkstack` | 109 | `int (L, n)` | Ensures `n` free slots. May grow stack. Returns 0 on failure |
| `lua_xmove` | 126 | `void (from, to, n)` | Moves `n` values between threads. Asserts same global state |
| `lua_closeslot` | 206 | `void (L, idx)` | Closes TBC slot, sets to nil |

**Derived macros** (lua.h):
```c
#define lua_insert(L,idx)    lua_rotate(L, (idx), 1)
#define lua_remove(L,idx)    (lua_rotate(L, (idx), -1), lua_pop(L, 1))
#define lua_replace(L,idx)   (lua_copy(L, -1, (idx)), lua_pop(L, 1))
#define lua_pop(L,n)         lua_settop(L, -(n)-1)
```

**⚠️ Go trap**: `lua_pop` calls `lua_settop` which can trigger `__close` metamethods. Popping values is NOT always trivial.

### 1.3 Push Functions (C → Stack)

All follow: set value at `L->top.p`, then `api_incr_top(L)`.

| Function | Line | Notes |
|----------|------|-------|
| `lua_pushnil` | 514 | Pushes strict nil (`LUA_VNIL`), not empty/abstkey |
| `lua_pushnumber` | 522 | Float value (`setfltvalue`) |
| `lua_pushinteger` | 530 | Integer value (`setivalue`) |
| `lua_pushlstring` | 543 | Creates interned TString. If `len==0`, uses `luaS_new(L, "")` |
| `lua_pushstring` | 570 | If `s == NULL`, pushes nil! Otherwise `luaS_new`. Returns internal pointer |
| `lua_pushfstring` | 598 | Formatted string via `luaO_pushvfstring` |
| `lua_pushcclosure` | 609 | **KEY**: If `n==0` → light C function (no GC). If `n>0` → C closure with upvalues. Pops `n` upvalues from stack |
| `lua_pushboolean` | 636 | Uses `setbtvalue`/`setbfvalue` — **two distinct tags** in Lua 5.5 |
| `lua_pushlightuserdata` | 647 | Raw pointer, no GC |
| `lua_pushthread` | 655 | Returns 1 if main thread |
| `lua_pushexternalstring` | 555 | **5.5 new**: external string with custom deallocator |

**Go insight**: `lua_pushcfunction(L, f)` = `lua_pushcclosure(L, f, 0)`. Light C functions have NO upvalues and are stored as raw function pointers. In Go, distinguish between `func(L *State) int` (light) and a closure-with-upvalues.

### 1.4 Type Checking Functions

#### `lua_type(L, idx)` (line 282)

Returns `ttype(o)` which strips variants via `novariant(rawtt(o))`. The **4 nil variants** all return `LUA_TNIL` (= 0):

```c
LUA_VNIL      = makevariant(LUA_TNIL, 0)  // "real" nil
LUA_VEMPTY    = makevariant(LUA_TNIL, 1)  // empty table slot
LUA_VABSTKEY  = makevariant(LUA_TNIL, 2)  // absent key marker
LUA_VNOTABLE  = makevariant(LUA_TNIL, 3)  // "not a table" marker (5.5 new)
```

#### lua_is* Functions — Coercion Rules

| Function | Line | Coercive? | Logic |
|----------|------|-----------|-------|
| `lua_iscfunction` | 295 | No | `ttislcf(o) \|\| ttisCclosure(o)` |
| `lua_isinteger` | 301 | **No** | Exact integer tag check |
| `lua_isnumber` | 307 | **Yes** | True for numbers AND convertible strings |
| `lua_isstring` | 314 | **Yes** | True for strings AND numbers |
| `lua_isuserdata` | 320 | No | Both full and light userdata |
| `lua_isfunction` | macro | No | `lua_type == LUA_TFUNCTION` |
| `lua_istable` | macro | No | `lua_type == LUA_TTABLE` |
| `lua_isnil` | macro | No | `lua_type == LUA_TNIL` |
| `lua_isnoneornil` | macro | No | `lua_type(L, n) <= 0` (catches TNONE=-1 and TNIL=0) |

**⚠️ Go trap**: `lua_isnumber` and `lua_isstring` are COERCIVE — they return true for convertible types. `lua_isinteger` is NOT coercive.

#### lua_to* Functions — Conversion

| Function | Line | Returns | Coercion? | Side Effects? |
|----------|------|---------|-----------|---------------|
| `lua_tonumberx` | 389 | `lua_Number` | Yes — converts strings | None |
| `lua_tointegerx` | 399 | `lua_Integer` | Yes — converts strings/floats | None |
| `lua_toboolean` | 409 | int (0/1) | N/A | None. `false` and `nil` → 0, everything else → 1 |
| `lua_tolstring` | 415 | `const char*` | **Yes — MUTATING** | Converts numbers to strings **in-place** on the stack. Triggers GC check |
| `lua_tocfunction` | 455 | `lua_CFunction` | No | None. Returns NULL for non-C-functions |
| `lua_touserdata` | 473 | `void*` | No | None |
| `lua_tothread` | 479 | `lua_State*` | No | None |
| `lua_topointer` | 492 | `const void*` | No | None. Works on any GC object |

**⚠️ Go trap for `lua_tolstring`**: Modifies the stack value in-place when converting a number to string. After conversion, the value at that index IS a string. The GC check can relocate the stack, so the code re-fetches `index2value` (line 427).

#### `lua_rawlen(L, idx)` (line 437)

Returns raw length without metamethods:
- Short string → `tsvalue(o)->shrlen`
- Long string → `tsvalue(o)->u.lnglen`
- Userdata → `uvalue(o)->len`
- Table → `luaH_getn(L, hvalue(o))` (the array length algorithm)
- Default → 0

### 1.5 Table Access Functions

#### Metamethod-Invoking (GET)

| Function | Line | Stack Effect | Metamethod |
|----------|------|-------------|------------|
| `lua_gettable` | 707 | Pops key, pushes value | `__index` via `luaV_finishget` |
| `lua_getfield` | 721 | Pushes value (key = C string arg) | `__index` |
| `lua_geti` | 727 | Pushes value (key = integer arg) | `__index` |
| `lua_getglobal` | 699 | Pushes `_G[name]` | `__index` on global table |

All use fast-path: `luaV_fastget` → if empty → `luaV_finishget` (metamethod lookup).

#### Raw GET — No Metamethods

| Function | Line | Stack Effect |
|----------|------|-------------|
| `lua_rawget` | 760 | Pops key, pushes value. Uses `luaH_get` |
| `lua_rawgeti` | 772 | Pushes value (integer key). Uses `luaH_fastgeti` |
| `lua_rawgetp` | 782 | Pushes value (light userdata key) |

#### Metamethod-Invoking (SET)

| Function | Line | Stack Effect | Metamethod |
|----------|------|-------------|------------|
| `lua_settable` | 886 | Pops key+value | `__newindex` via `luaV_finishset` |
| `lua_setfield` | 902 | Pops value (key = C string arg) | `__newindex` |
| `lua_seti` | 908 | Pops value (key = integer arg) | `__newindex` |
| `lua_setglobal` | 878 | Pops value, sets `_G[name]` | `__newindex` on global table |

#### Raw SET — No Metamethods

| Function | Line | Stack Effect |
|----------|------|-------------|
| `lua_rawset` | 940 | Pops key+value. Uses `luaH_set` |
| `lua_rawseti` | 952 | Pops value (integer key). Uses `luaH_setint` |
| `lua_rawsetp` | 945 | Pops value (light userdata key) |

**⚠️ Note**: Even `aux_rawset` (line 933) calls `invalidateTMcache(t)` and triggers GC barrier.

#### Table Creation & Metatable

| Function | Line | Notes |
|----------|------|-------|
| `lua_createtable` | 792 | Creates table with pre-sized array/hash. `lua_newtable` = `createtable(0,0)` |
| `lua_getmetatable` | 805 | For tables/userdata: object's own MT. For others: `G(L)->mt[ttype]`. Returns 0 if none |
| `lua_setmetatable` | 964 | Pops MT from top. Works on tables, userdata, and per-type MTs. Triggers GC barrier |

### 1.6 Call/Load Functions

#### `lua_callk(L, nargs, nresults, ctx, k)` (line 1037)

Stack before: `[... func arg1 arg2 ... argN]`
Stack after: `[... res1 res2 ... resNR]`

- `lua_call(L, na, nr)` = `lua_callk(L, na, nr, 0, NULL)`
- `LUA_MULTRET` = -1: keep all results
- If continuation `k != NULL` and yieldable: saves continuation, calls `luaD_call`
- Otherwise: `luaD_callnoyield`
- After: `adjustresults(L, nresults)` adjusts frame top

#### `lua_pcallk(L, nargs, nresults, errfunc, ctx, k)` (line 1076)

Protected call. `errfunc` = stack index to message handler (0 = no handler).

Key flow:
1. If `errfunc != 0`: converts to stack offset via `savestack` (because stack can move during call)
2. Without continuation: `luaD_pcall(L, f_call, &c, savestack(L, c.func), func)`
3. With continuation: saves info in `ci`, sets `CIST_YPCALL`, calls `luaD_call`
4. Returns `APIstatus(status)`

**⚠️ Go trap**: The error handler index is converted to a stack OFFSET (`ptrdiff_t`), not stored as an index. You must do the same — save an offset, not a pointer/index.

On error: stack is restored to saved position, error object replaces function+args.

#### `lua_load(L, reader, data, chunkname, mode)` (line 1120)

Parses/loads a chunk. On success:
1. Pushes compiled Lua closure
2. Sets first upvalue to global table: `f->upvals[0] = _G` (line 1135)

**Go insight**: The first upvalue of any loaded chunk is `_ENV`. `lua_load` automatically sets it to `_G`. This is how `load()` and `dofile()` get access to globals.

### 1.7 Other Important Functions

| Function | Line | Notes |
|----------|------|-------|
| `lua_error` | 1257 | Takes error from top. Special case for `memerrmsg`. Never returns |
| `lua_next` | 1272 | Table traversal. Pops key, pushes next key+value. Returns 0 at end |
| `lua_concat` | 1299 | Concatenates `n` values at top. `n==0` → empty string. Uses `luaV_concat` |
| `lua_len` | 1314 | Pushes length. **Invokes `__len` metamethod** |
| `lua_arith` | 333 | Binary: pops 2, pushes 1. Unary: pops 1, pushes 1. Can invoke metamethods |
| `lua_compare` | 349 | `LUA_OPEQ`/`LUA_OPLT`/`LUA_OPLE`. Can invoke metamethods. Returns 0 if invalid index |
| `lua_gc` | 1170 | Variadic GC control. Returns -1 if GC internally stopped |
| `lua_newuserdatauv` | 1358 | Creates full userdata with `nuvalue` user values |
| `lua_getupvalue` | 1399 | Pushes upvalue value, returns name |
| `lua_setupvalue` | 1413 | Pops value, sets upvalue, returns name |
| `lua_upvalueid` | 1446 | Returns opaque pointer for upvalue identity |
| `lua_upvaluejoin` | 1468 | Makes two closures share same upvalue |

---

## 2. lauxlib.c — Auxiliary Library

The auxiliary library (`lauxlib.c`, 1202 lines) provides higher-level functions built on the C API. Any function here could be written as an application function — it uses ONLY the public API.

### 2.1 Error Formatting

#### `luaL_where(L, level)` (line 220)

Pushes `"source:line: "` string identifying the call point at stack `level`. Uses `lua_getstack` + `lua_getinfo("Sl")`. If no line info available, pushes empty string.

#### `luaL_error(L, fmt, ...)` (line 238)

1. `luaL_where(L, 1)` — caller's location
2. `lua_pushvfstring(L, fmt, argp)` — format message
3. `lua_concat(L, 2)` — concatenate
4. `lua_error(L)` — raise (never returns)

**Error format**: `"source:line: formatted message"`

#### `luaL_argerror(L, arg, extramsg)` (line 171)

Complex logic for good error messages:
1. Gets function info via `lua_getinfo("nt")`
2. If `arg <= ar.extraargs` (5.5 feature): uses `"extra argument"` as argword
3. If method call (`namewhat == "method"`): decrements arg (skip self). If arg==0: `"calling '%s' on bad self (%s)"`
4. Tries `pushglobalfuncname` if `ar.name == NULL`
5. Final format: `"bad %s #%d to '%s' (%s)"`

#### `luaL_typeerror(L, arg, tname)` (line 197)

Gets actual type name (checks `__name` metafield first, then `luaL_typename`). Formats `"%s expected, got %s"` and delegates to `luaL_argerror`.

### 2.2 Argument Validation

**Pattern**: `check*` errors on wrong type; `opt*` returns default if nil/none, else delegates to `check*`.

| Function | Line | Accepts | Coercion? |
|----------|------|---------|-----------|
| `luaL_checktype` | 396 | Exact type match | No |
| `luaL_checkany` | 402 | Any type including nil | No (only rejects `LUA_TNONE`) |
| `luaL_checklstring` | 408 | String or number | **Yes** — `lua_tolstring` coerces numbers |
| `luaL_checknumber` | 426 | Number or convertible string | **Yes** — `lua_tonumberx` |
| `luaL_checkinteger` | 448 | Integer or convertible | **Yes** — `lua_tointegerx`. Two error msgs: "number has no integer representation" vs type error |
| `luaL_checkudata` | 351 | Userdata with matching metatable | No — exact metatable identity |
| `luaL_checkstack` | 386 | N/A | N/A — ensures stack space or errors |
| `luaL_checkoption` | 366 | String matching one of `lst[]` | For enum-like args (e.g., collectgarbage modes) |

**Optional variants**: `luaL_optlstring` (415), `luaL_optnumber` (435), `luaL_optinteger` (458). All use `luaL_opt` macro: `lua_isnoneornil(L,n) ? (d) : f(L,n)`.

**⚠️ Go trap**: `luaL_checklstring` (and `luaL_checkstring`) accepts numbers because `lua_tolstring` coerces. `luaL_checkinteger` has two distinct error messages depending on whether the value is a non-integer number vs not-a-number-at-all.

### 2.3 State Creation

#### `luaL_newstate()` (line 1184)

```c
lua_State *L = lua_newstate(luaL_alloc, NULL, luaL_makeseed(NULL));
lua_atpanic(L, &panic);
lua_setwarnf(L, warnfon, L);
```

- 5.5 adds seed parameter (3rd arg to `lua_newstate`)
- Default allocator (`luaL_alloc`, line 1049): `free`/`realloc` wrapper
- Default panic handler (line 1064): prints `"PANIC: unprotected error in call to Lua API (%s)\n"`
- Warning system: state machine with 3 states (`warnfoff`/`warnfon`/`warnfcont`). Control messages start with `'@'`: `"@off"` disables, `"@on"` enables

### 2.4 Library Registration

#### `luaL_setfuncs(L, l, nup)` (line 965)

Precondition: table at `-(nup+1)`, `nup` upvalues at top.
- Iterates `luaL_Reg` array
- If `func == NULL`: pushes `false` (placeholder)
- Else: copies upvalues, creates closure with `lua_pushcclosure`, sets field
- Finally pops upvalues

#### `luaL_newlib` macro

```c
luaL_checkversion(L);
luaL_newlibtable(L, l);     // lua_createtable pre-sized for array length
luaL_setfuncs(L, l, 0);     // register with 0 upvalues
```

#### `luaL_requiref(L, modname, openf, glb)` (line 1006)

Stripped-down `require`:
1. Get/create `_LOADED` table from registry
2. Check `_LOADED[modname]` — if truthy, reuse
3. If not loaded: call `openf(modname)`, store in `_LOADED`
4. If `glb`: also set `_G[modname]`
5. Leave module on stack

#### `luaL_openlibs` / Library Load Order

Via `luaL_openselectedlibs` (linit.c). Order:
1. `_G` (base), 2. `package`, 3. `coroutine`, 4. `debug`, 5. `io`, 6. `math`, 7. `os`, 8. `string`, 9. `table`, 10. `utf8`

### 2.5 Loading Functions

#### `luaL_loadfilex(L, filename, mode)` (line 808)

- `filename == NULL` → stdin, source = `"=stdin"`
- Otherwise: source = `"@filename"`
- **BOM handling** (line 779): Skips UTF-8 BOM (0xEF 0xBB 0xBF)
- **Shebang handling** (line 795): If first char is `'#'`, skips entire first line. Adds `'\n'` to preserve line numbers
- **Binary detection** (line 827): If first char == `LUA_SIGNATURE[0]`, reopens in binary mode
- Returns: `LUA_OK`, `LUA_ERRSYNTAX`, `LUA_ERRMEM`, or `LUA_ERRFILE`

#### `luaL_loadbufferx(L, buff, size, name, mode)` (line 867)

Simple wrapper: creates `LoadS` reader struct, calls `lua_load`.

#### `luaL_loadstring(L, s)` (line 876)

Calls `luaL_loadbuffer(L, s, strlen(s), s)` — **the string itself IS the chunk name**.

### 2.6 Reference System

References are integer keys into a table (usually the registry). `t[1]` is the head of a free list.

#### `luaL_ref(L, t)` (line 689)

- **Pops** value from top, stores in table `t`
- If nil → returns `LUA_REFNIL` (-1)
- If free slot available (`t[1] != 0`): reuses it, advances free list
- Otherwise: allocates at `rawlen(t) + 1`
- Returns reference integer (≥ 2, since slot 1 is the free list head)

#### `luaL_unref(L, t, ref)` (line 716)

- `t[ref] = t[1]` (point freed slot to old head)
- `t[1] = ref` (new head)
- Negative refs silently ignored

**Classic free-list pattern**: freed slots form a singly-linked list through the table itself.

### 2.7 Buffer System

Two modes: static (embedded `char[LUAL_BUFFERSIZE]` ≈ 1024 bytes) and dynamic (`UBox` userdata on stack).

#### Key Functions

| Function | Line | Notes |
|----------|------|-------|
| `luaL_buffinit` | 661 | Starts with static buffer. **Pushes placeholder onto stack** |
| `luaL_buffinitsize` | 670 | Init + pre-allocate `sz` bytes |
| `luaL_prepbuffsize` | 592 | Returns pointer to `sz` free bytes |
| `luaL_addchar` | macro | Inline fast path, calls prepbuffsize on overflow |
| `luaL_addlstring` | 597 | Add `l` bytes from `s` |
| `luaL_addstring` | 606 | Add NUL-terminated string |
| `luaL_addvalue` | 650 | **Pops** string from stack top, adds to buffer. Uses index `-2` for box |
| `luaL_pushresult` | 611 | Pushes final string. Static: `lua_pushlstring`. Dynamic: `lua_pushexternalstring` (5.5 zero-copy!). Removes placeholder |

**⚠️ Go trap**: The buffer occupies one stack slot (placeholder/UBox). Must account for this in stack index calculations. In Go, use `strings.Builder` — no need for the complex UBox mechanism.

**Growth strategy** (line 546): 1.5x growth (`newsize += newsize >> 1`), minimum `n + sz + 1`.

### 2.8 Other Utilities

| Function | Line | Notes |
|----------|------|-------|
| `luaL_newmetatable` | 317 | Creates metatable in registry with `__name` field. Returns 0 if already exists |
| `luaL_setmetatable` | 330 | Gets MT from registry, sets on top-1 |
| `luaL_testudata` | 336 | Tests userdata has correct MT (identity check). Returns pointer or NULL |
| `luaL_checkudata` | 351 | Like testudata but errors on mismatch |
| `luaL_getmetafield` | 884 | Gets field from object's metatable via `lua_rawget`. Returns `LUA_TNIL` if not found |
| `luaL_callmeta` | 900 | Gets metafield + calls it with object as arg |
| `luaL_len` | 910 | `lua_len` + convert to integer. Errors if not integer |
| `luaL_tolstring` | 922 | Priority: `__tostring` → number format → string copy → bool/nil literal → `"%s: %p"` default |
| `luaL_traceback` | 127 | Builds stack traceback. Shows first 10 + last 11 levels, skips middle. Handles tail calls |
| `luaL_gsub` | 1039 | **Plain** string substitution (NOT pattern matching). Uses `strstr` |
| `luaL_getsubtable` | 986 | Gets or creates subtable at given field |

---

## 3. lbaselib.c — Base Library

The base library (`lbaselib.c`, 559 lines) registers functions directly into the global table — unique among all libraries. 24 functions + 2 values (`_G`, `_VERSION`).

### 3.1 Registration Pattern

```c
// luaopen_base (line 547)
lua_pushglobaltable(L);           // push _G
luaL_setfuncs(L, base_funcs, 0);  // register all functions into _G
lua_pushvalue(L, -1);
lua_setfield(L, -2, "_G");        // _G._G = _G (self-reference)
lua_pushliteral(L, LUA_VERSION);
lua_setfield(L, -2, "_VERSION");   // _G._VERSION = "Lua 5.5"
```

**⚠️ Note**: `require` is NOT in lbaselib.c — it's in `loadlib.c`, registered as part of the `package` library.

### 3.2 Function Reference

#### `print(···)` — `luaB_print` (line 25)

- Uses `luaL_tolstring(L, i, &l)` for each arg — **invokes `__tostring` metamethod**
- Separator: TAB (`\t`), not space
- Ends with newline (`lua_writeline`)
- Returns 0 values
- Accepts any number of arguments including zero

#### `type(v)` — `luaB_type` (line 260)

- `luaL_argcheck(L, t != LUA_TNONE, 1, "value expected")`
- Returns type name string: `"nil"`, `"number"`, `"string"`, `"boolean"`, `"table"`, `"function"`, `"userdata"`, `"thread"`
- Does NOT distinguish integer/float — both return `"number"`

#### `next(table [, key])` — `luaB_next` (line 268)

- Arg 1: must be table (`luaL_checktype`)
- Arg 2: defaults to nil (`lua_settop(L, 2)`)
- Returns: key, value (2 values) OR nil (1 value, end of traversal)
- Traversal order: non-deterministic (hash part order)

#### `pairs(t)` — `luaB_pairs` (line 285)

- **Returns 4 values in Lua 5.5** (not 3!) — 4th is to-be-closed variable
- Checks `__pairs` metamethod first. If present, calls it with `t`, expects 4 returns
- Without metamethod: returns `next, t, nil, nil`
- `luaL_checkany(L, 1)` — accepts ANY value (not just tables)

**⚠️ Lua 5.5 change**: Returns 4 values instead of 3.

#### `ipairs(t)` — `luaB_ipairs` (line 316) + `ipairsaux` (line 304)

- Returns 3 values: iterator, state, 0
- NO `__ipairs` metamethod check in 5.5
- Iterator uses `lua_geti` which **DOES invoke `__index`** — ipairs is NOT raw!
- Stops at first nil: when `lua_geti` returns `LUA_TNIL`, returns only 1 value (stops for loop)
- Initial value is 0 (incremented to 1 on first call)

**⚠️ TRAP**: `ipairs` respects `__index` metamethods. It is NOT equivalent to raw integer access.

#### `pcall(f, ···)` — `luaB_pcall` (line 482)

Stack manipulation:
1. Push `true` (success marker)
2. Insert below function+args: `[true, f, arg1, ..., argN]`
3. Call `lua_pcallk(L, gettop-2, LUA_MULTRET, 0, ...)`

Results:
- Success: `true, result1, result2, ...`
- Error: `false, error_object`
- Error object propagated **UNCHANGED** (not converted to string)

#### `xpcall(f, msgh, ···)` — `luaB_xpcall` (line 497)

- Arg 1: function to call
- Arg 2: error handler (REQUIRED, must be function)
- Args 3+: passed to function
- `msgh = 2` (error handler at stack position 2)
- On error: handler called with error object, handler's return becomes error value
- Success: `true, results...`; Error: `false, handler_result`

#### `error(msg [, level])` — `luaB_error` (line 116)

- Level defaults to 1
- Position info ONLY added if: error is a STRING AND level > 0
- `level = 0`: no position info
- `level = 1`: current function's position
- Non-string errors passed through UNCHANGED

#### `assert(v [, message])` — `luaB_assert` (line 435)

- On success (truthy): returns ALL arguments — `assert(1, 2, 3)` → `1, 2, 3`
- On failure: uses arg 2 as error message, or default `"assertion failed!"`
- Calls `luaB_error` (adds position info if message is string)

#### `tonumber(e [, base])` — `luaB_tonumber` (line 83)

- Without base: accepts number (returns as-is) or string (parses via `lua_stringtonumber`)
- With base: arg 1 MUST be string, base 2-36
- `b_str2int` (line 61): custom parser, handles signs, skips whitespace
- **On failure: returns `false`** (via `luaL_pushfail`), NOT nil

**⚠️ 5.5 change**: Failure returns `false`, not `nil`.

#### `tostring(v)` — `luaB_tostring` (line 509)

- Uses `luaL_tolstring` which checks `__tostring` metamethod
- Always returns 1 string

#### `rawget(table, key)` — `luaB_rawget` (line 168)

- Arg 1: must be table. Arg 2: any type
- `lua_rawget(L, 1)` — NO metamethods
- Returns 1 value

#### `rawset(table, key, value)` — `luaB_rawset` (line 176)

- All 3 args required. Arg 1 must be table
- `lua_rawset(L, 1)` — NO metamethods
- **Returns the table** (1 value)

#### `rawlen(v)` — `luaB_rawlen` (line 159)

- Accepts table OR string (not just table)
- Returns integer length without metamethods

#### `rawequal(v1, v2)` — `luaB_rawequal` (line 151)

- Both args any type. Returns boolean. No metamethods.

#### `getmetatable(object)` — `luaB_getmetatable` (line 128)

- If no metatable → nil
- If metatable has `__metatable` field → returns that field's value (protection!)
- Otherwise → returns actual metatable

#### `setmetatable(table, metatable)` — `luaB_setmetatable` (line 139)

- Arg 1: must be TABLE (not any value)
- Arg 2: table or nil
- If existing MT has `__metatable` → error `"cannot change a protected metatable"`
- Returns the table

#### `select(index, ···)` — `luaB_select` (line 448)

- `select('#', ...)` → count of extra args
- `select(i, ...)` → all args from position i onwards
- Negative indices: `select(-1, a, b, c)` → `c`
- `i > n` → clamped (returns nothing). `i < 1` after adjustment → error

#### `collectgarbage([opt [, ...]])` — `luaB_collectgarbage` (line 201)

| Mode | Returns | Notes |
|------|---------|-------|
| `"collect"` (default) | integer | Full GC cycle |
| `"stop"` | integer | Stop GC |
| `"restart"` | integer | Restart GC |
| `"count"` | float | KB with fractional bytes (`GCCOUNT + GCCOUNTB/1024.0`) |
| `"step"` | boolean | True if cycle finished |
| `"isrunning"` | boolean | Is GC running? |
| `"generational"` | string | Previous mode name |
| `"incremental"` | string | Previous mode name |
| `"param"` | integer | 5.5: get/set GC parameter by name |

**⚠️ Trap**: If called inside a finalizer, `lua_gc` returns -1 → function returns `false`.

#### `load(chunk [, chunkname [, mode [, env]]])` — `luaB_load` (line 397)

- Chunk: string or function
- Chunkname: optional (default = string content or `"=(load)"`)
- Mode: `"b"` binary, `"t"` text, `"bt"` both (default)
- Env: optional environment table, set as 1st upvalue (`_ENV`)
- Function source: reader called repeatedly, must return string or nil
- Success: returns compiled function
- Error: returns `false, error_message`

#### `loadfile([filename [, mode [, env]]])` — `luaB_loadfile` (line 350)

- Filename nil → stdin
- Same mode/env semantics as `load`

#### `dofile([filename])` — `luaB_dofile` (line 425)

- Loads AND executes. Returns ALL results from chunk
- On load error: RAISES error (unlike loadfile which returns false+msg)

#### `warn(msg1 [, msg2, ...])` — `luaB_warn` (line 46)

- ALL arguments must be strings (validated upfront)
- Multi-part warning: all but last have `tocont=1`, last has `tocont=0`
- `'@'` prefix control messages handled by `lua_warning` internally

---

## 4. lstrlib.c — String Library

The string library (`lstrlib.c`, 1894 lines) is the most complex standard library and where most Go reimplementations break. The pattern matching engine (§4.2) requires particular attention.

### 4.1 Simple String Functions

#### Position Helpers

**`posrelatI(pos, len)`** (line 56): Converts Lua 1-based position to C 0-based, clipping to [1, ∞):
- `pos > 0` → `(size_t)pos`
- `pos == 0` → `1`
- `pos < -len` → `1` (clip to start)
- Otherwise → `len + pos + 1` (negative indexing: -1 = last char)

**`getendpos(L, arg, def, len)`** (line 72): Optional end position, clips to [0, len].

#### Function Reference

| Function | Lua Name | Line | Args | Returns | Notes |
|----------|----------|------|------|---------|-------|
| `str_len` | `string.len` | 40 | `(s)` | 1 int | Byte length, NOT UTF-8 chars |
| `str_sub` | `string.sub` | 85 | `(s, i [, j])` | 1 string | `j` defaults to -1. Empty if `i > j` |
| `str_reverse` | `string.reverse` | 97 | `(s)` | 1 string | Raw bytes, NOT UTF-8 aware |
| `str_lower` | `string.lower` | 109 | `(s)` | 1 string | C `tolower` per byte (locale-dependent) |
| `str_upper` | `string.upper` | 122 | `(s)` | 1 string | C `toupper` per byte |
| `str_rep` | `string.rep` | 139 | `(s, n [, sep])` | 1 string | Separator NOT after last copy. Overflow checks |
| `str_byte` | `string.byte` | 166 | `(s [, i [, j]])` | 0..n ints | **Default j = i, NOT -1**. Multi-return |
| `str_char` | `string.char` | 184 | `(···)` | 1 string | Each arg validated 0-255 |
| `str_dump` | `string.dump` | 227 | `(f [, strip])` | 1 string | Must be Lua function (not C). Returns bytecode |

**⚠️ Go traps**:
- `string.byte("hello")` returns just `104` (default j = i, not -1)
- `string.lower`/`upper` use C locale `tolower`/`toupper` — in Go, use ASCII-only for Lua compatibility
- `string.reverse` reverses raw bytes — breaks multi-byte UTF-8

#### String Arithmetic Metamethods (lines 259–349)

Strings have arithmetic metamethods (`__add`, `__sub`, `__mul`, `__mod`, `__pow`, `__div`, `__idiv`, `__unm`) that attempt string-to-number coercion. This enables `"10" + 5 == 15`.

The `__index` metamethod points to the string library table, enabling `s:upper()` method syntax.

### 4.2 Pattern Matching Engine — THE CRITICAL SECTION

This is a recursive backtracking NFA matcher. Most Go reimplementations break here.

#### 4.2.1 Constants & Data Structures

```c
#define CAP_UNFINISHED  (-1)   // capture opened but not closed
#define CAP_POSITION    (-2)   // empty "()" position capture
#define MAXCCALLS       200    // max match() recursion depth
#define L_ESC           '%'    // escape character (NOT backslash!)
#define SPECIALS  "^$*+?.([%-" // special pattern chars
#define LUA_MAXCAPTURES 32     // max captures
```

**`MatchState` struct** (lines 360–372):
```c
typedef struct MatchState {
  const char *src_init;   // start of source string
  const char *src_end;    // end of source string (past last byte)
  const char *p_end;      // end of pattern
  lua_State *L;
  int matchdepth;         // remaining recursion depth (starts MAXCCALLS, decrements)
  int level;              // number of captures (finished + unfinished)
  struct {
    const char *init;     // start of captured substring
    ptrdiff_t len;        // length, CAP_UNFINISHED (-1), or CAP_POSITION (-2)
  } capture[LUA_MAXCAPTURES];  // fixed 32 slots
} MatchState;
```

#### 4.2.2 Character Class Matching

**`match_class(c, cl)`** (line 429):

| Pattern | C Function | Matches |
|---------|-----------|---------|
| `%a` / `%A` | `isalpha` | Letters / non-letters |
| `%c` / `%C` | `iscntrl` | Control chars / non-control |
| `%d` / `%D` | `isdigit` | Digits / non-digits |
| `%g` / `%G` | `isgraph` | Printable non-space / complement |
| `%l` / `%L` | `islower` | Lowercase / non-lowercase |
| `%p` / `%P` | `ispunct` | Punctuation / non-punctuation |
| `%s` / `%S` | `isspace` | Whitespace / non-whitespace |
| `%u` / `%U` | `isupper` | Uppercase / non-uppercase |
| `%w` / `%W` | `isalnum` | Alphanumeric / non-alphanumeric |
| `%x` / `%X` | `isxdigit` | Hex digits / non-hex |
| `%z` / `%Z` | `c == 0` | NUL byte / non-NUL (deprecated but may appear in tests) |

**Negation**: Uppercase class letter = negation of lowercase (`islower(cl) ? res : !res`).
**Default**: `%!` matches literal `!` — any non-class-letter after `%` is a literal escape.
**⚠️ Go trap**: C `isalpha`/`isdigit` etc. are locale-dependent. Go should use ASCII-only.

**`matchbracketclass(c, p, ec)`** (line 449) — `[set]` and `[^set]`:
- `^` after `[` inverts match
- Handles `%x` classes inside brackets, `a-z` ranges (unsigned byte comparison), literal chars
- Range check: `(unsigned char)(p[-2]) <= c && c <= (unsigned char)(*p)`

**`singlematch(ms, s, p, ep)`** (line 472):
```c
if (s >= ms->src_end) return 0;  // past end → no match
switch (*p) {
  case '.': return 1;                          // any char (NOT past end)
  case L_ESC: return match_class(c, *(p+1));   // %x class
  case '[': return matchbracketclass(c, p, ep-1); // [set]
  default:  return (cast_uchar(*p) == c);      // literal
}
```

#### 4.2.3 `classend(p)` (line 405) — Find End of Pattern Element

- `%x` → returns `p+2`
- `[...]` → scans for `]`, handling `%` escapes. Returns past `]`
- `[^...]` → same but skips `^` first
- Other → returns `p+1`
- **Key**: `do...while` loop means `]` as first char in `[` is literal (e.g., `[]]` matches `]`)

#### 4.2.4 Quantifier Behavior

| Quantifier | Min | Max | Algorithm | Backtrack Direction |
|------------|-----|-----|-----------|-------------------|
| `*` | 0 | ∞ | `max_expand` (line 508) | Greedy → gives back one at a time |
| `+` | 1 | ∞ | Match 1 + `max_expand` | Greedy → gives back one at a time |
| `-` | 0 | ∞ | `min_expand` (line 523) | Lazy → adds one at a time |
| `?` | 0 | 1 | Try with, fallback without | Binary (one backtrack step) |
| (none) | 1 | 1 | Direct advance | No backtracking |

**`max_expand` (greedy)** (line 508):
```
Phase 1: Greedily match maximum chars → count in i
Phase 2: Try rest of pattern from longest match.
         If fails, reduce i by 1 and retry.
         Continue until i < 0 (includes trying 0 repetitions for *).
```

**`min_expand` (lazy)** (line 523):
```
Loop:
  1. Try rest of pattern with current repetition count
  2. If fails and current char matches, consume 1 more, retry
  3. Until rest matches or no more chars match
```

#### 4.2.5 `matchbalance(ms, s, p)` (line 488) — `%bxy`

Balanced matching: `p` points to `x` (open char). Source must start with `x`. Walks forward: `x` increments counter, `y` decrements. Returns past closing `y` when counter = 0. Returns NULL if unbalanced.

#### 4.2.6 Capture Functions

**`start_capture(ms, s, p, what)`** (line 536): Opens capture. `what` = `CAP_UNFINISHED` (normal) or `CAP_POSITION` (empty `()`). **Undo on failure**: decrements `level`.

**`end_capture(ms, s, p)`** (line 550): Closes innermost open capture (computes `len = s - init`). **Undo on failure**: resets `len` to `CAP_UNFINISHED`.

**`match_capture(ms, s, l)`** (line 561): Back-reference `%1`-`%9`. Byte-for-byte `memcmp`.

#### 4.2.7 THE `match()` FUNCTION (line 572) — The Heart of Pattern Matching

```c
static const char *match(MatchState *ms, const char *s, const char *p)
```

Returns: pointer past matched text (success) or NULL (failure).

**Recursion control**: `matchdepth--` at entry, `matchdepth++` at exit. Error at 0: `"pattern too complex"`.

**Tail-call optimization**: `goto init` label for cases that would be tail-recursive.

**Case-by-case logic**:

| Pattern | Lines | Action |
|---------|-------|--------|
| `(` | 578 | `()` → position capture (`CAP_POSITION`). `(...)` → normal capture (`CAP_UNFINISHED`) |
| `)` | 584 | `end_capture` — closes innermost open capture |
| `$` | 588 | **Only special when LAST char of pattern**. At end of source → success; else → NULL |
| `%b` | 596 | `matchbalance`. Consumes 4 pattern bytes. Tail-call on success |
| `%f[set]` | 602 | **Frontier pattern**: matches transition where prev char NOT in set AND curr char IN set. Prev at string start = `'\0'`. Does NOT consume source chars. Must be followed by `[` |
| `%1`-`%9` | 616 | Back-reference via `match_capture`. Tail-call on success |
| Default | 632 | `classend` to find element end, then apply quantifier logic |

**Default quantifier logic** (label `dflt`, line 632):
```
ep = classend(ms, p)

if (!singlematch(s, current_char)):
  * or ? or - → skip (accept 0 repetitions), continue with p=ep+1
  + or none   → FAIL (return NULL)

if (singlematch(s, current_char)):
  ?    → try match(s+1, ep+1), fallback to match(s, ep+1)
  +    → advance s++, then max_expand (greedy, min 1 already consumed)
  *    → max_expand (greedy, starts from 0 extra)
  -    → min_expand (lazy)
  none → advance s++, continue with p=ep (tail-call)
```

#### 4.2.8 Helper Functions

| Function | Line | Purpose |
|----------|------|---------|
| `check_capture` | 388 | Converts `'1'`-`'9'` to index 0-8. Validates range and not UNFINISHED |
| `capture_to_close` | 397 | Finds innermost open capture (for `)` handling) |
| `lmemfind` | 675 | Plain substring search (`memchr` + `memcmp`) |
| `get_onecapture` | 704 | Extracts one capture. Position capture → pushes integer |
| `push_captures` | 738 | Pushes all captures. If none defined, pushes whole match |
| `nospecials` | 749 | Checks for special chars via `strpbrk` |
| `prepstate` | 760 | Init MatchState (src_init, src_end, p_end, matchdepth=MAXCCALLS) |
| `reprepstate` | 770 | Reset for new attempt (level=0). Asserts matchdepth restored |

### 4.3 Pattern-Using Functions

#### `string.find(s, pattern [, init [, plain]])` — `str_find` (line 822)

Via `str_find_aux(L, 1)`:
1. Convert `init` to C index via `posrelatI`. If `init > len`, return fail
2. **Plain mode**: When `plain` arg is true OR pattern has no specials. Uses `lmemfind`. Returns start, end (1-based)
3. **Pattern mode**: Check for `^` anchor. Loop: try match at each position. For find: returns start, end, then captures. For match: returns only captures
4. Loop: `do { reprepstate; if (match(s1, p)) ... } while (s1++ < src_end && !anchor)` — tries through `src_end` INCLUSIVE

**Returns**: start, end, captures... OR `false` (fail)

#### `string.match(s, pattern [, init])` — `str_match` (line 827)

Via `str_find_aux(L, 0)`. Same as find but returns only captures, no positions.

#### `string.gmatch(s, pattern [, init])` — `gmatch` (line 857)

**`GMatchState` struct** (line 833):
```c
typedef struct GMatchState {
  const char *src;        // current position
  const char *p;          // pattern
  const char *lastmatch;  // end of last match (prevents empty-match loops)
  MatchState ms;
} GMatchState;
```

Stored as **userdata** upvalue. Creates closure with 3 upvalues: string, pattern, GMatchState.

**`gmatch_aux` iterator** (line 841): Loops through source. On match, checks `e != lastmatch` to prevent infinite empty-match loops. Updates `src = lastmatch = e`.

**⚠️ Go trap**: The 3 upvalues must keep string/pattern alive for GC. Empty match prevention via `lastmatch` tracking is critical.

#### `string.gsub(s, pattern, repl [, n])` — `str_gsub` (line 945)

**Replacement modes**:
- **String** (`add_s`, line 874): `%0` = whole match, `%1`-`%9` = captures, `%%` = literal `%`
- **Function** (`add_value`, line 909): Calls function with all captures as args
- **Table** (`add_value`): Indexes table with first capture (or whole match)
- **nil/false from function/table**: preserves original text (NOT empty replacement)

Default max replacements: `srcl + 1` (not unlimited).

**Empty match handling**: Same-position empty match → skip one char.

**Optimization** (line 976): If no changes made, returns original string object.

**Returns**: result string, number of replacements (2 values)

### 4.4 string.format

#### `str_format(L)` (line 1277)

Processes format string, consuming arguments for each conversion.

**Conversion specifiers**:

| Spec | Line | Flags | Value Source | Notes |
|------|------|-------|-------------|-------|
| `%c` | 1295 | `"-"` | `luaL_checkinteger` | Character code, cast to `int` |
| `%d`, `%i` | 1299 | `"-+0 "` | `luaL_checkinteger` | Signed integer |
| `%u` | 1301 | `"-0"` | `luaL_checkinteger` | Unsigned integer |
| `%o`, `%x`, `%X` | 1303 | `"-#0"` | `luaL_checkinteger` | Octal/hex |
| `%a`, `%A` | 1311 | `"-+#0 "` | `luaL_checknumber` | Hex float |
| `%f` | 1316 | `"-+#0 "` | `luaL_checknumber` | Float (uses `MAX_ITEMF` = `110 + MAX_10_EXP` buffer) |
| `%e`, `%E`, `%g`, `%G` | 1320 | `"-+#0 "` | `luaL_checknumber` | Float variants |
| `%p` | 1326 | `"-"` | `lua_topointer` | Pointer. NULL → `"(null)"` as `%s` |
| `%q` | 1334 | **NONE** | any | `addliteral()`. Error if ANY modifiers present |
| `%s` | 1339 | `"-"` | `luaL_tolstring` | Uses `__tostring` metamethod |
| `%%` | — | — | — | Literal `%`, no arg consumed |

**Width/precision**: Max 2 digits each (0-99). `MAX_FORMAT` = 32 bytes for format spec.

**`%s` special cases**:
- No modifiers → adds directly (no buffer limit)
- No precision + `len >= 100` → adds directly
- With modifiers + embedded zeros → error `"string contains zeros"`

#### `addquoted` (line 1126) — `%q` String Quoting Algorithm ⚠️ CRITICAL

```
1. Add opening "
2. For each byte (handles embedded NULs via length):
   - '"'  → \"
   - '\\' → \\
   - '\n' → \ + literal newline byte (0x5C 0x0A)
   - iscntrl(c):
     - If NEXT char is NOT digit → \N  (e.g., \0, \13)
     - If NEXT char IS digit → \NNN zero-padded (e.g., \000, \013)
   - Other → literal byte
3. Add closing "
```

**⚠️ Zero-padding prevents ambiguity**: `\0` followed by `1` would read as `\01`, so it becomes `\000` + `1`.

#### `quotefloat` (line 1155) — Float Quoting for `%q`

- `+inf` → `"1e9999"`, `-inf` → `"-1e9999"`, `NaN` → `"(0/0)"`
- Normal → hex float format, ensures `.` as radix point

#### `addliteral` (line 1179) — `%q` for All Types

- String → `addquoted()`
- Float → `quotefloat()`
- Integer → default format; `LUA_MININTEGER` uses hex (overflow avoidance)
- nil/boolean → `"nil"`, `"true"`, `"false"` via `luaL_tolstring`
- Other → error `"value has no literal form"`

### 4.5 string.pack / string.unpack / string.packsize

Binary packing/unpacking (lines 1393–1870).

#### Format Options (`getoption`, line 1477)

| Char | Kind | Size | Notes |
|------|------|------|-------|
| `b`/`B` | int/uint | `sizeof(char)` | Signed/unsigned byte |
| `h`/`H` | int/uint | `sizeof(short)` | Signed/unsigned short |
| `l`/`L` | int/uint | `sizeof(long)` | Signed/unsigned long |
| `j`/`J` | int/uint | `sizeof(lua_Integer)` | Lua integer (typically 8 bytes) |
| `T` | uint | `sizeof(size_t)` | size_t |
| `f` | float | `sizeof(float)` | C float (4 bytes) |
| `d` | double | `sizeof(double)` | C double (8 bytes) |
| `n` | number | `sizeof(lua_Number)` | Lua number (typically 8 bytes) |
| `i`/`I` | int/uint | optional (default `sizeof(int)`) | `i4` = 4-byte signed |
| `s` | string | optional prefix (default `sizeof(size_t)`) | Length-prefixed string |
| `c` | char | **REQUIRED** number | `c10` = 10-byte fixed string |
| `z` | zstr | 0 | Zero-terminated string |
| `x` | padding | 1 | One padding byte |
| `X` | paddalign | 0 | Alignment from NEXT option |
| `' '` | nop | 0 | Ignored |
| `<`/`>`/`=` | nop | 0 | Little/big/native endian |
| `!` | nop | 0 | Set max alignment |

**Key behaviors**:
- Default: native endian, maxalign=1 (no alignment)
- Byte order and alignment are **STATEFUL** — `<`, `>`, `=`, `!` affect all subsequent options
- `Kstring` (`s`): packs length prefix + string data
- `Kzstr` (`z`): checks no embedded zeros, packs string + `\0`
- `Kpadding`/`Kpaddalign`/`Knop`: don't consume an argument
- `string.packsize` errors on `s` and `z`: `"variable-length format"`
- `string.unpack` final return value: always the next position (1-based) for chaining

### 4.6 Library Registration (lines 1870–1894)

**17 functions**: `byte, char, dump, find, format, gmatch, gsub, len, lower, match, rep, reverse, sub, upper, pack, packsize, unpack`

**String metatable setup** (`createmetatable`, line 1889):
1. Create metamethod table with `stringmetamethods` (arithmetic + `__index`)
2. Push empty string, set this table as its metatable → applies to ALL strings
3. Set `metatable.__index = string_library_table`
4. Result: `s:upper()` → `__index` lookup → finds string library → finds `upper`

---

## 5. ltablib.c — Table Library

The table library (`ltablib.c`, 429 lines) provides 8 functions. All element access uses `lua_geti`/`lua_seti` (metamethod-aware), NOT raw operations.

### 5.1 Helper Infrastructure

#### TAB_* Constants (lines 28–31)
```c
#define TAB_R   1       // read — needs __index
#define TAB_W   2       // write — needs __newindex
#define TAB_L   4       // length — needs __len
#define TAB_RW  (TAB_R | TAB_W)
```

#### `checktab(L, arg, what)` (lines 47–61)

Validates argument is either a table OR has a metatable with required metamethods. If `lua_type == LUA_TTABLE` → passes immediately. Otherwise checks metatable for `__index`/`__newindex`/`__len` as needed.

**⚠️ Go trap**: Table library functions work on non-table objects with appropriate metamethods. Don't hard-check for table type.

#### `aux_getn(L, n, w)` (line 34)

Macro: validates capabilities (ORs in `TAB_L`), then calls `luaL_len` (metamethod-aware length).

### 5.2 Function Reference

#### `table.create(sizeseq [, sizerest])` — `tcreate` (line 64) — **NEW in 5.5**

Pre-allocates table with specified array and hash sizes. Both capped at `INT_MAX`. Returns empty table.

#### `table.insert(list, [pos,] value)` — `tinsert` (line 74)

**Two forms** (determined by `lua_gettop`):
- **2 args**: `table.insert(t, value)` — appends at end (no shifting)
- **3 args**: `table.insert(t, pos, value)` — shifts elements up, inserts at `pos`
- Other arg count → error `"wrong number of arguments to 'insert'"`

Position validation: `(lua_Unsigned)pos - 1u < (lua_Unsigned)e` — unsigned trick checks `pos ∈ [1, e]`.

Shifting uses `lua_geti`/`lua_seti` — **metamethod-aware**.

Returns: 0 values.

#### `table.remove(list [, pos])` — `tremove` (line 104)

- Default `pos` = last element (`size`)
- Pushes removed value as return
- Shifts elements down, sets last slot to nil
- All via `lua_geti`/`lua_seti` — metamethod-aware

Returns: 1 value (removed element).

#### `table.move(a1, f, e, t [, a2])` — `tmove` (line 128)

Copies `a1[f..e]` to `a2[t..t+(e-f)]`. Default `a2 = a1`.

**Copy direction**: Forward when no overlap or different tables. Backward when destination overlaps source (prevents overwriting). Overlap detection uses `lua_compare(LUA_OPEQ)` which invokes `__eq`.

Returns: 1 value (destination table).

#### `table.concat(list [, sep [, i [, j]]])` — `tconcat` (line 169)

- Separator defaults to `""`, start defaults to 1, end defaults to `#list`
- Uses `luaL_Buffer` for building
- `addfield` helper (line 160): `lua_geti` + validates result is string or number
- Error on non-string element: `"invalid value (%s) at index %I in table for 'concat'"`

Returns: 1 string.

**⚠️ Note**: `lua_isstring` returns true for numbers (coercible).

#### `table.pack(···)` — `tpack` (line 194)

```c
int n = lua_gettop(L);
lua_createtable(L, n, 1);     // n array + 1 hash (for "n" field)
lua_insert(L, 1);              // put table below args
for (i = n; i >= 1; i--)
    lua_seti(L, 1, i);         // assign in reverse (seti pops)
lua_pushinteger(L, n);
lua_setfield(L, 1, "n");       // t.n = count
```

**⚠️ Key**: `table.pack(nil, nil, nil)` returns `{n=3}` — the `n` field counts ALL arguments including nils.

Returns: 1 table.

#### `table.unpack(list [, i [, j]])` — `tunpack` (line 207)

- Default range: `1` to `#list`
- Stack check: `lua_checkstack` before pushing
- Overflow check: `n >= INT_MAX`
- Loop pushes elements, last element pushed separately (overflow avoidance)

**⚠️ Go trap**: Default end is `#list` (length operator), NOT `t.n`. So `table.unpack({1,nil,3})` returns only `1`. To unpack all of a `table.pack` result, use `table.unpack(t, 1, t.n)`.

Returns: 0..n values.

### 5.3 table.sort — Quicksort Implementation (lines 228–407)

#### Sort Entry Point — `sort(L)` (line 397)

```c
lua_Integer n = aux_getn(L, 1, TAB_RW);
if (n > 1) {
    luaL_argcheck(L, n < INT_MAX, 1, "array too big");
    if (!lua_isnoneornil(L, 2))
        luaL_checktype(L, 2, LUA_TFUNCTION);
    lua_settop(L, 2);    // normalize: [table, comp_or_nil]
    auxsort(L, 1, (IdxT)n, 0);
}
return 0;  // in-place, no return value
```

#### Comparison — `sort_comp(L, a, b)` (line 274)

- **No comparison function** (slot 2 is nil): `lua_compare(L, a, b, LUA_OPLT)` — invokes `__lt` metamethod
- **Custom function**: pushes function + args (with index adjustments for stack growth), calls, interprets result via `lua_toboolean`

**⚠️ Default comparison uses `__lt` metamethod — NOT raw comparison.**

#### Core Algorithm — `auxsort(L, lo, up, rnd)` (line 344)

Recursive quicksort (Sedgewick variant) with tail-call optimization:

```
auxsort(L, lo, up, rnd):
  while (lo < up):
    1. Sort endpoints a[lo], a[up] — swap if needed
    2. If up - lo == 1: return (2 elements, done)
    3. Choose pivot: middle element, or randomized if imbalanced
    4. Median-of-three: sort a[lo], a[pivot], a[up]
    5. If up - lo == 2: return (3 elements, done)
    6. Move pivot to up-1, partition [lo+1..up-2]
    7. Partition via partition(L, lo, up)
    8. Recurse on SMALLER half, loop on larger (O(log n) stack depth)
    9. If imbalanced (remaining/128 > partition_size): enable randomized pivots
```

#### Partition — `partition(L, lo, up)` (line 297)

Hoare partition (bidirectional scanning):
- Scans right for `a[i] >= pivot`, left for `a[j] <= pivot`
- Swaps until pointers cross
- Error detection: inconsistent comparison raises `"invalid order function for sorting"`

#### Pivot Selection — `choosePivot(lo, up, rnd)` (line 333)

Chooses pivot in middle 50% of array: `[lo + range/4, up - range/4]`. Randomization XORs `rnd` with endpoints.

#### Sort Properties

| Property | Value |
|----------|-------|
| Algorithm | Quicksort (Sedgewick variant) |
| Pivot selection | Median-of-three, randomization on imbalance |
| Partition | Hoare (bidirectional) |
| Tail-call opt | Yes — recurse smaller, loop larger |
| Stability | **NOT stable** |
| Comparison | `__lt` metamethod (default) or custom function |
| Max recursion | O(log n) |
| Array limit | `INT_MAX` |

### 5.4 Metamethod Usage Summary

| Function | `__index` | `__newindex` | `__len` | Other |
|----------|-----------|-------------|---------|-------|
| `table.insert` | ✅ (shift reads) | ✅ (shift writes) | ✅ | — |
| `table.remove` | ✅ (shift reads) | ✅ (shift writes + nil) | ✅ | — |
| `table.move` | ✅ (source) | ✅ (dest) | — | `__eq` (overlap) |
| `table.concat` | ✅ (reads) | — | ✅ | — |
| `table.sort` | ✅ (reads) | ✅ (writes) | ✅ | `__lt` (default comp) |
| `table.pack` | — | ✅ (`lua_seti`) | — | — |
| `table.unpack` | ✅ (reads) | — | ✅ (default end) | — |
| `table.create` | — | — | — | — |

**Critical**: ALL element access uses `lua_geti`/`lua_seti`, NEVER `lua_rawgeti`/`lua_rawseti`. The `rawXXX` functions in lbaselib.c are the ones that bypass metamethods.

---

## 6. ldblib.c — Debug Library

The debug library (`ldblib.c`, 477 lines) provides 16 functions for introspection. Uses `luaL_newlib` for registration (standard pattern).

### 6.1 Common Pattern: `getthread` (line 95)

Many debug functions accept an optional thread as first argument:
```c
if (lua_isthread(L, 1)) { *arg = 1; return lua_tothread(L, 1); }
else { *arg = 0; return L; }
```

`*arg` is an offset — subsequent argument indices add this offset.

### 6.2 Function Reference

| Function | Lua Name | Line | Args | Returns | Key Notes |
|----------|----------|------|------|---------|-----------|
| `db_getregistry` | `debug.getregistry` | 42 | none | 1 (registry table) | Simple `lua_pushvalue(L, LUA_REGISTRYINDEX)` |
| `db_getmetatable` | `debug.getmetatable` | 48 | `(v)` | 1 (MT or nil) | **Bypasses `__metatable` protection** |
| `db_setmetatable` | `debug.setmetatable` | 57 | `(v, mt)` | 1 (v) | **Bypasses `__metatable`**, works on ANY value (not just tables) |
| `db_getuservalue` | `debug.getuservalue` | 66 | `(u [, n])` | 1-2 | Returns value, true (success) or false |
| `db_setuservalue` | `debug.setuservalue` | 78 | `(u, v [, n])` | 1 | Returns u on success, false on invalid index |
| `db_getinfo` | `debug.getinfo` | 150 | `([thread,] f [, what])` | 1 (table) | Options: S, l, u, n, r, t, L, f. Default `"flnSrtu"` |
| `db_getlocal` | `debug.getlocal` | 206 | `([thread,] f, local)` | 0-2 | name, value or false |
| `db_setlocal` | `debug.setlocal` | 237 | `([thread,] level, local, value)` | 0-1 | name or nil |
| `db_getupvalue` | `debug.getupvalue` | 273 | `(f, up)` | 0-2 | name, value or nothing |
| `db_setupvalue` | `debug.setupvalue` | 278 | `(f, up, value)` | 0-1 | name or nothing |
| `db_upvalueid` | `debug.upvalueid` | 301 | `(f, n)` | 1 | Light userdata (pointer identity) or false |
| `db_upvaluejoin` | `debug.upvaluejoin` | 311 | `(f1, n1, f2, n2)` | 0 | Both must be Lua functions |
| `db_sethook` | `debug.sethook` | 368 | `([thread,] hook, mask [, count])` | 0 | Mask: `"c"` call, `"r"` return, `"l"` line |
| `db_gethook` | `debug.gethook` | 398 | `([thread])` | 1-3 | hook_func, mask_string, count |
| `db_traceback` | `debug.traceback` | 438 | `([thread,] [msg [, level]])` | 1 | Non-string msg returned as-is |
| `db_debug` | `debug.debug` | 423 | none | 0 | Interactive console, exits on "cont" |

### 6.3 Key Differences from Base Library

| Feature | Base Library | Debug Library |
|---------|-------------|---------------|
| `getmetatable` | Returns `__metatable` if set | Returns **real** metatable (bypasses protection) |
| `setmetatable` | Only on tables, checks `__metatable` | Works on **any value**, no protection check |
| Hook storage | N/A | Uses `registry["_HOOKKEY"][thread]` with weak keys |

### 6.4 `debug.getinfo` Options Detail (lines 150–203)

| Option | Fields Populated | Lines |
|--------|-----------------|-------|
| `'S'` | source, short_src, linedefined, lastlinedefined, what | 171–178 |
| `'l'` | currentline | 179–180 |
| `'u'` | nups, nparams, isvararg | 181–185 |
| `'n'` | name, namewhat | 186–189 |
| `'r'` | ftransfer, ntransfer | 190–193 |
| `'t'` | istailcall, extraargs | 194–197 |
| `'L'` | activelines (table) | 198–199 |
| `'f'` | func (the function itself) | 200–201 |

`'>'` option in user input is explicitly rejected (line 156) — it's only for internal use.

---

## 7. Key Patterns — Library Registration & Argument Validation

### 7.1 Library Registration Flow

```
1. luaL_newstate()           — create state + set panic + warnings
2. luaL_openlibs(L)          — calls luaL_openselectedlibs(L, ~0, 0)
3.   → for each lib:
4.     luaL_requiref(L, name, openf, 1)
5.       → openf(L):
6.         luaL_newlib(L, funcs)    — creates table + registers functions
7.         return 1                 — leaves table on stack
8.       → stores in _LOADED[name]
9.       → stores in _G[name] (if glb=1)
```

**Base library is special**: Registers directly into `_G` (not a sub-table). Uses `lua_pushglobaltable` + `luaL_setfuncs`.

**String library is special**: Also sets up string metatable so `s:method()` works via `__index`.

### 7.2 The `luaL_Reg` Array Pattern

Every library defines a NULL-terminated array of `{name, function}` pairs:

```c
static const luaL_Reg mylib[] = {
  {"funcname", c_function},
  {"other",    c_other},
  {NULL, NULL}  // sentinel
};
```

If `func == NULL`, `luaL_setfuncs` pushes `false` as a placeholder.

### 7.3 Argument Validation Patterns

Standard C library functions follow a consistent pattern:

```c
static int myfunction(lua_State *L) {
    // 1. Validate and extract arguments
    const char *s = luaL_checkstring(L, 1);      // required string (coerces numbers)
    lua_Integer n = luaL_optinteger(L, 2, 0);     // optional int, default 0
    luaL_checktype(L, 3, LUA_TTABLE);             // required exact type

    // 2. Do work...

    // 3. Push results
    lua_pushstring(L, result);
    return 1;  // number of return values
}
```

**Validation hierarchy**:
1. `luaL_checktype(L, n, t)` — exact type match, no coercion
2. `luaL_checkany(L, n)` — just checks argument exists (not `LUA_TNONE`)
3. `luaL_checkstring(L, n)` — accepts strings AND numbers (coerces via `lua_tolstring`)
4. `luaL_checknumber(L, n)` — accepts numbers AND convertible strings
5. `luaL_checkinteger(L, n)` — accepts integers AND convertible values. Two error messages: "number has no integer representation" (for floats like 3.5) vs type error
6. `luaL_checkudata(L, n, tname)` — exact metatable identity check
7. `luaL_checkoption(L, n, def, lst)` — string matching enum array

**Optional variants**: `luaL_opt*` return default if `lua_isnoneornil`, else delegate to `check*`.

### 7.4 Result Pushing Patterns

| Pattern | Example | Functions |
|---------|---------|-----------|
| Return 0 | `return 0;` | `print`, `table.insert`, `table.sort` |
| Return 1 value | `lua_pushX(L, v); return 1;` | `type`, `tostring`, most functions |
| Return table itself | `return 1;` (table at slot 1) | `rawset`, `setmetatable` |
| Return multi-value | `return n;` | `select`, `table.unpack`, `pcall` |
| Return success/fail | `true, results...` or `false, errmsg` | `pcall`, `xpcall`, `load` |
| Failure sentinel | `luaL_pushfail(L); return 1;` | `tonumber` failure (pushes `false` in 5.5) |
| Iterator protocol | Return iterator, state, initial | `pairs` (4 values in 5.5), `ipairs` (3 values) |

### 7.5 Error Raising from C

| Function | Use Case |
|----------|----------|
| `luaL_error(L, fmt, ...)` | General error with location prefix |
| `luaL_argerror(L, arg, msg)` | Bad argument with function name |
| `luaL_typeerror(L, arg, expected)` | Type mismatch: "X expected, got Y" |
| `luaL_argcheck(L, cond, arg, msg)` | Conditional argument check |
| `luaL_argexpected(L, cond, arg, expected)` | Conditional with expected type name |
| `lua_error(L)` | Raw error raise (pops error object from top) |

### 7.6 Buffer Usage Pattern

```c
luaL_Buffer b;
luaL_buffinit(L, &b);          // pushes placeholder on stack
luaL_addstring(&b, "hello ");  // add literal
luaL_addvalue(&b);             // add string from top of stack (pops it)
luaL_addchar(&b, '!');         // add single char
luaL_pushresult(&b);           // push final string, remove placeholder
// Stack: [..., result_string]
```

**⚠️ The buffer occupies one stack slot**. Account for this in index calculations.

---

## 8. Go Implementation Guide

### 8.1 The Stack Model in Go

The C API's stack is a contiguous `TValue` array. In Go:

```go
type LuaState struct {
    stack    []Value        // the value stack
    top      int            // index of first free slot
    ci       *CallInfo      // current call frame
    // ...
}

type CallInfo struct {
    funcIdx  int            // stack index of the function
    top      int            // max stack for this frame
    // ...
}
```

**Index resolution** (the foundation — get this right first):
```go
func (L *LuaState) index2value(idx int) *Value {
    if idx > 0 {
        // Positive: relative to frame base
        abs := L.ci.funcIdx + idx
        if abs >= L.top { return &nilValue }  // above top → nil sentinel
        return &L.stack[abs]
    } else if idx > LUA_REGISTRYINDEX {
        // Negative: relative to top
        return &L.stack[L.top + idx]
    } else if idx == LUA_REGISTRYINDEX {
        return &L.registry
    } else {
        // Upvalue access
        upIdx := LUA_REGISTRYINDEX - idx - 1
        return L.getUpvalue(upIdx)
    }
}
```

### 8.2 Registering Go Functions as Lua-Callable

**Light function** (no upvalues):
```go
type LuaCFunction func(L *LuaState) int

func (L *LuaState) PushCFunction(fn LuaCFunction) {
    // Store as light function value (no closure allocation)
    L.push(lightCFuncValue(fn))
}
```

**Closure with upvalues**:
```go
func (L *LuaState) PushCClosure(fn LuaCFunction, nup int) {
    if nup == 0 {
        L.PushCFunction(fn)  // light function optimization
        return
    }
    // Pop nup values from stack as upvalues
    upvalues := make([]Value, nup)
    for i := nup - 1; i >= 0; i-- {
        upvalues[i] = L.stack[L.top - nup + i]
    }
    L.top -= nup
    L.push(cClosureValue(fn, upvalues))
}
```

### 8.3 Argument Validation in Go

```go
// Pattern: mirror the luaL_check*/luaL_opt* functions
func (L *LuaState) CheckString(arg int) string {
    s, ok := L.ToString(arg)  // coerces numbers
    if !ok {
        L.TagError(arg, LUA_TSTRING)
    }
    return s
}

func (L *LuaState) OptInteger(arg int, def int64) int64 {
    if L.IsNoneOrNil(arg) {
        return def
    }
    return L.CheckInteger(arg)
}

func (L *LuaState) CheckInteger(arg int) int64 {
    val, ok := L.ToIntegerX(arg)
    if !ok {
        if L.IsNumber(arg) {
            L.ArgError(arg, "number has no integer representation")
        }
        L.TagError(arg, LUA_TNUMBER)
    }
    return val
}
```

### 8.4 Library Registration in Go

```go
type LuaReg struct {
    Name string
    Func LuaCFunction
}

func (L *LuaState) NewLib(funcs []LuaReg) {
    L.CreateTable(0, len(funcs))
    L.SetFuncs(funcs, 0)
}

func (L *LuaState) SetFuncs(funcs []LuaReg, nup int) {
    for _, reg := range funcs {
        if reg.Func == nil {
            L.PushBoolean(false)  // placeholder
        } else {
            // Copy upvalues for each function
            for i := 0; i < nup; i++ {
                L.PushValue(-(nup + 1))
            }
            L.PushCClosure(reg.Func, nup)
        }
        L.SetField(-(nup + 2), reg.Name)
    }
    L.Pop(nup)  // remove upvalues
}
```

### 8.5 Pattern Matching Engine in Go

The most critical piece. Architecture:

```go
type MatchState struct {
    src       string    // source string
    srcPos    int       // current position in source
    pattern   string    // pattern string
    L         *LuaState
    matchDepth int      // remaining recursion (starts at MAXCCALLS=200)
    level     int       // number of captures
    captures  [32]struct {
        init int        // start position
        len  int        // length, CAP_UNFINISHED(-1), or CAP_POSITION(-2)
    }
}

func (ms *MatchState) match(s, p int) int {
    ms.matchDepth--
    if ms.matchDepth == 0 {
        ms.L.Error("pattern too complex")
    }
    defer func() { ms.matchDepth++ }()

    for {  // replaces "goto init" tail calls
        if p >= len(ms.pattern) {
            return s  // end of pattern = success
        }
        switch ms.pattern[p] {
        case '(':
            // position capture vs normal capture
        case ')':
            // end_capture
        case '$':
            if p+1 == len(ms.pattern) {
                // anchor: only special at pattern END
                if s == len(ms.src) { return s }
                return -1
            }
            // else: literal '$', fall through to default
        case L_ESC:
            // handle %b, %f, %1-%9, %class
        default:
            // classend + quantifier logic
        }
    }
}
```

**Critical implementation details**:
1. Capture undo-on-failure: both `startCapture` and `endCapture` must restore state if recursive match fails
2. `$` only special at pattern END — otherwise literal
3. `%f[set]` frontier does NOT consume source chars
4. Empty match prevention: track `lastmatch` in gmatch/gsub
5. `singlematch` returns false past end-of-string (quantifiers naturally stop)
6. Position captures store `CAP_POSITION`, push 1-based integer

### 8.6 Buffer System in Go

No need for the complex C UBox mechanism. Use `strings.Builder`:

```go
// C: luaL_buffinit + luaL_addstring + luaL_pushresult
// Go equivalent:
var b strings.Builder
b.WriteString("hello ")
b.WriteString(someValue)
L.PushString(b.String())
```

**⚠️ But**: The C buffer occupies a stack slot. If your Go buffer doesn't, stack indices may differ from C. Be careful when porting code that mixes buffer operations with stack manipulation.

### 8.7 Reference System in Go

The C reference system uses a free-list in a table. In Go:

```go
type RefTable struct {
    refs     map[int]Value
    freeList []int
    nextRef  int  // starts at 2 (slot 1 is free-list head in C)
}

func (rt *RefTable) Ref(v Value) int {
    if v.IsNil() { return LUA_REFNIL }
    var ref int
    if len(rt.freeList) > 0 {
        ref = rt.freeList[len(rt.freeList)-1]
        rt.freeList = rt.freeList[:len(rt.freeList)-1]
    } else {
        ref = rt.nextRef
        rt.nextRef++
    }
    rt.refs[ref] = v
    return ref
}
```

---

## 9. Edge Cases & Traps

### 9.1 string.format Edge Cases

| Case | Behavior |
|------|----------|
| `%q` with embedded NUL | Handled via length (not strlen). Escapes as `\0` or `\000` |
| `%q` NUL followed by digit | Zero-padded: `\000` + digit (prevents ambiguity) |
| `%q` with newlines | Escaped as `\` + literal newline byte (NOT `\n`) |
| `%q` with non-printable | `\N` or `\NNN` depending on whether next char is digit |
| `%q` with modifiers | **ERROR**: `%q` rejects ALL width/precision/flags |
| `%q` on float | `+inf` → `"1e9999"`, `-inf` → `"-1e9999"`, NaN → `"(0/0)"` |
| `%q` on `LUA_MININTEGER` | Uses hex format (avoids signed overflow in decimal) |
| `%p` on NULL | Formats as `"(null)"` using `%s` |
| `%s` with embedded zeros + modifiers | **ERROR**: `"string contains zeros"` |
| `%s` without modifiers | Adds directly, no buffer limit, handles embedded zeros |
| Width/precision | Max 2 digits each (0-99) |

### 9.2 Pattern Matching Gotchas

| Gotcha | Detail |
|--------|--------|
| `$` is only special at pattern END | `"$abc"` matches literal `$` followed by `abc` |
| `.` does NOT match past end | `singlematch` returns 0 if `s >= src_end` |
| `%z` matches NUL byte | Deprecated but may appear in test suite |
| `%f[set]` frontier | Does NOT consume chars. Checks boundary: prev NOT in set AND curr IN set. At string start, prev = `\0` |
| `%bxy` balanced match | Walks forward counting `x` and `y`. Returns past closing `y` |
| `]` as first char in `[` | Literal: `[]]` matches `]` (due to `do...while` in `classend`) |
| Empty captures `()` | Position capture — pushes 1-based integer, not string |
| Back-references `%1`-`%9` | Byte-for-byte comparison (`memcmp`) |
| `MAXCCALLS` = 200 | Recursion limit. Complex patterns with nested quantifiers can hit this |
| `LUA_MAXCAPTURES` = 32 | Fixed limit on number of captures |

### 9.3 pairs vs ipairs Semantics

| Feature | `pairs(t)` | `ipairs(t)` |
|---------|-----------|------------|
| Returns | 4 values (5.5!) | 3 values |
| Iterator | `next` | `ipairsaux` |
| Traversal | All keys (hash+array) | Integer keys 1, 2, 3, ... |
| Stop condition | No more keys | First nil value |
| Metamethod check | `__pairs` | None (`__ipairs` removed in 5.5) |
| Element access | `lua_next` (raw) | `lua_geti` (**invokes `__index`**) |
| Order | Non-deterministic | Sequential |
| Accepts non-table | Yes (if has `__pairs`) | Yes (`luaL_checkany`) |

**⚠️ Critical**: `ipairs` uses `lua_geti` which invokes `__index`. It is NOT raw access. But `pairs` uses `lua_next` which IS raw table traversal.

### 9.4 table.sort Stability

`table.sort` is **NOT stable** — equal elements may be reordered. The quicksort implementation uses Hoare partition which does not preserve relative order of equal elements.

The comparison function must implement a **strict weak ordering** (irreflexive, asymmetric, transitive). If the comparison function is inconsistent, the sort detects this and raises `"invalid order function for sorting"`.

### 9.5 collectgarbage Modes

| Mode | Lua 5.4 | Lua 5.5 |
|------|---------|---------|
| `"collect"` | ✅ | ✅ (default) |
| `"stop"` | ✅ | ✅ |
| `"restart"` | ✅ | ✅ |
| `"count"` | ✅ (returns float KB) | ✅ |
| `"step"` | ✅ | ✅ |
| `"isrunning"` | ✅ | ✅ |
| `"generational"` | ✅ (with params) | ✅ (no inline params) |
| `"incremental"` | ✅ (with params) | ✅ (no inline params) |
| `"param"` | ❌ | ✅ **NEW** — get/set GC params by name |

5.5 `"param"` sub-parameters: `"minormul"`, `"majorminor"`, `"minormajor"`, `"pause"`, `"stepmul"`, `"stepsize"`.

### 9.6 Lua 5.5-Specific Changes

| Change | Impact |
|--------|--------|
| `pairs` returns 4 values (not 3) | 4th value is to-be-closed variable for generic `for` |
| `tonumber` failure returns `false` (not nil) | Via `luaL_pushfail` |
| `table.create` is new | Pre-allocate table sizes |
| `LUA_VNOTABLE` 4th nil variant | Affects type checking in lapi.c |
| `lua_pushboolean` uses two distinct tags | `setbtvalue` vs `setbfvalue` |
| `lua_pushexternalstring` is new | Zero-copy string creation from external buffer |
| `luaL_newstate` takes seed parameter | 3rd arg to `lua_newstate` |
| `collectgarbage("param", ...)` is new | Replaces inline params in gen/inc modes |
| `debug.getinfo` has `extraargs` field | New in 5.5, affects `luaL_argerror` |

---

## 10. Bug Pattern Guide

Common mistakes when reimplementing Lua standard libraries in Go.

### 10.1 Wrong Argument Count Handling

**Bug**: Not checking `lua_gettop` for functions with variable argument forms.

```go
// WRONG: table.insert always expects 3 args
func tableInsert(L *LuaState) int {
    t := L.CheckTable(1)
    pos := L.CheckInteger(2)
    val := L.CheckAny(3)
    // ...
}

// RIGHT: table.insert has 2-arg and 3-arg forms
func tableInsert(L *LuaState) int {
    switch L.GetTop() {
    case 2:  // append at end
        // ...
    case 3:  // insert at position
        // ...
    default:
        L.Error("wrong number of arguments to 'insert'")
    }
}
```

### 10.2 Missing Metamethod Calls in rawXXX

**Bug**: Accidentally invoking metamethods in `rawget`/`rawset`/`rawlen`/`rawequal`.

```go
// WRONG: using the metamethod-aware getter
func rawGet(L *LuaState) int {
    L.GetTable(1)  // This invokes __index!
    return 1
}

// RIGHT: use raw access
func rawGet(L *LuaState) int {
    L.RawGet(1)    // No metamethods
    return 1
}
```

**Conversely**: Table library functions (`table.insert`, `table.sort`, etc.) MUST use metamethod-aware access (`lua_geti`/`lua_seti`), not raw access.

### 10.3 pcall Error Object Propagation

**Bug**: Wrapping error objects in strings or modifying them.

```go
// WRONG: converting error to string
func pcallHandler(L *LuaState, err interface{}) {
    L.PushString(fmt.Sprint(err))  // Wraps in string!
}

// RIGHT: propagate error object unchanged
func pcallHandler(L *LuaState, err Value) {
    L.Push(err)  // Exact same object
}
```

The error object can be ANY Lua value (table, number, userdata, etc.), not just strings. `error()` only adds position info to STRING error objects.

### 10.4 String Pattern Backtracking

**Bug**: Incorrect greedy/lazy quantifier implementation.

```go
// WRONG: greedy * that doesn't backtrack
func maxExpand(ms *MatchState, s, p, ep int) int {
    count := 0
    for singlematch(ms, s+count, p, ep) { count++ }
    return ms.match(s+count, ep+1)  // Only tries max, no backtrack!
}

// RIGHT: greedy * with backtracking
func maxExpand(ms *MatchState, s, p, ep int) int {
    count := 0
    for singlematch(ms, s+count, p, ep) { count++ }
    for count >= 0 {
        res := ms.match(s+count, ep+1)
        if res >= 0 { return res }
        count--  // Give back one character
    }
    return -1
}
```

### 10.5 Capture Undo on Failure

**Bug**: Not restoring capture state when recursive match fails.

```go
// WRONG: capture state corrupted on backtrack
func (ms *MatchState) startCapture(s, p, what int) int {
    ms.captures[ms.level] = Capture{init: s, len: what}
    ms.level++
    res := ms.match(s, p)
    return res  // level not restored on failure!
}

// RIGHT: undo capture on failure
func (ms *MatchState) startCapture(s, p, what int) int {
    level := ms.level
    ms.captures[level] = Capture{init: s, len: what}
    ms.level = level + 1
    res := ms.match(s, p)
    if res < 0 {
        ms.level = level  // UNDO
    }
    return res
}
```

### 10.6 $ Only Special at Pattern End

**Bug**: Treating `$` as always special.

```go
// WRONG: $ is always an anchor
case '$':
    if s == len(ms.src) { return ms.match(s, p+1) }
    return -1

// RIGHT: $ is only special at pattern end
case '$':
    if p+1 == len(ms.pattern) {
        if s == len(ms.src) { return s }
        return -1
    }
    // Otherwise: literal '$', fall through to default handling
```

### 10.7 string.byte Default Range

**Bug**: Using wrong default for `j` parameter.

```go
// WRONG: default j = -1 (last char)
j := L.OptInteger(3, -1)

// RIGHT: default j = i (same as start)
i := L.OptInteger(2, 1)
j := L.OptInteger(3, i)  // Default j = i, not -1!
```

`string.byte("hello")` should return only `104` (one byte), not all bytes.

### 10.8 string.gsub Max Replacements

**Bug**: Unlimited replacements by default.

```go
// WRONG: unlimited default
maxReplacements := math.MaxInt64

// RIGHT: default is srclen + 1
maxReplacements := len(src) + 1
```

### 10.9 %q Not Handling Embedded NULs

**Bug**: Using Go string functions that stop at NUL.

```go
// WRONG: stops at first NUL
for _, c := range s {
    // range iterates runes, not bytes!
}

// RIGHT: iterate bytes, handle NULs explicitly
for i := 0; i < len(s); i++ {
    c := s[i]
    switch {
    case c == '"':  b.WriteString("\\\"")
    case c == '\\': b.WriteString("\\\\")
    case c == '\n': b.WriteString("\\\n")
    case c < 0x20:  // control char
        if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
            fmt.Fprintf(&b, "\\%03d", c)  // zero-padded
        } else {
            fmt.Fprintf(&b, "\\%d", c)
        }
    default:
        b.WriteByte(c)
    }
}
```

### 10.10 ipairs Using Raw Access

**Bug**: Using raw table access for ipairs.

```go
// WRONG: raw access (skips __index)
func ipairsAux(L *LuaState) int {
    i := L.CheckInteger(2) + 1
    L.PushInteger(i)
    L.RawGetI(1, i)  // WRONG — must invoke __index
    // ...
}

// RIGHT: metamethod-aware access
func ipairsAux(L *LuaState) int {
    i := L.CheckInteger(2) + 1
    L.PushInteger(i)
    typ := L.GetI(1, i)  // lua_geti — invokes __index
    if typ == LUA_TNIL {
        return 1  // stop iteration
    }
    return 2
}
```

### 10.11 pairs Return Count (5.5)

**Bug**: Returning 3 values instead of 4.

```go
// WRONG (Lua 5.4 behavior):
func luaB_pairs(L *LuaState) int {
    L.PushCFunction(next)
    L.PushValue(1)
    L.PushNil()
    return 3  // WRONG for 5.5!
}

// RIGHT (Lua 5.5):
func luaB_pairs(L *LuaState) int {
    L.PushCFunction(next)
    L.PushValue(1)
    L.PushNil()
    L.PushNil()  // to-be-closed variable
    return 4
}
```

### 10.12 lua_settop / lua_pop Triggering __close

**Bug**: Assuming `lua_pop` is a simple stack pointer decrement.

```go
// WRONG: simple decrement
func (L *LuaState) SetTop(idx int) {
    if idx >= 0 {
        for L.top < L.ci.funcIdx + 1 + idx {
            L.push(nilValue)
        }
        L.top = L.ci.funcIdx + 1 + idx
    } else {
        L.top = L.top + idx + 1
    }
}

// RIGHT: must check for to-be-closed variables
func (L *LuaState) SetTop(idx int) {
    var newTop int
    if idx >= 0 {
        newTop = L.ci.funcIdx + 1 + idx
        // Fill new slots with nil
        for i := L.top; i < newTop; i++ {
            L.stack[i] = nilValue
        }
    } else {
        newTop = L.top + idx + 1
    }
    if newTop < L.top {
        L.closeSlots(newTop)  // Check for TBC variables!
    }
    L.top = newTop
}
```

### 10.13 Error Handler Stack Offset in pcall

**Bug**: Storing error handler as stack index instead of offset.

```go
// WRONG: index can become invalid after stack reallocation
errFunc := errFuncIdx

// RIGHT: save as offset from stack base
errFuncOffset := L.saveStack(errFuncIdx)
// ... after potential stack reallocation ...
errFuncIdx = L.restoreStack(errFuncOffset)
```

### 10.14 Summary: Top 15 Reimplementation Mistakes

| # | Mistake | Correct Behavior |
|---|---------|-----------------|
| 1 | `rawget`/`rawset` invoking metamethods | Must use raw table access, NO metamethods |
| 2 | `table.*` functions using raw access | Must use `lua_geti`/`lua_seti` (metamethod-aware) |
| 3 | `pcall` wrapping error in string | Propagate error object unchanged (any type) |
| 4 | Pattern `$` always special | Only special at pattern END |
| 5 | Pattern greedy `*` without backtracking | Must try max, then give back one at a time |
| 6 | Capture state not undone on failure | Must restore `level` and `len` on backtrack |
| 7 | `string.byte` default j = -1 | Default j = i (same as start position) |
| 8 | `string.gsub` unlimited default | Default max = `srclen + 1` |
| 9 | `%q` not handling NULs/next-digit padding | Must iterate by length, zero-pad before digits |
| 10 | `ipairs` using raw access | Must use `lua_geti` (invokes `__index`) |
| 11 | `pairs` returning 3 values | Returns 4 in Lua 5.5 (to-be-closed) |
| 12 | `lua_pop` not checking TBC | Must call `luaF_close` when shrinking past TBC slots |
| 13 | `tonumber` failure returning nil | Returns `false` in 5.5 (via `luaL_pushfail`) |
| 14 | `pcall` errfunc stored as index | Must save as stack offset (stack can reallocate) |
| 15 | `error()` adding position to non-strings | Only adds `"source:line: "` prefix to STRING errors |
