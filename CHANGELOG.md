# Changelog

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
