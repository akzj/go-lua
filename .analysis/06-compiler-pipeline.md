# 06 — Lua 5.5.1 Compiler Pipeline: Lexer, Parser, Code Generator

> **Source files**: `llex.c` (604 lines), `llex.h` (93 lines), `lparser.c` (2202 lines), `lparser.h` (196 lines), `lcode.c` (1972 lines), `lcode.h` (105 lines)  
> **Total**: ~5,172 lines of tightly integrated C code  
> **Lua version**: 5.5.1 (development) — differences from 5.4 noted throughout  
> **Line references** cite the source files in `/home/ubuntu/workspace/go-lua/lua-master/`

---

## Table of Contents

1. [The Complete Compilation Flow](#1-the-complete-compilation-flow)
2. [Lexer — llex.c](#2-lexer--llexc)
3. [Parser — lparser.c](#3-parser--lparserc)
4. [Code Generator — lcode.c](#4-code-generator--lcodec)
5. [Upvalue Resolution](#5-upvalue-resolution)
6. [MULTRET Handling](#6-multret-handling)
7. [Vararg Handling](#7-vararg-handling)
8. [If I Were Building This in Go](#8-if-i-were-building-this-in-go)
9. [Edge Cases and Traps](#9-edge-cases-and-traps)
10. [Bug Pattern Guide](#10-bug-pattern-guide)

---

## 1. The Complete Compilation Flow

### 1.1 The Pipeline at a Glance

```
Source string (or reader function)
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│  ZIO (lzio.c)                                               │
│  Buffered character stream with refill callback              │
│  zgetc() → fast inline read with luaZ_fill() fallback       │
│  EOZ (-1) signals end-of-input                               │
└──────────────────────┬──────────────────────────────────────┘
                       │ characters
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Lexer — llex.c                                              │
│  llex() scans characters → Token (type + SemInfo)            │
│  String interning via ls->h table (GC anchor + dedup)        │
│  Number parsing delegates to luaO_str2num                    │
│  Reserved word detection via TString.extra field             │
│  Single-token lookahead via luaX_lookahead()                 │
└──────────────────────┬──────────────────────────────────────┘
                       │ tokens
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Parser — lparser.c                                          │
│  Recursive descent with Pratt precedence climbing            │
│  Builds expdesc (expression descriptors) — delayed eval      │
│  Manages FuncState chain for nested functions                │
│  BlockCnt stack for lexical scoping                          │
│  Variable resolution: local → upvalue → _ENV[name]           │
│  Goto/label resolution with scope checking                   │
│                                                              │
│  Calls into lcode.c for ALL code generation                  │
└──────────────────────┬──────────────────────────────────────┘
                       │ expdesc operations
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Code Generator — lcode.c                                    │
│  Converts expdesc → bytecode instructions in Proto->code[]   │
│  Register allocation: trivial stack pointer (freereg)        │
│  Constant pool management with kcache deduplication          │
│  Jump list patching (threaded through JMP instructions)      │
│  Constant folding for numeric operations                     │
│  Two-instruction pattern: arithmetic op + MMBIN fallback     │
│  luaK_finish() final fixup pass                              │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  Proto (lobject.h)                                           │
│  The compiled function prototype:                            │
│  • code[]         — bytecode instructions (Instruction[])    │
│  • k[]            — constant pool (TValue[])                 │
│  • p[]            — nested function prototypes (Proto*[])    │
│  • upvalues[]     — upvalue descriptors (Upvaldesc[])        │
│  • locvars[]      — local variable debug info                │
│  • lineinfo[]     — line number delta encoding               │
│  • abslineinfo[]  — periodic absolute line info              │
│  • numparams, maxstacksize, flag, source, etc.               │
│                                                              │
│  Wrapped in LClosure with _ENV as upvalue[0]                 │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 Entry Point: `luaY_parser()` (lparser.c line 2177)

This is the **sole public entry** to the entire compilation pipeline:

```c
LClosure *luaY_parser(lua_State *L, ZIO *z, Mbuffer *buff,
                       Dyndata *dyd, const char *name, int firstchar)
```

**Setup sequence** (lparser.c lines 2177–2201):
1. Creates an `LClosure` with 1 upvalue slot (for `_ENV`) — line 2181
2. Anchors it on the stack to prevent GC collection — lines 2182–2183
3. Creates scanner hash table `lexstate.h` for string interning — line 2184
4. Creates a new `Proto` for the main function, links to closure — line 2187
5. Creates source `TString` — line 2189
6. Resets `Dyndata` counters (actvar, gotos, labels all to 0) — line 2193
7. Calls `luaX_setinput()` to initialize the lexer — line 2194
8. Calls `mainfunc()` to compile the main body — line 2195
9. Asserts: no previous FuncState, exactly 1 upvalue (_ENV), no remaining scopes — lines 2196–2198
10. Pops scanner table, returns the closure — lines 2199–2200

**Key insight**: The return value is an `LClosure*`, not a `Proto*`. The main function is always wrapped in a closure with `_ENV` as `upvalue[0]`.

### 1.3 The Main Function: `mainfunc()` (lparser.c line 2159)

```c
static void mainfunc(LexState *ls, FuncState *fs) {
    open_func(ls, fs, &bl);
    setvararg(fs);           // main is always vararg (line 2163)
    env = allocupvalue(fs);  // _ENV = upvalue[0] (line 2164)
    env->instack = 1;        // references caller's stack slot 0
    env->idx = 0;
    env->kind = VDKREG;
    env->name = ls->envn;    // "_ENV" string
    luaX_next(ls);           // read first token (line 2170)
    statlist(ls);            // parse entire file body (line 2171)
    check(ls, TK_EOS);      // must reach end of stream (line 2172)
    close_func(ls);
}
```

**Critical**: `_ENV` upvalue is manually created here (not through the normal upvalue capture mechanism). It's `instack=1, idx=0` meaning it references register 0 of the enclosing C function that called `luaY_parser`.

### 1.4 Key Data Structures

#### LexState (llex.h lines 64–82)
```c
typedef struct LexState {
  int current;           // current character (charint)
  int linenumber;        // input line counter
  int lastline;          // line of last token 'consumed'
  Token t;               // current token
  Token lookahead;       // look ahead token
  struct FuncState *fs;  // current function (parser)
  struct lua_State *L;
  ZIO *z;                // input stream
  Mbuffer *buff;         // buffer for tokens
  Table *h;              // string anchor table (GC protection + dedup)
  struct Dyndata *dyd;   // dynamic structures used by the parser
  TString *source;       // current source name
  TString *envn;         // "_ENV" string (pre-interned)
  TString *brkn;         // "break" string (used as label name)
  TString *glbn;         // "global" string (5.5 compat mode)
} LexState;
```

#### FuncState (lparser.h lines 152–175)
```c
typedef struct FuncState {
  Proto *f;              // current function header (Proto being built)
  struct FuncState *prev; // enclosing function (linked list!)
  struct LexState *ls;   // lexical state (shared across all functions)
  struct BlockCnt *bl;   // chain of current blocks
  Table *kcache;         // cache for constant deduplication
  int pc;                // next position to code (= ncode)
  int lasttarget;        // label of last jump target
  int previousline;      // last line saved in lineinfo
  int nk;                // number of elements in k[]
  int np;                // number of elements in p[]
  int nabslineinfo;      // number of absolute line info entries
  int firstlocal;        // index of first local var (in Dyndata array)
  int firstlabel;        // index of first label (in dyd->label->arr)
  short ndebugvars;      // number of elements in f->locvars
  short nactvar;         // number of active variable declarations
  lu_byte nups;          // number of upvalues
  lu_byte freereg;       // first free register
  lu_byte iwthabs;       // instructions since last absolute line info
  lu_byte needclose;     // function needs to close upvalues on return
} FuncState;
```

#### expdesc (lparser.h lines 26–92)

The **central abstraction** of the compiler. An expression descriptor that enables delayed code generation:

```c
typedef enum {
  VVOID,       // empty list (no expression)
  VNIL,        // constant nil
  VTRUE,       // constant true
  VFALSE,      // constant false
  VK,          // constant in k[]; info = index
  VKFLT,       // floating constant; nval = value
  VKINT,       // integer constant; ival = value
  VKSTR,       // string constant; strval = TString*
  VNONRELOC,   // value in fixed register; info = register
  VLOCAL,      // local variable; var.ridx = register, var.vidx = index
  VVARGVAR,    // vararg parameter (5.5); var.ridx, var.vidx
  VGLOBAL,     // global variable (5.5); info = actvar index or -1
  VUPVAL,      // upvalue variable; info = upvalue index
  VCONST,      // compile-time <const>; info = absolute actvar index
  VINDEXED,    // t[k] general; ind.t = table reg, ind.idx = key reg
  VVARGIND,    // vararg[k] (5.5); ind.* as VINDEXED
  VINDEXUP,    // upval[K]; ind.idx = key's K index
  VINDEXI,     // t[integer]; ind.t = table reg, ind.idx = int value
  VINDEXSTR,   // t["string"]; ind.idx = key's K index
  VJMP,        // test/comparison; info = pc of JMP instruction
  VRELOC,      // result in any register; info = instruction pc
  VCALL,       // function call; info = instruction pc
  VVARARG      // vararg expression; info = instruction pc
} expkind;

typedef struct expdesc {
  expkind k;
  union {
    lua_Integer ival;       // for VKINT
    lua_Number nval;        // for VKFLT
    TString *strval;        // for VKSTR
    int info;               // for generic use
    struct {                // for indexed variables
      short idx;            // index (R or "long" K)
      lu_byte t;            // table (register or upvalue)
      lu_byte ro;           // true if variable is read-only
      int keystr;           // K index of string key, or -1
    } ind;
    struct {                // for local variables
      lu_byte ridx;         // register holding the variable
      short vidx;           // index in actvar.arr
    } var;
  } u;
  int t;  // patch list of 'exit when true'
  int f;  // patch list of 'exit when false'
} expdesc;
```

**The t/f jump lists**: Every expression carries two jump patch lists for short-circuit evaluation. These are linked lists threaded through JMP instructions' sJ fields. `NO_JUMP (-1)` marks the end.

### 1.5 How the Three Files Connect

The three source files form a strict layered pipeline:

```
llex.c ←── lparser.c ──→ lcode.c
  ↑              ↑              ↑
  │              │              │
characters    tokens        expdesc
              grammar       opcodes
              rules         registers
```

**lparser.c is the orchestrator**:
- It calls `luaX_next(ls)` and `luaX_lookahead(ls)` to consume tokens from llex.c
- It calls `luaK_*()` functions to emit code via lcode.c
- It NEVER directly writes to `Proto->code[]` — all code emission goes through lcode.c
- It manages the `FuncState` chain and `BlockCnt` stack that both llex.c and lcode.c reference

**llex.c is independent**: It only reads characters and produces tokens. It doesn't know about grammar or opcodes.

**lcode.c depends on parser structures**: It takes `FuncState*` and `expdesc*` as inputs, manipulates `Proto` fields, and emits instructions. It doesn't consume tokens directly.


---

## 2. Lexer — llex.c

### 2.1 Initialization: `luaX_init()` (llex.c lines 75–84)

Called once during `lua_newstate`. Creates interned TString objects for all reserved words:

```c
void luaX_init (lua_State *L) {
  int i;
  TString *e = luaS_newliteral(L, LUA_ENV);  // create "_ENV" string
  luaC_fix(L, obj2gco(e));                     // pin from GC forever
  for (i=0; i<NUM_RESERVED; i++) {
    TString *ts = luaS_new(L, luaX_tokens[i]);
    luaC_fix(L, obj2gco(ts));                  // reserved words never collected
    ts->extra = cast_byte(i+1);                // mark as reserved (1-based!)
  }
}
```

**Key mechanics**:
- Each reserved word's `TString.extra` field is set to `i+1` (1-based index). This is the **only** mechanism for detecting reserved words — the lexer checks `ts->extra > 0` via the `isreserved()` macro (lstring.h line 48).
- `_ENV` string is created and pinned from GC but is NOT a reserved word (no `extra` set).
- `NUM_RESERVED` = `TK_WHILE - FIRST_RESERVED + 1` = **23** reserved words in 5.5.1 (including `global`).

**🔑 Lua 5.5 change**: `TK_GLOBAL` is new — "global" is a reserved word (llex.h line 36). However, `LUA_COMPAT_GLOBAL` (default ON in luaconf.h line 347) un-reserves it at runtime (see §2.8).

### 2.2 Token Types (llex.h lines 30–49)

Three categories of tokens:

**Category 1: Single-char tokens** (value < FIRST_RESERVED = 256)
Returned as their ASCII value: `+`, `*`, `%`, `{`, `}`, `(`, `)`, `;`, `,`, `#`, `^`, `&`, `|`, `[`, `]`, `=`, `<`, `>`, `/`, `~`, `:`, `.`, `-`

**Category 2: Reserved words** (FIRST_RESERVED to TK_WHILE)
```
TK_AND=256, TK_BREAK, TK_DO, TK_ELSE, TK_ELSEIF, TK_END,
TK_FALSE, TK_FOR, TK_FUNCTION, TK_GLOBAL, TK_GOTO, TK_IF,
TK_IN, TK_LOCAL, TK_NIL, TK_NOT, TK_OR, TK_REPEAT, TK_RETURN,
TK_THEN, TK_TRUE, TK_UNTIL, TK_WHILE
```
Total: 23 reserved words (22 in Lua 5.4 + `global`).

**Category 3: Multi-char operators and value tokens** (TK_IDIV to TK_STRING)
```
TK_IDIV (//), TK_CONCAT (..), TK_DOTS (...), TK_EQ (==),
TK_GE (>=), TK_LE (<=), TK_NE (~=), TK_SHL (<<), TK_SHR (>>),
TK_DBCOLON (::), TK_EOS (<eof>),
TK_FLT (<number>), TK_INT (<integer>), TK_NAME (<name>), TK_STRING (<string>)
```

**Token struct** (llex.h lines 58–61):
```c
typedef struct Token {
  int token;        // token type (ASCII char, or RESERVED enum value)
  SemInfo seminfo;  // semantic payload
} Token;

typedef union {
  lua_Number r;    // for TK_FLT
  lua_Integer i;   // for TK_INT
  TString *ts;     // for TK_NAME, TK_STRING
} SemInfo;
```

### 2.3 The Main Scan Loop: `llex()` (llex.c lines 467–585)

The core lexer function. A `for(;;)` loop with a `switch` on `ls->current`:

| Character(s) | Lines | Action |
|---|---|---|
| `\n`, `\r` | 471–474 | `inclinenumber(ls)`, continue (skip whitespace) |
| ` `, `\f`, `\t`, `\v` | 475–478 | `next(ls)`, continue (skip whitespace) |
| `-` | 479–497 | If `--`: comment (long or short). If just `-`: return `'-'` |
| `[` | 498–507 | `skip_sep()` → if sep≥2: long string. Else return `'['` |
| `=` | 508–512 | `==` → TK_EQ, else `'='` |
| `<` | 513–518 | `<=` → TK_LE, `<<` → TK_SHL, else `'<'` |
| `>` | 519–524 | `>=` → TK_GE, `>>` → TK_SHR, else `'>'` |
| `/` | 525–529 | `//` → TK_IDIV, else `'/'` |
| `~` | 530–534 | `~=` → TK_NE, else `'~'` |
| `:` | 535–539 | `::` → TK_DBCOLON, else `':'` |
| `"`, `'` | 540–543 | `read_string()` → TK_STRING |
| `.` | 544–553 | `...` → TK_DOTS, `..` → TK_CONCAT, `.N` → `read_numeral()`, else `'.'` |
| `0`–`9` | 554–557 | `read_numeral()` → TK_INT or TK_FLT |
| EOZ | 558–560 | return TK_EOS |
| default | 561–582 | If `lislalpha`: identifier/reserved word. Else: single-char token |

**Comment handling** (llex.c lines 479–497):
```
'-' → next(ls)
  not '-' → return '-'  (minus operator)
  is '-' → next(ls)     (it's a comment)
    is '[' → skip_sep(ls)
      sep >= 2 → read_long_string(ls, NULL, sep)  // long comment
      else → fall through to short comment
    short comment → skip until newline or EOZ
```

**Dot disambiguation** (llex.c lines 544–553): The `.` character is the most overloaded — it could be `.` (field access), `..` (concat), `...` (vararg), or the start of a number like `.5`.

### 2.4 String Interning: `luaX_newstring()` and `anchorstr()` (llex.c lines 129–158)

**Two-level interning**:

1. **Level 1 — `luaS_newlstr()`** (lstring.c): Creates or finds an interned TString in the global string table. Short strings (≤LUAI_MAXSHORTLEN=40) are always interned (hash-based dedup). Long strings get a unique TString each time.

2. **Level 2 — `anchorstr()`** (llex.c lines 135–150): Anchors the string in `ls->h` (a Lua Table used as a hash set). Two purposes:
   - **GC protection**: Strings referenced only by compiler temporaries could be collected. Anchoring in `ls->h` keeps them alive.
   - **Long string dedup**: Since `luaS_newlstr` doesn't dedup long strings, `anchorstr` does it via `luaH_getstr`.

```c
static TString *anchorstr (LexState *ls, TString *ts) {
  TValue oldts;
  int tag = luaH_getstr(ls->h, ts, &oldts);
  if (!tagisempty(tag))           // already present?
    return tsvalue(&oldts);       // reuse existing
  else {
    TValue *stv = s2v(L->top.p++);
    setsvalue(L, stv, ts);
    luaH_set(L, ls->h, stv, stv); // t[string] = string
    luaC_checkGC(L);
    L->top.p--;
    return ts;
  }
}
```

### 2.5 Number Parsing: `read_numeral()` (llex.c lines 244–273)

**Strategy**: The lexer is deliberately **liberal** — it collects characters that *might* be part of a number, then delegates to `luaO_str2num` for strict validation.

```c
static int read_numeral (LexState *ls, SemInfo *seminfo) {
  const char *expo = "Ee";
  int first = ls->current;
  save_and_next(ls);
  if (first == '0' && check_next2(ls, "xX"))  // hex?
    expo = "Pp";                                // hex uses P exponent
  for (;;) {
    if (check_next2(ls, expo))     // exponent mark?
      check_next2(ls, "-+");      // optional sign after exponent
    else if (lisxdigit(ls->current) || ls->current == '.')
      save_and_next(ls);
    else break;
  }
  if (lislalpha(ls->current))     // numeral touching a letter?
    save_and_next(ls);            // force error (e.g., "123abc")
  save(ls, '\0');
  // ... delegates to luaO_str2num for actual parsing
}
```

**Key behaviors**:
- **Hex detection**: `0x`/`0X` prefix switches exponent marker from `Ee` to `Pp` (IEEE 754 hex float syntax)
- **Sign after exponent only**: `+-` is only accepted immediately after `E/e/P/p`
- **Letter-touching detection**: If a letter immediately follows (e.g., `123abc`), it's saved to force a parse error
- **Integer vs float**: `luaO_str2num` decides — numbers without `.` or exponent that fit in `lua_Integer` become TK_INT; everything else becomes TK_FLT

### 2.6 Long Strings and Comments (llex.c lines 282–333)

#### `skip_sep()` (llex.c lines 282–294)
Reads bracket delimiter `[=*[` or `]=*]`:
- Returns `count + 2` (≥2) for well-formed delimiter: `[[` → 2, `[=[` → 3, `[==[` → 4
- Returns `1` for single bracket `[` with no `=` signs
- Returns `0` for unfinished `[=...` (error condition)

#### `read_long_string()` (llex.c lines 297–333)

```c
static void read_long_string (LexState *ls, SemInfo *seminfo, size_t sep) {
  save_and_next(ls);               // skip 2nd '['
  if (currIsNewline(ls))
    inclinenumber(ls);             // skip leading newline (Lua spec!)
  for (;;) {
    switch (ls->current) {
      case EOZ: /* error */
      case ']': if (skip_sep(ls) == sep) { save_and_next(ls); goto endloop; }
      case '\n': case '\r':
        save(ls, '\n');            // normalize to \n
        inclinenumber(ls);
        if (!seminfo) luaZ_resetbuffer(ls->buff);  // comments: save memory
      default:
        if (seminfo) save_and_next(ls);
        else next(ls);             // comments: don't save content
    }
  } endloop:
  if (seminfo)
    seminfo->ts = luaX_newstring(ls,
      luaZ_buffer(ls->buff) + sep,           // skip opening delimiter
      luaZ_bufflen(ls->buff) - 2 * sep);     // exclude both delimiters
}
```

**Key behaviors**:
- **Leading newline skip**: First newline after `[[` is always discarded (per Lua spec, llex.c line 300–301)
- **Newline normalization**: All `\r`, `\n`, `\r\n`, `\n\r` become `\n`
- **Comment memory optimization**: When `seminfo == NULL` (comment mode), characters are NOT saved and buffer is reset on newlines — prevents huge comments from consuming memory
- **No nesting**: `[==[` is only closed by `]==]`. A `]]` inside is just content

### 2.7 Short Strings: `read_string()` (llex.c lines 404–464)

Handles all escape sequences:

| Escape | Lines | Result |
|---|---|---|
| `\a` | 419 | Bell (0x07) |
| `\b` | 420 | Backspace (0x08) |
| `\f` | 421 | Form feed (0x0C) |
| `\n` | 422 | Newline (0x0A) |
| `\r` | 423 | Carriage return (0x0D) |
| `\t` | 424 | Tab (0x09) |
| `\v` | 425 | Vertical tab (0x0B) |
| `\xHH` | 426 | Hex escape (exactly 2 hex digits) |
| `\u{XXXX}` | 427 | UTF-8 escape (variable hex digits, value ≤ 0x7FFFFFFF) |
| `\\` | 430–431 | Literal backslash |
| `\"` | 430–431 | Literal double quote |
| `\'` | 430–431 | Literal single quote |
| `\<newline>` | 428–429 | Line break in string (increments line counter!) |
| `\z` | 433–441 | Zap whitespace — skips all following whitespace including newlines |
| `\ddd` | 442–445 | Decimal escape (1–3 digits, value ≤ 255) |

**Processing flow**: The escape handler uses `goto` labels: `read_save` → `only_save` → `no_save` for clean control flow. The `\\` is always saved first (llex.c line 417) for error context, then removed at `only_save` (line 452).

### 2.8 Reserved Words: Identifier vs Keyword Detection (llex.c lines 562–576)

```c
if (lislalpha(ls->current)) {  // starts with letter or '_'
  TString *ts;
  do { save_and_next(ls); } while (lislalnum(ls->current));
  ts = luaS_newlstr(ls->L, luaZ_buffer(ls->buff), luaZ_bufflen(ls->buff));
  if (isreserved(ts))
    return ts->extra - 1 + FIRST_RESERVED;  // convert to token type
  else {
    seminfo->ts = anchorstr(ls, ts);
    return TK_NAME;
  }
}
```

**Mechanism**: `isreserved(ts)` expands to `strisshr(s) && s->extra > 0`. If reserved, token type = `ts->extra - 1 + FIRST_RESERVED` (the `-1` is because `extra` was stored 1-based in `luaX_init`).

**🔑 Lua 5.5 `global` keyword compatibility** (luaX_setinput, llex.c lines 191–195):
```c
#if LUA_COMPAT_GLOBAL
  ls->glbn = luaS_newliteral(L, "global");
  ls->glbn->extra = 0;  // UN-reserve "global"!
#endif
```
When `LUA_COMPAT_GLOBAL` is enabled (default), `luaX_setinput` **clears the `extra` field** of the "global" string, making it no longer reserved. The parser then handles "global" specially by comparing `ls->t.seminfo.ts == ls->glbn` (lparser.c line 2127).

**Important**: `luaS_newlstr` returns the *same* TString pointer for "global" that `luaX_init` created (short strings are interned). So setting `extra = 0` modifies the global interned string — the un-reservation persists across compilations.

### 2.9 The Lookahead Mechanism (llex.c lines 588–603)

```c
void luaX_next (LexState *ls) {
  ls->lastline = ls->linenumber;
  if (ls->lookahead.token != TK_EOS) {  // look-ahead token?
    ls->t = ls->lookahead;              // use it
    ls->lookahead.token = TK_EOS;       // discharge
  }
  else
    ls->t.token = llex(ls, &ls->t.seminfo);  // read next
}

int luaX_lookahead (LexState *ls) {
  lua_assert(ls->lookahead.token == TK_EOS);
  ls->lookahead.token = llex(ls, &ls->lookahead.seminfo);
  return ls->lookahead.token;
}
```

- Lua needs at most **one token of lookahead** (LL(1) with occasional LL(2))
- `lookahead.token == TK_EOS` means "no lookahead stored" (TK_EOS as sentinel)
- `luaX_lookahead` asserts it's not called twice without consuming
- Used in parser for: `suffixedexp` (function call disambiguation), `field` (lparser.c line 994), compat global detection (lparser.c line 2128)

### 2.10 ZIO Integration (lzio.h + llex.c line 32)

```c
#define next(ls)  (ls->current = zgetc(ls->z))
#define zgetc(z)  (((z)->n--)>0 ? cast_uchar(*(z)->p++) : luaZ_fill(z))
```

- `ls->current` always holds the **current character** (already read but not yet consumed)
- `next(ls)` advances to the next character
- `save_and_next(ls)` saves current to buffer, then advances
- `EOZ (-1)`: End-of-stream sentinel. The `lctype.h` character classification table has an entry for index -1 with no bits set, so `lisdigit(EOZ)` etc. are all false — safe to test without explicit EOZ checks

### 2.11 Line Counting (llex.c lines 165–173)

```c
static void inclinenumber (LexState *ls) {
  int old = ls->current;
  next(ls);                                    // skip '\n' or '\r'
  if (currIsNewline(ls) && ls->current != old)
    next(ls);                                  // skip '\n\r' or '\r\n'
  if (++ls->linenumber >= INT_MAX)
    lexerror(ls, "chunk has too many lines", 0);
}
```

Handles all four newline conventions: `\n` (Unix), `\r` (old Mac), `\n\r`, `\r\n` (Windows). The check `ls->current != old` prevents `\n\n` from being treated as one line break.

### 2.12 Scanner Initialization: `luaX_setinput()` (llex.c lines 176–197)

```c
void luaX_setinput (lua_State *L, LexState *ls, ZIO *z,
                    TString *source, int firstchar) {
  ls->current = firstchar;        // first char already read by caller
  ls->lookahead.token = TK_EOS;   // no look-ahead
  ls->linenumber = 1;
  ls->envn = luaS_newliteral(L, LUA_ENV);    // "_ENV"
  ls->brkn = luaS_newliteral(L, "break");    // used as label
  // ... LUA_COMPAT_GLOBAL handling ...
  luaZ_resizebuffer(ls->L, ls->buff, LUA_MINBUFFER);
}
```

Notable: `firstchar` is passed in — the first character has already been read by the caller (to detect BOM or binary chunks). `ls->brkn` ("break") is pre-interned because Lua 5.5 treats `break` as `goto "break"`.


---

## 3. Parser — lparser.c

### 3.1 Grammar Rules Mapped to Functions

The parser is a **recursive descent parser** where each grammar production maps to a C function. Complete mapping:

#### Statement-level functions:

| Function | Lines | Grammar Production |
|----------|-------|-------------------|
| `statlist()` | 875–884 | `statlist → { stat [';'] }` |
| `statement()` | 2058–2148 | Dispatch to specific statement parsers |
| `block()` | 1419–1426 | `block → statlist` (with enter/leave scope) |
| `ifstat()` | 1776–1787 | `IF cond THEN block {ELSEIF cond THEN block} [ELSE block] END` |
| `test_then_block()` | 1761–1773 | `[IF|ELSEIF] cond THEN block` |
| `whilestat()` | 1583–1599 | `WHILE cond DO block END` |
| `repeatstat()` | 1602–1624 | `REPEAT block UNTIL cond` |
| `forstat()` | 1743–1758 | `FOR (fornum | forlist) END` |
| `fornum()` | 1694–1713 | `NAME = exp,exp[,exp] forbody` |
| `forlist()` | 1716–1740 | `NAME {,NAME} IN explist forbody` |
| `forbody()` | 1659–1682 | `DO block` (shared by numeric/generic for) |
| `localfunc()` | 1790–1799 | `LOCAL FUNCTION NAME body` |
| `localstat()` | 1827–1868 | `LOCAL attrib NAME attrib {',' NAME attrib} ['=' explist]` |
| `globalstatfunc()` | 1971–1978 | `GLOBAL globalfunc | GLOBAL globalstat` (5.5 only) |
| `globalstat()` | 1940–1953 | `GLOBAL attrib '*' | GLOBAL attrib NAME ...` (5.5 only) |
| `globalfunc()` | 1956–1968 | `GLOBAL FUNCTION NAME body` (5.5 only) |
| `funcstat()` | 1995–2005 | `FUNCTION funcname body` |
| `funcname()` | 1981–1992 | `NAME {fieldsel} [':' NAME]` |
| `exprstat()` | 2008–2023 | `func | assignment` |
| `restassign()` | 1498–1525 | `',' suffixedexp restassign | '=' explist` |
| `retstat()` | 2026–2055 | `RETURN [explist] [';']` |
| `breakstat()` | 1547–1558 | `BREAK` (→ goto "break") |
| `gotostat()` | 1538–1541 | `GOTO NAME` |
| `labelstat()` | 1573–1580 | `'::' NAME '::'` |
| `cond()` | 1528–1535 | `expr` (produces jump list) |

#### Expression-level functions:

| Function | Lines | Grammar Production |
|----------|-------|-------------------|
| `expr()` | 1404–1406 | `expr → subexpr(limit=0)` |
| `subexpr()` | 1374–1401 | `(simpleexp | unop subexpr) { binop subexpr }` |
| `simpleexp()` | 1254–1306 | `FLT | INT | STRING | NIL | TRUE | FALSE | ... | constructor | FUNCTION body | suffixedexp` |
| `primaryexp()` | 1195–1214 | `NAME | '(' expr ')'` |
| `suffixedexp()` | 1217–1251 | `primaryexp { '.' NAME | '[' exp ']' | ':' NAME funcargs | funcargs }` |
| `funcargs()` | 1138–1183 | `'(' [explist] ')' | constructor | STRING` |
| `explist()` | 1125–1135 | `expr { ',' expr }` |

#### Table construction:

| Function | Lines | Grammar Production |
|----------|-------|-------------------|
| `constructor()` | 1028–1054 | `'{' [ field { sep field } [sep] ] '}'` |
| `field()` | 990–1009 | `listfield | recfield` |
| `recfield()` | 936–952 | `(NAME | '['exp']') = exp` |
| `listfield()` | 983–987 | `exp` |
| `fieldsel()` | 887–895 | `['.' | ':'] NAME` |
| `yindex()` | 898–904 | `'[' expr ']'` |

#### Function definition:

| Function | Lines | Grammar Production |
|----------|-------|-------------------|
| `body()` | 1103–1122 | `'(' parlist ')' block END` |
| `parlist()` | 1065–1100 | `[ {NAME ','} (NAME | '...') ]` |

### 3.2 Statement Dispatch: `statement()` (lparser.c lines 2058–2148)

```c
switch (ls->t.token) {
    case ';':        → skip (empty statement)
    case TK_IF:      → ifstat(ls, line)
    case TK_WHILE:   → whilestat(ls, line)
    case TK_DO:      → block(ls) with DO...END
    case TK_FOR:     → forstat(ls, line)
    case TK_REPEAT:  → repeatstat(ls, line)
    case TK_FUNCTION:→ funcstat(ls, line)
    case TK_LOCAL:   → localfunc(ls) or localstat(ls)
    case TK_GLOBAL:  → globalstatfunc(ls, line)          // 5.5 only
    case TK_DBCOLON: → labelstat(ls, ...)
    case TK_RETURN:  → retstat(ls)
    case TK_BREAK:   → breakstat(ls, line)
    case TK_GOTO:    → gotostat(ls, line)
    case TK_NAME:    → (compat) globalstatfunc or...     // 5.5 compat
    default:         → exprstat(ls)
}
```

**Post-statement invariant** (lparser.c line 2144–2146): After every statement, `freereg` is reset to `luaY_nvarstack(fs)` — all temporary registers are freed. This is crucial: statements don't leave values on the stack.

**5.5-specific**: `TK_GLOBAL` case (line 2100) is entirely new. The `TK_NAME` compatibility path (lines 2123–2137) handles `global` as a non-reserved word when `LUA_COMPAT_GLOBAL` is defined — uses lookahead to distinguish `global x` from `global = 5`.

### 3.3 Expression Parsing with Precedence: `subexpr()` (lparser.c lines 1374–1401)

The algorithm is **Pratt-style precedence climbing**:

```c
static BinOpr subexpr(LexState *ls, expdesc *v, int limit) {
    enterlevel(ls);              // recursion depth check
    uop = getunopr(ls->t.token);
    if (uop != OPR_NOUNOPR) {   // prefix unary operator?
        luaX_next(ls);
        subexpr(ls, v, UNARY_PRIORITY);  // parse operand at unary priority
        luaK_prefix(fs, uop, v, line);   // emit unary code
    }
    else simpleexp(ls, v);       // parse atom

    op = getbinopr(ls->t.token);
    while (op != OPR_NOBINOPR && priority[op].left > limit) {
        luaX_next(ls);
        luaK_infix(fs, op, v);           // prepare left operand
        nextop = subexpr(ls, &v2, priority[op].right);  // parse right
        luaK_posfix(fs, op, v, &v2, line); // emit binary code
        op = nextop;
    }
    leavelevel(ls);
    return op;  // return first untreated operator
}
```

#### Precedence Table (lparser.c lines 1351–1365):

```
Priority  Operators              Left  Right  Associativity
  14/13   ^                       14    13    RIGHT
  12      unary (-, ~, not, #)    —     —     PREFIX
  11      * % / //               11    11    LEFT
  10      + -                    10    10    LEFT
   9/8    ..                      9     8    RIGHT
   7      << >>                   7     7    LEFT
   6      &                       6     6    LEFT
   5      ~                       5     5    LEFT
   4      |                       4     4    LEFT
   3      == < <= ~= > >=         3     3    LEFT
   2      and                     2     2    LEFT
   1      or                      1     1    LEFT
```

**Right-associativity trick**: For `^` and `..`, `left > right` means the recursive call with `priority[op].right` (= `left - 1`) will consume the same operator on the right, creating right-associative grouping. For left-associative operators, `left == right`, so the while loop continues at the same level.

**Interaction with lcode.c** — three critical calls per binary operation:
1. `luaK_infix(fs, op, v)` — BETWEEN parsing left and right: prepares left operand (discharge to register, or emit conditional jump for `and`/`or`)
2. `subexpr(ls, &v2, priority[op].right)` — parse right operand
3. `luaK_posfix(fs, op, v, &v2, line)` — AFTER both parsed: emit the actual operation

### 3.4 Variable Resolution

This is the most complex part of the parser. Variable lookup follows a chain:

```
singlevar(ls, var)
  └→ buildvar(ls, name, var)
       └→ singlevaraux(fs, name, var, base=1)  — recursive!
            ├→ searchvar(fs, name, var)         — search locals
            ├→ searchupvalue(fs, name)          — search existing upvalues
            └→ singlevaraux(fs->prev, ...)      — recurse into enclosing function
```

#### `searchvar()` (lparser.c lines 414–444) — Local Variable Lookup

Searches active variables **backwards** (most recent first) in the current function:

```c
for (i = fs->nactvar - 1; i >= 0; i--) {
    Vardesc *vd = getlocalvardesc(fs, i);
    if (varglobal(vd)) {          // global declaration? (5.5)
        // Track collective (*) and named global declarations
    }
    else if (eqstr(n, vd->name)) { // found local?
        if (vd->kind == RDKCTC)     → VCONST
        else if (vd->kind == RDKVAVAR) → VVARGVAR
        else                         → VLOCAL
    }
}
```

#### `singlevaraux()` (lparser.c lines 476–499) — The Full Resolution Chain

Recursive function walking the FuncState chain:

1. `searchvar(fs, name, var)` → found locally?
   - YES, and `base=0` (not the defining function): mark for upvalue capture via `markupval()`
   - YES, `base=1`: return directly
2. NOT found locally:
   - `searchupvalue(fs, name)` → already an upvalue? → return VUPVAL
   - NOT an upvalue, and `fs->prev` exists → recurse into `singlevaraux(fs->prev, name, var, 0)`
   - If recursion found VLOCAL or VUPVAL → create new upvalue via `newupvalue()`
   - If VGLOBAL or VCONST → return as-is (no upvalue needed)

#### `buildvar()` / `buildglobal()` — Global Variable Handling

```c
static void buildvar(LexState *ls, TString *varname, expdesc *var) {
    init_exp(var, VGLOBAL, -1);  // assume global initially
    singlevaraux(fs, varname, var, 1);
    if (var->k == VGLOBAL) {     // still global after search?
        buildglobal(ls, varname, var);  // convert to _ENV[name]
    }
}
```

`buildglobal()` (lparser.c lines 502–513) converts a global variable to `_ENV[name]`:
1. Resolves `_ENV` itself via `singlevaraux()` (it's always upvalue[0] of the main function)
2. Calls `luaK_exp2anyregup()` to get `_ENV` into a register or upvalue
3. Creates a string constant key for the variable name
4. Calls `luaK_indexed()` → produces VINDEXUP or VINDEXSTR

**Resolution result kinds**:
- **VLOCAL**: Found in current function → `var.ridx` = register
- **VVARGVAR**: Vararg parameter found locally (5.5)
- **VCONST**: Compile-time `<const>` → inlined at use sites
- **VUPVAL**: Found via upvalue chain → `var.info` = upvalue index
- **VGLOBAL → VINDEXUP/VINDEXSTR**: Not found → `_ENV[name]`

### 3.5 Block/Scope Management

#### `BlockCnt` Structure (lparser.c lines 49–57):

```c
typedef struct BlockCnt {
    struct BlockCnt *previous;  // chain to enclosing block
    int firstlabel;             // index of first label in this block
    int firstgoto;              // index of first pending goto
    short nactvar;              // active vars at block entry
    lu_byte upval;              // true if some var captured as upvalue
    lu_byte isloop;             // 1=loop, 2=loop with pending breaks
    lu_byte insidetbc;          // true if inside to-be-closed scope
} BlockCnt;
```

#### `enterblock()` (lparser.c lines 720–731):

```c
static void enterblock(FuncState *fs, BlockCnt *bl, lu_byte isloop) {
    bl->isloop = isloop;
    bl->nactvar = fs->nactvar;
    bl->firstlabel = fs->ls->dyd->label.n;
    bl->firstgoto = fs->ls->dyd->gt.n;
    bl->upval = 0;
    bl->insidetbc = (fs->bl != NULL && fs->bl->insidetbc);
    bl->previous = fs->bl;
    fs->bl = bl;
    lua_assert(fs->freereg == luaY_nvarstack(fs));  // CRITICAL invariant
}
```

#### `leaveblock()` (lparser.c lines 745–762):

```c
static void leaveblock(FuncState *fs) {
    BlockCnt *bl = fs->bl;
    lu_byte stklevel = reglevel(fs, bl->nactvar);
    if (bl->previous && bl->upval)          // need OP_CLOSE?
        luaK_codeABC(fs, OP_CLOSE, stklevel, 0, 0);
    fs->freereg = stklevel;                 // free registers
    removevars(fs, bl->nactvar);            // remove block locals
    if (bl->isloop == 2)                    // pending breaks?
        createlabel(ls, ls->brkn, 0, 0);   // create "break" label
    solvegotos(fs, bl);                     // resolve gotos to labels
    fs->bl = bl->previous;
}
```

**Key behaviors**:
- `OP_CLOSE` is emitted only if the block has captured upvalues AND it's not the outermost block
- `break` is implemented as `goto "break"` — the break label is created at block exit
- Gotos are resolved against labels in the closing block; unresolved ones "export" to outer block

### 3.6 Function Definition: `body()` (lparser.c lines 1103–1122)

```c
static void body(LexState *ls, expdesc *e, int ismethod, int line) {
    FuncState new_fs;
    BlockCnt bl;
    new_fs.f = addprototype(ls);        // add Proto to parent's p[]
    new_fs.f->linedefined = line;
    open_func(ls, &new_fs, &bl);        // push new FuncState
    if (ismethod) {
        new_localvarliteral(ls, "self"); // implicit 'self' parameter
        adjustlocalvars(ls, 1);
    }
    parlist(ls);                         // parse parameters
    checknext(ls, ')');
    statlist(ls);                        // parse function body
    new_fs.f->lastlinedefined = ls->linenumber;
    check_match(ls, TK_END, TK_FUNCTION, line);
    codeclosure(ls, e);                  // emit OP_CLOSURE in PARENT
    close_func(ls);                      // pop FuncState, shrink arrays
}
```

**`open_func()`** (lparser.c lines 799–827): Links new FuncState into chain (`fs->prev = ls->fs; ls->fs = fs`), initializes counters, sets `f->maxstacksize = 2`, creates `kcache` table.

**`close_func()`** (lparser.c lines 830–849): Emits final `OP_RETURN`, calls `leaveblock()`, calls `luaK_finish()` for fixups, **shrinks all arrays** to exact size, pops `ls->fs` back to parent.

**`codeclosure()`** (lparser.c lines 792–796): Emits `OP_CLOSURE` in the **parent** function (note: `fs = ls->fs->prev`).

### 3.7 For Loops

#### Numeric For: `fornum()` (lparser.c lines 1694–1713)

Register layout in 5.5:
```
base+0: (for state) — internal counter/initial value
base+1: (for state) — limit
base+2: control variable (user-visible, read-only by default)
```

**🔑 5.5 vs 5.4 difference**: Only 2 internal state variables (not 3 like 5.4). In 5.4, the layout was `init, limit, step, control`. In 5.5, `OP_FORPREP` converts the initial values into a counter-based representation, consuming the step.

#### Generic For: `forlist()` (lparser.c lines 1716–1740)

Register layout:
```
base+0: (for state) — iterator function
base+1: (for state) — state
base+2: (for state) — closing variable (to-be-closed!)
base+3: control variable (first user var)
base+4+: additional user variables
```

The closing variable at `base+2` is marked as to-be-closed via `marktobeclosed(fs)`.

#### `forbody()` (lparser.c lines 1659–1682) — Shared loop body

Uses `OP_FORPREP`/`OP_TFORPREP` for setup and `OP_FORLOOP`/`OP_TFORLOOP` for iteration. Jump encoding uses `SETARG_Bx` (not the normal sJ format), checked against `MAXARG_Bx` limit.

### 3.8 Assignment: `restassign()` (lparser.c lines 1498–1525)

Multi-assignment uses a **recursive linked list**:

```c
struct LHS_assign {
    struct LHS_assign *prev;
    expdesc v;  // the variable being assigned to
};
```

Algorithm for `a, b, c = x, y, z`:
1. Parse `a` → `LHS_assign{prev=NULL, v=a}`
2. See `,` → `restassign()` recursive call
3. Parse `b` → `LHS_assign{prev=&a, v=b}`
4. `check_conflict(ls, &a, &b)` — detect if `b` conflicts with `a`
5. See `,` → recurse again for `c`
6. See `=` → parse explist → `adjust_assign`
7. Unwind: store c, store b, store a (LIFO order!)

**`check_conflict()`** (lparser.c lines 1445–1480): Detects when an assignment target is also used as a table base or index in a previous LHS entry. If conflict found, copies the value to a temporary register. Example: `a[i], i = ...` — old value of `i` is saved before assignment.

### 3.9 Goto/Label Resolution

#### Data structures:

```c
typedef struct Labeldesc {
    TString *name;
    int pc;           // position in code
    int line;
    short nactvar;    // active variables at this point
    lu_byte close;    // true if goto needs CLOSE
} Labeldesc;
```

#### `newgotoentry()` (lparser.c lines 663–668):

Every goto emits JMP followed by a dead `OP_CLOSE` placeholder:
```c
static int newgotoentry(LexState *ls, TString *name, int line) {
    int pc = luaK_jump(fs);                    // emit JMP
    luaK_codeABC(fs, OP_CLOSE, 0, 1, 0);      // placeholder (dead code)
    return newlabelentry(ls, &ls->dyd->gt, name, line, pc);
}
```

When resolved via `closegoto()` (lparser.c lines 597–618):
- If needs `OP_CLOSE`: the two instructions are **swapped** so CLOSE comes before JMP
- If doesn't need CLOSE: the placeholder remains dead code
- Scope validation: `gt->nactvar < label->nactvar` → error "can't jump into scope"

**Break** is syntactic sugar for `goto "break"` using the pre-interned `ls->brkn` string.

### 3.10 Proto Construction During Parsing

| Proto field | Populated by | When |
|-------------|-------------|------|
| `code[]` | `luaK_code()` (lcode.c) | Every instruction emission |
| `k[]` | `luaK_intK()`, `luaK_numberK()`, etc. | When constants needed |
| `p[]` | `addprototype()` (lparser.c line 768) | Nested function parsed |
| `upvalues[]` | `newupvalue()` / `allocupvalue()` | Variable resolution |
| `locvars[]` | `registerlocalvar()` (lparser.c line 175) | Local vars enter scope |
| `lineinfo[]` | `savelineinfo()` (lcode.c) | With each instruction |
| `abslineinfo[]` | `savelineinfo()` (lcode.c) | Periodically |
| `numparams` | `parlist()` (lparser.c line 1093) | After parsing parameters |
| `flag` (vararg) | `setvararg()` (lparser.c line 1060) | If function is vararg |
| `maxstacksize` | `luaK_checkstack()` (lcode.c) | As registers grow |

All arrays are **shrunk to exact size** in `close_func()` using `luaM_shrinkvector`.

### 3.11 Lua 5.5-Specific: Global Declarations

**Entirely new in 5.5.** Three forms:

1. **`global *`** (lparser.c line 1946): Collective declaration — all undeclared names in scope are global. Creates variable with `name=NULL`, kind `GDKREG`.

2. **`global x, y, z`** (via `globalnames()`, lparser.c lines 1924–1937): Named global declarations. Each name registered as `GDKREG` or `GDKCONST`.

3. **`global function f`** (via `globalfunc()`, lparser.c lines 1956–1968): Global function declaration.

After creating a global variable, `checkglobal()` (lparser.c lines 1885–1892) calls `luaK_codecheckglobal()` to emit runtime checking code via `OP_ERRNNIL`.

### 3.12 The Dyndata Shared State

`Dyndata` (actvar, gotos, labels) is shared across ALL nested functions in a compilation unit:

```c
typedef struct Dyndata {
  struct { Vardesc *arr; int n; int size; } actvar;
  Labellist gt;     // pending gotos
  Labellist label;  // active labels
} Dyndata;
```

Each FuncState records its `firstlocal` and `firstlabel` offsets into these arrays. This is why they're "dynamic" — they grow and shrink as functions are entered and left.


---

## 4. Code Generator — lcode.c

### 4.1 Instruction Emission

#### Core: `luaK_code()` (lcode.c lines 384–392)

Every instruction goes through this function:

```c
int luaK_code (FuncState *fs, Instruction i) {
  Proto *f = fs->f;
  luaM_growvector(fs->ls->L, f->code, fs->pc, f->sizecode,
                  Instruction, INT_MAX, "opcodes");
  f->code[fs->pc++] = i;
  savelineinfo(fs, f, fs->ls->lastline);
  return fs->pc - 1;  // returns index of new instruction
}
```

#### Format-specific emitters:

| Function | Lines | Format | Used for |
|----------|-------|--------|----------|
| `luaK_codeABCk()` | 399–404 | iABC | Most instructions |
| `luaK_codevABCk()` | 407–412 | ivABC | OP_NEWTABLE, OP_SETLIST |
| `luaK_codeABx()` | 418–422 | iABx | OP_LOADK, OP_CLOSURE, OP_GETUPVAL |
| `codeAsBx()` | 428–433 | iAsBx | OP_LOADI, OP_LOADF, OP_FORLOOP |
| `codesJ()` | 439–444 | isJ | OP_JMP |
| `codeextraarg()` | 450–453 | iAx | OP_EXTRAARG (25-bit argument) |

#### `luaK_codek()` — Load constant (lcode.c lines 461–469):
```c
static int luaK_codek (FuncState *fs, int reg, int k) {
  if (k <= MAXARG_Bx)
    return luaK_codeABx(fs, OP_LOADK, reg, k);
  else {
    int p = luaK_codeABx(fs, OP_LOADKX, reg, 0);
    codeextraarg(fs, k);
    return p;
  }
}
```
If constant index fits in 17-bit Bx → `OP_LOADK`. Otherwise → `OP_LOADKX` + `OP_EXTRAARG` (25-bit index, supports up to ~33M constants).

#### Line Info: `savelineinfo()` (lcode.c lines 331–346)

Uses **delta encoding**: stores line difference from previous as a signed byte. When delta exceeds ±127 (`LIMLINEDIFF = 0x80`) or after `MAXIWTHABS` instructions, stores absolute line info. `removelastlineinfo()` (lines 355–367) reverses line info when removing an instruction.

#### `previousinstruction()` (lcode.c lines 117–123)

Returns pointer to the last emitted instruction, BUT returns an invalid instruction if `fs->pc <= fs->lasttarget` (a jump target exists between current and previous instruction). This prevents incorrect optimizations across basic block boundaries.

### 4.2 Register Allocation

The register allocator is **trivially simple** — a stack pointer:

```
┌─────────────────────────────────────────────┐
│  Register 0   │  local 'a'                  │
│  Register 1   │  local 'b'                  │
│  Register 2   │  local 'c'                  │  ← nactvar = 3
│  Register 3   │  (temporary)                │
│  Register 4   │  (temporary)                │  ← freereg = 5
│  Register 5   │  (free)                     │
│  ...          │  (free)                     │
└─────────────────────────────────────────────┘
```

#### `luaK_reserveregs()` (lcode.c lines 488–491):
```c
void luaK_reserveregs (FuncState *fs, int n) {
  luaK_checkstack(fs, n);
  fs->freereg = cast_byte(fs->freereg + n);
}
```

#### `freereg()` (lcode.c lines 499–504):
```c
static void freereg (FuncState *fs, int reg) {
  if (reg >= luaY_nvarstack(fs)) {  // only free temporaries
    fs->freereg--;
    lua_assert(reg == fs->freereg);  // LIFO discipline!
  }
}
```
**Critical**: Only frees if `reg >= nvarstack` (temporary, not a local). Asserts LIFO order.

#### `freeexps()` (lcode.c lines 535–539):
Frees registers for two expressions, freeing the higher-numbered one first to maintain LIFO order.

### 4.3 The Expression Discharge Chain

This is the **heart of the code generator**. The discharge chain converts abstract `expdesc` descriptors into concrete register values:

```
VLOCAL/VUPVAL/VINDEXED/...
    │
    ▼ luaK_dischargevars()
VNONRELOC/VRELOC/VK/VKINT/VKFLT/VKSTR
    │
    ▼ discharge2reg()
VNONRELOC (value in specific register)
    │
    ▼ exp2reg() — also handles jump list patching
VNONRELOC (final, with t/f lists resolved)
```

#### `luaK_dischargevars()` (lcode.c lines 819–874)

Converts variable-kind expressions into values:

| Input kind | Action | Output kind |
|---|---|---|
| `VCONST` | Converts compile-time constant to value | VKINT/VKFLT/VKSTR/etc. |
| `VVARGVAR` | `luaK_vapar2local()`, sets `needvatab` | → VLOCAL → VNONRELOC |
| `VLOCAL` | Copies `var.ridx` to `info` | `VNONRELOC` |
| `VUPVAL` | Emits `OP_GETUPVAL` | `VRELOC` |
| `VINDEXUP` | Emits `OP_GETTABUP` | `VRELOC` |
| `VINDEXI` | Emits `OP_GETI`, frees table reg | `VRELOC` |
| `VINDEXSTR` | Emits `OP_GETFIELD`, frees table reg | `VRELOC` |
| `VINDEXED` | Emits `OP_GETTABLE`, frees both regs | `VRELOC` |
| `VVARGIND` | Emits `OP_GETVARG`, frees both regs | `VRELOC` |
| `VVARARG`/`VCALL` | `luaK_setoneret()` | VNONRELOC/VRELOC |

**Key**: `VRELOC` means "instruction emitted but destination register (A field) not yet assigned."

#### `discharge2reg()` (lcode.c lines 882–929)

Puts expression value into a specific register:

| Input kind | Opcode emitted |
|---|---|
| `VNIL` | OP_LOADNIL (with merging optimization) |
| `VFALSE` | OP_LOADFALSE |
| `VTRUE` | OP_LOADTRUE |
| `VK` | OP_LOADK or OP_LOADKX+EXTRAARG |
| `VKFLT` | OP_LOADF (if fits sBx) or OP_LOADK |
| `VKINT` | OP_LOADI (if fits sBx) or OP_LOADK |
| `VRELOC` | Sets A field of existing instruction |
| `VNONRELOC` | OP_MOVE (if register differs) |

#### `exp2reg()` (lcode.c lines 971–993) — Full pipeline with jump lists

1. Calls `discharge2reg()`
2. If VJMP, adds jump to `t` list
3. If expression has jumps (`t != f`):
   - Checks if any jumps need value loading (`need_value()`)
   - Emits `OP_LFALSESKIP` + `OP_LOADTRUE` sequence
   - Patches `f` list to LFALSESKIP, `t` list to LOADTRUE
   - Uses `patchlistaux()` with separate vtarget/dtarget

#### Key discharge helpers:

| Function | Lines | Purpose |
|----------|-------|---------|
| `luaK_exp2nextreg()` | 999–1004 | Allocate next register, discharge there |
| `luaK_exp2anyreg()` | 1011–1026 | Reuse existing register if possible |
| `luaK_exp2anyregup()` | 1033–1036 | Keep VUPVAL as-is if no jumps |
| `luaK_exp2val()` | 1043–1048 | Minimal discharge (just ensure has value) |
| `luaK_exp2K()` | 1055–1076 | Try to convert to VK constant |
| `exp2RK()` | 1085–1092 | Try K first, fall back to register |

### 4.4 Constant Handling

#### The constant cache: `k2proto()` (lcode.c lines 565–583)

```c
static int k2proto (FuncState *fs, TValue *key, TValue *v) {
  TValue val;
  int tag = luaH_get(fs->kcache, key, &val);
  if (!tagisempty(tag)) {  // cache hit
    return cast_int(ivalue(&val));  // reuse index
  } else {  // cache miss
    int k = addk(fs, fs->f, v);
    setivalue(&val, k);
    luaH_set(fs->ls->L, fs->kcache, key, &val);
    return k;
  }
}
```

Uses `fs->kcache` (a Lua Table) to deduplicate constants. The key and value can differ.

#### Special constant cases:

- **`luaK_numberK()`** (lcode.c lines 617–639): **Tricky float caching**. Floats with integral values would collide with integer keys. Solution: perturbs the key by adding the smallest significant fraction. For `r == 0`: uses `FuncState*` pointer as key (guaranteed unique per function).
- **`nilK()`** (lcode.c lines 660–671): Cannot use nil as table key. Uses `fs->kcache` table pointer itself as key.
- **`boolF()`/`boolT()`**: Use false/true TValues as keys.

#### Loading constants to registers:
- `luaK_int()` (lcode.c lines 692–697): If fits sBx → `OP_LOADI`. Else → `OP_LOADK`.
- `luaK_float()` (lcode.c lines 700–706): If integral and fits sBx → `OP_LOADF`. Else → `OP_LOADK`.

### 4.5 Constant Folding (lcode.c lines 1395–1437)

```c
static int constfolding (FuncState *fs, int op, expdesc *e1,
                         const expdesc *e2) {
  TValue v1, v2, res;
  if (!tonumeral(e1, &v1) || !tonumeral(e2, &v2) ||
      !validop(op, &v1, &v2))
    return 0;
  luaO_rawarith(fs->ls->L, op, &v1, &v2, &res);
  if (ttisinteger(&res)) {
    e1->k = VKINT; e1->u.ival = ivalue(&res);
  } else {
    lua_Number n = fltvalue(&res);
    if (luai_numisnan(n) || n == 0)
      return 0;  // don't fold NaN or 0.0
    e1->k = VKFLT; e1->u.nval = n;
  }
  return 1;
}
```

**Rules**:
1. Both operands must be numeric constants with no jumps
2. Operation must be safe: `validop()` rejects division by zero, and bitwise ops require integer-convertible operands
3. **Does NOT fold NaN** — NaN has special comparison behavior
4. **Does NOT fold 0.0** — to avoid problems with -0.0 vs +0.0
5. Only foldable operations: arithmetic + bitwise (`foldbinop(opr)` = `opr <= OPR_SHR`)

**Where invoked**:
- `luaK_prefix()` (lcode.c line 1703): for unary minus and bitwise not
- `luaK_posfix()` (lcode.c line 1791): for all foldable binary ops

### 4.6 Jump Patching

#### Jump list threading

Jump lists are **threaded through the sJ field of JMP instructions**. Each JMP's sJ field points to the next JMP in the list (as a relative offset). `NO_JUMP (-1)` marks the end.

```
Jump list: jmp1 → jmp2 → jmp3 → NO_JUMP
           │         │         │
           sJ=offset sJ=offset sJ=-1
```

#### Core functions:

| Function | Lines | Purpose |
|----------|-------|---------|
| `getjump()` | 155–161 | Follow one link: `(pc+1) + offset` |
| `fixjump()` | 168–176 | Set JMP target: `offset = dest - (pc+1)` |
| `luaK_concat()` | 182–193 | Concatenate two jump lists |
| `luaK_jump()` | 200–202 | Emit JMP with NO_JUMP (to be patched) |
| `luaK_getlabel()` | 234–237 | Mark current pc as jump target |
| `luaK_patchlist()` | 308–311 | Patch all jumps in list to target |
| `luaK_patchtohere()` | 314–317 | Patch list to current pc |

#### `patchlistaux()` (lcode.c lines 290–300) — The core patching logic

```c
static void patchlistaux (FuncState *fs, int list,
                          int vtarget, int reg, int dtarget) {
  while (list != NO_JUMP) {
    int next = getjump(fs, list);
    if (patchtestreg(fs, list, reg))
      fixjump(fs, list, vtarget);  // TESTSET → value target
    else
      fixjump(fs, list, dtarget);  // other → default target
    list = next;
  }
}
```

**Two targets**: `vtarget` for jumps that produce values (TESTSET), `dtarget` for others. This is how `and`/`or` short-circuit evaluation works.

#### `getjumpcontrol()` (lcode.c lines 245–251)

Returns the instruction that "controls" a jump. If the instruction before the JMP is a test-mode instruction (`testTMode`), returns that instruction. This is how conditional jumps work: TEST/TESTSET/comparison + JMP.

### 4.7 Conditional Code Generation

#### `luaK_goiftrue()` (lcode.c lines 1178–1199)

"Go through if true, jump if false":
- `VJMP` → negate condition, use existing jump
- `VK/VKFLT/VKINT/VKSTR/VTRUE` → always true, `pc = NO_JUMP`
- Default → `jumponcond(fs, e, 0)` (jump when false)
- Adds jump to `e->f` (false list), patches `e->t` to here

#### `luaK_goiffalse()` (lcode.c lines 1205–1225)

Mirror: "Go through if false, jump if true":
- `VJMP` → use existing jump directly
- `VNIL/VFALSE` → always false, `pc = NO_JUMP`
- Default → `jumponcond(fs, e, 1)` (jump when true)

#### `jumponcond()` (lcode.c lines 1160–1172)

Emits conditional jump:
1. If VRELOC with OP_NOT → **optimization**: removes NOT, emits `TEST` with inverted condition
2. Otherwise → discharges to register, emits `TESTSET`

#### `codenot()` (lcode.c lines 1231–1259)

Constant folding for `not`:
- `VNIL/VFALSE` → `VTRUE`
- `VK/VKFLT/VKINT/VKSTR/VTRUE` → `VFALSE`
- `VJMP` → negate condition
- `VRELOC/VNONRELOC` → emit `OP_NOT`
- **Always**: swaps `t` and `f` lists

### 4.8 Binary Operations: `luaK_posfix()` (lcode.c lines 1788–1862)

#### The two-instruction pattern for arithmetic

Every arithmetic/bitwise binary op emits TWO instructions:
1. The operation itself (e.g., `OP_ADD`, `OP_ADDK`, `OP_ADDI`)
2. A metamethod fallback (`OP_MMBIN`, `OP_MMBINI`, or `OP_MMBINK`)

```c
// finishbinexpval() (lcode.c lines 1489–1500)
int pc = luaK_codeABCk(fs, op, 0, v1, v2, 0);     // arithmetic op
luaK_codeABCk(fs, mmop, v1, v2, cast_int(event), flip); // metamethod
```

The `flip` flag tells the VM that operands were swapped (so metamethod receives them in original order).

#### Operator dispatch:

| Operator | Special handling |
|----------|-----------------|
| `+`, `*` | Commutative: swap if e1 is constant. Try `OP_ADDI` for small int |
| `-` | Try `OP_ADDI` with negated immediate. Correct MMBINI argument |
| `/`, `//`, `%`, `^` | Non-commutative, no immediate variants |
| `&`, `\|`, `~` | Commutative: swap if e1 is VKINT. Try K operand |
| `<<` | If e1 is small int → `OP_SHLI`. If e2 is small int → `OP_SHRI` with negation |
| `>>` | If e2 is small int → `OP_SHRI`. Else register-register |
| `==`, `~=` | Try `OP_EQI`, `OP_EQK`, `OP_EQ` |
| `>`, `>=` | **Swap operands** → convert to `<`/`<=` |
| `<`, `<=` | Try `OP_LTI`/`OP_LEI`, reversed `OP_GTI`/`OP_GEI`, or register `OP_LT`/`OP_LE` |
| `and` | Short-circuit: concatenate `e1->f` into `e2->f` |
| `or` | Short-circuit: concatenate `e1->t` into `e2->t` |
| `..` | Merge adjacent CONCAT ops by incrementing B count |

#### `luaK_infix()` — First operand preparation (lcode.c lines 1719–1761)

Called BEFORE reading the second operand:
- `and` → `luaK_goiftrue(e)` (short-circuit: skip if false)
- `or` → `luaK_goiffalse(e)` (short-circuit: skip if true)
- `..` → `luaK_exp2nextreg(e)` (must be on stack)
- Arithmetic/bitwise → if numeric constant, keep as-is (for folding/immediate)
- Equality → try `exp2RK()` (constant or register)
- Order → if fits sC number, keep; else register

### 4.9 Unary Operations: `luaK_prefix()` (lcode.c lines 1698–1712)

```c
void luaK_prefix (FuncState *fs, UnOpr opr, expdesc *e, int line) {
  static const expdesc ef = {VKINT, {0}, NO_JUMP, NO_JUMP};
  luaK_dischargevars(fs, e);
  switch (opr) {
    case OPR_MINUS: case OPR_BNOT:
      if (constfolding(fs, opr + LUA_OPUNM, e, &ef)) break;
      /* FALLTHROUGH */
    case OPR_LEN:
      codeunexpval(fs, unopr2op(opr), e, line);  // OP_UNM, OP_BNOT, OP_LEN
      break;
    case OPR_NOT: codenot(fs, e); break;
  }
}
```

### 4.10 Store Operations: `luaK_storevar()` (lcode.c lines 1105–1140)

| Variable kind | Opcode | A field | B field | C field |
|---|---|---|---|---|
| VLOCAL | (direct or OP_MOVE) | local's register | — | — |
| VUPVAL | OP_SETUPVAL | value reg | upvalue index | — |
| VINDEXUP | OP_SETTABUP | upvalue index | key K index | value (R/K) |
| VINDEXI | OP_SETI | table reg | int key | value (R/K) |
| VINDEXSTR | OP_SETFIELD | table reg | key K index | value (R/K) |
| VINDEXED | OP_SETTABLE | table reg | key reg | value (R/K) |
| VVARGIND | OP_SETTABLE | vararg reg | key reg | value (R/K) |

**VLOCAL optimization**: Uses `exp2reg()` to compute directly into the local's register — no MOVE needed if the expression can be relocated.

### 4.11 Table Construction

#### `luaK_settablesize()` (lcode.c lines 1875–1883)

```c
void luaK_settablesize (FuncState *fs, int pc, int ra,
                        int asize, int hsize) {
  int extra = asize / (MAXARG_vC + 1);  // higher bits
  int rc = asize % (MAXARG_vC + 1);     // lower bits
  hsize = (hsize != 0) ? luaO_ceillog2(hsize) + 1 : 0;
  *inst = CREATE_vABCk(OP_NEWTABLE, ra, hsize, rc, k);
  *(inst + 1) = CREATE_Ax(OP_EXTRAARG, extra);
}
```

- **Hash size**: logarithmic encoding (`ceil(log2(hsize)) + 1`) in B field
- **Array size**: split between vC (lower bits) and EXTRAARG (higher bits)
- Always followed by EXTRAARG (even if extra=0)

#### `luaK_setlist()` (lcode.c lines 1893–1906)

- `tostore = 0` means MULTRET (store all values up to stack top)
- Large `nelems` uses EXTRAARG for overflow
- **Resets freereg** to `base + 1` after storing

### 4.12 Self Calls: `luaK_self()` (lcode.c lines 1321–1341)

Two paths:
- **Short string key in K range** → single `OP_SELF` instruction
- **Long string or out of range** → `OP_MOVE` (copy receiver) + `OP_GETTABLE` (get method)

### 4.13 `luaK_finish()` — Final Pass (lcode.c lines 1929–1972)

Five critical fixups:

1. **RETURN0/RETURN1 → RETURN**: If function needs to close upvalues (`needclose`) or uses hidden vararg arguments (`PF_VAHID`), upgrades simple returns to full `OP_RETURN`

2. **RETURN/TAILCALL k-flag**: Sets `k=1` if function has to-be-closed variables

3. **RETURN/TAILCALL C-field**: Sets `C=numparams+1` if function uses hidden vararg args

4. **GETVARG → GETTABLE**: If function uses vararg table (`PF_VATAB`), converts vararg indexing to regular table access

5. **JMP chain optimization**: `finaltarget()` (lcode.c lines 1912–1922) follows chains of JMPs (up to 100 hops) to find final target, patches directly

### 4.14 Global Variable Checking: `luaK_codecheckglobal()` (lcode.c lines 715–722)

**New in Lua 5.5!**

```c
void luaK_codecheckglobal (FuncState *fs, expdesc *var, int k, int line) {
  luaK_exp2anyreg(fs, var);
  k = (k >= MAXARG_Bx) ? 0 : k + 1;
  luaK_codeABx(fs, OP_ERRNNIL, var->u.info, k);
  freeexp(fs, var);
}
```

Emits `OP_ERRNNIL` which checks if a global variable is nil and raises an error. Bx field contains `k+1` (constant index of variable name for error message), or 0 if name doesn't fit.


---

## 5. Upvalue Resolution

### 5.1 The FuncState Chain

Nested functions create a linked list of FuncStates via `fs->prev`:

```
main function (fs3)          ← ls->fs
  └→ outer function (fs2)    ← fs3->prev
       └→ inner function (fs1) ← fs2->prev
            └→ NULL            ← fs1->prev
```

When variable `x` is referenced in `fs3` but defined in `fs1`, the resolution walks the chain:

1. `singlevaraux(fs3, "x", var, 1)` — not found in fs3
2. `singlevaraux(fs2, "x", var, 0)` — not found in fs2
3. `singlevaraux(fs1, "x", var, 0)` — found as VLOCAL in fs1!
4. Back in fs2: `newupvalue(fs2, "x", var)` — creates upvalue `instack=1, idx=register_of_x_in_fs1`
5. Back in fs3: `newupvalue(fs3, "x", var)` — creates upvalue `instack=0, idx=upvalue_index_of_x_in_fs2`

### 5.2 The `newupvalue()` Function (lparser.c lines 382–400)

```c
static int newupvalue(FuncState *fs, TString *name, expdesc *v) {
    Upvaldesc *up = allocupvalue(fs);
    FuncState *prev = fs->prev;
    if (v->k == VLOCAL) {
        up->instack = 1;                    // references parent's register
        up->idx = v->u.var.ridx;            // which register
        up->kind = getlocalvardesc(prev, v->u.var.vidx)->vd.kind;
    }
    else {  // v->k == VUPVAL
        up->instack = 0;                    // references parent's upvalue
        up->idx = cast_byte(v->u.info);     // which upvalue index
        up->kind = prev->f->upvalues[v->u.info].kind;
    }
    up->name = name;
    return fs->nups - 1;
}
```

### 5.3 The instack/idx Encoding in Upvaldesc

```c
typedef struct Upvaldesc {
  TString *name;     // upvalue name (for debug)
  lu_byte instack;   // 1 = captures parent's register; 0 = captures parent's upvalue
  lu_byte idx;       // index of register or upvalue in parent
  lu_byte kind;      // variable kind (VDKREG, RDKCONST, etc.)
} Upvaldesc;
```

**Two cases**:
- `instack=1, idx=N`: Captures register N from immediately enclosing function (the variable is a local there)
- `instack=0, idx=N`: Captures upvalue N from immediately enclosing function (the variable was already an upvalue there)

**Chain example** for `x` defined in function A, used in function C (two levels deep):
```
Function A: local x  (register 3)
Function B: upvalue[0] = {instack=1, idx=3}   -- captures A's register 3
Function C: upvalue[0] = {instack=0, idx=0}   -- captures B's upvalue 0
```

### 5.4 BlockCnt and Upvalue Closing

#### `markupval()` (lparser.c lines 451–457)

When a local is captured as an upvalue, the block containing that local is marked:

```c
static void markupval(FuncState *fs, int level) {
    BlockCnt *bl = fs->bl;
    while (bl->nactvar > level)  // find block owning this variable
        bl = bl->previous;
    bl->upval = 1;               // mark: has upvalue-captured vars
    fs->needclose = 1;           // function needs close instructions
}
```

This drives three things:
1. `leaveblock()` emits `OP_CLOSE` when leaving a block with `bl->upval == 1`
2. `luaK_finish()` upgrades RETURN0/RETURN1 to full RETURN when `fs->needclose`
3. Goto resolution checks whether jumps cross upvalue-captured scopes

### 5.5 The _ENV Upvalue

Every global variable access goes through `_ENV`:

```
Global variable "print" in nested function:
  1. Parser resolves "print" → not found locally
  2. buildglobal() resolves "_ENV" → finds it as upvalue[0] of main function
  3. If in nested function: _ENV itself is captured as an upvalue chain
  4. luaK_indexed() creates: _ENV["print"] → VINDEXUP or VINDEXSTR
  5. Code gen emits: OP_GETTABUP (upvalue table, string key)
```

---

## 6. MULTRET Handling

### 6.1 The MULTRET Encoding

Lua uses a simple encoding: field value 0 means "multiple returns" (MULTRET). Since 0 would normally mean "0 arguments/results", all counts are stored as `count + 1`:

- `B = 0` in OP_CALL/OP_TAILCALL: pass all arguments up to stack top
- `C = 0` in OP_CALL: receive all return values (expand to fill)
- `B = 0` in OP_RETURN: return all values from first to stack top
- `C = 0` (tostore) in OP_SETLIST: store all values from base to stack top

### 6.2 Function Calls (lparser.c lines 1138–1183)

In `funcargs()`:

```c
// lparser.c line 1171-1172
if (hasmultret(args.k))
    nparams = LUA_MULTRET;  // open call — B=0
```

When the last argument is a function call or `...`, the parser sets `nparams = LUA_MULTRET`, which causes `OP_CALL` to be emitted with `B = 0`.

The call instruction is created at lparser.c line 1178:
```c
init_exp(f, VCALL, luaK_codeABC(fs, OP_CALL, base, nparams+1, 2));
```
Default `C = 2` (one result). Callers adjust via `luaK_setreturns()` or `luaK_setmultret()`.

### 6.3 `luaK_setreturns()` (lcode.c lines 757–768)

```c
void luaK_setreturns (FuncState *fs, expdesc *e, int nresults) {
  Instruction *pc = &getinstruction(fs, e);
  if (e->k == VCALL)
    SETARG_C(*pc, nresults + 1);  // C=0 means MULTRET
  else {  // VVARARG
    SETARG_C(*pc, nresults + 1);
    SETARG_A(*pc, fs->freereg);
    luaK_reserveregs(fs, 1);
  }
}
```

### 6.4 `adjust_assign()` — The MULTRET Integration Point (lparser.c lines 547–567)

```c
static void adjust_assign (LexState *ls, int nvars, int nexps, expdesc *e) {
  int needed = nvars - nexps;
  if (hasmultret(e->k)) {  // last expression has multiple returns?
    int extra = needed + 1;
    if (extra < 0) extra = 0;
    luaK_setreturns(fs, e, extra);  // last exp provides the difference
  }
  else {
    if (e->k != VVOID) luaK_exp2nextreg(fs, e);
    if (needed > 0) luaK_nil(fs, fs->freereg, needed);  // pad with nils
  }
}
```

This is how `local a, b, c = f()` works: `f()` is the last expression with MULTRET, and `adjust_assign` sets its return count to 3.

### 6.5 Tail Calls (lparser.c lines 2036–2042)

```c
// In retstat():
if (hasmultret(e.k)) {
    luaK_setmultret(fs, &e);
    if (e.k == VCALL && nret == 1 && !fs->bl->insidetbc) {
        SET_OPCODE(getinstruction(fs, &e), OP_TAILCALL);
    }
    nret = LUA_MULTRET;
}
```

Tail call detection: single VCALL return, not inside a to-be-closed scope → upgrade `OP_CALL` to `OP_TAILCALL`.

### 6.6 SETLIST with MULTRET (lparser.c lines 969–971)

In table constructors, when the last field is a function call or vararg:
```c
if (hasmultret(cc->v.k)) {
    luaK_setmultret(fs, &cc->v);
    luaK_setlist(fs, cc->t->u.info, cc->na, LUA_MULTRET);
}
```

---

## 7. Vararg Handling

### 7.1 Two Vararg Modes in 5.5

Lua 5.5 has two distinct vararg representations:

1. **Hidden vararg arguments** (`PF_VAHID`): The traditional approach. Varargs are passed on the stack above the fixed parameters. `OP_VARARGPREP` adjusts the stack frame at function entry. `OP_VARARG` copies vararg values to the destination.

2. **Vararg table** (`PF_VATAB`): New in 5.5. When the vararg parameter is named (e.g., `function f(a, ...args)`), the varargs are collected into a table. `OP_GETVARG` accesses individual elements.

### 7.2 `setvararg()` (lparser.c lines 1059–1061)

```c
static void setvararg (FuncState *fs) {
  fs->f->flag |= PF_VAHID;  // by default, use hidden vararg arguments
  luaK_codeABC(fs, OP_VARARGPREP, 0, 0, 0);
}
```

Always called for vararg functions (including the main function). Sets `PF_VAHID` and emits `OP_VARARGPREP` as the first instruction.

### 7.3 Parsing `...` in Parameter Lists (lparser.c lines 1065–1100)

```c
// In parlist():
if (ls->t.token == TK_DOTS) {
    varargk = 1;
    luaX_next(ls);
    if (testnext(ls, TK_NAME)) {  // named vararg: ...name
        new_varkind(ls, str_checkname(ls), RDKVAVAR);
    } else {
        new_localvarliteral(ls, "(vararg table)");  // unnamed
    }
}
if (varargk) {
    setvararg(fs);
    adjustlocalvars(ls, 1);  // vararg parameter
}
```

**5.5 addition**: After `...`, an optional NAME creates a named vararg parameter with kind `RDKVAVAR`. If no name follows, a placeholder `"(vararg table)"` is created.

### 7.4 Using `...` in Expressions (lparser.c lines 1284–1288)

```c
case TK_DOTS: {
    check_condition(ls, isvararg(fs->f),
                    "cannot use '...' outside a vararg function");
    init_exp(v, VVARARG, luaK_codeABC(fs, OP_VARARG, 0, fs->f->numparams, 1));
    break;
}
```

Creates a VVARARG expression. The `OP_VARARG` instruction's B field = `numparams` (used by the VM to locate varargs on the stack).

### 7.5 The PF_VAHID / PF_VATAB Interaction

In `luaK_finish()` (lcode.c lines 1929–1972):

```c
if (p->flag & PF_VATAB)      // uses vararg table?
    p->flag &= ~PF_VAHID;    // then NOT hidden args

// For each instruction:
case OP_GETVARG:
    if (p->flag & PF_VATAB)
        SET_OPCODE(*pc, OP_GETTABLE);  // convert to table access
case OP_VARARG:
    if (p->flag & PF_VATAB)
        SETARG_k(*pc, 1);  // signal vararg table mode
case OP_RETURN: case OP_TAILCALL:
    if (p->flag & PF_VAHID)
        SETARG_C(*pc, p->numparams + 1);  // hidden args count
```

**The logic**:
- If `PF_VATAB` is set (named vararg parameter is indexed), the function uses a vararg table → clear `PF_VAHID` (not hidden anymore)
- `OP_GETVARG` is converted to `OP_GETTABLE` (access the vararg table)
- `OP_VARARG` gets `k=1` flag to signal vararg table mode
- Returns with `PF_VAHID` get `C=numparams+1` so the VM knows how many hidden args to handle

### 7.6 `luaK_vapar2local()` (lcode.c lines 808–812)

```c
void luaK_vapar2local (FuncState *fs, expdesc *var) {
    needvatab(fs->f);  // sets PF_VATAB flag
    // vararg parameter becomes equivalent to a regular local variable
}
```

When a vararg parameter (VVARGVAR) is used in a context that requires a regular value (e.g., assigned to, or used as a table), it's converted to a regular local and the function is marked as needing a vararg table.


---

## 8. If I Were Building This in Go

### 8.1 Recursive Descent Parser

Go is an excellent fit for recursive descent parsing. The C code maps almost 1:1:

```go
// Token types as an enum
type TokenType int

const (
    TK_AND TokenType = iota + 256
    TK_BREAK
    TK_DO
    // ... all reserved words
    TK_FLT
    TK_INT
    TK_NAME
    TK_STRING
)

// Lexer state
type Lexer struct {
    current  rune          // current character
    line     int           // current line
    lastLine int           // line of last consumed token
    token    Token         // current token
    ahead    Token         // lookahead token
    hasAhead bool          // whether lookahead is populated
    reader   *bufio.Reader // input source
    buf      strings.Builder // token buffer
    strings  map[string]string // string interning (Go strings are immutable!)
    source   string
}

// Parser state
type Parser struct {
    lex      *Lexer
    fs       *FuncState
    dyd      *Dyndata
}
```

**Key differences from C**:
- Go strings are immutable and comparable — no need for the `TString.extra` hack for reserved words. Use a `map[string]TokenType` instead.
- Go has garbage collection — no need for the `ls->h` anchor table.
- Go's `rune` type naturally handles Unicode — simplifies character classification.
- Error handling: use `panic`/`recover` instead of `longjmp` for syntax errors.

### 8.2 The expdesc Equivalent

The expdesc pattern maps well to Go with a tagged union using interfaces or a struct with a kind field:

```go
type ExpKind int

const (
    VVOID ExpKind = iota
    VNIL
    VTRUE
    VFALSE
    VK
    VKFLT
    VKINT
    VKSTR
    VNONRELOC
    VLOCAL
    VUPVAL
    VINDEXED
    VINDEXUP
    VINDEXI
    VINDEXSTR
    VJMP
    VRELOC
    VCALL
    VVARARG
)

type ExpDesc struct {
    Kind   ExpKind
    Info   int       // generic info (register, pc, upvalue index, etc.)
    IVal   int64     // for VKINT
    NVal   float64   // for VKFLT
    StrVal string    // for VKSTR
    Ind    IndexInfo // for indexed variables
    Var    VarInfo   // for local variables
    T      int       // true jump list
    F      int       // false jump list
}

type IndexInfo struct {
    Table    byte // table register or upvalue
    Idx      int  // key index
    ReadOnly bool // read-only flag
    KeyStr   int  // K index of string key, or -1
}

type VarInfo struct {
    RegIdx byte  // register holding the variable
    VarIdx int16 // index in actvar array
}
```

**Design choice**: Use a flat struct with a Kind field rather than Go interfaces. The expdesc is mutated frequently and its fields overlap — an interface-based approach would require too many type assertions and allocations.

### 8.3 Register Allocator Design

The C register allocator is so simple it maps trivially:

```go
type FuncState struct {
    Proto     *Proto
    Prev      *FuncState
    Lexer     *Lexer
    Block     *BlockCnt
    KCache    map[interface{}]int // constant dedup cache
    PC        int
    FreeReg   byte
    NumActVar int16
    NumUps    byte
    NeedClose bool
    // ... other fields
}

func (fs *FuncState) ReserveRegs(n int) {
    fs.CheckStack(n)
    fs.FreeReg += byte(n)
}

func (fs *FuncState) FreeReg(reg int) {
    if reg >= fs.NumVarStack() { // only free temporaries
        fs.FreeReg--
        // assert reg == fs.FreeReg
    }
}
```

**Go advantage**: The `KCache` can be a `map[interface{}]int` where keys are `int64`, `float64`, `string`, `bool`, or a sentinel for nil. No need for the tricky float caching workaround — Go maps handle float keys correctly (NaN is handled specially by Go's map implementation, but we'd avoid NaN keys anyway).

### 8.4 Jump List Management

```go
const NoJump = -1

func (fs *FuncState) GetJump(pc int) int {
    offset := fs.Proto.Code[pc].GetSJ()
    if offset == NoJump {
        return NoJump
    }
    return (pc + 1) + offset
}

func (fs *FuncState) FixJump(pc, dest int) {
    offset := dest - (pc + 1)
    if offset > MaxSJ || offset < MinSJ {
        fs.Lexer.SyntaxError("control structure too long")
    }
    fs.Proto.Code[pc].SetSJ(offset)
}

func (fs *FuncState) PatchList(list, target int) {
    for list != NoJump {
        next := fs.GetJump(list)
        fs.FixJump(list, target)
        list = next
    }
}
```

### 8.5 Constant Pool

```go
type Proto struct {
    Code         []Instruction
    Constants    []Value       // replaces k[]
    InnerProtos  []*Proto      // replaces p[]
    Upvalues     []UpvalDesc
    LocalVars    []LocalVar    // debug info
    LineInfo     []int8        // delta encoding
    AbsLineInfo  []AbsLine    // periodic absolute
    NumParams    int
    MaxStack     int
    Source       string
    // ...
}

func (fs *FuncState) AddConstant(v Value) int {
    // Check cache first
    key := constantKey(v)
    if idx, ok := fs.KCache[key]; ok {
        return idx
    }
    idx := len(fs.Proto.Constants)
    fs.Proto.Constants = append(fs.Proto.Constants, v)
    fs.KCache[key] = idx
    return idx
}
```

### 8.6 Architecture Recommendations

1. **Single-pass compilation**: Keep the C architecture. Don't be tempted to add an AST intermediate representation — the expdesc pattern is more efficient and the C code proves it works.

2. **Error handling**: Use `panic` with a custom error type, `recover` at the top-level `Parse()` function. This mirrors C's `longjmp` pattern.

3. **String interning**: Go strings are already immutable and comparable. For the constant pool, use `map[string]int` for dedup. No need for a separate string table.

4. **Instruction encoding**: Use `uint32` for instructions, with methods for field extraction/insertion. Consider a type alias: `type Instruction uint32`.

5. **Memory management**: Go's GC handles everything. No need for `luaC_fix`, anchor tables, or careful stack management for GC safety.

6. **Testing strategy**: Port the Lua test suite. Each grammar production should have targeted tests. The compiler is the hardest part to get right — invest heavily in testing.

---

## 9. Edge Cases and Traps

### 9.1 Maximum Constants: MAXARG_Ax (~33 million)

Constants are stored in `Proto->k[]` with indices up to `MAXARG_Ax` (25 bits = 33,554,431). The limit is checked in `addk()` (lcode.c line 549). For indices > `MAXARG_Bx` (17 bits = 131,071), `OP_LOADKX` + `OP_EXTRAARG` is used instead of `OP_LOADK`.

**Trap**: Forgetting the EXTRAARG path. If you only implement `OP_LOADK`, programs with >131K unique constants will silently corrupt.

### 9.2 Maximum Locals: MAXVARS = 200

Defined at lparser.c line 35. Checked in `adjustlocalvars()` (lparser.c line 337) via `luaY_checklimit()`. This counts ALL active variables including internal ones (for-loop state variables, etc.).

**Trap**: The limit is checked against `reglevel` (register level), not `nactvar`. Compile-time constants (`RDKCTC`) don't occupy registers, so `nactvar` can exceed `reglevel`. The check is at lparser.c line 337: `luaY_checklimit(fs, reglevel, MAXVARS, "local variables")`.

### 9.3 Maximum Upvalues: MAXUPVAL = 255

Checked in `allocupvalue()` (lparser.c lines 370–379). Since `nups` is a `lu_byte`, the maximum is 255.

**Trap**: The main function's `_ENV` counts as upvalue[0]. So user functions can only have 254 additional upvalues.

### 9.4 Maximum Registers: MAX_FSTACK = MAXARG_A = 255

The register window is limited to 255 registers (A field is 8 bits). Checked in `luaK_checkstack()` (lcode.c line 479).

**Trap**: `maxstacksize` is stored as `lu_byte` in Proto. If your implementation uses `int`, you might not catch overflow until runtime.

### 9.5 Long Jump Patching

Jump offsets in the sJ format are 25 bits signed, giving a range of about ±16 million instructions. For-loop jumps use the Bx format (17 bits unsigned = 131,071 max). Checked in `fixjump()` (lcode.c line 173) and `fixforjump()` (lparser.c lines 1645–1653).

**Trap**: The error message "control structure too long" is not just theoretical — deeply nested or very large functions can hit this limit. For-loops are more constrained (17-bit Bx) than general jumps (25-bit sJ).

### 9.6 Expression as Statement Validation

In `exprstat()` (lparser.c lines 2008–2023), after parsing a `suffixedexp`, the parser checks:
- If `=` or `,` follows → assignment
- Otherwise → must be VCALL (function call as statement)

**Trap**: If you don't validate this, expressions like `a + b` would be silently accepted as statements and their results discarded.

### 9.7 The `previousinstruction()` Guard

`previousinstruction()` (lcode.c lines 117–123) returns an invalid instruction if `fs->pc <= fs->lasttarget`. This prevents optimizations (like LOADNIL merging or NOT elimination) from crossing basic block boundaries.

**Trap**: If you optimize across jump targets, you'll get wrong code for cases like:
```lua
if cond then x = nil end
x = nil  -- this LOADNIL should NOT merge with the one inside the if
```

### 9.8 The Goto/Close Swap Trick

Every goto emits `JMP` + dead `OP_CLOSE`. When resolved, if the goto needs to close upvalues, the instructions are **swapped** (lparser.c line 609). The `gt->pc` is updated to point to the new JMP position.

**Trap**: If you don't implement this swap, gotos that cross upvalue scopes will leave upvalues open, causing memory leaks or incorrect captures.

### 9.9 Repeat-Until Scope Subtlety

`repeatstat()` (lparser.c lines 1602–1624) uses TWO nested blocks:
- `bl1`: the loop block (for break)
- `bl2`: the scope block (for variables)

The condition is evaluated inside `bl2` so it can see loop body locals. But if `bl2` has upvalues, `OP_CLOSE` must be emitted before the back-jump.

**Trap**: Using only one block means the `until` condition can't reference loop body locals, breaking Lua semantics.

### 9.10 The `freereg` Invariant

At block entry (`enterblock`, lparser.c line 730): `assert(fs->freereg == luaY_nvarstack(fs))`. After every statement (lparser.c line 2144): `fs->freereg` is reset to `luaY_nvarstack(fs)`.

**Trap**: If temporaries leak across statements, register allocation will be wrong and values will be overwritten.

### 9.11 Compile-Time Constants Don't Occupy Registers

Variables with kind `RDKCTC` (compile-time constant) are inlined at use sites. They don't occupy registers. This means `nactvar` can exceed the register level, and `reglevel()` (lparser.c lines 236–243) must walk backwards to find the actual highest register.

**Trap**: If you assume `nactvar == freereg`, compile-time constants will break your register allocation.

### 9.12 The MMBIN Result Goes to RA of the Previous Instruction

The two-instruction pattern (e.g., `OP_ADD` + `OP_MMBIN`) has a subtle behavior: the metamethod result goes to the A field of the **previous** (arithmetic) instruction, not the MMBIN instruction itself. The MMBIN's A field contains the original left operand register.

**Trap**: If you put the result in the wrong register, metamethod results will be lost.

---

## 10. Bug Pattern Guide

### 10.1 Forgetting the Two-Instruction Arithmetic Pattern

**Bug**: Emitting only `OP_ADD` without the following `OP_MMBIN`.

**Symptom**: Arithmetic works for numbers but crashes or gives wrong results when metamethods are involved (e.g., adding two tables with `__add`).

**Fix**: Every arithmetic/bitwise binary op MUST emit two instructions: the operation + the metamethod fallback. The `flip` flag must be set correctly when operands are swapped.

### 10.2 Incorrect Jump List Threading

**Bug**: Using a separate data structure for jump lists instead of threading through JMP instructions.

**Symptom**: Works initially but breaks when jump lists are concatenated or when `patchlistaux` needs to distinguish value targets from default targets.

**Fix**: Thread jump lists through the sJ field of JMP instructions, exactly as the C code does. The `NO_JUMP (-1)` sentinel must be preserved.

### 10.3 Not Handling VRELOC Correctly

**Bug**: Setting the A field of a VRELOC instruction at the wrong time, or not setting it at all.

**Symptom**: Values end up in wrong registers. Function calls return to wrong locations.

**Fix**: VRELOC means "instruction emitted, destination not yet assigned." The A field is set to 0 as placeholder and must be patched when the destination register is known (in `discharge2reg`, lcode.c line 921).

### 10.4 Breaking LIFO Register Discipline

**Bug**: Freeing registers out of order (not LIFO).

**Symptom**: The `assert(reg == fs->freereg)` in `freereg()` fires. In production, registers overlap and values are corrupted.

**Fix**: Always free the higher-numbered register first. The `freeexps()` function (lcode.c lines 535–539) shows the correct pattern: free the higher of two expression registers first.

### 10.5 Forgetting `luaK_finish()` Fixups

**Bug**: Not implementing the final pass, or implementing it incompletely.

**Symptom**: Functions with to-be-closed variables crash on return. Vararg functions behave incorrectly. Jump chains are suboptimal (or wrong if they form cycles).

**Fix**: Implement all five fixups in `luaK_finish()`. Test with: to-be-closed variables + return, vararg functions, deeply nested if/else chains (jump chain optimization).

### 10.6 Incorrect Short-Circuit Evaluation

**Bug**: Not properly managing the `t` and `f` jump lists during `and`/`or` evaluation.

**Symptom**: `a and b` evaluates both sides. `a or b` doesn't short-circuit. Complex boolean expressions produce wrong values.

**Fix**: Follow the exact pattern:
- `and`: `luaK_goiftrue(e1)` → parse e2 → concatenate `e1->f` into `e2->f`
- `or`: `luaK_goiffalse(e1)` → parse e2 → concatenate `e1->t` into `e2->t`

The `infix`/`posfix` split is critical — `infix` prepares the left operand BEFORE the right is parsed.

### 10.7 Not Validating Goto Scope

**Bug**: Allowing gotos to jump into the scope of local variables.

**Symptom**: Variables are used before initialization, or upvalue captures reference uninitialized stack slots.

**Fix**: In `closegoto()`, check `gt->nactvar < label->nactvar` and error with "jump into scope." Also check whether the goto crosses upvalue-captured scopes and emit `OP_CLOSE` if needed.

### 10.8 Incorrect `_ENV` Resolution

**Bug**: Treating `_ENV` as a global variable instead of an upvalue.

**Symptom**: Global variable access fails, or `_ENV` modifications don't affect the correct scope.

**Fix**: `_ENV` is always upvalue[0] of the main function, created manually in `mainfunc()`. It's resolved through the normal upvalue chain — `singlevaraux` will find it. Global variables become `_ENV["name"]` via `buildglobal()`.

### 10.9 Forgetting the Leading Newline Strip in Long Strings

**Bug**: Not discarding the first newline after `[[` in long strings.

**Symptom**: `[[\nfoo]]` produces `"\nfoo"` instead of `"foo"`. Subtle test failures.

**Fix**: After reading the opening `[[`, check if the next character is a newline and skip it (llex.c lines 300–301).

### 10.10 Not Handling the `\z` Escape

**Bug**: Not implementing `\z` in short strings, or implementing it without handling newlines.

**Symptom**: Strings with `\z` contain unexpected whitespace, or line counting goes wrong.

**Fix**: `\z` skips ALL following whitespace including newlines. Each newline must still increment the line counter (llex.c lines 433–441).

### 10.11 Incorrect SETLIST Batch Counting

**Bug**: Using the wrong `nelems` value for SETLIST, or not handling the EXTRAARG overflow.

**Symptom**: Table constructors with >50 elements have wrong indices. Very large tables corrupt memory.

**Fix**: `nelems` is cumulative (total elements so far), not per-batch. The starting index in the array is computed from `nelems` at runtime. Large values use EXTRAARG.

### 10.12 Not Resetting `freereg` After Statements

**Bug**: Not resetting `freereg` to `nvarstack` after each statement.

**Symptom**: Temporary registers accumulate across statements, eventually hitting the 255 register limit on functions that should work fine.

**Fix**: After every `statement()` call in `statlist()`, reset `fs->freereg = luaY_nvarstack(fs)` (lparser.c line 2144–2146).

### 10.13 Comparison Operand Swapping

**Bug**: Not swapping operands for `>` and `>=`.

**Symptom**: `a > b` compiles as `LT a b` instead of `LT b a`, giving inverted results.

**Fix**: In `luaK_posfix()`, `OPR_GT` and `OPR_GE` swap their operands and convert to `OPR_LT`/`OPR_LE` (lcode.c lines 1851–1855).

### 10.14 The `OP_SHLI` Reversed Operand Trap

**Bug**: Not accounting for `OP_SHLI` having reversed operand semantics.

**Symptom**: `3 << x` computes `x << 3` instead.

**Fix**: `OP_SHLI` means "immediate << register" (not "register << immediate"). When the left operand is an immediate and right is a register, the `flip` flag is set to 1 so the metamethod receives operands in the correct order. See lcode.c lines 1828–1838 and `.analysis/05-vm-execution-loop.md`.


---

## Appendix A: Cross-Reference — Parser→CodeGen API

Every call from lparser.c into lcode.c, grouped by category:

### Expression Discharge
| lcode.c function | Purpose | Called from (lparser.c) |
|-----------------|---------|------------------------|
| `luaK_exp2nextreg()` | Push expression to next register | `exp1()`, `explist()`, `funcargs()`, `restassign()`, `fornum()` |
| `luaK_exp2anyreg()` | Reuse register or allocate | `funcargs()`, `retstat()`, various |
| `luaK_exp2anyregup()` | Keep VUPVAL as-is | `buildglobal()` |
| `luaK_exp2val()` | Minimal discharge | `recfield()`, `yindex()` |
| `luaK_dischargevars()` | Variable → value | `subexpr()`, `cond()` |
| `luaK_setreturns()` | Set call/vararg result count | `adjust_assign()`, `funcargs()` |
| `luaK_setoneret()` | Force single result | (called from lcode.c internally) |
| `luaK_setmultret()` | Set MULTRET | `constructor()`, `retstat()` |

### Variable Operations
| lcode.c function | Purpose | Called from (lparser.c) |
|-----------------|---------|------------------------|
| `luaK_indexed()` | Create indexed expression | `buildglobal()`, `fieldsel()`, `yindex()` |
| `luaK_storevar()` | Assign to variable | `restassign()`, `storevartop()` |
| `luaK_self()` | Method call setup | `suffixedexp()` |
| `luaK_codecheckglobal()` | Global nil check (5.5) | `checkglobal()` |
| `luaK_vapar2local()` | Vararg→local (5.5) | `singlevaraux()` |

### Operators
| lcode.c function | Purpose | Called from (lparser.c) |
|-----------------|---------|------------------------|
| `luaK_prefix()` | Unary operator | `subexpr()` |
| `luaK_infix()` | Binary op: prepare left | `subexpr()` |
| `luaK_posfix()` | Binary op: emit code | `subexpr()` |

### Jump/Flow Control
| lcode.c function | Purpose | Called from (lparser.c) |
|-----------------|---------|------------------------|
| `luaK_jump()` | Emit unconditional JMP | `newgotoentry()`, `test_then_block()`, `whilestat()` |
| `luaK_patchlist()` | Patch jump list to target | `closegoto()`, `whilestat()`, `repeatstat()` |
| `luaK_patchtohere()` | Patch list to current pc | `ifstat()`, `whilestat()`, `test_then_block()`, `forbody()` |
| `luaK_concat()` | Concatenate jump lists | `ifstat()` |
| `luaK_getlabel()` | Mark jump target | `whilestat()`, `repeatstat()`, `forbody()` |
| `luaK_goiftrue()` | Conditional: go if true | `cond()` |
| `luaK_goiffalse()` | Conditional: go if false | (called from lcode.c for `or`) |

### Instruction Emission
| lcode.c function | Purpose | Called from (lparser.c) |
|-----------------|---------|------------------------|
| `luaK_codeABC()` | Emit iABC instruction | `funcargs()`, `setvararg()`, `newgotoentry()`, `leaveblock()` |
| `luaK_codeABx()` | Emit iABx instruction | `forbody()`, `codeclosure()` |
| `luaK_codevABCk()` | Emit ivABC instruction | (via luaK_settablesize, luaK_setlist) |

### Register/Constant Management
| lcode.c function | Purpose | Called from (lparser.c) |
|-----------------|---------|------------------------|
| `luaK_reserveregs()` | Allocate registers | `body()`, `forbody()`, `adjustlocalvars()` |
| `luaK_checkstack()` | Check register limit | `adjust_assign()`, `forlist()` |
| `luaK_nil()` | Emit LOADNIL | `adjust_assign()` |
| `luaK_int()` | Load integer | `fornum()` |
| `luaK_exp2const()` | Try compile-time eval | `localstat()` |

### Table Construction
| lcode.c function | Purpose | Called from (lparser.c) |
|-----------------|---------|------------------------|
| `luaK_settablesize()` | Set NEWTABLE size | `constructor()` |
| `luaK_setlist()` | Emit SETLIST | `closelistfield()`, `lastlistfield()` |

### Finalization
| lcode.c function | Purpose | Called from (lparser.c) |
|-----------------|---------|------------------------|
| `luaK_ret()` | Emit return instruction | `close_func()`, `retstat()` |
| `luaK_finish()` | Final fixup pass | `close_func()` |
| `luaK_fixline()` | Fix line info | `funcargs()`, `funcstat()` |

---

## Appendix B: Lua 5.5 Changes Summary

| Feature | Files affected | Description |
|---------|---------------|-------------|
| `TK_GLOBAL` keyword | llex.h, llex.c | New reserved word "global" (with compat mode to un-reserve) |
| `LUA_COMPAT_GLOBAL` | llex.c line 191 | Clears `extra` field to un-reserve "global" |
| `VGLOBAL` expdesc kind | lparser.h | New expression kind for global variables |
| `VVARGVAR` expdesc kind | lparser.h | Named vararg parameter |
| `VVARGIND` expdesc kind | lparser.h | Indexed vararg parameter |
| `GDKREG`/`GDKCONST` | lparser.h | Variable kinds for global declarations |
| `global *` declaration | lparser.c line 1946 | Collective global declaration |
| `global x` declaration | lparser.c line 1924 | Named global declaration |
| `global function f` | lparser.c line 1956 | Global function declaration |
| `OP_ERRNNIL` | lcode.c line 715 | Runtime nil-check for global variables |
| `OP_GETVARG` | lcode.c line 1955 | Vararg table element access |
| `PF_VATAB` flag | lcode.c line 1933 | Function uses vararg table |
| Named vararg `...name` | lparser.c line 1083 | Named vararg parameter in function signature |
| Counter-based FORLOOP | lparser.c line 1694 | 2 internal vars instead of 3 (5.4 had 3) |
| `glbn` field in LexState | llex.h line 82 | "global" string for compat mode |

---

## Appendix C: Key Invariants for Testing

1. **`freereg == nvarstack` at block entry and after each statement**
2. **Jump lists are properly terminated with NO_JUMP (-1)**
3. **Every arithmetic/bitwise op has exactly 2 instructions (op + MMBIN)**
4. **VRELOC expressions have A=0 until discharged to a register**
5. **Upvalue chains: every level has either instack=1 (direct) or instack=0 (indirect)**
6. **`_ENV` is always upvalue[0] of the main function**
7. **MULTRET encoding: field value 0 = multiple returns (actual count stored as count+1)**
8. **`luaK_finish()` runs on every function, including the main function**
9. **`close_func()` shrinks all Proto arrays to exact size**
10. **For-loop jumps use Bx encoding (17-bit), not sJ encoding (25-bit)**

