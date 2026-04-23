# Changelog

## [v0.2.0] — 2026-04-23

### API Additions
- `SetHook` / `GetHook` — debug hook support (call, return, line, count hooks)
- `LoadFile` / `DoFile` — load and execute Lua files
- `cmd/glua -l` flag — preload libraries from command line

### Performance (vs C Lua 5.5.1)
- Geometric mean: **2.86×** (improved from 3.50× in v0.1.0)
- Best: Concat Multi 1.05×, Method Call 1.47×, Pattern Match 1.61×

### Performance Optimizations (Rounds 4-6)
**Round 4 (3.50× → 3.10×):**
- `strings.Join` for multi-value concat operator
- Stack-allocated string parts for small concats
- `Table.SetIfExists` skips hash insertion for existing keys

**Round 5 (3.10× → 2.73×):**
- Inline `checkGC` with countdown counter (cost 83→75)
- Integer for-loop fast path extraction (`forLoopInt` inlines at cost 43)
- Array fast paths for OP_GETI/OP_SETI (bypass tableSetWithMeta)
- Skip string interning for concat results (non-interned long strings)
- `intToString` uses `strconv.AppendInt` with stack-allocated buffer
- Fix string cross-type equality for non-interned concat results

**Round 6 (2.73× → 2.86× measured):**
- OP_GETFIELD/GETTABUP/SELF use GetStr for direct string lookup
- OP_SETFIELD/SETTABUP string fast paths (skip nil/NaN checks)
- OP_POW/POWK fast paths for x², x³, √x (avoid math.Pow)

### Documentation
- Documented all 21 SKIP annotations in test suite (all are C-only features)
- Added embedding guide (`docs/embedding-guide.md`)
- Added 6 godoc examples (Resume, Yield, SetHook, DoFile, sandbox, NewMetatable)
- Updated README performance data (3.10× → 2.86×)

### Bug Fixes
- Fix string cross-type equality for non-interned concat results
- Remove committed glua binary, add to .gitignore

## [v0.1.0] — 2026-04-22

### Features
- Full Lua 5.5.1 language implementation in pure Go (no CGo)
- 10 standard libraries: base, string, table, math, io, os, coroutine, debug, utf8, package
- Lua 5.5 additions: to-be-closed variables (`<close>`), generational GC, integer/float types
- Coroutine yield across metamethods (`__index`, `__newindex`, `__call`, etc.)
- Mark-and-sweep GC with generational mode
- testC testing library (97 C-API-level instructions)
- Public embedding API (`pkg/lua/`)

### Testing
- 29/29 official Lua 5.5.1 test suites pass
- 16+ Go packages with unit tests

### Performance (vs C Lua 5.5.1)
- Geometric mean: 3.50x
- Best: Pattern Match 1.79x, Fibonacci 1.82x
- Optimization highlights: zero-alloc numerics, table pool, LuaState pool,
  CallInfo slab, O(n²) concat fix, closure/upval pooling

### Performance Optimizations (5 rounds)
1. State caching, atomic removal, stack reslice, strong string pointers
2. Closure/upval pooling, table pool optimization, sweep consolidation
3. LuaState pool, O(n²) concat fix, CISlab reuse

### GC Correctness Fixes
- Mark L.Hook in GC traversal (db.lua fix)
- Close upvalues before thread pool return (coroutine.lua fix)
- Align white flip with C Lua single-flip design (gc.lua fix)
- Fix isDeadMark/IsDead to bitwise AND matching C Lua
- Fix isCleared/separateTobeFnz to IsWhite() matching C Lua

### Tools
- `tools/bench.sh` — Go benchmark runner with markdown output
- `tools/luabench.sh` — C Lua vs go-lua comparison
- `tools/luarunner/` — minimal go-lua CLI
- `tools/benchmarks/` — 13 Lua benchmark scripts
- `tools/vmcompare/` — opcode correctness verification
- `tools/bccompare/` — bytecode comparison
