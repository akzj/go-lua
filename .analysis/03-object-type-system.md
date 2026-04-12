# 03 — Object & Type System (lobject.h / lobject.c)

> **Source**: Lua 5.5.1 — `lobject.h` (864 lines), `lobject.c` (718 lines), `lua.h` (547 lines)
> **Scope**: TValue tagged union, all type tags, macro system, GCObject header, every object struct, Proto in full detail, string types, number representation, key operation pseudocode, Go mapping, edge cases.

---

## Table of Contents

1. [TValue Structure — The Tagged Union](#1-tvalue-structure--the-tagged-union)
2. [Type Tag Constants — Complete Map](#2-type-tag-constants--complete-map)
3. [Macro System — set/get/check](#3-macro-system--setgetchecktest)
4. [GCObject Header (CommonHeader)](#4-gcobject-header-commonheader)
5. [Object Structs](#5-object-structs)
6. [Proto — Full Detail](#6-proto--full-detail)
7. [String Types — Short vs Long, Interning](#7-string-types--short-vs-long-interning)
8. [Number Representation](#8-number-representation)
9. [Pseudocode for Key Operations](#9-pseudocode-for-key-operations)
10. [If I Were Building This in Go](#10-if-i-were-building-this-in-go)
11. [Edge Cases](#11-edge-cases)

---

## 1. TValue Structure — The Tagged Union

### 1.1 Value Union (`lobject.h:51–58`)

```c
typedef union Value {
  struct GCObject *gc;    /* collectable objects */
  void *p;                /* light userdata */
  lua_CFunction f;        /* light C functions */
  lua_Integer i;          /* integer numbers */
  lua_Number n;           /* float numbers */
  lu_byte ub;             /* not used, but avoids warnings */
} Value;
```

This is a **C union** — all fields share the same memory. Only one is valid at a time, determined by the tag byte.

### 1.2 TValue Struct (`lobject.h:66–69`)

```c
#define TValuefields  Value value_; lu_byte tt_

typedef struct TValue {
  TValuefields;
} TValue;
```

**Layout**: `{ Value value_; lu_byte tt_; }` — the value union followed by a single-byte type tag. On a 64-bit system, `Value` is 8 bytes (pointer-sized), `tt_` is 1 byte, so `TValue` is 16 bytes (with padding).

### 1.3 Tag Byte Encoding (`lobject.h:37–42`)

```
bits 0-3: base type (a LUA_T* constant, 0–15)
bits 4-5: variant bits (0–3)
bit 6:    collectable flag (BIT_ISCOLLECTABLE = 1<<6 = 0x40)
bit 7:    unused
```

Key macros for tag manipulation (`lobject.h:74–86`):

| Macro | Definition | Purpose |
|-------|-----------|---------|
| `rawtt(o)` | `(o)->tt_` | Raw tag byte, all bits |
| `novariant(t)` | `(t) & 0x0F` | Base type only (bits 0–3) |
| `withvariant(t)` | `(t) & 0x3F` | Type + variant (bits 0–5), strips collectable bit |
| `ttypetag(o)` | `withvariant(rawtt(o))` | Type tag with variant, no collectable bit |
| `ttype(o)` | `novariant(rawtt(o))` | Base type only |
| `makevariant(t,v)` | `(t) \| ((v) << 4)` | Combine base type + variant |
| `checktag(o,t)` | `rawtt(o) == (t)` | Exact tag match (includes collectable bit) |
| `checktype(o,t)` | `ttype(o) == (t)` | Base type match only |
| `iscollectable(o)` | `rawtt(o) & BIT_ISCOLLECTABLE` | Test collectable bit |
| `ctb(t)` | `(t) \| BIT_ISCOLLECTABLE` | Mark tag as collectable |

### 1.4 StackValue (`lobject.h:137–143`)

```c
typedef union StackValue {
  TValue val;
  struct {
    TValuefields;
    unsigned short delta;  /* distance to previous tbc variable */
  } tbclist;
} StackValue;
```

Stack slots are `StackValue`, not bare `TValue`. The `delta` field forms a linked list of to-be-closed variables. `s2v(o)` (`lobject.h:157`) converts `StackValue*` → `TValue*`.

---

## 2. Type Tag Constants — Complete Map

### 2.1 Base Types (`lua.h:63–72`)

| Constant | Value | Meaning |
|----------|-------|---------|
| `LUA_TNONE` | -1 | Invalid/absent (API only, never in TValue) |
| `LUA_TNIL` | 0 | Nil |
| `LUA_TBOOLEAN` | 1 | Boolean |
| `LUA_TLIGHTUSERDATA` | 2 | Light userdata (raw pointer, no GC) |
| `LUA_TNUMBER` | 3 | Number (integer or float) |
| `LUA_TSTRING` | 4 | String (short or long) |
| `LUA_TTABLE` | 5 | Table |
| `LUA_TFUNCTION` | 6 | Function (Lua closure, C closure, light C function) |
| `LUA_TUSERDATA` | 7 | Full userdata (GC-managed) |
| `LUA_TTHREAD` | 8 | Coroutine/thread |
| `LUA_NUMTYPES` | 9 | Count of public types |

### 2.2 Internal Extra Types (`lobject.h:22–24`)

| Constant | Value | Meaning |
|----------|-------|---------|
| `LUA_TUPVAL` | 9 (`LUA_NUMTYPES`) | Upvalue (internal GC object) |
| `LUA_TPROTO` | 10 (`LUA_NUMTYPES+1`) | Function prototype (internal GC object) |
| `LUA_TDEADKEY` | 11 (`LUA_NUMTYPES+2`) | Dead key marker in tables |
| `LUA_TOTALTYPES` | 12 (`LUA_TPROTO+2`) | Total type count (includes TNONE, excludes DEADKEY) |

### 2.3 Variant Tags — Full Enumeration

All variant tags are formed via `makevariant(base, variant)`:

#### Nil Variants (`lobject.h:169–192`)

| Variant Tag | Value | Hex | Meaning |
|-------------|-------|-----|---------|
| `LUA_VNIL` | `makevariant(0, 0)` | `0x00` | Standard nil |
| `LUA_VEMPTY` | `makevariant(0, 1)` | `0x10` | Empty table slot |
| `LUA_VABSTKEY` | `makevariant(0, 2)` | `0x20` | Absent key (key not found) |
| `LUA_VNOTABLE` | `makevariant(0, 3)` | `0x30` | **Lua 5.5 NEW** — fast-get hit a non-table |

#### Boolean Variants (`lobject.h:224–225`)

| Variant Tag | Value | Hex | Meaning |
|-------------|-------|-----|---------|
| `LUA_VFALSE` | `makevariant(1, 0)` | `0x01` | Boolean false |
| `LUA_VTRUE` | `makevariant(1, 1)` | `0x11` | Boolean true |

#### Number Variants (`lobject.h:271–272`)

| Variant Tag | Value | Hex | Meaning |
|-------------|-------|-----|---------|
| `LUA_VNUMINT` | `makevariant(3, 0)` | `0x03` | Integer number |
| `LUA_VNUMFLT` | `makevariant(3, 1)` | `0x13` | Float number |

#### String Variants (`lobject.h:299–300`)

| Variant Tag | Value | Hex | Meaning |
|-------------|-------|-----|---------|
| `LUA_VSHRSTR` | `makevariant(4, 0)` | `0x04` | Short string (interned) |
| `LUA_VLNGSTR` | `makevariant(4, 1)` | `0x14` | Long string (not interned) |

#### Function Variants (`lobject.h:537–539`)

| Variant Tag | Value | Hex | Meaning |
|-------------|-------|-----|---------|
| `LUA_VLCL` | `makevariant(6, 0)` | `0x06` | Lua closure |
| `LUA_VLCF` | `makevariant(6, 1)` | `0x16` | Light C function (no GC object!) |
| `LUA_VCCL` | `makevariant(6, 2)` | `0x26` | C closure |

#### Other Collectable Types

| Variant Tag | Value | Hex | Meaning |
|-------------|-------|-----|---------|
| `LUA_VLIGHTUSERDATA` | `makevariant(2, 0)` | `0x02` | Light userdata (NOT collectable) |
| `LUA_VUSERDATA` | `makevariant(7, 0)` | `0x07` | Full userdata (collectable) |
| `LUA_VTHREAD` | `makevariant(8, 0)` | `0x08` | Thread (collectable) |
| `LUA_VTABLE` | `makevariant(5, 0)` | `0x05` | Table (collectable) |
| `LUA_VPROTO` | `makevariant(10, 0)` | `0x0A` | Proto (collectable, internal) |
| `LUA_VUPVAL` | `makevariant(9, 0)` | `0x09` | Upvalue (collectable, internal) |

### 2.4 Collectable Bit Summary

When stored in a TValue's `tt_`, collectable types have bit 6 set (`0x40`):

| Stored Tag (in tt_) | Hex | Type |
|---------------------|-----|------|
| `ctb(LUA_VSHRSTR)` | `0x44` | Short string |
| `ctb(LUA_VLNGSTR)` | `0x54` | Long string |
| `ctb(LUA_VTABLE)` | `0x45` | Table |
| `ctb(LUA_VLCL)` | `0x46` | Lua closure |
| `ctb(LUA_VCCL)` | `0x66` | C closure |
| `ctb(LUA_VUSERDATA)` | `0x47` | Full userdata |
| `ctb(LUA_VTHREAD)` | `0x48` | Thread |
| `ctb(LUA_VPROTO)` | `0x4A` | Proto |
| `ctb(LUA_VUPVAL)` | `0x49` | Upvalue |

**NOT collectable** (no bit 6): `LUA_VNIL` (`0x00`), `LUA_VFALSE` (`0x01`), `LUA_VTRUE` (`0x11`), `LUA_VNUMINT` (`0x03`), `LUA_VNUMFLT` (`0x13`), `LUA_VLIGHTUSERDATA` (`0x02`), `LUA_VLCF` (`0x16`).

---

## 3. Macro System — set/get/check/test

### 3.1 Tag Setting (`lobject.h:102`)

```c
#define settt_(o,t)  ((o)->tt_=(t))
```

All `set*value` macros ultimately use `settt_` to assign the type tag.

### 3.2 Object Copy — `setobj` (`lobject.h:105–109`)

```c
#define setobj(L,obj1,obj2) \
  { TValue *io1=(obj1); const TValue *io2=(obj2); \
    io1->value_ = io2->value_; settt_(io1, io2->tt_); \
    checkliveness(L,io1); lua_assert(!isnonstrictnil(io1)); }
```

**Critical**: Copies both value AND tag. The assertion ensures non-standard nils (EMPTY, ABSTKEY, NOTABLE) are never propagated via `setobj` — they're internal-only.

Specialized aliases (`lobject.h:116–124`):
- `setobjs2s(L,o1,o2)` — stack to stack (uses `s2v`)
- `setobj2s(L,o1,o2)` — to stack
- `setobjt2t` — table to table (same as `setobj`)
- `setobj2n` — to new object (same as `setobj`)
- `setobj2t` — to table (same as `setobj`)

### 3.3 Nil / Boolean Setters

```c
/* lobject.h:212 */ #define setnilvalue(obj)   settt_(obj, LUA_VNIL)
/* lobject.h:221 */ #define setempty(v)        settt_(v, LUA_VEMPTY)
/* lobject.h:231 */ #define setbfvalue(obj)    settt_(obj, LUA_VFALSE)
/* lobject.h:232 */ #define setbtvalue(obj)    settt_(obj, LUA_VTRUE)
```

For nil and booleans, **only the tag matters** — the value union is not set (its contents are irrelevant).

### 3.4 Number Setters (`lobject.h:283–292`)

```c
#define setfltvalue(obj,x) \
  { TValue *io=(obj); val_(io).n=(x); settt_(io, LUA_VNUMFLT); }

#define setivalue(obj,x) \
  { TValue *io=(obj); val_(io).i=(x); settt_(io, LUA_VNUMINT); }
```

Also `chgfltvalue`/`chgivalue` — change value without changing tag (asserts type matches).

### 3.5 GC Object Setters

All GC object setters follow the same pattern:
1. Store `obj2gco(x)` into `val_(io).gc`
2. Set tag to `ctb(variant_tag)` (adds collectable bit)
3. Call `checkliveness(L,io)` for debug validation

```c
/* String — lobject.h:307–310 */
#define setsvalue(L,obj,x) \
  { TValue *io = (obj); TString *x_ = (x); \
    val_(io).gc = obj2gco(x_); settt_(io, ctb(x_->tt)); \
    checkliveness(L,io); }
/* Note: uses x_->tt, so the tag comes from the string object itself
   (which knows whether it's short or long) */

/* Table — lobject.h:643 */
#define sethvalue(L,obj,x) \
  { TValue *io = (obj); Table *x_ = (x); \
    val_(io).gc = obj2gco(x_); settt_(io, ctb(LUA_VTABLE)); \
    checkliveness(L,io); }

/* Lua Closure — lobject.h:558 */
#define setclLvalue(L,obj,x) \
  { TValue *io = (obj); LClosure *x_ = (x); \
    val_(io).gc = obj2gco(x_); settt_(io, ctb(LUA_VLCL)); \
    checkliveness(L,io); }

/* C Closure — lobject.h:568 */
#define setclCvalue(L,obj,x) \
  { TValue *io = (obj); CClosure *x_ = (x); \
    val_(io).gc = obj2gco(x_); settt_(io, ctb(LUA_VCCL)); \
    checkliveness(L,io); }

/* Thread — lobject.h:246 */
#define setthvalue(L,obj,x) \
  { TValue *io = (obj); lua_State *x_ = (x); \
    val_(io).gc = obj2gco(x_); settt_(io, ctb(LUA_VTHREAD)); \
    checkliveness(L,io); }

/* Full Userdata — lobject.h:376 */
#define setuvalue(L,obj,x) \
  { TValue *io = (obj); Udata *x_ = (x); \
    val_(io).gc = obj2gco(x_); settt_(io, ctb(LUA_VUSERDATA)); \
    checkliveness(L,io); }
```

### 3.6 Non-GC Setters

```c
/* Light C function — lobject.h:564 — NO collectable bit */
#define setfvalue(obj,x) \
  { TValue *io=(obj); val_(io).f=(x); settt_(io, LUA_VLCF); }

/* Light userdata — lobject.h:372 — NO collectable bit */
#define setpvalue(obj,x) \
  { TValue *io=(obj); val_(io).p=(x); settt_(io, LUA_VLIGHTUSERDATA); }
```

### 3.7 Value Getters

| Macro | Source | Returns |
|-------|--------|---------|
| `ivalue(o)` | `lobject.h:278` | `val_(o).i` (with assert) |
| `fltvalue(o)` | `lobject.h:277` | `val_(o).n` (with assert) |
| `nvalue(o)` | `lobject.h:275` | Float: `fltvalue(o)`, Int: `cast_num(ivalue(o))` |
| `tsvalue(o)` | `lobject.h:305` | `gco2ts(val_(o).gc)` → `TString*` |
| `hvalue(o)` | `lobject.h:641` | `gco2t(val_(o).gc)` → `Table*` |
| `clLvalue(o)` | `lobject.h:549` | `gco2lcl(val_(o).gc)` → `LClosure*` |
| `clCvalue(o)` | `lobject.h:551` | `gco2ccl(val_(o).gc)` → `CClosure*` |
| `fvalue(o)` | `lobject.h:550` | `val_(o).f` → `lua_CFunction` |
| `pvalue(o)` | `lobject.h:370` | `val_(o).p` → `void*` |
| `uvalue(o)` | `lobject.h:371` | `gco2u(val_(o).gc)` → `Udata*` |
| `thvalue(o)` | `lobject.h:244` | `gco2th(val_(o).gc)` → `lua_State*` |
| `gcvalue(o)` | `lobject.h:261` | `val_(o).gc` → `GCObject*` |

### 3.8 GCObject Cast Macros (`lstate.h:412–424`)

All GC objects are stored in a `GCUnion` and cast via:
```c
#define gco2ts(o)   &((cast_u(o))->ts)    // GCObject* → TString*
#define gco2u(o)    &((cast_u(o))->u)     // GCObject* → Udata*
#define gco2lcl(o)  &((cast_u(o))->cl.l)  // GCObject* → LClosure*
#define gco2ccl(o)  &((cast_u(o))->cl.c)  // GCObject* → CClosure*
#define gco2t(o)    &((cast_u(o))->h)     // GCObject* → Table*
#define gco2th(o)   &((cast_u(o))->th)    // GCObject* → lua_State*
#define gco2upv(o)  &((cast_u(o))->upv)   // GCObject* → UpVal*
#define gco2p(o)    &((cast_u(o))->p)     // GCObject* → Proto*
#define obj2gco(v)  &(cast_u(v)->gc)      // Any object → GCObject*
```

---

## 4. GCObject Header (CommonHeader)

### 4.1 Definition (`lobject.h:254`)

```c
#define CommonHeader  struct GCObject *next; lu_byte tt; lu_byte marked

typedef struct GCObject {
  CommonHeader;
} GCObject;
```

Every GC-managed object starts with these 3 fields:

| Field | Type | Size | Purpose |
|-------|------|------|---------|
| `next` | `GCObject*` | 8 bytes (64-bit) | Intrusive linked list for GC allocation tracking |
| `tt` | `lu_byte` | 1 byte | Object's type tag (variant, WITHOUT collectable bit) |
| `marked` | `lu_byte` | 1 byte | GC color + age bits |

### 4.2 `marked` Field Bit Layout (`lgc.h:86–92`)

```
bits 0-2: age in generational mode (G_NEW=0, G_SURVIVAL=1, G_OLD0=2, G_OLD1=3, G_OLD=4, G_TOUCHED1=5, G_TOUCHED2=6)
bit 3:    WHITE0BIT — white color type 0
bit 4:    WHITE1BIT — white color type 1
bit 5:    BLACKBIT  — black color
bit 6:    FINALIZEDBIT — marked for finalization
bit 7:    TESTBIT — used by tests
```

Color determination:
- **White**: `(marked & WHITEBITS) != 0` (either WHITE0 or WHITE1)
- **Black**: `testbit(marked, BLACKBIT)`
- **Gray**: neither white nor black

### 4.3 Relationship Between `tt` in GCObject and `tt_` in TValue

- `GCObject.tt` stores the variant tag **without** the collectable bit (e.g., `LUA_VSHRSTR = 0x04`)
- `TValue.tt_` stores the variant tag **with** the collectable bit (e.g., `ctb(LUA_VSHRSTR) = 0x44`)
- The `righttt(obj)` macro (`lobject.h:91`) verifies: `ttypetag(obj) == gcvalue(obj)->tt`

---

## 5. Object Structs

### 5.1 TString (`lobject.h:339–349`)

```c
typedef struct TString {
  CommonHeader;          // next, tt, marked
  lu_byte extra;         // reserved words for short strings; "has hash" for longs
  ls_byte shrlen;        // length for short strings (>=0), negative for long strings
  unsigned int hash;     // hash value
  union {
    size_t lnglen;       // length for long strings
    struct TString *hnext; // linked list for hash table (short strings)
  } u;
  char *contents;        // pointer to content in long strings
  lua_Alloc falloc;      // deallocation function for external strings
  void *ud;              // user data for external strings
} TString;
```

See [Section 7](#7-string-types--short-vs-long-interning) for detailed string analysis.

### 5.2 Table (`lobject.h:692–701`)

```c
typedef struct Table {
  CommonHeader;              // next, tt, marked
  lu_byte flags;             // 1<<p means tagmethod(p) is not present (cache)
  lu_byte lsizenode;         // log2 of number of hash slots
  unsigned int asize;        // number of array slots
  Value *array;              // array part (Value, not TValue — saves tag bytes)
  Node *node;                // hash part
  struct Table *metatable;   // metatable (or NULL)
  GCObject *gclist;          // for GC traversal
} Table;
```

**Key design note**: The array part stores `Value` (not `TValue`), relying on the fact that array entries are always set via `setobj` which validates types. **Lua 5.5 difference**: The array part uses `Value*` instead of `TValue*` — this saves 1 byte per array slot (no tag byte needed since the tag is checked on access).

#### Node (`lobject.h:660–668`)

```c
typedef union Node {
  struct NodeKey {
    TValuefields;       // value fields (value_ + tt_)
    lu_byte key_tt;     // key type tag
    int next;           // offset to next node in chain (not pointer!)
    Value key_val;      // key value
  } u;
  TValue i_val;         // direct access to value as TValue
} Node;
```

The key is split (`key_tt` + `key_val`) rather than being a proper `TValue` to minimize padding and achieve better alignment.

### 5.3 LClosure (`lobject.h:588–592`)

```c
#define ClosureHeader  CommonHeader; lu_byte nupvalues; GCObject *gclist

typedef struct LClosure {
  ClosureHeader;           // next, tt, marked, nupvalues, gclist
  struct Proto *p;         // prototype
  UpVal *upvals[1];        // flexible array of upvalue pointers
} LClosure;
```

### 5.4 CClosure (`lobject.h:583–587`)

```c
typedef struct CClosure {
  ClosureHeader;           // next, tt, marked, nupvalues, gclist
  lua_CFunction f;         // the C function
  TValue upvalue[1];       // flexible array of upvalue TValues (inline!)
} CClosure;
```

**Key difference**: `LClosure` stores **pointers to UpVal** (shared upvalues), while `CClosure` stores **TValue directly** (private upvalues).

### 5.5 Closure Union (`lobject.h:594–597`)

```c
typedef union Closure {
  CClosure c;
  LClosure l;
} Closure;
```

### 5.6 UpVal (`lobject.h:574–582`)

```c
typedef struct UpVal {
  CommonHeader;
  union {
    TValue *p;           // points to stack or to its own value
    ptrdiff_t offset;    // used during stack reallocation
  } v;
  union {
    struct {             // when open (still on stack)
      struct UpVal *next;
      struct UpVal **previous;
    } open;
    TValue value;        // the value (when closed)
  } u;
} UpVal;
```

**Open upvalue**: `v.p` points to a stack slot; `u.open` forms a doubly-linked list.
**Closed upvalue**: `v.p` points to `u.value`; the value is stored locally.

### 5.7 Udata (`lobject.h:389–397`)

```c
typedef struct Udata {
  CommonHeader;
  unsigned short nuvalue;   // number of user values
  size_t len;               // number of bytes of user data
  struct Table *metatable;
  GCObject *gclist;
  UValue uv[1];             // flexible array of user values
} Udata;
```

#### Udata0 — Optimized for zero user values (`lobject.h:407–413`)

```c
typedef struct Udata0 {
  CommonHeader;
  unsigned short nuvalue;
  size_t len;
  struct Table *metatable;
  union {LUAI_MAXALIGN;} bindata;  // ensures alignment for binary data
} Udata0;
```

When `nuvalue == 0`, the `gclist` field is absent (saves memory, and the object doesn't need to be gray during GC).

Memory layout: `udatamemoffset(nuv)` computes where user data bytes start (`lobject.h:416`).

---

## 6. Proto — Full Detail

### 6.1 Proto Struct (`lobject.h:492–515`)

```c
typedef struct Proto {
  CommonHeader;                    // next, tt, marked
  lu_byte numparams;               // number of fixed (named) parameters
  lu_byte flag;                    // PF_VAHID | PF_VATAB | PF_FIXED
  lu_byte maxstacksize;            // registers needed by this function
  int sizeupvalues;                // length of 'upvalues' array
  int sizek;                       // length of 'k' (constants) array
  int sizecode;                    // length of 'code' array
  int sizelineinfo;                // length of 'lineinfo' array
  int sizep;                       // length of 'p' (child protos) array
  int sizelocvars;                 // length of 'locvars' array
  int sizeabslineinfo;             // length of 'abslineinfo' array
  int linedefined;                 // first line of function definition
  int lastlinedefined;             // last line of function definition
  TValue *k;                       // array of constants
  Instruction *code;               // array of bytecode instructions
  struct Proto **p;                // array of child function prototypes
  Upvaldesc *upvalues;             // array of upvalue descriptors
  ls_byte *lineinfo;               // per-instruction line delta (debug)
  AbsLineInfo *abslineinfo;        // absolute line info (debug)
  LocVar *locvars;                 // local variable info (debug)
  TString *source;                 // source file name (debug)
  GCObject *gclist;                // for GC traversal
} Proto;
```

### 6.2 Field-by-Field Analysis

#### Core Execution Data

| Field | Type | Purpose |
|-------|------|---------|
| `numparams` | `lu_byte` | Fixed parameter count (max 255) |
| `flag` | `lu_byte` | Bitfield: `PF_VAHID`(1)=hidden vararg, `PF_VATAB`(2)=vararg table, `PF_FIXED`(4)=parts in fixed memory |
| `maxstacksize` | `lu_byte` | Number of registers (stack slots) this function uses |
| `code` | `Instruction*` | Bytecode array; `Instruction` = `l_uint32` (4 bytes each) |
| `sizecode` | `int` | Number of instructions |
| `k` | `TValue*` | Constant pool — literals used by the function |
| `sizek` | `int` | Number of constants |

#### Nested Functions

| Field | Type | Purpose |
|-------|------|---------|
| `p` | `Proto**` | Array of child function prototypes (nested `function` definitions) |
| `sizep` | `int` | Number of child prototypes |

#### Upvalue Descriptors

| Field | Type | Purpose |
|-------|------|---------|
| `upvalues` | `Upvaldesc*` | Array describing each upvalue's origin |
| `sizeupvalues` | `int` | Number of upvalues |

```c
typedef struct Upvaldesc {
  TString *name;     // upvalue name (for debug information)
  lu_byte instack;   // 1 = in enclosing function's stack (register)
  lu_byte idx;       // index in stack or in outer function's upvalue list
  lu_byte kind;      // kind of corresponding variable
} Upvaldesc;
```

- `instack=1, idx=N` → upvalue is register N in the immediately enclosing function
- `instack=0, idx=N` → upvalue is upvalue N of the immediately enclosing function

#### Debug Information — Line Numbers

| Field | Type | Purpose |
|-------|------|---------|
| `lineinfo` | `ls_byte*` | Per-instruction line delta from previous instruction |
| `sizelineinfo` | `int` | Length (should equal `sizecode`) |
| `abslineinfo` | `AbsLineInfo*` | Sparse absolute line numbers (for binary search) |
| `sizeabslineinfo` | `int` | Number of absolute line entries |
| `linedefined` | `int` | Source line where function definition starts |
| `lastlinedefined` | `int` | Source line where function definition ends |

```c
typedef struct AbsLineInfo {
  int pc;    // instruction index
  int line;  // absolute source line
} AbsLineInfo;
```

**Line computation algorithm** (`lobject.h:459–472`): `lineinfo[i]` gives the line delta for instruction `i`. When the delta doesn't fit in a signed byte (-128..127), an entry is added to `abslineinfo`. To find the line for a given PC: binary search `abslineinfo` for the nearest entry ≤ PC, then linearly scan `lineinfo` from there.

#### Debug Information — Local Variables

| Field | Type | Purpose |
|-------|------|---------|
| `locvars` | `LocVar*` | Array of local variable descriptors |
| `sizelocvars` | `int` | Number of local variables |

```c
typedef struct LocVar {
  TString *varname;  // variable name
  int startpc;       // first instruction where variable is active
  int endpc;         // first instruction where variable is dead
} LocVar;
```

#### Other

| Field | Type | Purpose |
|-------|------|---------|
| `source` | `TString*` | Source file name (prefixed with `@` for files, `=` for literals) |
| `gclist` | `GCObject*` | GC traversal list |

### 6.3 Proto Flags (`lobject.h:479–488`)

```c
#define PF_VAHID   1  /* function has hidden vararg arguments */
#define PF_VATAB   2  /* function has vararg table */
#define PF_FIXED   4  /* prototype has parts in fixed memory */

#define isvararg(p)    ((p)->flag & (PF_VAHID | PF_VATAB))
#define needvatab(p)   ((p)->flag |= PF_VATAB)
```

**Lua 5.5 vararg handling**: Two modes:
- `PF_VAHID`: vararg arguments are hidden (traditional `...` handling)
- `PF_VATAB`: vararg arguments collected into a table (new optimization)
- `PF_FIXED`: parts of the proto are in fixed (non-GC) memory — **Lua 5.5 new**

---

## 7. String Types — Short vs Long, Interning

### 7.1 Discrimination

The `shrlen` field (`ls_byte`, signed) serves double duty:
- **`shrlen >= 0`**: Short string; `shrlen` IS the length (max 127 bytes)
- **`shrlen < 0`**: Long string; actual length is in `u.lnglen`

Long string sub-kinds via `shrlen` value (`lobject.h:323–325`):
| `shrlen` | Constant | Meaning |
|----------|----------|---------|
| `-1` | `LSTRREG` | Regular long string (allocated by Lua) |
| `-2` | `LSTRFIX` | Fixed external long string (not freed by Lua) |
| `-3` | `LSTRMEM` | External long string with deallocation callback |

### 7.2 Short Strings — Interning

- **Interned** in a global hash table (`stringtable` in `global_State`)
- `u.hnext` links collisions in the hash table
- **Equality test is pointer comparison**: `eqshrstr(a,b)` → `(a) == (b)` (`lstring.h:54`)
- `extra` field marks reserved words (keywords) — if `extra > 0`, the string is a keyword
- Content is stored **inline** after the struct: `rawgetshrstr(ts)` → `cast_charp(&(ts)->contents)` — the `contents` field address IS the string data for short strings
- Hash is computed eagerly at creation time

### 7.3 Long Strings — Not Interned

- **Not interned** — created independently, compared by content
- `contents` is a **pointer** to separately allocated memory (or external memory)
- `extra` field: 1 = hash has been computed (lazy hashing)
- `u.lnglen` stores the actual length
- `falloc` and `ud` fields support external strings (**Lua 5.5 new feature** — `lua_pushexternalstring`)

### 7.4 String Content Access (`lobject.h:354–358`)

```c
#define rawgetshrstr(ts)  (cast_charp(&(ts)->contents))  // short: inline after struct
#define getshrstr(ts)     check_exp(strisshr(ts), rawgetshrstr(ts))
#define getlngstr(ts)     check_exp(!strisshr(ts), (ts)->contents)  // long: follow pointer
#define getstr(ts)        (strisshr(ts) ? rawgetshrstr(ts) : (ts)->contents)
#define tsslen(ts)        (strisshr(ts) ? cast_sizet((ts)->shrlen) : (ts)->u.lnglen)
```

### 7.5 String Equality (`lstring.h:54,58`)

```c
#define eqshrstr(a,b)  check_exp((a)->tt == LUA_VSHRSTR, (a) == (b))
// Short strings: pointer equality (because they're interned)

LUAI_FUNC int luaS_eqstr (TString *a, TString *b);
// General: handles short==short (pointer), short==long, long==long (content comparison)
```

---

## 8. Number Representation

### 8.1 Types (`luaconf.h:456,535`)

Default configuration (64-bit):
- `lua_Number` = `double` (64-bit IEEE 754)
- `lua_Integer` = `long long` (64-bit signed)
- `lua_Unsigned` = `unsigned long long`

Alternative configurations available:
- 32-bit float + 32-bit int
- Long double + long

### 8.2 No NaN Boxing

Lua 5.5 does **NOT** use NaN boxing. The TValue is a simple struct with a separate tag byte:

```
TValue = { Value value_ (8 bytes), lu_byte tt_ (1 byte) }
// Total: 16 bytes on 64-bit (with padding)
```

NaN boxing would pack the type tag into the NaN payload of a 64-bit double, reducing TValue to 8 bytes. Lua chose the simpler approach for portability and clarity.

### 8.3 Integer ↔ Float Coercion

Integer and float are **distinct variants** of `LUA_TNUMBER`:
- `LUA_VNUMINT` (variant 0) — stored in `value_.i`
- `LUA_VNUMFLT` (variant 1) — stored in `value_.n`

Coercion macros (`lvm.h:51–70`):
```c
// Float coercion (no string coercion):
#define tonumberns(o,n) \
  (ttisfloat(o) ? ((n) = fltvalue(o), 1) : \
  (ttisinteger(o) ? ((n) = cast_num(ivalue(o)), 1) : 0))

// Integer coercion (no string coercion):
#define tointegerns(o,i) \
  (l_likely(ttisinteger(o)) ? (*(i) = ivalue(o), 1) \
                            : luaV_tointegerns(o,i,LUA_FLOORN2I))
```

### 8.4 Arithmetic Dispatch (`lobject.c:107–166`)

`luaO_rawarith` implements raw arithmetic:
1. **Bitwise ops** (band, bor, bxor, shl, shr, bnot): integers only → `tointegerns` both operands
2. **Division, power**: floats only → `tonumberns` both operands
3. **Other ops** (add, sub, mul, mod, idiv, unm): prefer integers, fall back to floats
   - Both integers? → `intarith` (wrapping unsigned arithmetic)
   - Otherwise → `tonumberns` both, then `numarith`

---

## 9. Pseudocode for Key Operations

### 9.1 Type Checking

```
function typeOf(v: TValue) -> int:
    return v.tt_ & 0x0F           // base type (novariant)

function variantOf(v: TValue) -> int:
    return v.tt_ & 0x3F           // type + variant (withvariant)

function isCollectable(v: TValue) -> bool:
    return (v.tt_ & 0x40) != 0    // bit 6

function isNil(v: TValue) -> bool:
    return typeOf(v) == 0          // any nil variant

function isStrictNil(v: TValue) -> bool:
    return v.tt_ == 0x00           // only standard nil

function isEmpty(v: TValue) -> bool:
    return isNil(v)                // any nil variant counts as empty

function isAbsKey(v: TValue) -> bool:
    return v.tt_ == 0x20           // LUA_VABSTKEY

function isFalsy(v: TValue) -> bool:
    return v.tt_ == LUA_VFALSE || isNil(v)
```

### 9.2 Type Coercion (Number)

```
function toNumber(v: TValue) -> (float64, bool):
    if variantOf(v) == LUA_VNUMFLT:
        return v.value_.n, true
    if variantOf(v) == LUA_VNUMINT:
        return float64(v.value_.i), true
    if isString(v):
        return parseNumber(getString(v))
    return 0, false

function toInteger(v: TValue) -> (int64, bool):
    if variantOf(v) == LUA_VNUMINT:
        return v.value_.i, true
    if variantOf(v) == LUA_VNUMFLT:
        f = v.value_.n
        if f == floor(f) && fitsInInt64(f):
            return int64(f), true
    if isString(v):
        return parseInteger(getString(v))
    return 0, false
```

### 9.3 Equality Comparison (`lvm.c:582–653`)

```
function equalObj(L: *State, t1: TValue, t2: TValue) -> bool:
    // Step 1: Different base types → not equal
    if typeOf(t1) != typeOf(t2):
        return false

    // Step 2: Same base type, different variant
    if variantOf(t1) != variantOf(t2):
        switch variantOf(t1):
            case LUA_VNUMINT:  // int == float?
                i2 = floatToInt(fltvalue(t2))
                return i2.ok && ivalue(t1) == i2.val
            case LUA_VNUMFLT:  // float == int?
                i1 = floatToInt(fltvalue(t1))
                return i1.ok && i1.val == ivalue(t2)
            case LUA_VSHRSTR, LUA_VLNGSTR:  // short == long?
                return stringEqual(tsvalue(t1), tsvalue(t2))
            default:
                return false  // other types can't cross-variant equal

    // Step 3: Same variant
    switch variantOf(t1):
        case LUA_VNIL, LUA_VFALSE, LUA_VTRUE:
            return true                          // singletons
        case LUA_VNUMINT:
            return ivalue(t1) == ivalue(t2)      // integer comparison
        case LUA_VNUMFLT:
            return fltvalue(t1) == fltvalue(t2)  // float comparison (NaN != NaN)
        case LUA_VLIGHTUSERDATA:
            return pvalue(t1) == pvalue(t2)      // pointer comparison
        case LUA_VSHRSTR:
            return tsvalue(t1) == tsvalue(t2)    // pointer (interned!)
        case LUA_VLNGSTR:
            return stringEqual(tsvalue(t1), tsvalue(t2))  // content comparison
        case LUA_VLCF:
            return fvalue(t1) == fvalue(t2)      // function pointer comparison
        case LUA_VUSERDATA:
            if uvalue(t1) == uvalue(t2): return true
            // try __eq metamethod...
        case LUA_VTABLE:
            if hvalue(t1) == hvalue(t2): return true
            // try __eq metamethod...
        default:  // closures, threads
            return gcvalue(t1) == gcvalue(t2)    // identity comparison
```

**Key insight**: Numbers and strings can be equal across variants (int==float, short==long). All other types require same variant AND identity (pointer equality), unless a `__eq` metamethod is defined.

---

## 10. If I Were Building This in Go

### 10.1 The Central Problem

C uses a tagged union (1 byte tag + 8 byte union = 16 bytes with padding). Go has no unions. The main options:

### 10.2 Option A: Interface-Based (Idiomatic Go)

```go
type Value interface {
    luaValue()  // marker method
}

type Nil struct{}
type Boolean bool
type Integer int64
type Float float64
type LightUserdata unsafe.Pointer
type LightCFunction func(*State) int
// GC objects: *String, *Table, *LClosure, *CClosure, *Udata, *Thread, *UpVal, *Proto
```

**Pros**: Type-safe, no manual tag management, garbage collector handles everything.
**Cons**: Interface boxing allocates (2 words: type pointer + data pointer = 16 bytes minimum), small values like integers get heap-allocated. Very high GC pressure in a VM that manipulates millions of values.

### 10.3 Option B: Struct with Tag (Closer to C)

```go
type TValue struct {
    tt uint8  // type tag, same encoding as C
    // padding
    val [8]byte  // raw storage, interpreted based on tt
}

// Or more practically:
type TValue struct {
    tt  uint8
    i   int64   // overlapping interpretation
    n   float64 // overlapping interpretation  
    p   unsafe.Pointer
}
```

**Problem**: Go doesn't have unions. You'd need `unsafe` or store all fields separately (wasting memory).

### 10.4 Option C: Hybrid — What the Go-Lua Project Actually Uses

Looking at the existing `types/` package, the project already defines type constants matching C. A practical approach:

```go
// TValue as a compact struct
type TValue struct {
    Type uint8       // tag byte (same encoding as C)
    // 7 bytes padding on 64-bit
    Int  int64       // for integers (overlaps with other fields via unsafe, or separate)
    Num  float64     // for floats
    GC   GCObject    // interface for GC objects (or *GCObject pointer)
}
```

**Recommended approach for Go-Lua**:

```go
// Use a tagged struct with explicit fields
type Value struct {
    tt   uint8        // type tag
    ival int64        // integer value (when tt == LUA_VNUMINT)
    nval float64      // float value (when tt == LUA_VNUMFLT)
    obj  interface{}  // GC objects, light userdata, light C functions
}
```

This is 32+ bytes per value (larger than C's 16), but:
- No `unsafe` needed
- GC objects naturally tracked by Go's GC
- Type safety maintained
- `obj` field handles all pointer types via type assertion

### 10.5 Critical Go Mapping Decisions

| C Concept | Go Recommendation | Rationale |
|-----------|-------------------|-----------|
| `Value` union | Separate `int64` + `float64` + `interface{}` fields | No unions in Go |
| `tt_` byte | `uint8` with same bit encoding | Direct port of tag system |
| `CommonHeader` | Embedded struct or interface | `GCObject` interface with `gcNext()`, `gcType()`, `gcMarked()` |
| `TString` | Go `string` + metadata struct | Go strings are immutable, interned by runtime |
| `Table` | Custom struct with `[]Value` + hash map | Can't use Go `map` (different semantics) |
| `LClosure` | Struct with `*Proto` + `[]*UpVal` | Direct port |
| `CClosure` | Struct with `func(*State) int` + `[]Value` | Direct port |
| `Proto` | Direct struct port | All fields map naturally |
| `UpVal` open/closed | Struct with pointer + optional local value | Similar to C |
| Light C function | `func(*State) int` stored in `obj` field | No allocation needed |
| Light userdata | `unsafe.Pointer` stored in `obj` field | Or use `uintptr` |
| Nil variants | Separate constants for tag byte | `EMPTY`, `ABSTKEY`, `NOTABLE` as tag values |

### 10.6 Short vs Long Strings in Go

Go strings are already immutable and the runtime interns some strings. For the Lua VM:
- **Short strings**: Could use Go's string type (runtime handles interning for small strings)
- **Long strings**: Also Go strings, but equality must be content-based
- **External strings**: Need a wrapper with finalizer for the deallocation callback
- **Interning**: Implement explicit interning table for short strings (Go's runtime interning is not guaranteed)

---

## 11. Edge Cases

### 11.1 Nil Variants — Four Kinds of "Nothing"

| Variant | Tag | When Used | Visible to User? |
|---------|-----|-----------|-------------------|
| `LUA_VNIL` | `0x00` | Standard nil value | ✅ Yes |
| `LUA_VEMPTY` | `0x10` | Empty table array slot | ❌ Internal only |
| `LUA_VABSTKEY` | `0x20` | Key not found in table | ❌ Internal only |
| `LUA_VNOTABLE` | `0x30` | Fast-get on non-table (**5.5 new**) | ❌ Internal only |

**Critical invariant**: `setobj` asserts `!isnonstrictnil(io1)` — non-standard nils must NEVER escape into user-visible values. They exist only as internal sentinel returns from table lookups.

`LUA_VNOTABLE` is new in Lua 5.5: when a fast table access (`luaV_fastget`) is attempted on a non-table value, it returns `LUA_VNOTABLE` instead of crashing, allowing the VM to fall through to metamethod handling (`lvm.h:82,90`, `lvm.c:296`).

### 11.2 Boolean Encoding — Tag IS the Value

Booleans are encoded **entirely in the tag byte**:
- `LUA_VFALSE = 0x01` — the `value_` union is irrelevant
- `LUA_VTRUE = 0x11` — the `value_` union is irrelevant

This means `setbfvalue(obj)` and `setbtvalue(obj)` only call `settt_` — no value assignment needed.

The `l_isfalse(o)` macro (`lobject.h:229`) tests both false and nil:
```c
#define l_isfalse(o)  (ttisfalse(o) || ttisnil(o))
```

### 11.3 Light Userdata vs Full Userdata

| Property | Light Userdata | Full Userdata |
|----------|---------------|---------------|
| Base type | `LUA_TLIGHTUSERDATA` (2) | `LUA_TUSERDATA` (7) |
| Tag | `LUA_VLIGHTUSERDATA` (`0x02`) | `ctb(LUA_VUSERDATA)` (`0x47`) |
| GC managed | ❌ No | ✅ Yes |
| Stored in | `value_.p` (raw `void*`) | `value_.gc` → `Udata*` |
| Has metatable | ❌ No (per-type only) | ✅ Yes (per-object) |
| Has user values | ❌ No | ✅ Yes (`nuvalue` count) |
| Memory | Just a pointer in TValue | Full GC object + data block |
| Identity | Pointer comparison | Pointer comparison (or `__eq`) |

**Note**: Despite both being "userdata", they have **different base type numbers** (2 vs 7). This is a historical compatibility decision noted in `lobject.h:365`: "Light userdata should be a variant of userdata, but for compatibility reasons they are also different types."

### 11.4 Light C Functions vs Closures

| Property | Light C Function | C Closure | Lua Closure |
|----------|-----------------|-----------|-------------|
| Variant | `LUA_VLCF` (`0x16`) | `ctb(LUA_VCCL)` (`0x66`) | `ctb(LUA_VLCL)` (`0x46`) |
| GC managed | ❌ No | ✅ Yes | ✅ Yes |
| Stored in | `value_.f` (function pointer) | `value_.gc` → `CClosure*` | `value_.gc` → `LClosure*` |
| Upvalues | ❌ None | ✅ Inline `TValue[]` | ✅ `UpVal*[]` (shared) |
| Memory | Just a function pointer | GC object + upvalues | GC object + proto + upval ptrs |

A light C function is created by `lua_pushcfunction(L, f)` which calls `lua_pushcclosure(L, f, 0)`. When `n == 0` (no upvalues), the implementation stores just the function pointer — no GC allocation.

### 11.5 String Edge Cases

**Short string max length**: `shrlen` is `ls_byte` (signed byte), so max value is 127. Strings longer than `LUAI_MAXSHORTLEN` (typically 40) become long strings.

**External strings** (Lua 5.5 new — `lua_pushexternalstring`):
- `shrlen` set to `LSTRFIX` (-2) or `LSTRMEM` (-3)
- `contents` points to external memory (not owned by Lua)
- `LSTRMEM` strings have a `falloc` callback for deallocation
- `LSTRFIX` strings are never freed by Lua

**Hash laziness**: Short strings compute hash eagerly at creation. Long strings compute hash lazily (only when needed, tracked by `extra` field).

### 11.6 Table Array Part — Value, Not TValue

The table's `array` field is `Value*`, not `TValue*` (`lobject.h:696`). This is significant:
- Array elements don't store their own type tag inline
- The type information must be tracked separately or the array is homogeneous
- **Actually**: Looking more carefully, the array stores `Value` which includes the `gc` pointer, and the type is checked via the node/key system. Empty slots are detected by checking against nil patterns.

**Correction**: Re-examining the code — the `array` field stores `Value` (the union without the tag). This appears to be a Lua 5.5 optimization. The array part uses a separate mechanism to track which slots are occupied vs empty, likely using the GC bits or sentinel values.

### 11.7 Dead Keys in Tables

When a key is removed during table traversal, it's marked with `LUA_TDEADKEY` (`lobject.h:730`):
```c
#define setdeadkey(node)  (keytt(node) = LUA_TDEADKEY)
#define keyisdead(node)   (keytt(node) == LUA_TDEADKEY)
```

Dead keys keep their original `gc` value so that `next` chain traversal still works, but they're skipped during lookups.

---

## Appendix A: Complete Tag Byte Reference

```
Hex   Binary      Type
────  ──────────  ──────────────────────────
0x00  00_00_0000  LUA_VNIL (standard nil)
0x10  00_01_0000  LUA_VEMPTY (empty slot)
0x20  00_10_0000  LUA_VABSTKEY (absent key)
0x30  00_11_0000  LUA_VNOTABLE (non-table fast-get)
0x01  00_00_0001  LUA_VFALSE
0x11  00_01_0001  LUA_VTRUE
0x02  00_00_0010  LUA_VLIGHTUSERDATA
0x03  00_00_0011  LUA_VNUMINT
0x13  00_01_0011  LUA_VNUMFLT
0x44  01_00_0100  ctb(LUA_VSHRSTR)  [stored in tt_]
0x54  01_01_0100  ctb(LUA_VLNGSTR)  [stored in tt_]
0x45  01_00_0101  ctb(LUA_VTABLE)
0x46  01_00_0110  ctb(LUA_VLCL)     [Lua closure]
0x16  00_01_0110  LUA_VLCF          [light C func, NOT collectable]
0x66  01_10_0110  ctb(LUA_VCCL)     [C closure]
0x47  01_00_0111  ctb(LUA_VUSERDATA)
0x48  01_00_1000  ctb(LUA_VTHREAD)
0x49  01_00_1001  ctb(LUA_VUPVAL)   [internal]
0x4A  01_00_1010  ctb(LUA_VPROTO)   [internal]

Binary format: CC_VV_TTTT
  C = collectable (bit 6), V = variant (bits 4-5), T = base type (bits 0-3)
```

## Appendix B: Lua 5.5 Differences from 5.4

Based on analysis of the source code:

1. **`LUA_VNOTABLE` (lobject.h:192)**: New nil variant (variant 3) for fast-get on non-table values. Lua 5.4 had only 3 nil variants (nil, empty, abstkey).

2. **External strings (lobject.h:323–325, 345–347)**: `TString` now has `falloc` and `ud` fields for external string support. `lua_pushexternalstring` is a new API function (`lua.h:302`).

3. **`PF_FIXED` flag (lobject.h:481)**: New prototype flag indicating parts are in fixed memory.

4. **`PF_VATAB` (lobject.h:480)**: Vararg table optimization — varargs can be collected into a table instead of hidden arguments.

5. **Table array as `Value*` (lobject.h:696)**: The array part stores `Value` instead of `TValue`, saving memory per array element.

6. **Copyright 2026**: Confirms this is a development/pre-release version of 5.5.

7. **`lua_newstate` takes `unsigned seed` (lua.h:167)**: New parameter for hash randomization seed.

8. **`lua_Debug.extraargs` (lua.h:510)**: New field for extra arguments count in debug info.

---

*Generated from Lua 5.5.1 source (lua-master/). Line references are to the source files as of this analysis.*
